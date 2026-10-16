# SIUIBridge — toate API-urile SIUI prin WSMServer

Încarcă cele două fișiere servite de WSM:

```html
<script src="https://wslocal.wisemed.eu/sdk/wsm.js"></script>
<script src="https://wslocal.wisemed.eu/sdk/siui-bridge.js"></script>
```

Wrapperul primește obiectul WSM deja creat. Nu deschide un socket nou, nu cere
JWT local SIUI și nu execută fetch/HTTP către SIUI Bridge. Toate cererile,
inclusiv descoperirea, polling-ul și descărcările, folosesc socketul lui objWSM.

```js
await objWSM.ready;
const siui = await SIUIBridge.create(objWSM, {equipmentID:'18'});
if (siui === null) {
  afiseazaMesaj('SIUI Bridge nu este conectat.');
  return;
}
const status = await siui.getStatus();
```

`equipmentID`, `clientID`, `connectionID` sunt selectoare opționale. Fără selector
este acceptat numai un singur bridge online; mai multe produc `ambiguous_bridge`.
Fără clienți potriviți, `create()` returnează null. Lipsa conexiunii WSM sau o eroare
WSM respinge Promise-ul. Alternativ: `const siui = SIUIBridge(objWSM, options)`
sau `new SIUIBridge(...)`, apoi `await siui.ready` (client sau null).

Referința este `siui.client`. Fiecare cerere reîmprospătează descoperirea și verifică
bridge-ul înainte de trimitere. Se păstrează identitatea echipamentului ales;
wrapperul nu trece automat la alt bridge dacă primul dispare. `isConnected`
reflectă ultima descoperire și conexiunea WSM, nu garantează că următorul apel va
reuși. `await siui.refresh()` verifică din nou și întoarce clientul sau null.
La restartul bridge-ului, un selector equipmentID/clientID poate găsi noua conexiune;
un selector connectionID rămâne legat de sesiunea veche.

## API-uri simple și formatul rezultatului

Toate metodele sunt async și întorc corpul JSON al răspunsului API, fără anvelopa
`api.response` a WSM. Erorile HTTP rămân erori cu `status` și `response` de la SDK;
`ok:false` produce de asemenea o eroare. Se recomandă `try/catch` în toate fluxurile.

Metodele GET primesc `(options = {})`. Metodele POST/PUT auxiliare primesc
`(body = {}, options = {})`. Options: `signal`, `query`, `requestTimeout` în ms.
Default requestTimeout este 135000 ms. Exemplu certificate:

```js
const list = await siui.getCertificates({query:{store:'CurrentUser'}});
const status = await siui.getTokenPINStatus(); // Doar starea; nu returnează PIN-ul.
```

| Metodă wrapper | Metodă și cale API |
| --- | --- |
| `getStatus` | GET /api/siui/status |
| `getCatalog` | GET /api/siui/catalog |
| `getOpenAPI` | GET /api/siui/openapi.json |
| `createAPIToken` | POST /api/siui/auth/token |
| `getSettings` | GET /api/siui/settings |
| `saveSettings` | PUT /api/siui/settings |
| `getCertificates` | GET /api/siui/certificates |
| `refreshCertificates` | POST /api/siui/certificates/refresh |
| `getTokenPINStatus` | GET /api/siui/token-pin |
| `setTokenPIN` | PUT /api/siui/token-pin |
| `getOCSPDebug` | GET /api/siui/general/ocsp |
| `getOCSPSession` | POST /api/siui/general/ocsp |
| `testOCSP` | POST /api/siui/ocsp-test |
| `getPersonalization` | POST /api/siui/personalization |
| `checkInsured` | POST /api/siui/insured |
| `getInvoiceModel` | GET /api/siui/invoice/model |
| `previewInvoice` | POST /api/siui/invoice/preview |
| `packageInvoice` | POST /api/siui/invoice/package |
| `submitValidation` | POST /api/siui/validations |
| `listValidations` | GET /api/siui/validations |

Endpointuri cu identificator:

| Metodă | API prin WSM |
| --- | --- |
| `getCall(id, options)` | GET /api/siui/calls/{id} |
| `getCallFileChunk(id, offset, options)` | GET /api/siui/calls/{id}/file?offset=... |
| `getValidation(id, options)` | GET /api/siui/validations/{id} |

`createAPIToken` expune endpointul existent pentru completitudine; JWT-ul local
returnat NU este folosit de wrapper. `getOCSPSession` poate returna tokenul OCSP.
Nu loga/persista aceste răspunsuri sau PIN-ul trimis la `setTokenPIN`.
`setTokenPIN({pin:'...',manual:true})` furnizează PIN temporar;
`setTokenPIN({forget:true})` îl șterge conform regulilor bridge-ului.

## Cele 34 de operații CNAS

Fiecare operație are două forme echivalente:

```js
const call = await siui.general.getInsured({
  pid: cnp,
  requestDate: '2026-10-16T00:00:00+03:00'
}, {
  operationId: crypto.randomUUID(),
  onProgress: call => actualizeazaMesaj(call.status)
});
// Identic: siui.getInsured(parameters, options)
console.log(call.status); // completed
// call.result.values conține răspunsurile CNAS; completed nu înseamnă „asigurat”.
```

Parametrii sunt obiecte cu exact câmpurile WSDL, toate valori string, inclusiv
cele numerice. Șirurile XML se transmit ca string JSON; nu le transforma în SOAP.
Wrapperul verifică numele/câmpurile și păstrează validarea semantică în bridge.
Metodele sunt și sub `general`, `clinice`, `facturare`, și direct pe obiect.

| Metodă | Parametri string | Administrator |
| --- | --- | --- |
| `general.getCatalogues` | `partnerCategory`, `start` | Nu |
| `general.getProviderInfo` | `partnerCategory`, `start`, `stop`, `uic` | Nu |
| `general.getProviderInfoForPhysician` | `partnerCategory`, `start`, `stop`, `uic`, `stencilNo` | Nu |
| `general.sendReport` | `reportType`, `reportXML` | Da |
| `general.getReportFeedback` | `fileName` | Nu |
| `general.getRefund` | `partnerCategory`, `start`, `stop`, `uic` | Nu |
| `general.getRefundForPhysician` | `partnerCategory`, `start`, `stop`, `uic`, `stencil` | Nu |
| `general.getRefundForInvoice` | `partnerCategory`, `invoiceNumber`, `invoiceDate`, `uic` | Nu |
| `general.getPhysicianCatalogues` | Obiect gol `{}` | Nu |
| `general.getInsured` | `pid`, `requestDate` | Nu |
| `general.getInsuredByCID` | `cid`, `requestDate` | Nu |
| `general.getPersonInfo` | `pid`, `fromDate` | Nu |
| `clinice.getPrescription` | `serial`, `number`, `pid`, `stencil` | Nu |
| `clinice.getClinicReferral` | `serial`, `number`, `pid`, `stencil` | Nu |
| `clinice.getLabReferral` | `serial`, `number`, `pid`, `stencil` | Nu |
| `clinice.validatePrescription` | `reportXml`, `reportType` | Da |
| `clinice.validateClinicReferral` | `reportXml`, `reportType` | Da |
| `clinice.validateLabReferral` | `reportXml`, `reportType` | Da |
| `clinice.validateSickLeave` | `reportXml` | Da |
| `general.validateReport` | `reportXml`, `reportType`, `requestType` | Da |
| `clinice.processPrescribedPrescription` | `reportXml` | Da |
| `clinice.getPrescribedPrescription` | `providerCode`, `physicianStencilNo`, `contractNo`, `contractType`, `insuranceHouseCode`, `series`, `no`, `prescriptionDate` | Nu |
| `clinice.cancelPrescribedPrescription` | `providerCode`, `physicianStencilNo`, `contractNo`, `contractType`, `insuranceHouseCode`, `series`, `no`, `prescriptionDate`, `cancellationReason` | Da |
| `clinice.downloadNotReportedPrescriptionsReport` | `insuranceHouseCode`, `fromDate`, `toDate`, `providerCode`, `stencilNo` | Nu |
| `clinice.getStatusForPrescriptions` | `insuranceHouseCode`, `providerCode`, `contractNo`, `contractType`, `startFrom`, `endTo`, `workPlaceCode` | Nu |
| `clinice.getPrescribedPrescriptionsForCid` | `requestXml` | Nu |
| `clinice.processHospitalPrescription` | `reportXml` | Da |
| `clinice.cancelReleasedHospitalPrescription` | `providerCode`, `workplaceCode`, `insuranceHouseCode`, `series`, `no`, `fractionNo`, `cancellationReason` | Da |
| `clinice.getReleasedHospitalPrescription` | `providerCode`, `workplaceCode`, `insuranceHouseCode`, `series`, `no`, `fractionNo` | Nu |
| `facturare.sendEInvoice` | `invoiceFile` | Da |
| `facturare.getEInvoice` | `serialCode`, `serialNumber`, `providerFiscalCode` | Nu |
| `facturare.getEInvoiceId` | `serialCode`, `serialNumber`, `providerFiscalCode` | Nu |
| `facturare.getEInvoiceClearingNote` | `serialCode`, `serialNumber`, `providerFiscalCode` | Nu |
| `facturare.cancelEInvoice` | `serialCode`, `serialNumber`, `providerFiscalCode`, `cancellationDate`, `cancellationReason` | Da |

Inventarul este derivat din `readersv3/modules/siui/assets/operations.json`;
testul de paritate detectează orice operație nouă neadăugată în wrapper.
Corpurile și semantica endpointurilor rămân cele din
[API SIUI Bridge](../../readersv3/apps/siui-bridge/docs/API.md).

## Așteptarea execuțiilor și idempotency

`operationId` este obligatoriu pentru cele 34 de operații. Generează-l o singură
dată pentru o intenție de lucru și păstrează-l pentru reconcilierea acelei intenții.
Nu genera un ID nou la fiecare retry al unei operații cu efecte.

Metodele de operație trimit POST o singură dată, apoi interoghează GET calls/{id}
până la finalizare. Default: polling la 1s, timeout de așteptare 180s după
submit. Configurabile prin `pollInterval`, `timeout`; timeoutul fiecărei cereri
este separat (`requestTimeout`). Nu există retry automat de POST.

- `completed`: întoarce obiectul call; interpretează rezultatul CNAS separat.
- `failed`: eroare `siui_failed`, cu `error.call`.
- `unknown`: eroare `siui_unknown`, cu `error.call`; reconciliază înainte de retrimitere.
- `wait_timeout`: operația poate continua pe bridge.
- `aborted` / `AbortError`: s-a oprit doar așteptarea locală.

```js
const submitted = await siui.general.getInsured(parameters, {
  operationId: idStabil,
  wait: false
}); // call queued/running/completed, fără polling
const completed = await siui.waitCall(submitted.id, {timeout:180000});
```

Forma generică `submitOperation(group, name, parameters, options)` întoarce corpul
inițial `{ok, call, ...}`; `executeOperation(...)` este echivalentul metodelor numite.
Erorile din submit păstrează `operationId`; după confirmarea call.id, erorile din
fluxul combinat păstrează și `callId`. O eroare de transport la submit nu dovedește
că operația nu a fost acceptată. Wrapperul nu retransmite automat.

Pentru endpointul legacy de validări:

```js
const submitted = await siui.submitValidation({operation_id:idStabil, xml:xmlValidare});
const job = await siui.waitValidation(submitted.job.id);
```

`waitValidation` folosește GET validations/{id}, cu aceleași reguli de timeout,
anulare și stări terminale. Endpointurile legacy și operațiile generale rămân
distincte; nu se substituie automat unul altuia.

## Verificare asigurat și abandonarea așteptării

Pentru răspunsul interpretat al endpointului `/insured`:

```js
const controller = new AbortController();
try {
  const response = await siui.checkInsured({
    mode:'cnas', cnp:cnp, date:'2026-10-16'
  }, {signal:controller.signal});
  if (response.result?.insured === true) {
    // Continuă fluxul autorizat de aplicația WiseMED.
  } else if (response.result?.insured === false) {
    // Răspuns neasigurat; nu îl confunda cu o eroare de transport.
  } else {
    // Statut neinterpretabil/necunoscut.
  }
} catch(error) {
  if (error.name !== 'AbortError') afiseazaEroare(error.message);
}
// Din handlerul butonului „Oprește așteptarea / folosește adeverința”:
controller.abort();
```

Butonul trebuie să apeleze controller.abort() dintr-un handler separat, cât timp
cererea este în curs. Wrapperul respinge imediat așteptarea și ignoră rezultatul
întârziat. Nu deconectează objWSM, nu anulează pe CNAS și nu setează automat
„asigurat”. Alegerea adeverinței și auditul ei rămân în backendul WiseMED.
`mode:'local'` produce exclusiv fixture de test; nu îl folosi pentru decizii reale.

## Fișiere

```js
const bytes = await siui.downloadCallFile(call.id, {
  signal:controller.signal,
  onProgress: ({receivedBytes}) => actualizeazaProgres(receivedBytes)
});
const blob = new Blob([bytes], {type:'application/octet-stream'});
```

Descărcarea folosește fragmente prin WebSocket, verifică offseturile și oprește
bucla la EOF. Limita implicită este 64 MiB; `maxBytes` poate impune o limită mai
mică. Nu încarcă un URL CNAS direct în browser. Calea/formatul fișierului se citesc
din metadatele call.result.file; wrapperul nu inventează extensia.

## Instalare și drepturi

Recompilează/repornește WSM pentru publicarea `/sdk/siui-bridge.js`. Endpointul JS
este public, datele rămân autentificate prin WSM. Include scriptul după wsm.js.
JWT-ul browserului necesită `connections:read`, `route:command`, `api:invoke`;
`api:admin` este cerut de operațiile administrative SIUI (setări, certificate/PIN,
OCSP cu token, semnare și operații CNAS mutante). Reglementarea drepturilor rămâne
pe server; wrapperul nu le escaladează. Identitatea utilizatorului (`sub`) și
tenantul trebuie păstrate la reînnoirea JWT ca să poată consulta aceleași execuții.

Adaptorul comun de pe reader/utilitar avea o limită de 30s, mai mică decât limita
OCSP de 120s. Pentru POST general/ocsp, ocsp-test și invoice/package, limita WSM
este acum 130s, cu anularea contextului păstrată. Pentru aceste operații lungi
recompilează și SIUI Bridge cu adaptorul actualizat. Celelalte endpointuri își
păstrează timeouturile. Durata JWT trebuie să acopere operația planificată.

Validarea dezvoltării folosește fixture-uri și un peer SIUI simulat, fără acces
la tokenul fizic sau apeluri CNAS reale.
