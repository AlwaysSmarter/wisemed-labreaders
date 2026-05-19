#!/usr/bin/env bash
set -euo pipefail

log() {
  printf '[build-installer] %s\n' "$*"
}

require_arg() {
  local name="$1"
  local value="$2"
  if [[ -z "$value" ]]; then
    printf 'missing required argument: %s\n' "$name" >&2
    exit 1
  fi
}

RUNTIME_DIR=""
STAGE_DIR=""
OUTPUT_PATH=""
APP_NAME=""
INSTALL_DIR_NAME=""
BINARY_NAME=""
VERSION=""
ICON_PATH=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --runtime-dir) RUNTIME_DIR="$2"; shift 2 ;;
    --stage-dir) STAGE_DIR="$2"; shift 2 ;;
    --output) OUTPUT_PATH="$2"; shift 2 ;;
    --app-name) APP_NAME="$2"; shift 2 ;;
    --install-dir-name) INSTALL_DIR_NAME="$2"; shift 2 ;;
    --binary-name) BINARY_NAME="$2"; shift 2 ;;
    --version) VERSION="$2"; shift 2 ;;
    --icon) ICON_PATH="$2"; shift 2 ;;
    *) printf 'unknown argument: %s\n' "$1" >&2; exit 1 ;;
  esac
done

require_arg "--runtime-dir" "$RUNTIME_DIR"
require_arg "--stage-dir" "$STAGE_DIR"
require_arg "--output" "$OUTPUT_PATH"
require_arg "--app-name" "$APP_NAME"
require_arg "--install-dir-name" "$INSTALL_DIR_NAME"
require_arg "--binary-name" "$BINARY_NAME"
require_arg "--version" "$VERSION"

if ! command -v makensis >/dev/null 2>&1; then
  printf 'makensis is not available in PATH\n' >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
NSIS_SCRIPT="${ROOT_DIR}/installer/windows/installer.nsi"
PAYLOAD_DIR="${STAGE_DIR}/payload"
WRAPPER_SCRIPT="${STAGE_DIR}/build-installer.nsi"
SELFTEST_SCRIPT="${STAGE_DIR}/makensis-selftest.nsi"
SELFTEST_OUTPUT="${STAGE_DIR}/makensis-selftest.exe"

log "pregatesc staging dir: ${STAGE_DIR}"
rm -rf "${STAGE_DIR}"
mkdir -p "${PAYLOAD_DIR}"
mkdir -p "$(dirname "${OUTPUT_PATH}")"

cat > "${SELFTEST_SCRIPT}" <<EOF
Name "makensis-selftest"
OutFile "${SELFTEST_OUTPUT}"
Section
SectionEnd
EOF

log "verific makensis cu un self-test minimal"
if ! makensis "${SELFTEST_SCRIPT}" >/tmp/wisemed-makensis-selftest.log 2>&1; then
  cat /tmp/wisemed-makensis-selftest.log >&2 || true
  printf 'makensis local a esuat chiar si pentru un script minim. Problema este in tool-ul NSIS instalat pe acest macOS, nu in scriptul WiseMED.\n' >&2
  exit 1
fi
rm -f "${SELFTEST_SCRIPT}" "${SELFTEST_OUTPUT}"

log "copiez binarul: ${BINARY_NAME}"
cp "${RUNTIME_DIR}/${BINARY_NAME}" "${PAYLOAD_DIR}/${BINARY_NAME}"

if [[ -d "${RUNTIME_DIR}/deployments" ]]; then
  log "copiez deployments/"
  cp -R "${RUNTIME_DIR}/deployments" "${PAYLOAD_DIR}/deployments"
fi

rm -f "${PAYLOAD_DIR}/deployments/config.yaml"
find "${PAYLOAD_DIR}" -name '*.db' -delete
find "${PAYLOAD_DIR}" -name '*.db-shm' -delete
find "${PAYLOAD_DIR}" -name '*.db-wal' -delete
find "${PAYLOAD_DIR}" -name '*.log' -delete

log "payload inclus in installer:"
find "${PAYLOAD_DIR}" -type f | sort | sed 's#^#  - #'

cat > "${WRAPPER_SCRIPT}" <<EOF
!define APP_NAME "${APP_NAME}"
!define APP_VERSION "${VERSION}"
!define APP_BINARY_NAME "${BINARY_NAME}"
!define APP_INSTALL_DIR_NAME "${INSTALL_DIR_NAME}"
!define APP_PAYLOAD_DIR "${PAYLOAD_DIR}"
!define OUTPUT_EXE "${OUTPUT_PATH}"
!addincludedir "${ROOT_DIR}/installer/windows"
EOF

if [[ -n "${ICON_PATH}" && -f "${ICON_PATH}" ]]; then
  printf '!define APP_ICON "%s"\n' "${ICON_PATH}" >> "${WRAPPER_SCRIPT}"
fi

printf '!include "%s"\n' "${NSIS_SCRIPT}" >> "${WRAPPER_SCRIPT}"

log "rulez makensis pentru ${APP_NAME} ${VERSION}"
makensis "${WRAPPER_SCRIPT}"
log "installer generat: ${OUTPUT_PATH}"
