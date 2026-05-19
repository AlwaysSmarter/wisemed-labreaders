$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
go run ./tools/releasectl package-all @args
