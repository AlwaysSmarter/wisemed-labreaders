#!/bin/sh
# Repeatable local verification. Uses temporary fixtures, never live equipment.
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
(cd "$root/wsm-server" && go test -race ./... && go vet ./...)
(cd "$root/readersv3" && go test -race ./... && go vet ./... && go build ./apps/...)
node --test "$root/readersv3/modules/localhttp/ui/wss-remote.test.cjs" "$root/readersv3/modules/localhttp/ui/wss-ui.test.cjs"
node --test "$root/wsm-server/internal/server/adminui/app.test.cjs"
# Runs real TLS sockets and waits for the actual 30-second reader reconnect.
(cd "$root/wsm-server/integration" && go test -race -count=1 ./... && go vet ./...)
