# Catalog API — expus și consumat

| | |
|---|---|
| **Scop** | Inventarul unic al interfețelor expuse și consumate de aplicațiile din repository (`readersv3/`, `wsm-server/`, `apis/`): rută/comandă, handler, autentificare, autorizare, contract, erori, efecte, timeout, idempotency, transport (HTTP local / WSS). |
| **Data** | 2026-11-14 (checkout local `main`, HEAD `d53088e`) |
| **Surse** | `_reports/01-runtime-and-apps.md` (§2–§3), `_reports/02-wsm-security.md`, `_reports/03-laboratory.md`, `_reports/04-cnas-siui.md`, `_reports/05-utilities-ai.md`, `_reports/06-qa-build-ops.md` (§B, rezultate de test); verificări punctuale, doar citire, în cod (marcate „verificat la redactare”). |
| **Limite** | Nu este o specificație OpenAPI și nu inventează scheme: unde rapoartele nu au detaliul, apare `NECUNOSCUT` sau trimiterea la sursa din cod. Backendul WiseMED real, serverele CNAS, endpointul de token WSM din WiseMED și aplicația Flutter completă sunt **externe**; exemplele PHP/JS din repo nu dovedesc implementarea lor. Configurațiile runtime reale și bazele de date nu au fost citite. Niciun secret nu este reprodus. |

**Etichete** (fiecare afirmație importantă poartă una):
`[COD]` = CONFIRMAT DIN COD · `[TEST]` = CONFIRMAT PRIN TEST LOCAL (numai ce a rulat raportul 06 §B.1) · `[DOC]` = DOCUMENTAT, NEVERIFICAT · `[INF]` = INFERENȚĂ · `[NEC]` = NECUNOSCUT.

**Prescurtări de cale**: `R/` = `readersv3/` · `LH` = `readersv3/modules/localhttp/module.go` · `WS` = `readersv3/modules/ws/module.go` · `AB` = `readersv3/shared/apibridge/bridge.go` · `S/` = `wsm-server/internal/server/` · `C/` = `wsm-server/internal/config/` · `SI/` = `readersv3/modules/siui/` · `H/` = `readersv3/modules/siuihistory/` · `D/` = `readersv3/modules/siuidata/` · `AI/` = `readersv3/modules/wisemedai/`.

**Documente înrudite**: [application-catalog.md](application-catalog.md) · [runtime-and-modules.md](runtime-and-modules.md) · [communication-map.md](communication-map.md) · [end-to-end-flows.md](end-to-end-flows.md) · [data-and-state-machines.md](data-and-state-machines.md) · [security-and-identity.md](security-and-identity.md) · [build-deploy-update.md](build-deploy-update.md) · [verification-matrix.md](verification-matrix.md) · [gaps-and-contradictions.md](gaps-and-contradictions.md) · [agent-context.md](agent-context.md).

---

## Cuprins

1. [Reguli transversale](#1-reguli-transversale)
2. [Envelope-uri JSON](#2-envelope-uri-json)
3. [Runtime `readersv3`](#3-runtime-readersv3)
4. [`local-http`](#4-local-http)
5. [Module de laborator](#5-module-de-laborator)
6. [WSM — WebSocket, HTTP, admin, SDK](#6-wsm)
7. [Comenzile de dispatcher WS din readere](#7-comenzile-de-dispatcher-ws)
8. [SIUI bridge](#8-siui-bridge)
9. [SIUI Bridge History](#9-siui-bridge-history)
10. [WiseMED SIUI Data](#10-wisemed-siui-data)
11. [WiseMED AI](#11-wisemed-ai)
12. [HTML Print Formatter](#12-html-print-formatter)
13. [Barcode Printer](#13-barcode-printer)
14. [Signing pad / eSignature](#14-signing-pad--esignature)
15. [ANAF DocSMART](#15-anaf-docsmart)
16. [Update server](#16-update-server)
17. [API-uri consumate](#17-api-uri-consumate)
18. [Paritate](#18-paritate)
19. [Necunoscute](#19-necunoscute)

---

## 1. Reguli transversale

### 1.1 Un singur mux, fără middleware global

- Toate modulele unei aplicații `readersv3` înregistrează rute pe același `*http.ServeMux` (`rt.Handle`), servit de `local-http` (`LH:323`); implicit `127.0.0.1:18080`, HTTP simplu, TLS opțional. `[COD]` (`R/core/runtime/runtime.go:77-95`).
- **Nu există middleware global de autentificare.** O rută este protejată numai dacă este înfășurată explicit cu `requireSession` (`LH:568-577`) sau dacă modulul își face propria verificare. `[COD]`
- Excepție de compunere: `update-server` are listener propriu și nu folosește `runner` (`R/apps/update-server/main.go:57`). `[COD]`

### 1.2 HTTP local vs WSS (`api.request`)

`api.request` este o **invocare in-process** a aceluiași mux (`handler.ServeHTTP` pe un recorder), nu un proxy de rețea (`AB:51-162`, `WS:512-533`). `[COD]`; apelul API cap-coadă printr-un WSM cu TLS pe loopback a rulat în testul de integrare `[TEST]` (06 §B.1 r.10).

| Aspect | Regulă | Referință |
|---|---|---|
| Căi acceptate | relative, `/api/…` sau exact `/barcode/print`; fără `//`, host, schemă, `\`, NUL, `/../`, `/./` | `AB:55-65` `[COD]` |
| Căi excluse | `/api/wss/bridge*`, `/api/wss/open`, `/api/wss/debug`, `/api/session/login`, `/api/session/logout` | `AB:63-65` `[COD]` |
| Metode | GET, POST, PUT, PATCH, DELETE, HEAD | `AB:66-71` `[COD]` |
| Cerere | `args{method, path, body \| body_base64 + content_type}`; ≤ 4 MiB decodat | `AB:18,72-87` `[COD]` |
| Răspuns | `{kind:"api.response", status, content_type, headers{Content-Disposition,ETag,Location}, body \| body_base64}`; ≤ 4 MiB; `text/html` refuzat | `AB:138-161,177-186` `[COD]` |
| Streaming | inexistent; răspuns bufferizat într-un singur mesaj; `boundedRecorder` nu implementează `http.Flusher` ⇒ SSE imposibil | `AB`, 05 §0 `[COD]` |
| Fragmentare | doar la nivel de aplicație (ex. `calls/{id}/file?offset=`, 256 KiB) | 04 §2.4 `[COD]` |
| Timeout intern | 30 s implicit; 610 s `POST /api/html-print-formatter/{render,pdf}`; 130 s `POST /api/siui/general/ocsp`, `/api/siui/ocsp-test`, `/api/siui/invoice/package`; 310 s `POST /api/history/calls/*/commit` | `AB:95-110` `[COD]` |
| Anulare | contextul derivă din sesiunea WSS; deconectarea îl anulează; handlerul sincron nu este întrerupt forțat | `AB:109-126`, `WS:411-441` `[COD]` |
| Identitate | `Principal{Subject, TenantID, ConnectionID, SourceIP, Admin}` din câmpurile `sender_*` ștampilate de WSM; `api:invoke` obligatoriu; `api:admin` ⇒ `Admin` | `WS:517-528` `[COD]` |
| Plafon suplimentar | `max_message_bytes` al WSM (1 MiB implicit; 8 MiB în șablonul de producție); read limit 8 MiB în reader | `C/config.go:131-172`, `WS:368-396` `[COD]` |

În coloanele „Transport” de mai jos: **L** = doar HTTP local; **L+W** = HTTP local și WSS `api.request`; **W** = doar comandă WS. Regula implicită: orice rută `/api/…` care nu este exclusă mai sus este L+W; rutele din afara `/api/` (cu excepția `/barcode/print`) sunt doar L.

### 1.3 Ce nu există la nivel de transport

Fără coadă persistentă, replay, deduplicare de `request_id`, mesaj de anulare sau livrare garantată în WSM (`S/hub.go:43-56`, `S/server.go:531-564`). `[COD]` Idempotency există numai unde un handler o implementează explicit (SIUI `operation_id`, AI `Idempotency-Key` și `(chunk id, revision)`, History `INSERT IGNORE`+digest). Vezi [data-and-state-machines.md](data-and-state-machines.md).

---

## 2. Envelope-uri JSON

Envelope-ul **nu este uniform** între aplicații și nici în interiorul unora. `[COD]`

| Aplicație / grup de rute | Succes | Eroare | Referință |
|---|---|---|---|
| `local-http` — `/api/wss/*` | `{"ok":true,…}` | `{"ok":false,"error":"text"}` | `R/modules/localhttp/wss.go:36-60` (verificat la redactare) |
| `local-http` — restul rutelor | `NECUNOSCUT` per rută (handlerii nu au fost inventariați pe formă); vezi sursa `LH` | idem | 03 §Necunoscute pct. 6 |
| SIUI bridge — majoritatea `/api/siui/*` | `{"ok":true,…}` | `{"ok":false,"error":"mesaj"}` | `SI/module.go:223-231` |
| SIUI bridge — `operations*`, `dashboard`, `downloads/storage`, `siui-history-sync/*` | `{"success":true,…}` | `{"ok":false,"error":…}` (erorile rămân pe `ok`) | `SI/operations_history.go:220,282`, `SI/download_storage.go:175`, `SI/history_sync.go:137,156` |
| SIUI bridge — `GET /api/siui-history-sync/settings` | obiect brut, fără envelope | `{"ok":false,…}` | `SI/history_sync.go:95-99` |
| History | `{"success":true,…}` | `{"success":false,"error":…}` | `H/module.go:97-104` |
| SIUI Data | `{"success":true,"data":…}`; liste `{"success":true,"rows":[…],"totalCount":N}` | `{"success":false,"error":…}` | `D/module.go:184-203` |
| WiseMED AI | obiect/array brut | `{"error":"<cod_snake>"}`; la 402 și `subscription`/`account_url` | `AI/module.go:138,145` |
| html-print-formatter — `/render*`, `/pdf` | `text/html` / `application/pdf` brut | `{"ok":false,"error":{"code","message"},"request_id"}` | `R/modules/htmlprintformatter/module.go:192-198` |
| html-print-formatter — `/api/html-print-formatter/*` | `{"ok":true,"html",…,"request_id"}` | idem | idem |
| barcodeprinter | `{"success":true,…}` | `{"success":false,"error":"text"}` | 05 §0 |
| signingpad (protocol nou) | `{"id","action","ok":true,…}` | `{"id","ok":false,"message"}` sau text `http.Error` | 05 §0 |
| signingpad (WS legacy) | `{"success":true,"forevent":<cererea>,"data":{…}}` | `{"success":false,"forevent":…,"data":{},"error":"…"}` | `R/modules/signingpad/legacy.go:25-30` |
| anaf-docsmart | `{"ok":true,…}` | `{"ok":false,"error":"text"}` | 05 §0 |
| update-server admin | `{"ok":true,…}` | `{"ok":false,"error":"text"}` | 05 §0 |
| update-server public | `appupdates.CheckResponse` | `{"ok":false,"message":"text"}` (cheia este `message`) | 05 §0 |
| WSM HTTP | `{request_id, connection_id, response}` / snapshot | `{"error":"<cod>","message"}` | `S/http_commands.go`, `S/auth.go:42-51` |
| Reply de adaptor WSS | `{"kind":"api.response","status",…}` | eroare de adaptor `{"ok":false,"error":{"message"}}` | `AB:42-49`, `WS:545` |
| Reply de dispatcher WS | payload liber al handlerului | `{"ok":false,"error":{"message":"unknown action"}}` | `WS:506-551` |

Consecință practică `[INF]`: un client generic trebuie să trateze trei niveluri distincte de eșec pe calea WSS — eroare de transport/rutare (`error`, `command_ack recipients:0`), eroare de adaptor (`ok:false`), și `status` HTTP non-2xx în `api.response` cu envelope-ul propriu al aplicației.

---

## 3. Runtime `readersv3`

Înregistrate de runtime, în orice aplicație (`R/core/runtime/runtime.go:50-52`). `[COD]`

| Metodă | Rută | Auth | Răspuns | Transport |
|---|---|---|---|---|
| GET | `/healthz` | publică | `NECUNOSCUT` (forma exactă: vezi sursa) | L |
| GET | `/api/menu` | publică | lista `MenuEntry{ID, Group, Label, Path, Order}` sortată pe `(Group, Order)` (`R/core/module/types.go:8`) | L+W |
| GET | `/api/modules` | publică | modulele și serviciile descriptive | L+W |

Rute `/meta` de modul, toate publice, GET, fără logică (03 §9.2) `[COD]`: `/api/storage/meta` (`R/modules/storage/sqlite/module.go:75`), `/api/events/meta` (`events/module.go:21`), `/api/analytes/meta`, `/api/analytes/management/meta`, `/api/orders/meta`, `/api/dashboard/meta`, `/api/qc/meta`, `/api/stats/meta`, `/api/login/meta` (`login/module.go:16` — modulul `login` este placeholder), `/api/daily-details/meta`, `/api/wisemed/meta` (`wisemedapi/module.go:154`), `/api/protocol/meta` (fiecare protocol). `/help`, `/help/*` — fișiere statice (`help/module.go:28-41`).

---

## 4. `local-http`

Modul `local-http` (`R/modules/localhttp/`), prezent în toate aplicațiile bazate pe `runner`. Sesiune = cookie HMAC local **sau** principal WSM (`currentSession`, `LH:2566-2593`). Admin local = `UserType <= 0`. Detalii de identitate: [security-and-identity.md](security-and-identity.md).

### 4.1 UI, sesiune, configurare

| Metodă | Rută | Handler | Auth | Contract / efecte | Transport |
|---|---|---|---|---|---|
| GET | `/`, `/settings*`, `/orders`, `/qc`, `/history`, `/operations`, `/debug` | `handleIndex` (`LH:218-229`) | publică | shell UI | L |
| GET | `/app.js`, `/styles.css`, `/wss-remote.js`, `/docmed-forms.js`, `/siui-invoice.js` | `handleStaticAsset` (`LH:230-234`) | publică | asset-uri embed | L |
| GET | `/api/session` | `handleSessionStatus` (`LH:235`/`:626`) | publică | stare sesiune; include `reader.id` (`LH:652,2724`) | L+W |
| GET… | `/api/preferences`, `/api/preferences/language` | `handlePreferences`, `handleLanguage` (`LH:236-237`) | publică | metode/corp: `NECUNOSCUT` | L+W |
| POST | `/api/session/login` | `handleLogin` (`LH:241`/`:1106-1170`) | publică; cere `SetupComplete()` | `{username, password, medical_unit_id}` → login la WiseMED → `EnsureEquipmentOnline` → cookie 12 h. Fără rate limit găsit | **L** (exclusă din WSS) |
| POST | `/api/session/logout` | `handleLogout` (`LH:242`/`:1252`) | sesiune | șterge intrarea din map și cookie-ul; tokenul semnat rămâne valid până la expirare | **L** |
| GET, PUT | `/api/wisemed/setup` | `handleWiseMEDSetup` (`LH:239`/`:1172`) | **fără sesiune** la readere; sesiune doar la utilitarele login-only deja configurate (`LH:1188-1194`) | scrie configurarea WiseMED | L+W |
| GET | `/api/wisemed/bootstrap` | `handleWiseMEDBootstrap` (`LH:240`/`:1233`) | **fără sesiune** | liste unități medicale / tipuri de analizoare | L+W |
| GET, PUT | `/api/reader-settings` | `handleReaderSettings` (`LH:238`/`:699`) | sesiune | corp: `NECUNOSCUT` | L+W |
| GET | `/api/app-update/status` | `handleAppUpdateStatus` (`LH:243`, `:2796-2893`) | **fără sesiune** | stare update (cache 30 s); declanșează `check-update` la update-server | L+W |
| GET, PUT | `/api/app-update/settings` | `handleAppUpdateSettings` (`LH:244`) | sesiune | corp: `NECUNOSCUT` | L+W |

Toate `[COD]`. CORS local: origini din `cors_allowed_origins` (implicit `https://ldse.wisemed.eu`), `*` acceptat, cu `Access-Control-Allow-Credentials: true` (`LH:213,464-523`).

### 4.2 Comenzi, rezultate, QC, sincronizare

Toate cu `requireSession`; nu există roluri per rută: orice utilizator autentificat (sau orice principal cu `api:invoke`) poate șterge comenzi, corecta ID-uri și trimite în buletin. `[COD]` (03 §9.1). Metodele marcate `*` sunt deduse din nume/UI `[INF]`.

| Metodă | Rută | Handler (`LH`) | Contract / efecte |
|---|---|---|---|
| GET | `/api/status` | `:246`/`:1290` | stare reader + `analyzer-activity` |
| GET | `/api/dashboard` | `:247`/`:1376` | `DashboardSnapshot` |
| GET | `/api/logs?limit=` | `:248`/`:1432` | jurnal calculat (`UNION`), implicit 40 |
| GET, POST | `/api/analytes` | `:249`/`:1459` | catalog analite |
| GET, PUT, DELETE | `/api/analytes/{id}` | `:250`/`:1490` | |
| PUT | `/api/orders/id` | `:251`; `R/modules/localhttp/order_identity.go:15-51` | corp ≤ 8192 octeți: `{order_id, expected_id, expected_revision, new_id, confirmation:"deacord", reason}`; 400 validare, 409 concurență/coliziune; tranzacție unică + audit `order-id-change`; actor din sesiune |
| GET | `/api/orders?order_date=&round_no=&search=` | `:252`/`:1535` | |
| GET | `/api/order-image` | `:253`/`:1670` | |
| GET | `/api/orders/worklist` | `:254`/`:1966` | doar protocolul `cary60-uvvis` |
| GET, POST | `/api/orders/rounds` | `:255`/`:1618` | POST = rundă nouă |
| POST | `/api/orders/import` | `:256`/`:1689-1731` | multipart → `.tmp-imports/` → `ImportFileNow` |
| POST | `/api/orders/export` | `:257`/`:1732` | |
| POST, DELETE | `/api/orders/delete` | `:258`/`:1736` | șterge comanda + rezultate; auditat |
| POST | `/api/orders/reapply-transformations` | `:259`/`:1771` | |
| POST | `/api/orders/send-to-wisemed`, `/api/orders/send-to-bulletin` | `:260-261`/`:1802-1941` | sincronizare + `PATCH` rezultate la WiseMED cu `login_token` al operatorului; 502 la eșec; audit `results-send-bulletin`; fără retry, fără cheie de idempotency |
| GET, POST* | `/api/daily-details/definitions`, `…/{id}` | `:262-263` | |
| GET, PUT | `/api/daily-details` | `:264`/`:2008` | |
| GET | `/api/daily-details/worksheet` | `:265`/`:2147` | |
| GET, POST, DELETE* | `/api/daily-analysis-filters`, `…/{id}` | `:266-267` | |
| GET, POST | `/api/qc-records` | `:268`/`:2276` | POST = QC manual |
| POST | `/api/qc-records/delete` | `:269`/`:2348` | |
| GET, POST | `/api/qc-targets`; GET/PUT/DELETE `…/{id}` | `:270-271` | |
| GET | `/api/qc/metrics?analyte_tag=&control_level=&lot_no=&date_from=&date_to=` | `:272`/`:2456` | `QCPerformance` + Westgard (informativ) |
| PUT | `/api/results/default` | `:273`/`:2486-2510` | rezultat implicit; respectă `repeat_mode` |
| GET | `/api/result-sync/status` | `:274`/`:984` | |
| GET, PUT | `/api/result-sync/settings` | `:275`/`:999` | |
| POST | `/api/result-sync/run`, `/api/result-sync/orders`, `/api/result-sync/reset` | `:276-278` | o singură rulare simultană; a doua primește eroare |

Transport: L+W. Corpurile și codurile nespecificate: `NECUNOSCUT` (handlerii nu au fost citiți integral — 03 §Necunoscute pct. 6). Rute absente în v3, deși apar în documentație/memorie veche: `/api/stats`, `/api/stats/series`, `/api/order-analysis`, `/api/config` `[COD]` (03 §7.3, C3).

### 4.3 Control WSS local (`/api/wss/*`)

`handleWSS` (`R/modules/localhttp/wss.go:35-175`, verificat la redactare). Sesiune obligatorie; toate în afară de `status` cer admin local (`:41-44`); mutațiile cer `Content-Type: application/json` și Origin same-host dacă există (`:52-64`); timeout 25 s (`:65`). Envelope `{ok,…}`; erori 400/401/403/405/503. `[COD]`

| Metodă | Rută | Contract | Transport |
|---|---|---|---|
| GET | `/api/wss/status` | `{ok, status, settings, trace}`; `settings` și `trace` populate doar pentru admin (`:69-86`) | L+W |
| PUT | `/api/wss/settings` | map de setări `wisemed-ws` ≤ 32 KiB → `SaveSettings` (rescrie YAML-ul aplicației) | L+W |
| POST | `/api/wss/control` | `{action:"disconnect"\|"reconnect"}` | L+W |
| POST | `/api/wss/debug` | `{action, connection_id, text}` → `device.ping`/`ws.reconnect`/`debug.message`/listări către peer | **L** |
| POST | `/api/wss/open` | `{equipment_id}` → cere `control.ticket` la WSM; răspunde `{ok, url:"https://<wsm>/control/?equipment_id=…&opener_origin=…", ticket}` | **L** |
| POST | `/api/wss/bridge` | `{connection_id, method, path, …}` ≤ 6 MiB → `api.request` către alt peer, cu JWT-ul readerului | **L** |

### 4.4 Debug

| Metodă | Rută | Handler | Auth |
|---|---|---|---|
| GET | `/api/debug/tests` | `R/modules/localhttp/debug_replay.go:36` | sesiune + `UserType <= 0` (`:21-34`) |
| POST | `/api/debug/tests/run` | `debug_replay.go:55` | idem; rulează scripturi printr-un listener loopback (simulare, nu hardware) |

---

## 5. Module de laborator

Rute înregistrate direct de module, **toate fără autentificare** (ocolesc `requireSession`, fiindcă sunt montate separat pe mux). `[COD]` (03 §9.2)

| Metodă | Rută | Handler | Efecte | Transport |
|---|---|---|---|---|
| GET | `/api/settings/astm-specimens` | `R/modules/astmspecimen/module.go:24,33-63` | citește `protocol-astm.specimen_code_*` | L+W |
| **PUT** | `/api/settings/astm-specimens` | idem | **rescrie `config.yaml` fără sesiune** | L+W |
| GET | `/api/protocol/astm/status` | `R/modules/protocols/astm/module.go:167` | expune host/port, stare conexiune | L+W |
| GET | `/settings/protocol/astm` | `astm/module.go:168` | pagină | L |
| GET | `/api/protocol/horiba-abx-pentra400/query-status` | `R/modules/protocols/horibaabxpentra400/module.go:90` | starea interogărilor per ID de probă | L+W |
| GET | `/protocol/horiba-abx-pentra400/query` | `:91` | pagină | L |
| GET | `/api/protocol/labnovation-ld560/settings` | `R/modules/protocols/labnovationld560/module.go:80,298` | | L+W |
| **PUT** | `/api/protocol/labnovation-ld560/settings` | idem | **schimbă modul/host/port și repornește comunicația, fără sesiune** | L+W |
| GET | `/api/protocol/labnovation-ld560/status` | `:79` | | L+W |
| GET | `/settings/protocol/labnovation-ld560` | `:81` | pagină | L |

Protocoalele de analizor (ASTM, Pentra, miniVIDAS, LAURA, Zonci, LD-560, importuri de fișiere) nu sunt API-uri HTTP: sunt descrise în [communication-map.md](communication-map.md) și `_reports/03-laboratory.md §1–§2, §11.1`. Niciun listener de analizor nu are autentificare, TLS sau allow-list `[COD]`.

Module fără rute proprii: `result-sync`, `transport-*`, `protocol-erba-mannheim-laura`, `protocol-biomerieux-minividas`, `protocol-zonci-xl1000i`, `protocol-ir-biotyper` (01 §3). `[COD]`

---

## 6. WSM

Server `wsm-server/`. Hub WSS multi-tenant, exclusiv în memorie. Rute: `S/server.go:131-173`, `S/admin.go:49-69`, `S/admin_ui.go:13-39`. Middleware global: `nosniff`, `Cache-Control: no-store`, HSTS pe TLS; **fără CORS** pe API-urile de date. `[COD]` Suita Go a serverului (autentificare, rutare, admin, SDK, izolare per tenant) a trecut cu 0 teste sărite `[TEST]` (06 §B.1 r.7–8).

### 6.1 WebSocket `/ws`

**Upgrade**: `GET /ws`, handler `handleWS` (`S/server.go:197-377`). Credențial: `Authorization: Bearer <JWT>`, subprotocol `wsm.jwt.<JWT>`, `?token=` doar cu `security.allow_query_token`, sau tichet `wsm.ticket.<ticket>`. Subprotocol negociat `wsm.v1`. Eșec înainte de upgrade: `401 {error:"wsm_<cod>", message}`, `403 origin denied`, `503 connection limit`. `[COD]`

**Envelope** (`S/message.go:5-26`): `type`, `request_id`, `correlation_id`, `connection_id` (setat de server), `target{mode, connection_id(s), equipment_id(s), reader_id(s), client_type, topic}`, `payload`, `timestamp`; `broadcast` este respins.

| `type` (client → server) | Scop | Autorizare (`authorizeMessage`, `S/server.go:387-528`) | Răspuns server |
|---|---|---|---|
| `hello` | primul mesaj, ≤ `hello_timeout_ms` (10 s). `payload{client_type, client_id, reader_id, equipment_id?, user_id?, label?}` (`S/message.go:28-35`) | trebuie să corespundă claim-urilor (`S/auth.go:168-173`) | `hello_ack{connection_id, tenant_id, equipment_id, client_type, client_id, reader_id, subject, expires_at}` (`S/server.go:339`) sau close 1008 |
| `ping` | keepalive aplicativ | — | `pong{server_time}` |
| `list_connections` | discovery | `connections:read` | `connections` (`Hub.Snapshot(tenant)`) |
| `subscribe` / `unsubscribe` | topicuri (≤ 64/conexiune) | `topics:subscribe` | `subscribe_ack` / `unsubscribe_ack` |
| `command` | rutare către peer sau `mode:"server"` | `route:command`; `api.request` cere și `api:invoke`; restricții de rol `reader` (§7) | `command_ack{routed_type, recipients, target, delivery:"queued"}`; pentru `mode:"server"` direct `reply` / `error code=command_failed` |
| `reply` | răspuns la comandă; cere `correlation_id` | `route:reply`; rol `reader`: doar `mode:"connection"` | `command_ack` |
| `event` | publicare | `route:event`; rol `reader`: doar topic `results:<reader_id>` / `logs:<reader_id>` (`S/server.go:475`) | `command_ack` |

Emise doar de server: `presence{event:"connected"|"disconnected", …}` (difuzat tenantului), `error`.

**Ținte** (`mode`): `connection`, `equipment`, `reader` (1:1); `connections`/`equipments`/`readers` (1..100 ID-uri), `all`, `client_type`, `topic` — cele multiple cer `route:broadcast`; `server` — doar `command`. `[COD]` (`S/server.go:494-524`, `S/hub.go:121-168`).

**Comenzi `mode:"server"`** (`S/server.go:418-436`):

| `payload.command` | Scope | Efect | Referință |
|---|---|---|---|
| `control.ticket` `{equipment_id}` | `api:invoke` + `route:command` | tichet unic, 1 min, pentru o sesiune de control ≤ 5 min; `reply{ticket, equipment_id, expires_at}` | `S/control.go:24-70` |
| `equipment.status`, `equipment.list` | `connections:read` | snapshot | `S/equipment.go` |
| `server.status` | `server:status` | statistici hub | — |
| `wisemed.ensure_equipment_online`, `wisemed.fetch_file_for_analyzer` | `wisemed:proxy` | proxy către upstreamul WiseMED al tenantului (URL-uri fixe), timeout 20 s, răspuns ≤ 4 MiB | `S/wisemed.go:55-71` |

**Semantica `command_ack`**: confirmă doar intrarea în coada de trimitere a `recipients` conexiuni; nu recepția, nu execuția. `recipients:0` = fără destinatar eligibil în tenant sau coadă plină. `[COD]` (`S/server.go:561-563`, `S/hub.go:195-217`).

**Limite** (implicite, `C/config.go:131-172`): mesaj 1 MiB (max 16 MiB); 50 msg/s, burst 100 → close 1008; coadă de trimitere 128 → mesaj aruncat și conexiunea lentă închisă; 2000 conexiuni global / 200 per tenant; `read_timeout` 60 s prelungit doar de Pong; lease strict = `exp` al JWT. `[COD]`

**Timeout / anulare / retry**: fără mesaj de anulare; apelantul care renunță doar își șterge intrarea locală `pending`, execuția remote continuă (`S/sdk/wsm.js:145`, `WS:568-588`). `[COD]` Tabel complet: `_reports/02-wsm-security.md §13`.

**Teste**: `S/hub_test.go:14 TestHubTenantIsolationEveryRoute`, `S/server_test.go:168,547,570`, `S/http_commands_test.go:68` — parte din suita trecută `[TEST]` (06 §B.1 r.7); integrare reală reader↔WSM (apel API, tichet, reconectare după ~30 s) `[TEST]` (r.10).

### 6.2 HTTP de date

| Metodă | Rută | Handler | Auth / autorizare | Răspuns / erori |
|---|---|---|---|---|
| GET | `/healthz` | inline `S/server.go:164` | publică | `{status:"ok",service:"wsm-server"}` |
| GET | `/api/connections[?client_type=]` | `connections` `S/server.go:174` | Bearer; `connections:read`; Origin | `{tenant_id, connections, count, stats}`; 401/403 |
| GET | `/api/debug/state` | idem (alias) | idem | idem |
| GET | `/api/equipment/{id}` | `equipmentHTTP` `S/equipment.go:21` | Bearer; `connections:read`; Origin | snapshot echipament |
| POST | `/api/connections/{id}/commands` | `commandHTTP` `S/http_commands.go:17-193` | Bearer; rol `service` sau `browser`; Origin; apoi `authorizeMessage` | vezi mai jos |
| GET | `/sdk/wsm.js`, `/sdk/siui-bridge.js` | `serveBrowserSDK` `S/browser_sdk.go:11` | publică; `Access-Control-Allow-Origin: *` | JS embed |
| GET | `/control`, `/control/*`, `/app.js`, `/styles.css`, `/wss-remote.js`, `/docmed-forms.js`, `/siui-invoice.js` | `controlAsset` `S/control.go:93-126` | publică; allowlist de fișiere din `server.control_ui_dir`; 503 dacă directorul lipsește | UI de control remote |

`wsm-server/web/` (UI vechi cu `stats.get`, `orders.*`) nu este servit de nicio rută. `[COD]`

**Wrapperul `POST /api/connections/{id}/commands`** `[COD]`:

- Body `{command, args, timeout_ms}`; câmpuri necunoscute → 400; ≤ `max_message_bytes` → 413.
- Creează un participant temporar în hub (numărat în limite, fără `presence`), îngheață ținta `{id}` din același tenant, generează `request_id`, acceptă numai `reply` cu `correlation_id == request_id` și `connection_id == {id}`.
- `timeout_ms` implicit 135 000, interval 1..300 000, plafonat la `exp`.
- **HTTP 200** = a sosit un `reply` autentic; **nu** garantează succesul API-ului remote (`response.status` poate fi 4xx/5xx; `response.ok=false` e refuz de aplicație). Corp: `{request_id, connection_id, response}`.
- POST nu este niciodată retrimis de clientul PHP; închiderea cererii de către apelant nu anulează execuția remote (`:186-190`).

| HTTP | `error` | Semnificație |
|---|---|---|
| 400 | `invalid_json`, `invalid_command_or_timeout` | |
| 401 | `wsm_*`, `credentials_expired_or_reloaded` | JWT invalid sau reload/expirare între autentificare și admitere |
| 403 | `forbidden` | rol, Origin sau `authorizeMessage` |
| 404 | `client_unavailable` | ID inexistent / alt tenant |
| 413 | `request_too_large` | |
| 502 | `client_disconnected_execution_unknown` | peer deconectat după trimitere |
| 503 | `connection_limit`, `registration_failed`, `client_unavailable` (ACK `recipients:0`), `request_interrupted_execution_unknown` | |
| 504 | `timeout_execution_unknown` | |

> **Distincție obligatorie** (rezoluție verificată în cod): codurile `*_execution_unknown` sunt erori ale **wrapperului HTTP WSM** (`S/http_commands.go`). Ele nu au legătură cu starea de job SIUI **`unknown`** (§8.4). Șirul `execution_unknown` nu există în modulele `siui*` (04 §6.4). `[COD]`

### 6.3 Consola admin

Activă doar cu `admin.enabled`; cere `device_keys_file` și origini HTTPS; potrivire strictă `Host`+`Origin`; mutațiile cer `Content-Type: application/json`, `Origin` egal și `X-CSRF-Token`; body ≤ 8 KiB. `[COD]` (`S/admin.go:96-104,341`, `C/devices.go:99-131`)

| Metodă | Rută | Handler | Auth |
|---|---|---|---|
| GET | `/admin`, `/admin/`, `/admin/app.js`, `/admin/styles.css` | `registerAdminUI` (`S/admin_ui.go:13-39`) | Host/Origin configurat; CSP strict |
| GET | `/admin/api/tenants` | `adminTenants` `S/admin.go:125-138` | **fără sesiune**: ID-urile tenanturilor cu upstream WiseMED |
| POST | `/admin/api/login` | `adminLogin` `S/admin.go:169` | same-origin JSON; rate limit 10/5 min/IP, 60/min global; validare `PUT <base_url>/administrative/login` la WiseMED; acceptă doar `user_type == -1` |
| GET | `/admin/api/session` | `adminSessionStatus` | cookie opțional |
| POST | `/admin/api/logout` | `adminLogout` | sesiune + CSRF |
| GET | `/admin/api/logs/dates`, `/admin/api/logs` | `S/admin_logs.go` | sesiune; filtrare pe tenant; paginare cu cursor în bytes, ≤ 256 KiB/pagină |
| GET / PATCH | `/admin/api/tenant` | `adminTenantInfo` / `adminUpdateTenant` | sesiune (+CSRF) — `issuer`, `base_url`, `api_key` |
| PATCH | `/admin/api/tenant/origins` | `adminOrigins` | sesiune + CSRF |
| GET / POST | `/admin/api/browsers` | `adminBrowsers` / `adminCreateBrowser` | sesiune (+CSRF) |
| POST | `/admin/api/browsers/key` | `adminRevealBrowser` | sesiune + CSRF; **întoarce secretul în clar** |
| PATCH / DELETE | `/admin/api/browsers/{keyID}` | `adminChangeBrowser` | sesiune + CSRF |
| GET / POST | `/admin/api/devices` | `adminDevices` / `adminCreateDevice` | sesiune (+CSRF) |
| POST | `/admin/api/devices/key` | `adminRevealDevice` | sesiune + CSRF; **întoarce secretul în clar** |
| PATCH / DELETE | `/admin/api/devices/{keyID}` | `adminUpdateDevice` / `adminDeleteDevice` | sesiune + CSRF |

Efecte: scriere atomică în `security.device_keys_file`; ștergerea/actualizarea unui device închide doar conexiunile `(tenant, subject)`; modificările de cheie browser / origini / setări tenant apelează `revokeTenantAccess` și incrementează `authEpoch` **global** (`S/admin_access.go:347-363`). Coduri: 401/403/409/429/500. `[COD]`

Fără API de rețea: `wsmctl` (`keygen`, `token`) este utilitar offline (`wsm-server/cmd/wsmctl/main.go`); reload prin `SIGHUP` (doar Unix). `[COD]`

### 6.4 SDK JS și client PHP

**`WSMServer`** — `S/sdk/wsm.js`, servit la `/sdk/wsm.js`. `[COD]`

- Constructor `WSMServer(url, token, options)`; opțiuni `timeout` 35 000, `connectTimeout` 10 000, `reconnectDelay` 30 000, `autoReconnect`, `autoConnect`, `tokenProvider`.
- Metode: `connect`, `disconnect`, `reconnect`, `ping`, `serverCommand`, `serverStatus`, `equipmentStatus`, `controlTicket`, `listClients`, `getClientsByType`, `command`, `callAPI`, `subscribe`, `unsubscribe`, `send`, `reply`.
- Callback-uri: `onConnect`, `onDisconnect`, `onError`, `onTokenRequired`, `onEvent`, `onCommand`, `onReply`, `onPresence`, `onMessage`.
- Acceptă doar JWT cu `role=browser` (`:62`); `command()` țintește mereu `mode:"connection"` și așteaptă `reply` de la exact acel peer (`:126`); `callAPI` aruncă `http_error` la status non-2xx; max 256 cereri în așteptare; reconectarea nu retrimite nimic.

**`SIUIBridge`** — `S/sdk/siui-bridge.js`, servit la `/sdk/siui-bridge.js`: `executeOperation`, `submitOperation`, `waitCall`, `getCall`, `completeCall`, `downloadCallFile`, `getCallFileChunk` (`:295-326`), `retryCallWMAPI` (`:293`), `getValidation`, `waitValidation`, `downloadDocMedAttachment` + metode generate pentru cele 63 de operații; catalog `SIUIBridge.endpoints` (`:98`). Lipsește `report/package` — vezi §18. `[COD]`/`[TEST]`

**`WSMHttpClient`** — `wsm-server/examples/php/WSMHttpClient.php`: `listClients`, `firstClient`, `sendCommand`, `callAPI`, `setTimeouts`, `setReconnectOptions`; excepție `WSMHttpException{errorCode, requestId, executionUnknown, httpStatus}`. Mapare 1:1 pe `GET /api/connections` și `POST /api/connections/{id}/commands`; retry doar pe GET; `callAPI` **nu** aruncă pe status remote non-2xx (diferit de SDK-ul JS). `[COD]`; test `S/php_http_client_test.go:18` în suita trecută `[TEST]`.

Exemplele `wsm-server/examples/browser/*.php|js`, `examples/php/siui-server.php`, `examples/reader/main.go` sunt ilustrative; implementarea din backendul WiseMED este **externă** `[NEC]`.

---

## 7. Comenzile de dispatcher WS

Tratate de clientul din readere (`handleCommandContext`, `WS:506-551`; dispatcher `WS:54-65`). Vin ca `command` cu `payload.command` + `payload.args`; răspunsul este `reply` cu `correlation_id = request_id`, `target = {mode:"connection", connection_id: sender_connection_id}`. `[COD]`

| `payload.command` | Handler | Condiție în reader | Autorizare la WSM | Răspuns |
|---|---|---|---|---|
| `api.request` | `apibridge.Invoke` (`WS:512-533`, `AB:51-162`) | `sender_scopes` ∋ `api:invoke` | `route:command` + `api:invoke` | `{kind:"api.response", …}` (§1.2) |
| `reader.status`, `ws.status`, `wisemed.ws.status` | `Module.Status()` (`WS:554`) | niciuna | `route:command` | stare client WSS |
| `device.ping` | `WS:556` | niciuna | `route:command` | `{ok, equipment_id, time}` |
| `ws.reconnect` | `WS:558`, `:476-479` | niciuna | `route:command` | răspunde, apoi se reconectează |
| `debug.message` | `WS:560` | niciuna | `route:command` | ecou `received` |
| `barcode.print`, `barcode/print`, `print_barcode` | `barcodeprinter.HandleWSAction` (`R/modules/barcodeprinter/module.go:100-117`) | doar aplicațiile cu modulul | `route:command` | `{"printed":true}` |
| `barcode.printers`, `barcode/list_printers` | idem `:118` | idem | `route:command` | `{"printers":[…]}` |
| `esignature.health`, `.devices`, `.start`, `.retry`, `.confirm`, `.cancel` | `signingpad.HandleWSAction` (`R/modules/signingpad/remote.go:29-81`) | `session_id` 16–128 caractere (exceptând `health`/`devices`) | `route:command` | `{ok,…}`; `confirm` întoarce imaginea |
| orice altceva | — | — | — | `{ok:false,error:{message:"unknown action"}}` |

Reguli importante `[COD]`:

- **Niciun handler de dispatcher nu verifică scope-uri.** Doar `api.request` este protejat suplimentar (`api:invoke`). Pentru expeditori `browser`/`service`, WSM nu filtrează numele comenzii: orice `payload.command` trece cu `route:command` (`S/server.go:437-442`).
- Un expeditor cu rol `reader` poate trimite doar 1:1 și doar `api.request` (+`api:invoke`) sau `device.ping`/`ws.reconnect`/`debug.message` (+`devices:debug`) (`S/server.go:443-478`).
- Execuție serială, un worker per sesiune, coadă de 16 comenzi (plină ⇒ sesiunea se închide); coadă de trimitere 128 (plină ⇒ reply pierdut) (`WS:428-455,627-645`).
- Comenzile `orders.*`, `analytes.*`, `config.*`, `stats.*`, `results.activate`, `logs.*` **nu au handler** în `readersv3` — vezi §18.

Mesaje emise de reader: `reply`; `command mode:"server"` (`control.ticket`, `wisemed.ensure_equipment_online`, `wisemed.fetch_file_for_analyzer`); `list_connections`, `ping`; `command` către peer (`device.ping`/`ws.reconnect`/`debug.message`, `api.request` prin `/api/wss/bridge`); `event` către o conexiune (`R/modules/ws/events.go:8-13`, folosit doar de `AI/events.go:28`). Nu există emitere pe topicurile `results:<reader_id>` / `logs:<reader_id>`. `[COD]`

> **Rezoluție** (verificată în cod): WiseMED AI se conectează la WSM cu rol `reader` (`WS:707-712` — doar `siui-bridge`/`siui-bridge-history` primesc `service`). WSM respinge `event` de la rolul `reader` dacă ținta nu este topicul propriu (`S/server.go:475`). Deci evenimentele `wisemed-ai.{start,progress,result}` trimise cu `mode:"connection"` sunt **refuzate conform codului**; neverificat la runtime. `[COD]` pentru ambele verigi, `[INF]` pentru efectul cap-coadă.

---

## 8. SIUI bridge

Aplicație `siui-bridge`, modul `siui` (`SI/`). Prefixuri: `/api/siui/` (`SI/module.go:135`), `/api/siui-history-sync/` (`SI/history_sync.go:60`), ambele prin `m.authenticate`; pagina `/history-sync-ui` fără middleware (`SI/history_sync.go:61-64`). Port local implicit `127.0.0.1:19112` (HTTPS). `[COD]`

**Autentificare** (`authenticate`, `SI/auth.go:27-49`): (1) principal WSS → sesiune; (2) `Authorization: Bearer` JWT local SIUI — doar pe TLS; (3) cookie local. **Identitate** (`SI/module.go:249-277`): `owner = "local:"+subject` / `owner` semnat din JWT / `wsm:"<tenant>":"<subject>"`. Cererile non-WSS și non-GET verifică `Origin` (403 `origin rejected`). Legendă: **U** = orice identitate autentificată; **A** = administrator (local `UserType <= 0`, JWT `admin:true`, sau scope `api:admin`); „proprietar” = același `owner`.

**Corp**: `Content-Type: application/json` obligatoriu, `DisallowUnknownFields`, un singur obiect; limită 2 MiB (`SI/module.go:232-248`); `sendReport` 6×10 MiB + 1 MiB; DocMED 64 MiB (`SI/service_jobs.go:184-191`). Rută necunoscută → **405**, nu 404 (`SI/module.go:444-446`). `[COD]`

### 8.1 Rute

| Metodă + cale (prefix `/api/siui/`) | Handler | Auth | Corp / parametri | Coduri |
|---|---|---|---|---|
| `POST {general,clinice,facturare,docmed}/{op}` | `serviceHTTP` `SI/service_jobs.go:164-348` | U; A dacă `mutating` | `{operation_id, parameters{…}, fields? (doar docmed), callwmapi?}` | **202** rezervat, **200** `{duplicate:true}`, 400, 403, 404, 405, 409, 500, 503 |
| `GET calls/{id}` | `SI/service_jobs.go:142-162` | U, proprietar | — | 200 `{call}`, 404, 500, 503 |
| `GET calls/{id}/file` | `serviceFileHTTP` `SI/service_jobs.go:493-588` | U, proprietar | `offset`, `unzip`, `entry`, `format`, `validateXSD`, `schema`, `docmed_attachment` | 200 fragment base64 ≤ 256 KiB, 400, 404, 409, 422, 500 |
| `POST calls/{id}/callwmapi/retry` | `SI/service_jobs.go:123-141` | U, proprietar | — | 202, 404, 409, 500 — **nu reapelează CNAS** |
| `GET general/ocsp` | `SI/service_jobs.go:82-94` | A | — | 200 `{debug}` |
| `POST general/ocsp` | `SI/service_jobs.go:95-115` | A | `{}` | 200 — **include tokenul sesiunii CNAS în clar** (`token`); 400, 403, 409, 502, 503 |
| `POST auth/token` | `tokenHTTP` `SI/auth.go:92-109` | cookie sau WSS; **nu** JWT (403) | `{}` | 200 `{access_token, token_type, expires_in:900}`; 400 fără HTTPS |
| `GET catalog` | `SI/module.go:298-302` | U | — | 200 `{operations, insurance_houses, docmed_defaults}` |
| `GET openapi.json` | `SI/module.go:303-304` | U | — | 200 OpenAPI 3.0.3 generat |
| `GET status` | `SI/module.go:311-315` | U | — | 200 `{platform, native_supported, configured, pending, report_types}` |
| `GET settings` / `PUT settings` | `settingsHTTP` `SI/module.go:461-639` | A | PUT `fiscal_code, cas_code, licence?, certificate_store, certificate_thumbprint, base_url, pe_base_url?, einvoice_base_url?, docmed_base_url?, allow_*?` | 200, 400, 403, 405, 409 (operații active), 500 |
| `GET certificates?store=` | `SI/module.go:344-364` | A | — | 200, 403, 503 |
| `POST certificates/refresh` | `SI/module.go:338-343` | **U** (fără admin) | — | 200, 503 |
| `GET token-pin` / `PUT token-pin` | `tokenPINHTTP` `SI/token_pin_http.go:8-58` | A | PUT `{pin, forget, manual}` | 200 (PIN-ul nu e returnat niciodată), 400, 403, 405, 409, 500, 503 |
| `POST ocsp-test` | `ocspHTTP` `SI/passive.go:39-57` | U | `{}` | 200, 400, 409, 502, 503 |
| `POST insured` | `insuredHTTP` `SI/insured.go:101-158` | U | `{mode:"local"\|"cnas", cnp, date}` | 200, 400, 409, 500, 502, 503; antet `X-SIUI-Call-ID` — sincron, 25 s |
| `POST personalization` | `personalizationHTTP` `SI/passive.go:201-240` | A | `{start, stop}` (YYYY-MM-DD) | 200 `{file{data_base64…}}`, 400, 403, 409, 500, 502 — sincron, 25 s |
| `POST validations` | `SI/module.go:365-411` | U | `{operation_id, xml}` | 202, 200 duplicat, 400, 409, 503 |
| `GET validations`, `GET validations/{id}` | `SI/module.go:412-443` | U, proprietar | — | 200 (ultimele 50), 404, 500 |
| `GET invoice/model` | `invoiceHTTP` `SI/invoice.go:280-287` | U | — | 200 |
| `POST invoice/preview` | `SI/invoice.go:292-332` | U | model factură | 200, 400, 500 |
| `POST invoice/package` | `SI/invoice.go:333-378` | A | model factură | 200, 400, 403, 413, 500, 502, 503 |
| `POST report/package` | `reportPackageHTTP` `SI/report.go:45-105`; rutat la `SI/module.go:307` | A | `{filename, report_xml}` | 200, 400, 403, 405, 413, 500, 502, 503 |
| `GET operations` | `listOperations` `SI/operations_history.go:222-283` | U (propriile) / A (toate) | `from, to, operation_id, operation, status, offset` | 200 pagină de 100, 400, 500, 503 |
| `GET operations/{id}` | `SI/operations_history.go:139-221` | proprietar / A | — | 200 `{call}`, 404, 500 |
| `GET operations/{id}/request?name=&offset=` | `SI/operations_history.go:169-193` | proprietar / A | — | 200 binar sau fragment base64; servește **parametrii cererii** păstrați în outbox |
| `GET operations/{id}/file` | `SI/operations_history.go:148-156` | proprietar / A | ca `calls/{id}/file` | idem |
| `GET operations/{id}/history`, `/history/request`, `/history/{artifact}` | `operationHistoryHTTP` `SI/operations_history.go:287-415` | proprietar / A | `offset?` | 200, 400, 404, 409, 500, 502, 503 |
| `GET dashboard?from=&to=` | `dashboardHTTP` `SI/dashboard.go:117-340` | U (propriile) / A | interval ≤ 31 zile | 200, 400, 405, 500, 503 |
| `POST dashboard/checks` | `dashboardChecksHTTP` `SI/dashboard.go:60-116` | A | — | 200, 403, 405, 409 |
| `GET downloads/storage` / `POST downloads/storage` | `downloadStorageHTTP` `SI/download_storage.go:65-176` | A | POST `{confirm:true}` | 200, 400, 403, 405, 409, 500, 503 |

`/api/siui-history-sync/*` (toate A; `SI/history_sync.go:66-182`): `GET settings` (token golit), `PUT settings`, `GET status`, `POST retry`, `POST test`; coduri 200/400/403/404/500/502/503.

Toate `[COD]`. Transport: L+W, cu limitările adaptorului — cereri > 4 MiB (`sendReport`, DocMED) nu pot trece prin WSS `[INF]`; descărcările merg fragmentat (`offset`, 256 KiB). `POST auth/token` prin WSS emite un JWT local cu `owner` WSM semnat.

### 8.2 Semantica `POST {grup}/{op}`

- **202** = cererea a fost validată local, rezervată durabil în SQLite (`siui_calls` + outbox + sursă + callback opțional, o tranzacție) și pusă în coada din memorie. **Nu s-a trimis nimic la CNAS.** `[COD]` (`SI/service_jobs.go:302-348`)
- **Deduplicare**: `UNIQUE(owner, operation_id)`; `operation_id` ales de client, tipar `^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$` (`SI/jobs.go:17`). Același digest → 200 `{duplicate:true, call}`; digest diferit → 409. Digestul include configurația curentă ⇒ după orice schimbare de setări CNAS, repetarea dă 409 `[INF]`.
- `request_id`/`correlation_id` din plicul WSS **nu** sunt persistate și nu participă la deduplicare. `[COD]`
- **409** `CNAS token or queue busy`: verificare sincronă în curs sau canal plin (20). Nimic rezervat.
- Fără rută de anulare a unui job. Buget 120 s / job. `[COD]`
- `callwmapi` (opțional): `{apipath, method:"POST|PUT", transform_path[], body{…, transform_result[]}, file{…}, lt?}` (`SI/callback.go:30-37`); livrare ulterioară către WiseMED cu `Idempotency-Key: siui-<call id>`.

### 8.3 Cele 63 de operații (41 + 22 DocMED)

Sursa autoritară: `SI/assets/operations.json` (41) și `SI/assets/docmed_operations.json` (22), concatenate de `operations()` (`SI/operations.go:40-45`). Catalogul, dispatcherul (`findOperation`) și OpenAPI derivă din aceeași funcție, deci nu pot diverge între ele. `[COD]` Rută: `POST /api/siui/<grup>/<operație>`. **M** = `mutating` (cere administrator); **D** = descărcare (CNAS întoarce URL + dimensiune, bridge-ul aduce fișierul). Toate sunt asincrone (202 + polling `calls/{id}`).

| # | Operație | Serviciu CNAS | Grup | M | D |
|---|---|---|---|---|---|
| 1 | `getCatalogues` | SiuiWS | general | – | D |
| 2 | `getProviderInfo` | SiuiWS | general | – | D |
| 3 | `getProviderInfoForPhysician` | SiuiWS | general | – | D |
| 4 | `sendReport` | SiuiWS | general | M | – |
| 5 | `getReportFeedback` | SiuiWS | general | – | D |
| 6 | `getRefund` | SiuiWS | general | – | D |
| 7 | `getRefundForPhysician` | SiuiWS | general | – | D |
| 8 | `getRefundForInvoice` | SiuiWS | general | – | D |
| 9 | `generatePrescriptionSeries` | SiuiWS | general | M | D |
| 10 | `getPrescriptionSeriesInfo` | SiuiWS | general | – | D |
| 11 | `getPhysicianCatalogues` | SiuiWS | general | – | D |
| 12 | `getInsured` | SiuiInsuredWS | general | – | – |
| 13 | `getInsuredByCID` | SiuiInsuredWS | general | – | – |
| 14 | `getPersonInfo` | SiuiInsuredWS | general | – | – |
| 15 | `validateReport` (CLIN/PARA) | SiuiValidateWS | **general** | M | – |
| 16 | `getPrescription` | SiuiValidateWS | clinice | – | – |
| 17 | `getClinicReferral` | SiuiValidateWS | clinice | – | – |
| 18 | `getLabReferral` | SiuiValidateWS | clinice | – | – |
| 19 | `validatePrescription` | SiuiValidateWS | clinice | M | – |
| 20 | `validateClinicReferral` | SiuiValidateWS | clinice | M | – |
| 21 | `validateLabReferral` | SiuiValidateWS | clinice | M | – |
| 22 | `validateSickLeave` | SiuiValidateWS | clinice | M | – |
| 23 | `processIssuedPrescription` | EPrescriptionWS | clinice | M | – |
| 24 | `cancelReleasedPrescription` | EPrescriptionWS | clinice | M | – |
| 25 | `updateInvoices` | EPrescriptionWS | clinice | M | – |
| 26 | `getReleasedPrescription` | EPrescriptionWS | clinice | – | – |
| 27 | `processPrescribedPrescription` | EPrescriptionWS | clinice | M | – |
| 28 | `getPrescribedPrescription` | EPrescriptionWS | clinice | – | – |
| 29 | `cancelPrescribedPrescription` | EPrescriptionWS | clinice | M | – |
| 30 | `downloadNotReportedPrescriptionsReport` | EPrescriptionWS | clinice | – | – |
| 31 | `getStatusForPrescriptions` | EPrescriptionWS | clinice | – | – |
| 32 | `getPrescriptionsSeriesNo` | EPrescriptionWS | clinice | – | – |
| 33 | `getPrescribedPrescriptionsForCid` | EPrescriptionWS | clinice | – | – |
| 34 | `processHospitalPrescription` | EPrescriptionWS | clinice | M | – |
| 35 | `cancelReleasedHospitalPrescription` | EPrescriptionWS | clinice | M | – |
| 36 | `getReleasedHospitalPrescription` | EPrescriptionWS | clinice | – | – |
| 37 | `sendEInvoice` | SiuiEInvoiceWS | facturare | M | – |
| 38 | `getEInvoice` | SiuiEInvoiceWS | facturare | – | D |
| 39 | `getEInvoiceId` | SiuiEInvoiceWS | facturare | – | – |
| 40 | `getEInvoiceClearingNote` | SiuiEInvoiceWS | facturare | – | D |
| 41 | `cancelEInvoice` | SiuiEInvoiceWS | facturare | M | – |
| 42–56 | `MedicalLeave`, `MedicalLetter`, `PhysicianElectronicPrescription`, `RecoveryMedicalPlan`, `LaboratoryMFReferral`, `LaboratoryAGReferral`, `LaboratoryCTReferral`, `LaboratorySGReferral`, `PalliativeRecommendation`, `MedicalDeviceRecommendation`, `HomeCareMedicalPlan`, `ClinicalReferral`, `LaboratoryRMNReferral`, `LaboratoryReferral`, `HomeCareRecommendation` | DocMedService (`/loader/ws`) | docmed | M | – |
| 57 | `ConsumeDocument` | DocStatusChangerService (`/status/ws`) | docmed | M | – |
| 58 | `UnconsumeDocument` | DocStatusChangerService | docmed | M | – |
| 59 | `GetDocumentStatus` | DocStatusChangerService | docmed | – | – |
| 60 | `GenDocumentSeries` | DocumentSeries (`/seq/ws`) | docmed | M | – |
| 61 | `GetDocumentSeries` | DocumentSeries | docmed | – | – |
| 62 | `GetMedicalDocumentXml` | MedicalDocumentDownloader (`/download/ws`) | docmed | – | – |
| 63 | `GetMedicalDocument` | MedicalDocumentDownloader | docmed | – | – |

Totaluri: 34 `mutating` (16 + 18 DocMED), 29 citiri, 12 `download`. `[COD]` Parametrii fiecărei operații: vezi JSON-urile sursă și `GET /api/siui/catalog` (nu sunt reproduși aici). DocMED acceptă `parameters:{requestXml}` **sau** `fields:{…}` (formular din `SI/assets/docmed_forms.json`), nu ambele (`SI/service_jobs.go:196-211`).

Operații prezente în WSDL-urile încorporate dar **neexpuse** (10): `validateEnlisted`, `validatePharmacyDrugs` (SiuiValidateWS); `getDecisions`, `getPartnerInfo`, `getRegisterFeedback`, `getRegisterFeedbackAggregated`, `getSupplierFeedback`, `isPhysicianOnContract`, `registerApplicationInstance`, `sendSupplierData` (SiuiWS). `[COD]`

### 8.4 Stări și interpretare (rezumat)

`siui_calls.status`: `queued` → `running` → `completed` | `failed` | `unknown`; la repornire `queued`/`running` → `unknown`; nu există reconciliere automată și nimic nu este reexecutat. `[COD]` (`SI/service_jobs.go:48,390-438`)

- `completed` = SOAP valid primit (și fișier descărcat); **nu** înseamnă acceptare de business la operațiile clasice — apelantul trebuie să citească `result.values`. La DocMED implică și XSD valid + absența mesajelor blocante.
- `unknown` = o scriere ar fi putut ajunge la CNAS. Repetarea aceluiași `operation_id` întoarce jobul existent; pentru o nouă încercare trebuie ales conștient un `operation_id` nou.

Detalii: [data-and-state-machines.md](data-and-state-machines.md), `_reports/04-cnas-siui.md §5.5, §6`. Teste (în suita `readersv3` trecută, cu 2 teste `siui` sărite — token conectat, TLS CNAS) `[TEST]` la nivel de suită (06 §B.1 r.3): `TestServiceRestartNeverReplaysPendingCalls`, `TestNoRetryAfterAmbiguousWrite`, `TestJobIdempotencyAndRestart`, `TestWMCallbackQueueAndIdempotency`, `TestCatalogMatchesBundledWSDL`.

---

## 9. SIUI Bridge History

Aplicație `siui-bridge-history`, modul `siui-history` (`H/`). Prefix `/api/history/` (`H/module.go:79`) prin `debugAuthenticate` → `authenticate` (`H/debug.go:67`, `H/module.go:111-163`); pagina `/history-ui` fără middleware (`H/module.go:80-83`). `[COD]`

**Autentificare**: (a) bridge — `Authorization: Bearer <token>`; se compară `sha256(token)` în timp constant cu `bridge_keys[].hash`; tokenul fixează `(tenant, bridge)` (`H/module.go:127-143`); (b) admin — sesiune locală cu admin, sau principal WSS **numai dacă** `TenantID` = tenantul configurat (`H/module.go:148-162`).

| Metodă + cale (prefix `/api/history/`) | Cine | Rol | Handler |
|---|---|---|---|
| `GET identity` | bridge / admin | `{tenant, bridge}` | `H/module.go:256-259` |
| `POST calls` | bridge (admin doar cu `?bridge=`) | manifest → `Progress{status, offsets}`; `INSERT IGNORE`; digest diferit → 409 | `H/calls.go:72-112` |
| `PUT calls/{id}/files/{name}?offset=` | bridge | fragment ≤ 256 KiB în staging; offset ≠ dimensiunea curentă → 409 | `H/calls.go:358-394` |
| `POST calls/{id}/commit` | bridge | verifică dimensiune + SHA-256 per artefact, mută, indexează, marchează `archived` | `H/calls.go:282-336` |
| `GET calls` | bridge (ale lui) / admin | filtre `from, to, time_field, operation, operation_id, call_id, status, operation_status, pid, cid, bridge, offset` | `H/calls.go:113-203` |
| `GET calls/{id}` | idem | manifest + detalii + progres | `H/calls.go:268-281` |
| `GET calls/{id}/request` | idem | previzualizarea parametrilor din `record-gzip` | `H/calls.go:236-267` |
| `GET calls/{id}/files/{name}?offset=&format=json` | idem | fragment 256 KiB, binar sau base64 | `H/calls.go:395-426` |
| `GET/PUT settings`, `GET/POST/DELETE keys`, `POST test-storage`, `POST test-db`, `GET dashboard`, `GET debug/requests` | admin | administrare; `POST keys {bridge}` întoarce tokenul o singură dată | `H/module.go:233-378` |

Manifest: `history.Record` (`R/shared/history/protocol.go:27-39`), ≤ 8 artefacte. Upload reluabil de la offsetul raportat de server; bridge-ul consideră arhivarea confirmată doar cu `success` și `status == "archived"` la commit. Transport: bridge → History este HTTPS direct (nu prin WSM); accesul admin este L+W (timeout 310 s pentru `commit`). `[COD]` 2 teste `siuihistory` sunt sărite (cer MySQL) — 06 §B.1 r.3.

---

## 10. WiseMED SIUI Data

Aplicație `wisemed-siui-data`, modul `siui-data` (`D/`), PostgreSQL. `[COD]`

**Autentificare**: cookie local sau principal WSS (`D/module.go:246-254`); JWT propriu HS256, `iss=wisemed-siui-data`, `aud=siui-data-api`, ≤ 15 min, doar pe HTTPS (`D/module.go:219-240`); chei per client `client_jwt_secrets[cui]` selectate prin `kid=<cui>`, limitate la GET/HEAD pe serviciile țintite ale propriului CUI (`D/clients_auth.go:51-79`).

| Cale (prefix `/api/siui-data/`) | Metodă | Acces | Handler |
|---|---|---|---|
| `auth/token` | POST | doar browser local admin, HTTPS | `D/clients_auth.go:95-141` |
| `targeted_services/{cui}`, `targeted-simple-services/{cui}` | GET/HEAD | JWT de CUI (propriu), JWT global admin sau admin local | `D/clients_targeted.go` |
| `settings` | GET/PUT | admin | `D/settings.go:59-135` |
| `bridge/connections` | GET | autentificat | `D/bridge_sync.go:67-102` |
| `bridge/sync` | POST | admin — `{partnerCategory, connection_id?}` → **202** `{importId, statusUrl}`; max 2 lucrări simultane (409) | `D/bridge_sync.go:103-222` |
| `categories` | GET | autentificat | `D/categories.go` |
| `upload`, `import` (și `/upload` la rădăcină) | POST | admin, HTTPS direct, multipart | `D/upload.go:25-250` |
| `versions`, `status`, `imports/{id}` | GET; DELETE la `imports/{id}` | autentificat (DELETE: admin, doar `ready/failed/unchanged` neactive) | `D/query.go:33-110`, `D/versions.go` |
| `catalogues`, `services`, `simple-services`, `services/{cod}`, `records/{tip}` | GET/HEAD | autentificat | `D/query.go:254-540` |
| `drugs`, `simple-drugs`, `drugs/{cod}`, `drug-compensations`, `drug-relations`, `drug-catalogues`, `drug-records/{tip}` | GET/HEAD | autentificat | `D/query.go:229-253`, `D/drugs_query.go` |
| `physicians`, `simple-physicians`, `physicians/{…}`, `physician-catalogues`, `physician-records/{tip}` | GET/HEAD | autentificat | `D/query.go:214-228`, `D/physicians_query.go` |
| `cim10`, `cim10/tree` | GET/HEAD | autentificat | `D/query.go:210-213`, `D/cim10.go` |
| `debug` | GET | autentificat | `D/module.go:275-276` |

Rute interne `/siui-data-clients`, `/data`, `/import`, `/keys`, `/token` (inclusiv `POST /siui-data-clients/import` pentru personalizări, `D/clients_http.go:56,73-250`): doar sesiune de browser local cu admin — nici Bearer, nici principal WSS (`D/clients_auth.go:84-93`); non-GET cer HTTPS, antet `X-SIUI-Internal: 1` și `Origin` corect (`D/clients_http.go:25-40`). Transport: **L**.

Parametrii de interogare, paginarea și forma rândurilor: `NECUNOSCUT` (semantica interogărilor nu a fost analizată — 04 §Necunoscute pct. 10); `statement_timeout` 30 s. 31 de teste `siuidata` sunt sărite (cer PostgreSQL). `[TEST]` pentru faptul săririi (06 §B.1 r.3).

---

## 11. WiseMED AI

Aplicație `wisemed-ai`, modul `wisemed-ai` (`AI/`), PostgreSQL (`wai_*`). Rute montate (`AI/module.go:76-102`): `/v1/`, alias `/api/wisemed-ai/v1/` (rescrie calea, același `serve`), `/ai/` (UI Vue embed), `/ai/voice`, `/health/ai`, `/ready/ai`. Port implicit `127.0.0.1:19120`. `[COD]`

- **Transport**: `/v1/*` este doar L; prin WSS se folosește aliasul `/api/wisemed-ai/v1/*` (L+W). SSE prin WSS → 400 `sse_not_supported_on_transport`.
- **Limite**: body JSON ≤ 1 MiB, câmpuri necunoscute respinse (`AI/auth.go:33-47`); fragmente model 8 MiB; audio comunitate 256 KiB. Timeout per cerere 55 s; apel OpenAI 45 s; prin WSS 30 s (o extragere lentă poate eșua doar pe WSS `[INF]`).
- **Fără bază de date**: orice `/v1/*` → 503 `database_not_configured` (`AI/auth.go:174`).
- **Auth** (legendă): **P** = publică; **S** = sesiune (cookie `wai_session` sau `Authorization: Bearer` token opac), orice rol; **D** = S + rol ≠ `viewer` pentru metode ≠ GET; **OA** = rol `admin` de organizație; **IA** = `instance_admin`; **G** = grant `Authorization: Browser <token>`. Prin WSS, identitatea vine din maparea `wai_memberships.external_subject` și numai dacă `wsm_identity_enabled`; scope-ul `api:admin` este ignorat.

| Metodă + rută | Handler | Auth | Body / parametri | Răspuns / coduri | Efecte |
|---|---|---|---|---|---|
| GET `/health/ai`, `/ready/ai` | `AI/module.go:89-101` | P | — | 200 / 503 `database_unavailable` | — |
| GET `/v1/auth/providers` | `AI/auth.go:182` | P | — | 200 `{standalone, wisemed}` | — |
| POST `/v1/auth/login` | `login` (`AI/auth.go:66-160`) | P | `email, password, tenant_id?, wisemed?` | 200 `{token, profile, expires_in}`; 400, 401, 429 (15/10 min/IP), 503 | sesiune 8 h + audit |
| POST `/v1/auth/logout` | `AI/auth.go:199` | S | — | 200 | DELETE sesiune |
| GET/PUT `/v1/profile` | `AI/admin.go:13` | S | nume/parolă | 200; 403 `current_password_required` | — |
| GET/PUT `/v1/profile/speech-consent` | `speechConsent` | S | consimțământ | 200 | evenimente |
| GET/POST `/v1/sessions` | `sessions`, `createSession` | S / D | `template_version, patient_ref ≤128, work_context?`; antet `Idempotency-Key ≤128` | 201 nou / 200 replay; 400, 402, 404, 409 `idempotency_conflict`/`session_conflict` | INSERT sesiune |
| GET/DELETE `/v1/sessions/{id}` | `AI/sessions.go:149-167` | S / D | — | 200; 404 | DELETE cascade (inclusiv rezultate aprobate, outbox) |
| GET `/v1/sessions/{id}/result` | | S | — | 200; 409 `result_unavailable` | — |
| GET `/v1/sessions/{id}/events` | `eventStream` (`AI/sessions.go:596-626`) | S | `Last-Event-ID` | SSE `event: progress`, tăiat la 55 s; 400 pe WSS | — |
| POST `…/transcript-chunks`, `…/audio-transcript-chunks` | `AI/sessions.go:231-343` | D | `Chunk{id UUID, sequence 1..10000, revision ≥1, text ≤16000 octeți, timestamp, is_final, target_field?}` | 200 `{accepted, patch_revision, result}`; 400 `invalid_chunk`, 402, 409, 413 `session_limit`, 422, 502 | UPSERT segment, AI, revizie nouă |
| POST `…/audio-transcription` | `AI/sessions.go:198` | D | `{"audio":"<base64 WAV>"}` (PCM16 mono, ≤ 700000 octeți, ≤ 8 s) | 200 `{text}`; 400 `invalid_audio`, 409, 502, 503 `audio_transcription_not_configured` | apel OpenAI + usage |
| POST `…/field-edit`, `…/field-options` | `sessionField` | D | `revision, field, value?` | 200; 400, 409 | override / sugestii |
| POST `…/presentation` | `sessionPresentation` | D | `revision, …` | 200; 400, 409 | revizie fără AI |
| POST `…/finalize` | `AI/sessions.go:344` | D | `{}` | 200 `Result`; 409 `transcript_required`/`final_chunks_required` | status `review` |
| POST `…/reopen` | `AI/sessions.go:408` | D | `{revision}` | 200; 409 `current_review_required` | status `recording` |
| POST `…/approve` | `AI/sessions.go:434-487` | D | `{revision, confirmed}` | 200; 409 `explicit_current_revision_required`/`finalize_required`; 422 `field_validation_failed`/`required_fields_missing` | rezultat clinic + aprobare, o tranzacție |
| POST `…/deliver` | `deliver` (`AI/connectors.go:36-79`) | D | `{connector_id, revision, confirmed}` | **202** `{delivery_id, status}`; 400, 404, 409 | rând outbox (unic per rezultat×conector) |
| POST `…/briefing` | `AI/sessions.go:195` | D | — | 200 mesaj static | — |
| POST `…/pairing` | `AI/sessions.go:488` | D | — | 201 `{token, qr_png, expires_in:120}` | token 2 min |
| POST `/v1/pairing/redeem` | `redeem` | D | `{token}` | 200 `{session_id}`; 410 | consum unic |
| GET `/v1/templates[/{id}]`, POST `/v1/templates` | `AI/templates.go:289,319` | S / D | `Template` + `addon_confirmation?` | 200; 402/403/409 | versiune nouă |
| POST `/v1/templates/proposals` | `proposals` | D | text raport | 200 | AI / local |
| GET `/v1/templates/{id}/speech-adaptation`; POST `…/sections/{sid}/speech-terms`, `…/speech-feedback` | `speechAdaptation` | S / D (editor) | termeni / feedback | 200; 403 `template_editor_required`, 404 | vocabular |
| `/v1/community/speech-contributions…` (GET, POST, DELETE, `/export`, `/{id}/audio` GET + PUT `?offset=`, `/finalize`, `/review`) | `communitySpeech` | S; POST/PUT ≠ viewer; review/export OA | fragmente ≤ 256 KiB | 200; 409 `unexpected_offset` | fișiere pe disc |
| `/v1/speech-models` GET; `/manage` GET; POST; `/blobs/{sha256}` GET, PUT `?offset=`; `/{id}/{ver}/{mirror\|check-update\|publish}` POST | `speechModels` | S (catalog, blob GET); IA rest | fragmente ≤ 8 MiB | 200; 409 `model_offset_conflict` | joburi mirror |
| POST `/v1/browser-speech` | `createBrowserSpeech` | D | `session_id?, template_version, section_id, target_field` | 200 `{token, path, expires_in:7200}` | grant |
| GET/POST `/v1/browser-speech/bridge` | `browserSpeechBridge` | G | GET `?after=`; POST `{action, state, event_id, ordinal, text, timestamp}` | 200; 401, 413 | evenimente criptate |
| GET `/ai/voice` | `browserSpeechPage` | P | — | HTML cu CSP nonce | — |
| `/v1/memory/{preferences,examples,feedback}` GET/POST/DELETE; POST `/v1/memory/search`; POST `/v1/memory/examples/{id}/embedding` | `resources`, `memorySearch`, `memoryEmbedding` | S / D (proprii) | `{name, payload, confirmed}` | 200; 400, 409 | — |
| `/v1/integrations`, `/v1/connector-mappings` GET/POST/DELETE | `resources` (`AI/admin.go:305`) | OA | secretele redactate la citire | 200; 400 `connector_host_not_allowlisted` | — |
| GET `/v1/outbox`, `/v1/audit` | `outbox`, `auditHTTP` | OA | — | 200 (max 100 / 250) | — |
| GET/POST/PUT `/v1/users` | `users` (`AI/admin.go:74`) | OA (+IA) | `email, name, role, external_subject, …` | 200; 403 | mapare identitate WSM/WiseMED |
| GET/PUT `/v1/organizations` | `resources` / `organization` | S / OA | `{name}` | 200 | — |
| GET `/v1/specialties`, `/v1/specialty-packs`, `/v1/dashboard`, `/v1/usage` | | S | — | 200 | — |
| GET/PUT `/v1/settings` | `settings`, `saveSettings` (`AI/admin.go:230`) | IA | provider, model, buget, prețuri, retenție, `openai_key` | 200 `{restart_required:true}` | rescrie `config.yaml` + fișier cheie |
| GET/PUT `/v1/runtime-settings`; POST `/v1/updates/check`; POST `/v1/wsm/reconnect` | `runtimeSettings` | IA | secțiuni whitelisted (`AI/runtime_settings.go:14-19`) | 200; 400, 502, 503 | rescrie `config.yaml` |
| `/v1/billing/*` (status, account, plans, assignments, subscriptions, invoices, payments, settle, specialties, statistics, usage, usage/prompt, users, users/role, users/password); DELETE `/v1/billing/template-addons/{id}` | `billing` (`AI/plans.go:76-125`) | S (proprii); IA (administrare) | variabil — `NECUNOSCUT` per rută | 200; 402/403/409 | — |
| GET `/v1/admin/sessions[/{id}]` | `adminSessions` (`AI/admin_sessions.go:118-151`) | OA sau IA | `user, status, offset, limit` | 200 `{items, total, read_only:true}` | audit la citire |

Idempotency `[COD]`: creare sesiune — `Idempotency-Key`; segmente — pe `(id, revision)`: conținut identic → 200 `{duplicate:true}`; conținut diferit → 409 `chunk_revision_conflict`; revizie mai mică → 200 `{accepted:false, reason:"stale_revision"}`; `approve` repetat → 200 idempotent; `deliver` — `ON CONFLICT DO NOTHING`.

Tabelul provine din dispatch și verificările de metodă/rol; handlerele de billing, voce, memorie nu au fost citite integral (05 §10). 26 de teste `wisemedai` sunt sărite (cer PostgreSQL) — inclusiv cele de izolare; nu sunt `[TEST]`. Frontend Vue: 10 teste Vitest + typecheck trecute `[TEST]` (06 §B.1 r.14–15).

---

## 12. HTML Print Formatter

Aplicație `html-print-formatter`, modul `R/modules/htmlprintformatter`. Port implicit `127.0.0.1:19113`; fără persistență. `[COD]`

| Metodă + rută | Auth | Body | Răspuns | Transport |
|---|---|---|---|---|
| POST `/render`, `/render/html`, `/render/url` | JWT HS256 | `html` **xor** `html_link`, `base_url?`, `timeout_ms?`, `viewport_width/height?`, `wait_for_print_ready?` | `text/html` + `X-Request-ID` | L |
| POST `/pdf` | JWT | idem | `application/pdf`, `Content-Disposition: attachment` | L |
| POST `/api/html-print-formatter/render` | JWT **sau** orice principal WSS | idem | `{"ok":true,"html","request_id"}` | L+W (610 s) |
| POST `/api/html-print-formatter/pdf` | JWT sau WSS | idem | `{"ok":true,"html","pdf_base64","filename","request_id"}` | L+W (610 s) |
| POST `/api/html-print-formatter/token` | sesiune local-http, same-origin | `{}` | `{"ok":true,"token","expires_in":300}` | L+W |
| GET `/health`, `/api/html-print-formatter/health` | publică | — | 200/503 `{"ok","service"}` | L / L+W |

- JWT (`R/modules/htmlprintformatter/auth.go:18-46`): `alg=HS256`, `kid` cunoscut din `jwt_keys`, `iss`=`aud`=`html-print-formatter`, `sub`, `iat`, `exp` obligatorii; fără chei → 503 `auth_not_configured`. Un principal WSS trece fără JWT și fără verificare de `Admin`/tenant (`auth.go:19-21`).
- Limite: body `max_body_bytes` 4 MiB (max 64 MiB); `default_render_timeout_ms` 120 000 (max 600 000); `max_concurrent_renders` 2. Prin WSS: 4 MiB (PDF în base64 +33 %).
- Coduri: 400 `invalid_json`/`invalid_request`, 401, 405, 408 `render_canceled`, 413 `body_too_large`, 415, 422 `page_error`, 502 `render_failed`/`source_load_failed`, 503 `browser_unavailable`, 504 `render_timeout`.
- `html_link` este încărcat direct de Chromium; nu s-a găsit restricție pe adrese interne `[COD]`.
- Exemple: `R/apps/html-print-formatter/examples/` (Postman, `pdf.php`).

---

## 13. Barcode Printer

Aplicație `barcodeprinter`, modul `R/modules/barcodeprinter`. Port implicit `127.0.0.1:8010`. **Toate rutele HTTP sunt neautentificate** — înregistrate doar cu `withCORS` (`module.go:71-82`). `[COD]`

| Metodă + rută | Handler | Parametri | Răspuns | Transport |
|---|---|---|---|---|
| GET/POST `/barcode/print` | `handleLegacyPrint` | form/query: `no`/`copies`, `othercfg_sel_printer`/`bc_selprinter`, … (restul: sursa) | 200 `{"success":true}` / 400 | L+W (singura cale non-`/api/` permisă, `AB:59`) |
| GET/POST `/barcode/print/posta-romana` | `handleLegacyPostaRomanaPrint` | form/query | idem | L |
| POST `/api/barcode/print`, `/api/barcode/print/posta-romana` | `handlePrintJSON`, `handlePostaRomanaPrintJSON` | JSON | idem | L+W |
| POST `/api/barcode/test-print` | `handleTestPrint` | JSON | idem | L+W |
| GET `/api/barcode/printers` | `handlePrinters` | — | `{"success":true,"printers":[…]}` | L+W |
| GET/PUT `/api/barcode/settings` | `handleSettingsAPI` | map setări | PUT rescrie `config.yaml`; poate schimba adresa/TLS listenerului | L+W |
| GET `/api/barcode/jobs`, `/api/barcode/stats/daily` | `handleJobs`, `handleDailyStats` | interval date, `limit` | listă | L+W |
| WS `barcode.print` etc. | `HandleWSAction` (`module.go:100-122`) | payload plat | vezi §7 | W |

Efecte: ZPL trimis RAW la spooler (`lp`/`lpr` sau spooler Windows), sincron, copie cu copie, fără coadă și fără retry; jurnal în SQLite `barcode_print_jobs`; ZPL-ul complet scris în log (`module.go:509-517`). Prin WSS, rutele `/api/barcode/*` devin protejate de `api:invoke` (la WSM), dar handlerul în sine rămâne fără verificare. `[COD]`

---

## 14. Signing pad / eSignature

Două aplicații, același modul `signing-pad` (`R/modules/signingpad`): `esignature-server` (nativ Signotec, doar Windows 32 biți; `127.0.0.1:19111` TLS) și `signing-pad-utility` (helper Java — **stub**, `HelperMain.java:48`; `0.0.0.0:19110`). Rute: `module.go:86-120`. `[COD]`

| Rută / comandă | Auth | Contract | Transport |
|---|---|---|---|
| WS `/ws` | doar `Origin` (`websocket.go:28-54`; Origin absent = permis) | protocol nou `{"id","action"}` cu `health\|devices\|start\|retry\|confirm\|cancel` → `{"id","action","ok",…}` (`confirm` → `imageBase64` PNG); protocol legacy (primul mesaj are `cmd`): `init`, `signpatient{sig_type, pacient_id, nume_pacient ≤512}`, apoi `retry\|activity\|confirm\|cancel\|disconnect` → `{success, forevent, data{id:1, sigbase64, sigenc}}` | L (WebSocket) |
| WS `/ws/demo` | sesiune local-http (mod `shared_http`) | idem | L |
| POST `/api/esignature/command` | doar `Origin` | `{"id","action","session_id"}` → 200 / 409 `{"ok":false,"message"}` / 400 | L+W |
| GET/POST/PUT `/api/esignature/settings` | sesiune | `manufacturer=signotec`, `pad_type=omega`, `model ≤128`, timeout 1–600 s, `device_index` 0–31 | L+W |
| GET `/api/esignature/jobs`, `/api/esignature/stats/daily` | sesiune | istoric | L+W |
| WSM `esignature.*` | `route:command` | vezi §7 | W |
| GET `/api/signing-pad/health`; POST `/api/signing-pad/capture` (doar dacă `helper_command` e setat, `module.go:117-120`) | doar CORS (`*` implicit) | `request_id, patient_id, file_id, sample_code, specimen_code, message, device_*` → răspunsul helperului + PNG pe disc; 502 `helper_error` | L+W |
| GET `/esignature`, `/health`, `/signing-pad/*` | `NECUNOSCUT` | pagini / stare | L |

Limite: mesaje WS și body HTTP ≤ 4096 octeți; sesiune ≤ `session_timeout_seconds` (180 s, max 600); un singur pad simultan (`deviceMu.TryLock`, „pad is busy in another session”); max 32 sesiuni remote; nimic nu supraviețuiește restartului. Teste: `R/apps/esignature-server/tests/*.test.cjs` rulate în suita Node (63 trecute, 12 sărite) `[TEST]` la nivel de suită (06 §B.1 r.12); hardware real neverificat.

---

## 15. ANAF DocSMART

Aplicație `anaf-docsmart`, modul `protocol-anaf-docsmart` (`R/modules/protocols/anafdocsmart`). Nu comunică cu ANAF; completează local un PDF XFA dintr-un XML. `[COD]`

| Metodă | Rută | Auth | Răspuns |
|---|---|---|---|
| GET | `/api/protocol/meta` | **niciuna** | `{"ok":true,"protocol":"anaf-docsmart"}` |
| GET | `/api/docsmart/parsed-files` | **niciuna** (`module.go:51-54`) | `{"ok":true,"files":[…]}` — metadate XML (an, lună, instituție, sumă control, totaluri, primele 5 conturi, **cale completă pe disc**) |

Intrarea reală este pe fișiere: `inbox/*.xml` (polling 5 s) → `outbox/<nume>.filled.pdf`; funcțional doar pe macOS cu Adobe Acrobat (`acrobat_darwin.go:15-43`). Transport: L+W.

---

## 16. Update server

Aplicație `update-server`, modul `app-update-server` (`R/modules/appupdateserver`). Listener propriu, implicit `0.0.0.0:19090`; SQLite + `./files`. Sesiunile admin și tokenurile de download sunt doar în memorie. `[COD]`

| Metodă + rută | Auth | Contract |
|---|---|---|
| POST `/api/session/login` | login WiseMED + `user_type` ∈ `allowed_user_types` (șablon „1,9,10”; implicit din cod `-1`, `module.go:2110`) | cookie `wmr_update_server_session`, 12 h, SameSite=Lax |
| GET `/api/session`, `/api/update-server/meta` | publică | stare |
| POST `/api/session/logout` | sesiune | |
| GET/PUT `/api/update-server/settings` | sesiune | `files_dir, public_base_url, allowed_user_types, verbose_level, cfg_wisemed_*` |
| GET/POST `/api/update-server/apps`; PUT/DELETE `…/apps/{id}` | sesiune | `application` |
| GET/POST `…/apps/{id}/versions` | sesiune | POST multipart ≤ 512 MiB: `version, channel, target_os, target_arch, mandatory, release_notes, download_url?, package?` |
| POST `…/apps/{id}/make-release` | sesiune | `{channel, target_os, target_arch, mandatory, release_notes}` → build din sursă pe server (`module.go:1322-1446`) |
| GET/PUT/DELETE `…/versions/{id}` | sesiune | |
| GET `…/package-download/{id}`, `…/installer-download/{id}`; POST `…/download-link/{id}` | sesiune | fișier / link cu token |
| POST `/api/public/check-update` | JWT HS256 semnat cu `cfg_wisemed_key`, scope `check` (`module.go:2120-2141`) | `CheckRequest{app_id, channel, current_version, os, arch}` → `CheckResponse{status, latest_version, mandatory, download_url, checksum_sha256, file_name, release_notes, version_record_id}` |
| GET `/api/public/download/{versionId}?token=…[&artifact=installer]` | token de unică folosință, 15 min (fără JWT) | fișier (`ServeContent`) sau 307 către `download_url` extern |

Note `[COD]`: `app_id` din JWT nu este comparat cu cel din body (`module.go:706`); tokenul de download este consumat la prima utilizare (fără reluare); checksumul este opțional și nu există semnătură a artefactului. Transport: doar L (nu trece prin `local-http`/WSS). Vezi [build-deploy-update.md](build-deploy-update.md).

---

## 17. API-uri consumate

### 17.1 WiseMED API (din readere, modul `R/modules/wisemedapi`)

Autentificare: JWT HS256 **per cerere**, semnat cu `cfg_wisemed_key`, `exp` +5 min, claims `caller_id="WM-Lab-Reader"`, `caller_type`, opțional `lt` și `login_user_id` (`module.go:1465-1493`). Timeout 20 s; fără retry/backoff. `?XDEBUG_TRIGGER=debug` este trimis și în producție pe #5–#7. `[COD]`

| # | Metodă + cale (relativ la `cfg_wisemed_path`) | Apelant | Payload | Răspuns așteptat | PHP local în repo |
|---|---|---|---|---|---|
| 1 | `GET /administrative/medicalunits` | `Bootstrap` (`:281`), `BootstrapClient` (`:1107`) | — | array (sau `{rows\|data\|items\|result}` doar în `BootstrapClient`) | `apis/wisemed-api/apiv2/implementation/administrative.php:94` |
| 2 | `GET /administrative/wmanalyzertypes` | `:285`, `:1115` | — | idem | `administrative.php:366` |
| 3 | `PUT /administrative/analyzer` | `ensureEquipmentHTTP` (`:354`), `RegisterEquipment` (`:1123`) | echipament + catalog analite (`:733-790`) | `echipament_id` (>0), `api_key_echipament`, … | `administrative.php:71` |
| 4 | `PUT /administrative/login` | `Login` (`:312`) | `{username, password, medical_unit_id, device_id, device_name}` | `login_token`/`token`/`lt`, `user_id`, `user_type` | `logins_user.php:2` |
| 5 | `GET /fileforanalyzer/{file_id}/{equipment_id}/` | `FetchFileForAnalyzer` (`:402-423`) | — | `o_file_*`, `o_patient_*`, `o_tests[{t_tag, t_sm_id, t_fsm_id, …}]` | `analyzers.php:2-131` |
| 6 | `PATCH /file/services/results/{file_id}` | `SaveFileServiceResults` (`:565-599`) | form: `srv[i][fsmid\|result\|interpretation\|conclusion]`, `[reagent_set_name]` | JSON nevalidat | `file_services.php:249-307` |
| 7 | `PATCH /file/services/dosimetry/` | `SaveDosimetry` (`:543-563`) — doar BEOSL | form: `dos[i][serial\|hp_10\|hp_007]` | `{success, results}` | `file_services.php:218-247` |
| 8 | `POST <token_path>` | `WSSAccessToken` (`R/modules/wisemedapi/wss.go:15-51`) | `{reader_id, client_id, equipment_id, role, client_type}` | `{token}`; doar 200; ≤ 16 KiB | PHP local are `GET /wsm/token` (`wsm_wsmtoken.php:4`) — **metodă diferită**; endpointul real: extern `[NEC]` |
| 9 | `POST\|PUT <apipath callback>` | `SendCallback` (`R/modules/wisemedapi/callback.go:30-74`) — SIUI bridge | JSON; antet `Idempotency-Key: siui-<call id>`; `lt` per job | doar codul HTTP | `[NEC]` |
| WS | `wisemed.fetch_file_for_analyzer` prin WSM, încercat înaintea #5 | `fetchFileForAnalyzerViaWS` (`:525-541`) | `{file_id, equipment_id}` | ca #5 | — |

Stratul REST PHP (`apis/wisemed-api/apiv2`, `apis/mp-api`) este în repo; logica de business (validarea JWT pe server, autorizarea fișă × echipament, blocarea rezultatelor) este externă `[NEC]`. Dacă versiunea din repo este cea din producție: `[NEC]`.

### 17.2 WiseMED din WSM

| Apel | Când | Auth | Referință |
|---|---|---|---|
| `PUT <base_url>/administrative/login` | login în consola admin | JWT HS256 cu cheia API a tenantului (`createWiseMedJWT`); timeout 8 s | `S/admin.go:247-304`, `S/wisemed.go:138-153` |
| upstream pentru `wisemed.ensure_equipment_online`, `wisemed.fetch_file_for_analyzer` | comenzi `mode:"server"` | idem, 5 min; URL-uri fixe per tenant; timeout 20 s | `S/wisemed.go:55-71` |

### 17.3 WiseMED din alte aplicații

- WiseMED AI: login `{"wisemed":true}` → `wisemedapi.Login` (`AI/auth.go:108-127`).
- update-server: login admin prin serviciul `wisemed-api` (`R/modules/appupdateserver/module.go:2057`).
- SIUI bridge: callback `callwmapi` (#9 de mai sus).

### 17.4 CNAS (din SIUI bridge)

mTLS (TLS 1.2 fix) cu certificatul de pe token; fără retry HTTP. `[COD]` pentru construcția cererilor; comportamentul serverelor CNAS `[DOC]`.

| Scop | URL | Referință |
|---|---|---|
| Sesiune („OCSP”) | `GET {base_url}/OCSP/validator?username=…` + `Authorization: Basic`; răspuns în antetul `OSCP_RESPONSE` | `SI/client.go:108-206` |
| SOAP SIUI / SIPE / eFactura | `POST {origine}/svapntws/services/{Service}` (`EPrescriptionWS` → `pe_base_url`; `SiuiEInvoiceWS` → `einvoice_base_url`) | `SI/operations.go:304,358-372` |
| DocMED | `POST {docmed_base_url}/{loader,seq,download,status}/ws` | `SI/endpoints.go:14-26`, `SI/docmed.go:172` |
| Fișiere | `GET <URL întors de CNAS>` — aceeași origine HTTPS, 1…64 MiB | `SI/operations.go:333-347`, `SI/service_jobs.go:448-492` |
| Verificări dashboard | `HEAD` pe endpointuri, fără credențiale | `SI/dashboard.go:60` |

Origini implicite: `https://siui.e-pias.ro`, `https://sipe.e-pias.ro`, `https://docmed.e-pias.ro` (`SI/endpoints.go:7-9`).

### 17.5 History (din SIUI bridge) și bridge (din SIUI Data)

- Bridge → History: `GET identity`, `POST calls`, `PUT calls/{id}/files/{name}?offset=`, `POST calls/{id}/commit`, plus citiri — §9; `Authorization: Bearer <token bridge>`, HTTPS, timeout 5 min, backoff ≤ 1 h (`SI/history_sync.go:183-323,467-515`). `[COD]`
- SIUI Data → bridge prin WSM: `list_connections`; `api.request` → `POST /api/siui/general/getCatalogues` sau `getPhysicianCatalogues` cu `operation_id = "siui-data-<importId>"`; `GET /api/siui/calls/{id}` la 1 s; `GET …/file?offset=`; 35 s per schimb (`D/bridge_sync.go:44-63,224-247,280-413`). `[COD]`

### 17.6 OpenAI (din WiseMED AI)

| Apel | Detalii | Referință |
|---|---|---|
| `POST https://api.openai.com/v1/chat/completions` | `model = cfg.Model`, `store:false`, `max_completion_tokens:4096`, `response_format: json_schema strict` | `AI/extract.go:217-222` |
| `POST https://api.openai.com/v1/audio/transcriptions` | model fix `gpt-transcribe`, multipart | `AI/transcription.go:20,68,91` |

Bearer cheie API (env `OPENAI_API_KEY` sau fișier); timeout 45 s; fără retry; rezervare de buget înaintea apelului. Provider implicit `mock` (fără rețea). `[COD]`

### 17.7 Conectori HIS (din WiseMED AI)

`POST <payload.url>` — `https`, host în `connector_allowed_hosts` (gol implicit), `Authorization: Bearer <token conector>`, `Idempotency-Key: <delivery id>`; max 5 încercări, backoff 2^n min (`AI/connectors.go:16-35,80-133`). `[COD]`

### 17.8 S3 (din History)

Stocare obiecte `s3` prin `minio-go`, `s3_endpoint` implicit `s3.amazonaws.com`, credențiale statice; cheie `<tenant>/<YYYY>/<MM>/<DD>/<bridge>/<call_id>/<artefact>` (`H/storage.go`, `H/calls.go:50-53`). `[COD]` Operațiile S3 exacte folosite: `NECUNOSCUT`.

### 17.9 Update-server (din orice aplicație bazată pe `runner`)

`POST /api/public/check-update` apoi `GET download_url` — o singură verificare la pornire; client `R/shared/appupdates/appupdates.go:84-197`; timeout 30 min; sărit în tăcere fără `cfg_wisemed_key` sau `base_url`. `[COD]`

### 17.10 Alte surse externe

Modele vocale (WiseMED AI): URL-uri din manifest, cu blocarea țintelor private (`AI/speech_models.go:471-491`). Chromium (formatter): `html_link` și sub-resurse. `[COD]`

---

## 18. Paritate

Rute implementate ↔ OpenAPI ↔ SDK JS ↔ client PHP/Dart ↔ frontend ↔ documentație ↔ teste. Fiecare rând este o nepotrivire concretă.

### 18.1 WSM

| # | Nepotrivire | Dovadă | Etichetă |
|---|---|---|---|
| W1 | `wsm-server/docs/protocol.md:211-212,261-262` trimite la `readersv3/modules/` pentru handleri `orders.*`, `analytes.*`, `config.*`, `stats.*`, `results.activate`, `logs.*` și descrie fluxul `results.activate` + topic. Nu există nicio implementare în `readersv3` (`HandleWSAction` doar în `ws`, `barcodeprinter`, `signingpad`). | 02 §15.1; liniile 211 și 261 verificate la redactare | `[COD]` |
| W2 | WSM autorizează `event` pe `results:<reader_id>` / `logs:<reader_id>`, dar niciun reader nu emite pe aceste topicuri. | `S/server.go:475`; grep fără rezultate (02 §6) | `[COD]` |
| W3 | Evenimentele WiseMED AI (`mode:"connection"`) sunt refuzate de WSM pentru rolul `reader`. | `WS:707-712`, `S/server.go:475`, `AI/events.go:28` | `[COD]` / efect `[INF]` |
| W4 | `hello_ack` din `protocol.md:149` nu conține `equipment_id`; codul îl trimite și readerul îl verifică. | `S/server.go:339`, `WS:408` | `[COD]` |
| W5 | `docs/http-commands.md:53-54` leagă „client inexistent” doar de 404; codul întoarce și `503 client_unavailable` la ACK `recipients:0`. | `S/http_commands.go:150-153` | `[COD]` |
| W6 | SDK JS `callAPI` aruncă pe status non-2xx; PHP `callAPI` păstrează statusul. Comportament divergent între cei doi clienți oficiali. | `S/sdk/wsm.js`, `WSMHttpClient.php` | `[COD]` |
| W7 | SDK JS acceptă doar `role=browser` și țintește doar `mode:"connection"`; țintele `equipment`/`reader` sunt disponibile numai prin `send()` (care întoarce doar ACK). | `S/sdk/wsm.js:62,126` | `[COD]` |
| W8 | `wsm-server/web/` (UI vechi) listează comenzile din W1, dar nu este servit de nicio rută. | `S/server.go:131-173` | `[COD]` |
| W9 | Cheile de device generate au `Roles: ["reader","service"]`; `docs/remote-control.md:33` spune „rol reader”. | `C/devices.go:194` | `[COD]` |
| W10 | `docs/verification.md` (22 sept. 2026) raportează toate verificările PASS; astăzi suita Node a SDK-ului are un eșec (S1). | 06 §B.3 pct. 11 | `[TEST]` |

### 18.2 SIUI bridge

| # | Nepotrivire | Dovadă | Etichetă |
|---|---|---|---|
| S1 | **`report/package`** este implementată (`SI/module.go:307`, `SI/report.go:45-105`) și documentată în OpenAPI (`SI/openapi.go:111`), dar lipsește din `SIUIBridge.endpoints` (`S/sdk/siui-bridge.js:98`). Testul `wsm-server/internal/server/sdk/siui-bridge.test.cjs:16-22` eșuează: `missing auxiliary path report/package`. | 06 §B.2 E1 (51 teste, 1 eșuat) | `[TEST]` |
| S2 | Rute omise din OpenAPI-ul generat (`SI/openapi.go`): `validations` (3 rute), `insured`, `personalization`, `ocsp-test`, `certificates/refresh`, `GET general/ocsp` (doar `post` e declarat), toate `siui-history-sync/*`. | 04 §2.5 | `[COD]` |
| S3 | OpenAPI: codurile 403 lipsesc la `settings`/`certificates`/`status` (doar 200/401); `certificate_thumbprint` e `required` deși poate fi gol (`SI/module.go:550-551`). | 04 §2.5 | `[COD]` |
| S4 | OpenAPI: operațiile DocMED folosesc tag `docmed` nedeclarat; titlul și `tags` enumeră doar „GENERAL / CLINICE / FACTURARE”. | `SI/openapi.go` | `[COD]` |
| S5 | `R/apps/siui-bridge/docs/API.md`: numește 41 de operații; DocMED delegat către `DocMED.md`; lipsesc secțiuni proprii pentru 12 operații (prescripții serii, eFactura etc.); `:990` descrie „63 de operații (general, clinice, facturare)”, omițând grupul `docmed`. | 04 §2.5, Contradicții 6 | `[COD]` |
| S6 | `wsm-server/docs/siui-browser-wrapper.md:182` anunță 63 de operații, dar numește explicit 41 + 2 DocMED; SDK-ul le are pe toate 63. Căile `siui-history-sync/*` nu apar în lista wrapperului. | 04 §2.5, Contradicții 12 | `[COD]` |
| S7 | Envelope: documentația descrie `{"ok":…}` ca contract comun; `operations*`, `dashboard`, `downloads/storage`, `siui-history-sync/*` folosesc `"success"`. | §2 | `[COD]` |
| S8 | `API.md:91`, `protocol.md:123`: „XML-ul cererii nu e păstrat în istoric”; codul scrie parametrii compleți în `siui_history_outbox.parameters` și îi servește prin `operations/{id}/request`. | `SI/service_jobs.go:323-324`, `SI/operations_history.go:162-193` | `[COD]` |
| S9 | `API.md:87`: „25 secunde pentru testul OCSP”; codul folosește 120 s. | `SI/client.go:16`, `SI/passive.go:50` | `[COD]` |
| S10 | 10 operații din WSDL neexpuse; documentația menționează ca excluse doar 2. | 04 §1.2 | `[COD]` |
| S11 | Mesajul 503 „Real CNAS operations require Windows or macOS” (`SI/service_jobs.go:254`) contrazice suportul Linux PKCS#11 (`SI/module.go:132`). | 04 Contradicții 4 | `[COD]` |
| S12 | Testele de rute lipsesc din `R/apps/siui-bridge/tests/` (doar oracolul Java DocMED); paritatea catalog ↔ WSDL este fixată de teste Go din suita trecută. | 04 §2.5 | `[COD]` / `[TEST]` la nivel de suită |

### 18.3 WiseMED AI

`R/apps/wisemed-ai/docs/openapi.json`: OpenAPI 3.1.0, `info.version` 1.3.0, 74 de căi. `contract_test.go` verifică doar existența a 9 căi. `[COD]`

| # | Nepotrivire | Cod | OpenAPI | Vue | Dart (în repo) |
|---|---|---|---|---|---|
| A1 | `POST /v1/sessions/{id}/audio-transcription` | `AI/sessions.go:198` | lipsă | nu | nu |
| A2 | `POST /v1/sessions/{id}/audio-transcript-chunks` | `AI/sessions.go:231` | lipsă | nu | nu |
| A3 | `PUT /v1/billing/users/password` | `AI/plans.go:113` | lipsă | da | nu |
| A4 | `GET /v1/billing/usage/prompt` | `AI/plans.go:117` | lipsă | da | nu |
| A5 | `DELETE /v1/billing/template-addons/{id}` | `AI/auth.go:255` | lipsă | da | nu |
| A6 | `GET /v1/templates/{id}` | `AI/templates.go:319` | lipsă | nu | nu |
| A7 | `/health/ai`, `/ready/ai` | `AI/module.go:89-101` | lipsă | nu | nu |
| A8 | Coduri 402, 405, 410, 413, 500, 502 returnate efectiv | — | nedeclarate | — | — |
| A9 | `Idempotency-Key` la creare sesiune | suportat | documentat | **netrimis** | trimis |
| A10 | `work_context` | suportat | documentat | netrimis | netrimis |
| A11 | Clientul Dart (`R/apps/wisemed-ai/clients/flutter`) acoperă doar login/logout, sesiune, chunk, field-edit/options, reopen, finalize, result, approve, pairing/redeem, events | — | — | — | fără deliver, presentation, audio, browser-speech, templates |
| A12 | `docs/WisemedAI.md:15`: „STT este pe telefon în MVP”; codul are și transcriere pe server | `AI/transcription.go` | lipsă | — | — |

A1–A5 sunt cele cinci rute de API omise (05 §9.2). Toate `[COD]`. Aplicația Flutter completă este externă `[NEC]`.

### 18.4 Laborator / `local-http` / WiseMED API

| # | Nepotrivire | Dovadă | Etichetă |
|---|---|---|---|
| L1 | Readerul face `POST <token_path>`; PHP-ul local expune `GET /wsm/token`. | `R/modules/wisemedapi/wss.go:15`, `apis/wisemed-api/apiv2/implementation/wsm_wsmtoken.php:4` | `[COD]` |
| L2 | `Module.Bootstrap` așteaptă array brut; PHP-ul local întoarce `{success, rows}` ⇒ `GET /api/wisemed/bootstrap` ar întoarce liste goale față de acest backend. | `R/modules/wisemedapi/module.go:715-731`, `administrative.php:140` | `[COD]`; producția `[NEC]` |
| L3 | Readerul acceptă ID corectat nenumeric (≤ 200 caractere); PHP-ul local aplică `intval()`. | `analyzers.php:5-6`, `file_services.php:251` | `[COD]` |
| L4 | Rute PHP existente dar neconsumate: `PUT /fileforanalyzer/results/`, `PUT /file/mplabresults`. | `analyzers.php:137`, `files.php:266` | `[COD]` |
| L5 | Memorie/documente vechi: `/api/stats`, `/api/order-analysis`, `/api/config`, tabel `event_logs`, eveniment `result_available` — inexistente în v3. | 03 C3 | `[COD]` |
| L6 | Nu există OpenAPI pentru rutele `local-http`/laborator; contractul este doar codul și UI-ul (`R/modules/localhttp/ui/app.js`). | absență | `[COD]` |
| L7 | `ensureEquipmentOnlineViaWS` este cod mort (niciun apelant), deși WSM expune comanda. | `R/modules/wisemedapi/module.go:508-523` | `[COD]` |

### 18.5 Utilitare

| # | Nepotrivire | Dovadă | Etichetă |
|---|---|---|---|
| U1 | Formularul UI barcodeprinter afișează „Auth user/pass”; nicio rută nu verifică credențiale. | `R/modules/barcodeprinter/ui/index.html:65-66` | `[COD]` |
| U2 | `anaf-docsmart/README.md`: „se autentifică cu login WiseMED”; `/api/docsmart/parsed-files` nu cere sesiune. | `anafdocsmart/module.go:51-54` | `[COD]` |
| U3 | `signing-pad-utility`: șablonul configurează helper Java; helperul livrat este un stub. | `HelperMain.java:48` | `[COD]` |
| U4 | update-server: `allowed_user_types` „1,9,10” în șablon vs `-1` implicit în cod. | `appupdateserver/module.go:2110` | `[COD]` |
| U5 | update-server public folosește cheia `message` pentru erori; restul aplicației `error`. | §2 | `[COD]` |

---

## 19. Necunoscute

- Forma exactă a corpurilor/răspunsurilor pentru majoritatea rutelor `local-http` (§4.1–4.2), pentru interogările SIUI Data și pentru `/v1/billing/*`, voce și memorie din WiseMED AI — handlerii nu au fost citiți integral în rapoarte. `[NEC]`
- Schemele din `openapi.json` (AI) și din OpenAPI-ul SIUI generat nu au fost comparate câmp cu câmp cu structurile Go. `[NEC]`
- Implementarea din backendul WiseMED (endpoint de token WSM, emiterea JWT-urilor de browser, respectarea `Idempotency-Key`, autorizarea fișă × echipament) și comportamentul real al CNAS. `[NEC]`
- Configurația de producție a WSM (`max_message_bytes`, `allow_query_token`, `admin.enabled`, scope-uri reale). `[NEC]`
- Ce expun paginile HTML fără middleware `/history-sync-ui` și `/history-ui` (conținut necitit). `[NEC]`
- Rutele `/esignature`, `/health`, `/signing-pad/*` ale modulului `signing-pad` și paginile `/`, `/settings`, `/help-ui`, `/app-updates/*` ale update-serverului: autentificare și conținut neinventariate. `[NEC]`
- Testele marcate `[TEST]` „la nivel de suită” au trecut ca parte a `go test ./...`; raportul 06 nu nominalizează fiecare test. Testele care cer PostgreSQL/MySQL/token CNAS au fost sărite (60 în `readersv3`).

Pentru urmărirea lacunelor: [gaps-and-contradictions.md](gaps-and-contradictions.md) și [verification-matrix.md](verification-matrix.md).
