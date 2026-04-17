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
