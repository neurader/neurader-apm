#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# loki-node-setup.sh
# Installs and starts Loki on a Grafana EC2 / Linux VM for use with neurader.
#
# Usage:
#   curl -L https://neurader.operman.in/neurader/releases/loki-node-setup.sh | sudo bash
#
# Supported OS:
#   Amazon Linux 2023, Amazon Linux 2, RHEL/CentOS 7+, Ubuntu 20.04+, Debian 11+
# ─────────────────────────────────────────────────────────────────────────────

set -euo pipefail

LOKI_VERSION="3.0.0"
LOKI_PORT="3100"
INSTALL_DIR="/usr/bin"
CONFIG_DIR="/etc/loki"
DATA_DIR="/var/loki"

# ── Colors ───────────────────────────────────────────────────────────────────
GREEN='\033[0;32m'
CYAN='\033[0;36m'
RED='\033[0;31m'
BOLD='\033[1m'
DIM='\033[2m'
RESET='\033[0m'

ok()   { echo -e "  ${GREEN}✓${RESET} $1"; }
info() { echo -e "  ${CYAN}→${RESET} $1"; }
err()  { echo -e "  ${RED}✕ ERROR:${RESET} $1" >&2; exit 1; }
step() { echo -e "\n${BOLD}$1${RESET}"; }

# ── Banner ───────────────────────────────────────────────────────────────────
echo -e "${CYAN}"
echo "  ┌──────────────────────────────────────────┐"
echo "  │  neurader  ·  Loki Node Setup             │"
echo "  │  Installs Loki for Ansible log monitoring │"
echo "  └──────────────────────────────────────────┘"
echo -e "${RESET}"

# ── Root check ───────────────────────────────────────────────────────────────
step "[1] Checking permissions"
[[ $EUID -ne 0 ]] && err "Must be run as root. Try: curl ... | sudo bash"
ok "Running as root"

# ── OS detection ─────────────────────────────────────────────────────────────
step "[2] Detecting OS"
OS_ID=""
if [[ -f /etc/os-release ]]; then
  OS_ID=$(grep '^ID=' /etc/os-release | cut -d= -f2 | tr -d '"')
fi

case "$OS_ID" in
  amzn)
    OS_VERSION=$(grep '^VERSION_ID=' /etc/os-release | cut -d= -f2 | tr -d '"' 2>/dev/null || echo "2")
    ok "Amazon Linux ${OS_VERSION}"
    ;;
  rhel|centos|fedora|rocky|almalinux)
    ok "RHEL-based: ${OS_ID}"
    ;;
  ubuntu|debian)
    ok "Debian-based: ${OS_ID}"
    ;;
  *)
    ok "OS: ${OS_ID:-unknown} — will attempt generic install"
    ;;
esac

# ── Detect architecture ───────────────────────────────────────────────────────
step "[3] Detecting architecture"
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  LOKI_ARCH="amd64" ;;
  aarch64) LOKI_ARCH="arm64" ;;
  armv7l)  LOKI_ARCH="arm"   ;;
  *) err "Unsupported architecture: ${ARCH}" ;;
esac
ok "Architecture: ${ARCH} → ${LOKI_ARCH}"

# ── Install dependencies (unzip, curl) ───────────────────────────────────────
step "[4] Checking dependencies"
install_pkg() {
  case "$OS_ID" in
    amzn|rhel|centos|fedora|rocky|almalinux)
      dnf install -y "$1" 2>/dev/null || yum install -y "$1" 2>/dev/null || true ;;
    ubuntu|debian)
      apt-get install -y "$1" 2>/dev/null || true ;;
    *)
      which "$1" &>/dev/null || true ;;
  esac
}

for pkg in curl unzip; do
  if command -v "$pkg" &>/dev/null; then
    ok "${pkg} found"
  else
    info "Installing ${pkg}..."
    install_pkg "$pkg"
    ok "${pkg} installed"
  fi
done

# ── Install Loki ──────────────────────────────────────────────────────────────
step "[5] Installing Loki ${LOKI_VERSION}"

if command -v loki &>/dev/null; then
  CURRENT=$(loki --version 2>&1 | head -1 || echo "unknown")
  ok "Loki already installed: ${CURRENT}"
else
  DOWNLOAD_URL="https://github.com/grafana/loki/releases/download/v${LOKI_VERSION}/loki-linux-${LOKI_ARCH}.zip"
  TMP_DIR=$(mktemp -d)
  info "Downloading from: ${DOWNLOAD_URL}"

  curl -L --progress-bar --fail "$DOWNLOAD_URL" -o "${TMP_DIR}/loki.zip" || \
    err "Download failed. Check internet connectivity."

  unzip -o "${TMP_DIR}/loki.zip" -d "${TMP_DIR}" > /dev/null
  mv "${TMP_DIR}/loki-linux-${LOKI_ARCH}" "${INSTALL_DIR}/loki"
  chmod +x "${INSTALL_DIR}/loki"
  rm -rf "${TMP_DIR}"

  ok "Loki installed at ${INSTALL_DIR}/loki"
fi

# ── Write Loki config ─────────────────────────────────────────────────────────
step "[6] Writing Loki config"
mkdir -p "${CONFIG_DIR}" "${DATA_DIR}/chunks" "${DATA_DIR}/rules"

cat > "${CONFIG_DIR}/loki.yaml" << LOKICFG
auth_enabled: false

server:
  http_listen_port: ${LOKI_PORT}

common:
  path_prefix: ${DATA_DIR}
  storage:
    filesystem:
      chunks_directory: ${DATA_DIR}/chunks
      rules_directory:  ${DATA_DIR}/rules
  replication_factor: 1
  ring:
    instance_addr: 127.0.0.1
    kvstore:
      store: inmemory

schema_config:
  configs:
    - from: 2024-01-01
      store: tsdb
      object_store: filesystem
      schema: v13
      index:
        prefix: index_
        period: 24h

limits_config:
  reject_old_samples: false
LOKICFG

ok "Config written to ${CONFIG_DIR}/loki.yaml"

# ── Write systemd service ─────────────────────────────────────────────────────
step "[7] Setting up systemd service"
cat > /etc/systemd/system/loki.service << SYSTEMD
[Unit]
Description=Loki log aggregation (neurader)
After=network.target

[Service]
ExecStart=${INSTALL_DIR}/loki -config.file=${CONFIG_DIR}/loki.yaml
Restart=on-failure
RestartSec=5
User=root

[Install]
WantedBy=multi-user.target
SYSTEMD

systemctl daemon-reload
systemctl enable loki
systemctl restart loki
ok "Loki service enabled and started"

# ── Firewall — open port 3100 for Ansible controller ────────────────────────
step "[8] Firewall"
if command -v firewall-cmd &>/dev/null && systemctl is-active firewalld &>/dev/null; then
  firewall-cmd --permanent --add-port="${LOKI_PORT}/tcp" > /dev/null 2>&1 || true
  firewall-cmd --reload > /dev/null 2>&1 || true
  ok "firewalld: port ${LOKI_PORT} opened"
elif command -v ufw &>/dev/null; then
  ufw allow "${LOKI_PORT}/tcp" > /dev/null 2>&1 || true
  ok "ufw: port ${LOKI_PORT} opened"
else
  info "No firewall detected — make sure port ${LOKI_PORT} is open in your EC2 security group"
fi

# ── Verify Loki is running ────────────────────────────────────────────────────
step "[9] Verifying Loki"
sleep 3
for i in 1 2 3 4 5; do
  STATUS=$(curl -s -o /dev/null -w "%{http_code}" "http://localhost:${LOKI_PORT}/ready" 2>/dev/null || echo "000")
  if [[ "$STATUS" == "200" ]]; then
    ok "Loki is ready at http://localhost:${LOKI_PORT}"
    break
  fi
  info "Waiting for Loki to start... (${i}/5)"
  sleep 2
done

if [[ "$STATUS" != "200" ]]; then
  err "Loki did not start in time. Check: journalctl -u loki -n 20"
fi

# ── Done ──────────────────────────────────────────────────────────────────────
PUBLIC_IP=$(curl -s http://169.254.169.254/latest/meta-data/public-ipv4 2>/dev/null || \
            curl -s http://checkip.amazonaws.com 2>/dev/null || \
            hostname -I | awk '{print $1}')

echo ""
echo -e "  ${GREEN}${BOLD}✅  Loki installed successfully!${RESET}"
echo ""
echo -e "  ${BOLD}Loki endpoint:${RESET}"
echo -e "  ${CYAN}http://${PUBLIC_IP}:${LOKI_PORT}${RESET}"
echo ""
echo -e "  ${DIM}Open port ${LOKI_PORT} in your EC2 security group if not already open.${RESET}"
echo -e "  ${DIM}Then on your Ansible controller, run: neurader loki-setup${RESET}"
echo ""