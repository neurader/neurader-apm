#!/usr/bin/env bash
# ────────────────────────────────────────────────────────────────────────────
# neurader installer
# Usage: curl -L https://neurader.cloud/neurader/install.sh | sudo bash | 
# ────────────────────────────────────────────────────────────────────────────

set -euo pipefail

BASE_URL="https://neurader.cloud/neurader/releases/latest"
INSTALL_DIR="/usr/bin"
BINARY_NAME="neurader"
TMP_FILE="/tmp/neurader-download-$$"

# ── Colors ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
RESET='\033[0m'

ok()   { echo -e "  ${GREEN}✓${RESET} $1"; }
info() { echo -e "  ${CYAN}→${RESET} $1"; }
err()  { echo -e "  ${RED}✕ ERROR:${RESET} $1" >&2; exit 1; }
step() { echo -e "\n${BOLD}$1${RESET}"; }

# ── Banner ───────────────────────────────────────────────────────────────────
echo -e "${CYAN}"
cat << 'BANNER'
███╗   ██╗███████╗██╗   ██╗██████╗  █████╗ ██████╗ ███████╗██████╗
████╗  ██║██╔════╝██║   ██║██╔══██╗██╔══██╗██╔══██╗██╔════╝██╔══██╗
██╔██╗ ██║█████╗  ██║   ██║██████╔╝███████║██║  ██║█████╗  ██████╔╝
██║╚██╗██║██╔══╝  ██║   ██║██╔══██╗██╔══██║██║  ██║██╔══╝  ██╔══██╗
██║ ╚████║███████╗╚██████╔╝██║  ██║██║  ██║██████╔╝███████╗██║  ██║
╚═╝  ╚═══╝╚══════╝ ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚═════╝ ╚══════╝╚═╝  ╚═╝
BANNER
echo -e "${RESET}"
echo -e "  ${BOLD}Ansible Execution Monitor — Installer${RESET}"
echo -e "  ${DIM}──────────────────────────────────────${RESET}"

# ── Root check ───────────────────────────────────────────────────────────────
step "[1] Checking permissions"
if [[ $EUID -ne 0 ]]; then
  err "This installer must be run as root.\n     Try: curl -L https://neurader.cloud/neurader/install.sh | sudo bash"
fi
ok "Running as root"

# ── OS check ─────────────────────────────────────────────────────────────────
step "[2] Checking operating system"
if [[ "$(uname -s)" != "Linux" ]]; then
  err "neurader only supports Linux. Detected: $(uname -s)"
fi
ok "Linux detected"

# ── Detect architecture ───────────────────────────────────────────────────────
step "[3] Detecting architecture"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)
    BINARY_SUFFIX="amd64"
    ;;
  aarch64 | arm64)
    BINARY_SUFFIX="arm64"
    ;;
  armv7l | armv6l | arm)
    BINARY_SUFFIX="arm"
    ;;
  *)
    err "Unsupported architecture: ${ARCH}\n     Supported: x86_64 (amd64), aarch64 (arm64), armv7l (arm)"
    ;;
esac

BINARY_FILE="neurader-linux-${BINARY_SUFFIX}"
DOWNLOAD_URL="${BASE_URL}/${BINARY_FILE}"
ok "Architecture : ${ARCH} → ${BINARY_SUFFIX}"
info "Binary       : ${BINARY_FILE}"

# ── Check for curl or wget ────────────────────────────────────────────────────
step "[4] Checking download tools"
if command -v curl &>/dev/null; then
  DOWNLOADER="curl"
  ok "curl found"
elif command -v wget &>/dev/null; then
  DOWNLOADER="wget"
  ok "wget found"
else
  err "Neither curl nor wget found. Please install one and retry."
fi

# ── Download binary ───────────────────────────────────────────────────────────
step "[5] Downloading neurader"
info "URL: ${DOWNLOAD_URL}"

if [[ "$DOWNLOADER" == "curl" ]]; then
  curl -L --progress-bar --fail "$DOWNLOAD_URL" -o "$TMP_FILE" || \
    err "Download failed. Check your internet connection or try again."
else
  wget --show-progress -q "$DOWNLOAD_URL" -O "$TMP_FILE" || \
    err "Download failed. Check your internet connection or try again."
fi

if [[ ! -s "$TMP_FILE" ]]; then
  err "Downloaded file is empty. Something went wrong."
fi
ok "Download complete ($(du -sh "$TMP_FILE" | cut -f1))"

# ── Verify checksum ───────────────────────────────────────────────────────────
step "[6] Verifying checksum"
CHECKSUM_URL="${BASE_URL}/checksums.sha256"
CHECKSUM_FILE="/tmp/neurader-checksums-$$"

if [[ "$DOWNLOADER" == "curl" ]]; then
  curl -sL --fail "$CHECKSUM_URL" -o "$CHECKSUM_FILE" 2>/dev/null || true
else
  wget -q "$CHECKSUM_URL" -O "$CHECKSUM_FILE" 2>/dev/null || true
fi

if [[ -s "$CHECKSUM_FILE" ]]; then
  EXPECTED=$(grep "$BINARY_FILE" "$CHECKSUM_FILE" | awk '{print $1}')
  ACTUAL=$(sha256sum "$TMP_FILE" | awk '{print $1}')
  rm -f "$CHECKSUM_FILE"
  if [[ -n "$EXPECTED" && "$EXPECTED" == "$ACTUAL" ]]; then
    ok "Checksum verified"
  else
    rm -f "$TMP_FILE"
    err "Checksum mismatch — download may be corrupted. Please retry."
  fi
else
  info "Checksum file unavailable — skipping verification"
fi

# ── Install binary ────────────────────────────────────────────────────────────
step "[7] Installing neurader"
chmod +x "$TMP_FILE"

VERIFY_VERSION=$("$TMP_FILE" version 2>/dev/null || true)
if [[ -z "$VERIFY_VERSION" ]]; then
  rm -f "$TMP_FILE"
  err "Downloaded binary failed verification — it may be corrupted."
fi
ok "Binary verified: ${VERIFY_VERSION}"

INSTALL_PATH="${INSTALL_DIR}/${BINARY_NAME}"
if mv "$TMP_FILE" "$INSTALL_PATH" 2>/dev/null; then
  chmod +x "$INSTALL_PATH"
else
  cp "$TMP_FILE" "$INSTALL_PATH"
  chmod +x "$INSTALL_PATH"
  rm -f "$TMP_FILE"
fi
ok "Installed to ${INSTALL_PATH}"

# ── Final check ───────────────────────────────────────────────────────────────
step "[8] Verifying installation"
if command -v neurader &>/dev/null; then
  ok "$(neurader version) is ready"
else
  info "neurader installed at ${INSTALL_PATH}"
  info "Add ${INSTALL_DIR} to your PATH if needed"
fi

# ── Done ──────────────────────────────────────────────────────────────────────
echo ""
echo -e "  ${GREEN}${BOLD}✅  neurader installed successfully!${RESET}"
echo ""
echo -e "  ${BOLD}Next step:${RESET}"
echo -e "  ${CYAN}sudo neurader init${RESET}   ← run the setup wizard once"
echo ""
echo -e "  ${DIM}Then run any ansible-playbook command as normal.${RESET}"
echo -e "  ${DIM}View runs : neurader list${RESET}"
echo -e "  ${DIM}Inspect   : neurader show <filename>${RESET}"
echo ""


