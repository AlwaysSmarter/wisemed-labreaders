#!/usr/bin/env bash
# Cross-compile using Roslyn and official .NET Framework 4.8 reference assemblies.
# These are build-only dependencies; no runtime NuGet/network restore on site.
set -euo pipefail
app="$(cd "$(dirname "$0")/.." && pwd)"
: "${DOTNET:?path to dotnet runtime}"
: "${CSC_DLL:?path to Roslyn csc.dll}"
: "${NET48_REFS:?directory containing .NET Framework 4.8 reference DLLs}"
args=()
for dll in mscorlib System System.Core System.Security System.Xml System.Web.Extensions; do args+=("-r:$NET48_REFS/$dll.dll"); done
DOTNET_ROLL_FORWARD=Major "$DOTNET" "$CSC_DLL" -nologo -deterministic -optimize+ -target:exe -platform:x86 -nostdlib "${args[@]}" "-out:$app/deployments/host/ECardHost.exe" "$app/deployments/host/ECardHost.cs"

python3 - "$app/deployments/host" <<'PYCODE'
import pathlib,sys,json,hashlib
p=pathlib.Path(sys.argv[1])
(p/'manifest.json').write_text(json.dumps({n:hashlib.sha256((p/n).read_bytes()).hexdigest() for n in ['ECardHost.cs','ECardHost.exe','ECardHost.exe.config']},indent=2)+'\n')
PYCODE
