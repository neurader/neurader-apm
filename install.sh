#!/usr/bin/env bash
# neurader install.sh
# Usage: curl -fsSL https://neurader.operman.in/releases/install.sh | sudo bash
set -euo pipefail

REPO="yourorg/neurader"
INSTALL_DIR="/usr/local/bin"

RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'
BOLD='\033[1m'; RESET='\033[0m'

info() { echo -e "${CYAN}  →${RESET} $*"; }
ok()   { echo -e "${GREEN}  ✓${RESET} $*"; }
fail() { echo -e "${RED}  ✗ ERROR:${RESET} $*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || fail "Run as root: curl ... | sudo bash"

# ── Detect architecture ───────────────────────────────────────────────────────
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)           ARCH_LABEL="amd64" ;;
  aarch64|arm64)    ARCH_LABEL="arm64" ;;
  armv7l|armv6l)    ARCH_LABEL="arm"   ;;
  *) fail "Unsupported architecture: $ARCH" ;;
esac

[ "$(uname -s)" = "Linux" ] || fail "neurader only supports Linux"

echo ""
echo -e "${BOLD}  Neurader Installer${RESET}"
echo "  ─────────────────────────────"
info "Architecture : $ARCH ($ARCH_LABEL)"

# ── Latest version ────────────────────────────────────────────────────────────
info "Fetching latest release..."
VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
  | grep '"tag_name"' | head -1 | cut -d'"' -f4)
[ -n "$VERSION" ] || fail "Could not determine latest version"
info "Version      : $VERSION"

# ── Download ──────────────────────────────────────────────────────────────────
TARBALL="neurader_${VERSION}_linux_${ARCH_LABEL}.tar.gz"
BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

info "Downloading $TARBALL..."
curl -fsSL --progress-bar "${BASE_URL}/${TARBALL}" -o "${TMP}/${TARBALL}"
curl -fsSL "${BASE_URL}/checksums.sha256" -o "${TMP}/checksums.sha256"

# ── Verify checksum ───────────────────────────────────────────────────────────
info "Verifying checksum..."
cd "$TMP"
grep "$TARBALL" checksums.sha256 | sha256sum -c - 2>/dev/null \
  || fail "Checksum verification failed"
ok "Checksum verified"

# ── Install ───────────────────────────────────────────────────────────────────
tar -xzf "$TARBALL"
install -m 0755 "neurader-linux-${ARCH_LABEL}" "${INSTALL_DIR}/neurader"
ok "Installed → ${INSTALL_DIR}/neurader"

echo ""
echo -e "${GREEN}${BOLD}  neurader ${VERSION} installed!${RESET}"
echo ""
echo "  Next step:"
echo -e "  ${BOLD}sudo neurader init${RESET}"
echo ""
