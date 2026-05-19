$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
go run ./tools/releasectl build-all @args
