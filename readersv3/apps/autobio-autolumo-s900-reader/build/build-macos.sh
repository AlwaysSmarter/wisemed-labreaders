#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../.."
go run ./tools/releasectl build --app autobio-autolumo-s900-reader --target darwin-amd64 "$@"
