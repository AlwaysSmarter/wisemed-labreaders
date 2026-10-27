#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
app=${1:?Usage: prepare-docker-output.sh APP}
case $app in siui-bridge|siui-bridge-history|wisemed-siui-data);; *) echo 'Unsupported Docker app' >&2; exit 2;; esac
out="$root/output/$app"
mkdir -p "$out/docker"
sed "s/@APP@/$app/g" "$root/docker/update.sh" > "$out/update-$app.sh"
chmod +x "$out/update-$app.sh"
sed -e 's@database: ./siui-jobs.db@database: ../data/siui-jobs.db@' \
    -e 's@base_path: ./history@base_path: ../data/history@' \
    -e 's@staging_path: ./history-staging@staging_path: ../data/history-staging@' \
    -e 's@upload_dir: ./catalogue-uploads@upload_dir: ../data/catalogue-uploads@' \
    "$root/apps/$app/deployments/config.install.yaml" > "$out/docker/config.install.yaml"
cp "$root/docker/README.md" "$out/docker/README.md"
printf 'Docker updater ready: %s\n' "$out/update-$app.sh"
