$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..\..\..')
go run ./tools/releasectl build-all --app snibe-maglumi-x3 @args
