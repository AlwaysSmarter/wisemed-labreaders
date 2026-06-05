$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..\..\..')
go run ./tools/releasectl build --app analytikjena-plasmaquantms-elite-reader --target windows-amd64 @args
