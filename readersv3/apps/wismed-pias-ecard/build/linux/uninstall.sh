#!/usr/bin/env bash
set -euo pipefail

APP_ID="wismed-pias-ecard"
SERVICE_NAME="wisemed-wismed-pias-ecard"

sudo systemctl disable --now "${SERVICE_NAME}.service" || true
sudo rm -f "/usr/lib/systemd/system/${SERVICE_NAME}.service"
sudo systemctl daemon-reload
sudo rm -f "/usr/local/bin/${APP_ID}"
sudo rm -rf "/opt/${APP_ID}"
