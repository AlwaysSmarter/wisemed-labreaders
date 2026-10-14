#!/bin/sh
set -eu
# Refresh defaults for new image versions; the runner preserves existing config.yaml.
mkdir -p /app/deployments
cp /opt/wisemed/defaults/config.install.yaml /app/deployments/config.install.yaml
exec /opt/wisemed/bin/html-print-formatter "$@"
