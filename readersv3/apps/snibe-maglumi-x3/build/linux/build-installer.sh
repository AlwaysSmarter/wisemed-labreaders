#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../../.."
go run ./tools/releasectl package --app snibe-maglumi-x3 --target linux-amd64 "$@"
