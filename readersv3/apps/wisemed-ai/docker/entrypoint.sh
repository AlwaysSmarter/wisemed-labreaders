#!/bin/sh
set -eu
umask 077
cp /defaults/config.install.yaml /app/deployments/config.install.yaml
if [ ! -f /app/deployments/config.yaml ]; then
 sed 's/127.0.0.1:19120/0.0.0.0:19120/g' /defaults/config.install.yaml > /app/deployments/config.yaml
fi
mkdir -p /app/deployments/help
cp /defaults/help/index.html /app/deployments/help/index.html
exec /app/wisemed-ai -config /app/deployments/config.yaml -showlog
