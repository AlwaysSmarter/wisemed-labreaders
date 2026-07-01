#!/usr/bin/env bash
set -euo pipefail

APP_ID="cfx96-reader"
SERVICE_NAME="wisemed-cfx96-reader"

sudo systemctl disable --now "${SERVICE_NAME}.service" || true
sudo rm -f "/usr/lib/systemd/system/${SERVICE_NAME}.service"
sudo systemctl daemon-reload
sudo rm -f "/usr/local/bin/${APP_ID}"
sudo rm -rf "/opt/${APP_ID}"
