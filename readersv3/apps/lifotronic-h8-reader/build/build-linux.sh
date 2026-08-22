#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../.."
go run ./tools/releasectl build --app lifotronic-h8-reader --target linux-amd64 "$@"
