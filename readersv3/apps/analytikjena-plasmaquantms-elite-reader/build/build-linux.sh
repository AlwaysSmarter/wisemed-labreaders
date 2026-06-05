#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../.."
go run ./tools/releasectl build --app analytikjena-plasmaquantms-elite-reader --target linux-amd64 "$@"
