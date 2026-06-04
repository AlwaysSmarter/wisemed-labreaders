#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../.."
go run ./tools/releasectl build --app biosan-hipo-mpp-96-reader --target linux-amd64 "$@"
