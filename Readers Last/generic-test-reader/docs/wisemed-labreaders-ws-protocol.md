# WiseMED Lab Readers – WS Protocol Reference

> **Purpose:** AI context for the three-tier WebSocket architecture connecting WiseMED GUI to lab analyzers.
> **Last updated:** 2026-04-15

---

## Architecture

```
Lab Analyzer ←─ file/serial/network ──→ Reader (Go binary)
                                           │
                                           │ WS (Envelope protocol)
                                           │ JWT auth, auto-reconnect
                                           ▼
                                      wsm-server (Go relay hub)
                                           │
                                           │ WS (same Envelope protocol)
                                           │ JWT auth, topic pub/sub
                                           ▼
                                    WiseMED GUI (browser JS)
                                    layout/apps/lab/tpl/analyzers/labreaders/*
```

## Projects

| Name | Path | Language |
|------|------|----------|
| **wsserver** | `/Users/raduichim/work/gowork/wisemed-labreaders/Server Last/wsm-server/` | Go |
| **reader** | `/Users/raduichim/work/gowork/wisemed-labreaders/Readers Last/generic-test-reader/` | Go |
| **WiseMED GUI** | `/Users/raduichim/www/git/wisemed/layout/apps/lab/tpl/analyzers/labreaders/` | JS/HTML |

---

## Envelope Format (JSON on the wire)

```json
{
    "type":           "command|reply|event|hello|hello_ack|ping|pong|subscribe|error|presence",
    "request_id":     "r-<nanotime>",
    "correlation_id": "<original request_id>",
    "connection_id":  "conn-<seq>",
    "target": {
        "mode":          "reader|connection|topic|client_type|self|all",
        "connection_id": "conn-X",
        "client_type":   "reader|browser",
        "reader_id":     "reader-file-001",
        "topic":         "logs:reader-file-001"
    },
    "broadcast": false,
    "payload":   { ... },
    "timestamp": "2026-04-14T10:00:00Z"
}
```

---

## WS Connection Lifecycle

### 1. Browser connects to wsm-server
```
ws(s)://<wsm-server-host>:<port>/ws?token=<JWT>
```

### 2. Browser sends hello
```json
{ "type": "hello", "payload": {
    "client_type": "browser",
    "client_id":   "wisemed-gui-<user_id>",
    "user_id":     "<user_id>",
    "reader_id":   "",
    "label":       "WiseMED GUI"
}}
```

### 3. Server responds hello_ack
```json
{ "type": "hello_ack", "payload": {
    "connection_id": "conn-5",
    "client_type":   "browser",
    "client_id":     "wisemed-gui-42",
    "subject":       "...",
    "role":          "browser"
}}
```

### 4. Browser subscribes to reader events
```json
{ "type": "subscribe", "payload": { "topic": "logs:<reader_id>" } }
```

### 5. Browser sends commands targeting a reader
```json
{
    "type": "command",
    "request_id": "b-1713088800000-1234",
    "target": { "mode": "reader", "reader_id": "reader-file-001" },
    "payload": { "command": "orders.list", "args": { "order_date": "2026-04-14", "round_no": 1, "include_analysis": true } }
}
```

### 6. Reader responds with reply
```json
{
    "type": "reply",
    "correlation_id": "b-1713088800000-1234",
    "payload": { "success": true, "data": { "orders": [...] }, "error": "" }
}
```

---

## Reader Commands Catalog

| Command | Aliases | Args | Response data |
|---------|---------|------|---------------|
| `reader.status` | `get_status` | — | `{ reader: {...}, communication: {...}, layout: {...}, connections: {...}, stats: {...} }` |
| `stats.get` | `get_stats` | `series_limit?` | `{ stats: {...}, today: {...}, series: [...], connections: {...}, reader: {...} }` |
| `config.get` | `get_config` | `section?` | `{ config: Config }` or `{ section, config: SectionConfig }` |
| `config.set` | `set_config` | top-level config patch, or `section + data` | `{ config: Config }` or `{ section, config: SectionConfig }` |
| `orders.list` | `list_orders` | `round_no?, order_date?, include_analysis?` | `{ orders: [Order, ...] }` or `{ orders: [OrderBundle, ...] }` |
| `orders.get` | — | `id` | `{ order: Order }` |
| `orders.create` | `create_order` | `order_date, round_no?, sample_id?, file_id?, patient_id?, patient_name?, rack_no?, rack_position?, list_position?, sample_no?, status?` | `{ order: Order }` |
| `orders.update` | `update_order` | same as create | `{ order: Order }` |
| `orders.delete` | `delete_order` | `id` | `{ deleted: <id> }` |
| `order_analysis.list` | `list_order_analysis` | `order_id` | `{ order_analyses: [OrderAnalysis, ...] }` |
| `order_analysis.get` | `get_order_analysis` | `id` | `{ order_analysis: OrderAnalysis }` |
| `order_analysis.create` | `create_order_analysis` | `order_id, analyte_tag, analyte_name?, status?, analyte_id?, default_result_id?, result_value?, raw_value?, interpreted_value?, unit?, source_file?` | `{ order_analysis: OrderAnalysis }` |
| `order_analysis.update` | `update_order_analysis` | same as create plus `id` | `{ order_analysis: OrderAnalysis }` |
| `order_analysis.delete` | `delete_order_analysis` | `id` | `{ deleted: <id> }` |
| `analytes.list` | `list_analytes` | — | `{ analytes: [Analyte, ...] }` |
| `analytes.get` | — | `id` or `tag` | `{ analyte: Analyte }` |
| `analytes.create` | `upsert_analyte` | `tag, code, name, description?, result_type?, result_formatting?, result_weighting?, result_measure_unit?, result_reagents_set?, active?` | `{ id, tag }` |
| `analytes.update` | `upsert_analyte` | same as create plus `id` | `{ id, tag }` |
| `analytes.delete` | `delete_analyte` | `id` | `{ deleted: <id> }` |
| `results.list` | `list_results` | `limit?` | `{ results: [OrderAnalysisResult, ...] }` |
| `logs.list` | `get_logs` | `limit?` | `{ logs: [EventLog, ...] }` |
| `logs.tail` | `read_last_log_lines` | `lines?/limit?` | `{ lines, topic, logs: [...] }` |
| `logs.activate` | `activate_real_time_logs` | — | `{ active: true, topic }` |
| `logs.deactivate` | `deactivate_real_time_logs` | — | `{ active: false, topic }` |
| `results.activate` | `activate_real_time_results` | — | `{ active: true, topic }` |
| `results.deactivate` | `deactivate_real_time_results` | — | `{ active: false, topic }` |
| `comm.get` | `get_comm_config` | — | `{ communication: {...}, layout: {...} }` |
| `comm.set` | `set_comm_config` | `type?, protocol?` | `{ communication: {...} }` |
| `imports.run_file` | `import_file` | `path, order_date?` | `{ imported, warnings, protocol, file_name }` |

---

## Events (pushed by reader)

| Event type | Payload | When |
|------------|---------|------|
| `log` | `{ level, event_type, message, payload, created_at }` | When real-time logs are active |
| `tick` | `{ reader_id, stats }` | On heartbeat interval |
| `result_available` | `{ source_file, round_no, order, analysis, result }` | When new results are persisted by the reader |

## Presence Events (from wsm-server)

| Event | Payload fields |
|-------|---------------|
| `connected` | `connection_id, client_type, client_id, reader_id, label` |
| `disconnected` | same |

---

## Data Models

### Order
```json
{
    "id": 1, "round_no": 1, "order_date": "2026-04-14",
    "sample_id": "", "file_id": "B-1234", "patient_id": "P001",
    "patient_name": "Popescu Ion", "rack_no": 1, "rack_position": 3,
    "list_position": 0, "sample_no": 1, "status": "scheduled",
    "source_file": "", "meta": {}, "created_at": "...", "updated_at": "..."
}
```

### OrderBundle (from `orders.list` with `include_analysis=true`)
```json
{
    "order": Order,
    "analyses": [{ "analysis": OrderAnalysis, "results": [OrderAnalysisResult] }]
}
```

### OrderAnalysis
```json
{
    "id": 100,
    "order_id": 1,
    "analyte_id": 1,
    "analyte_tag": "IRBT_SALMONELLA_B_GROUPS_V1_O_9_D1",
    "analyte_name": "Salmonella B-groups v1 | O:9 (D1)",
    "analyte_description": "Auto-generated from IR Biotyper tuple",
    "status": "completed",
    "requested_at": "2026-04-15T10:00:00Z",
    "received_at": "2026-04-15T10:01:00Z",
    "default_result_id": 900,
    "result_value": "O:1 (F)",
    "raw_value": "O:1 (F)",
    "interpreted_value": "valid",
    "unit": "",
    "source_file": "manual-test.csv",
    "flags": {}
}
```

### OrderAnalysisResult
```json
{
    "id": 900,
    "order_analysis_id": 100,
    "result_value": "O:1 (F)",
    "raw_value": "O:1 (F)",
    "interpreted_value": "valid",
    "unit": "",
    "source_file": "manual-test.csv",
    "flags": {},
    "created_at": "2026-04-15T10:01:00Z"
}
```

### Analyte
```json
{
    "id": 1, "active": true, "tag": "WBC", "code": "WBC01",
    "name": "White Blood Cells", "description": "",
    "result_type": "numeric", "result_formatting": "raw",
    "result_weighting": 1.0, "result_measure_unit": "10^3/uL",
    "result_reagents_set": "", "created_at": "...", "updated_at": "..."
}
```

### EventLog
```json
{
    "id": 1, "level": "info", "event_type": "ws_connected",
    "message": "connected to WiseMedWS", "payload": {},
    "created_at": "2026-04-14T10:00:00Z"
}
```

---

## WiseMED GUI Files

| File | Purpose |
|------|---------|
| `analyzers_menu.tpl.html` | Top-level analyzers view with grid of registered equipment |
| `analyzers_menu.js` | Equipment grid, row-click → `showLabReader(record)` |
| `labreaders/labreaders.tpl.html` | Reader control panel template (6 tabs) |
| `labreaders/js/labreaders.js` | Full WS controller: Envelope protocol, commands, rendering |

### GUI Tabs
1. **Date pacient** — Patient data + analysis results sub-grid
2. **Dashboard** — Today's stats (with/without result), connection status
3. **Analiti** — CRUD for analytes with inline form
4. **Log comunicare** — Event log viewer + real-time toggle
5. **Comunicare** — Read-only communication config display
6. **Analizor** — Equipment details from WiseMED DB + reader status

---

## Client integration notes

For AI-assisted client generation, the important distinction is:

- `orders.list` can be cheap or rich, depending on `include_analysis`
- use `include_analysis=false` when building compact master lists
- use `include_analysis=true` when building a detail screen from one round-trip
- use `order_analysis.*` when the client edits or fetches analyses independently of the order list

Recommended patterns:

0. Configuration screen:
   - call `config.get`
   - update with `config.set`
1. Dashboard screen:
   - call `stats.get` when one aggregated payload is preferred
1. Orders screen list:
   - call `orders.list` without `include_analysis`
2. Orders screen with embedded detail tree:
   - call `orders.list` with `include_analysis=true`
3. Analysis edit flow:
   - call `order_analysis.get`
   - then `order_analysis.update`
4. Per-order analysis load:
   - call `order_analysis.list` with `order_id`

Supported config sections for `config.get` / `config.set` section mode:

- `wisemed_api`
- `wisemed_ws`
- `local_http`
- `reader`
- `communication`
- `layout`
- `capabilities`

Analyte-specific note:

- `tag` is unique but is no longer the identity used for update/delete
- for edit flows, the client must keep `analyte.id`
- changing `tag` during `analytes.update` updates the same analyte row
- if the target tag already belongs to another analyte, the update fails

---

## JWT Authentication

### Reader → wsm-server
- Signs with `reader.api_key`
- Claims: `sub=<reader_id>`, `role=reader`, `client_id`, `reader_id`, `medical_unit_id`

### Browser → wsm-server
- **Server-minted JWT** — the signing secret never reaches the browser.
- `lrFetchWSToken(readerId)` in `labreaders.js` calls `window._wsmWS.tokenUrl`
  (e.g. `http://localhost:8090/api/test-token`) with query params:
  `subject`, `role=browser`, `client_id`, `reader_id`, `label`
- wsm-server signs the token with its own HS256 key and returns `{ ok:true, token:"<jwt>" }`
- `objLabReaderWSConnect()` calls `lrFetchWSToken()` before every (re)connect,
  so auto-reconnects always get a fresh token
- Token carried as `?token=<jwt>` query param on the WS URL
- Template vars injected by `ini/include_all.php` into `window._wsmWS`:
  - `url`      → `{WISEMED_WS_URL}`       (e.g. `ws://localhost:8090/ws`)
  - `tokenUrl` → `{WISEMED_WS_TOKEN_URL}` (e.g. `http://localhost:8090/api/test-token`)
  - `sub`      → `{WISEMED_WS_JWT_SUB}`   (e.g. `browser-test-a`)
  - `clientId` → `wisemed-gui-{app_user_id}`
  - `label`    → `WiseMED GUI`

### wsm-server config
- `security.accepted_keys`: map of `subject → secret` pairs
- Each reader's `reader.api_key` must match an entry in this map
- Browser subject (e.g. `browser-test-a`) has its own entry; the wsm-server
  uses this to sign the token returned by `/api/test-token`
- WiseMED's `ini/include_all.php` only needs `_wisemed_ws_url`,
  `_wisemed_ws_token_url`, and `_wisemed_ws_jwt_sub` — no secret on the PHP side
