#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../.."
go run ./tools/releasectl build --app biomerieux-minividas-reader --target linux-amd64 "$@"
