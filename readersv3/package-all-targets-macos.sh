#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "This script must be run on macOS." >&2
  exit 1
fi

cd "$SCRIPT_DIR"

echo "[package-all-targets-macos] Building all readersv3 apps for all targets..."
go run ./tools/releasectl build-all "$@"

echo "[package-all-targets-macos] Packaging native macOS installers for darwin-amd64 and darwin-arm64..."
go run ./tools/releasectl package-all "$@"

echo "[package-all-targets-macos] Release artifacts saved under:"
echo "  $SCRIPT_DIR/release/<app>/darwin-amd64"
echo "  $SCRIPT_DIR/release/<app>/darwin-arm64"
echo
echo "Note: Windows and Linux installers still require native packaging tools on those platforms."
