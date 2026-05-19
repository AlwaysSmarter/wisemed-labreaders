#!/usr/bin/env bash
set -euo pipefail

systemctl disable --now "wisemed-barcodeprinter.service" || true
rm -f "/usr/local/bin/barcodeprinter" || true
