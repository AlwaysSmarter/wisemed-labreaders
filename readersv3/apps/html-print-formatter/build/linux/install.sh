#!/usr/bin/env bash
set -euo pipefail

APP_ID="html-print-formatter"
SERVICE_NAME="wisemed-html-print-formatter"
SOURCE_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_DIR="/opt/${APP_ID}"

sudo mkdir -p "$INSTALL_DIR"
sudo rsync -a --delete "$SOURCE_DIR/runtime/" "$INSTALL_DIR/"
sudo ln -sf "$INSTALL_DIR/${APP_ID}" "/usr/local/bin/${APP_ID}" || true
sudo install -m 0644 "$SOURCE_DIR/${APP_ID}.service" "/usr/lib/systemd/system/${SERVICE_NAME}.service"
id html-print-formatter >/dev/null 2>&1 || sudo useradd --system --user-group --home-dir /opt/html-print-formatter --shell /usr/sbin/nologin html-print-formatter
sudo chown -R html-print-formatter:html-print-formatter "$INSTALL_DIR"
sudo systemctl daemon-reload
sudo systemctl enable --now "${SERVICE_NAME}.service"
