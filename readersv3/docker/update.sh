#!/usr/bin/env bash
# Standalone EC2 updater. Generated per app; no checkout or companion file required.
set -Eeuo pipefail
umask 077
app=@APP@
setup=false
if [[ ${1:-} == --setup ]]; then setup=true; shift; fi
version=${1:-latest}
root=${SIUI_DATA_DIR:-/opt/$app}
name=${SIUI_CONTAINER_NAME:-$app}
cache=${SIUI_UPDATE_CACHE:-$HOME/.cache/$app-update}
health_timeout=${SIUI_HEALTH_TIMEOUT:-120}
sshkey=${SIUI_SSH_KEY:-$HOME/.ssh/github_dockers}
repo=git@github.com:AlwaysSmarter/dockers.git
[[ $# -le 1 && ( $version == latest || $version =~ ^[0-9]{8}-[0-9]{6}$ ) ]] || { echo "Usage: $0 [--setup] [latest|YYYYMMDD-HHMMSS]" >&2; exit 2; }
[[ $health_timeout =~ ^[1-9][0-9]*$ && $root == /* && $root != / && $root != /srv && $root != /opt ]] || { echo 'Invalid timeout or absolute application data directory' >&2; exit 2; }
for tool in git ssh ssh-agent ssh-add ssh-keygen docker flock python3 gzip; do
 command -v "$tool" >/dev/null || { echo "Install missing dependency: $tool" >&2; exit 1; }
done
[[ -r $sshkey ]] || { echo "Missing or unreadable SSH key: $sshkey" >&2; exit 1; }
mkdir -p "$cache"
exec 9>"$cache/update.lock"
flock -n 9 || { echo "Another $app update is running" >&2; exit 1; }
privilege=(env)
if [[ $(id -u) != 0 ]]; then privilege=(sudo); fi
docker_cmd=(docker)
if ! docker info >/dev/null 2>&1; then docker_cmd=(sudo docker); "${docker_cmd[@]}" info >/dev/null; fi
work=$(mktemp -d "$cache/run.XXXXXX")
backup="${name}-backup-$(date -u +%Y%m%d-%H%M%S)-$$"
owned_agent=false
old=false
old_running=false
renamed=false
replacement=false
committed=false
healthy() {
 local deadline=$((SECONDS + health_timeout))
 while (( SECONDS < deadline )); do
  if [[ $("${docker_cmd[@]}" inspect -f '{{.State.Running}}' "$name" 2>/dev/null) == true &&
        $("${docker_cmd[@]}" inspect -f '{{.State.Health.Status}}' "$name" 2>/dev/null) == healthy ]]; then return 0; fi
  sleep 2
 done
 return 1
}
cleanup() {
 local status=$?
 trap - EXIT INT TERM
 if [[ $committed != true && $renamed == true ]]; then
  echo 'Update failed; restoring previous container.' >&2
  if [[ $replacement == true ]]; then "${docker_cmd[@]}" rm -f "$name" >/dev/null 2>&1 || true; fi
  if "${docker_cmd[@]}" rename "$backup" "$name"; then
   if [[ $old_running == true ]]; then
    if "${docker_cmd[@]}" start "$name" >/dev/null && healthy; then echo 'Rollback healthy.' >&2
    else echo 'ROLLBACK NEEDS ATTENTION: check docker logs.' >&2; fi
   fi
  else echo "ROLLBACK FAILED: previous container is $backup" >&2; fi
 elif [[ $committed != true && $replacement == true ]]; then
  "${docker_cmd[@]}" rm -f "$name" >/dev/null 2>&1 || true
 fi
 if [[ $owned_agent == true ]]; then ssh-agent -k >/dev/null 2>&1 || true; fi
 rm -rf "$work"
 exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
# Reuse the current agent; only start a temporary one if none is reachable.
agent_status=0
ssh-add -l >/dev/null 2>&1 || agent_status=$?
if [[ $agent_status == 2 ]]; then eval "$(ssh-agent -s)" >/dev/null; owned_agent=true; fi
fingerprint=$(ssh-keygen -lf "$sshkey")
fingerprint=${fingerprint#* }; fingerprint=${fingerprint%% *}
identities=$(ssh-add -l 2>/dev/null || true)
if [[ $identities != *"$fingerprint"* ]]; then
 echo "Unlocking SSH key: $sshkey"
 ssh-add "$sshkey"
fi
echo "Fetching private Docker archive repository using $sshkey…"
export SIUI_DEPLOY_SSH_KEY="$sshkey"
GIT_SSH_COMMAND='ssh -F /dev/null -o IdentitiesOnly=yes -o BatchMode=yes -i "$SIUI_DEPLOY_SSH_KEY"' \
 GIT_TERMINAL_PROMPT=0 git clone --depth 1 "$repo" "$work/repo"
archive=$(python3 - "$work/repo" "$app" "$version" <<'PY'
import pathlib,re,subprocess,sys
root=pathlib.Path(sys.argv[1]); app=sys.argv[2]; wanted=sys.argv[3]
paths=subprocess.check_output(['git','-C',str(root),'ls-files','-z']).decode().split('\0')
found=[]
for path in paths:
 m=re.fullmatch(re.escape(app)+r'-(\d{8}-\d{6})\.tar\.gz',pathlib.PurePosixPath(path).name)
 if m and (wanted=='latest' or m[1]==wanted): found.append((m[1],path))
if not found: sys.exit('No matching timestamped archive for '+app)
latest=max(v for v,p in found); selected=[p for v,p in found if v==latest]
if len(selected)!=1: sys.exit('Duplicate archives for version '+latest)
p=root/selected[0]
if p.is_symlink() or not p.is_file() or root.resolve() not in p.resolve().parents: sys.exit('Archive must be a regular file within the clone')
print(p)
PY
)
file=${archive##*/}; version=${file#"$app"-}; version=${version%.tar.gz}
image="$app:$version"
gzip -t "$archive"
"${docker_cmd[@]}" load -i "$archive"
image_id=$("${docker_cmd[@]}" image inspect -f '{{.Id}}' "$image")
architecture=$("${docker_cmd[@]}" image inspect -f '{{.Architecture}}' "$image_id")
case $(uname -m) in x86_64) expected=amd64;; aarch64|arm64) expected=arm64;; *) echo 'Unsupported host architecture' >&2; exit 1;; esac
[[ $architecture == "$expected" ]] || { echo "Wrong image architecture: $architecture (host needs $expected)" >&2; exit 1; }
[[ $("${docker_cmd[@]}" image inspect -f '{{index .Config.Labels "eu.wisemed.app"}}' "$image_id") == "$app" ]] || { echo 'Archive application mismatch' >&2; exit 1; }
[[ $("${docker_cmd[@]}" run --rm --entrypoint /opt/wisemed/bin/app "$image_id" -version) == "$version" ]] || { echo 'Archive tag and binary version mismatch' >&2; exit 1; }
if "${docker_cmd[@]}" inspect "$name" >/dev/null 2>&1; then
 old=true
 old_running=$("${docker_cmd[@]}" inspect -f '{{.State.Running}}' "$name")
fi
if [[ $setup == true && $old_running == true ]]; then
 echo "Stop $name before interactive setup; configuration is shared with the running container." >&2; exit 1
fi
# The utilities write settings, TLS material, logs, SQLite and import/archive data.
# Prepare only this application's persistent directories; never modify DB servers.
"${privilege[@]}" install -d -o "$(id -u)" -g "$(id -g)" -m 750 "$root"
"${privilege[@]}" install -d -o 65532 -g 65532 -m 750 "$root/deployments" "$root/data"
"${privilege[@]}" chown -R 65532:65532 "$root/deployments" "$root/data"
"${privilege[@]}" find "$root/deployments" "$root/data" -type d -exec chmod 750 {} +
"${privilege[@]}" find "$root/deployments" "$root/data" -type f -exec chmod 640 {} +
run_args=(--network host --read-only --cap-drop ALL --security-opt no-new-privileges:true
 --tmpfs /tmp:rw,nosuid,nodev,mode=1777
 --mount "type=bind,src=$root/deployments,dst=/data/deployments"
 --mount "type=bind,src=$root/data,dst=/data/data")
# Optional persistent environment and driver/token mounts; one complete option per line.
if [[ -f $root/docker.env ]] || "${privilege[@]}" test -f "$root/docker.env"; then
 "${privilege[@]}" cat "$root/docker.env" > "$work/docker.env"
 run_args+=(--env-file "$work/docker.env")
fi
if [[ -f $root/docker.args ]] || "${privilege[@]}" test -f "$root/docker.args"; then
 "${privilege[@]}" cat "$root/docker.args" > "$work/docker.args"
 while IFS= read -r option || [[ -n $option ]]; do
  case $option in ''|'#'*) continue;; --mount=*|--device=*|--group-add=*) run_args+=("$option");;
   *) echo "Unsupported docker.args option: $option" >&2; exit 2;; esac
 done < "$work/docker.args"
fi
if [[ $setup == true ]]; then
 "${docker_cmd[@]}" run --rm -it "${run_args[@]}" "$image_id" -config /data/deployments/config.yaml -reconfigure -showlog
 committed=true
 echo "Setup finished. Run $0 $version to start the persistent container."
 exit 0
fi
if ! "${privilege[@]}" test -f "$root/deployments/config.yaml"; then
 echo "Configure the app first: $0 --setup $version" >&2; exit 1
fi
# Parse and verify identity/listener as the image's UID before stopping the old app.
# This does not contact CNAS or register equipment; module/DB checks happen at startup.
"${docker_cmd[@]}" run --rm "${run_args[@]}" --entrypoint /opt/wisemed/bin/docker-runtime \
 "$image_id" check-config /data/deployments/config.yaml
if [[ $old == true ]]; then
 if [[ $("${docker_cmd[@]}" inspect -f '{{.Image}}' "$name") == "$image_id" && $old_running == true ]] && healthy; then
  committed=true; echo "Already running healthy version $version"; exit 0
 fi
 "${docker_cmd[@]}" rename "$name" "$backup"
 renamed=true
 "${docker_cmd[@]}" stop -t 30 "$backup" >/dev/null
fi
replacement=true
"${docker_cmd[@]}" run -d --name "$name" --restart unless-stopped \
 --log-opt max-size=10m --log-opt max-file=3 "${run_args[@]}" "$image_id" >/dev/null
if ! healthy; then
 echo "New $app failed its local UI health check." >&2
 "${docker_cmd[@]}" logs --tail 40 "$name" >&2 || true
 exit 1
fi
committed=true
echo "$app updated and healthy: $image"
if [[ $old == true ]]; then echo "Previous container retained (stopped): $backup"; fi
echo "Persistent config/data preserved in $root; database migrations are not rolled back."
