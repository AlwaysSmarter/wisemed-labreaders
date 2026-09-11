#!/usr/bin/env bash
# Build a native reader and populate its local development workspace.
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
APP="${1:?Usage: prepare-reader-output.sh <reader>}"
[[ "$APP" =~ ^[a-z0-9][a-z0-9-]*$ ]] || { echo "Invalid reader name" >&2; exit 1; }
[[ -f "$ROOT_DIR/apps/$APP/main.go" ]] || { echo "Unknown reader: $APP" >&2; exit 1; }
cd "$ROOT_DIR"
TARGET="$(go env GOOS)-$(go env GOARCH)"
go run ./tools/releasectl build --app "$APP" --target "$TARGET"
RUNTIME="$ROOT_DIR/dist/$TARGET/$APP/runtime"
DEST="$ROOT_DIR/output/$APP"
mkdir -p "$DEST/deployments"
# Copy the built executable using its generated name.
for binary in "$RUNTIME"/*; do
  [[ -f "$binary" && "$(basename "$binary")" != "manifest.json" ]] || continue
  cp "$binary" "$DEST/"
done
# Source deployments contain static assets only; preserve all existing local files.
cp -Rn "$RUNTIME/deployments/." "$DEST/deployments/"
if [[ ! -f "$DEST/deployments/config.yaml" ]]; then
  cp "$DEST/deployments/config.install.yaml" "$DEST/deployments/config.yaml"
fi
printf 'Reader output ready: %s\n' "$DEST"
