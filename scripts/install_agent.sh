#!/usr/bin/env bash
# =============================================================================
# MiniProbe Agent — Quick Installer
# Installs the agent as a systemd user service (no root required).
#
# Usage:
#   bash install_agent.sh -s ws://SERVER_IP:8080/ws -t TOKEN [-i 3]
#
# One-liner (copy agent.py to remote server first):
#   scp agent/agent.py user@host:~/ && ssh user@host \
#     "bash -s -- -s ws://SERVER:8080/ws -t TOKEN" < scripts/install_agent.sh
# =============================================================================
set -euo pipefail

SERVER="ws://localhost:8080/ws"
TOKEN="miniprobe"
INTERVAL=3

while [[ $# -gt 0 ]]; do
  case $1 in
    -s|--server)   SERVER="$2";   shift 2 ;;
    -t|--token)    TOKEN="$2";    shift 2 ;;
    -i|--interval) INTERVAL="$2"; shift 2 ;;
    *) shift ;;
  esac
done

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'
info() { echo -e "${BLUE}[*]${NC} $*"; }
ok()   { echo -e "${GREEN}[+]${NC} $*"; }

info "MiniProbe Agent Installer"
info "Server: $SERVER  Token: $TOKEN  Interval: ${INTERVAL}s"

# ── Check Python ───────────────────────────────────────────────────────────────
PYTHON=""
for cmd in python3 python; do
  if command -v "$cmd" &>/dev/null; then
    PYTHON="$cmd"; break
  fi
done
if [[ -z "$PYTHON" ]]; then
  info "Python not found — trying to install ..."
  if command -v apt-get &>/dev/null; then
    sudo apt-get update -qq && sudo apt-get install -y python3 python3-pip -q
    PYTHON=python3
  elif command -v yum &>/dev/null; then
    sudo yum install -y python3 python3-pip -q
    PYTHON=python3
  else
    echo "Please install Python 3 manually."; exit 1
  fi
fi
ok "Python: $($PYTHON --version)"

# ── Copy agent script ──────────────────────────────────────────────────────────
AGENT_DIR="$HOME/.miniprobe"
mkdir -p "$AGENT_DIR"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AGENT_SRC="$(dirname "$SCRIPT_DIR")/agent/agent.py"

if [[ -f "$AGENT_SRC" ]]; then
  cp "$AGENT_SRC" "$AGENT_DIR/agent.py"
  ok "Copied agent.py to $AGENT_DIR/"
elif [[ -f "$HOME/agent.py" ]]; then
  cp "$HOME/agent.py" "$AGENT_DIR/agent.py"
  ok "Used agent.py from HOME"
else
  echo "agent.py not found. Please place it in $AGENT_DIR/agent.py"; exit 1
fi

# ── Install Python deps ────────────────────────────────────────────────────────
info "Installing Python dependencies ..."
"$PYTHON" -m pip install psutil websocket-client -q
ok "Dependencies installed"

# ── Systemd user service (no root) ────────────────────────────────────────────
if command -v systemctl &>/dev/null; then
  SVC_DIR="$HOME/.config/systemd/user"
  mkdir -p "$SVC_DIR"
  PYTHON_PATH="$(command -v $PYTHON)"

  cat > "$SVC_DIR/miniprobe-agent.service" <<EOF
[Unit]
Description=MiniProbe Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${PYTHON_PATH} ${AGENT_DIR}/agent.py -s ${SERVER} -t ${TOKEN} -i ${INTERVAL}
Restart=always
RestartSec=10
Environment="PYTHONUNBUFFERED=1"

[Install]
WantedBy=default.target
EOF

  systemctl --user daemon-reload
  systemctl --user enable --now miniprobe-agent
  ok "Agent service started"

  echo ""
  echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo -e "${GREEN}  MiniProbe Agent installed!${NC}"
  echo ""
  echo -e "  Status  : ${YELLOW}systemctl --user status miniprobe-agent${NC}"
  echo -e "  Logs    : ${YELLOW}journalctl --user -u miniprobe-agent -f${NC}"
  echo -e "  Stop    : ${YELLOW}systemctl --user stop miniprobe-agent${NC}"
  echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
else
  # Fallback: run directly
  info "systemctl not available — running agent directly ..."
  ok "Agent starting (Ctrl+C to stop) ..."
  exec "$PYTHON" "$AGENT_DIR/agent.py" -s "$SERVER" -t "$TOKEN" -i "$INTERVAL"
fi
