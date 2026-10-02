#!/bin/sh
# Prepare conventional output/deployments/config.yaml without overwriting settings.
set -eu
cd "$(dirname "$0")"
umask 077
mkdir -p output/deployments/keys output/control
if [ ! -e output/deployments/keys/local-backend.key ]; then
  if [ -f output/deployments/local/backend.key ]; then
    cp output/deployments/local/backend.key output/deployments/keys/local-backend.key
  else
    openssl rand -base64 32 > output/deployments/keys/local-backend.key
  fi
  chmod 600 output/deployments/keys/local-backend.key
fi
if [ ! -e output/deployments/config.yaml ]; then
  cp deployments/config.yaml output/deployments/config.yaml
fi
for asset in index.html app.js styles.css wss-remote.js; do
  cp "../readersv3/modules/localhttp/ui/$asset" "output/control/$asset"
done
echo "Prepared output/deployments/config.yaml and output/control; existing configuration preserved."
echo "GoLand working directory: wsm-server/output"
echo "Program arguments: -config deployments/config.yaml --showlog"
