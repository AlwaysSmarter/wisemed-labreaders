param(
  [string]$InstallRoot = $PSScriptRoot
)

$ErrorActionPreference = 'Stop'
$serviceExe = Join-Path $InstallRoot 'wisemed-signing-pad-utility-winsw.exe'
$serviceXml = Join-Path $InstallRoot 'wisemed-signing-pad-utility.xml'

if (-not (Test-Path $serviceExe)) {
  throw "WinSW executable not found: $serviceExe"
}

& $serviceExe stop | Out-Null
& $serviceExe uninstall | Out-Null
& $serviceExe install
sc.exe failure "wisemed-signing-pad-utility" reset= 86400 actions= restart/60000/restart/120000/restart/300000 | Out-Null
Set-Service -Name "wisemed-signing-pad-utility" -StartupType Automatic
New-ItemProperty -Path "HKLM:\SYSTEM\CurrentControlSet\Services\wisemed-signing-pad-utility" -Name DelayedAutostart -PropertyType DWord -Value 1 -Force | Out-Null
& $serviceExe start
