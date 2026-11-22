#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../../.."
go run ./tools/releasectl package --app wismed-pias-ecard --target linux-amd64 "$@"
