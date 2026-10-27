#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
exec bash ./readersv3/docker/build.sh siui-bridge
