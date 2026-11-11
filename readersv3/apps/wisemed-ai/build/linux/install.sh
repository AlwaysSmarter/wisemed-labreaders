#!/usr/bin/env bash
set -euo pipefail

APP_ID="wisemed-ai"
SERVICE_NAME="wisemed-ai"
SOURCE_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_DIR="/opt/${APP_ID}"

sudo mkdir -p "$INSTALL_DIR"
sudo rsync -a "$SOURCE_DIR/runtime/" "$INSTALL_DIR/"
sudo ln -sf "$INSTALL_DIR/${APP_ID}" "/usr/local/bin/${APP_ID}" || true
sudo install -m 0644 "$SOURCE_DIR/${APP_ID}.service" "/usr/lib/systemd/system/${SERVICE_NAME}.service"
id wisemed-ai >/dev/null 2>&1 || sudo useradd --system --user-group --home-dir /opt/wisemed-ai --shell /usr/sbin/nologin wisemed-ai
sudo chown -R wisemed-ai:wisemed-ai "$INSTALL_DIR"
sudo systemctl daemon-reload
sudo systemctl enable --now "${SERVICE_NAME}.service"
