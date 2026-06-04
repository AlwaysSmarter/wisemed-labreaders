#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../../.."
go run ./tools/releasectl package --app biosan-hipo-mpp-96-reader --target darwin-amd64 "$@"
