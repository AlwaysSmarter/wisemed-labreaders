#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../.."
go run ./tools/releasectl build --app signing-pad-utility --target darwin-amd64 "$@"
