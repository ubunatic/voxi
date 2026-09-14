#!/usr/bin/env bash
# Test Voxi curl installation workflow using Podman
set -euo pipefail

_cyan="\033[36m"
_green="\033[32m"
_yellow="\033[33m"
_red="\033[31m"
_bold="\033[1m"
_reset="\033[0m"

log_info()  { echo -e "${_cyan}${_bold}==>${_reset} $*"; }
log_ok()    { echo -e "${_green}${_bold}==>${_reset} $*"; }
log_error() { echo -e "${_red}${_bold}error:${_reset} $*" >&2; }

if ! command -v podman >/dev/null 2>&1; then
  log_error "Podman is not installed on this host."
  exit 1
fi

IMAGE_TAG="voxi-install-test"
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

log_info "Building Podman test image (${IMAGE_TAG})..."
podman build -t "${IMAGE_TAG}" -f "${PROJECT_ROOT}/test/install/Containerfile" "${PROJECT_ROOT}"

log_info "Testing go install from this checkout, then voxi install and voxi install --modifierd..."
podman run --rm --security-opt label=disable -v "${PROJECT_ROOT}:/src:ro" "${IMAGE_TAG}" bash /src/test/install/test-go-install.sh

log_info "Testing curl installer in clean container environment..."
podman run --rm --security-opt label=disable -v "${PROJECT_ROOT}/scripts/install.sh:/tmp/install.sh:ro" "${IMAGE_TAG}" bash -c '
set -euo pipefail
export PATH="/home/testuser/.local/bin:${PATH}"

mkdir -p "$HOME/bin"
cat <<STUB > "$HOME/bin/systemctl"
#!/bin/bash
echo "[stub systemctl] \$*"
exit 0
STUB
chmod +x "$HOME/bin/systemctl"
export PATH="$HOME/bin:$PATH"

echo "==> Testing curl download followed by automatic voxi install..."
bash /tmp/install.sh

# Verify installed binaries
if [[ ! -x "$HOME/.local/bin/voxi" ]]; then
  echo "❌ Error: ~/.local/bin/voxi was not installed or is not executable."
  exit 1
fi

if [[ ! -x "$HOME/.local/bin/voxi-modifierd" ]]; then
  echo "❌ Error: ~/.local/bin/voxi-modifierd was not installed or is not executable."
  exit 1
fi

echo "✅ Verified binaries installed in ~/.local/bin"
"$HOME/.local/bin/voxi" --help >/dev/null
echo "✅ Verified voxi binary runs."

# Verify CrispASR installation
if [[ ! -x "$HOME/.local/lib/voxi/crispasr/crispasr" ]]; then
  echo "❌ Error: CrispASR binary was not installed."
  exit 1
fi
echo "✅ CrispASR version: $("$HOME/.local/lib/voxi/crispasr/crispasr" --version)"

# Verify Dotool installation
if [[ ! -x "$HOME/.local/bin/dotoold" ]]; then
  echo "❌ Error: dotoold was not installed."
  exit 1
fi
echo "✅ Dotool components installed."

# Verify generated user systemd unit files
for unit in voxi-agent.service voxi-eager.service dotoold.service; do
  unit_path="$HOME/.config/systemd/user/${unit}"
  if [[ ! -f "$unit_path" ]]; then
    echo "❌ Error: Expected user unit $unit_path does not exist."
    exit 1
  fi
  grep -q "ExecStart=" "$unit_path" || {
    echo "❌ Error: Unit file $unit_path missing ExecStart."
    exit 1
  }
done

echo "✅ Verified user systemd units properly generated."

# Verify man page installed by install.sh'"'"'s `voxi man --install` step
man_path="$HOME/.local/share/man/man1/voxi.1"
if [[ ! -f "$man_path" ]]; then
  echo "❌ Error: Expected man page $man_path does not exist."
  exit 1
fi
grep -q '"'"'\.TH "VOXI"'"'"' "$man_path" || {
  echo "❌ Error: $man_path does not look like a generated voxi roff man page."
  exit 1
}
echo "✅ Verified man page installed to $man_path."

echo "==> All Podman installation tests PASSED successfully!"
'

log_ok "Podman curl install test suite completed successfully!"
