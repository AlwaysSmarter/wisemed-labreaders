#!/usr/bin/env bash
set -euo pipefail

ln -sf "/opt/barcodeprinter/BarcodePrinter" "/usr/local/bin/barcodeprinter" || true
systemctl daemon-reload || true
systemctl enable "wisemed-barcodeprinter.service" || true
systemctl restart "wisemed-barcodeprinter.service" || systemctl start "wisemed-barcodeprinter.service" || true
