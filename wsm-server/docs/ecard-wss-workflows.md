# WiseMED: fișe și semnare cu cardul prin WSS

Exemplul folosește API-ul actual `wisemed-pias-ecard` și SDK-ul WSM existent,
`/sdk/wsm.js`. Browserul vorbește numai cu WSM pentru accesul la stație; nu
apelează IP-ul stației și nu încarcă DLL-uri. Crearea fișelor și salvarea
semnăturilor aparțin backendului WiseMED autentificat.

Fișiere:

- [`ecard-workflows.js`](../examples/browser/ecard-workflows.js): orchestratorul reutilizabil.
- [`ecard-wss-test.html`](../examples/browser/ecard-wss-test.html): interfață de test cu WSS/card reale și fișe simulate.
- [`ecard-workflows.test.cjs`](../examples/browser/ecard-workflows.test.cjs): teste automate cu transport și card simulate.
- [API-ul complet al stației](../../readersv3/apps/wisemed-pias-ecard/docs/API.md).

## Test manual WSS, din interfață

Din rădăcina repository-ului:

```sh
./wsm-server/scripts/serve-ecard-example.sh
```

Deschide `http://127.0.0.1:8088/ecard-wss-test.html`. Pentru alt port:

```sh
ECARD_EXAMPLE_PORT=8098 ./wsm-server/scripts/serve-ecard-example.sh
```

Scriptul servește numai cele trei fișiere publice ale exemplului, dintr-un
director temporar; nu expune configurațiile sau cheile repository-ului.

1. Stația Windows trebuie să aibă `wisemed-pias-ecard` conectat la același WSM,
   cu înrolare locală și un card/cititor funcțional.
2. Originea `http://127.0.0.1:8088` trebuie permisă în politica WSM a browserului
   pentru acest test. Nu dezactiva verificarea certificatului TLS.
3. Introdu adresa WSS și un JWT de **browser** emis de backendul tău. Drepturi:
   `route:command`, `api:invoke`, `connections:read`. Cheia WSM trebuie să permită
   aceste drepturi și rolul browser. Stația publică evenimente cu `route:event`
   și răspunde cu `route:reply`. Exemplul nu necesită `api:admin`.
4. Apasă **Conectează WSS**, alege echipamentul eCard și completează numele
   fizic exact al cititorului și contractul real, identic cu înrolarea stației.
   Lista poate include alte echipamente; orchestratorul verifică API-ul eCard.
5. Alege unul dintre cele două fluxuri descrise mai jos. PIN-ul se introduce
   în dialogul paginii sau pe tastatura cititorului, în funcție de hardware.

Nu introduce cheia secretă WSM în pagină. JWT-ul și PIN-ul nu se salvează în
localStorage/sessionStorage sau în jurnal. La expirarea JWT-ului, introdu un
JWT nou și reconectează. Pentru schimbarea adresei/echipamentului/contractului,
încheie testul și folosește Deconectează înainte de un nou test. În producție,
folosește `WSMServer(..., {tokenProvider})` cu emitentul WiseMED existent.

### Fluxul 1: card introdus → fișă → servicii → semnături

Apasă **Mod 1**. Dacă nu există card, pagina așteaptă `card_inserted`; dacă este
introdus deja, îl detectează prin operația `status`, fără să aștepte un eveniment
nou. Citește A1/B1/B2/B4/C1 și creează o fișă de test în memoria paginii.

Apasă **Adaugă serviciul și declanșează semnarea**. Serviciul este adăugat în
fișa simulată, se pregătește textul și se lansează semnarea automat. După un
succes, serviciul apare `semnat`. La retragere se invalidează cardul citit;
următoarea introducere pornește din nou fluxul de creare. Backendul real decide
idempotent dacă deschide o fișă nouă sau reutilizează una deja deschisă.

### Fluxul 2: fișă existentă → verificare card → semnare

Completează ID-ul fișei de test și **CNP/CID-ul pacientului așteptat**. Apasă
**Mod 2**. La prima deschidere, exemplul pregătește un serviciu `TEST-SERVICIU`
nesemnat. Dacă există un card cu identitatea corectă, începe semnarea automat;
dacă nu există, așteaptă introducerea lui. Un CNP diferit blochează semnarea.
Pentru CNP vid sau `0000000000000`, CID-ul așteptat este obligatoriu.

Pagina semnează **texte de test cu cardul real**, dar nu creează fișe reale în
WiseMED și nu trimite rapoarte de servicii la CNAS. Starea testului se pierde la
reîncărcarea paginii. Backendul real trebuie să păstreze planul/operația pentru
recuperare după navigare, reîncărcare sau deconectare.

## Integrarea în interfața WiseMED

Copiază `ecard-workflows.js` în asseturile WiseMED și încarcă-l după `wsm.js`.
Exemplul nu adaugă un endpoint nou în serverul WSM și nu cere recompilarea lui.

```js
// objWSM este instanța deja folosită de interfața WiseMED.
// backend = adaptorul către funcțiile/FID-urile existente în aplicația ta.
// Cere PIN printr-un dialog type=password; vezi pagina de test pentru implementare.
async function configureECardUI({objWSM, equipmentID, readerName, contract,
                                backend, requestPIN, showState, showEvent, showError,
                                initialFisa, initialPlan}) {
  const flow = new WiseMEDECard.Workflow({
    wsm: objWSM, equipmentID, readerName, contract, backend, requestPIN,
    onState: showState, onEvent: showEvent, onError: showError
  });
  // Dacă pagina este deja pe o fișă, setează contextul înainte de start():
  // pornirea nu trebuie să creeze o altă fișă.
  if (initialFisa) { flow.mode = 'existing'; flow.fisa = initialFisa; }
  await flow.start({pendingPlan: initialPlan});
  if (initialPlan) await flow.resume();
  return flow;
}

// Flux 1: ecranul de recepție, fără fișă deschisă.
const flow = await configureECardUI({
  objWSM, equipmentID: 'ID_ECHIPAMENT',
  readerName: 'NUMELE_FIZIC_SELECTAT', contract,
  backend, requestPIN, showState, showEvent, showError
});

// După ce backendul a salvat serviciile, nu înainte:
await flow.servicesSaved();

// Flux 2: apelat de navigarea existentă la deschiderea unei fișe.
await flow.enterFisa({
  id: fisa.id,
  expected: {cnp: fisa.cnp, cid: fisa.cid}
});

// Înapoi la recepție / creare la card introdus:
await flow.createOnCard();

// În handlerul WSM onConnect, la reconectare:
await flow.reconnect(); // Reface abonarea; nu retrimite operațiile.

// La închiderea componentei UI:
await flow.stop();     // Nu anulează automat o semnare deja acceptată.
```

Păstrează un singur orchestrator activ pe componenta de lucru și selectează
explicit echipamentul/cititorul. `start()` păstrează handlerii `onEvent` și
`onDisconnect` anteriori și îi apelează înaintea logicii proprii. Deconectarea
invalidează cardul memorat în UI. Nu instala ulterior alți handleri care îi
înlocuiesc fără să le transmită mesajele. `reconnect()` trebuie
legat în handlerul `onConnect` al aplicației, după pornirea inițială, fără să fie
apelat de două ori pentru aceeași reconectare.

## Contractul adaptorului de backend

Numele de mai jos sunt **callbackuri ale exemplului**, nu endpointuri sau FID-uri
noi care ar exista deja în WiseMED. Le legi la implementarea ta existentă.

### `createFisaFromCard({card, expected, readOperationId})`

`card` conține codurile citite: A1=număr card, B1=nume, B2=prenume,
B4=CNP, C1=CID. `expected` este `{cnp,cid}`. Backendul autorizează operatorul,
identifică pacientul și creează/găsește fișa, cu prevenirea duplicatelor pentru
apeluri repetate, mai multe taburi și reconectări. Întoarce:

```json
{"id":"ID_FISA","expected":{"cnp":"CNP_PACIENT","cid":"CID_PACIENT"}}
```

Dacă identitatea este anonimă sau incompletă, aplică identificarea pacientului
prin CID conform regulilor tale, fără să inventezi un CNP. Datele citite din
browser nu înlocuiesc autorizarea backendului.

### `prepareSigning({fisa, card})`

Backendul selectează serviciile noi/modificate și nesemnate ale acelei fișe.
Întoarce `null` dacă nu există nimic de semnat. Altfel **persistă înainte de WSS**
un plan imuabil cu UUID unic, versiunea fișei, identitatea pacientului, ID-urile și
textele exacte, apoi îl întoarce:

```json
{
  "operationId":"UUID_PERSISTAT_DE_BACKEND",
  "fisaId":"ID_FISA",
  "version":17,
  "expected":{"cnp":"CNP_PACIENT","cid":"CID_PACIENT"},
  "encoding":"ascii",
  "certificate":"insured",
  "items":[{"id":"ID_SERVICIU","text":"TEXTUL_EXACT_PREGATIT_DE_BACKEND"}]
}
```

Planul este legat de utilizator, tenant, echipament, fișă, versiune și conținut.
Apelurile concurente pentru același set de servicii trebuie să primească același
plan/UUID. Nu genera un UUID nou la fiecare polling, eveniment sau reconectare.
Nu recalcula data, nu tăia spațiile și nu modifica newline-urile după pregătire.

### `saveSigningResult({plan, report})`

Primește raportul complet, inclusiv `used_sdk`, `events`, `errors`, `other_sdks`
și rezultatele. Pentru semnare, `report.response.data` este un array:

```json
[
  {"id":"ID_SERVICIU_1","ok":true,"signature":"CMS_DER_BASE64"},
  {"id":"ID_SERVICIU_2","ok":false,"code":"card_changed"}
]
```

Salvează idempotent numai semnăturile confirmate și leagă-le de textul exact,
versiunea și pacientul din plan. Validează CMS-ul și conținutul în backend înainte
de a marca serviciul semnat. Nu accepta identitatea și asocierea fișă–serviciu
numai din cererea browserului. Un rezultat final eșuat poate conține semnături
valide pentru primele servicii: se păstrează și acelea.

Un succes de semnare nu înseamnă validare online a serviciilor sau acceptare CNAS.
În caz de stare `unknown`, păstrează rezultatele confirmate și trimite restul la
reconciliere; nu lansa automat încă un batch. Dacă salvarea în backend eșuează,
`resume()` recuperează același raport și reapelează salvarea, fără o nouă semnare.

După reîncărcarea paginii, încarcă planul deschis din backend și contextul
fișei **înainte de pornire**: `await flow.start({pendingPlan: plan})`, apoi
`await flow.resume()`. Astfel, pornirea nu declanșează alt batch înainte de
recuperarea celui existent. Acest apel face numai GET, nu postează
o altă operație. Dacă jobul nu există (404 după un POST cu răspuns incert), nu se
creează automat un înlocuitor; reconciliază backendul/stația și, numai explicit,
retrimite **același UUID și același corp**, niciodată un UUID nou pentru aceleași
servicii. Nu șterge jobul înainte de salvarea și reconcilierea rezultatului.

## Legătura cu implementarea veche

Am comparat `old-src/WiseMEDeCARDManager/Server/WMeCardWebSocketService.cs` și
`WMCEASCard.GetSignature`. Acolo, `cmd:read`/`cmd:sign` erau comenzi pe socketul
local; acum acestea sunt operații asincrone rutate de WSM la stația selectată.
PIN-ul nu se expune ca token sau răspuns public.

Textul vechi era:

```text
CID|NUMAR_CARD|ServiceDate[|SecondDate]|Code
```

`WiseMEDECard.legacyServiceText()` reproduce concatenarea exactă, inclusiv cazul
în care `SecondDate` este string gol (se adaugă separatorul). Nu impune un format
nou de dată. În producție, backendul trebuie să pregătească aceleași valori
canonice ca înainte. Exemplul de test folosește date ISO și coduri TEST, fără
pretenția că acestea sunt formatul raportării tale.

Corespondența rezultatelor vechi:

- `ComputedHashB64` ← `signature` (CMS DER Base64);
- `SignedData` ← textul exact din plan;
- `ComputedHash` ← SHA-1 al **octeților CMS**, în hexazecimal uppercase, ca în
  `WMCEASCard.CalculateSHA1`, numai dacă modelul tău existent îl mai cere;
- `FileID`/`services[].Id` ← `fisaId`/`items[].id` din planul backendului.

Verificarea identității actuale este strictă: un CNP normal diferit este respins
chiar dacă CID coincide. Pentru CNP vid/anonimizat este necesar CID așteptat.
SDK-ul verifică din nou identitatea în aceeași sesiune înainte de semnare și
continuitatea cardului înainte/după fiecare semnătură; verificarea din browser
servește interfeței și nu înlocuiește această protecție.

## Teste automate și limitele exemplului

```sh
node --test wsm-server/examples/browser/ecard-workflows.test.cjs
```

Testele folosesc un transport și card simulate; verifică cele două fluxuri,
CNP/CID diferit/anonim, PIN cu challenge, textul exact, retragerea cardului,
rezultatele parțiale, UUID-ul stabil și recuperarea fără replay. Nu demonstrează
că CNAS/cardul fizic funcționează. Pagina HTML permite testul real WSS + Windows.

Abonarea la evenimente este reînnoită la 20 minute; după reconnect se creează o
abonare nouă. Polling-ul jobului rămâne sursa de adevăr pentru PIN și finalizare,
deoarece livrarea live este best-effort. Nu se păstrează JWT/PIN în stocare browser.
Textele, datele pacientului și rapoartele rămân în memoria componentei pentru
operația curentă; persistența de business aparține backendului WiseMED.

POST-ul trebuie să respecte limita WSM de 4 MiB. Pentru loturi mai mari, folosește
API-ul de încărcare în chunk-uri descris în documentația eCard; exemplul postează
batch-ul într-un singur corp. Rapoartele peste 3 MiB sunt reconstruite din părți,
cu verificarea SHA-256, până la 64 MiB în acest exemplu; pentru rapoarte mai mari,
consumă rezultatele paginate în backend sau adaptează limita explicit.
