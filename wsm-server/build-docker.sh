#!/usr/bin/env bash
# Builds and exports a timestamped image for the target Linux host.
set -euo pipefail
cd "$(dirname "$0")/.."
version=$(date -u +%Y%m%d-%H%M%S)
platform=${WSM_DOCKER_PLATFORM:-linux/amd64}
repository=${WSM_DOCKER_IMAGE:-wisemed-wsm}
out=${WSM_DOCKER_OUTPUT_DIR:-wsm-server/dist/docker}
image="$repository:$version"
mkdir -p "$out"
archive="$out/wisemed-wsm-$version.tar.gz"
if [[ -e "$archive" ]]; then
  echo "Archive already exists: $archive" >&2
  exit 1
fi
docker buildx build --platform "$platform" \
  -f wsm-server/Dockerfile \
  --build-arg "WSM_VERSION=$version" \
  --label "org.opencontainers.image.version=$version" \
  -t "$image" --load .
temporary=$(mktemp "$out/.wsm-export.XXXXXX")
trap 'rm -f "$temporary"' EXIT
docker save "$image" | gzip > "$temporary"
mv "$temporary" "$archive"
printf 'Version (UTC): %s\nImage: %s\nArchive: %s\n' "$version" "$image" "$archive"
