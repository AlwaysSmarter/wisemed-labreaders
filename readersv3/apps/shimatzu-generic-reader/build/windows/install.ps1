param(
  [string]$InstallRoot = $PSScriptRoot
)

$ErrorActionPreference = 'Stop'
$serviceExe = Join-Path $InstallRoot 'wisemed-shimatzu-generic-reader-winsw.exe'
$serviceXml = Join-Path $InstallRoot 'wisemed-shimatzu-generic-reader.xml'

if (-not (Test-Path $serviceExe)) {
  throw "WinSW executable not found: $serviceExe"
}

& $serviceExe stop | Out-Null
& $serviceExe uninstall | Out-Null
& $serviceExe install
sc.exe failure "wisemed-shimatzu-generic-reader" reset= 86400 actions= restart/60000/restart/120000/restart/300000 | Out-Null
Set-Service -Name "wisemed-shimatzu-generic-reader" -StartupType Automatic
New-ItemProperty -Path "HKLM:\SYSTEM\CurrentControlSet\Services\wisemed-shimatzu-generic-reader" -Name DelayedAutostart -PropertyType DWord -Value 1 -Force | Out-Null
& $serviceExe start
