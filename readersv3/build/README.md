# readersv3 release system

This directory is generated and maintained by `go run ./tools/releasectl sync`.

The release flow is:

1. `go run ./tools/releasectl sync` regenerates per-app build wrappers and native installer assets for every `apps/*/main.go` entrypoint.
2. `go run ./tools/releasectl build-all` cross-compiles every app into `dist/<target>/<app>/runtime` with only the binary and filtered `apps/<app>/deployments` tree.
3. `go run ./tools/releasectl release --app <app> --target <target>` generates the update payload and, for Windows, the NSIS installer.
4. Final artifacts are written into `dist/updates`, `dist/installers` and `dist/releases`.

Notes:

- Windows packaging uses NSIS through `makensis` and the shared script in `installer/windows/installer.nsi`.
- Linux packaging prefers `fpm` and falls back to `dpkg-deb` / `rpmbuild` when present.
- macOS packaging uses `pkgbuild`, `productbuild` and `hdiutil`.
- Future readers are picked up automatically after re-running `sync`.
