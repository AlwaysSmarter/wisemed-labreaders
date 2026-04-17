# wisemed-labreaders memory

## Workspace

- Root workspace: `/Users/raduichim/work/gowork/wisemed-labreaders`
- Main active reader project in this thread:
  - `/Users/raduichim/work/gowork/wisemed-labreaders/Readers Last/generic-test-reader`

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
- sidebar made sticky with viewport-height behavior
- sidebar now includes:
  - visual identity card
  - reader summary card

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

Important orders API behavior:

- `GET /api/orders` now supports `include_analysis`
- when `include_analysis=1`, response `orders` is `OrderBundle[]`
- when omitted or `0`, response `orders` is plain `Order[]`
- the current web UI uses `include_analysis=1`

Important config/statistics API behavior:

- `GET /api/config` returns the full runtime config snapshot
- `PUT /api/config` applies partial top-level config patches and persists them to YAML
- `GET /api/config/<section>` and `PUT /api/config/<section>` work on one config section at a time
- `GET /api/stats` aggregates the statistics shown in the web dashboard:
  - total entity counters
  - today with/without result
  - dashboard time series
  - connection flags
  - reader identity summary

Important analytes API behavior:

- analyte creation is separate from analyte update
- analyte update is now by `id`, not by `tag`
- analyte delete is now by `id`, not by `tag`
- `tag` must stay unique
- changing a tag on edit updates the same analyte row, it does not create a new one

## Commands/events

Reader WS commands from README:

- `reader.status`
- `stats.get`
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

## Order analysis API notes

The reader now exposes explicit CRUD for `order_analyses`, separate from `orders`.

Practical usage for clients:

- list orders first
- if the client only needs order headers, use `orders.list` / `GET /api/orders` without `include_analysis`
- if the client needs the full order tree in one round-trip, use `include_analysis=1`
- if the client needs analysis-only operations, use:
  - `order_analysis.list` or `GET /api/order-analysis?order_id=...`
  - `order_analysis.get` or `GET /api/order-analysis/<id>`
  - `order_analysis.create`
  - `order_analysis.update`
  - `order_analysis.delete`

Current WS response shapes:

- `orders.list` without `include_analysis`:
  - `{ orders: Order[] }`
- `orders.list` with `include_analysis=true`:
  - `{ orders: OrderBundle[] }`
- `order_analysis.list`:
  - `{ order_analyses: OrderAnalysis[] }`
- `order_analysis.get`:
  - `{ order_analysis: OrderAnalysis }`
- `order_analysis.create` / `order_analysis.update`:
  - `{ order_analysis: OrderAnalysis }`
- `order_analysis.delete`:
  - `{ deleted: <id> }`

Analytes write semantics:

- `analytes.create`:
  - creates a new analyte
  - fails if `tag` already exists
- `analytes.update`:
  - updates by `id`
  - tag may be changed
  - fails if the new tag is already used by another analyte
- `analytes.delete`:
  - deletes by `id`

## Config and stats API notes

The reader now exposes both HTTP and WS configuration/statistics APIs intended for client apps and AI-driven tooling.

HTTP:

- `GET /api/config`
- `PUT /api/config`
- `GET /api/config/<section>`
- `PUT /api/config/<section>`
- `GET /api/stats`

WS:

- `config.get`
- `config.set`
- `stats.get`

Supported config sections:

- `wisemed_api`
- `wisemed_ws`
- `local_http`
- `reader`
- `communication`
- `layout`
- `capabilities`

Practical client guidance:

- use section endpoints when editing a specific form/page
- use `GET /api/config` when building a full configuration screen
- use `GET /api/stats` when the client needs one aggregated payload for dashboard counters + chart + connection state
- `comm.get` / `comm.set` still exist for backward compatibility, but the broader config API should be preferred for new client work

## Localization notes

UI localization is dictionary-based in `internal/webui/ui/app.js`.

Important expectations from thread:

- Romanian should be the default/fallback language
- all visible control labels in Orders should be translated
- examples explicitly requested:
  - `Get worklist` translated
  - `Detalii order` changed to `Detalii proba`
  - orders table column headers translated

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
