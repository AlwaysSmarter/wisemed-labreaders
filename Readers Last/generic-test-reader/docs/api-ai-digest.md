# Generic Test Reader API AI Digest

## Scope

This document describes the local HTTP API and the WiseMedWS WebSocket command/event surface implemented by:

- `/Users/raduichim/work/gowork/wisemed-labreaders/Readers Last/generic-test-reader/internal/webui/server.go`
- `/Users/raduichim/work/gowork/wisemed-labreaders/Readers Last/generic-test-reader/internal/reader/command.go`
- `/Users/raduichim/work/gowork/wisemed-labreaders/Readers Last/generic-test-reader/internal/reader/app.go`
- `/Users/raduichim/work/gowork/wisemed-labreaders/Readers Last/generic-test-reader/internal/model/types.go`

It is intended for AI agents, automation, integration work, and rapid debugging.

## Runtime architecture

The reader exposes two different integration surfaces:

1. Local HTTP admin API
   - runs on `cfg.LocalHTTP.Address`
   - default is `127.0.0.1:18080`
   - session-cookie based
   - used by the browser admin UI

2. WiseMedWS WebSocket connection
   - outbound connection from the reader to `cfg.WiseMedWS.WSURL`
   - the reader authenticates with a JWT signed from `reader.api_key`
   - commands are received as WS envelopes
   - responses and events are sent back as WS envelopes

## Data models

Primary domain models from `internal/model/types.go`:

### Analyte

```json
{
  "id": 1,
  "active": true,
  "tag": "IRBT_SALMONELLA_B_GROUPS_V1_O_9_D1",
  "code": "IRBT_SALMONELLA_B_GROUPS_V1_O_9_D1",
  "name": "Salmonella B-groups v1 | O:9 (D1)",
  "description": "Auto-generated from IR Biotyper tuple",
  "result_type": "text",
  "result_formatting": "raw",
  "result_weighting": 1,
  "transformation": [],
  "result_measure_unit": "",
  "result_reagents_set": "",
  "protocol_options": {},
  "created_at": "2026-04-06T14:00:00Z",
  "updated_at": "2026-04-06T14:00:00Z"
}
```

### Order

```json
{
  "id": 10,
  "round_no": 1,
  "order_date": "2026-04-06",
  "sample_id": "233914",
  "file_id": "233914",
  "patient_id": "",
  "patient_name": "",
  "rack_no": 1,
  "rack_position": 0,
  "list_position": 0,
  "sample_no": 20,
  "status": "received",
  "source_file": "manual-1775488068162572000-test3.csv",
  "created_at": "2026-04-06T14:25:15Z",
  "updated_at": "2026-04-06T14:25:15Z"
}
```

### OrderAnalysis

```json
{
  "id": 100,
  "order_id": 10,
  "analyte_id": 1,
  "analyte_tag": "IRBT_SALMONELLA_B_GROUPS_V1_O_9_D1",
  "analyte_name": "Salmonella B-groups v1 | O:9 (D1)",
  "analyte_description": "Auto-generated from IR Biotyper tuple",
  "status": "received",
  "requested_at": "",
  "received_at": "2026-04-06T14:25:15Z",
  "default_result_id": 900,
  "result_value": "O:1 (F)",
  "raw_value": "{\"modelKey\":\"...\"}",
  "interpreted_value": "valid",
  "unit": "",
  "source_file": "manual-1775488068162572000-test3.csv",
  "flags": {}
}
```

### OrderAnalysisResult

```json
{
  "id": 900,
  "order_analysis_id": 100,
  "result_value": "O:1 (F)",
  "raw_value": "{\"modelKey\":\"...\"}",
  "interpreted_value": "valid",
  "unit": "",
  "source_file": "manual-1775488068162572000-test3.csv",
  "flags": {},
  "created_at": "2026-04-06T14:25:15Z"
}
```

### EventLog

```json
{
  "id": 2000,
  "level": "info",
  "event_type": "file_import_started",
  "message": "import started",
  "payload": {
    "path": "/abs/path/to/file.csv",
    "file_name": "manual-....csv"
  },
  "created_at": "2026-04-06T14:25:15Z"
}
```

## HTTP authentication model

The local HTTP API uses cookie-backed in-memory sessions.

### Session cookie

- cookie name: `wmr_local_session`
- set by `POST /api/session/login`
- required for all protected routes
- session lifetime: 12 hours
- sessions are kept in memory inside the running process

### Login source

Login is delegated to WiseMed API via `wisemedapi.AdministrativeLogin(...)`.

## HTTP base behavior

### Common response pattern

Most HTTP handlers return:

```json
{
  "ok": true
}
```

or

```json
{
  "ok": false,
  "error": "message"
}
```

### Common HTTP errors

- `400` invalid JSON, invalid parameters, unsupported file extension, domain validation failures
- `401` no authenticated session
- `403` log access denied
- `404` analyte not found
- `405` wrong HTTP method
- `500` internal/server/storage/config failures

## HTTP API

### `GET /api/session`

Returns current session and UI preferences.

Auth:
- not required

Response:

```json
{
  "ok": true,
  "authenticated": true,
  "session": {
    "id": "session-id",
    "username": "reader-file-001",
    "medical_unit_id": 24,
    "user_type": -1,
    "first_name": "John",
    "last_name": "Doe",
    "user_email": "john@example.com",
    "user_picture": "",
    "created_at": "2026-04-06T12:00:00Z",
    "expires_at": "2026-04-07T00:00:00Z"
  },
  "preferences": {
    "language": "ro"
  },
  "permissions": {
    "can_view_logs": true
  },
  "reader": {
    "id": "reader-file-001",
    "label": "IR Biotyper Reader",
    "analyzer_name": "IR Biotyper"
  }
}
```

### `GET /api/preferences`

Returns current UI preferences.

Auth:
- not required

Response:

```json
{
  "ok": true,
  "preferences": {
    "language": "ro"
  }
}
```

### `PUT /api/preferences/language`

Sets UI language.

Auth:
- not required

Body:

```json
{
  "language": "ro"
}
```

Allowed values:
- `ro`
- `en`

Response:

```json
{
  "ok": true,
  "preferences": {
    "language": "ro"
  }
}
```

### `POST /api/session/login`

Creates authenticated local session after administrative WiseMed login.

Auth:
- not required

Body:

```json
{
  "username": "reader-file-001",
  "password": "reader-key-demo-001",
  "medical_unit_id": 24
}
```

Notes:
- if `medical_unit_id` is `0`, backend defaults it to `1`

Response:

```json
{
  "ok": true,
  "session": {
    "id": "session-id",
    "username": "reader-file-001",
    "medical_unit_id": 24,
    "user_type": -1,
    "first_name": "John",
    "last_name": "Doe",
    "user_email": "john@example.com",
    "user_picture": "",
    "created_at": "2026-04-06T12:00:00Z",
    "expires_at": "2026-04-07T00:00:00Z"
  }
}
```

### `POST /api/session/logout`

Deletes current session and clears cookie.

Auth:
- required

Response:

```json
{
  "ok": true
}
```

### `GET /api/status`

Returns runtime snapshot.

Auth:
- required

Response:

```json
{
  "ok": true,
  "data": {
    "reader": {
      "id": "reader-file-001",
      "client_id": "reader-file-001",
      "label": "IR Biotyper Reader",
      "medical_unit_id": 24,
      "equipment_id": 321,
      "equipment_type_id": 12,
      "analyzer_name": "IR Biotyper",
      "analyzer_code": "ir-biotyper"
    },
    "communication": {
      "type": "file",
      "protocol": "IRBIOTYPER"
    },
    "layout": {
      "kind": "simple_list"
    },
    "db_path": "/abs/path/to/wisemed_reader.db",
    "stats": {
      "analytes": 10,
      "orders": 42,
      "results": 55,
      "events": 900
    },
    "connections": {
      "wisemed_ws_connected": false,
      "analyzer_connected": true
    }
  }
}
```

### `GET /api/config`

Returns the full current runtime configuration snapshot.

Auth:
- required

Response:

```json
{
  "ok": true,
  "config": {
    "wisemed_api": { ... },
    "wisemed_ws": { ... },
    "local_http": { ... },
    "reader": { ... },
    "communication": { ... },
    "layout": { ... },
    "capabilities": { ... }
  }
}
```

### `PUT /api/config`

Applies a partial top-level configuration patch and persists it to YAML.

Auth:
- required

Body:
- any subset of top-level config sections:
  - `wisemed_api`
  - `wisemed_ws`
  - `local_http`
  - `reader`
  - `communication`
  - `layout`
  - `capabilities`

Response:

```json
{
  "ok": true,
  "config": { ...full Config }
}
```

### `GET /api/config/{section}`

Returns one config section.

Supported sections:
- `wisemed_api`
- `wisemed_ws`
- `local_http`
- `reader`
- `communication`
- `layout`
- `capabilities`

Response:

```json
{
  "ok": true,
  "section": "reader",
  "config": { ...cfg.Reader }
}
```

### `PUT /api/config/{section}`

Updates one config section and persists it to YAML.

Auth:
- required

Response:

```json
{
  "ok": true,
  "section": "communication",
  "config": { ...updated section }
}
```

### `GET /api/stats?series_limit=30`

Returns the statistics used by the web dashboard in a single payload.

Auth:
- required

Query params:
- `series_limit` optional, default `14`

Response:

```json
{
  "ok": true,
  "data": {
    "stats": {
      "analytes": 10,
      "orders": 42,
      "results": 55,
      "events": 900
    },
    "today": {
      "without_result": 5,
      "with_result": 20
    },
    "series": [
      {
        "day": "2026-04-01",
        "orders": 10,
        "analyses": 40,
        "analyses_with_result": 31
      }
    ],
    "connections": {
      "wisemed_ws_connected": true,
      "analyzer_connected": true
    },
    "reader": {
      "id": "reader-file-001",
      "client_id": "reader-file-001",
      "label": "IR Biotyper Reader"
    }
  }
}
```

### `GET /api/dashboard`

Returns dashboard chart data.

Auth:
- required

Response:

```json
{
  "ok": true,
  "today": {
    "without_result": 5,
    "with_result": 20
  },
  "series": [
    {
      "day": "2026-04-01",
      "orders": 10,
      "analyses": 40,
      "analyses_with_result": 31
    }
  ]
}
```

### `GET /api/logs?limit=40`

Returns recent `event_logs`.

Auth:
- required
- additionally gated by `canViewLogs(sess)`:
  - allowed for `user_type == -1`
  - allowed for `user_type == 0`

Query params:
- `limit` optional, default `50`

Response:

```json
{
  "ok": true,
  "logs": [
    {
      "id": 1,
      "level": "info",
      "event_type": "file_import_started",
      "message": "import started",
      "payload": {
        "path": "/abs/path/file.csv"
      },
      "created_at": "2026-04-06T14:25:15Z"
    }
  ]
}
```

### `GET /api/orders`

Lists orders for a date/round.

Auth:
- required

Query params:
- `order_date` optional, `YYYY-MM-DD`
- `round_no` optional
- `include_analysis` optional
  - `0` or omitted: returns plain `Order[]`
  - `1`: returns `OrderBundle[]` with `analyses` and nested `results`

Response:

```json
{
  "ok": true,
  "orders": [ ...OrderBundle ],
  "rounds": [1, 2],
  "order_date": "2026-04-06",
  "round_no": 2,
  "include_analysis": true
}
```

### `GET /api/order-analysis`

Lists analyses for one order.

Auth:
- required

Query params:
- `order_id` required

Response:

```json
{
  "ok": true,
  "order_analyses": [ ...OrderAnalysis ]
}
```

### `POST /api/order-analysis`

Creates a new order analysis.

Auth:
- required

Body:
- JSON matching `OrderAnalysis`
- minimum required fields:
  - `order_id`
  - `analyte_tag`

Response:

```json
{
  "ok": true,
  "order_analysis": { ...OrderAnalysis }
}
```

### `GET /api/order-analysis/{id}`

Returns one order analysis by id.

### `PUT /api/order-analysis/{id}`

Updates one order analysis by id.

Auth:
- required

Body:
- JSON matching `OrderAnalysis`

Response:

```json
{
  "ok": true,
  "order_analysis": { ...OrderAnalysis }
}
```

### `DELETE /api/order-analysis/{id}`

Deletes one order analysis and its nested results.

Response:

```json
{
  "ok": true,
  "deleted": 100
}
```

Returns order bundles filtered by date and round.

Auth:
- required

Query params:
- `order_date` optional
- `round_no` optional

Behavior:
- if `order_date` missing, backend defaults to current date
- if `round_no` missing or `<= 0`, backend selects the last existing round for that date
- if no rounds exist yet, response still guarantees at least `[1]`

Response:

```json
{
  "ok": true,
  "orders": [
    {
      "order": {
        "id": 10,
        "round_no": 1,
        "order_date": "2026-04-06",
        "sample_id": "233914",
        "sample_no": 20,
        "status": "received"
      },
      "analyses": [
        {
          "analysis": {
            "id": 100,
            "analyte_tag": "IRBT_SALMONELLA_B_GROUPS_V1_O_9_D1",
            "result_value": "O:1 (F)",
            "default_result_id": 900
          },
          "results": [
            {
              "id": 900,
              "result_value": "O:1 (F)"
            }
          ]
        }
      ]
    }
  ],
  "rounds": [1, 2],
  "order_date": "2026-04-06",
  "round_no": 2
}
```

### `POST /api/orders`

Creates or updates a local order.

Auth:
- required

Body:
- JSON serialized `model.Order`

Response:

```json
{
  "ok": true,
  "order": {
    "id": 10,
    "round_no": 1,
    "order_date": "2026-04-06",
    "sample_id": "233914"
  }
}
```

### `POST /api/orders/rounds`

Creates next round for a given date.

Auth:
- required

Body:

```json
{
  "order_date": "2026-04-06"
}
```

Behavior:
- if `order_date` missing, backend defaults to current date

Response:

```json
{
  "ok": true,
  "order_date": "2026-04-06",
  "round_no": 2,
  "rounds": [1, 2]
}
```

### `POST /api/orders/import`

Uploads and imports a file in file-communication mode.

Auth:
- required

Content type:
- `multipart/form-data`

Fields:
- `file`: required uploaded file
- `order_date`: optional but strongly recommended; used to import into selected day

Behavior:
- only available when `communication.type == file`
- extension is validated against configured file pattern
- upload is saved into configured import directory
- manual import path calls forced import logic and returns a real summary

Response:

```json
{
  "ok": true,
  "path": "/abs/path/inbox/manual-1775....csv",
  "file_name": "manual-1775....csv",
  "imported": 3,
  "warnings": 0,
  "protocol": "IRBIOTYPER",
  "order_date": "2026-04-06"
}
```

Example failure:

```json
{
  "ok": false,
  "error": "no records imported from /abs/path/inbox/manual-....csv"
}
```

### `POST /api/orders/export`

Exports selected orders as CSV/worklist.

Auth:
- required

Body:

```json
{
  "order_ids": [10, 11, 12],
  "order_date": "2026-04-06"
}
```

Response:

```json
{
  "ok": true,
  "path": "/abs/path/outbox/ir-biotyper-worklist-20260406-180738.csv",
  "rows": 12
}
```

### `PUT /api/results/default`

Sets default result for a specific order analysis.

Auth:
- required

Body:

```json
{
  "order_analysis_id": 100,
  "result_id": 900
}
```

Response:

```json
{
  "ok": true
}
```

### `GET /api/analytes`

Returns all analytes.

Auth:
- required

Response:

```json
{
  "ok": true,
  "analytes": [
    {
      "id": 1,
      "tag": "IRBT_SALMONELLA_B_GROUPS_V1_O_9_D1",
      "name": "Salmonella B-groups v1 | O:9 (D1)"
    }
  ]
}
```

### `POST /api/analytes`

Creates analyte.

Auth:
- required

Body:
- JSON serialized `model.Analyte`
- `tag` must be unique

Response:

```json
{
  "ok": true,
  "id": 1,
  "tag": "IRBT_SALMONELLA_B_GROUPS_V1_O_9_D1"
}
```

### `GET /api/analytes/:id`

Returns single analyte by id.

Auth:
- required

Response:

```json
{
  "ok": true,
  "analyte": {
    "id": 1,
    "tag": "IRBT_SALMONELLA_B_GROUPS_V1_O_9_D1"
  }
}
```

### `PUT /api/analytes/:id`

Updates analyte by id.

Auth:
- required

Body:
- JSON serialized `model.Analyte`
- server overwrites `item.ID = :id`
- `tag` must remain unique across all analytes
- if the new tag already exists on another record, request fails

Response:

```json
{
  "ok": true,
  "id": 1,
  "tag": "IRBT_SALMONELLA_B_GROUPS_V1_O_9_D1"
}
```

### `DELETE /api/analytes/:id`

Deletes analyte by id.

Auth:
- required

Response:

```json
{
  "ok": true,
  "deleted": 1
}
```

### `GET /help/`

Serves local static help content.

Auth:
- required

Notes:
- backed by filesystem help directory
- default content is auto-generated if missing

## WebSocket transport model

The reader uses a generic envelope:

```json
{
  "type": "command",
  "request_id": "req-123",
  "correlation_id": "",
  "connection_id": "",
  "target": {
    "mode": "topic",
    "connection_id": "",
    "client_type": "",
    "reader_id": "",
    "topic": "logs:reader-file-001"
  },
  "broadcast": false,
  "payload": {},
  "timestamp": "2026-04-06T12:00:00Z"
}
```

Envelope types observed in runtime:

- outbound from reader:
  - `hello`
  - `ping`
  - `event`
  - command responses via `respond(...)`
- inbound to reader:
  - `command`
  - also ignored/pass-through message types:
    - `hello_ack`
    - `command_ack`
    - `presence`
    - `connections`
    - `pong`
    - `error`

### Reader WS startup behavior

On successful WS connect, the reader:

1. signs JWT with `reader.api_key`
2. connects to `cfg.WiseMedWS.WSURL?token=<jwt>`
3. sends:

```json
{
  "type": "hello",
  "request_id": "generated-id",
  "payload": {
    "client_type": "reader",
    "client_id": "reader-file-001",
    "reader_id": "reader-file-001",
    "label": "IR Biotyper Reader"
  }
}
```

4. periodically sends `ping` heartbeats
5. may emit `event` envelopes such as `log`, `tick`, `result_available`

## WebSocket commands

Incoming command envelope shape:

```json
{
  "type": "command",
  "request_id": "req-1",
  "payload": {
    "command": "reader.status",
    "args": {}
  }
}
```

The code accepts both canonical names and some legacy aliases.

### `reader.status`

Aliases:
- `get_status`

Args:
- none

Response data:
- same payload shape as `GET /api/status`, but without outer HTTP wrapper

### `logs.list`

Aliases:
- `get_logs`

Args:

```json
{
  "limit": 100
}
```

Response data:

```json
{
  "logs": [ ...EventLog ]
}
```

### `logs.tail`

Aliases:
- `read_last_log_lines`

Args:

```json
{
  "lines": 100
}
```

or

```json
{
  "limit": 100
}
```

Response data:

```json
{
  "lines": 100,
  "topic": "logs:reader-file-001",
  "logs": [ ...EventLog ]
}
```

### `logs.activate`

Aliases:
- `activate_real_time_logs`

Args:
- none

Response data:

```json
{
  "active": true,
  "topic": "logs:reader-file-001"
}
```

### `logs.deactivate`

Aliases:
- `deactivate_real_time_logs`

Args:
- none

Response data:

```json
{
  "active": false,
  "topic": "logs:reader-file-001"
}
```

### `results.activate`

Aliases:
- `activate_real_time_results`

Args:
- none

Response data:

```json
{
  "active": true,
  "topic": "results:reader-file-001"
}
```

### `results.deactivate`

Aliases:
- `deactivate_real_time_results`

Args:
- none

Response data:

```json
{
  "active": false,
  "topic": "results:reader-file-001"
}
```

### `analytes.list`

Aliases:
- `list_analytes`

Args:
- none

Response data:

```json
{
  "analytes": [ ...Analyte ]
}
```

### `analytes.get`

Args:

```json
{
  "id": 1
}
```

Alternative accepted arg:

```json
{
  "tag": "IRBT_SALMONELLA_B_GROUPS_V1_O_9_D1"
}
```

Response data:

```json
{
  "analyte": { ...Analyte }
}
```

### `analytes.create`

Aliases:
- `upsert_analyte`

Args:
- fields parsed by `analyteFromArgs(...)`
- same analyte write fields as HTTP create/update
- `tag` must be unique

Response data:

```json
{
  "id": 1,
  "tag": "IRBT_SALMONELLA_B_GROUPS_V1_O_9_D1"
}
```

### `analytes.update`

Aliases:
- `upsert_analyte`

Args:
- same as create
- `id` is required for correct update semantics
- `tag` can be changed, but it must remain unique

### `analytes.delete`

Aliases:
- `delete_analyte`

Args:

```json
{
  "id": 1
}
```

Response data:

```json
{
  "deleted": 1
}
```

### `orders.list`

Aliases:
- `list_orders`

Args:

```json
{
  "round_no": 1,
  "order_date": "2026-04-06",
  "include_analysis": true
}
```

Legacy compatibility:
- `round_id` also accepted

Response data:

```json
{
  "orders": [ ...OrderBundle ]
}
```

Important note:
- if `include_analysis` is omitted or `false`, WS `orders.list` returns plain `Order[]`
- if `include_analysis=true`, WS `orders.list` returns `OrderBundle[]`
- HTTP `GET /api/orders` uses the same behavior through `include_analysis`

### `orders.get`

Args:

```json
{
  "id": 10
}
```

Response data:

```json
{
  "order": { ...Order }
}
```

### `orders.create`

Aliases:
- `create_order`

Args:
- fields from `model.Order`
- defaults:
  - `order_date` defaults to current date
  - round defaults to current round for the date

Response data:

```json
{
  "order": { ...Order }
}
```

### `orders.update`

Aliases:
- `update_order`

Args:
- same as create

### `orders.delete`

Aliases:
- `delete_order`

Args:

```json
{
  "id": 10
}
```

Response data:

```json
{
  "deleted": 10
}
```

### `order_analysis.list`

Aliases:
- `list_order_analysis`

Args:

```json
{
  "order_id": 10
}
```

Response data:

```json
{
  "order_analyses": [ ...OrderAnalysis ]
}
```

### `order_analysis.get`

Aliases:
- `get_order_analysis`

Args:

```json
{
  "id": 100
}
```

Response data:

```json
{
  "order_analysis": { ...OrderAnalysis }
}
```

### `order_analysis.create`

Aliases:
- `create_order_analysis`

Args:
- fields from `model.OrderAnalysis`
- minimum required:
  - `order_id`
  - `analyte_tag`

Response data:

```json
{
  "order_analysis": { ...OrderAnalysis }
}
```

### `order_analysis.update`

Aliases:
- `update_order_analysis`

Args:
- same as create
- `id` required

### `order_analysis.delete`

Aliases:
- `delete_order_analysis`

Args:

```json
{
  "id": 100
}
```

Response data:

```json
{
  "deleted": 100
}
```

### `results.list`

Aliases:
- `list_results`

Args:

```json
{
  "limit": 100
}
```

Response data:

```json
{
  "results": [ ...OrderAnalysisResult ]
}
```

### `comm.get`

Aliases:
- `get_comm_config`

Args:
- none

Response data:

```json
{
  "communication": { ...cfg.Comm },
  "layout": { ...cfg.Layout }
}
```

### `config.get`

Aliases:
- `get_config`

Args:
- none, or:

```json
{
  "section": "reader"
}
```

Response data:

```json
{
  "config": { ...full Config }
}
```

If `section` is provided:

```json
{
  "section": "reader",
  "config": { ...cfg.Reader }
}
```

### `config.set`

Aliases:
- `set_config`

Args:
- either a top-level partial config patch:

```json
{
  "reader": {
    "label": "Updated Reader Label"
  },
  "communication": {
    "protocol": "IRBIOTYPER"
  }
}
```

- or a section update:

```json
{
  "section": "reader",
  "data": {
    "label": "Updated Reader Label"
  }
}
```

Response data:
- full config when patching multiple sections
- section payload when using `section` + `data`

### `stats.get`

Aliases:
- `get_stats`

Args:

```json
{
  "series_limit": 14
}
```

Response data:

```json
{
  "stats": { ... },
  "today": { ... },
  "series": [ ...DashboardSeriesPoint ],
  "connections": { ... },
  "reader": { ... }
}
```

### `comm.set`

Aliases:
- `set_comm_config`

Args:

```json
{
  "type": "file",
  "protocol": "IRBIOTYPER"
}
```

Behavior:
- only `type` and `protocol` are updated by current implementation
- config is persisted to YAML

Response data:

```json
{
  "communication": { ...cfg.Comm }
}
```

### `imports.run_file`

Aliases:
- `import_file`

Args:

```json
{
  "path": "/abs/path/to/file.csv",
  "order_date": "2026-04-06"
}
```

Response data:

```json
{
  "imported": 3,
  "warnings": 0,
  "protocol": "IRBIOTYPER",
  "file_name": "manual-1775....csv"
}
```

## WebSocket events emitted by reader

Events are sent through:

```json
{
  "type": "event",
  "request_id": "generated-id",
  "target": {
    "mode": "topic",
    "topic": "logs:reader-file-001"
  },
  "payload": {
    "event_type": "log",
    "reader_id": "reader-file-001",
    "payload": { ...event payload... }
  },
  "timestamp": "2026-04-06T12:00:00Z"
}
```

### `log`

Payload:

```json
{
  "level": "info",
  "event_type": "file_import_started",
  "message": "import started",
  "payload": {
    "path": "/abs/path/file.csv"
  },
  "created_at": "2026-04-06T12:00:00Z"
}
```

### `tick`

Typical payload:

```json
{
  "reader_id": "reader-file-001",
  "stats": {
    "analytes": 10,
    "orders": 42,
    "results": 55,
    "events": 900
  }
}
```

Import-related `tick` payloads may also include:

```json
{
  "mode": "file",
  "source_file": "manual-....csv",
  "imported": 3,
  "warnings": 0,
  "protocol": "IRBIOTYPER"
}
```

### `result_available`

Generic import payload example:

```json
{
  "source_file": "manual-....csv",
  "round_no": 1,
  "order": { ...Order },
  "analysis": { ...OrderAnalysis },
  "result": { ...OrderAnalysisResult }
}
```

## Important behavior notes

### Orders date and round semantics

- rounds are date-scoped
- UI and API should always expose at least round `1`
- import operations should target explicit `order_date` where provided
- first sample of a day should start at `sample_no = 1`

### File import constraints

- only available in `communication.type = file`
- server validates file extension against configured file pattern
- parser supports multiple file styles depending on protocol/content
- import logs are critical for debugging parser behavior

### Log visibility

- `GET /api/logs` is stricter than other protected routes
- only admin-like users can access logs in HTTP UI
- WS real-time logs require `logs.activate`
- WS result notifications use a dedicated topic returned by `results.activate`

## Source of truth

If this document and runtime behavior diverge, the source of truth is the implementation in:

- `internal/webui/server.go`
- `internal/reader/command.go`
- `internal/reader/app.go`
- `internal/storage/sqlite.go`
- `internal/model/types.go`
