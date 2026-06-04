#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../.."
go run ./tools/releasectl build --app tricarb-5110-tr-reader --target darwin-amd64 "$@"
