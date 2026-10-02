#!/usr/bin/env bash
# Run as ec2-user (Git uses its SSH key); Docker uses sudo if needed.
set -Eeuo pipefail
umask 077
repo=git@github.com:AlwaysSmarter/dockers.git
version=${1:-latest}
root=${WSM_DATA_DIR:-/srv/wsm}
name=${WSM_CONTAINER_NAME:-wsm-server}
port=${WSM_HOST_PORT:-8090}
cache=${WSM_UPDATE_CACHE:-$HOME/.cache/wsm-update}
health_timeout=${WSM_HEALTH_TIMEOUT:-60}
[[ $# -le 1 && ( $version == latest || $version =~ ^[0-9]{8}-[0-9]{6}$ ) ]] || { echo "Usage: $0 [latest|YYYYMMDD-HHMMSS]" >&2; exit 2; }
[[ $port =~ ^[0-9]+$ && $health_timeout =~ ^[1-9][0-9]*$ && $root == /* ]] || { echo 'Invalid port, timeout or absolute data directory' >&2; exit 2; }
for tool in git docker curl flock python3 gzip; do command -v "$tool" >/dev/null || { echo "Install missing dependency: $tool" >&2; exit 1; }; done
mkdir -p "$cache"
exec 9>"$cache/update.lock"
flock -n 9 || { echo 'Another WSM update is running' >&2; exit 1; }
docker_cmd=(docker)
if ! docker info >/dev/null 2>&1; then docker_cmd=(sudo docker); "${docker_cmd[@]}" info >/dev/null; fi
for directory in "$root/deployments" "$root/state"; do
 if ! [[ -d $directory ]] && ! sudo test -d "$directory"; then
  echo "Prepare $directory first (config/keys and UID 65532 permissions)." >&2; exit 1
 fi
done
work=$(mktemp -d "$cache/run.XXXXXX")
backup="${name}-backup-$(date -u +%Y%m%d-%H%M%S)-$$"
old=false
old_running=false
renamed=false
replacement=false
committed=false
cleanup() {
 status=$?
 trap - EXIT INT TERM
 if [[ $committed != true && $renamed == true ]]; then
  echo 'Update failed; restoring previous container.' >&2
  if [[ $replacement == true ]]; then "${docker_cmd[@]}" rm -f "$name" >/dev/null 2>&1 || true; fi
  if "${docker_cmd[@]}" rename "$backup" "$name"; then
   if [[ $old_running == true ]]; then
    if "${docker_cmd[@]}" start "$name" >/dev/null && healthy; then echo 'Rollback healthy.' >&2; else echo 'ROLLBACK NEEDS ATTENTION: check docker logs.' >&2; fi
   fi
  else echo "ROLLBACK FAILED: previous container is $backup" >&2; fi
 elif [[ $committed != true && $replacement == true ]]; then
  "${docker_cmd[@]}" rm -f "$name" >/dev/null 2>&1 || true
 fi
 rm -rf "$work"
 exit "$status"
}
healthy() {
 local deadline=$((SECONDS + health_timeout))
 while (( SECONDS < deadline )); do
  if [[ $("${docker_cmd[@]}" inspect -f '{{.State.Running}}' "$name" 2>/dev/null) == true ]] &&
    curl --noproxy '*' --fail --silent --max-time 3 "http://127.0.0.1:$port/healthz" |
      python3 -c 'import json,sys; d=json.load(sys.stdin); sys.exit(0 if d.get("status")=="ok" and d.get("service")=="wsm-server" else 1)' 2>/dev/null; then return 0; fi
  sleep 2
 done
 return 1
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
echo 'Fetching private Docker archive repository using current user SSH credentials…'
GIT_TERMINAL_PROMPT=0 git clone --depth 1 "$repo" "$work/repo"
# Select only tracked archives; fail rather than choose ambiguously duplicated versions.
archive=$(python3 - "$work/repo" "$version" <<'PY'
import pathlib,re,subprocess,sys
root=pathlib.Path(sys.argv[1]); wanted=sys.argv[2]
paths=subprocess.check_output(['git','-C',str(root),'ls-files','-z']).decode().split('\0')
found=[]
for path in paths:
 m=re.fullmatch(r'wisemed-wsm-(\d{8}-\d{6})\.tar\.gz',pathlib.PurePosixPath(path).name)
 if m and (wanted=='latest' or m[1]==wanted): found.append((m[1],path))
if not found: sys.exit('No matching timestamped WSM archive in repository')
latest=max(v for v,p in found); selected=[p for v,p in found if v==latest]
if len(selected)!=1: sys.exit('Duplicate archives for version '+latest)
p=root/selected[0]
if p.is_symlink() or not p.is_file(): sys.exit('Archive must be a regular file')
print(p)
PY
)
file=${archive##*/}
version=${file#wisemed-wsm-}; version=${version%.tar.gz}
image="wisemed-wsm:$version"
gzip -t "$archive" # Also rejects unresolved Git LFS pointer files.
"${docker_cmd[@]}" load -i "$archive"
image_id=$("${docker_cmd[@]}" image inspect -f '{{.Id}}' "$image")
architecture=$("${docker_cmd[@]}" image inspect -f '{{.Architecture}}' "$image_id")
case $(uname -m) in x86_64) expected=amd64;; aarch64|arm64) expected=arm64;; *) echo 'Unsupported host architecture' >&2; exit 1;; esac
[[ $architecture == "$expected" ]] || { echo "Wrong image architecture: $architecture (host needs $expected)" >&2; exit 1; }
[[ $("${docker_cmd[@]}" run --rm "$image_id" --version) == "$version" ]] || { echo 'Archive tag and binary version mismatch' >&2; exit 1; }
"${docker_cmd[@]}" run --rm --read-only --cap-drop ALL --security-opt no-new-privileges:true \
 --mount "type=bind,src=$root/deployments,dst=/etc/wsm-server,readonly" \
 --mount "type=bind,src=$root/state,dst=/var/lib/wsm-server,readonly" \
 "$image_id" -config /etc/wsm-server/config.yaml -check-config
if "${docker_cmd[@]}" inspect "$name" >/dev/null 2>&1; then
 old=true
 old_running=$("${docker_cmd[@]}" inspect -f '{{.State.Running}}' "$name")
 if [[ $("${docker_cmd[@]}" inspect -f '{{.Image}}' "$name") == "$image_id" && $old_running == true ]] && healthy; then
  committed=true; echo "Already running healthy version $version"; exit 0
 fi
 # Keep old container and all its settings available for rollback.
 "${docker_cmd[@]}" rename "$name" "$backup"
 renamed=true
 "${docker_cmd[@]}" stop -t 30 "$backup" >/dev/null
fi
replacement=true
"${docker_cmd[@]}" run -d --name "$name" --restart unless-stopped \
 --read-only --cap-drop ALL --security-opt no-new-privileges:true \
 --log-opt max-size=10m --log-opt max-file=3 \
 -p "127.0.0.1:$port:8090" \
 --mount "type=bind,src=$root/deployments,dst=/etc/wsm-server,readonly" \
 --mount "type=bind,src=$root/state,dst=/var/lib/wsm-server" \
 "$image_id" -config /etc/wsm-server/config.yaml >/dev/null
if ! healthy; then
 echo "New WSM failed health check. Review: sudo docker logs $name" >&2
 "${docker_cmd[@]}" logs --tail 30 "$name" >&2 || true
 exit 1
fi
committed=true
echo "WSM updated and healthy: $image"
if [[ $old == true ]]; then echo "Previous container retained (stopped): $backup"; fi
echo 'Configuration, device registry and Nginx were preserved.'
