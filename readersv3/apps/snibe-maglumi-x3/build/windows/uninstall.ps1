param(
  [string]$InstallRoot = $PSScriptRoot
)

$ErrorActionPreference = 'Continue'
$serviceExe = Join-Path $InstallRoot 'wisemed-snibe-maglumi-x3-winsw.exe'
if (Test-Path $serviceExe) {
  & $serviceExe stop | Out-Null
  & $serviceExe uninstall | Out-Null
}
