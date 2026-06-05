$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..\..\..\..')
go run ./tools/releasectl package --app analytikjena-plasmaquantms-elite-reader --target windows-amd64 @args
