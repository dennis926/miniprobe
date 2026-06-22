#!/usr/bin/env bash
# =============================================================================
# MiniProbe Server — One-Click Installer (Linux x86_64 / arm64)
#
# Usage:
#   bash install_server.sh [--port 8080] [--token mysecret]
#
# What it does:
#   1. Installs Go (if not present)
#   2. Builds the miniprobe-server binary
#   3. Creates & starts a systemd service
# =============================================================================
set -euo pipefail

PORT=8080
TOKEN=miniprobe

for arg in "$@"; do
  case $arg in
    --port=*)  PORT="${arg#*=}"  ;;
    --token=*) TOKEN="${arg#*=}" ;;
  esac
done

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'
info()  { echo -e "${BLUE}[*]${NC} $*"; }
ok()    { echo -e "${GREEN}[+]${NC} $*"; }
warn()  { echo -e "${YELLOW}[!]${NC} $*"; }
fatal() { echo -e "${RED}[✗]${NC} $*" >&2; exit 1; }

echo -e "${GREEN}"
echo "  ███╗   ███╗██╗███╗   ██╗██╗██████╗ ██████╗  ██████╗ ██████╗ ███████╗"
echo "  ████╗ ████║██║████╗  ██║██║██╔══██╗██╔══██╗██╔═══██╗██╔══██╗██╔════╝"
echo "  ██╔████╔██║██║██╔██╗ ██║██║██████╔╝██████╔╝██║   ██║██████╔╝█████╗  "
echo "  ██║╚██╔╝██║██║██║╚██╗██║██║██╔═══╝ ██╔══██╗██║   ██║██╔══██╗██╔══╝  "
echo "  ██║ ╚═╝ ██║██║██║ ╚████║██║██║     ██║  ██║╚██████╔╝██████╔╝███████╗"
echo "  ╚═╝     ╚═╝╚═╝╚═╝  ╚═══╝╚═╝╚═╝     ╚═╝  ╚═╝ ╚═════╝ ╚═════╝ ╚══════╝"
echo -e "${NC}"
info "Starting MiniProbe Server installation..."

# ── 1. Install Go ──────────────────────────────────────────────────────────────
if ! command -v go &>/dev/null; then
  warn "Go not found — installing Go 1.22 ..."
  ARCH=$(uname -m)
  case $ARCH in
    x86_64)  GO_ARCH=amd64 ;;
    aarch64) GO_ARCH=arm64 ;;
    *) fatal "Unsupported architecture: $ARCH" ;;
  esac
  GO_URL="https://go.dev/dl/go1.22.4.linux-${GO_ARCH}.tar.gz"
  info "Downloading $GO_URL ..."
  curl -fsSL "$GO_URL" | sudo tar -xz -C /usr/local
  export PATH=$PATH:/usr/local/go/bin
  echo 'export PATH=$PATH:/usr/local/go/bin' | sudo tee /etc/profile.d/go.sh > /dev/null
fi
ok "Go: $(go version)"

# ── 2. Build ───────────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_DIR="$(dirname "$SCRIPT_DIR")/server"
INSTALL_DIR="/opt/miniprobe"

[[ -d "$SRC_DIR" ]] || fatal "server/ directory not found at $SRC_DIR"

info "Building miniprobe-server ..."
sudo mkdir -p "$INSTALL_DIR"
sudo cp -r "$SRC_DIR"/. "$INSTALL_DIR/"
cd "$INSTALL_DIR"
sudo go mod tidy
sudo CGO_ENABLED=0 go build -ldflags="-s -w" -o miniprobe-server .
ok "Build complete: $INSTALL_DIR/miniprobe-server"

# ── 3. Systemd service ─────────────────────────────────────────────────────────
info "Creating systemd service ..."
sudo tee /etc/systemd/system/miniprobe.service > /dev/null <<EOF
[Unit]
Description=MiniProbe Server
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/miniprobe-server -port ${PORT} -token ${TOKEN}
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=miniprobe

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now miniprobe
ok "Service started: systemctl status miniprobe"

# ── Summary ────────────────────────────────────────────────────────────────────
HOST_IP=$(hostname -I 2>/dev/null | awk '{print $1}' || echo "YOUR_IP")
echo ""
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}  MiniProbe Server installed and running!${NC}"
echo ""
echo -e "  Dashboard  : ${BLUE}http://${HOST_IP}:${PORT}${NC}"
echo -e "  Token      : ${YELLOW}${TOKEN}${NC}"
echo ""
echo -e "  Agent command (run on each server to monitor):"
echo -e "  ${YELLOW}python3 agent/agent.py -s ws://${HOST_IP}:${PORT}/ws -t ${TOKEN}${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
