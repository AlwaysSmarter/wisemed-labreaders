$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..\..\..')
go run ./tools/releasectl build-all --app anatolia-geneworks-reader @args
