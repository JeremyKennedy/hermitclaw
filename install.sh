#!/usr/bin/env bash
set -euo pipefail

# hermitclaw installer
# Downloads or copies the hermitclaw binary and sets up config.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_DIR="${HOME}/.local/bin"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/hermitclaw"
SYSTEMD_FLAG=false

for arg in "$@"; do
  case "$arg" in
    --systemd) SYSTEMD_FLAG=true ;;
    --help|-h)
      echo "Usage: install.sh [--systemd]"
      echo ""
      echo "  --systemd  Also install a systemd user service"
      exit 0
      ;;
    *) echo "Unknown argument: $arg" >&2; exit 1 ;;
  esac
done

echo "=== Checking dependencies ==="
missing=0

if command -v tmux &>/dev/null; then
  echo "  tmux: OK ($(tmux -V))"
else
  echo "  tmux: MISSING — install tmux first"
  missing=1
fi

if command -v claude &>/dev/null; then
  echo "  claude: OK"
else
  echo "  claude: MISSING — install Claude Code first (https://claude.ai/code)"
  missing=1
fi

if [ "$missing" -gt 0 ]; then
  echo ""
  echo "Install missing dependencies and try again."
  exit 1
fi

if [[ ":$PATH:" != *":${INSTALL_DIR}:"* ]]; then
  echo ""
  echo "WARNING: ${INSTALL_DIR} is not in your PATH."
  echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
  echo ""
fi

# Build or copy binary
echo ""
echo "=== Installing hermitclaw ==="
mkdir -p "$INSTALL_DIR"

if [ -f "$SCRIPT_DIR/hermitclaw" ] && file "$SCRIPT_DIR/hermitclaw" | grep -q "ELF"; then
  cp "$SCRIPT_DIR/hermitclaw" "$INSTALL_DIR/hermitclaw"
elif command -v go &>/dev/null; then
  echo "  Building from source..."
  (cd "$SCRIPT_DIR" && CGO_ENABLED=0 go build -o "$INSTALL_DIR/hermitclaw" .)
else
  echo "  ERROR: No pre-built binary found and Go is not installed."
  echo "  Either build hermitclaw first (go build) or install Go."
  exit 1
fi

chmod +x "$INSTALL_DIR/hermitclaw"
echo "  Installed: ${INSTALL_DIR}/hermitclaw"

# Create config
echo ""
echo "=== Configuration ==="
mkdir -p "$CONFIG_DIR"
if [ ! -f "$CONFIG_DIR/config.toml" ]; then
  cp "$SCRIPT_DIR/config.example.toml" "$CONFIG_DIR/config.toml"
  echo "  Created: ${CONFIG_DIR}/config.toml"
  echo "  Edit this file to customize hermitclaw."
else
  echo "  Config exists: ${CONFIG_DIR}/config.toml (not overwritten)"
fi

# Systemd service
if [ "$SYSTEMD_FLAG" = true ]; then
  echo ""
  echo "=== Installing systemd service ==="
  UNIT_DIR="${HOME}/.config/systemd/user"
  mkdir -p "$UNIT_DIR"
  cat > "$UNIT_DIR/hermitclaw.service" <<UNIT
[Unit]
Description=Hermitclaw persistent Claude Code runner
After=default.target

[Service]
Type=oneshot
RemainAfterExit=true
Environment=TMUX_TMPDIR=%t
ExecStart=${INSTALL_DIR}/hermitclaw start
ExecStop=${INSTALL_DIR}/hermitclaw stop
UNIT
  systemctl --user daemon-reload
  echo "  Installed: ${UNIT_DIR}/hermitclaw.service"
  echo "  Enable with: systemctl --user enable --now hermitclaw"
fi

echo ""
echo "=== Done ==="
echo ""
echo "Quick start:"
echo "  hermitclaw              # Start and attach"
echo "  hermitclaw start        # Start in background"
echo "  hermitclaw attach       # Attach to running session"
echo "  hermitclaw stop         # Stop session"
echo "  hermitclaw fresh        # Start new conversation"
echo ""
echo "Config: ${CONFIG_DIR}/config.toml"
