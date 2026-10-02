# WSM Server — AI memory

Updated: 2026-09-24. This describes the current checked-out implementation, not a
running production deployment. Paths are relative to the repository root. The
application directory/binary is `wsm-server` (also called WMS/WiseMED WSS).
Read `wsm-server/AGENTS.md` first. This memory supersedes the 2026-09-21 audit:
its unauthenticated test-token endpoint, permissive role checks, missing TLS,
copylocks diagnostics and payload logging are no longer the current architecture.

## Start here

- `wsm-server/docs/remote-control.md`: equipment initialization, credentials,
  presence, generic API messages, control-window handoff and exclusions.
- `wsm-server/docs/protocol.md`: JWT, hello, 1:1/1:N, replies, scopes and failures.
- `wsm-server/docs/operations.md`: external TLS, installation, secrets and reload.
- `readersv3/docs/wss-control.md`: shared reader/utility setup and test commands.

## Architecture and boundaries

A central Go HTTPS/WSS process routes JSON between independently networked clients.
Each authenticated connection belongs to exactly one WiseMED instance (`tenant_id`).
Routing, presence, lists, topics, statistics and optional WiseMED HTTP upstreams
are tenant isolated. The hub has no analyzer business logic or patient database.
State is in memory; there is no durable queue, replay, offline command delivery,
cluster bus or exactly-once execution. An ACK confirms queue admission only.

All 26 equipment applications in `readersv3` share this transport: 22 readers,
`barcodeprinter`, `anaf-docsmart`, `esignature-server`, `signing-pad-utility`.
`update-server` is distribution infrastructure and is deliberately excluded.
Existing deployed binaries must be rebuilt/updated; changing this source does not
update installed software automatically.

Startup sequence is important:

1. Initialize/reinitialize equipment through the existing **WiseMED HTTP API**
   (`PUT /administrative/analyzer`), not through WSS. Concurrent startup callers
   share serialized initialization.
2. Persist/obtain a valid WiseMED `echipament_id` before attempting WSS. The signed
   `equipment_id` identifies the equipment; `reader_id` remains the app identity.
3. Obtain a JWT and open outbound WSS; green connected state requires `hello_ack`.
4. Reconnect after 30 seconds on failures. The UI shows a countdown; clicking
   disconnected status requests an immediate retry. Explicit local Disconnect
   pauses retries until Reconnect. A remote reconnect writes its reply first.

NAT/VPN/subnets need only permit outbound access to central WSS and WiseMED HTTP.
The browser never connects directly to a reader IP. `remote_ip` is the peer address
observed by the server, possibly the NAT/VPN/proxy address, not a discovered LAN IP.

## Authentication, keys and TLS

Native TLS uses operator-supplied PEM certificate/fullchain + private-key files,
TLS 1.2 minimum. Certificate verification remains enabled in readers; an optional
`ca_file` adds a private CA. Plaintext requires an explicit opt-in:
`allow_insecure_loopback` restricts the bind address to numeric loopback;
`allow_insecure_http` (added 2026-09-23) also permits private container-network
listeners such as 0.0.0.0 behind Nginx. TLS remains required by default. Omit the
TLS block for HTTP; configured certificates are still validated and used even if
HTTP opt-in is true. Never expose the plaintext backend publicly. New templates:
`deployments/config.http.yaml` and `deployments/nginx-wsm.conf`; build.sh includes
both. Local `output/deployments/local/config.http.yaml` preserves existing keys
and tenant settings while using 127.0.0.1:8090 HTTP. The original TLS config remains.
Public clients still use HTTPS/WSS; Nginx preserves Host including port for ticket
origin validation. No forwarded header bypasses JWT/tenant/origin checks.
Verified this mode with an actual temporary Nginx TLS listener proxying to an
HTTP WSM listener: health, control UI, JWT WebSocket upgrade and hello_ack passed
with TLS certificate verification. Both temporary processes were stopped. Server
race tests and vet passed, including explicit HTTP opt-in/default rejection tests.

JWT is HS256 with required `kid`, `iss`, `aud`, `sub`, `tenant_id`, `role`,
`client_id`, nonempty `scopes`, `iat`, `exp`; role is browser/reader/service.
Readers additionally carry reader/equipment identities. A key is bound to a tenant
and allowed roles/scopes; device keys may also bind subject, reader_id and
equipment_id. Claimed scopes must be a subset of the key's permissions. JWT expiry
closes the socket. Default maximum token lifetime is 900s.

Secrets live in `secret_file` or `secret_env`, minimum 32 bytes; no source secrets
or public token generator. `wsmctl keygen` writes a new protected file;
`wsmctl token` is an offline operator utility. The tenant master key belongs only
on trusted server/backend systems. Per-device `device_key` credentials are allowed
only with server-side identity restrictions. Reader auth modes:

- `device_key`: equipment key from WiseMED initialization or external secret file;
- `token_endpoint`: configured relative authenticated WiseMED HTTP route returning
  a JWT; that backend route must actually be implemented/provisioned externally;
- `token_file`: read JWT file on every attempt; an external issuer refreshes it.

Bearer Authorization works for native clients. Browser JWT uses `wsm.v1` and
`wsm.jwt.<JWT>` subprotocols; query tokens are disabled by default. Explicit allowed
origins apply. Server responses negotiate only `wsm.v1`, never echo the credential.

A successful Unix SIGHUP reload atomically loads config/keys/certificate, invalidates
unused control tickets and closes **all** sockets, including other tenants and
pre-hello connections. Invalid reload keeps the previous configuration. Listener,
TLS mode and server limit changes require restart; Windows uses restart. Logs go
to stderr/journal and omit credentials, query strings and medical message bodies.

## Routing and common API adapter

Envelope: type, request_id, correlation_id, target, payload; the server stamps
sender identity/tenant/scopes and transport metadata. Never trust caller-provided
sender fields. IDs are bound at hello and cannot be changed by repeated hello.
Only one reader connection per tenant+reader_id and tenant+equipment_id is allowed.

Targets include connection/reader/equipment, their explicit ID lists, client_type,
topic, self and all. No target or legacy broadcast=true is rejected. List/broadcast
routes require route:broadcast in addition to the message scope. A target in
another tenant is indistinguishable from an absent target.

`equipment.status` supports one ID or up to 100 IDs; `equipment.list` and
`list_connections` expose tenant-local presence. HTTP GET `/api/equipment/{id}` and
`/api/connections` require JWT+connections:read. Public `/healthz` is liveness only.
Reader diagnostics support device.ping, ws.reconnect and debug.message, gated by
route:command + devices:debug when a reader initiates peer commands.

The universal command is `api.request`, requiring route:command + api:invoke:

```json
{"type":"command","request_id":"example-1","target":{"mode":"equipment","equipment_id":"42"},"payload":{"command":"api.request","args":{"method":"GET","path":"/api/status"}}}
```

`readersv3/shared/apibridge` invokes the **existing local HTTP mux in-process** with
a verified principal. No loopback HTTP request or second implementation of the API
is used. Local session checks recognize that principal; api:admin enables endpoints
that require an admin session. Ordinary local users cannot use WSS administration,
open/debug/bridge to inherit privileged runtime credentials.

Result is reply.payload `{kind:"api.response",status,content_type,headers,body}`
or `body_base64` for binary. HTTP error status is preserved; routing ACK is not the
result. Request bodies use JSON body or base64+content_type (multipart supported).
Methods: GET/HEAD/POST/PUT/PATCH/DELETE. Decoded request and raw response maximum
4 MiB; production max_message_bytes is 8 MiB to allow base64/envelope overhead.
The bare config default remains 1 MiB; do not omit the larger production setting
when large API transfers are needed. Requests time out; commands with side effects
are not automatically replayed after ambiguity/disconnect.

Exclusions: HTML/static UI, HTTP streaming/WebSocket upgrades, local cookie
login/logout, and `/api/wss/open`, `/api/wss/debug`, `/api/wss/bridge` inside the
adapter (prevent delegation/relaying beyond a scoped equipment). Existing hardware
operations still require their drivers and actual hardware. Printing uses JSON
models rendered by the browser; eSignature demo uses the common HTTP command
handler through WSS instead of opening the local /ws/demo socket.

## Remote control window

The same bundled local UI is packaged centrally under `/control/`; its assets are
ordinary static HTTP(S), but API data is exclusively WSS. `wss-remote.js` is loaded
before app.js, converts fetch requests into api.request, preserves status/binary
responses, correlates concurrent replies, supports AbortSignal and bounded request
timeouts, and keeps navigation under /control with a logical route query.

An admin clicks Open in Debug WSS. Local POST `/api/wss/open` asks the central
`control.ticket` command for a cryptographically random single-use ticket. It is
usable within one minute; the resulting browser lease is at most five minutes and
never exceeds the parent JWT expiry. Child role is browser, client_id is control,
and the server permits only ping and api.request to the selected equipment.
Tickets cannot delegate more tickets. No credential appears in URL or storage.

The child sends `wsm-control-ready`/`wsm-control-refresh`; the opener responds with
`{type:"wsm-control-auth",ticket}`. Both sides validate exact origin and window
source. The socket uses `wsm.ticket.<ticket>`. Manual standalone ticket entry is
also supported. First API calls wait for the first successful authentication even
if credentials arrive late or an initial handshake fails. Canceled calls are never
replayed. Subsequent disconnected/renewal retries use a 30s countdown. Remote
logout closes only the control session, not the equipment connection.

Local WSS routes in every equipment app: GET status, PUT settings, POST control,
debug, bridge and open under `/api/wss/`. `/api/wss/bridge` is the reusable local
example endpoint for sending a chosen HTTP API method/path/body to a peer using WSS.
Remote UI hides prohibited relay actions and distinguishes browser reconnection
from reconnection of the selected equipment.

## Source map

| Path | Responsibility |
| --- | --- |
| `wsm-server/cmd/wsm-server/` | Startup, validation flags, signals |
| `wsm-server/cmd/wsmctl/` | Offline key generation/token issuance |
| `wsm-server/internal/config/` | Strict YAML, secrets, tenant policy, TLS |
| `wsm-server/internal/server/auth.go` | JWT claims and permission validation |
| `wsm-server/internal/server/server.go` | HTTP/WS lifecycle, authorization, dispatch |
| `wsm-server/internal/server/control.go` | Single-use tickets and static control assets |
| `wsm-server/internal/server/hub.go` | Tenant registry, routing, queues, snapshots |
| `wsm-server/internal/server/wisemed.go` | Equipment queries, optional per-tenant HTTP proxy |
| `readersv3/modules/ws/module.go` | Shared reader auth, reconnect, dispatcher and traces |
| `readersv3/shared/apibridge/` | Reuse of HTTP handlers and verified principal |
| `readersv3/modules/localhttp/wss.go` | Authenticated local WSS management endpoints |
| `readersv3/modules/localhttp/ui/wss-remote.js` | Browser remote transport |
| `readersv3/modules/wisemedapi/` | HTTP initialization and token endpoint |
| `wsm-server/integration/` | Separate Go module for server+reader TLS integration |

The historical module path is `wisemed-labreaders/serverlast/wsm-server`; it does
not imply a `Server Last/` folder. Server and readersv3 are separate Go modules.

## WiseMED administrator console (2026-09-24)

Embedded UI `/admin/` provides an allowlisted browser CLI: `help`, `add <equipment_id>
[reader_id]`, `list`/`devices`, `key <equipment_id>`, `clear`, `logout`. It cannot
execute OS commands. Enable `admin.enabled` and set `admin.public_origin` to the
exact public HTTPS origin, including nonstandard port; loopback HTTP is allowed
only for explicit development. Login is tenant-scoped PUT /administrative/login
against the configured tenant.wisemed HTTPS API using its backend credential.
Only explicit canonical user_type -1 is accepted, with a login token and no
failure response. User type 0 or another negative type is NOT administrator here.
Passwords/WiseMED tokens are not persisted. Random server sessions have bounded
TTL, HttpOnly/SameSiteStrict cookies (Secure in HTTPS), CSRF+exact-origin mutation
checks, login rate limits, and reload invalidation.

Generated equipment records live in `security.device_keys_file`, a separate
writable0600 YAML registry loaded with main config. Atomic persisted writes precede
publishing new auth state; adding devices doesn't disconnect existing peers.
Main config can stay read-only; Docker needs persistent writable /var/lib/wsm-server
(UID/GID65532). systemd StateDirectory provisions the equivalent. This is still
single-active-instance state, not shared horizontal scaling. Registry is secret
material and excluded from image/Git. No master/backend keys are displayed.

Creation generates a device key bound to tenant/equipment and a generated subject;
reader_id binding is optional, so `add 1` works without knowing the runtime's
reader_id. Reader JWT still must contain its real reader_id. Default generated
permissions cover reply/event/diagnostics, not api:invoke/admin. Returned identity
fields and secret are pasted into Debug WSS. `list` includes admin-managed devices
(including offline), not arbitrary master/static keys, and never contains secrets;
explicit `key` retrieves a managed device key for the same tenant (user requested
retrieval, not one-time-only semantics). UI keeps it in an ephemeral separate
panel, not history/browser storage. Delete API revokes generated credentials.

Reader `modules.wisemed-ws.device_secret` now has a masked editable UI field and
YAML persistence. Blank save preserves the existing key; settings API returns only
`device_secret_configured`. Auth precedence is explicit device_secret, secret_file,
then api_key_echipament. Source updates apply to all26 shared applications; rebuild
installed readers to get the field. Keys aren't returned by reader status API.

Local runtime admin config was prepared for https://wslocal.wisemed.eu/admin/ and
tenant local, reusing Yumizen's existing HTTPS WiseMED API configuration. API key
was copied privately to ignored output/deployments/keys/local-wisemed-api.key;
registry path is output/state/device-keys.yaml. No live login/request was made.
Real WiseMED authentication must still be verified by the operator. Read
wsm-server/docs/admin-console.md for API/config/provisioning details.

## Build, tests and remaining deployment work

Docker packaging added 2026-09-23: build from repo root with
`docker build -f wsm-server/Dockerfile -t wisemed-wsm:local .`. Multi-stage image
contains static server/wsmctl binaries, system CAs and the four shared UI assets
at `/opt/wsm-server/control`; no runtime repository dependency. UI is packaged
alongside the binary, not go:embed. Dockerfile-specific ignore excludes deployment
secrets and runtime data. Mount config/certs/keys under `/etc/wsm-server`, readable
by UID/GID 65532; use address 0.0.0.0 and control_ui_dir /opt/wsm-server/control.
The GoLand source-relative control_ui_dir remains a development-only setting.
Docker daemon was unavailable during this change, so the image itself has not
been built/run locally; isolated directory deployment is verified separately.

Local GoLand startup was consolidated on 2026-09-23 at user request:
`Last ServerWS` uses working directory `wsm-server/output`, arguments
`-config deployments/config.yaml --showlog`. Main defaults to deployments/config.yaml
and logs the resolved absolute path. There is no automatic search/fallback to local/.
Source deployments/config.yaml is now a valid HTTP template; runtime config lives
in output/deployments/config.yaml, allows HTTP on 0.0.0.0:8090, omits TLS, references
keys/local-backend.key and ../control. Old source/runtime configs were backed up in
ignored output/deployments/.backups; old local/ TLS files are retained but inactive.
prepare-local.sh creates missing config/key and refreshes output/control assets;
it preserves existing configuration/keys. Credentials/issuer were retained from
the prior local setup; real reader provisioning and public origins remain operator
configuration. The production TLS template remains available.

The combined repeatable suite is `wsm-server/verify.sh`; it runs the module checks,
app builds, Node tests and the real-delay integration test. Equivalent commands:

```sh
cd wsm-server
go test -race ./...
go vet ./...
./build.sh
cd ../readersv3
go test -race ./...
go vet ./...
node --test modules/localhttp/ui/wss-ui.test.cjs modules/localhttp/ui/wss-remote.test.cjs
cd ../wsm-server/integration
go test -race -v ./...
```

`build.sh` packages binaries, production config template, systemd unit, docs,
example reader and copies the shared static UI to dist/control. It includes no
secrets or certificates. `server.control_ui_dir` selects the installed assets;
use an absolute deployment path or a path relative to the config directory.
Legacy accepted_keys configurations are incompatible with the strict schema.
The source config.yaml is now HTTP; config.production.yaml is the direct TLS template.

Tests cover JWT failures, tenant routing, role/scope restrictions, origins, expiry,
reload/revocation, hello/rate/connection limits, duplicate identities, backpressure,
50 actual WSS sockets, external TLS certificates and clean shutdown. Proxy tests
use simulated TLS WiseMED upstreams. Reader tests cover initialization, auth/config,
API bridge and local admin restrictions. Browser tests cover transport state,
countdown, correlation, cancellation, binary/empty bodies and secure ticket handoff.
The integration module uses a real TLS hub and real reader transport, simulated
WiseMED/hardware, and includes an approximately 31s real retry check.

Completed local verification on 2026-09-22: server and readersv3 full
`go test -race ./...` and `go vet ./...` passed; `go build ./apps/...` passed for
readersv3. TLS server/reader integration passed, including the actual 30s retry
(about 32s total). UI tests passed (9 WSS UI + 12 remote transport). Final native
and Linux amd64 WSM binaries were built with Go 1.26.8; binary govulncheck reported
no vulnerabilities. These are local test results, not deployment certification.
Actual headless Chrome smoke also passed using unchanged shared UI assets and
remote bootstrap with simulated API/WebSocket data: normal reader, barcode,
Docsmart and eSignature startup/navigation, popup ticket handoff/reload, print,
help and logout. No JavaScript runtime errors or direct remote HTTP API/help calls
were observed. This browser check is separate from the real TLS socket test.
Reader preview artifacts were built under
`readersv3/dist/wss-preview/20260921T225216Z`: all 26 darwin-arm64 equipment apps
plus a Windows 386 eSignature package with its DLL. Native executable version
checks passed; the Windows package still needs validation on Windows/hardware.
Native server artifacts are under dist/bin; Linux artifacts under
`dist/linux-amd64/bin`. `WSM_OUTPUT_DIR` selects build output independently.

No central-server installation, real DNS/certificate deployment, real WiseMED
backend integration or physical analyzer/PAD/printer validation was performed.
Real tenant/device secrets, external certificate/key paths and optional token
backend route remain operator provisioning work. Test success does not establish
production load capacity or operation of every analyzer protocol on real hardware.
Use dedicated temporary config/keys/CA and loopback for further checks; do not
contact live WiseMED or overwrite deployment/runtime state merely to test routing.

Verification on 2026-09-24 for admin provisioning: server and readersv3 full race
suites and go vet passed; readers go build ./apps/... passed. Combined Node tests
passed (12 remote transport, 10 reader WSS, 4 admin). Headless Chrome admin smoke
passed with a simulated backend. Backend tests include strict role rejection,
CSRF/origin/session/tenant isolation, concurrent persisted creates, actual config.Load
restart, persistence failure, and real TLS device authentication/revocation. Ordinary
key additions preserve unrelated sockets and preissued remote tickets; only explicit
Reload increments the authentication epoch. Real WiseMED login remains untested.
Server/reader TLS integration was rerun after the authentication-epoch fix and
passed (31.4s, including reconnect delay). Native and Linux amd64 packages were
rebuilt, and the active output/deployments/config.yaml passed -check-config.

Admin console now supports `edit <equipment_id> <reader_id>`,
`edit <equipment_id> --remove-reader`, and `delete <equipment_id>`.
PATCH /admin/api/devices/{keyID} requires reader_id (empty string removes binding),
preserves key/subject/secret, persists atomically and closes only this device's
sockets for reauthentication. DELETE revokes WSS access, not the WiseMED record.
reader_id is the runtime application's text identity, not the WiseMED numeric ID.
Server race suite and vet passed for this change; tests cover edit auth/CSRF/tenant
isolation, validation, persisted binding/secret retention, real TLS disconnection,
JWT restriction add/remove and failed edit/delete writes leaving state unchanged.

2026-09-24 live troubleshooting: public wslocal.wisemed.eu TLS validated, but
GET /healthz timed out both through Nginx and directly on localhost:8090. The
WSM process was in TX (stopped/traced), parent debugserver: debugger pause prevented
HTTP handling. Reader saved issuer already matched server https://localhost:8443
(the earlier screenshot had a mismatch). No certificate bypass or proxy change
was needed. Reader connection diagnostics now distinguish DNS, TLS trust/name,
refusal and timeout without exposing raw errors; reconnect immediately clears
stale paused phase. Admin key instructions now refer to the editable Cheie WSS.

Admin device permission editor (2026-09-24): `rights <equipment_id>` lists a server
catalog with explanations/check marks; `toggle <numbers>`, `save`, `cancel` manage
an unsaved browser draft. Device.Scopes optional YAML list preserves legacy defaults
when absent. PATCH accepts scopes and/or reader_id, validates an allowlisted catalog
and api:invoke -> route:command, api:admin -> api:invoke dependencies. Effective
scopes appear in list/key responses. Persist-before-publish and targeted disconnect
apply; update/delete also invalidate pending tickets with matching tenant+subject.
Reader requested scopes are still configured separately; console prints CSV after
save. Do not silently expand device privileges or master credentials.
Live read-only configuration test through wslocal.wisemed.eu passed: reader secret
and issuer matched, device JWT status 200 and equipment 1 online. Temporary operator
WebSocket got a ticket, ticket socket hello_ack, and routed GET /api/status returned
200 through WSS. The device itself lacked api:invoke; no analyzer operation or
permission modification was performed against live state. Server full race tests
and vet plus six Node admin tests passed for permission editing.

Docker versioning: build-docker.sh (repo root invocation) generates UTC timestamp
YYYYMMDD-HHMMSS without Git SHA, defaults linux/amd64 for EC2 x86_64, passes
WSM_VERSION build arg and OCI version label, loads timestamped image and exports
atomically to dist/docker/wisemed-wsm-<timestamp>.tar.gz. Binary --version and startup
logs use injected main.buildVersion; source-only builds show dev. build.sh injects
a timestamp too. Dockerfile direct build has timestamp fallback for binary, but
automatic image tag/export require build-docker.sh. Shell syntax and mocked Docker
export pipeline verified; native version-injected binary compiled and checked.
Actual cross-platform Docker build not performed in this change.

Root make-wsm-docker.sh delegates to build-docker.sh, preserving the user export
destination ~/dockers and amd64 default. No fixed :1 tag/archive remains. Wrapper
works from any working directory; verified with mock Docker and temporary output.

Root update-wsm.sh installs timestamped archives from the fixed private SSH repo
git@github.com:AlwaysSmarter/dockers.git. Run as SSH-equipped ec2-user; sudo Docker
fallback. Default latest tracked archive or explicit version; checks gzip, host
architecture, binary version and config before replacing. Uses immutable image ID,
standard /srv/wsm mounts, localhost8090, hardening/log rotation, health polling,
rollback traps preserving old container settings, cache flock and temporary clone
cleanup. No secrets/production state modifications or Git push performed locally.
Offline orchestration tests passed (success, rollback, preflight rejection, same
image no-op); actual EC2/private repo access has not been tested. Docs docker-update.md.

2026-09-29 Docker build fix: user hit Go compiler SIGSEGV while running AMD64
builder under emulation on M2. Dockerfile now uses FROM --platform=$BUILDPLATFORM
and cross-compiles both binaries with CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH.
Actual ./make-wsm-docker.sh completed on Docker Desktop ARM64; output image
wisemed-wsm:20260928-213907 is linux/amd64, docker run --version returned the same
UTC version, and ~/dockers/wisemed-wsm-20260928-213907.tar.gz passed gzip -t (~10MB).
This supersedes the prior untested-Docker limitation for build/export; not an EC2
deployment test. No push or remote deployment performed.

2026-09-29 admin UI diagnosis: failed commands previously displayed errors only
in the notice footer, cleared by the next command; authenticated action errors
now also remain in text-only command history. Empty list after add requires the
actual API error; check registry path and writable state mount, not an extra save.
Creation persists immediately; save applies only the rights editor.

### SIUI utility integration (2026-09-29)
- The new readersv3 `siui-bridge` utility exposes its authenticated HTTP API through the existing WSM `api.request` adapter; no new WSM message protocol. Validation submission returns a job ID promptly, and callers poll `/api/siui/validations/{id}` to stay within the per-message deadline.
- Standard shared control UI now includes `/settings/siui`, certificate selection and validation testing when the target protocol is `siui`. Rebuild shared control assets when distributing this UI. Administrative certificate/settings actions require api:admin; job history is tenant/subject scoped. No .cer export or token PIN transmission over WSM.
