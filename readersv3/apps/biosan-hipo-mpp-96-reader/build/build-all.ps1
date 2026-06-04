$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..\..\..')
go run ./tools/releasectl build-all --app biosan-hipo-mpp-96-reader @args
