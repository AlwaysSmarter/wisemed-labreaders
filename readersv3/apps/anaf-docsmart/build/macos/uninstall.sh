#!/usr/bin/env bash
set -euo pipefail

APP_ID="anaf-docsmart"
BUNDLE_ID="eu.wisemed.readersv3.anaf.docsmart"

sudo launchctl bootout system "/Library/LaunchDaemons/${BUNDLE_ID}.plist" >/dev/null 2>&1 || true
sudo rm -f "/Library/LaunchDaemons/${BUNDLE_ID}.plist"
sudo rm -rf "/usr/local/${APP_ID}"
