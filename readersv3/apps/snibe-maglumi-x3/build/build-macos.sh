#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../.."
go run ./tools/releasectl build --app snibe-maglumi-x3 --target darwin-amd64 "$@"
