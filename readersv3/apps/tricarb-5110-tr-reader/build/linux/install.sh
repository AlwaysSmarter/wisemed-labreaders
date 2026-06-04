#!/usr/bin/env bash
set -euo pipefail

APP_ID="tricarb-5110-tr-reader"
SERVICE_NAME="wisemed-tricarb-5110-tr-reader"
SOURCE_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_DIR="/opt/${APP_ID}"

sudo mkdir -p "$INSTALL_DIR"
sudo rsync -a --delete "$SOURCE_DIR/runtime/" "$INSTALL_DIR/"
sudo ln -sf "$INSTALL_DIR/${APP_ID}" "/usr/local/bin/${APP_ID}" || true
sudo install -m 0644 "$SOURCE_DIR/${APP_ID}.service" "/usr/lib/systemd/system/${SERVICE_NAME}.service"
sudo systemctl daemon-reload
sudo systemctl enable --now "${SERVICE_NAME}.service"
