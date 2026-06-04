$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..\..\..')
go run ./tools/releasectl build --app tricarb-5110-tr-reader --target windows-amd64 @args
