#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../../.."
go run ./tools/releasectl package --app shimatzu-tocl-reader --target linux-amd64 "$@"
