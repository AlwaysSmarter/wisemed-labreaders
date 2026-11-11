# wisemed-labreaders memory

## Application-specific memories

- [WSM Server](wsm-server.md): current server architecture, protocol, startup,
  verification and known gaps. Agent entrypoint: `wsm-server/AGENTS.md`.
- [Shared WSS control for readers and utilities](../readersv3/docs/wss-control.md):
  setup, remote control, the common API adapter and verification commands for all 26 equipment apps.

## Workspace

- Root workspace: `/Users/raduichim/work/gowork/wisemed-labreaders`
- Main active reader project in this thread:
  - `/Users/raduichim/work/gowork/wisemed-labreaders/Readers Last/generic-test-reader`

## Project generation rules

- Every new reader must include EVERYTHING in the standard reader layout before delivery: app/module registration, configuration, help and protocol docs, branded PNG/ICO icons, build/install/service scripts, and a populated `readersv3/output/<reader>/` workspace with runnable binary, `deployments/config.yaml`, `config.install.yaml`, and help assets. Verify these explicitly; a `dist/` package alone is insufficient. Preserve existing local configuration and runtime data when refreshing output.

- When generating a new readersv3 application or a new related project, also generate an application icon.
- The Windows delivery path should include a real `.ico` file derived from the WiseMED brand mark but made distinct for that specific app by color, label, or app-specific graphic accent.
- Keep generated icon assets under the project resources tree so they can be reused by packaging/build automation.
- Prefer a white rounded-card background for generated icons, with transparent outer corners.
- On that white card, place the WiseMED logo and one clear application badge only.
- Do not add extra text on the main canvas, and do not add extra colored geometric decorations beyond the badge itself.
- Use a visible colored badge with high-contrast text; the `CARY60` `UV` badge style is the preferred reference for short app markers.

## Protocol docs

- For protocol and communication specifications, prefer the per-app docs directory under `readersv3/apps/<app-name>/docs/`.
- Treat `readersv3/apps/<app-name>/docs/` as the primary local source for analyzer communication notes, payload examples, and protocol exceptions observed in the field.
- If protocol notes also exist in module-level docs, keep them aligned with the app-level docs and assume app-level docs are the more current operational reference.
- For `labnovation-ld-560`, the real `Simple` payload observed in the field can differ from the original `LD-560LIS connection protocol.docx`; do not assume the `<I>` field layout from the old document is always authoritative.

## High-level purpose

`generic-test-reader` is a local WiseMED reader runtime that:

- bootstraps/configures a reader instance
- stores local state in SQLite
- exposes a local HTTP admin UI
- connects to WiseMedWS for command/event exchange
- supports communication modes:
  - `file` fully operational
  - `serial` configurable but runtime not implemented
  - `network` configurable but runtime not implemented

## Important entrypoints

- Binary entrypoint:
  - `cmd/generic-test-reader/main.go`
- Reader runtime:
  - `internal/reader/app.go`
  - `internal/reader/bootstrap.go`
  - `internal/reader/command.go`
  - `internal/reader/service.go`
  - `internal/reader/fileimport.go`
- Local HTTP server:
  - `internal/webui/server.go`
- Web UI:
  - `internal/webui/ui/index.html`
  - `internal/webui/ui/styles.css`
  - `internal/webui/ui/app.js`
- Persistence:
  - `internal/storage/sqlite.go`
- Domain models:
  - `internal/model/types.go`
- Configuration:
  - `internal/config/config.go`
- WiseMed API client:
  - `internal/wisemedapi/client.go`

## Config model

Main YAML config sections:

- `wisemed_api`
- `wisemed_ws`
- `local_http`
- `reader`
- `communication`
- `layout`
- `capabilities`

Useful defaults from `internal/config/config.go`:

- `LocalHTTP.Enabled = true`
- `LocalHTTP.Address = 127.0.0.1:18080`
- `LocalHTTP.Language = ro`
- `Comm.Type = file`
- `Comm.Protocol = GENERIC`
- file polling and archive paths default under local folders

Reader-specific identifiers available in config/status:

- `reader.id`
- `reader.client_id`
- `reader.label`
- `reader.api_key`
- `reader.analyzer_name`
- `reader.analyzer_code`
- `reader.medical_unit_id`
- `reader.equipment_id`
- `reader.equipment_type_id`

## Storage model

SQLite main tables:

- `analytes`
- `orders`
- `rounds`
- `order_analyses`
- `order_analysis_results`
- `event_logs`

Important model relationships:

- one `Order`
- many `OrderAnalysis`
- each analysis has many `OrderAnalysisResult`
- `rounds` are tracked per `order_date`

## Orders and rounds logic

Current intended behavior from this thread:

- `sample_no` must restart from `1` for each `order_date`
- `list_position` is legacy and no longer part of active UX logic
- UI should always show at least round `1`
- when a date changes, the round selector should auto-populate and pick the last round for that date
- import/manual operations should use the selected `order_date`, not always `today`
- schema migration must support old DBs on startup

Known code areas involved:

- `internal/storage/sqlite.go`
- `internal/reader/fileimport.go`
- `internal/webui/server.go`
- `internal/webui/ui/app.js`

## Import pipeline

Key import file:

- `internal/reader/fileimport.go`

Supported import styles:

- generic JSON
- generic CSV/delimited files
- IR Biotyper CSV

Important behaviors established in this thread:

- parser logs were expanded with detailed `log.Printf(...)`
- import should also create useful `event_logs` entries for UI visibility
- manual upload previously returned false success because `stableWait` short-circuited the import path
- fix direction:
  - background scan keeps stable-wait/deferred behavior
  - manual UI import must bypass stable-wait
  - manual UI import must return real summary and fail if `0` records imported

Important note:

- There were active edits to make `ImportFileNow(path, orderDate)` and downstream import/storage functions respect explicit import dates. If future work touches import, verify all call sites still match the latest signatures.

## Web UI structure

The local admin UI is a server-rendered static frontend served by `internal/webui/server.go` and implemented in:

- `internal/webui/ui/index.html`
- `internal/webui/ui/styles.css`
- `internal/webui/ui/app.js`

Current UI themes/features added during this thread:

- compact top bar instead of oversized header panel
- toast below/in the header area for import feedback
- orders workspace/table redesign
- analytes workspace/table redesign
- analyte edit/add moved into a modal popup
- analyte refresh button added in module toolbar
- analyte save/delete now refreshes state without page reload
- analyte edit semantics are now identity-by-`id`, not by `tag`
- sidebar made sticky with viewport-height behavior
- sidebar now includes:
  - visual identity card
  - reader summary card

Analyte UX note:

- edit requests must target `/api/analytes/<id>`
- changing `tag` during edit should update the same row
- if the new tag already exists on another analyte, backend returns validation error

## API endpoints used by UI

Observed/used endpoints include:

- `GET /api/preferences`
- `PUT /api/preferences/language`
- `GET /api/session`
- `POST /api/session/login`
- `POST /api/session/logout`
- `GET /api/config`
- `PUT /api/config`
- `GET /api/config/<section>`
- `PUT /api/config/<section>`
- `GET /api/status`
- `GET /api/stats`
- `GET /api/dashboard`
- `GET /api/logs`
- `GET /api/analytes`
- `POST /api/analytes`
- `GET /api/analytes/:id`
- `PUT /api/analytes/:id`
- `DELETE /api/analytes/:id`
- `GET /api/orders`
- `GET /api/order-analysis?order_id=<id>`
- `POST /api/order-analysis`
- `GET /api/order-analysis/<id>`
- `PUT /api/order-analysis/<id>`
- `DELETE /api/order-analysis/<id>`
- `POST /api/orders/import`
- `POST /api/orders/export`
- `POST /api/orders/rounds`

Important API behavior:

- `GET /api/orders` supports `include_analysis`
  - `include_analysis=1` returns `OrderBundle[]`
  - omitted or `0` returns plain `Order[]`
- analyte update/delete is now by `id`, not by `tag`
- analyte `tag` remains unique
- config/statistics now have dedicated APIs:
  - `GET/PUT /api/config`
  - `GET/PUT /api/config/<section>`
  - `GET /api/stats`

## Commands/events

Reader WS commands from README:

- `reader.status`
- `stats.get`
- `stats.series`
- `config.get`
- `config.set`
- `logs.list`
- `logs.tail`
- `logs.activate`
- `logs.deactivate`
- `results.activate`
- `results.deactivate`
- `analytes.list`
- `analytes.get`
- `analytes.create`
- `analytes.update`
- `analytes.delete`
- `orders.list`
- `orders.rounds`
- `orders.get`
- `orders.create`
- `orders.update`
- `orders.delete`
- `order_analysis.list`
- `order_analysis.get`
- `order_analysis.create`
- `order_analysis.update`
- `order_analysis.delete`
- `results.list`
- `comm.get`
- `comm.set`
- `imports.run_file`

Events:

- `log`
- `tick`
- `result_available`

Important WS command behavior:

- `orders.list` supports `include_analysis`
- `orders.rounds` returns all rounds for a given `order_date` and the latest/current `round_no`
- `analytes.get` accepts `id` or `tag`
- `analytes.update` must use `id`
- `analytes.delete` is by `id`
- `comm.get` / `comm.set` still exist for backward compatibility
- `config.get` / `config.set` are broader and preferred for new clients
- `results.activate` returns the dedicated real-time topic `results:<reader_id>`
- `result_available` is emitted when a new result is persisted and is the correct trigger for client auto-refresh

## Localization notes

UI localization is dictionary-based in `internal/webui/ui/app.js`.

Important expectations from thread:

- Romanian should be the default/fallback language
- all visible control labels in Orders should be translated
- examples explicitly requested:
  - `Get worklist` translated
  - `Detalii order` changed to `Detalii proba`
  - orders table column headers translated

## Config and stats notes

The reader now supports full config snapshot/update and explicit dashboard stats aggregation.

Supported config sections:

- `wisemed_api`
- `wisemed_ws`
- `local_http`
- `reader`
- `communication`
- `layout`
- `capabilities`

Config behavior:

- `GET /api/config` returns full runtime config
- `PUT /api/config` applies partial top-level patches and persists YAML
- `GET /api/config/<section>` / `PUT /api/config/<section>` operate on one section

Stats behavior:

- `GET /api/stats?order_date=YYYY-MM-DD` returns date-scoped counters under `data.stats`
- `GET /api/stats/series?series_limit=14` returns daily series under `data.series`
- legacy dashboard-style aggregate data remains available through `/api/dashboard`, not through `/api/stats`
- dashboard/statistics overview must be date-based:
  - use `orders.order_date` as the source of truth
  - do not split or aggregate by `round_no`, `rack_no`, or `rack_position`
  - `today.with_result` / `today.without_result` are calculated from analyses attached to orders whose `order_date` is today
- `stats.get` is the date-scoped WS command:
  - args: `{ "order_date": "YYYY-MM-DD" }`
  - response data: `{ "order_date": "...", "stats": { "analytes": 9, "events": 365540, "orders": 32, "results": 47 } }`
  - `orders` and `results` are filtered by `orders.order_date`
  - `analytes` and `events` are global counters
- `stats.series` is the WS command for date series:
  - args: `{ "series_limit": 14 }`
  - response data: `{ "series": [DashboardSeriesPoint, ...] }`
- HTTP mirrors this split:
  - `GET /api/stats?order_date=YYYY-MM-DD`
  - `GET /api/stats/series?series_limit=14`
- `GET /api/orders/rounds?order_date=YYYY-MM-DD` returns all rounds for a specific day:
  - `rounds: [1, 2, ...]`
  - `round_no`: the latest/current round for that day
  - if `order_date` is omitted, the reader uses the current local day
- WS `orders.rounds` mirrors the same behavior without requiring local HTTP session auth:
  - args: `{ "order_date": "YYYY-MM-DD" }`
  - response: `{ "order_date": "...", "round_no": 3, "rounds": [1, 2, 3] }`

## Real-time logs note

The local web UI does not use `logs.activate`; it uses HTTP polling via `GET /api/logs`.

For WS clients that use `logs.activate`:

- log events are still persisted in `event_logs`
- live broadcast now filters transport-noise events to avoid an infinite feedback loop
- filtered from live stream only:
  - `event_type` in `{ws_rx, ws_tx, ws_ping, ws_pong}`
  - or `message` in `{received ws message, sent ws message}`
- `logs.list` / `GET /api/logs` should still contain those persisted transport logs
- transport WS logs are now intentionally compacted:
  - new `ws_rx` events persist only a summary, not the full nested payload
  - `logs.list` / `logs.tail` sanitize old transport-log payloads before returning them
- reason: avoid `websocket close 1009 (message too big)` when a client requests logs in real time and the database already contains oversized nested WS payloads

## Real-time results note

Clients that need automatic refresh on new analyzer data should not watch log traffic.

Use the dedicated results flow:

- send `results.activate`
- read back the topic `results:<reader_id>`
- subscribe in WiseMedWS to that topic
- refresh UI on each `result_available` event

This event is transport-agnostic by design and is intended to be emitted regardless of whether the reader receives data from file import, serial, or TCP/IP.

## Visual identity notes

Sidebar visual identity was extended to classify reader category heuristically from `reader.analyzer_name` and `reader.label`.

Current categories:

- urine
- hematology
- biochemistry
- immunology
- gas chromatograph
- spectrophotometer
- generic fallback

Current implementation is heuristic string-matching in frontend JS. If robustness matters, move this mapping server-side or derive it from `equipment_type_id`.

## Known issues / gotchas

- Workspace path contains spaces: `Readers Last/...`
- Go import/module paths use `readerslast/...`, so some local build/test flows are fragile
- Sandbox/network restrictions can block `go build` dependency downloads unless module cache already exists
- Be careful not to reintroduce nested SQLite query stalls; there was an issue previously related to limiting SQLite to a single open connection
- Old DB compatibility matters; schema migration logic in `sqlite.go` is part of normal startup expectations
- If investigating “live logs stopped”, distinguish:
  - local admin UI, which polls HTTP logs
  - WS subscribers using `logs.activate`, which now intentionally do not receive `ws_rx/ws_tx/ws_ping/ws_pong`

## Practical workflow reminders

- Prefer `rg` for searching
- Use `apply_patch` for file edits
- If working on import bugs, check both:
  - `event_logs` visibility in UI
  - raw daily log file under `deployments/<reader-id>-YYYYMMDD.log`
- If a frontend change seems not visible, verify the user actually restarted the reader binary serving the local UI

## Files likely to matter first for future fixes

- `/Users/raduichim/work/gowork/wisemed-labreaders/Readers Last/generic-test-reader/internal/reader/fileimport.go`
- `/Users/raduichim/work/gowork/wisemed-labreaders/Readers Last/generic-test-reader/internal/storage/sqlite.go`
- `/Users/raduichim/work/gowork/wisemed-labreaders/Readers Last/generic-test-reader/internal/webui/server.go`
- `/Users/raduichim/work/gowork/wisemed-labreaders/Readers Last/generic-test-reader/internal/webui/ui/app.js`
- `/Users/raduichim/work/gowork/wisemed-labreaders/Readers Last/generic-test-reader/internal/webui/ui/styles.css`
- `/Users/raduichim/work/gowork/wisemed-labreaders/Readers Last/generic-test-reader/internal/webui/ui/index.html`

2026-09-24: Shared localhttp navigation groups Communication and WSS under Debug.
WSS retains /settings/wss for compatibility, but highlights Debug instead of Settings.
Communication retains existing debug availability/admin checks (disabled when
unavailable); WSS does not require analyzer debug fixtures. Settings order: analytes,
daily-details, daily-analysis-filters (Filtre), astm-specimens (Tipuri de probe),
reader, qc, plus app-specific PAD/demo entries where applicable. Translation uses
subview identifiers, not positional indexes that mislabeled Filters as QC.
Shared Node WSS tests and Chrome local/remote reader+utility smoke passed.

### SIUI Bridge (2026-09-29)
- Added `readersv3/apps/siui-bridge`, protocol `siui`, communication `utility`, using the standard runner/service/logging/login/HTTP/HTTPS/WSM infrastructure. Scope is PIAS paraclinical 72h validation (`SiuiValidateWS.validateReport`, PARA/RQ_PARA_SRV), not all PIAS services or patient eCard operations.
- Windows native WinHTTP uses a selected MY store certificate (CurrentUser/LocalMachine + thumbprint), without exporting a .cer or private key. Select via `/settings/siui`; service identity must have token access. CNAS activation licence is a separate local protected text file.
- `/api/siui/validations` is asynchronous and idempotent per authenticated owner + operation_id, with SQLite history, polling, no automatic submission retry, and unknown state after interrupted submission/restart. Same API works through WSM api.request. XML request/response use embedded official XSD validation; mocks never contact CNAS.
- See `readersv3/apps/siui-bridge/README.md` and `docs/protocol.md`. Real token/CNAS acceptance still needs Windows testing. Local NSIS aborts even on the standard minimal self-test (std::bad_alloc); Windows executable/update ZIP build, but local Setup generation is blocked by that tool.

### Local HTTPS trust fix (2026-10-03)
- SIUI uses the shared localtls implementation, with localhost already in SAN. Prior Windows PowerShell CA import swallowed every store error and EnsureMaterial discarded the process error; a browser warning did not prove that CA generation was missing.
- Replaced shell/PEM import with native Windows certificate-store operations on DER, exact thumbprint lookup and post-import verification. Read-only lookup permits reuse of already trusted machine CA without elevation. Interactive fallback is CurrentUser; noninteractive service execution requires LocalMachine trust.
- EnsureMaterialWithLogger sends trust success/failure to reader logs; trust failure keeps HTTPS available and does not rotate a valid CA. Tests do not mutate system trust stores. SIUI README documents importing the existing public local root with certutil from an elevated terminal, distinct from the CNAS token certificate.
- Follow-up: user tests on macOS before Windows deployment (Athena token; macOS CNAS driver support is not requested now). Added macOS Keychain verification/import with OS authorization and timeout; Linux supports root installation through update-ca-certificates/update-ca-trust, otherwise actionable log. HTTPS CA generation remains shared across OSes.
- CNAS licence is now a masked write-only settings field stored in modules.siui.licence inside config.yaml (Unix 0600 on save). No separate licence file. Empty/omitted field preserves saved secret; GET exposes only licence_configured. Settings can be saved with no token selected on Mac; submissions require complete configuration. Old licence_file is removed on settings save; re-enter existing licence in UI.
- Requested licence visibility: settings GET/PUT expose a redacted licence_hint (first 3 + … + last 3 characters; <=6 entirely masked). UI displays the hint separately from the empty replacement field so it cannot overwrite the real licence. User's running macOS entrypoint was output/siui-bridge/bin/SIUI_Bridge; refresh that executable too when rebuilding, preserving deployments/config.yaml.

### SIUI passive checks (2026-10-03)
- User requested useful testing without service validation: added POST /api/siui/ocsp-test, /insured (cnp/date), /personalization (start/stop, PARA, admin). All use the shared authenticated HTTP/WSM api.request handler. Actual calls require Windows token; every insured/personalization call automatically obtains a fresh OCSP session before SOAP. Session headers never returned or persisted.
- Insured WSDL is SOAP RPC encoded getInsured(pid,requestDate); GetInsuredResponse.xsd validates the response. States -1/0/1/2/3 are error/nonexistent/insured/uninsured/deceased; errors never map to uninsured. No CNP history persistence.
- Personalization WSDL is SiuiWS document/literal getProviderInfo(PARA,start,stop,uic). CUI comes from CUI_CAS-CODE username. Fetch returned URL only on configured HTTPS origin, no redirect; return ZIP/XML as base64 for browser download through WSM. Original bytes retained, no disk extraction. Download and uncompressed XML are bounded at 1 MiB; root must be PIAS provider, not a claim of full personalization XSD validation.
- Calls serialize against validation/token changes, timeout25s, no auto retry. UI has three separate passive actions; validation form is collapsed by default. Explicit local test uses fixed fictitious insured fixture and no native backend/configuration; simulated:true/cnas_contacted:false/sample_result prevents confusion with actual lookup.
- Tests cover TLS upstream + OCSP ordering, no validation endpoint calls, authentication failure stops SOAP/download, schema/states/patient matching, same-origin URL enforcement, archive validation, WSM admin permission and local no-network fixture. Browser smoke verifies separate buttons and file download.

### SIUI update integration correction
- SIUI already used runner/local-http updates but its install template disabled enabled/auto_download and left base_url empty. Aligned template with utilities: enabled=true, auto_download=true, stable, app_id=siui-bridge, base_url=http://127.0.0.1:19090. Keep custom operator URLs/settings intact; configure real update host through common settings.
- SIUI navigation now calls reader settings "Server și actualizări"; CNAS panel has direct settings and forced update-check buttons using the common app-update/status API (also through WSM). Uses WiseMED API key, never CNAS licence. Added mandatory utility-update requirement to readersv3/AGENTS.md.
- User reported WinHTTP12175 on Windows interactive run, old bridge used 443 (some setups444). Direct TLS1.2 probe www.siui.ro:443 at 2026-10-02 23:05 UTC found *.siui.ro certSIGN Web CA leaf expired 2026-04-09 08:20:01 UTC. Old C# CertPolicy/ServerCertificateBypass always accepts server certs. No verification bypass added.
- Added single global WinHTTP secure-failure callback, per-request context IDs/atomic flags, and specific safe diagnostics for12175. No PIN/licence/OCSP header exposure. UI exposes Port SIUI, persisting in base_url; default443 maintained. Fixed installed local SIUI app-updates enabled/auto_download to true, preserving URL/config secrets; source defaults aligned to other utilities.

## SIUI Bridge GENERAL / CLINICE (2026-10-05)

- CNAS settings are separate from Debug: fiscal_code + CAS catalogue, masked licence, Windows certificate/store and certificate-specific SIUI/PE origins. Legacy username/config migrates; licence and selected certificate are not exported.
- Bundled catalog contains the 29 requested SOAP operations from four WSDLs; excluded validateEnlisted, validatePharmacyDrugs, processIssuedPrescription. GENERAL OCSP additionally reveals a fresh token to admins on explicit request, never persists it.
- `/api/siui/general/<operation>` and `/clinice/<operation>` enqueue durable owner-scoped executions. `/calls/<id>` polls; operation_id deduplicates including configuration. Unknown writes/restarts are never replayed. CLIN/RQ_CLIN_SRV and PARA/RQ_PARA_SRV supported by new validateReport; legacy validations route remains PARA.
- Direct HTTPS accepts short-lived HS256 JWT from `/api/siui/auth/token` after shared browser login or verified WSM identity. Random process keys; SIUI browser sessions also use a private random process key, unlike legacy reader behavior. Restart requires login and new JWT. WSM uses the same handlers, preserves tenant ownership, and never logs medical payloads.
- Downloaded files (64 MiB cap) are stored next to the jobs DB and exposed as owner-scoped 256 KiB JSON/base64 chunks for WSM. No automatic retention cleanup yet. SOAP XML remains limited to 1 MiB.
- Full reference: readersv3/apps/siui-bridge/docs/API.md; packaged Help mirrors it. `/api/siui/openapi.json` is generated from the bundled catalog.
- Tests: modules/siui/operations_test.go, modules/localhttp/siui_session_test.go plus existing suites. No real CNAS writes tested; Windows token verification remains operator-side.

### SIUI Bridge macOS token support (2026-10-09)
Native Security.framework/CryptoTokenKit via purego (CGO not required), macOS 12+, CurrentUser identities and crypto.Signer using SecKeyCreateSignature. No CER/private-key/PIN persistence. Native TLS trust validates hostname/chain; optional SecTrust date-only exception. Linux remains local/simulated-only. Hardware enumeration and native ephemeral signing/trust tests run on Mac; actual CNAS authentication requires operator PIN/OCSP test.

### SIUI invoice test dashboard (2026-10-12)
Debug → FACTURĂ ELECTRONICĂ; all ElectronicInvoice.xsd attributes and 14 categories, decimal-string math, local XSD preview and ZIP binary DataMatrix (not QR). Native token CMS → ZIP → Base64; explicit sendEInvoice and getEInvoice/getEInvoiceId/getEInvoiceClearingNote, cancel API. Authenticated invoice/model, preview, package endpoints share JWT/WSM guard. getEInvoice 3-value download supported; getEInvoiceId second-string error invalidates ID. Unit tests independently decode DataMatrix, verify synthetic CMS with OpenSSL; no real invoice sent. Formular 13 numbering not established from supplied docs; do not invent waiting prerequisites.


### SIUI invoice debug refresh (2026-10-13)
Native sessionHeaders re-enumerates certificates using the Settings listCertificates path before every OCSP authentication (macOS USB reconnect). Invoice send first runs general/ocsp and displays its token in a separate transient debug area before CMS; failure prevents signing/send. SOAP still acquires its own session, so the displayed preflight token is not claimed to be its token. No caching or automatic retry introduced. invoice/model adds the supplied 34-row InvoiceItems snapshot, retaining code+contract variants; debug filters category/contract/date and starts with user-provided HIT MED HIT24/0337 PARACLLAB September 2026 invoice. API.md and packaged help document this. No actual invoice transmitted by tests.


### SIUI OCSP logger / eFactura origin (2026-10-13)
All production client instances wire runtime Logf into sessionHeaders: successful token acquisition logged before SOAP with operation/call ID, failures marked [ERROR] including HTTP status; no credential values logged. UI test errors/unknown outcomes use red alert styling, console [ERROR] records are red on TTY with -showlog (NO_COLOR respected), file logs stay plain. HTTP471 meaning unconfirmed in PIAS3.7.32. Settings adds einvoice_base_url and separate port UI per certificate; defaults to SIUI BaseURL when absent, persists independently. SiuiEInvoiceWS requests and downloads use invoice origin; OCSP still uses SIUI origin. Tests cover logger success/failure ordering, color routing, independent endpoint selection and settings restart persistence.

OCSP debug now supports admin GET general/ocsp for safe effective connection metadata, no network call; POST remains {} and includes metadata on success. UI shows parameters before authentication and retains them on failure. Runtime logs connection metadata before each attempt without Basic credentials/licence. Invoice provider fields are contextual only; user clarified partnerCategory=PARA was the getCatalogues selector, not OCSP. Do not add category to OCSP query or derive/override configured username from invoice fields.

CNAS connection errors now carry server URL/path + effective port above the message. Preserve the innermost OCSP/ SOAP/download target through nested errors; never substitute the invoice endpoint for failed OCSP. Query strings omitted from added diagnostics. Local preconnection validation explicitly says no connection initiated. UI shows error multiline above job/call JSON.

Invoice withBusy now uses shared setButtonLoading and mirrors status next to the send button; six optional jsdom regression cases verify loader + visible terminal outcome and no duplicate sends. Explicit OCSP routes timeout increased from25s to120s (native transport budget), debug exposes timeout_seconds and logs actual elapsed/deadline budget. This does not establish cause of intermittent upstream delays.

OCSP policy updated by explicit user request: reuse in-memory tokens up to150s across clients via Module.ocspSession mutex; cfg hash binds CUI_CODCAS, credentials, cert/store and endpoints/TLS. Reacquire when older or cfg differs; discard stale cache before acquisition. General OCSP and preflight no longer force renewal. Logs expose token hash ID+age; explicit admin debug displays token+reused+age; operation result has safe ocsp metadata. This supersedes earlier notes requiring a fresh session per call. New-token acquisition timeout remains120s.

Typed net.OpError Op=dial before SOAP is classified failed (submitted=false), preserving unknown for write/read/general deadlines. No textual error matching or auto retry; existing historical statuses untouched. TCP reachability probes are not proof a SOAP endpoint works on another port.

macOS transport now uses dialCNAS: max3 TCP dials,10s each,500ms gap, only typed dial timeouts retried before TLS/HTTP. No HTTP replay or TLS/read/write retry. Runtime logger attached to nativeBackend shows TCP attempts and TLS phase timings. Config TLS exceptions and port443 untouched.

- SIUI Bridge: enumerarea certificatelor pornește automat în Module.Start și la mountDashboard după login/restaurare sesiune prin POST /api/siui/certificates/refresh (autentificat, fără date certificate). Folosește magazinul configurat, păstrează profilul și nu contactează OCSP; erorile sunt non-fatale și logate. Setările CNAS încarcă lista la deschidere.

- getEInvoiceId: formatul cu două valori este [ID, mesaj]; formatul cu trei valori are mesajul pe poziția 3, iar poziția 2 poate conține un identificator numeric. Nu interpreta toate valorile după prima ca erori. Păstrează ambele valori de date în result.values dacă mesajul final e gol; mesajul de nevalidare de pe ultima poziție produce failed. Utilizatorul a confirmat că în formatul cu trei valori a doua valoare este BT-13 (exemplu 1516712462512); prima este numărul înregistrării SIUI, iar a treia este mesajul.

- Studiu PIN token multiplatformă documentat în readersv3/apps/siui-bridge/docs/API.md, secțiunea „Studiu: PIN prin driver”. Neimplementat. Preferă CSP/KSP Windows și LAContext smartCardPIN + kSecUseAuthenticationContext macOS față de automatizarea ferestrelor. Evenimentele de prezență nu sunt cereri PIN. După o încercare respinsă/incertă, suspendă automatizarea până la intervenția utilizatorului, fără retry pe alt backend. Teste obligatorii cu middleware real înainte de a promite protecția contra blocării.

- Implementare PIN multiplatformă SIUI: token_pin*.go, token_secret*.go și token_transport.go. Stare separată per certificat în siui-token-pins.json; DPAPI Windows, AES-GCM + cheie 0600 pe Unix. API /api/siui/token-pin admin-only, numai status <hidden>; PIN niciodată returnat. Suspendare persistentă înainte de predarea PIN către driver; reactivare numai după semnătură verificată. Manual temporar 150 secunde, eliminat la eșec, nu reactivează PIN-ul salvat. macOS LAContext smartCardPIN + identity query authentication context; Windows signer CSP/KSP silent (fără cache partajat), fallback clasic la următoarea operație; Linux PKCS#11 bibliotecă aleasă numai din config local, RSA/ECDSA TLS/CMS, fără CKA_ALWAYS_AUTHENTICATE. SoftHSM Linux: teste reale semnare/mTLS/CMS și PIN incorect → manual trecute. Windows compilat; tokenurile fizice Win/Mac încă necesită verificare. Studiul anterior a fost înlocuit de implementare în API.md.

## Unified WSM permission order (2026-10-16)

Audited all 11 ValidScope values against admin and shared reader/utility UI.
Added wisemed:proxy as admin permission 11; topics:subscribe remains 10.
Shared localhttp UI lists all permissions in admin order with visible 1–11 labels;
checkbox values and existing selections/defaults remain unchanged. Cross-module
Node regression compares UI order to the admin catalog and catalog completeness
to ValidScope. Shared embedded UI applies to all WSM-enabled readers/utilities
when rebuilt; remote control assets must also be refreshed when packaging WSM.
No installed binaries, runtime permissions or secrets changed in this update.

## Consistent permission text (2026-10-16)

Preserved admin permission descriptions as the common titles for all 11 rights.
Shared reader/utility checkbox labels now use these exact titles. Admin catalog
adds info and example fields copied from reader help; rights prints both below
each numbered title. Cross-module UI regression verifies all four fields match.
Requires rebuilding installed applications and refreshing central control assets.

## SIUIBridge browser wrapper (2026-10-16)

WSM embeds /sdk/siui-bridge.js, a wrapper over an existing WSMServer (no extra
socket/fetch). SIUIBridge.create returns null for no matching online bridge,
rejects ambiguity, and pins equipment/client identity across discovery refreshes.
All 34 WSDL-derived operations have grouped and flat methods; 23 auxiliary
method/route combinations cover settings, certificates, PIN, OCSP, invoices,
legacy validations, token issuance, status/catalog/OpenAPI and call/file reads.
Operations require stable operationId, submit once and poll by default. Explicit
wait:false, waitCall/waitValidation and chunked downloads supported. AbortSignal
abandons only local waiting; never claims to cancel remote CNAS work. Failed and
unknown remain errors; completed does not imply insured/business success.

Docs: wsm-server/docs/siui-browser-wrapper.md. Tests compare SOAP catalog and
explicit handler routes, exercise all methods, polling/abort/deadline/identity and
file chunk behavior. Existing TLS WSS wire test now exercises wrapper discovery
and status through a simulated SIUI peer. No real CNAS or physical token calls.

Shared apibridge now permits 130s for POST /api/siui/general/ocsp, /ocsp-test and
/invoice/package (previous generic30s undercut OCSP120s); other deadlines stay
unchanged and parent cancellation remains effective. SIUI binary rebuild needed
for these long operations. WSM build refreshed at output/wsm-browser-sdk; active
installations not replaced. Validation: 18 SDK Node tests, WSM race suite/vet,
reader apibridge + ws race tests passed.

## Consistent WSM naming (2026-10-17)

Canonical browser API is WSMServer, example instance objWSM, asset /sdk/wsm.js.
PHP helper is examples/browser/wsm-token.php and function wsmBrowserToken.
SIUIBridge consumes WSMServer; SDK tests/imports, embedded route and all technical
examples/docs use these names. Shared reader/utility diagnostics use WSM labels,
DOM IDs and /debug/wsm. Integrating pages must use the new names and asset path.
Staged output/wsm-browser-sdk rebuilt; active installations need normal updates.
Go WSM suite and reader localhttp tests passed, 31 SDK/shared UI Node tests passed;
optional DOM suite skipped because jsdom is unavailable. PHP syntax check passed.

### Prescription bridge coverage (2026-10-22)
The catalogue now exposes 40 SOAP operations, including generatePrescriptionSeries/getPrescriptionSeriesInfo (GENERAL, document/literal) and all 13 EPrescriptionWS operations: added processIssuedPrescription, getReleasedPrescription, cancelReleasedPrescription, updateInvoices. This supersedes the earlier processIssuedPrescription exclusion; validateEnlisted/validatePharmacyDrugs remain excluded. XML boolean parameters accept only true/false/1/0 strings; Debug and OpenAPI reflect that contract. Carnet responses preserve every raw return string, with no inferred structure or physician mapping. processIssuedPrescription/updateInvoices have syntax-only XML validation (no supplied XSD). Offline issuance/number allocation/storage/printing remain WiseMED responsibilities; CNAS operations require connectivity. See the prescription section in WSM wrapper docs and examples/browser/siui-prescriptions.js for aceOverWatch callbacks and stable generation IDs. No live CNAS writes or installed-service restart performed.
Local verification for this change: go test -race ./modules/siui ./modules/localhttp passed in readersv3; go test ./internal/server passed in WSM; both apps built successfully. SDK/example Node suites: 25 pass, 1 existing auxiliary-route coverage failure (missing dashboard in wrapper); SOAP catalogue parity and all new prescription/example cases pass. SIUI UI suite: 15 pass. Refreshed output/wsm-browser-sdk/bin/wsm-server and prescription docs/example; SIUI binary built to /tmp/wisemed-prescriptions-siui-bridge for verification, installed binaries were not replaced.

### Prescription null parameter and download correction (2026-10-22)
Corrected both SDK and backend for JSON null subUnitCode on generatePrescriptionSeries/getPrescriptionSeriesInfo only, using nullable catalogue metadata and SOAP xsi:nil=true. Required/unknown fields still reject null; null and empty-string payloads have distinct deduplication digests. The HTTP boundary uses string pointers to preserve null rather than silently converting it to empty. Debug blank subUnitCode sends null; OpenAPI declares nullable. Both catalogue operations now download the CNAS URL/size result through the existing HTTPS-origin-restricted download pipeline; values are URL+size, not individual carnets. This supersedes the prior raw-carnet-response assumption. SDK returns downloaded bytes in data.download.data by default. Reference: official PIAS specification §5.20 (portal.cnas.ro, v3.7.34 dated 2026-10-14). Backend race tests, including null-through-WSM-to-SOAP/file download and null/empty dedup isolation, passed; selected SDK/example suite 26 passed, excluding the previously known unrelated dashboard-route coverage failure; UI suite 15 passed. No CNAS operations invoked in verification.
Debug → CLINICE also lists getPrescriptionSeriesInfo and generatePrescriptionSeries; their operation group and general/* routes remain unchanged for SDK compatibility.
Refreshed local WSM output/bin/WSM_Server, SDK package binary/control/docs, and both local SIUI Bridge output binary paths after the fix. No running service was restarted and no real CNAS call was made.


## SIUI Docker deployment on EC2 (2026-10-27)

Root make-siui-bridge-docker.sh, make-siui-bridge-history-docker.sh and
make-wisemed-siui-data-docker.sh delegate to readersv3/docker/build.sh. Default
linux/amd64 built natively/cross-compiled on Apple Silicon, UTC timestamp in
archive/tag/appmeta.Version. Exports ~/dockers/<app>-YYYYMMDD-HHMMSS.tar.gz and
standalone update-<app>.sh. Updater also lives in readersv3/output/<app>/, refreshed
by prepare-docker-output.sh and prepare-reader-output.sh. Preserve local configs.

Shared Dockerfile uses Debian slim, CA/tzdata/xmllint/OpenSC/PCSC, UID/GID65532,
read-only root, writable deployments/data and tmpfs /tmp. Host networking on EC2
keeps loopback MySQL/PostgreSQL/WSM reachable; app UIs retain ports19112/19113/19114.
Defaults are /opt/<app>/deployments/config.yaml and /opt/<app>/data/ (SQLite,
history/staging, uploads); container mounts these at /data/deployments and
/data/data. Logs and generated local TLS stay in deployments. Existing operator
paths/config values are preserved by the common config merge. Databases remain
external services; no DB data files packaged. See readersv3/docker/README.md.

Update scripts reuse/start an SSH agent, unlock ~/.ssh/github_dockers, clone
AlwaysSmarter/dockers, select tracked per-app timestamped archives, verify gzip,
arch/app/version/config, prepare permissions and replace with health/rollback.
--setup runs the existing wizard interactively before persistent installation.
Host application root belongs to invoking user; deployments/data to65532.
Optional private docker.env and allowlisted mount/device/group docker.args are
read by host updater and are not mounted into the app. Health uses /api/session
protocol+embedded version and persistent local CA, without disabling TLS checks.
Rollback restores container settings, not mutable data/schema/config migrations.
Native updater settings/check/download stay intact; WISEMED_CONTAINER=1 prevents
auto-applying native archives to an immutable container binary.

Local verification: all three real AMD64 Docker images built/exported and started
with synthetic configurations, isolated networking and temporary persistent
mounts; version/UID/help/config/data paths and verified HTTPS health passed.
8 offline orchestration tests passed (all apps, agent lifecycle, rollback, first
install cleanup, preflight errors, same-version no-op, setup). Go race tests for
tools/docker-runtime and apps/runner passed. No EC2 installation, Git push,
production DB/CNAS/USB-token operations performed.

## CNAS PIAS 3.7.34 / DocMED origins (2026-11-02)

Audited docs/CNAS current PDFs/WSDLs against portal.cnas.ro/cnas/pias/specificatii and /docmed. PIAS and DocMED docs dated 20.10.2026. Production SIUI/OCSP/eFactura now default to https://siui.e-pias.ro; SIPE to https://sipe.e-pias.ro; new official origins use HTTPS/443. Legacy www/sipe/testsiui/testsipe.siui.ro still accepted on 443/444; locally configured custom origins/ports retained. The current docs remove issuer-specific AlfaSign/CertDigital 444 guidance but do not prove all old listeners disabled. CEAS remains eCard.SDK TCP umceas/testumceas.siui.ro:443; no CEAS transport in this bridge.

Added docmed_base_url (default https://docmed.e-pias.ro), per-certificate profile persistence, GET/PUT settings, bootstrap/UI/template and docmed_endpoints response. Dashboard lists DocMedService /loader/ws, DocumentSeries /seq/ws, MedicalDocumentDownloader /download/ws, DocStatusChangerService /status/ws. DocMED SOAP operations are not implemented; its document/literal contracts are distinct. WSDL comparison found no removed existing SIUI/SIPE operations, so no automatic rerouting of old forms into DocMED. Added SIPE clinice.getPrescriptionsSeriesNo(requestXml), a read of prescription history by series/number (not number allocation), with request XSD and result schema; catalogue/SDK now 41 operations. Updated embedded WSDLs from local current docs. Save errors now appear in form and identify the rejected origin field. docs/cnas-endpoints-2026-10.md records findings, scope and sources; docs/CNAS/Docs.txt remains an outdated April snapshot.

Verified Go race tests for modules/siui, modules/localhttp, apps/runner; native XSD positive/negative schema test for new SIPE operation; 43 UI/SDK tests and WSM server tests passed. Local runtime config production legacy origins without special ports migrated with a 0600 backup; explicit custom/testing/special ports preserved. Local SIUI and WSM executables/static bundles refreshed. No process restarted or live CNAS authentication/medical operation performed.

## DocMED complete bridge integration (2026-11-02)

Supersedes the configuration-only DocMED note above. All 22 methods from the four official DocMED WSDLs are implemented on separate POST /api/siui/docmed/<ExactName> routes, using parameters.requestXml with the complete Request element. Catalog now combines 41 legacy SIUI/SIPE and 22 DocMED operations, with request/response elements/schemas/path and synthetic examples in docmed_operations.json / docmed_examples.json. DocMED uses document/literal SOAP, its configured origin, and shared SIUI OCSP/token/certificate. No legacy route is redirected. Debug has a DOCMED tab defaulting to the nonmutating GetMedicalDocumentXml, and the WSM safe/direct SDKs have a docmed group.

Full request/response XSD validation uses separate embedded schemas/docmed. Business errors are failed and preserve structured codes/messages and the XML response file; series W/I/no-level messages are nonblocking, E blocks; all other methods reject any message. ConsumeDocument preflights GetDocumentStatus=FINALIZED, UnconsumeDocument requires CONSUMED. Ambiguous writes remain unknown with no replay. Replies use chunked XML files and bounded summaries; GetMedicalDocument ZIP is exposed as a projection of the same XML file with docmed_attachment=true (no duplicate file). SDK downloadDocMedAttachment preserves this query on every offset. Original responses are included in existing History flow. XML request/response cap 32 MiB, old SOAP cap stays 1 MiB; WSM adapter's smaller request cap remains.

Official local XSDs GetDocumentStatusRequest, ConsumeDocumentRequest, UnconsumeDocumentRequest reference undefined fpidType. Only embedded copies remove this undefined union branch, retaining documented numeric cidType. Originals in docs/CNAS untouched; foreign identifier branch remains unspecified and unsupported until CNAS supplies its definition. This is documented in apps/siui-bridge/docs/DocMED.md.

There is no list-all-documents operation. GetMedicalDocumentXml/GetMedicalDocument require CID, issueDate, serialCode, serialNo; GetDocumentStatus has optional CID. GetDocumentSeries lists number ranges, not medical documents. Finding a known old document proves its availability, while DOC_NOT_FOUND alone does not establish universal migration/absence.

Verified Go race tests modules/siui, modules/localhttp, apps/runner; native XSD and all 22 contract tests; business-failure persistence, tenant ownership, deduplication, state preflight/no retry, inherited namespaces, >1 MiB response, multi-chunk attachments. 45 combined UI/DOM/SDK tests passed, with jsdom installed only under /tmp for DOM tests. WSM server tests passed; macOS and Windows amd64 executables built. Local SIUI/WSM binaries and control/help/SDK docs refreshed with backups. No running debugger/service restarted and no live CNAS operations performed. See docs/DocMED.md for read-test steps and complete routes.

## DocMED variable forms and dropdowns (2026-11-03)

Supersedes the XML-entry Debug UI described above. All 22 DocMED operations now expose generated XSD form models (`assets/docmed_forms.json`, regenerated by scripts/generate-docmed-forms.py). Debug uses docmed-forms.js to collect ordinary variables, optional sections/repeated rows and PDF/ZIP uploads. XSD enums, document types and booleans are labelled dropdowns; CAS fields use the embedded insurance-house list. Dates use date controls. Fiscal/CAS defaults come from the authenticated catalog's docmed_defaults; common provider inputs persist only in panel memory. External nomenclature values absent from the repository are not guessed.

POST /api/siui/docmed/<Name> accepts {operation_id,fields:{...}} as an alternative to advanced {operation_id,parameters:{requestXml}}. Scalars are strings, children objects/lists, binary attachedFile accepts Base64 or {filename,data_base64}. Server builds XML in schema order with encoding/xml, validates required/unknown fields, enum/bounds/types and XSD before queueing, and archives PDF deterministically into ZIP. Existing raw XML clients remain compatible and share deduplication/ownership/admin checks. OpenAPI includes the structured fields schema and mutual exclusion. No production third-party frontend dependency added.

User's clean GetDocumentSeries request (ML, 2026-11-02 range) passes local XSD. The old generic error did not reveal whether request or response validation failed; no unsupported root-cause claim was made. Errors now identify DocMED request/response validation and schema; missing/unstartable validators are separate. Invalid received response XML is retained as a scoped file with docmed.validated=false and without trusted parsed medical data. Failed/unknown terminal DocMED calls can download that diagnostic file; ambiguous writes still are not replayed.

Verified full Go race tests for siui/localhttp/runner, all 22 generated requests including optional fields against XSD, exact GetSeries regression/escaping, PDF determinism, response-validation diagnostics. 47 UI/DOM/SDK tests passed, including dropdowns, repeated rows, uploads and switching back to legacy forms. WSM tests verify required JS modules are served while arbitrary control files remain inaccessible. macOS and Windows amd64 builds pass. Local binaries/control assets/help refreshed with dated backups; runtime config/credentials preserved; no real CNAS calls or running process restarts.

## DocMED SOAP Fault details (2026-11-03)

User reported unknown GenDocumentSeries call 151d8be70104e3ecd9ce0203d313f5dc. Read-only local audit confirmed request type PE and documentCount 2500; no response result had been persisted, so the actual remote fault reason cannot be recovered. Do not infer a CNAS business/root cause from the generic prior message and do not replay the write for diagnostics.

DocMED SOAP Faults now return typed code/reason/actor/detail/detail_xml/http_status and a redacted owner-scoped SOAPFault.xml artifact. Known licence, OCSP session and Authorization credentials are removed from both text/attributes and the stored fault XML. Fault summaries are bounded, are not marked as validated medical data, and are not written to operational logs. Writes retain unknown/no automatic replay with write_may_have_been_applied=true; reads remain failed. Existing call/file/History paths carry diagnostics without a new API route. The fault file does not advertise a DocMED success-response XSD.

Verified request/invoke and durable worker/file tests for SOAP Faults on HTTP 200/500, read/write classification, escaped/attribute credential redaction, nested detail retention and no replay; full siui race tests passed. macOS and Windows amd64 builds passed. Local SIUI executables/help updated with backups; runtime configuration, credentials and old call statuses preserved. No process restarted or live CNAS call performed. For the existing unknown generation, operator should inspect GetDocumentSeries for PE before deciding on another generation.

## DocMED GenDocumentSeries PE null: independent contract audit (2026-11-03)

User reported SOAP-ENV:Server NPE from GenDocumentSeriesRequest.GenDocumentSeries.getDocumentType(). Exact stored request for call 8531d05ff858647fdd10c2a5b8757a22 contains unqualified documentType="PE" and documentCount="2500" on qualified genDocumentSeries. Exact request validates against original docs/CNAS XSD. Re-downloaded public CNAS StructuriDocMed.zip and confirmed GenDocumentSeriesRequest, GetDocumentSeriesRequest and CommonDocumentTypesPias byte-identical to local originals; PE explicitly present. Do not remove PE or qualify its attribute to guess a workaround; qualification violates published XSD.

Built independent Java JAXB 4.0.5 classes from published XSD into ro.cnas.generated, then unmarshalled the exact stored request in its SOAP envelope: Java getter returns PE, count 2500. Also added reproducible sanitized oracle at apps/siui-bridge/tests/docmed-contract-oracle (Java 17+/Maven, public dependencies only, XML remains local), and Go regression TestDocMedPESeriesMatchesPublishedNamespaceContract including negative test for wrongly qualified attribute. Existing local GetDocumentSeries PE reads completed with zero series; this does not by itself prove no allocation. No authenticated CNAS call or replay of generation performed during audit; public live WSDL TLS handshake failed.

Evidence points to deployed CNAS binding/contract or server-side request processing discrepancy; exact internal root cause (PE support vs annotations vs another transformation) remains unconfirmed without server access. Technical report with sanitized request, source links, schema SHA-256 and proof is docs/cnas-docmed-documenttype-null-2026-11-03.md. Keep unknown write state/no retry. Production serialization is unchanged because it conforms to the published contract; no binary refresh/restart is needed solely for this audit.

### WiseMED AI (2026-11-03)
- Added utility `readersv3/apps/wisemed-ai`, module `modules/wisemedai`, protocol `wisemed-ai`, communication `utility`; native executable/output, standard runner/service/updates and separate embedded Vue clinical UI at `/ai/` (loopback 19120).
- Standalone opaque-token auth, bcrypt, installation admin separated from organization RBAC, PostgreSQL migrations/tables/indexes `wai_`, AES-GCM clinical/connector/memory payloads. Local PostgreSQL connection imported from SIUI Data only into ignored/protected runtime config; do not copy credentials into sources/docs/releases.
- 52 synthetic starter templates / 46 specialties, idempotent catalog, immutable publication, TipTap placeholders/sections and clinician-confirmed proposals. Versioned transcript, evidence validation, exact-revision approval, no automatic HIS persistence; optional allowlisted HTTPS outbox.
- Shared WSM `api.request` alias `/api/wisemed-ai/v1/` requires explicit attested subject/tenant membership mapping and enabled integration. Added targeted non-PHI event publishing to the shared WS client; no scope bypass or broadcast. Runtime/AI/API/WSM/update settings also exposed through installation-admin AI forms.
- OpenAI is optional and defaults to mock; strict schema, usage and conservative durable reservations for uncertain billing. Standalone works without API keys/WSM. Patient briefing returns unavailable authorized history unless a verified HIS adapter is supplied; do not fabricate patient authorization endpoints.
- PostgreSQL integration/race/RBAC/WSM bridge tests, full readersv3 Go tests/vet, Vue build/Vitest/Chromium desktop/mobile, Dart analysis/tests, OpenAPI validator and Docker app+pgvector passed locally. Native builds for Mac arm64/Linux amd64/Windows amd64. Native Mac NSIS self-test fails std::bad_alloc; `apps/wisemed-ai/docker/build-windows-release.sh` successfully generates the real Setup using Linux NSIS. See app README and docs/VALIDATION.md for actual verification boundaries.
- `releasectl sync-app wisemed-ai` refreshes only this app's scaffolding. Keep `app_id: wisemed-ai`, stable/default updates, existing config and secret files. Do not install the service or claim deployed WiseMED/HIS acceptance solely from local packaging.


### WiseMED AI packages 1.1.0 (2026-11-03)
- User explicitly requested package-definition phase first; monthly subscriptions/invoices/top-up/credit spending/EuPlătesc follow later using documentation they will supply. Do not invent payment endpoints or publish releases to update server; artifacts remain local.
- Dashboard roles: verified instance super admin user_type=-1; organization admin=0; normal=1. Super manages catalog/global user list, roles and explicit audited administrative assignments; org admins cannot grant global privileges.
- Additive migration007 uses only wai_ objects: immutable versioned package terms, pinned user assignments, active specialties, template ownership and usage attribution. Default Free1specialty/3own templates/50k monthly tokens; Pro2/25/500k and Premium10/100/2M are inactive drafts until priced. Prices/extras/credit use integer minor units and configurable currency; no actual debt or charge yet.
- Backend enforces per-user global specialty/template/field/token limits. Shared catalog does not consume own-template quota; edits by normal users clone shared templates. Accounting reserves independently of clinical transactions, serializes user reservations and reconciles exact provider input/output usage; uncertainty remains reserved. Never take the billing-user advisory lock in the clinical chunk transaction before the independent accounting call, or it self-deadlocks. Read-only templateUseActive is intentional.
- BillingPanel exposes package definitions, users/roles/assignment, own allowance and usage ledger. Full Go/vet, PostgreSQL race/concurrency/idempotence and frontend checks pass; actual delivery boundaries recorded in apps/wisemed-ai/docs/MONETIZATION.md and VALIDATION.md.


### WiseMED AI subscriptions/context 1.2.0 (2026-11-03)
- User extended scope: actual monthly renewal, five-day payment grace, financial suspension, per-package AI overage credit, exact per-generation/template statistics. Then clarified persistent context for each template rather than mandatory search, and asked whether templates can act as skills. Explained skills load instructions into token context; external storage is not free model context. No actual OpenAI-hosted Skills API integration was requested/claimed.
- Additive migrations008–010: subscriptions, monthly UTC periods, internal payment statements, uniquely referenced verified manual payments, usage-period attribution/context counters, encrypted per-template-version context. Paid activation explicit, first month full/no proration, renews first of month UTC. Existing manual assignments do not become subscriptions. Terms pinned; active subscription assignment cannot be overwritten through manual grants.
- Five days from issue then commercial402, without changing administrative disabled/roles. Login/profile/billing remain available for recovery. Credit includes unpaid AI usage and accrued usage, with uncertain/concurrent reservations across old periods. Cumulative integer ceil rate/1M calculation; no financial float. Super admin verifies external payments, no self-confirmation, partial/idempotent/no overpayment. EuPlatesc/live cards/fiscal invoices/cancellation/plan switching not implemented; docs/SUBSCRIPTIONS.md records boundaries.
- Template editor ai_context fields instructions/terminology/examples stored AES-GCM in wai_template_contexts, outside plaintext template JSON, pinned to immutable version. Extraction loads exact context; no patient memory or automatic clinical values. Compact definitions retain all fields/text, stable prefix, bounded local cache. Local colon-form proposals need no provider call; free text receives up to12 authorized matching definitions. This is not an external semantic RAG. Generation receipts/actual/cache usage and context-byte statistics exposed through billing APIs/UI.
- Full Go/vet, complete PostgreSQL race (including concurrent credit, grace boundary and encrypted context), isolated browser payment recovery, native clinical/editor/mobile, Vue/Vitest and OpenAPI passed. Keep runtime config/secrets untouched, native local app running, releases local only. No publication to update server and no real paid API/payment calls.


### WiseMED AI generic templates/mobile1.3 (2026-11-03, în lucru)
- User clarified sections/subsections must be generic (organs, cardiac components, history/symptoms/assessment/recommendations), with brief/standard/detailed wording and per-section overrides; explicitly requested adapting all52 examples. Implemented presentation engine/designer, evidence-gated equals/documented variants, safe fallback/ambiguity, immutable revision style changes and approved locks. No disease inference from silence. All52 live catalog examples are model_version2; old versions and user publications preserved. Clinical examples are starters, not validated disease protocols.
- User explicitly requested separate agent for complete Flutter app in /Users/raduichim/work/flutter/wisemed-ai; read WisemedAIFlutter.md, initialized Flutter and core API/encrypted-draft/STT modules, then delegated Flutter repo ownership to /root/flutter_app. Backend parent continues here. Do not edit same Flutter files concurrently; use collaboration messages. Added Session.template pinned snapshot and optional work_context to POST/sessions.
- User requested durable resume state. Current authoritative checkpoint is readersv3/apps/wisemed-ai/docs/WORK_STATUS.md; agent asked to maintain Flutter checkpoint too. Update real remaining statuses before concluding. Backend1.3 native running19120, releases local only. Flutter completion/tests/builds remain outstanding until agent final and review.


### Finalizare surse/backend/mobile1.3 (2026-11-03)
- Backend1.3 local pornit19120, toate52 exemple live model2, UI clinic mixt Scurt general/Ficat Detaliat trecut cu aprobare și mobil. Release-uri macOS/Linux/Windows și instalatoare locale verificate fără secrete/publicare. Checkpointul WORK_STATUS.md conține rezultatele finale și resume.
- Agentul Flutter a finalizat proiectul separat: analyzer curat,18 teste trecute, test liveGo1.3 și smokeChromium same-origin autentificat trecute. Web demo/producție și dist/wisemed-ai-web-1.0.0.zip generate; capturi mobile/workspace inspectate de părinte. Păstrează sursele și vezi README/docs/VALIDATION.md.
- Native mobile/desktop artifacts NOT built successfully: disk exhausted atAndroid extraction; iOS18.2 platform missing; macOS deployment fixed11 but no final rebuild due disk. Android native source versions aligned to deps. Niciun APK/IPA/macOS final nu este declarat; operator must provide disk/SDK/device then native builds/physical QA. Numai intermediarii proprii au fost curățați; fișierele utilizatorului, sursele și livrabilele finale păstrate. No licenses/signing/publication performed.
