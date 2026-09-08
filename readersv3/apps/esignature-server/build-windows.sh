#!/usr/bin/env bash
set -euo pipefail
app_dir="$(cd "$(dirname "$0")" && pwd)"
cd "$app_dir/../.."
go run ./tools/releasectl build --app esignature-server --target windows-386 "$@"
