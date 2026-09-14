#!/usr/bin/env bash
# Voxi installer — downloads and installs prebuilt voxi release binaries
# Usage:
#   curl -fsSL https://ubunatic.com/voxi/install.sh | bash
#   curl -fsSL https://ubunatic.com/voxi/install.sh | bash -s -- --modifierd

set -euo pipefail

# ANSI colors
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

# Parse arguments & flags
INSTALL_DIR="${VOXI_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${VOXI_VERSION:-}"
NO_BOOTSTRAP="${VOXI_NO_BOOTSTRAP:-0}"
PASSTHROUGH_ARGS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    -v|--version)
      VERSION="$2"
      shift 2
      ;;
    --install-dir)
      INSTALL_DIR="$2"
      shift 2
      ;;
    --no-bootstrap)
      NO_BOOTSTRAP=1
      shift
      ;;
    -h|--help)
      cat <<EOT
Voxi standalone installer

Usage:
  install.sh [options] [-- voxi install flags...]

Options:
  -v, --version <tag>    Specific version or tag to install (default: latest)
      --install-dir <dir> Destination directory for binary (default: ~/.local/bin)
      --no-bootstrap      Only download binaries, do not execute 'voxi install'
  -h, --help             Show this help message

Examples:
  curl -fsSL https://codeberg.org/ubunatic/voxi/raw/branch/main/scripts/install.sh | bash
  curl -fsSL https://codeberg.org/ubunatic/voxi/raw/branch/main/scripts/install.sh | bash -s -- --modifierd
EOT
      exit 0
      ;;
    *)
      PASSTHROUGH_ARGS+=("$1")
      shift
      ;;
  esac
done

# Check operating system
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
if [[ "$OS" != "linux" ]]; then
  log_error "Voxi requires Linux (detected: $OS)."
  exit 1
fi

# Detect architecture
RAW_ARCH="$(uname -m)"
case "$RAW_ARCH" in
  x86_64|amd64)
    ARCH="x86_64"
    ;;
  aarch64|arm64)
    ARCH="aarch64"
    ;;
  *)
    log_error "Unsupported architecture: $RAW_ARCH. Prebuilt binaries are available for x86_64 and aarch64."
    exit 1
    ;;
esac

# Check dependencies
for tool in curl tar; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    log_error "Required tool '$tool' is not installed."
    exit 1
  fi
done

# Resolve latest release tag if not specified
if [[ -z "$VERSION" ]]; then
  log_info "Discovering latest release on Codeberg..."
  REDIRECT_URL="$(curl -sI https://codeberg.org/ubunatic/voxi/releases/latest | tr -d '\r' | awk -F'/' '/[Ll]ocation:/{print $NF}' || true)"
  if [[ -n "$REDIRECT_URL" && "$REDIRECT_URL" == v* ]]; then
    TAG="$REDIRECT_URL"
  else
    # Fallback to Forgejo API
    TAG="$(curl -fsSL https://codeberg.org/api/v1/repos/ubunatic/voxi/releases/latest 2>/dev/null | awk -F'"tag_name": *"' '{print $2}' | cut -d'"' -f1 || true)"
  fi

  if [[ -z "$TAG" ]]; then
    log_error "Failed to discover latest release. Specify a version manually via VOXI_VERSION=vX.Y.Z."
    exit 1
  fi
else
  if [[ "$VERSION" == v* ]]; then
    TAG="$VERSION"
  else
    TAG="v$VERSION"
  fi
fi

RAW_VERSION="${TAG#v}"
ASSET_NAME="voxi-${RAW_VERSION}-${ARCH}-linux.tar.gz"
DOWNLOAD_BASE="https://codeberg.org/ubunatic/voxi/releases/download/${TAG}"
ASSET_URL="${DOWNLOAD_BASE}/${ASSET_NAME}"
SUMS_URL="${DOWNLOAD_BASE}/SHA256SUMS"

log_info "Installing Voxi ${TAG} (${ARCH}-linux)..."

TMP_DIR="$(mktemp -d -t voxi-install-XXXXXX)"
trap 'rm -rf "$TMP_DIR"' EXIT

log_info "Downloading ${ASSET_NAME}..."
if ! curl -fL -o "${TMP_DIR}/${ASSET_NAME}" "$ASSET_URL"; then
  log_error "Failed to download release archive from $ASSET_URL"
  exit 1
fi

if curl -fsL -o "${TMP_DIR}/SHA256SUMS" "$SUMS_URL" 2>/dev/null; then
  log_info "Verifying SHA256 checksum..."
  (
    cd "$TMP_DIR"
    if command -v sha256sum >/dev/null 2>&1; then
      grep "  ${ASSET_NAME}$" SHA256SUMS | sha256sum -c - --status || {
        log_error "SHA256 checksum verification failed!"
        exit 1
      }
    elif command -v shasum >/dev/null 2>&1; then
      grep "  ${ASSET_NAME}$" SHA256SUMS | shasum -a 256 -c - --status || {
        log_error "SHA256 checksum verification failed!"
        exit 1
      }
    fi
  )
fi

mkdir -p "$INSTALL_DIR"
log_info "Extracting binaries into ${INSTALL_DIR}..."
tar -xzf "${TMP_DIR}/${ASSET_NAME}" -C "$TMP_DIR"

install -m 0755 "${TMP_DIR}/voxi" "${INSTALL_DIR}/voxi"
if [[ -f "${TMP_DIR}/voxi-modifierd" ]]; then
  install -m 0755 "${TMP_DIR}/voxi-modifierd" "${INSTALL_DIR}/voxi-modifierd"
fi

export PATH="${INSTALL_DIR}:${PATH}"

if [[ "$NO_BOOTSTRAP" -eq 1 ]]; then
  log_ok "Voxi binary installed to ${INSTALL_DIR}/voxi"
  exit 0
fi

log_info "Running 'voxi install' to configure services and dependencies..."
"${INSTALL_DIR}/voxi" install "${PASSTHROUGH_ARGS[@]}"

"${INSTALL_DIR}/voxi" man --install || log_warn "man page install failed (non-fatal); run 'voxi man --install' manually later"

log_ok "Voxi installation complete!"
log_info "Run 'voxi monitor --watch' to inspect system status or 'voxi record toggle' to dictate."
