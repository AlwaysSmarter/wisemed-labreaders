#!/bin/sh
set -eu
umask 077
# Config, certificates, databases and logs live on the persistent writable mount.
mkdir -p /data/deployments /data/data
# Only the install template changes; the operator's existing config is preserved.
sed -e 's@database: ./siui-jobs.db@database: ../data/siui-jobs.db@' \
    -e 's@base_path: ./history@base_path: ../data/history@' \
    -e 's@staging_path: ./history-staging@staging_path: ../data/history-staging@' \
    -e 's@upload_dir: ./catalogue-uploads@upload_dir: ../data/catalogue-uploads@' \
    /opt/wisemed/defaults/config.install.yaml > /data/deployments/config.install.yaml
cp -R /opt/wisemed/defaults/help /data/deployments/
exec /opt/wisemed/bin/app "$@"
