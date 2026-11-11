#!/usr/bin/env bash
# Uses Linux NSIS when the host installation cannot compile even a minimal script.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
VERSION="${1:-1.3.0}"
docker build -t wisemed-ai-nsis-builder - < "$ROOT/apps/wisemed-ai/docker/Dockerfile.nsis"
WRAPPER_DIR="$(mktemp -d)"
trap 'rm -rf "$WRAPPER_DIR"' EXIT
cat > "$WRAPPER_DIR/makensis" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
exec docker run --rm -v "$WISEMED_AI_BUILD_ROOT/dist:$WISEMED_AI_BUILD_ROOT/dist" -v "$WISEMED_AI_BUILD_ROOT/installer:$WISEMED_AI_BUILD_ROOT/installer:ro" -v "$WISEMED_AI_BUILD_ROOT/resources/app-icons:$WISEMED_AI_BUILD_ROOT/resources/app-icons:ro" -w "$PWD" wisemed-ai-nsis-builder "$@"
SH
chmod +x "$WRAPPER_DIR/makensis"
export WISEMED_AI_BUILD_ROOT="$ROOT"
export PATH="$WRAPPER_DIR:$PATH"
cd "$ROOT"
go run ./tools/releasectl release --app wisemed-ai --target windows-amd64 --version "$VERSION"
