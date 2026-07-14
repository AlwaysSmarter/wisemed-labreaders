#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../../.."
go run ./tools/releasectl package --app signing-pad-utility --target darwin-amd64 "$@"
