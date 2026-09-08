$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '../../..')
go run ./tools/releasectl build --app esignature-server --target windows-386 @args
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
