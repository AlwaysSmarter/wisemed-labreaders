#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
# Preserve the existing export destination; the shared builder adds UTC versioning.
export WSM_DOCKER_OUTPUT_DIR="${WSM_DOCKER_OUTPUT_DIR:-$HOME/dockers}"
export WSM_DOCKER_PLATFORM="${WSM_DOCKER_PLATFORM:-linux/amd64}"
exec ./wsm-server/build-docker.sh
