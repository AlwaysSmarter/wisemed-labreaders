#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../../.."
go run ./tools/releasectl package --app horiba-abx-pentra400-reader --target darwin-amd64 "$@"
