# Backend HTTP: orice client conectat

WSM primește HTTPS de la backend și rutează asincron prin conexiunea WSS deja
deschisă de client. Cererea HTTP rămâne deschisă până la reply sau timeout.
Nu pornește procese pe echipament. Necesită rebuild și restart WSM; nu este
activat pe o instalare existentă doar prin copierea wrapperului PHP.

## 1. Descoperire

`GET /api/connections?client_type=siui-bridge`

`client_type` poate fi orice tip conectat: reader, service, browser,
siui-bridge, siui-bridge-history etc. Fără filtru sunt listate toate conexiunile
din tenantul autentificat. Răspunsul păstrează forma existentă:
`{tenant_id, connections:[{id,client_type,client_id,equipment_id,...}],count,stats}`.
`count` reflectă filtrul; `stats` rămâne statistica întregului tenant.
Ordinea este după connected_at crescător. Primul element este prima conexiune
din această ordine, nu o prioritate configurată. Lista este un snapshot.

## 2. Comandă și răspuns sincron către HTTP

`POST /api/connections/{id}/commands`

```json
{
  "command":"api.request",
  "args":{"method":"GET","path":"/api/siui/status"},
  "timeout_ms":135000
}
```

ID-ul vine din lista anterioară. Comanda și args sunt generice și trebuie
suportate de clientul ales. `api.request` este adaptorul comun pentru API-urile
readerelor/utilitarelor; un browser poate avea alți handleri.

HTTP 200 înseamnă că a sosit reply-ul autentic al conexiunii selectate:

```json
{
  "request_id":"<generat-de-WSM>",
  "connection_id":"<id-selectat>",
  "response":{"kind":"api.response","status":200,"body":{"...":"..."}}
}
```

`response` păstrează payload-ul clientului. Erorile locale de API rămân în
`response.status/body`; un `response.ok=false` rămâne refuz de aplicație.
ACK-ul de rutare nu încheie cererea HTTP. Reply-urile de la altă conexiune sau
cu alt correlation_id sunt ignorate. Nu există retry/replay automat.

Erorile HTTP sunt JSON `{error, request_id}` (autentificarea poate avea și
`message`). Coduri: 401 JWT invalid/expirat, 403 drepturi/rol/origine nepermise,
400 JSON/comandă/timeout invalid, 413 mesaj prea mare, 404 client inexistent sau
din alt tenant, 503 limită/coadă/reload/shutdown, 502 client deconectat după
trimitere, 504 timeout. Erorile cu `execution_unknown` nu dovedesc neexecutarea.
Închiderea cererii HTTP abandonează așteptarea, nu anulează operația remote.

`timeout_ms`: implicit 135000 ms, interval 1..300000 ms; JWT-ul limitează
suplimentar durata la expirarea sa. WSM folosește un participant temporar în hub,
vizibil în lista generală pe durata apelului, cu identitatea/drepturile JWT.
Acesta intră în limitele globale/per tenant de conexiuni și este eliminat la final.
Reload/revocare/shutdown întrerup așteptarea. Body-ul este limitat de
`server.max_message_bytes`. Proxy-urile și timeoutul aplicației PHP trebuie să
permită durata configurată. Template-ul Nginx folosește proxy_read_timeout 310s.

Toate apelurile cer `Authorization: Bearer <JWT>`, fără query token sau sesiune
admin. Lista necesită `connections:read`. POST acceptă rol service sau browser
și aplică aceeași autorizare ca WSS: `route:command`, plus `api:invoke` pentru
api.request și `api:admin` numai pentru operații care cer admin. Scope-urile
trebuie permise și de cheia JWT. Identitatea și tenantul sunt stabilite de WSM,
nu din args. Credentialele nu acordă acces la alte tenanturi.

## Wrapper PHP

`examples/php/WSMHttpClient.php` necesită PHP 8.1+ și ext-curl, fără Composer.
Pentru o CA privată, constructorul acceptă opțional al patrulea argument,
calea fișierului CA PEM. Validarea certificatului și a hostname-ului rămâne activă.

```php
require '/cale/WSMHttpClient.php';

$wsm = new WSMHttpClient('https://ws.example.ro', $jwt);
$wsm->setTimeouts([
    'connectTimeout' => 5000,
    'requestTimeout' => 135000,
    'totalTimeout' => 300000,
]);
$wsm->setReconnectOptions(['connectAttempts' => 3]);

// Tip arbitrar; nu trebuie cunoscut equipment_id.
$client = $wsm->firstClient('siui-bridge');
// Echivalent: $clients = $wsm->listClients('siui-bridge'); $client = $clients[0];

$response = $wsm->sendCommand($client, 'api.request', [
    'method' => 'GET',
    'path' => '/api/siui/status',
], ['requestTimeout' => 20000]);

// Shortcut pentru adaptorul API:
$response = $wsm->callAPI($client, 'GET', '/api/siui/status');
$status = $response['status'];
$body = $response['body'] ?? null;
```

Metode: listClients, firstClient, sendCommand, callAPI, setTimeouts,
setReconnectOptions, getOptions. Opțiunile per apel suprascriu configurația
obiectului numai pentru acel apel. Toate duratele sunt în ms, ca în JavaScript.
connectTimeout limitează conectarea HTTPS; requestTimeout limitează răspunsul
WSS și transportul HTTP (cu 5s marjă pentru răspunsul timeout); totalTimeout
limitează un apel PHP complet, inclusiv încercările GET. Limita requestTimeout
este 300000 ms. Nu se include și timpul unui apel separat firstClient în bugetul
unui sendCommand ulterior.

connectAttempts se aplică exclusiv erorilor tranzitorii ale descoperirii GET.
POST este trimis o singură dată, chiar și cu connectAttempts=3. Nu există socket
PHP persistent de reconectat. Clientul selectat rămâne aceeași conexiune; după
deconectare, redescoperirea este o acțiune explicită înaintea unei noi comenzi.

WSMHttpException expune errorCode, requestId, executionUnknown și httpStatus.
Wrapperul aruncă excepții pentru erorile WSM și response.ok=false; callAPI
păstrează statusul remote inclusiv non-2xx pentru tratarea de către aplicație.

waitTimeout/pollInterval/downloadTimeout din wrapperul SIUI JavaScript aparțin
fluxului SIUI (polling/descărcare), nu transportului generic. Dacă submit-ul SIUI
returnează call.id, răspunsul HTTP de aici confirmă submit-ul. Nu așteaptă automat
finalizarea CNAS, downloadul ori livrarea callwmapi. Pentru reluare folosiți acel ID
și operation_id stabil, fără repetarea oarbă a operației.

Verificare locală cu clienți WSS simulați, inclusiv PHP/ext-curl către HTTP WSM
prin TLS 1.2 cu CA verificată, răspuns API 422 păstrat și așteptare mai lungă
decât timeoutul HTTP obișnuit al serverului. Nu confirmă instalarea sau CNAS real.
