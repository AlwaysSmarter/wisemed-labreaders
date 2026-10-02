#!/usr/bin/env bash
# Build isolated WSS previews. Never refresh live output/ workspaces or configs.
# Usage: scripts/build-wss-preview.sh [new-destination-directory]
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"
PREVIEW_DIR="${1:-$ROOT_DIR/dist/wss-preview/$(date -u +%Y%m%dT%H%M%SZ)}"
if [[ -e "$PREVIEW_DIR" ]]; then
  echo "Preview destination already exists; choose a new directory: $PREVIEW_DIR" >&2
  exit 1
fi
mkdir -p "$PREVIEW_DIR"
PREVIEW_DIR="$(cd "$PREVIEW_DIR" && pwd)"
NATIVE_OS="$(go env GOHOSTOS)"
NATIVE_ARCH="$(go env GOHOSTARCH)"
NATIVE_TARGET="$NATIVE_OS-$NATIVE_ARCH"
PREVIEW_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
PREVIEW_LDFLAGS="-s -w -buildid= -X wisemed-labreaders/readersv3/shared/appmeta.Version=0.0.0-wss-preview -X wisemed-labreaders/readersv3/shared/appmeta.Commit=$PREVIEW_COMMIT -X wisemed-labreaders/readersv3/shared/appmeta.VersionMarker=WISEMED_APP_VERSION=0.0.0-wss-preview"
printf 'app\ttarget\tstatus\n' > "$PREVIEW_DIR/builds.tsv"

build_app() {
  local app="$1" target_os="$2" target_arch="$3"
  local target="$target_os-$target_arch" suffix=""
  [[ "$target_os" != windows ]] || suffix=".exe"
  local runtime_dir="$PREVIEW_DIR/$target/$app/runtime"
  mkdir -p "$runtime_dir/deployments"
  printf 'Building %s (%s)\n' "$app" "$target"
  GOWORK=off GOOS="$target_os" GOARCH="$target_arch" CGO_ENABLED=0 \
    go build -trimpath -buildvcs=false -mod=readonly -ldflags "$PREVIEW_LDFLAGS" \
    -o "$runtime_dir/$app$suffix" "./apps/$app"
  cp "apps/$app/deployments/config.install.yaml" "$runtime_dir/deployments/"
  if [[ -d "apps/$app/deployments/help" ]]; then
    cp -R "apps/$app/deployments/help" "$runtime_dir/deployments/help"
  fi
  if [[ "$app" == esignature-server && "$target" == windows-386 ]]; then
    mkdir -p "$runtime_dir/deployments/signotec"
    cp "$ROOT_DIR/../docs/signotec/WebSocketPadServer/STPadLib.dll" "$runtime_dir/deployments/signotec/"
  fi
  printf '%s\t%s\tbuilt\n' "$app" "$target" >> "$PREVIEW_DIR/builds.tsv"
}

for main_file in apps/*/main.go; do
  app="${main_file#apps/}"
  app="${app%/main.go}"
  # The release distribution service is infrastructure, not a tenant equipment.
  [[ "$app" != update-server ]] || continue
  build_app "$app" "$NATIVE_OS" "$NATIVE_ARCH"
done
# Signotec's bundled SDK is x86 Windows. Native previews provide UI/WSS only.
if [[ "$NATIVE_TARGET" != windows-386 ]]; then
  build_app esignature-server windows 386
fi
cat > "$PREVIEW_DIR/README.md" <<'DOC'
# Isolated WiseMED WSS preview

Each `<target>/<app>/runtime/` contains its executable, source install template
and available static help. No existing runtime config, DB, logs or output/
workspace was copied or modified. `builds.tsv` lists successful builds.

Before launch, copy `deployments/config.install.yaml` to
`deployments/config.yaml` INSIDE THIS PREVIEW and configure the real WiseMED API,
equipment identity and WSS URL/tenant/device authentication. Keep the original
install template intact. Launch from the runtime directory with
`./<app> -config deployments/config.yaml` (Windows: `<app>.exe`). The application
can create the runtime config from its template on first start. Use a distinct
local HTTP port and device/network settings if another installation is running.

These are development previews, not signed installers or production releases.
All 26 equipment apps are compiled for the native host. `esignature-server`
also includes a Windows/386 build and the existing x86 Signotec STPadLib.dll.
Its native non-Windows build tests UI/WSS but cannot operate that Windows DLL.
Signing-pad utility Java helper/SDK, printer drivers, analyzers, WiseMED backend,
WSS server/key provisioning and physical hardware remain external dependencies.
No executable was started against live equipment or a live WiseMED API.

To reproduce, run `scripts/build-wss-preview.sh <new-directory>` from readersv3.
The script uses cached Go compilation (no forced -a rebuild), disables CGO,
uses trimpath and does not overwrite an existing destination. WSS documentation
is in the repository's readersv3/docs and wsm-server/docs directories.
DOC
printf 'Preview ready: %s\n' "$PREVIEW_DIR"
