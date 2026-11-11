#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/readersv3"
if [[ $# -eq 0 ]]; then set -- up -d --build; fi
exec docker compose -f apps/wisemed-ai/docker/compose.yaml "$@"
