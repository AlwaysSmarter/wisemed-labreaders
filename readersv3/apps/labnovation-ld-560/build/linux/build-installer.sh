#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../../.."
go run ./tools/releasectl package --app labnovation-ld-560 --target linux-amd64 "$@"
