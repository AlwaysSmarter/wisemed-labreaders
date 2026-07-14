$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..\..\..')
go run ./tools/releasectl build --app signing-pad-utility --target windows-amd64 @args
