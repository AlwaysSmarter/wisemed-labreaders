#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../.."
go run ./tools/releasectl build --app labnovation-ld-560 --target darwin-amd64 "$@"
