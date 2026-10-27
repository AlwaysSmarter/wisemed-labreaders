#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
app=${1:?Usage: build.sh APP}
case $app in siui-bridge|siui-bridge-history|wisemed-siui-data);; *) echo 'Unsupported Docker app' >&2; exit 2;; esac
version=$(date -u +%Y%m%d-%H%M%S)
platform=${SIUI_DOCKER_PLATFORM:-linux/amd64}
out=${SIUI_DOCKER_OUTPUT_DIR:-$HOME/dockers}
image="$app:$version"
mkdir -p "$out"
out=$(cd "$out" && pwd)
archive="$out/$app-$version.tar.gz"
[[ ! -e $archive ]] || { echo "Archive already exists: $archive" >&2; exit 1; }
bash "$root/readersv3/scripts/prepare-docker-output.sh" "$app"
cp "$root/readersv3/output/$app/update-$app.sh" "$out/update-$app.sh"
chmod +x "$out/update-$app.sh"
docker buildx build --platform "$platform" -f "$root/readersv3/docker/Dockerfile" \
 --build-arg "APP=$app" --build-arg "APP_VERSION=$version" \
 --label "org.opencontainers.image.version=$version" -t "$image" --load "$root"
temporary=$(mktemp "$out/.$app-export.XXXXXX")
trap 'rm -f "$temporary"' EXIT
docker save "$image" | gzip > "$temporary"
mv "$temporary" "$archive"
printf 'Version (UTC): %s\nImage: %s\nArchive: %s\nUpdater: %s\n' "$version" "$image" "$archive" "$out/update-$app.sh"
