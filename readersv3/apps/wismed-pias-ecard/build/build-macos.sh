#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../.."
go run ./tools/releasectl build --app wismed-pias-ecard --target darwin-amd64 "$@"
