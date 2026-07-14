$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..\..\..\..')
go run ./tools/releasectl package --app snibe-maglumi-x3 --target windows-amd64 @args
