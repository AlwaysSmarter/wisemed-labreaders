#!/usr/bin/env bash
set -euo pipefail

WMLR_REPO="${WMLR_REPO:-/opt/wmlr}"
APP_ROOT="${APP_ROOT:-}"
DEPLOYMENTS_DIR="${DEPLOYMENTS_DIR:-}"
BUILD_OUTPUT="${BUILD_OUTPUT:-}"
GO_BUILD_PKG="${GO_BUILD_PKG:-./apps/update-server}"
GO_BUILD_FLAGS="${GO_BUILD_FLAGS:-}"
UPDATE_SERVER_BIND="${UPDATE_SERVER_BIND:-0.0.0.0:19090}"
PUBLIC_BASE_URL="${PUBLIC_BASE_URL:-}"

log() {
  printf '[wmlr-update-server] %s\n' "$*"
}

escape_sed_replacement() {
  printf '%s' "$1" | sed -e 's/[\/&]/\\&/g'
}

resolve_repo_root() {
  if [[ -f "${WMLR_REPO}/go.mod" && -d "${WMLR_REPO}/apps/update-server" ]]; then
    printf '%s\n' "${WMLR_REPO}"
    return 0
  fi
  if [[ -f "${WMLR_REPO}/readersv3/go.mod" && -d "${WMLR_REPO}/readersv3/apps/update-server" ]]; then
    printf '%s\n' "${WMLR_REPO}/readersv3"
    return 0
  fi
  return 1
}

sync_deployments() {
  local src_dir="$1"

  mkdir -p "${DEPLOYMENTS_DIR}"

  rsync -a \
    --exclude 'config.yaml' \
    "${src_dir}/" "${DEPLOYMENTS_DIR}/"

  if [[ ! -f "${DEPLOYMENTS_DIR}/config.install.yaml" ]]; then
    log "Lipsește ${DEPLOYMENTS_DIR}/config.install.yaml după sincronizare."
    exit 1
  fi
}

apply_runtime_config() {
  local target_file="$1"

  [[ -f "${target_file}" ]] || return 0

  local escaped_bind
  escaped_bind="$(escape_sed_replacement "${UPDATE_SERVER_BIND}")"
  sed -i \
    -e "s|^  address: .*|  address: ${escaped_bind}|" \
    -e "s|^    address: .*|    address: ${escaped_bind}|" \
    "${target_file}"

  if [[ -n "${PUBLIC_BASE_URL}" ]]; then
    local escaped_base_url
    escaped_base_url="$(escape_sed_replacement "${PUBLIC_BASE_URL}")"
    sed -i "s|^    public_base_url: .*|    public_base_url: ${escaped_base_url}|" "${target_file}"
  fi
}

REPO_ROOT="$(resolve_repo_root || true)"
if [[ -z "${REPO_ROOT}" ]]; then
  log "Nu pot identifica repo-ul readersv3 în ${WMLR_REPO}."
  log "Montează clona wisemed-labreaders în /opt/wmlr sau setează WMLR_REPO corect."
  exit 1
fi

if [[ ! -f "${REPO_ROOT}/go.mod" ]]; then
  log "Nu există ${REPO_ROOT}/go.mod."
  exit 1
fi

APP_ROOT="${APP_ROOT:-${REPO_ROOT}/output/update-server}"
DEPLOYMENTS_DIR="${DEPLOYMENTS_DIR:-${APP_ROOT}/deployments}"
BUILD_OUTPUT="${BUILD_OUTPUT:-${APP_ROOT}/Update_Server}"

mkdir -p "${APP_ROOT}" "${DEPLOYMENTS_DIR}" /go/pkg/mod /root/.cache/go-build

sync_deployments "${REPO_ROOT}/apps/update-server/deployments"
apply_runtime_config "${DEPLOYMENTS_DIR}/config.install.yaml"
apply_runtime_config "${DEPLOYMENTS_DIR}/config.yaml"

log "Compilez ultima versiune din ${REPO_ROOT}."
cd "${REPO_ROOT}"
GOCACHE="${GOCACHE:-/root/.cache/go-build}" \
GOMODCACHE="${GOMODCACHE:-/go/pkg/mod}" \
go build ${GO_BUILD_FLAGS} -o "${BUILD_OUTPUT}" "${GO_BUILD_PKG}"

if [[ ! -x "${BUILD_OUTPUT}" ]]; then
  log "Build-ul nu a produs binarul ${BUILD_OUTPUT}."
  exit 1
fi

log "Pornesc update-server (1) pe ${UPDATE_SERVER_BIND}."
exec "${BUILD_OUTPUT}" -config "${DEPLOYMENTS_DIR}/config.yaml"
