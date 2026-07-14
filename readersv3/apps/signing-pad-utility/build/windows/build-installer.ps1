$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..\..\..\..')
go run ./tools/releasectl package --app signing-pad-utility --target windows-amd64 @args
