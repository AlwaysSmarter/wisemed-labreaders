# SDK JavaScript WSM pentru WiseMED

## Începe aici: generează cheia pentru browser și configurează WSM

`WSMServer` primește un **JWT calculat de backendul PHP**, nu cheia secretă.
Pentru semnarea acestui JWT ai nevoie de o cheie WSM care permite rolul `browser`.
Comanda `add` din consola web `wsm ›` creează chei de echipament cu rolurile
`reader`/`service`; nu creează cheia pentru browser. Pașii de mai jos se execută
în terminalul sistemului de operare și în configurația WSM.

### 1. Generează cheia secretă

Execută din rădăcina repository-ului `wisemed-labreaders`. Exemplele folosesc
build-ul local din `output/wsm-browser-sdk`; într-o altă instalare înlocuiește
calea executabilelor cu cea din pachetul instalat.

```bash
mkdir -p wsm-server/output/deployments/keys
./output/wsm-browser-sdk/bin/wsmctl keygen \
  -out wsm-server/output/deployments/keys/wisemed-web.key
```

Dacă binarele nu există încă, construiește pachetul din rădăcina repository-ului:

```bash
WSM_OUTPUT_DIR="$PWD/output/wsm-browser-sdk" ./wsm-server/build.sh
```

Apoi execută generarea cheii. `keygen` creează un fișier cu permisiuni `0600`
și **nu suprascrie o cheie existentă**. Dacă fișierul există deja, reutilizează
cheia corectă; nu îl șterge doar pentru a repeta instrucțiunile.

### 2. Înregistrează cheia în configurația activă WSM

Pentru rularea locală descrisă în proiect, editează
`wsm-server/output/deployments/config.yaml`. În altă instalare, folosește calea
afișată la pornire în mesajul `loading configuration`.

Adaugă intrarea `wisemed-web` sub **`security.keys` existent**, păstrând celelalte
chei și setări. Nu crea o a doua secțiune YAML `security`.

```yaml
security:
  # Păstrează audience, device_keys_file și celelalte setări existente.
  keys:
    # Păstrează și cheile existente.
    wisemed-web:
      tenant_id: wisemed-local
      secret_file: keys/wisemed-web.key
      roles: [browser]
      scopes:
        - route:reply
        - route:event
        - route:command
        - connections:read
        - server:status
        - api:invoke
        - topics:subscribe
```

`secret_file` relativ este rezolvat față de directorul configurației. `wisemed-web`
este ID-ul cheii (`kid`), iar `wisemed-local` trebuie să fie un tenant deja definit.

Pentru operații administrative remote SIUI adaugă `api:admin` la cheia WSM și
emite acest drept în PHP numai utilizatorilor autorizați. Pentru mesaje către
grupuri/broadcast adaugă `route:broadcast`; pentru comenzile proxy WiseMED,
`wisemed:proxy`. Scope-urile din fiecare JWT trebuie să fie un subset al celor
permise cheii, nu obligatoriu întreaga listă.

### 3. Verifică tenantul, issuer și originea paginii WiseMED

În `tenants.wisemed-local`, păstrează `issuer` existent: **valoarea identică**
va fi folosită în PHP. Issuer-ul nu se deduce din adresa publică WSS.

Adaugă în `allowed_origins` originea reală a paginii WiseMED, păstrând originile
existente. De exemplu, pentru o pagină la `https://php.wisemed.eu/...`, adaugă:

```yaml
    allowed_origins:
      # Păstrează și originile existente necesare.
      - https://php.wisemed.eu
```

Originea nu include calea sau slash final; include portul dacă este nestandard.
Adresa publică WSM rămâne `wss://wslocal.wisemed.eu/ws`, chiar dacă Nginx comunică
intern cu WSM prin HTTP pe portul 8090.

### 4. Validează configurația și repornește WSM

Din rădăcina repository-ului:

```bash
./output/wsm-browser-sdk/bin/wsm-server \
  -config wsm-server/output/deployments/config.yaml \
  -check-config
```

După mesajul `configuration valid`, repornește procesul WSM prin mecanismul cu
care îl rulezi (GoLand, serviciu sau container), cu aceeași configurație activă.
Comanda de verificare nu repornește și nu reconfigurează procesul deja pornit.

### 5. Configurează cheia și generarea JWT în backendul PHP

Helperul acceptă **fie `secret`, fie `secret_file`**, nu ambele. Dacă ai deja cheia
în configurația privată PHP, o poți transmite direct, fără fișier:

```php
$config = [
    'kid' => 'wisemed-web',
    'tenant_id' => 'wisemed-local',
    'issuer' => 'VALOAREA_EXACTA_DIN_CONFIG_WSM',
    'audience' => 'wsm-server',
    'secret' => $cheiaSecretaWSM, // String din configurația privată a backendului.
    'allowed_scopes' => $scopes,
];
$tokenWSM = wsmBrowserToken($config, 'user-' . $authenticatedInternalUserID, $scopes, 300);
```

`$scopes` reprezintă drepturile autorizate pentru utilizator, ca în exemplul de mai
jos. Cheia trebuie să fie identică celei configurate în WSM și să aibă minimum
32 bytes. Nu o decoda din base64. Această opțiune este pentru helperul PHP;
configurația YAML WSM continuă să folosească `secret_file` sau `secret_env`.
Clasa JavaScript `WSMServer` primește numai JWT-ul, niciodată acest secret.

Alternativa cu fișier:

Dacă PHP rulează pe alt server, transferă fișierul secret printr-un canal protejat
într-un director privat, în afara rădăcinii web. Restricționează accesul la contul
procesului PHP. Cele două fișiere trebuie să conțină **același secret**, chiar dacă
au căi diferite. Nu pune cheia în Git, HTML, JavaScript sau în răspunsul endpointului.

Copiază helperul `examples/browser/wsm-token.php` pe serverul PHP și folosește-l
numai după autentificarea și autorizarea utilizatorului WiseMED:

```php
require_once '/private/wsm-token.php';

// Exemplu pentru utilizator autorizat să consulte și să apeleze API-uri remote.
// Selectează drepturile din politica backendului, nu din parametrii browserului.
$scopes = ['route:command', 'connections:read', 'server:status', 'api:invoke'];
$config = [
    'kid' => 'wisemed-web',
    'tenant_id' => 'wisemed-local',
    'issuer' => 'VALOAREA_EXACTA_DIN_TENANTS_WISEMED_LOCAL_ISSUER',
    'audience' => 'wsm-server',
    'secret_file' => '/private/keys/wisemed-web.key',
    'allowed_scopes' => $scopes,
];
$tokenWSM = wsmBrowserToken(
    $config,
    'user-' . $authenticatedInternalUserID,
    $scopes,
    300 // Durata în secunde; trebuie să respecte limita configurată în WSM.
);
```

Înlocuiește placeholder-ul issuer cu valoarea reală. Dacă ai modificat audiența
WSM, folosește aceeași valoare și aici. Identitatea utilizatorului și tenantul
provin din sesiunea autentificată. Secretul textual se folosește ca atare;
nu îl decoda din base64 înainte de semnare.

### 6. Transmite numai JWT-ul către pagina WiseMED

Pagina PHP trebuie servită cu `Cache-Control: no-store`. Inserează tokenul prin
JSON encoding, nu prin concatenarea manuală a unui string JavaScript:

```php
<script src="https://wslocal.wisemed.eu/sdk/wsm.js"></script>
<script>
window.objWSM = WSMServer('wss://wslocal.wisemed.eu', <?= json_encode(
    $tokenWSM,
    JSON_HEX_TAG | JSON_HEX_AMP | JSON_HEX_APOS | JSON_HEX_QUOT | JSON_THROW_ON_ERROR
) ?>);
objWSM.onError = error => console.error(error.code, error.message);
</script>
```

Acesta este tokenul de conectare, nu secretul din fișierul `.key`. Mai jos sunt
explicate comenzile, callbackurile, permisiunile și reînnoirea tokenului după
expirare. Exemplul complet de endpoint autentificat este în
`examples/browser/token-endpoint.php`; adaptează verificările de sesiune la
aplicația WiseMED reală.

## Cum funcționează SDK-ul

SDK-ul este un client WebSocket al WSM. Fișierul public este inclus în binarul
WSM și este servit la `https://wslocal.wisemed.eu/sdk/wsm.js`, inclusiv în spatele
Nginx. Nu depinde de activarea consolei admin sau de directorul `/control`.
După recompilare/restart, configurația Nginx existentă cu `location /` și
`proxy_pass` către WSM servește și această adresă. Nu este necesară copierea JS
în aplicația WiseMED. Build-ul de distribuție include exemplele în `examples/browser/`.

Fișierul JS se încarcă prin HTTPS. **Toate operațiile WSM și apelurile către
readere/utilitare se transmit prin același WebSocket**: browser → WSM → client
remote → WSM → browser. `/api/...` este calea logică din comanda `api.request`,
nu un `fetch` către calculatorul echipamentului. SDK-ul nu execută niciun `fetch`.
Eventuala funcție de reînnoire JWT aparține aplicației WiseMED.

## Integrare minimă în pagina WiseMED

```html
<script src="https://wslocal.wisemed.eu/sdk/wsm.js"></script>
<script>
// tokenWSM este JWT-ul calculat și furnizat de backendul autentificat WiseMED.
window.objWSM = WSMServer('wss://wslocal.wisemed.eu', tokenWSM);
// Este acceptat și: new WSMServer(url, token, options).
objWSM.onConnect = function (params) {
  console.log('WSM Connected', params.connection_id);
  document.getElementById('wsm-control').classList.add('wsm-connected');
};
objWSM.onDisconnect = function (params) {
  document.getElementById('wsm-control').classList.remove('wsm-connected');
};
objWSM.onError = error => console.error(error.code, error.message);

async function readSIUIStatus() {
  await objWSM.ready; // Rezolvat numai după hello_ack, nu doar deschiderea socketului.
  const clients = await objWSM.getClientsByType('siui-bridge');
  if (clients === null) return null;
  // Alege explicit clientul: pot exista mai multe bridge-uri în același tenant.
  const client = clients[0];
  const response = await objWSM.callAPI(client, '/api/status', {method:'GET'});
  return response.body;
}
</script>
```

`WSMServer()` pornește conectarea automat în următorul microtask, astfel încât
callbackurile pot fi atribuite imediat după creare. `isConnected` este un boolean;
`connectionID` identifică sesiunea curentă. Nu există apeluri de rețea sincrone:
`ready`, `ping`, descoperirea clienților și operațiile API folosesc Promise/`await`.

## Descoperirea și alegerea clientului

```js
const readers = await objWSM.getClientsByType('reader'); // Array sau null
const siui = await objWSM.getClientsByType('siui-bridge');
const other = await objWSM.getClientsByType('orice-string'); // null dacă nu există
const all = await objWSM.listClients(); // Array, posibil gol
```

`getClientsByType` acceptă orice string și compară exact `client_type`, fără alias,
listă fixă sau conversie de litere. Întoarce întotdeauna un **Array când găsește
clienți**, chiar și pentru un singur client; în lipsa lor întoarce **null**.
O eroare de autentificare/rețea/permisiune respinge Promise-ul, nu întoarce null.

Fiecare obiect păstrează câmpurile protocolului (`id`, `client_id`, `client_type`,
`equipment_id`, `reader_id`, etc.) și adaugă aliasurile `connectionID`, `clientID`,
`clientType`, `equipmentID`, `readerID`.

În implementarea curentă readersv3, SIUI anunță `siui-bridge`, iar celelalte readere
și utilitare folosesc tipul comun `reader`. Numele aplicației/protocolului nu este
neapărat client_type. Pentru un anumit utilitar din acest grup, selectează după
`readerID` sau `equipmentID` din lista întoarsă. SDK-ul nu schimbă identitatea
anunțată de aplicații și poate comunica cu toate prin adaptorul API comun.
Nu schimbă nici regulile WSM pentru tipurile acceptate la înregistrarea unui client.

```js
const clients = await objWSM.listClients();
const device = clients.find(c => c.equipmentID === '18');
if (device) {
  await objWSM.callAPI(device, '/api/status');
  await objWSM.callAPI(device.clientID, '/api/status');
}
```

Obiectul selectat țintește o conexiune precisă. Un string este rezolvat printr-o
listă proaspătă după `clientID` sau `connectionID`; dacă mai multe sesiuni au același
clientID, apelul este refuzat cu `ambiguous_target`. Un equipment ID numeric nu este
interpretat automat ca clientID. După reconectarea echipamentului, reinteroghează
lista; connectionID se schimbă.

## API remote: Promise și callbackuri

```js
const clients = await objWSM.getClientsByType('siui-bridge');
if (clients) {
  objWSM.callAPI(
    clients[0].clientID,
    '/api/status',
    {method:'GET'},
    function onFail(error) { console.error(error.code, error.status, error.response); },
    function onSuccess(response) { console.log(response.status, response.body); }
  );
}
```

Semnătura: `callAPI(clientOrArray, path, params, onFail?, onSuccess?)`.
Callbackurile sunt opționale; metoda întoarce și un Promise. Dacă folosești `await`,
tratează respingerea prin `try/catch`, chiar dacă ai furnizat și `onFail`.

`params` este un obiect de opțiuni, nu corpul API implicit:

| Opțiune | Rol |
| --- | --- |
| `method` | GET implicit; GET, HEAD, POST, PUT, PATCH, DELETE |
| `query` | Obiect transformat în query string pe calea logică |
| `body` | Corp JSON; folosește aici parametrii operației |
| `body_base64` | Alternativă la body pentru date binare/multipart |
| `content_type` | Tipul corpului; implicit application/json în adaptor |
| `timeout` | Timeout local în milisecunde, implicit 35000 |

```js
const response = await objWSM.callAPI(device, '/api/my-operation', {
  method:'POST', body:{example:'value'}, timeout:35000
}); // Înlocuiește calea cu o rută implementată de aplicația țintă.
```

Succesul este răspunsul real `api.response`, cu `status`, `content_type`, `headers`,
`body` sau `body_base64`. Pentru base64 SDK-ul adaugă `bytes` (Uint8Array).
HTTP 4xx/5xx și celelalte statusuri non-2xx resping Promise-ul și apelează onFail;
`error.status` și `error.response` păstrează diagnosticul remote. ACK-ul de rutare
nu înseamnă succesul API-ului. Răspunsul este corelat cu cererea și conexiunea țintă;
un reply de la alt client nu poate rezolva acea cerere.

Un Array trimis ca prim argument face apeluri separate către **toți** clienții din
Array și întoarce un Array de răspunsuri, în aceeași ordine. La primul eșec Promise-ul
este respins; celelalte operații deja trimise nu sunt anulate. Pentru rezultate
parțiale controlate de aplicație folosește `Promise.allSettled`:

```js
const readers = await objWSM.getClientsByType('reader');
const results = readers ? await Promise.allSettled(
  readers.map(reader => objWSM.callAPI(reader, '/api/status'))
) : [];
```

Se aplică limitările adaptorului existent: rute de date `/api/...` și
`/barcode/print`, maximum 4 MiB corp/răspuns brut, fără HTML, streaming sau upgrade
WebSocket; sunt excluse autentificarea locală și rutele de relay/open/debug.
Operațiile hardware cer componentele și hardware-ul instalate. SDK-ul nu creează
API-uri noi în echipamente. Timeout-ul local nu oprește automat o operație remote.

## Restul protocolului WebSocket

| Funcție | Rezultat |
| --- | --- |
| `ping()` | Payload pong, inclusiv server_time |
| `serverStatus()` | server.status |
| `equipmentStatus(idOrArray)` | equipment.status |
| `serverCommand(name, args)` | Orice comandă WSM implementată, inclusiv equipment.list și wisemed.* |
| `controlTicket(equipmentID)` | control.ticket pentru UI remote existentă |
| `command(client, name, args, {timeout}?)` | Comandă generică 1:1, așteaptă reply-ul clientului |
| `subscribe(topic)` / `unsubscribe(topic)` | Confirmarea operației pe topic |
| `send(type, target, payload)` | command sau event către orice target acceptat de WSM |
| `reply(receivedCommand, payload)` | Răspuns la expeditorul comenzii primite |
| `disconnect()` | Oprire manuală, fără retry |
| `connect(token?)` / `reconnect(token?)` | Conectare explicită / reconectare cu JWT opțional nou |

`send` acceptă target-urile protocolului: connection, connections, reader, readers,
equipment, equipments, client_type, topic, all, self și server (pentru server
folosește `serverCommand`, care așteaptă reply în loc de ACK). `send` întoarce doar
ACK-ul de coadă cu `requestID`, `recipients` și `delivery`; pentru răspunsurile
multiple folosește `onReply`, corelând `message.correlation_id` cu `requestID`.
Un reply poate sosi înaintea ACK-ului: handlerul onReply trebuie instalat înainte
și aplicația trebuie să păstreze temporar răspunsurile până la primirea ACK-ului.
Zero recipients este afișat în ACK; `command`/`callAPI` îl tratează ca eroare.

```js
objWSM.onEvent = message => console.log(message.payload);
objWSM.onPresence = message => console.log(message.payload.event);
objWSM.onReply = message => console.log(message.correlation_id, message.payload);
objWSM.onCommand = async message => {
  // Validează numele și argumentele înainte de executarea operațiilor proprii.
  await objWSM.reply(message, {ok:false,error:{code:'unsupported_command',message:'Unsupported'}});
};
await objWSM.subscribe('logs:reader-1');
await objWSM.send('event', {mode:'client_type',client_type:'siui-bridge'}, {
  event:'example', data:{value:1}
});
```

Evenimente: `onConnect(payload)`, `onDisconnect({code,reason,wasClean})`,
`onError(error)`, `onTokenRequired()`, `onEvent(envelope)`, `onCommand(envelope)`,
`onReply(envelope)`, `onPresence(envelope)`, `onMessage(envelope)`.
Callbackurile async sunt acceptate; erorile lor sunt direcționate la onError.

## Token, reconectare și permisiuni

Tokenul este trimis în subprotocoalele `wsm.v1`, `wsm.jwt.<JWT>`. Nu este introdus
în URL sau în localStorage. WSM verifică semnătura, tenantul, identitatea, originea
și drepturile. SDK-ul decodează doar identitatea necesară mesajului hello.

Implicit SDK-ul reîncearcă după 30 de secunde. Nu reexecută cereri trimise înaintea
unei deconectări; toate cererile în așteptare sunt respinse. Un timeout poate însemna
că operația a fost deja executată. Abonamentele se pierd la reconectare; refă-le în
onConnect și reinteroghează starea. Folosește idempotency în API pentru operații cu
efecte. `autoReconnect:false` dezactivează retry-ul.

Dacă ai furnizat doar un token și acesta expiră, SDK-ul emite onTokenRequired și
oprește retry-ul până la `reconnect(newToken)`. Alternativ:

```js
const getToken = async () => {
  const response = await fetch('/my-wisemed/wsm-token.php', {
    credentials:'same-origin', cache:'no-store'
  });
  if (!response.ok) throw new Error('WiseMED session or WSM permission missing');
  return (await response.json()).token;
};
const objWSM = WSMServer('wss://wslocal.wisemed.eu', null, {tokenProvider:getToken});
```

Providerul este apelat la fiecare conectare, inclusiv prima; dacă este prezent,
valoarea returnată de el înlocuiește tokenul din constructor. Reînnoirea se face
prin backendul WiseMED, niciodată cu secretul în browser. Un handshake refuzat nu
expune în browser corpul HTTP 401/403; diagnosticul exact se citește din logul WSM.

Minim pentru listare și API: `connections:read`, `route:command`, `api:invoke`.
Adaugă `server:status` pentru status, `topics:subscribe` pentru abonări,
`route:reply` pentru răspunsuri și `route:event` pentru evenimente.
Listele/broadcasturile cer și `route:broadcast`. `api:admin` este necesar numai
pentru operații administrative remote autorizate. `wisemed:proxy` dă acces la
operațiile proxy WiseMED configurate. Nu acorda automat toate drepturile.
Administrarea cheilor din `/admin/` nu face parte din API-ul WebSocket și rămâne
în consola admin, cu autentificarea proprie.

## Generare JWT în PHP

Fișiere complete:

- `examples/browser/wsm-token.php`: funcția `wsmBrowserToken`, fără dependențe Composer.
- `examples/browser/token-endpoint.php`: exemplu de endpoint cu sesiune și autorizare.
- `examples/browser/wisemed-page.html`: exemplu de pagină integrată.

Endpointul este un exemplu care refuză accesul implicit. Adaptează verificările de
sesiune/autorizare la aplicația WiseMED reală. Câmpurile demonstrative
`$_SESSION['user_id']` și `$_SESSION['wsm_access_allowed']` trebuie setate numai de
codul de login/autorizare de încredere; ele nu sunt o integrare deja instalată.
Tenantul, cheia și lista de scopes provin din configurația serverului și din
permisiunile utilizatorului, niciodată din parametrii trimiși de browser.

```php
require_once '/private/wsm-token.php';
// După autentificarea și autorizarea utilizatorului în WiseMED:
$config = [
    'kid' => 'wisemed-web',
    'tenant_id' => 'wisemed-local',
    'issuer' => 'ISSUER_EXACT_DIN_CONFIG_WSM',
    'audience' => 'wsm-server',
    'secret_file' => '/private/keys/wisemed-web.key',
    'allowed_scopes' => ['route:command', 'connections:read', 'server:status', 'api:invoke'],
];
$tokenWSM = wsmBrowserToken($config, 'user-' . $authenticatedInternalUserID,
    ['route:command', 'connections:read', 'server:status', 'api:invoke'], 300);
```

Cheia trebuie configurată în WSM, păstrând restul configurației:

```yaml
security:
  keys:
    wisemed-web:
      tenant_id: wisemed-local
      secret_file: keys/wisemed-web.key
      roles: [browser]
      scopes: ["route:command", "connections:read", "server:status", "api:invoke"]
tenants:
  wisemed-local:
    issuer: ISSUER_EXACT_DIN_CONFIG_WSM
    allowed_origins: ["https://php.wisemed.eu"]
    # Păstrează și configurarea wisemed existentă.
```

Folosește același conținut secret (minimum 32 bytes) pe backendul PHP și în WSM;
fișierele pot avea căi diferite. Nu confunda cheia browser/backend cu cheia de
aparat generată de `add`, ale cărei roluri sunt reader/service. `kid` selectează
cheia statică. Pentru o cheie existentă, folosește ID-ul și permisiunile ei reale.
`issuer` este EXACT valoarea existentă a tenantului; nu îl deduce automat din URL-ul
WSS. `allowed_origins` conține originea paginii WiseMED, fără cale/slash final,
nu doar originea serverului WSM. Păstrează celelalte origini necesare.

Helperul emite JWT HS256 cu `kid`, `iss`, `aud`, `sub`, `tenant_id`, `role=browser`,
`client_id` unic, scopes, iat, nbf, exp. Semnătura folosește
[`hash_hmac('sha256', ..., $secret, true)`](https://www.php.net/manual/en/function.hash-hmac.php),
cu rezultat binar convertit în base64url. Secretul textual nu se decodează din
base64. TTL implicit este 300s și trebuie să respecte limita WSM. Sincronizează
ceasurile PHP și WSM.

Pentru inserarea directă a tokenului într-un script PHP/HTML, encodează în JSON:

```php
<script src="https://wslocal.wisemed.eu/sdk/wsm.js"></script>
<script>
window.objWSM = WSMServer('wss://wslocal.wisemed.eu', <?= json_encode(
    $tokenWSM, JSON_HEX_TAG | JSON_HEX_AMP | JSON_HEX_APOS | JSON_HEX_QUOT | JSON_THROW_ON_ERROR
) ?>);
</script>
```

Trimite pagina/endpointul cu `Cache-Control: no-store`. Adaptează nonce-ul scriptului
inline și directivele CSP ale paginii: `script-src` trebuie să permită originea
HTTPS WSM și `connect-src` originea WSS. Nu loga tokenul sau secretul.

## Wrapper SIUI Bridge

Pentru toate endpointurile SIUI și polling-ul operațiilor CNAS, încarcă
`/sdk/siui-bridge.js` și folosește `await SIUIBridge.create(objWSM, {equipmentID:"18"})`.
[Documentație, inventar complet și anularea așteptării](siui-browser-wrapper.md).
