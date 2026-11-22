param([Parameter(Mandatory=$true)][string]$Sdk)
$ErrorActionPreference = 'Stop'
$dir = Split-Path -Parent $MyInvocation.MyCommand.Path
$source = Join-Path $dir 'ECardHost.cs'
$exe = Join-Path $dir 'ECardHost.exe'
# Build once using Windows' .NET Framework compiler, not a network download.
# Release packaging may precompile this same source with build-host.ps1.
if (!(Test-Path $exe) -or (Get-Item $source).LastWriteTimeUtc -gt (Get-Item $exe).LastWriteTimeUtc) {
  & (Join-Path $dir 'build-host.ps1')
}
& $exe $Sdk
exit $LASTEXITCODE
