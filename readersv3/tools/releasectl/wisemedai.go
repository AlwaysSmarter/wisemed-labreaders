package main

const aiLinuxTarInstall = `#!/usr/bin/env bash
set -euo pipefail
SOURCE_DIR="$(cd "$(dirname "$0")" && pwd)"
if [[ "$EUID" -eq 0 ]]; then SUDO=(); else SUDO=(sudo); fi
"${SUDO[@]}" mkdir -p /opt/wisemed-ai
# Payload contains config.install.yaml, never an active config or secret file.
"${SUDO[@]}" cp -R "$SOURCE_DIR/opt/wisemed-ai/." /opt/wisemed-ai/
id wisemed-ai >/dev/null 2>&1 || "${SUDO[@]}" useradd --system --user-group --home-dir /opt/wisemed-ai --shell /usr/sbin/nologin wisemed-ai
"${SUDO[@]}" chown -R wisemed-ai:wisemed-ai /opt/wisemed-ai
"${SUDO[@]}" install -m 0644 "$SOURCE_DIR/usr/lib/systemd/system/wisemed-ai.service" /etc/systemd/system/wisemed-ai.service
"${SUDO[@]}" ln -sf /opt/wisemed-ai/wisemed-ai /usr/local/bin/wisemed-ai
"${SUDO[@]}" systemctl daemon-reload
"${SUDO[@]}" systemctl enable --now wisemed-ai.service
printf 'Installed wisemed-ai. Configure PostgreSQL in /opt/wisemed-ai/deployments/config.yaml or the service environment, then restart.\n'
`
const aiLinuxTarUninstall = `#!/usr/bin/env bash
set -euo pipefail
if [[ "$EUID" -eq 0 ]]; then SUDO=(); else SUDO=(sudo); fi
"${SUDO[@]}" systemctl disable --now wisemed-ai.service || true
"${SUDO[@]}" rm -f /etc/systemd/system/wisemed-ai.service /usr/local/bin/wisemed-ai
"${SUDO[@]}" systemctl daemon-reload
printf 'Service removed. Application, configuration and secrets are preserved in /opt/wisemed-ai.\n'
`
