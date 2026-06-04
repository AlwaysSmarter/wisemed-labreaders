param(
  [string]$InstallRoot = $PSScriptRoot
)

$ErrorActionPreference = 'Continue'
$serviceExe = Join-Path $InstallRoot 'wisemed-gammavision-reader-winsw.exe'
if (Test-Path $serviceExe) {
  & $serviceExe stop | Out-Null
  & $serviceExe uninstall | Out-Null
}
