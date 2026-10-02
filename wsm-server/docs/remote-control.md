# Control complet al echipamentelor prin WSS

## Fluxul implementat

1. Readerul/utilitarul pornește și înregistrează/reinițializează echipamentul prin
   **HTTP API WiseMED**, `PUT /administrative/analyzer`. Inițializarea este
   serializată și o reușită este reutilizată de modulele care pornesc simultan.
2. Numai după răspuns valid și persistarea `echipament_id`, modulul comun WSS
   deschide o conexiune externă WSS. JWT-ul și conexiunea conțin `equipment_id`.
3. Serverul păstrează perechea `(tenant_id, equipment_id)`. Un echipament are o
   singură conexiune; același ID la alt client WiseMED nu intră în conflict.
4. Interfața WiseMED sau Debug WSS interoghează prezența/lista, apoi trimite comenzi
   prin conexiunea externă. Nu este necesar un port public pe calculatorul readerului.
5. „Deschide” lansează aceeași interfață comună la `/control/` pe serverul central.
   HTML/CSS/JS sunt fișiere statice centrale; **toate cererile de date API merg
   prin WSS**, nu prin HTTP către echipament. Datele binare sunt codificate base64.

Nu există o a doua implementare a logicii API: `shared/apibridge` construiește o
cerere internă și invocă același `http.ServeMux` și aceiași handleri ca HTTP-ul local.
Modulul WSS nu deschide un proxy HTTP spre o adresă furnizată de utilizator.

## Identitate, provisionare și drepturi

JWT-ul readerului adaugă `equipment_id` obligatoriu. `reader_id` rămâne identitatea
locală a aplicației; nu trebuie înlocuit peste tot cu ID-ul numeric WiseMED.
`hello` poate include equipment_id, dar valoarea efectivă vine din tokenul semnat.

Sunt suportate trei moduri în `modules.wisemed-ws`:

- `device_key`: readerul semnează cu cheia **proprie echipamentului** introdusă în
  câmpul mascat „Cheie WSS” (`device_secret` în YAML), apoi alternativ `secret_file`
  sau `api_key_echipament`, în această ordine. Cheia trebuie provisionată și în WSM și
  limitată server-side la subject/reader_id/equipment_id + rol reader. Nu introduceți
  cheia principală a tenantului pe echipament.
- `token_endpoint`: `token_path` este o rută relativă configurată a API-ului WiseMED,
  apelată prin POST autentificat, cu reader_id/client_id/equipment_id. Backendul
  trebuie să implementeze acea rută și să întoarcă `{"token":"<JWT>"}`. WSM nu
  inventează și nu activează automat o rută de autentificare în backendul WiseMED.
- `token_file`: JWT citit la fiecare încercare din fișier extern, util pentru teste
  sau un agent extern de refresh. Un token expirat trebuie înlocuit extern.

Pentru configurarea simplă, folosiți [consola admin](admin-console.md): login
WiseMED cu tip -1, `add <equipment_id>`, apoi copiați cheia și identitatea afișate
în Debug WSS. `list` arată aparatele cunoscute, iar `key <equipment_id>` permite
administratorului aceluiași tenant să afișeze cheia unui aparat. Readerul nu
returnează cheia salvată către UI; lăsarea câmpului gol la salvare o păstrează.

Exemplu server pentru o cheie dedicată echipamentului (se adaugă în configurația
cu tenantul și TLS deja configurate):

```yaml
security:
  keys:
    clinic-a-device-42:
      tenant_id: clinic-a
      subject: reader-42
      reader_id: reader-42
      equipment_id: "42"
      secret_file: /etc/wsm-server/keys/device-42.key
      roles: [reader]
      scopes: ["route:reply", "route:event", "route:command", "devices:debug", "connections:read", "server:status", "api:invoke", "api:admin"]
```

Fișierul conține exact cheia echipamentului returnată/provisionată de WiseMED, cu
minimum 32 bytes. Schimbarea ei în WSM nu modifică cheia HTTP din WiseMED; cele două
părți trebuie sincronizate de operator/provisioner. Pentru producție cu multe
readere, emiterea JWT-urilor de backend (`token_endpoint`) simplifică provisionarea.

Exemplu reader:

```yaml
modules:
  wisemed-ws:
    enabled: true
    url: wss://ws.example.ro:8443/ws
    auth_mode: device_key
    tenant_id: clinic-a
    key_id: clinic-a-device-42
    issuer: https://clinic-a.example.ro
    audience: wsm-server
    subject: reader-42
    client_id: reader-42
    # secret_file: ./secrets/device-42.key  # altfel api_key_echipament din HTTP
    ca_file: ""                          # CA custom opțional, validare TLS activă
    scopes: route:reply,route:event,route:command,devices:debug,connections:read,server:status,api:invoke,api:admin
    reconnect_delay_ms: 30000
```

Scope-uri suplimentare:

| Scope | Permisiune |
| --- | --- |
| `devices:debug` | Reader → peer: doar device.ping, ws.reconnect, debug.message (plus route:command) |
| `api:invoke` | Comanda api.request, plus route:command; echivalentul unei sesiuni locale autentificate |
| `api:admin` | Marchează sesiunea remote ca administrator pentru API-urile locale care cer admin |

Fără api:invoke nu se poate apela API-ul sau emite un tichet de control. Cheile cu
aceste scope-uri reprezintă o autoritate puternică în tenant. Defaultul modulului
reader include diagnosticul, dar **nu** api:invoke/api:admin; acestea se activează
explicit în ambele configurații pentru control remote. În UI local, deschiderea
controlului, schimbarea setărilor WSS și folosirea proxy-ului de debug sunt permise
numai administratorilor locali (UserType <= 0). Un utilizator obișnuit nu poate
moșteni implicit privilegiile tokenului runtime.

## Prezența echipamentelor în WiseMED / ACE

Printr-un client deja conectat cu `connections:read`:

```json
{"type":"command","request_id":"presence-1","target":{"mode":"server"},"payload":{"command":"equipment.status","args":{"equipment_ids":["42","43"]}}}
```

Sau `args={"equipment_id":"42"}` pentru unul. Maximum 100 per cerere.
Răspunsul `reply.payload.equipment` conține pentru fiecare ID:

```json
{"equipment_id":"42","online":true,"connected_seconds":80,"connection":{"id":"...","equipment_id":"42","reader_id":"reader-42","client_type":"reader","remote_ip":"203.0.113.10","connected_at":"2026-09-22T00:00:00Z","last_seen_at":"..."}}
```

ID offline/inexistent/străin tenantului: `{"equipment_id":"43","online":false}`.
`equipment.list` sau `list_connections` întorc conexiunile tenantului.
Alternativ: `GET /api/equipment/42` cu Bearer + connections:read.

`remote_ip` este IP-ul observat de server, de obicei NAT/VPN. Nu este o adresă pe
care browserul trebuie s-o contacteze. Cu reverse proxy local poate fi IP-ul proxy;
serverul nu crede orbește header-ele X-Forwarded-For. IP-urile private nu sunt
obținute prin scanarea rețelei echipamentului.

Ping aplicație:

```json
{"type":"command","request_id":"ping-42","target":{"mode":"equipment","equipment_id":"42"},"payload":{"command":"device.ping","args":{}}}
```

Reconectare: aceeași structură cu `command=ws.reconnect`. Readerul scrie reply înainte
să-și întrerupă socketul și reîncearcă imediat. Pentru un echipament deja offline nu
se poate trimite o comandă prin socketul inexistent; reconectarea este inițiată local
la fiecare 30 secunde. Nu există coadă offline de comenzi.

## API → JSON WSS → același API

Exemplu universal pentru orice reader/utilitar:

```json
{
  "type":"command",
  "request_id":"api-42-1",
  "target":{"mode":"equipment","equipment_id":"42"},
  "payload":{
    "command":"api.request",
    "args":{"method":"GET","path":"/api/status"}
  }
}
```

Comandă cu date:

```json
{"type":"command","request_id":"api-42-2","target":{"mode":"equipment","equipment_id":"42"},"payload":{"command":"api.request","args":{"method":"PUT","path":"/api/reader-settings","body":{"reader_label":"Reader laborator"}}}}
```

Argumentele sunt exact cele acceptate de endpointul local; exemplul ilustrează
transportul, nu înlocuiește schema completă a endpointului. Folosiți aceleași câmpuri
pe care le trimite UI-ul existent.

Răspuns:

```json
{"type":"reply","correlation_id":"api-42-1","payload":{"kind":"api.response","status":200,"content_type":"application/json","body":{"ok":true,"reader":{}}}}
```

Codurile 400/401/403/404/500 din API rămân în `payload.status`; erorile adaptorului
sunt reply cu `ok=false,error.message`. ACK-ul de rutare nu este răspunsul API.
Metode: GET, HEAD, POST, PUT, PATCH, DELETE. Path relativ `/api/...` sau compatibil
`/barcode/print`, inclusiv query string. Fără scheme/host extern sau redirect urmat.

Pentru import/multipart/binare: `body_base64`, `content_type` (inclusiv boundary),
în loc de `body`. Pentru răspuns binar: `body_base64` și `content_type`. Maximum
4 MiB corp decodat și răspuns brut; configurați serverul cu `max_message_bytes:
8388608` pentru overhead-ul base64 (template-ul de producție folosește această
valoare). Nu sunt permise HTML/elemente de interfață în WSS. Imprimările folosesc
modelele JSON ale acelorași handleri, randate local de browser.

Excluderi intenționate: login/logout local cu cookie, `/api/wss/bridge`,
`/api/wss/open`, `/api/wss/debug` (ar permite ocolirea restricției la echipament),
stream-uri HTTP/upgrade WebSocket și pagini statice HTML. Logout-ul remote închide
sesiunea de control, fără să deconecteze echipamentul. Operațiile cu hardware
utilizează în continuare driverele/permisiunile echipamentului.

## Deschiderea ferestrei de control

Din Settings → Debug WSS, „Deschide” pe un echipament online:

- apel local autentificat admin `POST /api/wss/open {"equipment_id":"42"}`;
- readerul cere `control.ticket` serverului, prin propria conexiune WSS;
- serverul emite un tichet aleator, consumabil o singură dată în maximum un minut;
- fereastra nouă încarcă `/control/?equipment_id=42&opener_origin=...`;
- tichetul se transferă prin postMessage, cu verificare strictă de origin și source;
  nu intră în URL, storage sau log;
- conexiunea folosește `wsm.ticket.<ticket>` ca subprotocol și hello browser/control;
- serverul permite numai api.request către equipment_id-ul autorizat și ping.

Sesiunea copil are maximum cinci minute și nu depășește expirarea JWT-ului părinte.
Părintele poate emite un tichet nou după reconectare/refresh. Sesiunea remote nu
poate delega alte tichete. Reload-ul configurației WSM revocă și tichetele nefolosite.
Origin-ul ferestrei de control trebuie să fie exact originea HTTPS a serverului.

ACE/backendul poate folosi aceeași comandă:

```json
{"type":"command","request_id":"open-42","target":{"mode":"server"},"payload":{"command":"control.ticket","args":{"equipment_id":"42"}}}
```

UI-ul ACE păstrează tichetul în memorie și implementează aceeași predare postMessage
sau îl oferă operatorului în câmpul securizat al ferestrei standalone. Pentru o
fereastră cu opener, implementați răspuns la `wsm-control-ready` și
`wsm-control-refresh` cu `{type:"wsm-control-auth",ticket}` pe originea exactă.

## Status și retry local

Statusul real devine „conectat” numai după `hello_ack`, nu imediat după handshake.
Topbarul și Debug WSS afișează faza, echipamentul, eroarea și countdownul până la
următoarea încercare. Reîncercarea automată este fix 30 secunde. Click pe statusul
deconectat oprește așteptarea și încearcă imediat. „Deconectare” local pune o pauză
manuală până la „Reconectare”; nu reîncearcă în fundal împotriva intenției operatorului.

`GET /api/wss/status`, `PUT /api/wss/settings`, `POST /api/wss/control`,
`POST /api/wss/debug`, `POST /api/wss/bridge`, `POST /api/wss/open` există în fiecare
aplicație echipament prin modulul comun. `/api/wss/bridge` este endpointul local de
exemplu care expediază API-ul ales prin WSS, cu body
`{"connection_id":"...","method":"GET","path":"/api/status"}`.

## Aplicații și verificare

26 aplicații echipament: 22 readere + barcodeprinter, anaf-docsmart,
esignature-server și signing-pad-utility. Modulul comun asigură același transport.
`update-server` rămâne infrastructură de distribuție software, nu echipament medical;
nu am expus administrarea pachetelor prin WSS.

Testul din `wsm-server/integration` pornește server TLS real + modul reader real,
folosește certificat/CA temporare și verifică API→WSS, identitatea, tichetul scoped,
ping/reconnect, pauza manuală și reconectarea efectivă după 30 secunde. Backendul
WiseMED și hardware-ul fizic sunt simulate în teste; configurarea lor reală rămâne
necesară înainte de utilizarea pe teren.
