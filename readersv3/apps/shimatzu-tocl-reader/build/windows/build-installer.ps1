$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..\..\..\..')
go run ./tools/releasectl package --app shimatzu-tocl-reader --target windows-amd64 @args
