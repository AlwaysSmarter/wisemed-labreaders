param([string]$OutputDirectory = $PSScriptRoot)
$ErrorActionPreference = 'Stop'
$dir = Split-Path -Parent $MyInvocation.MyCommand.Path
$csc = Join-Path $env:WINDIR 'Microsoft.NET\Framework\v4.0.30319\csc.exe'
if (!(Test-Path $csc)) { throw '.NET Framework 4.8 is required' }
New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null
$out = Join-Path $OutputDirectory 'ECardHost.exe'
& $csc /nologo /target:exe /platform:x86 /optimize+ /r:System.Web.Extensions.dll /r:System.Core.dll /r:System.Security.dll /r:System.Xml.dll /out:$out (Join-Path $dir 'ECardHost.cs') | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'eCard host compilation failed' }

if ([IO.Path]::GetFullPath($OutputDirectory) -ne [IO.Path]::GetFullPath($dir)) { exit 0 }
$manifest = @{}
foreach ($name in @('ECardHost.cs','ECardHost.exe','ECardHost.exe.config')) {
  $manifest[$name] = (Get-FileHash (Join-Path $dir $name) -Algorithm SHA256).Hash.ToLowerInvariant()
}
$manifest | ConvertTo-Json | Set-Content (Join-Path $dir 'manifest.json') -Encoding Ascii
