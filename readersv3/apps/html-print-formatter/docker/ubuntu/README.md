# Ubuntu Linux container

Build from the repository root, using `readersv3/` as the Docker build context:

```sh
docker build --platform linux/amd64 \
  -f readersv3/apps/html-print-formatter/docker/ubuntu/Dockerfile \
  -t wisemed-html-print-formatter:local readersv3
```

Use `--platform linux/arm64` for an ARM64 Linux server. Omitting the platform
builds for Docker's native architecture. Both produce Linux executables, even
when building with Docker Desktop on macOS or Windows. BuildKit is required.

The first stage uses the Go 1.25 toolchain and compiles `./apps/html-print-formatter`
from source with `CGO_ENABLED=0`, `GOOS=linux`, and the selected `GOARCH`.
`COPY --from=build /out/html-print-formatter ...` copies only that compiled binary
into the Ubuntu 24.04 runtime. No binary from `output/` or `dist/` is used.
The runtime installs the official Google Chrome .deb for the selected architecture,
fonts, CA certificates, curl, and tini. There is no Go toolchain in the runtime.
Optional version stamp: `--build-arg VERSION=1.0.0`.

## Compose (same directory layout as update-server)

From `readersv3/apps/html-print-formatter/docker/ubuntu`:

```sh
docker compose up --build -d
docker compose logs -f
```

Compose defaults to Linux amd64. On an ARM64 Linux host, use
`WMLR_PLATFORM=linux/arm64 docker compose up --build -d`.
Change the host port with `WMLR_PORT=29113`, or the host bind address with
`WMLR_BIND=0.0.0.0` for internal remote access. Data uses a persistent named volume.

The directory contains Dockerfile, entrypoint.sh, compose.yaml and README.md,
like update-server. Source compilation happens during `docker compose build`
(or `up --build`), not on each container restart. After pulling source updates,
run `docker compose up --build -d` to compile and deploy the new Linux binary.
No source checkout or Go toolchain is needed inside the running container.
`docker compose down` retains the data volume; adding `-v` deletes it.

## Run without Compose

```sh
docker run -d --name html-print-formatter \
  --cap-add=SYS_ADMIN --shm-size=512m \
  -p 127.0.0.1:19113:19113 \
  -v html-print-formatter-data:/app/deployments \
  --restart unless-stopped \
  wisemed-html-print-formatter:local

docker logs -f html-print-formatter
curl --fail http://127.0.0.1:19113/health
```

The process runs as UID/GID 10001, in the foreground, with tini forwarding
termination signals and reaping child processes. Chromium's sandbox stays enabled.
`SYS_ADMIN` allows its sandbox namespace operations under Docker's seccomp policy;
it is a broad capability, so use this internal service only for trusted documents.
Do not add `--privileged` or `--no-sandbox`. Host AppArmor/user-namespace policy
must also allow the Chromium sandbox. If it is denied, inspect the host policy
and container logs rather than disabling the sandbox.

The server binds to `0.0.0.0:19113` inside the container; the example publishes
only on the host's loopback interface. HTML/JavaScript can access the container's
network. To expose it remotely, configure the host binding and access controls
for your internal network. TLS is off by default, as in the utility template.

The named volume stores config.yaml, database, logs, and downloaded updates.
The entrypoint refreshes only config.install.yaml; the runner creates or merges
missing defaults while preserving operator settings in config.yaml. Helper files
come from the image at `/opt/wisemed/defaults/help`, so upgrades refresh them.
For bind mounts instead of a named volume, ensure the directory is writable by
UID/GID 10001. Mount your report fonts under `/usr/local/share/fonts/` or extend
the image to install them.

Supported environment overrides:

```sh
-e PORT=19113 \
-e MAX_CONCURRENT_RENDERS=2 \
-e MAX_BODY_BYTES=4194304 \
-e DEFAULT_RENDER_TIMEOUT_MS=120000
```

If changing PORT, also change the container-side port in `-p`. CHROME_PATH
already points to `/usr/bin/google-chrome-stable`. The healthcheck uses PORT;
if enabling HTTPS in config.yaml, adapt the healthcheck accordingly.

WiseMED WSS, update settings and identity remain configurable in config.yaml.
The default update-server address `127.0.0.1:19090` refers to this container;
set it to the actual reachable update server when using WiseMED updates.
For container deployments, rebuild and recreate the image to persist binary
upgrades across container replacement; the named volume preserves configuration.

## Convert HTML directly to PDF

No printReady signal is required by `/pdf`:

```sh
curl --fail-with-body -sS http://127.0.0.1:19113/pdf \
  -H 'Content-Type: application/json' \
  --data '{"html":"<!DOCTYPE html><html><body><h1>Linux PDF</h1></body></html>"}' \
  -o rendered.pdf
```

Use `/render` for DOM processing with mandatory `window.printReady === true`.
The browser UI and helper are available at `/` and `/help/`.

## Stop gracefully

```sh
docker stop --time 30 html-print-formatter
```

References: [Chrome Linux installation](https://support.google.com/chrome/answer/95346)
and [Chromium sandbox/container troubleshooting](https://pptr.dev/troubleshooting).
