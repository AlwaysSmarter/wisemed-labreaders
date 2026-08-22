#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../../.."
go run ./tools/releasectl package --app autobio-autolumo-s900-reader --target linux-amd64 "$@"
