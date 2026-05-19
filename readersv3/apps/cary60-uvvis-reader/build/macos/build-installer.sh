#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../../.."
go run ./tools/releasectl package --app cary60-uvvis-reader --target darwin-amd64 "$@"
