#!/usr/bin/env bash
# Headless end-to-end integration test for Voxi in container (Issue 105)
set -euo pipefail

_cyan="\033[36m"
_green="\033[32m"
_yellow="\033[33m"
_red="\033[31m"
_bold="\033[1m"
_reset="\033[0m"

log_info()  { echo -e "${_cyan}${_bold}==>${_reset} $*"; }
log_ok()    { echo -e "${_green}${_bold}==>${_reset} $*"; }
log_warn()  { echo -e "${_yellow}${_bold}warning:${_reset} $*"; }
log_error() { echo -e "${_red}${_bold}error:${_reset} $*" >&2; }

if ! command -v podman >/dev/null 2>&1; then
  log_error "Podman is not installed on this host."
  exit 1
fi

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGE_TAG="voxi-e2e-test"

# Ensure local voxi binary is built and up to date
log_info "Building fresh voxi binary..."
(cd "${PROJECT_ROOT}" && go build -o voxi ./cmd/voxi)

# Build/check container image
log_info "Ensuring Podman test image (${IMAGE_TAG})..."
podman build -t "${IMAGE_TAG}" -f "${PROJECT_ROOT}/test/e2e/Containerfile" "${PROJECT_ROOT}"

# Prepare mount arguments for host caches if available
VOLUMES=(-v "${PROJECT_ROOT}:/workspace:ro")

if [[ -d "${HOME}/.cache/voxi" ]]; then
  VOLUMES+=(-v "${HOME}/.cache/voxi:/home/testuser/.cache/voxi:ro")
fi

if [[ -d "${HOME}/.local/lib/voxi/crispasr" ]]; then
  VOLUMES+=(-v "${HOME}/.local/lib/voxi/crispasr:/home/testuser/.local/lib/voxi/crispasr:ro")
fi

if [[ -d "${HOME}/go/bin" ]]; then
  VOLUMES+=(-v "${HOME}/go/bin:/tmp/host-gobin:ro")
fi

log_info "Running headless end-to-end integration test in container..."

podman run --rm "${VOLUMES[@]}" "${IMAGE_TAG}" bash -c '
set -euo pipefail

export HOME="/home/testuser"
export USER="testuser"
export PATH="/home/testuser/.local/lib/voxi/crispasr:/tmp/host-gobin:/workspace:${PATH}"
export XDG_RUNTIME_DIR="/tmp/runtime"
export DOTOOL_PIPE="/tmp/dotool-pipe"
mkdir -p -m 0700 "$XDG_RUNTIME_DIR"

echo "==> 1. Initializing isolated virtual audio daemon (PulseAudio)..."
pulseaudio --daemonize=yes --exit-idle-time=-1 --system=false --disallow-exit
pactl info | grep -E "(Server Name|Default Sink|Default Source)"

echo "==> 2. Initializing isolated dotool typing sink..."
rm -f "$DOTOOL_PIPE"
mkfifo "$DOTOOL_PIPE"

mkdir -p /tmp/test-output
TYPED_LOG="/tmp/test-output/typed.log"
rm -f "$TYPED_LOG"

# Persistent O_RDWR FIFO listener (non-blocking across multiple dotoolc connections)
python3 -c "
import os, sys
fd = os.open(\"/tmp/dotool-pipe\", os.O_RDWR)
with open(fd, \"r\") as f:
    for line in f:
        sys.stdout.write(line)
        sys.stdout.flush()
" > "$TYPED_LOG" &
SINK_PID=$!
trap "kill -9 $SINK_PID 2>/dev/null || true" EXIT

# Ensure dotoolc is available
if ! command -v dotoolc >/dev/null 2>&1; then
  mkdir -p "$HOME/go/bin"
  go install git.sr.ht/~geb/dotool@latest
  dotoold_dir="$(go list -m -f "{{.Dir}}" git.sr.ht/~geb/dotool@latest)"
  install -m 0755 "$dotoold_dir/dotoold" "$dotoold_dir/dotoolc" "$HOME/go/bin/"
fi

# Ensure crispasr is available
if ! command -v crispasr >/dev/null 2>&1; then
  echo "Downloading CrispASR..."
  arch="$(uname -m)"
  case "$arch" in
    x86_64) asset=crispasr-linux-x86_64.tar.gz ;;
    aarch64|arm64) asset=crispasr-linux-arm64.tar.gz ;;
    *) echo "Unsupported architecture $arch"; exit 1 ;;
  esac
  dir="$HOME/.local/lib/voxi/crispasr"
  mkdir -p "$dir"
  curl -fsSL "https://github.com/CrispStrobe/CrispASR/releases/latest/download/${asset}" | tar -xz -C "$dir" --strip-components=1
  export PATH="$dir:$PATH"
fi

echo "✅ Audio & Typing infrastructure ready."

echo "==> 3. Running eager dictation pipeline against fixture (short-one-two.wav)..."
FIXTURE="/workspace/test/fixtures/short-one-two.wav"
if [[ ! -f "$FIXTURE" ]]; then
  echo "❌ Error: Fixture $FIXTURE not found!"
  exit 1
fi

VOXI_LOG="/tmp/test-output/voxi.log"
/workspace/voxi eager --threshold 100 --silence 500 > "$VOXI_LOG" 2>&1 &
VOXI_PID=$!

# Wait for eager session to start listening
sleep 1.2

# Play synthetic audio fixture into virtual microphone
echo "  -> Playing $FIXTURE into virtual microphone..."
paplay "$FIXTURE"

# Allow processing & transcription to complete
sleep 2.5
kill -TERM $VOXI_PID 2>/dev/null || true
pkill -TERM arecord 2>/dev/null || true

echo "==> 4. Verifying pipeline outputs and deterministic transcription..."
echo "--- Voxi Output ---"
cat "$VOXI_LOG"
echo "-------------------"

echo "--- Voxi Chunks ---"
/workspace/voxi chunks list
echo "-------------------"

# Validate that expected words were captured in chunks
CHUNKS_JSON="/tmp/runtime/voxi/chunks/manifest.json"
if [[ ! -f "$CHUNKS_JSON" ]]; then
  echo "❌ Error: Chunk manifest $CHUNKS_JSON not found!"
  exit 1
fi

if ! grep -qi "One" "$CHUNKS_JSON" || ! grep -qi "Two" "$CHUNKS_JSON"; then
  echo "❌ Error: Expected utterances (One, Two) missing from chunk manifest!"
  exit 1
fi

echo "✅ Verified: Virtual audio streamed, VAD segmented utterances, CrispASR neural network transcribed deterministic output, and ring buffer recorded chunks!"
'

log_ok "Headless end-to-end integration test suite PASSED successfully!"
