#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../.."
go run ./tools/releasectl build --app wisemed-siui-data --target darwin-amd64 "$@"
