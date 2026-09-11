#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../.."
go run ./tools/releasectl build --app erba-mannheim-laura-reader --target darwin-amd64 "$@"
