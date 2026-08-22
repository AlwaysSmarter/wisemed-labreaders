#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../.."
go run ./tools/releasectl build-all --app horiba-yumizen-h500-reader "$@"
