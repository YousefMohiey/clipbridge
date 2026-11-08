#!/usr/bin/env bash
# Installs the ClipBridge host for the current user. No root needed.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
TARGET="$HOME/.local/share/clipbridge"
UNIT_DIR="$HOME/.config/systemd/user"

if ! command -v python3 >/dev/null; then
  echo "python3 is required" >&2
  exit 1
fi

if ! command -v xclip >/dev/null && [ ! -x "$HOME/.local/opt/xclip/usr/bin/xclip" ]; then
  echo "xclip is required for the clipboard on X11:"
  echo "  sudo apt install -y xclip"
  exit 1
fi

mkdir -p "$TARGET" "$UNIT_DIR"
cp "$HERE/clipd.py" "$TARGET/clipd.py"
cp "$HERE/clipbridge.service" "$UNIT_DIR/clipbridge.service"

systemctl --user daemon-reload
systemctl --user enable --now clipbridge.service
sleep 1

ADDR=$(hostname -I | awk '{print $1}')
if curl -fsS --max-time 3 "http://127.0.0.1:1717/health" >/dev/null; then
  echo
  echo "ClipBridge host is running."
  echo "  address : http://$ADDR:1717"
  echo "  status  : systemctl --user status clipbridge"
  echo "  logs    : journalctl --user -u clipbridge -f"
else
  echo "Service did not answer on port 1717. Check: journalctl --user -u clipbridge -n 40" >&2
  exit 1
fi
