#!/usr/bin/env bash
set -euo pipefail

APP_ID="tricarb-5110-tr-reader"
SERVICE_NAME="wisemed-tricarb-5110-tr-reader"

sudo systemctl disable --now "${SERVICE_NAME}.service" || true
sudo rm -f "/usr/lib/systemd/system/${SERVICE_NAME}.service"
sudo systemctl daemon-reload
sudo rm -f "/usr/local/bin/${APP_ID}"
sudo rm -rf "/opt/${APP_ID}"
