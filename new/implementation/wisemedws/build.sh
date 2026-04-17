#!/usr/bin/env zsh
set -euo pipefail
cd "$(dirname "$0")"
GO111MODULE=on go build -o ../../output/wisemedws/wisemedws ./cmd/server
