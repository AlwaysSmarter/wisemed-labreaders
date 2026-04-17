#!/usr/bin/env zsh
set -euo pipefail
cd "$(dirname "$0")"
GO111MODULE=on go build -o ../../output/maglumi-800/reader-maglumi-800 .
