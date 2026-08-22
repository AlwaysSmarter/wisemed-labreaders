#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../../.."
go run ./tools/releasectl package --app biomerieux-minividas-reader --target linux-amd64 "$@"
