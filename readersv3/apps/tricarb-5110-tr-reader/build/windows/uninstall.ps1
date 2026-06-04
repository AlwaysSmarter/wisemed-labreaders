param(
  [string]$InstallRoot = $PSScriptRoot
)

$ErrorActionPreference = 'Continue'
$serviceExe = Join-Path $InstallRoot 'wisemed-tricarb-5110-tr-reader-winsw.exe'
if (Test-Path $serviceExe) {
  & $serviceExe stop | Out-Null
  & $serviceExe uninstall | Out-Null
}
