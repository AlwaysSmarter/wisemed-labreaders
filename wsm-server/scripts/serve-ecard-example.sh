#!/usr/bin/env bash
set -euo pipefail
app_root="$(cd "$(dirname "$0")/.." && pwd)"
example_dir="$(mktemp -d "${TMPDIR:-/tmp}/wisemed-ecard-wss.XXXXXX")"
trap 'rm -rf "$example_dir"' EXIT
cp "$app_root/internal/server/sdk/wsm.js" "$example_dir/wsm.js"
cp "$app_root/examples/browser/ecard-workflows.js" "$app_root/examples/browser/ecard-wss-test.html" "$example_dir/"
printf 'Test eCard WSS: http://127.0.0.1:%s/ecard-wss-test.html\n' "${ECARD_EXAMPLE_PORT:-8088}"
python3 -m http.server "${ECARD_EXAMPLE_PORT:-8088}" --bind 127.0.0.1 --directory "$example_dir"
