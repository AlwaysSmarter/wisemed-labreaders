#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "This script must be run on macOS." >&2
  exit 1
fi

cd "$SCRIPT_DIR"

echo "[build-all-targets-macos] Building all readersv3 apps for all targets..."
go run ./tools/releasectl build-all "$@"

echo "[build-all-targets-macos] Build outputs saved under:"
echo "  $SCRIPT_DIR/dist/<target>/<app>/runtime"
