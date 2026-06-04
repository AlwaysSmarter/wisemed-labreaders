#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../../.."
go run ./tools/releasectl package --app anatolia-geneworks-reader --target linux-amd64 "$@"
