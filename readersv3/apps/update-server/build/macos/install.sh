#!/usr/bin/env bash
set -euo pipefail

APP_ID="update-server"
BUNDLE_ID="eu.wisemed.readersv3.update.server"
SOURCE_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_DIR="/usr/local/${APP_ID}"

sudo mkdir -p "$INSTALL_DIR"
sudo rsync -a --delete "$SOURCE_DIR/runtime/" "$INSTALL_DIR/"
sudo install -m 0644 "$SOURCE_DIR/${BUNDLE_ID}.plist" "/Library/LaunchDaemons/${BUNDLE_ID}.plist"
sudo launchctl bootout system "/Library/LaunchDaemons/${BUNDLE_ID}.plist" >/dev/null 2>&1 || true
sudo launchctl bootstrap system "/Library/LaunchDaemons/${BUNDLE_ID}.plist"
sudo launchctl enable "system/${BUNDLE_ID}"
