#!/usr/bin/env bash
# Compile the Windows .NET Framework host on Linux/macOS. No compiler on clients.
set -euo pipefail
app="$(cd "$(dirname "$0")/.." && pwd)"
output="${1:?Usage: build-host.sh OUTPUT_DIRECTORY}"
mkdir -p "$output"
output="$(cd "$output" && pwd)"

# Existing servers need no administrator install: bootstrap the official SDK in
# their writable cache. Containers can preinstall dotnet to avoid this download.
dotnet_cmd="${DOTNET:-}"
if [[ -z "$dotnet_cmd" ]]; then dotnet_cmd="$(command -v dotnet || true)"; fi
if [[ -n "$dotnet_cmd" ]]; then
  if [[ -z "$("$dotnet_cmd" --list-sdks)" ]]; then
    echo 'Configured dotnet has no SDK; install .NET SDK 8 or newer.' >&2
    exit 1
  fi
else
  cache="${WMR_ECARD_TOOLCHAIN_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/wisemed-ecard}"
  mkdir -p "$cache"
  dotnet_cmd="$cache/dotnet/dotnet"
  # Serialize bootstrap across independent build processes.
  if [[ ! -x "$dotnet_cmd" ]] || [[ -z "$("$dotnet_cmd" --list-sdks 2>/dev/null || true)" ]]; then
    lock="$cache/install.lock"
    attempts=0
    until mkdir "$lock" 2>/dev/null; do
      attempts=$((attempts+1))
      if [[ "$attempts" -gt 300 ]]; then echo "Timed out waiting for $lock; check other builds before removing a stale lock." >&2; exit 1; fi
      sleep 1
    done
    trap 'rmdir "$lock"' EXIT
    if [[ ! -x "$dotnet_cmd" ]] || [[ -z "$("$dotnet_cmd" --list-sdks 2>/dev/null || true)" ]]; then
      echo 'Preparing .NET SDK in the eCard build cache...' >&2
      curl --fail --silent --show-error --location --retry 3 --connect-timeout 30 --max-time 180 https://dot.net/v1/dotnet-install.sh -o "$cache/dotnet-install.sh"
      bash "$cache/dotnet-install.sh" --channel 8.0 --install-dir "$cache/dotnet" --no-path >&2
    fi
    rmdir "$lock"
    trap - EXIT
  fi
fi
export DOTNET_CLI_TELEMETRY_OPTOUT=1 DOTNET_NOLOGO=1
# The build does not require locale data; support minimal Linux build images.
export DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1
"$dotnet_cmd" build "$app/scripts/ECardHost.csproj" --configuration Release --output "$output" --nologo \
  "-p:BaseIntermediateOutputPath=$output/obj/" "-p:MSBuildProjectExtensionsPath=$output/obj/" >&2
