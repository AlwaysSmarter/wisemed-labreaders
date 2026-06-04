param(
  [string]$InstallRoot = $PSScriptRoot
)

$ErrorActionPreference = 'Stop'
$serviceExe = Join-Path $InstallRoot 'wisemed-shimatzu-tocl-reader-winsw.exe'
$serviceXml = Join-Path $InstallRoot 'wisemed-shimatzu-tocl-reader.xml'

if (-not (Test-Path $serviceExe)) {
  throw "WinSW executable not found: $serviceExe"
}

& $serviceExe stop | Out-Null
& $serviceExe uninstall | Out-Null
& $serviceExe install
sc.exe failure "wisemed-shimatzu-tocl-reader" reset= 86400 actions= restart/60000/restart/120000/restart/300000 | Out-Null
Set-Service -Name "wisemed-shimatzu-tocl-reader" -StartupType Automatic
New-ItemProperty -Path "HKLM:\SYSTEM\CurrentControlSet\Services\wisemed-shimatzu-tocl-reader" -Name DelayedAutostart -PropertyType DWord -Value 1 -Force | Out-Null
& $serviceExe start
