# readersv3 release system

This directory is generated and maintained by `go run ./tools/releasectl sync`.

The release flow is:

1. `go run ./tools/releasectl sync` regenerates per-app build wrappers and native installer assets for every `apps/*/main.go` entrypoint.
2. `go run ./tools/releasectl build-all` cross-compiles every app into `dist/<target>/<app>/runtime` with only the binary and filtered `output/<app>/deployments` tree.
3. `go run ./tools/releasectl package-all` runs native packaging for the current host OS and writes final artifacts into `release/<app>/<target>`.

Notes:

- Windows packaging prefers WiX and falls back to NSIS when WiX is unavailable.
- Linux packaging prefers `fpm` and falls back to `dpkg-deb` / `rpmbuild` when present.
- macOS packaging uses `pkgbuild`, `productbuild` and `hdiutil`.
- Future readers are picked up automatically after re-running `sync`.
