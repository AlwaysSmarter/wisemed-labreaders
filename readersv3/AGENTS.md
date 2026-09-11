# Reader creation and delivery

For every new reader, create the complete standard reader layout. Do not stop at
protocol code or a `dist/` package. Read the project generation rules in
`../aimemory/wisemed-labreaders.md` and the runtime layout in `README.md`.

Before reporting completion, verify:
- App entrypoint, protocol module, registrations and communication settings.
- Standard reader modules and analyzer-specific repeat behavior.
- Source deployment configuration, local help, and app-level protocol docs.
- Distinct WiseMED application icon in PNG and real multi-resolution ICO formats.
- Build, installer and service scripts for the supported platforms.
- Populated `output/<reader>/` with a runnable local binary,
  `deployments/config.yaml`, `deployments/config.install.yaml`, and help assets.
- Requested distribution artifacts and appropriate validation.

Use `scripts/prepare-reader-output.sh <reader>` to initialize or refresh the local
output workspace. Preserve existing runtime configuration, databases, logs and
other local state. Output is gitignored, so verify it on disk explicitly.
