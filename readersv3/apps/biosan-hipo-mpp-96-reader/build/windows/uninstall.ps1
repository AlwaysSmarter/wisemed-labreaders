param(
  [string]$InstallRoot = $PSScriptRoot
)

$ErrorActionPreference = 'Continue'
$serviceExe = Join-Path $InstallRoot 'wisemed-biosan-hipo-mpp-96-reader-winsw.exe'
if (Test-Path $serviceExe) {
  & $serviceExe stop | Out-Null
  & $serviceExe uninstall | Out-Null
}
