#!/bin/sh
set -eu
cd "$(dirname "$0")"
out=${WSM_OUTPUT_DIR:-dist}
version=$(date -u +%Y%m%d-%H%M%S)
mkdir -p "$out/bin" "$out/deployments" "$out/docs" "$out/examples/reader" "$out/control"
go build -trimpath -ldflags "-X main.buildVersion=$version" -o "$out/bin/wsm-server" ./cmd/wsm-server
go build -trimpath -o "$out/bin/wsmctl" ./cmd/wsmctl
cp deployments/config.yaml deployments/config.production.yaml deployments/config.http.yaml deployments/nginx-wsm.conf deployments/wsm-server.service "$out/deployments/"
cp README.md "$out/"
cp docs/*.md "$out/docs/"
cp examples/reader/main.go "$out/examples/reader/"

cp ../readersv3/modules/localhttp/ui/index.html ../readersv3/modules/localhttp/ui/app.js ../readersv3/modules/localhttp/ui/styles.css ../readersv3/modules/localhttp/ui/wss-remote.js "$out/control/"
