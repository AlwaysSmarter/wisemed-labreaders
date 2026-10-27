#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../../.." && pwd)
exec bash "$root/docker/build.sh" siui-bridge-history
