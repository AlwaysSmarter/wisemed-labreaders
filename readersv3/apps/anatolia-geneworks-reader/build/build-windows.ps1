$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..\..\..')
go run ./tools/releasectl build --app anatolia-geneworks-reader --target windows-amd64 @args
