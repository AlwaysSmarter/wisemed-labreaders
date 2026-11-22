# 02 — WSM: comunicație și securitate

Investigație read-only (2026-11-14). Nu s-a rulat niciun test/binar; etichetele `CONFIRMAT PRIN TEST LOCAL` nu sunt folosite. Unde există un test care acoperă afirmația, este menționat ca „test existent, nerulat”.

Etichete: **[COD]** = CONFIRMAT DIN COD · **[DOC]** = DOCUMENTAT, NEVERIFICAT · **[INF]** = INFERENȚĂ · **[NEC]** = NECUNOSCUT.

Căi relative la rădăcina repo-ului. Prescurtări: `S/` = `wsm-server/internal/server/`, `C/` = `wsm-server/internal/config/`, `WS` = `readersv3/modules/ws/module.go`, `LH` = `readersv3/modules/localhttp/module.go`, `AB` = `readersv3/shared/apibridge/bridge.go`.

---

## 0. Rezumat executiv

- WSM este un hub WSS **exclusiv în memorie**, multi-tenant, cu JWT HS256 per cheie (`kid`) legată de un singur tenant. Fără coadă persistentă, replay, deduplicare sau livrare garantată. [COD]
- `command_ack` confirmă doar **intrarea în coada de trimitere** (`delivery:"queued"`), nu recepția și nu execuția. [COD]
- `api.request` invocă **același `http.ServeMux` in-process** (`handler.ServeHTTP` pe un recorder), nu deschide conexiuni de rețea. Identitatea vine din câmpurile `sender_*` ștampilate de WSM. [COD]
- Riscuri/observații noi găsite în cod (detalii în secțiunile indicate):
  1. Cookie-ul local al readerelor obișnuite e semnat HMAC cu o cheie **predictibilă** (`"wisemed-local-session:"+ReaderID`) și e acceptat fără lookup server-side → sesiune falsificabilă, logout neefectiv (§10).
  2. În modul `device_key`, readerul semnează JWT de 5 min, iar WSM închide socketul la `exp` → reconectare ciclică cu pauză fixă de 30 s (§6, §13).
  3. `authEpoch` este **global**: o modificare admin în tenantul A (cheie browser, origini șterse, setări tenant) invalidează lease-urile și tichetele tuturor tenanturilor la următorul mesaj (§11).
  4. Comenzile de dispatcher (`barcode.print`, `esignature.*`, `ws.reconnect`…) cer doar `route:command` pentru expeditori `browser`/`service`; `api:invoke` protejează doar `api.request` (§6, §8).
  5. `PUT /api/wisemed/setup` local nu cere sesiune la readerele obișnuite (§10).

---

## 1. Verificarea JWT în WSM

Funcție: `authenticate` — `S/auth.go:65-163`. [COD]

| Aspect | Regulă | Referință |
|---|---|---|
| Algoritm | doar `HS256` (`jwt.WithValidMethods`) | `S/auth.go:101` |
| Selecția cheii | header `kid` obligatoriu → `cfg.Security.Keys[kid]`; cheie/tenant `Disabled` sau secret gol → respins | `S/auth.go:90-100` |
| Sursa tokenului | `Authorization: Bearer`; pentru WS și subprotocol `wsm.jwt.<JWT>` (unul singur); `?token=` doar cu `security.allow_query_token` | `S/auth.go:66-83` |
| Dimensiune | ≤ 8192 caractere | `S/auth.go:84` |
| `exp` | obligatoriu (`WithExpirationRequired`) | `S/auth.go:101` |
| `iat` | obligatoriu, validat (nu în viitor) | `S/auth.go:101,123` |
| `aud` | trebuie să conțină `security.audience` (implicit `wsm-server`) | `S/auth.go:101`, `C/config.go:190` |
| `nbf` | opțional, validat de bibliotecă dacă există | [INF] din `golang-jwt/v5` |
| `tenant_id` | == `Key.TenantID` al cheii selectate | `S/auth.go:117` |
| `iss` | == `Tenant.Issuer` | `S/auth.go:120` |
| `sub`, `client_id` | `ValidID` (`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`) | `S/auth.go:123`, `C/config.go:87` |
| Durata | `0 < exp-iat ≤ max_token_ttl_seconds` (implicit 900, max 86400) | `S/auth.go:126`, `C/config.go:193-198` |
| `role` | ∈ `Key.Roles` (`browser`/`reader`/`service`) | `S/auth.go:129`, `C/config.go:257-261` |
| `scopes` | nevid; fiecare valid (`ValidScope`) și ⊆ `Key.Scopes` (un scope în plus respinge tot tokenul) | `S/auth.go:132,154-160` |
| Restricții cheie | dacă `Key.Subject`/`ReaderID`/`EquipmentID` sunt setate, claim-urile trebuie să fie egale | `S/auth.go:135-143` |
| `reader`/`service`+`reader_id` | cer `equipment_id` și `reader_id` valide; alte roluri nu pot purta `reader_id` | `S/auth.go:144-153` |
| `client_type` | **nu este claim**; vine din `hello` și trebuie să fie egal cu `role` (sau `siui-bridge`/`siui-bridge-history` pentru `service`) | `S/auth.go:168-173` |
| `jti` / revocare individuală | inexistentă | [COD] absent; [DOC] `docs/protocol.md:76-77` |

Scope-uri valide: `route:command`, `route:reply`, `route:event`, `route:broadcast`, `topics:subscribe`, `connections:read`, `server:status`, `wisemed:proxy`, `devices:debug`, `api:invoke`, `api:admin` — `C/config.go:90-96`. [COD]

**Chei per tenant.** `security.keys.<kid>` are `tenant_id`, secret din `secret_file` xor `secret_env`, ≥ 32 bytes; două tenanturi nu pot împărți același secret (`C/config.go:241-287`, `C/access.go:163-171`, `C/devices.go:196-205`). Chei generate: device (`Roles: reader, service`, legate de `subject`/`equipment_id`/opțional `reader_id` — `C/devices.go:194`) și browser (`Roles: browser, service` — `C/access.go:161`). [COD]

**Revocare.** [COD]
- Dezactivare cheie/tenant + `SIGHUP` → `Server.Reload` închide **toate** socketurile, șterge sesiunile admin, incrementează `authEpoch` (`S/server.go:50-77`, `cmd/wsm-server/reload_unix.go:16-36`). Pe Windows nu există reload; e nevoie de restart (`reload_windows.go`).
- Ștergere/actualizare device din admin → închide doar conexiunile cu `(tenant, subject)` și tichetele aferente (`S/admin.go:520-527,592-600,606-614`).
- Modificare cheie browser / ștergere origine / setări tenant → `revokeTenantAccess` (`S/admin_access.go:347-363`): închide conexiunile tenantului **și** incrementează `authEpoch` global (vezi §11).
- Expirare: `time.AfterFunc(time.Until(exp), conn.Close)` — lease strict (`S/server.go:265`); verificat și la fiecare mesaj (`S/server.go:311`).

**Origini.** `allowedOrigin`: Origin absent ⇒ permis; altfel potrivire exactă în `tenants.<id>.allowed_origins` (doar `https`, fără path) — `S/auth.go:174-177`, `C/config.go:219-224`. Pentru sesiunile de control (tichet): Origin trebuie să fie exact `https://<Host>` (`S/control.go:87-92`). Aplicat la upgrade (`S/server.go:210,238`), `GET /api/connections`, `GET /api/equipment/{id}`, `POST …/commands`. [COD]

**Rolurile identificatorilor.** [COD]

| Identificator | Cine îl stabilește | Rol |
|---|---|---|
| `tenant_id` | claim JWT, forțat egal cu tenantul cheii | granița de izolare; cheia de partiționare a hubului (`Connection.TenantID`, `S/server.go:246`) |
| `sub` | claim JWT (poate fi fixat de cheie) | identitatea principalului; ștampilat ca `sender_subject`; folosit la revocarea device-urilor și ca `Username` în sesiunea sintetică a readerului (`LH:2572`) |
| `client_id` | claim JWT; `hello.client_id` trebuie să coincidă | etichetă de sesiune/aplicație; neunic (mai multe sesiuni browser cu același `client_id` permise) |
| `equipment_id` | claim JWT (ID-ul WiseMED al echipamentului) | țintă de rutare `mode:"equipment"`; unicitate per `(tenant, equipment_id)` la `Register` (`S/hub.go:66`) |
| `reader_id` | claim JWT (identitatea locală a aplicației) | țintă `mode:"reader"`; unicitate per `(tenant, reader_id)`; prefix de topic permis readerului (`results:<id>`, `logs:<id>`) |
| `connection_id` | generat de server, 16 bytes aleatori hex (`S/hub.go:50-56`) | identifică socketul; se schimbă la fiecare reconectare; ținta reply-urilor |

---

## 2. Handshake `hello` / `hello_ack`, prezență, discovery

Flux în `handleWS` — `S/server.go:197-377`. [COD]

1. Autentificare (JWT sau tichet) + Origin + limite de conexiuni **înainte** de upgrade. Eșec: `401` JSON `{error:"wsm_<cod>", message}` (`writeAuthFailure`, `S/auth.go:42-51`), `403 origin denied`, `503 connection limit`.
2. Upgrade cu subprotocol negociat `wsm.v1`, `HandshakeTimeout` 5 s (`S/server.go:238`).
3. Primul mesaj trebuie să fie `hello` în `hello_timeout_ms` (implicit 10 s; `S/server.go:271-272`). Pong-urile înainte de hello **nu** prelungesc termenul (`S/server.go:274-280`).

`hello.payload` (`HelloPayload`, `S/message.go:28-35`): `client_type`, `client_id`, `reader_id`, `equipment_id` (opțional), `user_id` (opțional, == `sub`), `label` (opțional, == claim). Validare `validateHelloAgainstClaims` (`S/auth.go:168-173`).

`hello_ack.payload` (`S/server.go:339`): `connection_id`, `tenant_id`, `equipment_id`, `client_type`, `client_id`, `reader_id`, `subject`, `expires_at`.

| Eșec | Efect | Referință |
|---|---|---|
| alt tip de mesaj înainte de hello | close 1008 „hello required” | `S/server.go:317-321` |
| hello ≠ claims | close 1008 „hello identity denied” | `S/server.go:324-328` |
| reader/SIUI duplicat (`reader_id` sau `equipment_id` deja conectat în tenant) | close 1008 „reader identity already connected” — **conexiunea nouă pierde**, cea veche rămâne | `S/server.go:331-335`, `S/hub.go:64-68` |
| timeout hello | socket închis, log `hello_timeout` | `S/server.go:288-293` |
| hello repetat | close 1008 | `S/server.go:346-349` |

**Prezență.** `presence` `{event:"connected"|"disconnected", connection_id, client_type, client_id, reader_id, equipment_id}` difuzat tuturor conexiunilor tenantului (inclusiv expeditorului la `connected`) — `S/server.go:298,340,378-380`. Fără istoric. Observație [COD]: la ieșirea prin `authEpoch`/expirare detectată la mesaj (`S/server.go:311-313`) și la `policyClose` pentru rată nu se emite `disconnected`.

**Discovery.** `list_connections` (WS, scope `connections:read`) → `connections`; comenzi server `equipment.list` / `equipment.status`; HTTP `GET /api/connections[?client_type=]`, `GET /api/equipment/{id}`. Toate folosesc `Hub.Snapshot(tenant)` (`S/hub.go:98-112`), sortat după `connected_at`. `ConnectionInfo` expune și `remote_ip`, `subject`, `topics`, `queue_depth` (`S/hub.go:15-29`). [COD]

Client reader: `hello` cu `client_type`, `client_id`, `reader_id`, `equipment_id`; așteaptă `hello_ack` 10 s; verifică `ack.equipment_id == echipament_id` local (`WS:394-410`). [COD]

---

## 3. Envelope, rutare, ACK, limite

`Envelope` (`S/message.go:5-14`): `type`, `request_id`, `correlation_id`, `connection_id` (setat de server = expeditor), `target`, `broadcast` (respins), `payload`, `timestamp`. `Target` (`S/message.go:16-26`): `mode` + `connection_id`/`connection_ids`/`equipment_id`/`equipment_ids`/`reader_id`/`reader_ids`/`client_type`/`topic`.

Tipuri acceptate de la client: `hello`, `ping`, `list_connections`, `subscribe`, `unsubscribe`, `command`, `reply`, `event`. Emise de server: `hello_ack`, `pong`, `connections`, `subscribe_ack`/`unsubscribe_ack`, `command_ack`, `reply` (comenzi server), `error`, `presence`. [COD] `S/server.go:387-566`

**Autorizare per mesaj** — `authorizeMessage`, `S/server.go:387-528`: [COD]

| Regulă | Linie |
|---|---|
| sesiune de control (tichet): doar `ping` și `command api.request` cu `mode:"equipment"` == echipamentul tichetului | 389-391 |
| `request_id`/`correlation_id` ≤ 128; `command` cere `request_id`; `reply` cere `correlation_id` | 392-417 |
| `target` obligatoriu; `broadcast:true` respins | 408-410 |
| `mode:"server"`: doar `command`; `control.ticket` (`api:invoke`+`route:command`), `equipment.status`/`equipment.list` (`connections:read`), `server.status` (`server:status`), `wisemed.*` (`wisemed:proxy`) | 418-436 |
| `api.request` cere `api:invoke`; orice `command`/`reply`/`event` cere `route:<type>` | 437-442 |
| rol `reader`: comenzi doar 1:1 (`connection`/`equipment`/`reader`) și doar `api.request` (+`api:invoke`) sau `device.ping`/`ws.reconnect`/`debug.message` (+`devices:debug`); `reply` doar `mode:"connection"`; `event` doar pe topic `results:<reader_id>` / `logs:<reader_id>` | 443-478 |
| `connections`/`readers`/`equipments` (1..100 ID-uri), `all`, `client_type`, `topic` (non-reader) cer `route:broadcast` | 494-524 |

De reținut [COD]: pentru roluri `browser`/`service`, **numele comenzii nu este filtrat** (în afară de `api.request`); orice `payload.command` este rutat cu doar `route:command`. Rolul `service` (SIUI bridge) nu are restricțiile rolului `reader`.

**Rutare** — `Hub.Route`/`route`, `S/hub.go:121-168`: filtrează întâi `c.TenantID == sender.TenantID`, apoi după mod. `dispatch` (`S/server.go:548-565`) suprascrie în payload `sender_connection_id`, `sender_ip`, `sender_client_type`, `sender_client_id`, `sender_reader_id`, `sender_tenant_id`, `sender_subject`, `sender_scopes` (valori furnizate de client sunt înlocuite; test existent, nerulat: `server_test.go:570`). [COD]

**Ce confirmă ACK-ul.** `command_ack.payload = {routed_type, recipients:n, target, delivery:"queued"}` unde `n` = numărul de conexiuni pentru care `c.send <- msg` a reușit (`S/server.go:561-563`, `S/hub.go:195-217`). Nu confirmă scrierea pe socket, recepția sau execuția. `recipients:0` = fără destinatar eligibil în tenant sau coadă plină. ACK se emite și pentru `reply`/`event`. Comenzile `mode:"server"` nu primesc ACK, ci direct `reply`/`error code=command_failed` (`S/server.go:358-373`). [COD]

**Coadă persistentă / replay / deduplicare / livrare garantată: nu există.** Dovezi [COD]:
- `Hub` conține doar `connections map` și `stats map` în memorie (`S/hub.go:43-49`); singurul buffer e `Connection.send chan Envelope` (`S/hub.go:56`).
- Nicio scriere pe disc în `S/hub.go`, `S/server.go`; `logstore` scrie doar loguri text.
- Niciun registru de `request_id` văzute; reply-urile nu sunt validate față de comenzi în așteptare (doar HTTP wrapper și clienții corelează).
- Destinatar offline ⇒ `recipients:0`, mesajul e pierdut. Abonamentele (`Topics`) trăiesc în `Connection` și dispar la deconectare.
- Contra-dovezi căutate: niciuna găsită.

**Limite și timeouturi server** (valori implicite din `C/config.go:131-172`; producția-șablon `deployments/config.production.yaml` setează `max_message_bytes: 8388608`): [COD]

| Parametru | Implicit | Efect la depășire |
|---|---|---|
| `max_message_bytes` | 1 MiB (max 16 MiB) | `SetReadLimit` → eroare de citire, socket închis (`S/server.go:270`) |
| `messages_per_second` / `message_burst` | 50 / 100 (token bucket per conexiune) | close 1008 „message rate exceeded” (`S/server.go:302-309`) |
| `send_queue_size` | 128 (max 4096) | **buffer plin ⇒ mesajul e aruncat, `TotalDropped++`, conexiunea lentă e închisă** (`S/hub.go:204-216`); fără blocare a expeditorului |
| `max_connections` / `…_per_tenant` | 2000 / 200 | `503 connection limit` (include socketuri pre-hello și participanți HTTP temporari) |
| `max_topics_per_connection` | 64 | `subscribe_ack ok:false` |
| `hello_timeout_ms` | 10 000 | închidere |
| `read_timeout_ms` | 60 000 (resetat la pong; setat o dată după hello) | închidere `read_timeout` |
| `ping_interval_ms` | 25 000 (control frame Ping din `writePump`) | — |
| `write_timeout_ms` | 5 000 | `writePump` iese, conexiune închisă |
| HTTP server | `ReadHeaderTimeout` 5 s, `ReadTimeout`/`WriteTimeout` 10 s, `IdleTimeout` 60 s, header ≤ 16 KiB | `S/server.go:90` |
| liste țintă | ≤ 100 ID-uri | `forbidden` |
| TLS | min TLS 1.2; fără TLS doar cu `allow_insecure_loopback` pe loopback sau `allow_insecure_http` explicit | `S/server.go:91`, `C/config.go:173-189` |

Observație [COD]: `read_timeout` este prelungit doar de **Pong** (`S/server.go:274-280`), nu de mesajele JSON primite; un client care nu răspunde la Ping-uri este închis după 60 s chiar dacă trimite date.

`ping` JSON → `pong {server_time}` (`S/server.go:532-534`), distinct de control frames.

---

## 4. Endpointuri HTTP ale WSM

Rute: `S/server.go:131-173`, `S/admin.go:49-69`, `S/admin_ui.go:13-39`. Middleware global: `X-Content-Type-Options: nosniff`, `Cache-Control: no-store`, HSTS pe TLS. **Nu există CORS** pe API-urile de date (doar pe SDK). [COD]

| Metodă | Rută | Handler | Autentificare / autorizare |
|---|---|---|---|
| GET | `/healthz` | inline `S/server.go:164` | public; `{status:"ok",service:"wsm-server"}` |
| GET | `/ws` | `handleWS` | JWT (header / `wsm.jwt.` / query opțional) sau tichet `wsm.ticket.`; Origin |
| GET | `/api/connections` | `connections` `S/server.go:174` | Bearer; `connections:read`; Origin; filtru `?client_type=` |
| GET | `/api/debug/state` | `connections` | alias identic |
| GET | `/api/equipment/{id}` | `equipmentHTTP` `S/equipment.go:21` | Bearer; `connections:read`; Origin |
| POST | `/api/connections/{id}/commands` | `commandHTTP` `S/http_commands.go:17` | Bearer; rol `service` sau `browser`; Origin; apoi `authorizeMessage` |
| GET | `/sdk/wsm.js`, `/sdk/siui-bridge.js` | `serveBrowserSDK` `S/browser_sdk.go:11` | public; `Access-Control-Allow-Origin: *`; embed |
| GET | `/control`, `/control/*`, `/app.js`, `/styles.css`, `/wss-remote.js`, `/docmed-forms.js`, `/siui-invoice.js` | `controlAsset` `S/control.go:93` | public, doar 6 fișiere din `server.control_ui_dir` (allowlist); orice altceva → `index.html`; 503 dacă directorul nu e configurat |
| GET | `/admin`, `/admin/`, `/admin/app.js`, `/admin/styles.css` | `registerAdminUI` | `admin.enabled` + Host/Origin configurat; CSP strict; embed |
| GET | `/admin/api/tenants` | `adminTenants` `S/admin.go:125` | **fără sesiune** (doar `adminConfig`): listează ID-urile tenanturilor cu upstream WiseMED configurat |
| POST | `/admin/api/login` | `adminLogin` `S/admin.go:169` | same-origin JSON; rate limit; validare la WiseMED |
| GET | `/admin/api/session` | `adminSessionStatus` | cookie opțional |
| POST | `/admin/api/logout` | `adminLogout` | sesiune + CSRF |
| GET | `/admin/api/logs/dates`, `/admin/api/logs` | `S/admin_logs.go` | sesiune; filtrare pe tenant |
| GET / PATCH | `/admin/api/tenant` | `adminTenantInfo` / `adminUpdateTenant` | sesiune (+CSRF la PATCH) |
| PATCH | `/admin/api/tenant/origins` | `adminOrigins` | sesiune + CSRF |
| GET / POST | `/admin/api/browsers` | `adminBrowsers` / `adminCreateBrowser` | sesiune (+CSRF) |
| POST | `/admin/api/browsers/key` | `adminRevealBrowser` | sesiune + CSRF; întoarce secretul în clar |
| PATCH / DELETE | `/admin/api/browsers/{keyID}` | `adminChangeBrowser` | sesiune + CSRF |
| GET / POST | `/admin/api/devices` | `adminDevices` / `adminCreateDevice` | sesiune (+CSRF) |
| POST | `/admin/api/devices/key` | `adminRevealDevice` | sesiune + CSRF; întoarce secretul în clar |
| PATCH / DELETE | `/admin/api/devices/{keyID}` | `adminUpdateDevice` / `adminDeleteDevice` | sesiune + CSRF |

Directorul `wsm-server/web/` (UI de test vechi cu comenzi `stats.get`, `orders.*` etc.) **nu este servit de nicio rută** din `routes()`. [COD]

### Wrapperul HTTP de comenzi (`commandHTTP`, `S/http_commands.go:17-193`) [COD]

- **Discovery**: `GET /api/connections?client_type=…` (snapshot; ordonat după `connected_at`).
- **Body**: `{command, args, timeout_ms}`; câmpuri necunoscute/JSON în plus → 400; body ≤ `max_message_bytes` → 413.
- **Participant temporar**: `hub.NewConnection(nil, tenant, queue)` înregistrat cu `ClientType = claims.Role`, numărat în limitele de conexiuni, vizibil în listări pe durata apelului, eliminat la final (`:73-129`). Nu emite `presence`.
- **Țintă înghețată**: `peer` = conexiunea `{id}` din același tenant, capturată înainte de dispatch (`:131-141`); ID inexistent sau din alt tenant → `404 client_unavailable`.
- **Corelare**: `request_id` = ID-ul conexiunii temporare (aleator 128 biți). Se acceptă numai `type=="reply"` cu `correlation_id == request_id` **și** `connection_id == {id}` (`:147-156`); restul e ignorat.
- **Timeout**: `timeout_ms` implicit 135 000, interval 1..300 000, plafonat la `exp` al JWT-ului; write deadline HTTP extins la `deadline+5s` (`:62-92`).
- **Ce înseamnă HTTP 200**: a sosit un `reply` autentic de la conexiunea selectată. **Nu garantează succesul API-ului remote**: `response.status` poate fi 4xx/5xx, iar `response.ok=false` e un refuz de aplicație (ex. eroare de adaptor). Corpul: `{request_id, connection_id, response:<payload reply>}`.

| Cod | `error` | Semnificație |
|---|---|---|
| 401 | `wsm_*` / `credentials_expired_or_reloaded` | JWT invalid, sau reload/expirare între autentificare și admitere |
| 403 | `forbidden` | rol ≠ service/browser, Origin, sau `authorizeMessage` |
| 400 | `invalid_json` / `invalid_command_or_timeout` | |
| 413 | `request_too_large` | |
| 404 | `client_unavailable` | ID inexistent / alt tenant |
| 503 | `connection_limit`, `registration_failed`, `client_unavailable` (ACK cu `recipients:0`), `request_interrupted_execution_unknown` | |
| 502 | `client_disconnected_execution_unknown` | peer deconectat după trimitere |
| 504 | `timeout_execution_unknown` | |

Dacă apelantul HTTP închide cererea, handlerul iese fără răspuns și **fără a anula** execuția remote (`:186-190`).

---

## 5. Administrare, persistență, `wsmctl`, `logstore`

**Config principal** — YAML strict (`KnownFields`, un singur document, ≤ 1 MiB), `config.Load` (`C/config.go:97-123`); `WSM_SERVER_PORT` suprascrie portul. Tratată ca read-only de proces. Secretele vin din `secret_file`/`secret_env`, cheia API WiseMED din `api_key_file`/`api_key_ref`. [COD]

**Registru scriibil** — `security.device_keys_file` (`C/access.go:28-33`, `C/devices.go:216-259`): YAML cu `devices` (inclusiv `secret` în clar), `browsers` (inclusiv `secret`), `tenant_settings` (`issuer`, `base_url`, `api_key` în clar), `tenant_origins`. Scriere atomică: temp `0600` + `fsync` + `rename` + `fsync` director; dimensiune ≤ 4 MiB. Suprapus peste config la `Prepare`/`RefreshDevices` (`C/devices.go:136-155`). Regula „persist înainte de publicare”: `s.cfg = next` doar după scrierea reușită (`S/admin.go:455-459`, `S/admin_access.go:60-72`). [COD]

**Consola admin** (`S/admin.go`) [COD]:
- activă doar cu `admin.enabled`; cere `device_keys_file` și origini publice HTTPS (HTTP doar loopback explicit) — `C/devices.go:99-131`;
- potrivire strictă `Host` + `Origin` cu o origine configurată (`adminRequestOrigin`, `:96-104`); mutațiile cer `Content-Type: application/json`, `Origin` egal și header `X-CSRF-Token` (comparare în timp constant, `:341`);
- login: `PUT <base_url>/administrative/login` la WiseMED-ul tenantului, cu JWT HS256 semnat cu cheia API a tenantului (`createWiseMedJWT`, `S/wisemed.go:138-153`); acceptă doar `user_type == -1` și prezența unui `login_token`/`token`/`lt` (`:247-304`). Parola nu e stocată;
- sesiune: token aleator 32 bytes, stocat ca `sha256` în `map` în memorie, TTL `session_ttl_seconds` (implicit 900, 60..3600), max 1024; cookie `wsm_admin` `Path=/admin; HttpOnly; SameSite=Strict; Secure` (pe HTTPS) (`:242-244,305-311`);
- rate limit login: 10 / 5 min per IP, 60 / min global, max 1024 IP-uri urmărite (`:139-168`). IP-ul este `RemoteAddr` (în spatele unui proxy = IP-ul proxy-ului) [INF];
- sesiunea e legată de un singur tenant; toate operațiile filtrează pe `sess.TenantID` (ex. `:381,480,500,556`).

Operații: device create/reveal/update(binding, scopes)/delete; browser create/reveal/patch(scopes, origin, rotate)/delete; tenant `issuer`/`base_url`/`api_key`; origini add/delete. Cheile din config-ul principal sunt read-only în consolă (`S/admin_access.go:194-198`).

**`wsmctl`** (`cmd/wsmctl/main.go`) — utilitar **offline**, nu contactează serverul: `keygen -out FILE` (32 bytes aleatori base64url, `O_EXCL`, `0600`) și `token …` (semnează un JWT HS256 din `-secret-file`; TTL implicit 5 min, 1 s..24 h). [COD]

**`logstore`** (`wsm-server/internal/logstore/store.go`) [COD]:
- oglindește `log.Printf` către stderr și către `server.log_dir/wsm-YYYY-MM-DD.log` (UTC, `0600`, director `0700`); `log_dir` implicit = `<dir device_keys_file>/logs` sau `../logs` față de config (`C/config.go:202-210`);
- persistă doar loguri operaționale text (conectări, respingeri, acțiuni admin cu `tenant=…`, `key=…`); codul evită explicit logarea payloadurilor, query-urilor, headerelor (`S/server.go:133,286`);
- citire paginată (cursor în bytes, max 256 KiB/pagină, linii > 64 KiB omise) cu filtru de vizibilitate: o linie cu `tenant=X` e vizibilă doar adminului tenantului X; liniile fără marcaj de tenant sunt vizibile **tuturor** adminilor (`visible`, `:134-141`);
- fără rotație/ștergere automată a fișierelor vechi [COD — absent].

Istoricul comenzilor din consola admin se ține în `localStorage` al browserului (`S/adminui/app.js:96`), nu pe server. [COD]

---

## 6. Clientul din readere — `readersv3/modules/ws`

ID modul `wisemed-ws`; servicii înregistrate: `ws-action-dispatcher`, `wisemed-ws-status`, `wisemed-ws-client` (`WS:126-144`). Inclus în 31 de `apps/*/main.go` (numărat cu grep). [COD]

**Config** (`modules.wisemed-ws`, salvată în YAML-ul aplicației; `SaveSettings`, `WS:158-212`): `enabled`, `url` (doar `wss://`, fără query/credențiale), `auth_mode` (`device_key` implicit | `token_file` | `token_endpoint`), `tenant_id`, `key_id`, `issuer`, `audience`, `subject`, `client_id`, `secret_file`, `device_secret` (write-only, ≥ 32 bytes; nu e returnat de `Settings()`), `token_file`, `token_path`, `ca_file`, `scopes`. `reconnect_delay_ms` e scris mereu ca 30000, dar **nu este citit** nicăieri (`intFromSettings` nefolosit; pauza e hard-codată `WS:319`); la fel `heartbeat_ms` (ping hard-codat 15 s, `WS:463`). [COD]

**Când se conectează** (`Start`/`runSession`, `WS:255-461`) [COD]:
1. doar dacă `enabled != "false"` și `url` nevid; altfel fază `disabled`;
2. obține serviciul `wisemed-api` și apelează `EnsureEquipmentInitialized()` — adică `PUT /administrative/analyzer` prin HTTP WiseMED (`modules/wisemedapi/module.go:332-355`); eșec ⇒ nu se conectează;
3. cere `echipament_id` nevid și ≠ `"0"` (`WS:356-359`).
- Excepție: aplicațiile „login-only” (`html-print-formatter`, `wisemed-ai`) sar peste înregistrare (`registration_skipped`, `wisemedapi/module.go:346-348`), dar tot au nevoie de un `echipament_id` în setări pentru WSS. Cum ajunge acel ID acolo pentru aceste utilitare: [NEC] (vezi `localhttp/siui_session_test.go:39 TestHistoryUsesEquipmentSetup`, necitit în detaliu).
- Nu există mod „standalone” fără WiseMED pentru WSS. [COD]

**De unde vine JWT-ul** (`accessToken`, `WS:717-761`) [COD]:

| `auth_mode` | Sursă |
|---|---|
| `device_key` | readerul **își semnează singur** un JWT HS256 (`exp = now+5min`, fără `nbf`) cu, în ordine: `device_secret` → `secret_file` → `api_key_echipament` (cheia primită de la WiseMED la înregistrare). Claims: `tenant_id`, `iss`, `aud`, `sub` (implicit `ReaderID`), `client_id`, `reader_id`, `equipment_id`, `role` (`reader`, sau `service` pentru protocoalele `siui`/`siui-history`), `scopes` (implicit `route:reply,route:event,route:command,devices:debug,connections:read,server:status`) |
| `token_file` | conținutul fișierului, citit la fiecare încercare |
| `token_endpoint` | `POST <WiseMED>/<token_path>` cu `{reader_id, client_id, equipment_id, role, client_type}`, autentificat cu JWT-ul API WiseMED; așteaptă `{token}` (`modules/wisemedapi/wss.go:15-51`). **Endpointul din backendul WiseMED este extern acestui repo** — [NEC] dacă există |

Transport: `Authorization: Bearer`, TLS ≥ 1.2 cu verificare (CA suplimentar opțional `ca_file`), `HandshakeTimeout` 10 s, read limit 8 MiB (`WS:368-396`).

**Reconectare/backoff** [COD]: pauză **fixă de 30 s** după orice sfârșit de sesiune (`WS:319`), fără backoff exponențial sau jitter. `Reconnect()` (manual sau după `ws.reconnect`) anulează așteptarea; `Disconnect()` pune pauză până la `Reconnect()`. La sfârșitul sesiunii `failPending()` rezolvă toate cererile în așteptare cu eroarea „WSS disconnected” (`WS:617-626`). Keepalive: Ping la 15 s, read deadline 60 s resetat la pong și la fiecare mesaj.

**Consecință [INF, din două fapte COD]:** în `device_key`, tokenul expiră la 5 min și serverul închide socketul la `exp` (`S/server.go:265`); readerul nu reînnoiește tokenul pe conexiunea existentă ⇒ fiecare reader e deconectat la ~5 min și offline ~30 s până la reîncercare (≈ 9% din timp), cu pierderea comenzilor în curs. Nu am găsit cod sau documentație care să atenueze acest ciclu.

**Comenzi tratate** (`handleCommandContext`, `WS:506-551`; dispatcher `WS:54-65`):

| `payload.command` | Handler | Condiții suplimentare în reader |
|---|---|---|
| `api.request` | `apibridge.Invoke` pe `rt.Mux()` (`WS:512-533`) | `sender_scopes` trebuie să conțină `api:invoke` |
| `reader.status`, `ws.status`, `wisemed.ws.status` | `Module.Status()` (`WS:554`) | niciuna |
| `device.ping` | `{ok, equipment_id, time}` (`WS:556`) | niciuna |
| `ws.reconnect` | răspunde, apoi se reconectează după scrierea reply-ului (`WS:558`, `WS:476-479`) | niciuna |
| `debug.message` | ecou `received` (`WS:560`) | niciuna |
| `barcode.print`, `barcode/print`, `print_barcode` | `barcodeprinter.HandleWSAction` (`modules/barcodeprinter/module.go:100-117`) | niciuna (doar în aplicațiile cu modulul) |
| `barcode.printers`, `barcode/list_printers` | idem `:118` | niciuna |
| `esignature.health|devices|start|retry|confirm|cancel` | `signingpad.HandleWSAction` (`modules/signingpad/remote.go:29`) | `session_id` 16–128 caractere (în afară de health/devices) |
| orice altceva | reply `{ok:false,error:{message:"unknown action"}}` | — |

Niciun handler de dispatcher nu verifică scope-uri; autorizarea lor este doar `route:command` la WSM (și restricțiile rolului `reader` pentru expeditor). [COD]

Execuție: o singură gorutină worker per sesiune, **serială**, coadă `commands` de 16; coadă plină ⇒ sesiunea e închisă cu „WSS command queue is full” (`WS:428-455`). Reply-ul: `type:"reply"`, `correlation_id = request_id`, `target = {mode:"connection", connection_id: sender_connection_id}`. Coada de trimitere 128; plină ⇒ eroare „WSS queue is full” (reply pierdut) (`WS:627-645`).

**Mesaje emise de reader** [COD]:
- `reply` la comenzi;
- `command mode:"server"`: `control.ticket` (`localhttp/wss.go:137`), `wisemed.ensure_equipment_online`, `wisemed.fetch_file_for_analyzer` (`wisemedapi/module.go:508-540`, cu fallback pe HTTP direct);
- `list_connections`, `ping` (Debug WSM, `WS:589-603`; `siuidata/bridge_sync.go:47`);
- `command` către peer: `device.ping`/`ws.reconnect`/`debug.message`, `api.request` (`/api/wss/bridge`);
- `event` către o conexiune: `PublishEventToConnection` (`modules/ws/events.go:8-13`), folosit doar de `wisemedai/events.go:28` (`event:"wisemed-ai.<…>"`). [INF]: pentru rolul `reader`, WSM respinge `event` cu `mode:"connection"` (`S/server.go:475`), deci funcționează doar dacă acea aplicație rulează cu rol `service`/alt rol — neverificat.
- **Nu există** emitere pe topicurile `results:<reader_id>` / `logs:<reader_id>` în `readersv3` (grep fără rezultate), deși WSM le autorizează.

Trace de diagnostic: ultimele 200 de mesaje, în memorie, doar câmpuri sigure (`WS:646-665`).

---

## 7. `api.request` → `shared/apibridge`

`apibridge.Invoke(ctx, handler, principal, request)` — `AB:51-162`. [COD]

- **Nu este proxy**: construiește `http.NewRequestWithContext(ctx, method, in.Path, body)` și apelează direct `handler.ServeHTTP(recorder, req)` pe `m.rt.Mux()` — același `*http.ServeMux` pe care îl servește `local-http` (`core/runtime/runtime.go:77-95`, `LH:323`). Nicio conexiune de rețea, niciun host din cerere. Test existent, nerulat: `bridge_test.go:15 TestSameHandlerAndIdentity`.
- **Validare cale** (`AB:55-65`): relativă, începe cu `/`, nu `//`, fără host/schemă, fără `\`, NUL, `/../`, `/./`; doar `/api/…` sau exact `/barcode/print`. Excluse: `/api/wss/bridge*`, `/api/wss/open`, `/api/wss/debug`, `/api/session/login`, `/api/session/logout`.
- **Metode**: GET, POST, PUT, PATCH, DELETE, HEAD (`AB:66-71`).
- **Corp**: `body` (JSON brut) xor `body_base64` + `content_type`; ≤ 4 MiB decodat (`AB:18,72-87`). Răspuns ≤ 4 MiB, altfel eroare „API response exceeds WSS bridge limit” (`AB:177-186`).
- **Răspuns** (`AB:138-161`): `{kind:"api.response", status, content_type, headers{Content-Disposition,ETag,Location}, body | body_base64}`. JSON valid → `body`; altfel base64. `text/html` este refuzat. Panică în handler → eroare „API handler failed”.
- **Streaming / fragmentare / upgrade**: inexistente la nivel de adaptor; răspunsul e bufferizat integral într-un singur mesaj WSS. Fișierele mari se transferă la nivel de aplicație prin chunk-uri (ex. `getCallFileChunk` în `S/sdk/siui-bridge.js:295-326`). Plafon efectiv suplimentar: `max_message_bytes` al WSM (1 MiB implicit; 8 MiB în șablonul de producție) și read limit 8 MiB în reader. [COD]
- **Timeout** (`AB:95-110`): 30 s implicit; 610 s pentru `POST /api/html-print-formatter/{render,pdf}`; 130 s pentru `POST /api/siui/general/ocsp`, `/api/siui/ocsp-test`, `/api/siui/invoice/package`; 310 s pentru `POST /api/history/calls/*/commit`. Contextul derivă din sesiunea WSS ⇒ deconectarea anulează contextul cererii interne.
- **Identitate / autorizare** [COD]:
  - `Principal{Subject, TenantID, ConnectionID, SourceIP, Admin}` construit din `sender_subject`, `sender_tenant_id`, `sender_connection_id`, `sender_ip` și `sender_scopes` (ștampilate de WSM) — `WS:517-528`. Readerul verifică local `api:invoke`.
  - `currentSession` (`LH:2566-2573`) tratează orice cerere cu principal ca **sesiune autentificată**: `ID="wss:<conn>"`, `Username=sub`, `UserType = 0` dacă `api:admin`, altfel `1`. Deci `api:invoke` ≡ utilizator local autentificat non-admin; `api:admin` ≡ admin local (`UserType <= 0`).
  - `RemoteAddr = "wss:<connection_id>"`, `Accept: application/json`; nu se transmit cookie-uri/headere de la apelant.
  - Nu există token de login WiseMED asociat (`sessionAuth{}` gol, `LH:2596-2599`) — handlerii care au nevoie de tokenul utilizatorului WiseMED nu îl au pe această cale. [INF]
  - Readerul **nu** verifică `sender_tenant_id` față de propriul tenant (se bazează pe izolarea WSM); unele module o fac explicit (ex. `siuihistory/module.go:157`). [COD]
- **`request_id` vs `correlation_id`**: apelantul pune un `request_id` unic pe `command`; WSM îl copiază în `command_ack` (ambele câmpuri); readerul răspunde cu `reply.correlation_id = request_id` (fără `request_id` propriu). Clienții corelează după `correlation_id || request_id` (`WS:605`, `S/sdk/wsm.js:117`). Wrapperul HTTP generează el `request_id`.

---

## 8. Control remote

- **Unde e servită interfața**: central, de WSM, la `/control/` + cele 5 assets, din `server.control_ui_dir` (`S/control.go:93-126`). Sunt aceleași fișiere ca UI-ul local al readerului (`readersv3/modules/localhttp/ui/`), plus `wss-remote.js` care interceptează `fetch` către `/api/…` și `/barcode/print` și le transformă în `api.request` (`ui/wss-remote.js:77,125`). `output/wsm-browser-sdk/control/wss-remote.js` este identic cu sursa (verificat cu `cmp`). [COD]
- **Cum circulă datele**: exclusiv WSS: browser → WSM → reader (`command api.request`, `mode:"equipment"`) și înapoi `reply` 1:1. Niciun HTTP către IP-ul readerului. [COD]
- **Tichetul** (`issueControlTicket`, `S/control.go:24-70`) [COD]:
  - cerut prin `command mode:"server" control.ticket {equipment_id}`; cere `api:invoke` + `route:command`; echipamentul trebuie să fie online; sesiunile de control nu pot delega;
  - 32 bytes aleatori; valabil 1 min pentru consum, **o singură utilizare** (`delete` la citire, `:75-78`); max 1024 în așteptare; invalid după schimbarea `authEpoch`;
  - sesiunea rezultată: `role=browser`, `client_id="control"`, `ControlEquipmentID=<eq>`, scope-uri `api:invoke`, `route:command` (+ `api:admin` **doar dacă părintele îl are**), `exp = min(now+5min, exp părinte)`;
  - Origin obligatoriu `https://<Host>`; subprotocol `wsm.ticket.<ticket>`;
  - permis doar `ping` și `api.request` către acel echipament (`S/server.go:389-391`). Test existent, nerulat: `server_test.go:547`.
  - Local, `POST /api/wss/open` cere admin local (`UserType <= 0`) și întoarce URL-ul `/control/?equipment_id=…&opener_origin=…` + tichetul; tichetul trece prin `postMessage`, nu prin URL (`localhttp/wss.go:41-44,136-155`; `ui/wss-remote.js:96-132`).
- **Scope-uri și ce NU acordă** [COD]:

| Scope | Acordă | Nu acordă |
|---|---|---|
| `route:command` | trimiterea de comenzi; pentru `browser`/`service` orice nume de comandă (deci și `barcode.print`, `esignature.*`, `ws.reconnect`, `device.ping`) | `api.request` (cere `api:invoke`) |
| `api:invoke` | `api.request` + emiterea `control.ticket`; în reader ≡ sesiune locală autentificată non-admin pe toate rutele `/api/…` cu `requireSession` | rutele locale care cer admin; login/logout; `/api/wss/{open,debug,bridge}` |
| `api:admin` | `UserType=0` în reader: `/api/wss/settings`, `/api/wss/control`, `/api/debug/tests*` etc. | **login în consola admin WSM** (aceea cere cont WiseMED `user_type=-1`); acces la alt tenant; modificarea configurației WSM |
| `devices:debug` | pentru rol `reader`: `device.ping`/`ws.reconnect`/`debug.message` către peer | API |
| `connections:read` | listări/prezență | control |
| `wisemed:proxy` | cele două comenzi proxy către upstreamul tenantului (URL-uri fixe, `S/wisemed.go:55-71`) | proxy către adrese arbitrare |

---

## 9. SDK JS și wrapper PHP

### `WSMServer` — `S/sdk/wsm.js` (servit la `/sdk/wsm.js`) [COD]

API public: `WSMServer(url, token, options)` / `WSMServer.Client`; opțiuni `timeout` 35 000, `connectTimeout` 10 000, `reconnectDelay` 30 000, `autoReconnect`, `autoConnect`, `tokenProvider`. Metode: `connect`, `disconnect`, `reconnect`, `ping`, `serverCommand`, `serverStatus`, `equipmentStatus`, `controlTicket`, `listClients`, `getClientsByType`, `command`, `callAPI`, `subscribe`, `unsubscribe`, `send`, `reply`. Callback-uri: `onConnect`, `onDisconnect`, `onError`, `onTokenRequired`, `onEvent`, `onCommand`, `onReply`, `onPresence`, `onMessage`. Proprietăți: `ready`, `isConnected`, `connectionID`, `clients`.

Paritate cu protocolul:
- folosește subprotocoalele `wsm.v1` + `wsm.jwt.<JWT>` și `hello {client_type:"browser", client_id}`; **acceptă doar JWT cu `role=browser`** (`:62`) — deci nu poate fi folosit ca `service`;
- `command()` țintește mereu `mode:"connection"`; așteaptă `reply` de la **exact acel peer** (`:126`), tratează `command_ack recipients:0` ca `no_recipient`, `error` de server ca eșec, `payload.ok===false` ca `operation_failed`;
- `callAPI` aruncă `http_error` pentru status non-2xx (spre deosebire de PHP `callAPI`, care păstrează statusul);
- max 256 cereri în așteptare; timeout cu mesaj explicit „execution may have occurred. Do not automatically replay”;
- reconectarea **nu** retrimite cereri; fără `tokenProvider` și cu token expirat emite `onTokenRequired`;
- nu sunt expuse: ținte `equipment`/`reader`/liste în `command()` (doar prin `send()`, care întoarce doar ACK), `wisemed.*` (doar generic prin `serverCommand`).

`siui-bridge.js` (`SIUIBridge`): strat peste `WSMServer` pentru operațiile SIUI (`executeOperation`, `submitOperation`, `waitCall`, `getCall`, `completeCall`, `downloadCallFile`, `getCallFileChunk`, `retryCallWMAPI`, `getValidation`, `waitValidation`, `downloadDocMedAttachment` + metode generate per operație) — detaliile aparțin raportului SIUI.

### `WSMHttpClient` — `wsm-server/examples/php/WSMHttpClient.php` [COD]

API public: constructor `(baseURL https, jwt, options, caFile)`, `setTimeouts`, `setReconnectOptions`, `getOptions`, `listClients`, `firstClient`, `sendCommand`, `callAPI`; excepție `WSMHttpException{errorCode, requestId, executionUnknown, httpStatus}`. Paritate: mapare 1:1 pe `GET /api/connections` și `POST /api/connections/{id}/commands`; `timeout_ms = min(requestTimeout, totalTimeout)`; verificare TLS activă; retry **doar** pe GET (erori cURL 6/7/28/52/55/56), POST o singură dată; `response.ok===false` → excepție `command_failed`; `callAPI` nu aruncă pe status remote non-2xx. Test existent, nerulat: `S/php_http_client_test.go:18`.

### Ce este exemplu și ce este extern

| Artefact | Statut |
|---|---|
| `examples/browser/wsm-token.php` (`wsmBrowserToken`) | exemplu de emitere JWT browser; test existent, nerulat `browser_sdk_test.go:104` arată că formatul e acceptat de WSM. **Nu dovedește** că backendul WiseMED îl folosește. |
| `examples/browser/token-endpoint.php` | exemplu ilustrativ (`$_SESSION['user_id']`, `wsm_access_allowed` sunt fictive). Endpointul real de token din WiseMED: **EXTERN, [NEC]**. |
| `examples/php/siui-server.php`, `examples/browser/siui-*.js`, `wisemed-page.html` | exemple de integrare; implementarea din WiseMED/ACE: **EXTERN, [NEC]**. |
| `examples/reader/main.go` | demo minimal (`reader.status`), nu runtime-ul real. |
| `token_endpoint` (`token_path`) din reader | ruta trebuie implementată de backendul WiseMED: **EXTERN, [NEC]**. |
| `PUT /administrative/login`, `PUT /administrative/analyzer`, `GET /fileforanalyzer/…` | apelate de WSM/reader; implementarea: **EXTERN**. |
| `output/wsm-browser-sdk/` | pachet de distribuție (binare, `control/`, docs, exemple, șabloane de config). Binarele nu au fost inspectate. |

---

## 10. Autentificarea locală a readerelor

`readersv3/modules/login/module.go` este un **placeholder**: o singură rută `/api/login/meta` cu răspuns static. Autentificarea reală este în `modules/localhttp/module.go`. [COD]

**Login** — `POST /api/session/login` (`handleLogin`, `LH:1106-1170`): `{username, password, medical_unit_id}` → `wiseMED.Login(...)` (validare la backendul WiseMED, extern) → `EnsureEquipmentOnline` → sesiune 12 h. Cere `SetupComplete()`. Nu există conturi locale; `createSession(username)` (`LH:2518`) nu are apelanți găsiți. Nu am găsit rate limiting pe login. [COD]

**Sesiune / cookie** [COD]:
- cookie `wmr_local_session_<sha256(ReaderID)[:6] hex>` (+ acceptat și numele vechi `wmr_local_session`), `Path=/; HttpOnly; SameSite=Lax`, **fără `Secure`** (`LH:1158-1165,2667-2670`);
- valoarea = `base64url(JSON sesiune) + "." + base64url(HMAC-SHA256)` (`LH:2672-2710`);
- **cheia HMAC** (`sessionSecret`, `LH:2660-2665`): 32 bytes aleatori per proces doar pentru protocoalele `siui`, `siui-history`, `siui-data` și utilitarele login-only; pentru **toate celelalte readere** este șirul `"wisemed-local-session:" + ReaderID` — valoare nesecretă, derivabilă (ReaderID apare neautentificat în `GET /api/session` → `reader.id`, `LH:652,2724`);
- `currentSession` acceptă orice token cu semnătură validă și `ExpiresAt` în viitor **fără** să consulte `m.sessions` (`LH:2578-2580`). Consecințe: (a) oricine poate ajunge la portul local poate fabrica un cookie cu `UserType:-1` (admin); (b) `logout` șterge doar intrarea din map și cookie-ul din browser, tokenul rămâne valid până la expirare. Testul `localhttp/siui_session_test.go:26-34` numește explicit cheia veche „predictable legacy signing key” și verifică respingerea ei doar pentru SIUI/formatter.
- `sessionAuth` (login token WiseMED + user id) e ținut doar în memorie, indexat pe ID-ul sesiunii (`LH:2558-2561`).

**Roluri locale**: `UserType` din WiseMED; `<= 0` = admin (`requireDebugSession`, `debug_replay.go:21-34`; `/api/wss/*` cu excepția `status`, `localhttp/wss.go:41-44`). Restul rutelor `/api/…` cer doar sesiune (`requireSession`, `LH:568-577`).

**Rute fără sesiune** (din `Init`, `LH:218-245`): UI static, `GET /api/session`, `/api/preferences*`, `/api/app-update/status`, `POST /api/session/login`, `GET /api/wisemed/bootstrap`, și `/api/wisemed/setup` — **inclusiv `PUT`** pentru readerele obișnuite (sesiunea e cerută doar pentru utilitarele login-only deja configurate, `LH:1188-1194`). [COD] Modulele proprii (ex. barcode `/api/barcode/*` cu `withCORS`) își gestionează separat accesul — în afara acestui raport.

**CORS local**: origini permise din `cors_allowed_origins`, implicit `https://ldse.wisemed.eu`; `*` acceptat ca wildcard; cu `Access-Control-Allow-Credentials: true` și reflectarea headerelor cerute (`LH:213,464-523`). Mutațiile `/api/wss/*` cer în plus `Content-Type: application/json` și Origin same-host dacă e prezent (`localhttp/wss.go:52-64`). [COD]

**Bind și TLS local** [COD]:
- adresă implicită `127.0.0.1:18080` (`LH:525-531`, `core/config/config.go:98`); configurabilă (poate fi pusă pe alte interfețe);
- TLS opțional (`modules.local-http.tls`): `shared/localtls` generează un CA local ECDSA (10 ani) și un certificat de server (2 ani) pentru hosturile adresei, chei `0600`, și încearcă instalarea CA-ului în trust store-ul sistemului (darwin/linux/windows) — `shared/localtls/tls.go:35-64,97-190`, `trust_*.go`;
- **`shared/bindguard` nu este un mecanism de securitate**: tratează doar „address already in use” căutând un port liber în `port+1..port+50` și rescriind `config.yaml` (`shared/bindguard/bindguard.go:14-77`, apelat din `LH:413-437`). Nu restricționează interfața de ascultare.

**Trei mecanisme distincte de autentificare** [COD]:

| Mecanism | Emitent | Cheie | Unde e verificat | Scop |
|---|---|---|---|---|
| JWT WSM | backend WiseMED / reader (`device_key`) / `wsmctl` | secret per `kid` din config/registru WSM | `S/auth.go:65` | acces la hubul WSM; devine `Principal` în reader prin `api.request` |
| Cookie local | readerul, după login WiseMED | `sessionSecret()` (vezi mai sus) | `LH:2566-2593` | UI/API local în browser |
| JWT local SIUI | `siui` (`tokenHTTP`), `iss=siui-bridge`, `aud=siui-api`, ≤ 900 s | `m.jwtKey` al modulului | `readersv3/modules/siui/auth.go:28-49,58-93` | doar rutele SIUI, doar pe HTTPS; detalii în raportul SIUI |

---

## 11. Izolarea per tenant

Unde este impusă [COD]:

| Punct | Mecanism | Referință |
|---|---|---|
| Autentificare | `claims.tenant_id == Key.TenantID`; secrete distincte între tenanturi | `S/auth.go:117`, `C/config.go:280-282` |
| Conexiune | `Connection.TenantID` fixat din claims la creare, imuabil | `S/server.go:246` |
| Rutare | `hub.route` filtrează `c.TenantID == tenant` înaintea oricărui predicat | `S/hub.go:152-160` |
| Listări / prezență / statistici | `Snapshot(tenant)`, `Stats(tenant)`, `Broadcast(tenant, …)` | `S/hub.go:98-120` |
| Unicitate reader/echipament | comparată doar în același tenant | `S/hub.go:66` |
| HTTP commands | `peer.TenantID != claims.TenantID` ⇒ 404 | `S/http_commands.go:131-141` |
| Tichete | moștenesc `TenantID` al părintelui | `S/control.go:47` |
| Upstream WiseMED | `cfg.Tenants[claims.TenantID]` — nu poate fi furnizat de client | `S/server.go:209,365`, `S/wisemed.go:55-71` |
| Admin | toate operațiile filtrează pe `sess.TenantID` | `S/admin.go`, `S/admin_access.go` |
| Loguri | filtrare pe `tenant=` | `logstore/store.go:134-141` |
| Topicuri | abonații sunt filtrați tot pe tenant ⇒ același nume de topic e izolat | `S/hub.go:144,156` |

**Pot două tenanturi să se vadă?** La nivel de date/rutare: **nu** am găsit nicio cale [COD]; teste existente, nerulate: `hub_test.go:14 TestHubTenantIsolationEveryRoute`, `http_commands_test.go:68`, `server_test.go:168`.

Scurgeri/efecte laterale între tenanturi găsite [COD]:
1. **`authEpoch` global** (`S/server.go:26`): `revokeTenantAccess` (`S/admin_access.go:347-363`) îl incrementează pentru orice schimbare de cheie browser / ștergere origine / setări tenant. Efect asupra **altor** tenanturi: fiecare conexiune e terminată la următorul mesaj JSON pe care îl trimite (`S/server.go:311-314`, fără `presence` și cu pierderea acelui mesaj — de regulă un `reply`); apelurile `commandHTTP` în curs primesc 401/503 (`S/http_commands.go:95,159`); tichetele de control neutilizate devin invalide (`S/control.go:79`). Este un impact de disponibilitate, nu de confidențialitate.
2. `GET /admin/api/tenants` expune fără sesiune lista ID-urilor de tenant cu upstream configurat (`S/admin.go:125-138`).
3. Liniile de log fără `tenant=` (ex. respingeri de autentificare cu `peer`/`origin`, `S/auth.go:48`) sunt vizibile adminilor oricărui tenant.
4. Limita globală `max_connections` și rate limit-ul global de login sunt partajate.

În interiorul unui tenant **nu există ACL** per utilizator/pacient/echipament (în afara tichetelor de control și a restricțiilor de rol `reader`): orice principal cu `route:command`+`api:invoke` poate apela API-ul oricărui echipament din tenant. [COD]

---

## 12. Concurență în hub; restart

**Gorutine** [COD]:
- per conexiune WSS: gorutina handlerului HTTP (bucla de citire, `S/server.go:283`) + `writePump` (`S/server.go:268,567-586`) + timer `AfterFunc` pentru expirare;
- per `commandHTTP`: gorutina handlerului (fără `writePump`; citește direct din `c.send`);
- globale: watcher shutdown (`S/server.go:99-109`), watcher `SIGHUP` (`reload_unix.go:19`);
- hubul nu are gorutină proprie; rutarea se face în gorutina expeditorului.

**Mutexuri** [COD]:

| Lock | Protejează |
|---|---|
| `Server.gate` (RWMutex) | `cfg`, `authEpoch`; RLock la admitere și la fiecare dispatch; Lock la reload și mutații admin (`S/server.go:29`) |
| `Server.activeMu` | `active` (toate socketurile, inclusiv pre-hello), `reserved` (contoare per tenant), `stopping` |
| `Server.ticketMu` | `tickets` |
| `adminState.mu` | sesiuni admin, ferestre de rate limit, `epoch` |
| `Hub.mu` (RWMutex) | `connections`, `stats`, `LastSeenAt`, `Topics` |
| `logstore.Store.mu` | fișierul de log curent |

**Canale**: `Connection.send` (buffer `send_queue_size`), `Connection.closed` (închis o dată prin `closeOnce`), `writerDone`. Trimiterea e neblocantă (`select … default`) ⇒ fără backpressure către expeditor; consumatorul lent e deconectat (`S/hub.go:195-217`).

Observații [COD/INF]:
- comenzile proxy WiseMED rulează în afara `gate`, dar blochează bucla de citire a conexiunii respective până la 20 s (`S/server.go:358-373`, client HTTP cu timeout 20 s `S/server.go:40`);
- `Hub.Register` iterează toate conexiunile sub lock exclusiv (O(n) per hello);
- [INF] un participant HTTP temporar creat cu un JWT de device (rol `service`, cu `equipment_id`) ocupă pe durata apelului „locul” acelui `equipment_id` în verificarea de unicitate (`S/hub.go:66` compară `other.EquipmentID`), deci o reconectare a readerului în acel interval ar fi respinsă cu „reader identity already connected”.

**Restart WSM** [COD]: se pierd toate conexiunile, abonamentele, prezența, statisticile `HubStats`, tichetele de control, sesiunile admin, ferestrele de rate limit și orice mesaj aflat în cozi. Persistă doar: config-ul, registrul `device_keys_file`, fișierele de log. Clienții trebuie să se reconecteze (readere: la 30 s), să retrimită `hello`, să refacă abonamentele. `Close()` la shutdown închide toate socketurile, apoi `srv.Shutdown` cu 5 s (`S/server.go:78-87,99-109`).

---

## 13. Timeout, anulare, retrimitere

| Situație | Comportament | Etichetă / referință |
|---|---|---|
| Apelantul WSS renunță la așteptare (timeout SDK, `AbortSignal`, `ctx` în `Exchange`) | doar se șterge intrarea locală din `pending`; **nu există mesaj de anulare în protocol**; execuția remote continuă, reply-ul ulterior e ignorat | [COD] `S/sdk/wsm.js:145`, `WS:568-588`; niciun tip `cancel` în `S/server.go:531-564` |
| Apelantul HTTP închide cererea `POST …/commands` | handlerul iese; participantul temporar e eliminat; comanda deja rutată se execută; reply-ul ajunge la o conexiune inexistentă (`recipients:0` pentru reader) | [COD] `S/http_commands.go:186-190,114-124` |
| Timeout `commandHTTP` | `504 timeout_execution_unknown` | [COD] `:188` |
| Peer deconectat după rutare | `502 client_disconnected_execution_unknown` | [COD] `:173-182` |
| Readerul se deconectează în timpul unui `api.request` | contextul sesiunii e anulat ⇒ contextul cererii interne e anulat (handlerii care respectă `r.Context()` se opresc; ceilalți termină); reply-ul nu mai poate fi trimis | [COD] `WS:411-441`, `AB:109-112`; efectul asupra fiecărui handler: [INF] |
| Timeout intern `apibridge` (30/130/310/610 s) | contextul expiră; handlerul sincron nu e întrerupt forțat — `Invoke` întoarce abia când `ServeHTTP` revine | [COD] `AB:109-126` |
| Reconectarea readerului | **nu retrimite nimic**: `failPending()` eșuează cererile proprii în așteptare; comenzile primite și neexecutate din coada de 16 se pierd; nu există jurnal de comenzi | [COD] `WS:328,617-626` |
| Reconectarea SDK-ului JS | toate promisiunile în așteptare sunt respinse cu `disconnected`; fără replay | [COD] `S/sdk/wsm.js:96-102` |
| Wrapper PHP | POST niciodată retrimis; GET discovery retrimis de max `connectAttempts` ori | [COD] `WSMHttpClient.php:128-168` |
| Reply fără destinatar (apelantul s-a reconectat ⇒ alt `connection_id`) | `command_ack recipients:0`; readerul ignoră ACK-urile pentru reply-uri (nu are intrare `pending` pentru ele) ⇒ rezultatul e pierdut | [COD] `WS:489-495,604-616` |
| Idempotență | nu există la nivel de transport; `request_id` nu e deduplicat; rămâne responsabilitatea handlerului/aplicației | [COD] absent; [DOC] `docs/protocol.md:241-243` |

---

## 14. Matricea comunicațiilor

| # | Sursă → Destinație | Transport | Endpoint / comandă | Autentificare | Autorizare | Payload | Răspuns | Persistență | Timeout | Retry / idempotență | Erori |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 1 | Reader → WiseMED | HTTPS | `PUT /administrative/analyzer` (înregistrare înainte de WSS) | JWT API WiseMED (modul `wisemedapi`) | backend extern | date analizor | `echipament_id`, `api_key_echipament`… | config reader | clientul HTTP al modulului [NEC] | reîncercat la fiecare sesiune WSS dacă nu e inițializat | blochează conectarea WSS |
| 2 | Reader → WiseMED | HTTPS | `POST <token_path>` (mod `token_endpoint`) | JWT API WiseMED | backend extern [NEC] | `{reader_id, client_id, equipment_id, role, client_type}` | `{token}` | — | ctx sesiune | la fiecare încercare (30 s) | „token request rejected” |
| 3 | Reader → WSM | WSS (TLS ≥1.2) | `GET /ws` + `hello` | `Authorization: Bearer <JWT HS256>` | cheie `kid`, rol, scope-uri, unicitate reader/echipament | `hello{client_type, client_id, reader_id, equipment_id}` | `hello_ack` | memorie | handshake 10 s; hello 10 s; lease = `exp` | pauză fixă 30 s; manual imediat | 401 `wsm_*`, 403, 503, close 1008 |
| 4 | Browser → WSM | WSS | `GET /ws` + `hello` | subprotocol `wsm.jwt.<JWT>` (rol `browser`) | Origin ∈ `allowed_origins`; scope-uri | `hello{client_type:"browser", client_id}` | `hello_ack` | memorie | SDK `connectTimeout` 10 s | SDK: 30 s, fără replay | idem |
| 5 | Browser/Service → Reader (prin WSM) | WSS | `command api.request` (`connection`/`equipment`/`reader`) | JWT expeditor | WSM: `route:command`+`api:invoke`; reader: `api:invoke` în `sender_scopes`; `api:admin` ⇒ admin local | `args{method, path, body|body_base64, content_type}` ≤ 4 MiB | `command_ack` (coadă) apoi `reply{kind:"api.response", status, body|body_base64}` | niciuna la WSM; conform handlerului local | SDK 35 s; reader 30/130/310/610 s | fără retry; fără dedup; handler-dependent | `error forbidden`; `recipients:0`; reply `ok:false`; `status` 4xx/5xx |
| 6 | Browser/Service → Reader | WSS | `command` dispatcher: `reader.status`, `device.ping`, `ws.reconnect`, `debug.message`, `barcode.*`, `esignature.*` | JWT | doar `route:command` (rol `reader` expeditor: `devices:debug` și doar cele 3 de diagnostic) | `args{…}` | `reply{…}` / `{ok:false,error}` | conform handlerului (ex. job imprimare) | SDK 35 s; `esignature` până la timeout pad | fără | „unknown action” |
| 7 | Backend PHP → WSM | HTTPS | `GET /api/connections[?client_type]` | Bearer JWT | `connections:read`, Origin | — | `{tenant_id, connections, count, stats}` | — | PHP `connectTimeout` 5 s | GET ≤ 3 încercări | 401/403 |
| 8 | Backend PHP → Reader (prin WSM) | HTTPS → WSS | `POST /api/connections/{id}/commands` | Bearer JWT (rol `service`/`browser`) | ca la #5/#6, prin participant temporar | `{command, args, timeout_ms}` | 200 `{request_id, connection_id, response}` | — | 135 s implicit, ≤ 300 s, ≤ `exp` | POST niciodată retrimis | 400/401/403/404/413/502/503/504 (`*_execution_unknown`) |
| 9 | Reader (admin local) → WSM | WSS | `command mode:server control.ticket` | JWT reader | `api:invoke`+`route:command`; echipament online | `{equipment_id}` | `reply{ticket, equipment_id, expires_at}` | memorie (≤ 1 min, unic) | 25 s (`/api/wss/open`) | fără | `command_failed` |
| 10 | Fereastră `/control/` → WSM → Reader | HTTPS static + WSS | `wsm.ticket.<ticket>`; apoi `api.request mode:equipment` | tichet unic | doar `ping` + `api.request` către echipamentul tichetului; Origin `https://<Host>` | ca #5 | ca #5 | — | lease ≤ 5 min; cerere 35 s (610 s render) | re-tichet prin opener la 30 s; fără replay | 401 „invalid control ticket” |
| 11 | Reader → WSM → WiseMED | WSS → HTTPS | `wisemed.ensure_equipment_online`, `wisemed.fetch_file_for_analyzer` | JWT reader; WSM→WiseMED JWT HS256 cu cheia API a tenantului (5 min) | `wisemed:proxy` | `{reader}` / `{file_id, equipment_id}` | `reply` = JSON WiseMED (≤ 4 MiB) | — | 20 s (WSM și reader) | fără retry la WSM; readerul cade pe HTTP direct | `command_failed` „WiseMED HTTP n” |
| 12 | Reader → abonat | WSS | `event` (`topic results:/logs:<reader_id>` sau `connection`) | JWT | `route:event`; restricții rol `reader` | liber | `command_ack` | — (nepăstrat pentru offline) | — | fără | `forbidden`, `recipients:0` |
| 13 | Admin → WSM | HTTPS | `/admin/api/*` | cookie `wsm_admin` + CSRF; login validat la WiseMED (`user_type=-1`) | sesiune legată de tenant | JSON ≤ 8 KiB | JSON | registru `device_keys_file` (atomic) | login upstream 8 s; sesiune 900 s | fără | 401/403/409/429/500 |
| 14 | Browser local → Reader | HTTP(S) local | `/api/*` | cookie `wmr_local_session_*` | `requireSession`; admin = `UserType <= 0` | JSON | JSON | conform handlerului | fără timeout global (doar `ReadHeaderTimeout` 10 s) | — | 401/403 |
| 15 | Browser local → Reader → peer | HTTP local → WSS | `POST /api/wss/bridge` | cookie admin local | admin local; apoi ca #5 cu JWT-ul readerului | `{connection_id, method, path, …}` ≤ 6 MiB | `{ok, data}` | — | 25 s | fără | 400 cu mesaj |
| 16 | Operator → proces WSM | semnal | `SIGHUP` (Unix) | acces OS | — | — | reload; **toate conexiunile închise** | — | — | — | „reload rejected; previous configuration retained” |

---

## 15. Contradicții cod ↔ documentație / memorie

1. **Handleri `orders.*`, `analytes.*`, `config.*`, `stats.*`, `results.activate`, `logs.*`.** `docs/protocol.md:211-212,261-262` trimite la `readersv3/modules/` pentru acești handleri și descrie fluxul `results.activate` + topic. În `readersv3` nu există nicio implementare (`HandleWSAction` doar în `ws`, `barcodeprinter`, `signingpad`; grep pe aceste nume fără rezultate). `wsm-server/web/app.js` îi listează ca mostre, dar directorul nu e servit.
2. **Rolul cheilor de device.** `docs/remote-control.md:33` spune „limitată … + rol reader”; codul generează chei cu `Roles: ["reader","service"]` (`C/devices.go:194`). Analog, cheile browser gestionate primesc și rolul `service` (`C/access.go:161`), nemenționat în `docs/protocol.md`.
3. **`reader_id` la alte roluri.** `docs/protocol.md:71-72`: „Pentru celelalte roluri, `reader_id` trebuie să lipsească”; codul îl permite (și îl cere împreună cu `equipment_id`) pentru `service` (`S/auth.go:144-150`). Documentul se corectează parțial la `:331-337`.
4. **`hello_ack`.** Exemplul din `docs/protocol.md:149` nu conține `equipment_id`; codul îl trimite (`S/server.go:339`) și readerul îl verifică (`WS:408`).
5. **`reconnect_delay_ms`.** Apare în exemplul de config (`docs/remote-control.md:86`) și e scris la salvare, dar nu e citit; pauza de 30 s e hard-codată (`WS:319`). `heartbeat_ms` din `apps/runner/bootstrap.go:369` e de asemenea neutilizat de modul.
6. **Numărul de aplicații.** Memoria și `docs/remote-control.md:235` spun 26; `readersv3/docs/wss-control.md` spune 28; în cod 31 de `apps/*/main.go` includ `wisemed-ws`.
7. **Coduri HTTP wrapper.** `docs/http-commands.md:53-54` leagă „client inexistent” de 404; codul întoarce și `503 client_unavailable` când ACK-ul are `recipients:0` (`S/http_commands.go:150-153`). Documentul menționează 503 „coadă”, dar nu acest cod de eroare.
8. **Ciclul de 5 minute în `device_key`.** Memoria (`aimemory/wsm-server.md:73-74`) notează „JWT expiry closes the socket”, dar nicăieri nu se menționează consecința pentru modul implicit al readerului (JWT auto-semnat de 5 min ⇒ deconectare periodică + 30 s offline). Documentația descrie reconectarea de 30 s doar ca reacție la „întreruperi”.
9. **Modulul `login`.** Cerința de investigație îl tratează ca loc al autentificării locale; în cod este un placeholder, iar logica e în `localhttp`.
10. **`bindguard`.** Numele sugerează o protecție a bind-ului; codul gestionează doar conflictul de port.
11. **„Reload-ul … revocă tichetele”** (`docs/remote-control.md:205`) e corect, dar incomplet: și orice `revokeTenantAccess` din consola admin (alt tenant inclusiv) le invalidează, prin `authEpoch` global.

Concordanțe verificate (documentația e corectă): hub în memorie fără coadă/replay (`aimemory/wsm-server.md:24-25`); semantica ACK; limitele și timeouturile din `docs/protocol.md`; `apibridge` pe același mux; excluderile de rute; contractul `commandHTTP` (în afară de pct. 7); tichetul de control.

---

## 16. Necunoscute / zone neinvestigate

- **Niciun test nu a fost rulat** (restricție: fără binare). Toate afirmațiile „test existent” sunt despre existența testului, nu despre rezultat.
- **Backendul WiseMED extern**: implementarea `PUT /administrative/login`, `PUT /administrative/analyzer`, `GET /fileforanalyzer/…`, a unui eventual endpoint de token WSS și a emiterii JWT-urilor de browser — nu se află în repo. Nu se știe ce scope-uri emite în producție și cui.
- **Configurația de producție reală** (`wsm-server/output/deployments/*`, `output/state`, `deployments/local`, registrul de device-uri): necitite deliberat (pot conține secrete). Valorile efective ale `max_message_bytes`, `allow_query_token`, `admin.enabled`, originile și scope-urile reale sunt [NEC]. A fost citit doar șablonul `wsm-server/deployments/config.production.yaml` (fără liniile cu chei).
- **Binarele din `output/wsm-browser-sdk/bin` și `wsm-server/output/bin`**: neverificat dacă corespund sursei curente.
- **Proxy invers** (`deployments/nginx-wsm.conf`): văzute doar `client_max_body_size 16m` și `proxy_read_timeout 310s`; efectul asupra `RemoteAddr` (rate limit admin, `sender_ip`) e dedus, nu verificat.
- **`siui-bridge.js`** și documentele SIUI (`siui-browser-wrapper.md`, `siui-browser-quickstart.md`): doar suprafața API; detaliile aparțin raportului SIUI.
- **`docs/admin-console.md`, `operations.md`, `migration.md`, `verification.md`, `docker-update.md`, `browser-sdk.md`**: consultate doar punctual (grep), nu comparate integral cu codul.
- **Consola admin JS** (`S/adminui/app.js`) și UI-ul `/control/` (`app.js`): neanalizate dincolo de `wss-remote.js`.
- **Handlerii locali individuali** din `localhttp` și din celelalte module: nu am verificat care respectă `r.Context()` la anulare, nici autorizarea fiecărei rute în parte (în afara celor citate). Rutele modulelor proprii (barcode etc.) fără `requireSession` nu au fost inventariate.
- **`wisemed-ai` și `PublishEventToConnection`**: nu am verificat cu ce rol rulează și dacă evenimentele `mode:"connection"` trec de regula pentru rolul `reader`.
- **Utilitarele login-only**: proveniența `echipament_id` necesar WSS nu a fost urmărită.
- **`nbf`**: comportamentul exact depinde de `golang-jwt/v5` (versiunea din `go.mod` neverificată).
- **Analiză de race**: nu s-a rulat `-race`; observațiile din §12 sunt din lectură.
