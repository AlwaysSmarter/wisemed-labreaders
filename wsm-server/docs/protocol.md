# Protocol WSM v1 — integrare WiseMED

Vezi și [controlul remote și adaptorul API–WSS](remote-control.md), inclusiv
`equipment.status`, `api.request` și `control.ticket`.

## Modelul de încredere

O instanță WiseMED = un `tenant_id`. Administratorul WSM configurează chei cu ID-uri
`kid` unice, fiecare legată de un singur tenant. Backendul WiseMED autentifică
utilizatorul/readerul și emite un JWT scurt cu drepturile necesare. Browserul primește **tokenul**, niciodată cheia de semnare. Readerul folosește
un token emis de backend sau propria cheie de echipament, restricționată în WSM;
nu primește cheia principală a instanței. Vezi modul `device_key` în
[control remote](remote-control.md).

Serverul selectează cheia după `kid`, verifică semnătura HS256, apoi verifică
`tenant_id` și `iss` față de configurația acelei chei. O cheie de la clinica A nu
poate autentifica un client al clinicii B, chiar dacă tokenul pretinde acest lucru.
Nici mesajele, nici `hello` nu pot schimba tenantul. Un `tenant_id` introdus arbitrar
în payload nu este folosit pentru rutare. API-ul WiseMED este selectat exclusiv din
configurația tenantului autentificat.

Cheile pot fi limitate prin `roles` și `scopes`. Drepturile din JWT trebuie să fie
un subset al celor permise cheii; un scope suplimentar respinge întregul token.
Backendul emitent este autoritatea pentru acces **în interiorul** tenantului:
`route:command` permite comenzi către readerele din tenant, conform restricțiilor
rolului și scope-urilor suplimentare; nu există ACL per pacient, utilizator sau
analiză. Tichetele de control sunt limitate suplimentar la un singur echipament și
comanda `api.request`. Nu dați acest scope
unui utilizator care nu are acest nivel de acces. `wisemed:proxy` este un drept
privilegiat pentru cele două operații proxy la nivelul întregii instanțe.

## JWT

Header:

```json
{"alg":"HS256","typ":"JWT","kid":"clinic-a-2026-09"}
```

Payload de browser (timpi ilustrativi; emitentul îi calculează la momentul emiterii):

```json
{
  "iss":"https://clinic-a.example.ro",
  "aud":"wsm-server",
  "sub":"user-123",
  "tenant_id":"clinic-a",
  "role":"browser",
  "client_id":"browser-session-456",
  "scopes":["route:command","route:broadcast","route:reply","topics:subscribe","connections:read","server:status"],
  "iat":1790000000,
  "nbf":1790000000,
  "exp":1790000300
}
```

Pentru `api.request`/control remote, adăugați `api:invoke`; acordați `api:admin`
numai dacă utilizatorul trebuie să primească privilegiile administrative locale.
Scope-urile trebuie permise și în cheia configurată pe server.

- Obligatorii: `kid`, `iss`, `aud`, `sub`, `tenant_id`, `role`, `client_id`,
  `scopes` nevid, `iat`, `exp`. `nbf` este opțional și se validează dacă există.
- `exp > iat`; durata maximă implicită 900 secunde, configurabilă până la 86400.
  Recomandarea operațională a acestui proiect: tokenuri de 5–15 minute.
- `iat` nu poate fi în viitor. Sincronizați ceasurile emitentului și serverului
  prin NTP; nu există toleranță suplimentară configurată.
- Roluri: `browser`, `reader`, `service`. `service` este un backend/automatizare
  cu identitate proprie, nu un administrator global al tuturor tenanturilor.
- Reader: adăugați `equipment_id` (ID-ul WiseMED) și `reader_id` și `role=reader`. Pentru celelalte roluri,
  `reader_id` trebuie să lipsească sau să fie gol.
- `label` este opțional. Identificatorii și topicurile acceptă
  `[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}`; folosiți ID-uri interne, nu adrese email.
- Nu există fallback la tokenurile vechi fără `kid`/tenant/issuer/audience.
- Tokenurile sunt bearer: cine le deține le poate folosi până la expirare sau
  revocarea cheii. Nu există stocare/revocare individuală pe `jti`.

| Scope | Permisiune |
| --- | --- |
| `route:command` | Comenzi către clienți (browser/service) |
| `route:reply` | Răspunsuri la comenzi |
| `route:event` | Publicare de evenimente |
| `route:broadcast` | În plus față de scope-ul tipului de mesaj: liste, all, client_type, topic |
| `topics:subscribe` | Subscribe/unsubscribe la topicuri în tenant |
| `connections:read` | Lista conexiunilor și diagnosticul tenantului |
| `server:status` | Comanda server.status |
| `wisemed:proxy` | Cele două comenzi proxy WiseMED |
| `devices:debug` | Reader → peer: device.ping, ws.reconnect, debug.message, împreună cu route:command |
| `api:invoke` | api.request și emiterea control.ticket, împreună cu route:command |
| `api:admin` | Principal administrativ în adaptorul API al echipamentului |

Restricții suplimentare pentru reader: comenzile către peer sunt permise numai
1:1 (`connection`, `equipment`, `reader`): cele trei comenzi de diagnostic cu
`devices:debug`, respectiv `api.request` cu `api:invoke`, toate cerând și
`route:command`. Celelalte comenzi către peer sunt respinse. `reply` numai către o
conexiune; `event` numai pe `results:<reader_id>` sau
`logs:<reader_id>`. Aceste evenimente nu necesită `route:broadcast`. Răspunsurile nu
sunt validate față de un registru de comenzi în așteptare; destinatarul verifică
`correlation_id` și identitatea expeditorului. Un reader este o componentă de
încredere a propriului tenant, nu un utilizator cu acces la un singur pacient.

## Conectare WSS

URL: `wss://ws.example.ro:8443/ws` (sau port 443 la terminarea TLS în proxy).
Certificatul trebuie validat normal: domeniu, perioadă și CA. Nu dezactivați
verificarea certificatului în clienți.

Backend/reader: handshake HTTP cu `Authorization: Bearer <JWT>`.

Browser: API-ul WebSocket nu permite setarea headerului Authorization. Folosiți
subprotocolurile `wsm.v1` și `wsm.jwt.<JWT>`:

```js
// getToken este implementat de aplicația WiseMED după autentificarea utilizatorului.
const token = await getToken();
const ws = new WebSocket("wss://ws.example.ro:8443/ws", ["wsm.v1", `wsm.jwt.${token}`]);
ws.onopen = () => ws.send(JSON.stringify({
  type: "hello",
  payload: { client_type: "browser", client_id: "browser-session-456" }
}));
ws.onmessage = ({data}) => handleWSMessage(JSON.parse(data));
```

Serverul negociază doar `wsm.v1`, fără să reflecte JWT-ul în răspuns. Originea
browserului trebuie să fie exact în `tenants.<id>.allowed_origins`, de exemplu
`https://clinic-a.example.ro`, fără slash final. Conexiunile fără Origin sunt
permise pentru readere/backenduri; Origin nu înlocuiește autentificarea JWT.

`?token=<JWT>` este disponibil numai cu `security.allow_query_token: true` și nu
este implicit. Evitați logarea query-urilor și headerelor sensibile în proxy.
Ordinea de selecție: Authorization, subprotocol JWT, apoi query dacă este activat.
API-urile HTTP de diagnostic acceptă numai Authorization.

Înainte de `hello_ack` nu trimiteți alt mesaj. Timeout hello implicit: 10 secunde.
`hello.client_type`, `client_id`, `reader_id` trebuie să corespundă JWT-ului;
`user_id`, dacă este prezent, trebuie să fie egal cu `sub`; label, dacă este trimis,
trebuie să corespundă label-ului din token. Identitatea nu se poate modifica ulterior.

Reader hello:

```json
{"type":"hello","payload":{"client_type":"reader","client_id":"reader-client-1","reader_id":"reader-1","equipment_id":"42"}}
```

Răspuns:

```json
{"type":"hello_ack","payload":{"connection_id":"<id-generat>","tenant_id":"clinic-a","client_type":"reader","client_id":"reader-client-1","reader_id":"reader-1","subject":"reader-1","expires_at":"2026-09-22T12:05:00Z"}}
```

Este permisă o singură conexiune reader înregistrată per `(tenant_id, reader_id)`
și per `(tenant_id, equipment_id)`. Conexiunea duplicată este respinsă; același reader_id poate exista la alt tenant.
Mai multe sesiuni browser cu același client_id sunt permise, având connection_id
separate. Connection_id este aleator și se schimbă la reconectare.

## Envelope și răspunsul de transport

```json
{"type":"command","request_id":"req-unique-123","target":{"mode":"reader","reader_id":"reader-1"},"payload":{"command":"reader.status","args":{}}}
```

Câmpuri: `type`, `request_id`, `correlation_id`, `target`, `payload`, plus
`connection_id` și `timestamp` setate de server la rutare. `request_id` este
obligatoriu pentru comenzi; `correlation_id` pentru reply. Ambele au maximum 128
caractere. Folosiți UUID-uri pentru cereri; hubul nu deduplică cererile.

Serverul adaugă/suprascrie în payload:

```json
{"sender_connection_id":"<id>","sender_client_type":"browser","sender_client_id":"browser-session-456","sender_reader_id":"","sender_tenant_id":"clinic-a","sender_subject":"user-123","sender_scopes":["route:command","api:invoke"]}
```

După rutare, expeditorul primește:

```json
{"type":"command_ack","request_id":"req-unique-123","correlation_id":"req-unique-123","payload":{"routed_type":"command","recipients":1,"target":{"mode":"reader","reader_id":"reader-1"},"delivery":"queued"}}
```

ACK-ul apare și pentru `reply`/`event`. `recipients` este numărul de conexiuni pentru
care mesajul a intrat în coada de trimitere. Nu dovedește recepția sau execuția.
`recipients=0` înseamnă fără destinatar eligibil/coadă disponibilă; un ID străin
altui tenant este tratat la fel ca unul inexistent. Nu expune existența clientului.
Un răspuns de aplicație poate sosi înainte de ACK; clientul trebuie să le gestioneze
independent. Coada este FIFO per conexiune, fără ordine globală între expeditori.

## 1:1 și procesarea comenzii

`target={"mode":"reader","reader_id":"reader-1"}` ajunge la readerul unic.
Pentru ID-ul WiseMED, folosiți `{"mode":"equipment","equipment_id":"42"}`.
Pentru o sesiune precisă, folosiți `{"mode":"connection","connection_id":"..."}`.

Destinatarul:

1. Verifică `type=command`, numele comenzii, argumentele și identitatea expeditorului.
2. Execută un handler dintr-o listă explicită; nu execută shell/cod arbitrar din mesaj.
3. Răspunde la `payload.sender_connection_id`, cu `correlation_id=request_id`:

```json
{"type":"reply","correlation_id":"req-unique-123","target":{"mode":"connection","connection_id":"<sender_connection_id>"},"payload":{"ok":true,"data":{"status":"online"}}}
```

Eșec de aplicație recomandat:

```json
{"type":"reply","correlation_id":"req-unique-123","target":{"mode":"connection","connection_id":"<sender_connection_id>"},"payload":{"ok":false,"error":{"code":"unsupported_command","message":"Comandă necunoscută"}}}
```

`payload.ok/data/error` este convenția recomandată; handler-ele readerelor existente
pot avea altă schemă și trebuie păstrată compatibilitatea la nivelul aplicației.
Hubul transportă payload-ul și nu implementează comenzile `orders.*`, `analytes.*`,
`config.*`, `stats.*`. Vezi `readersv3/modules/` pentru acei handleri.

## 1:N și broadcast

| Țintă | Exemplu target | Destinatari în tenant |
| --- | --- | --- |
| Listă echipamente | `{"mode":"equipments","equipment_ids":["42","43"]}` | Echipamentele enumerate |
| Listă readere | `{"mode":"readers","reader_ids":["reader-1","reader-2"]}` | Readerele enumerate |
| Listă conexiuni | `{"mode":"connections","connection_ids":["id1","id2"]}` | Sesiunile enumerate |
| Tip client | `{"mode":"client_type","client_type":"reader"}` | Toate readerele |
| Topic | `{"mode":"topic","topic":"results:reader-1"}` | Abonații topicului |
| Toți | `{"mode":"all"}` | Toate conexiunile, inclusiv expeditorul |
| Propria sesiune | `{"mode":"self"}` | Doar expeditorul |

Listele au maximum 100 ID-uri; ID-urile duplicate nu multiplică livrarea. Listele
și broadcasturile necesită `route:broadcast` și scope-ul tipului de mesaj.
`target` absent și vechiul `broadcast=true` sunt respinse, pentru a evita un
broadcast accidental. Nu există target cross-tenant sau administrator global WS.

```json
{"type":"command","request_id":"batch-123","target":{"mode":"readers","reader_ids":["reader-1","reader-2"]},"payload":{"command":"reader.status","args":{}}}
```

Fiecare reader trimite propriul reply cu `correlation_id=batch-123`. Clientul ține
un registru local de cereri și colectează răspunsurile după
`(correlation_id, sender_connection_id)`. ACK-ul spune câte trimiteri au fost puse
în coadă, nu câte vor răspunde. Stabiliți un timeout și raportați separat succesul,
eșecul și lipsa răspunsului. Nu există tranzacție comună pentru un broadcast.

Pentru operații cu efecte (salvare rezultate, pornire analiză etc.), implementați
idempotency în handler folosind un ID de operație persistent. Nu retrimiteți automat
o comandă cu efecte doar pentru că a expirat așteptarea: ea poate fi deja executată.

## Evenimente, topicuri, prezență și ping

```json
{"type":"subscribe","request_id":"sub-1","payload":{"topic":"results:reader-1"}}
```

Răspunsul `subscribe_ack` are `topic`, `ok`, `subscribed`. Pentru unsubscribe:
`type=unsubscribe`, răspuns `unsubscribe_ack`, cu `ok` și `unsubscribed`.
Maximum implicit 64 topicuri/conexiune. Un topic identic la doi clienți rămâne izolat.

Readerul publică:

```json
{"type":"event","target":{"mode":"topic","topic":"results:reader-1"},"payload":{"event":"result_available","data":{"sample_id":"sample-123"}}}
```

Pentru fluxul readerelor existente, trimiteți întâi `results.activate`, apoi vă
abonați la topicul întors de reader. Abonamentele se pierd la deconectare.
Evenimentele nu sunt păstrate pentru abonați offline. După reconectare, refaceți
abonamentele și recuperați starea prin comenzile/HTTP-urile aplicației.

`presence` cu `event=connected/disconnected` este emis în interiorul tenantului.
Prezența este informativă, nu jurnal persistent; interogați lista pentru starea
curentă. `list_connections` cere `connections:read` și întoarce `type=connections`.
`ping` JSON întoarce `pong`; este separat de control frames WebSocket ping/pong,
folosite pentru heartbeat și read timeout. Clienții trebuie să proceseze control
frames (browserul o face automat, gorilla prin bucla ReadJSON/ReadMessage).

## Comenzi procesate de server și HTTP diagnostic

```json
{"type":"command","request_id":"srv-1","target":{"mode":"server"},"payload":{"command":"server.status","args":{}}}
```

| Comandă | Scope | Rezultat |
| --- | --- | --- |
| `equipment.status` | `connections:read` | Prezență pentru equipment_id sau equipment_ids (maximum 100) |
| `equipment.list` | `connections:read` | Conexiunile tenantului |
| `control.ticket` | `route:command` + `api:invoke` | Tichet unic pentru controlul unui echipament online |
| `server.status` | `server:status` | Numele serviciului, tenant_id, număr conexiuni din tenant, UTC |
| `wisemed.ensure_equipment_online` | `wisemed:proxy` | `args.reader` nevid; PUT `/administrative/analyzer` la upstreamul tenantului |
| `wisemed.fetch_file_for_analyzer` | `wisemed:proxy` | `args.file_id`, `args.equipment_id`; GET `/fileforanalyzer/<file>/<equipment>/` |

Comenzile serverului întorc direct `reply` sau `error`, fără command_ack; payload-ul
proxy este răspunsul WiseMED. `wisemed:proxy` permite selectarea echipamentului în
propria instanță, nu o restricție per echipament. Nu acordați acest scope browserelor
obișnuite. URL-ul/cheia API nu pot fi furnizate de client în comandă.

Proxy: HTTPS, timeout 20 secunde, răspuns maximum 4 MiB, fără redirecturi sau retry,
JWT outbound separat de cheile de transport. Nu mai adaugă XDEBUG_TRIGGER.
Un apel proxy ocupă bucla de citire a conexiunii respective până la finalizarea sa;
ceilalți clienți continuă independent. Nu sunt disponibile toate API-urile WiseMED.

| HTTP | Auth | Conținut |
| --- | --- | --- |
| `GET /healthz` | Public | Liveness, fără date de tenant; nu testează WiseMED |
| `GET /api/connections` | Bearer + connections:read | Conexiuni și contoare numai din tenant |
| `GET /api/debug/state` | La fel | Alias pentru același diagnostic |
| `GET /api/equipment/{id}` | Bearer + connections:read | Prezența unui echipament din tenant |
| `GET /control/` și assets | Public, numai fișiere statice | UI; datele cer separat autentificare WSS |

Nu există REST pentru trimiterea comenzilor sau managementul cheilor. WS este
API-ul de mesagerie; administrarea se face local prin configurație/CLI. Nu este
activat CORS HTTP cross-origin: browserul poate folosi `list_connections` prin WS,
iar backendul poate folosi GET cu Bearer.

## Erori și reconectare

Handshake: `401` token invalid/lipsă/expirat; `403` Origin nepermis; `503` limita
conexiunilor sau server în oprire. După upgrade, hello incorect/repetat și depășirea
ratei duc la close `1008`. Operațiile nepermise întorc:

```json
{"type":"error","request_id":"req-1","correlation_id":"req-1","payload":{"code":"forbidden","message":"operation not permitted or invalid message"}}
```

Eșecurile comenzilor serverului folosesc `code=command_failed`. Pentru expirare,
reload, overflow și probleme de rețea, socketul este închis; poate fi observat ca
`1006` fără close frame. Clientul obține JWT nou de la propriul backend, apoi se
reconectează, trimite hello, reface abonamentele și reinteroghează starea.
Transportul comun readersv3 și consola remote folosesc o pauză fixă de 30 secunde
cu countdown și retry manual imediat. Clienții externi pot adopta propriul backoff
cu jitter, respectând limitele serverului. Nu reutilizați un token expirat.
