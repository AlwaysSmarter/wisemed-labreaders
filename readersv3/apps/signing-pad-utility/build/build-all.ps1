$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..\..\..')
go run ./tools/releasectl build-all --app signing-pad-utility @args
