$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..\..\..\..')
go run ./tools/releasectl package --app biosan-hipo-mpp-96-reader --target windows-amd64 @args
