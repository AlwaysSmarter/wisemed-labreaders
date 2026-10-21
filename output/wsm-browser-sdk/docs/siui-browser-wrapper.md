# Singleton global objWSMSIUI — API recomandat

**Începe cu [ghidul practic și exemplele JavaScript complete](siui-browser-quickstart.md).**
Callback-ul implementat se numește **callwmapi**, nu callwsapi.

Include siui-bridge.js în pagina care folosește deja objWSM. Scriptul publică
automat un singur objWSMSIUI și detectează objWSM la fiecare apel, inclusiv dacă
acesta a fost inițializat după încărcarea scriptului. Nu este necesară configurare. Nu mai trebuie SIUIBridge.create sau
await objWSM.ready.

~~~js
const result = await objWSMSIUI.general.getInsured({
  pid: '1000000000000',
  requestDate: '2026-10-18T00:00:00+03:00'
}, {
  operationId: crypto.randomUUID(), // păstrează ID-ul pentru aceeași intenție/retry
  onProgress: call => console.log(call.status)
});
if (result.success) {
  console.log(result.data.result.insured);
} else {
  console.error(result.error); // string, nu obiect Error
}
~~~

Toate metodele singletonului rezolvă Promise-ul cu:
- succes: {success:true, data:...};
- eșec: {success:false, error:"mesaj", code:"cod", ...}.

status (HTTP), operationId, callId și attempts pot apărea separat la eroare.
Nu există Promise respins pentru erorile de transport, reconectare, timeout,
parametri, CNAS, procesare sau callback-uri ale aplicației. TotalTimeout limitează
întregul apel, inclusiv un getWSM/onProgress blocat. Expirarea sau anularea locală
nu garantează oprirea operației pe CNAS; POST-ul nu este repetat automat.
Conectarea WSM nu înseamnă că echipamentul SIUI este online: dacă acesta lipsește,
rezultatul este success:false, code:bridge_offline. Conectează utilitarul la WSM
și poți apela din nou singletonul.

Dacă objWSM nu există, configurează și wsmFactory pentru a-l crea cu URL-ul și
tokenProvider ale aplicației. Configurarea este opțională, doar pentru integrare avansată; prima operație deschide conexiunea. Configure returnează {success:true} sau {success:false,error:...}.
Fără configurare se detectează automat window.objWSM sau variabila globală objWSM. Variabilele globale let/const objWSM sunt detectate automat.

Aceleași opțiuni file, callwmapi, lt și validateXSD se aplică singletonului.
Pentru cele trei funcții SIUIWorkflows de mai jos poți transmite objWSMSIUI;
rezultatul lor trebuie verificat prin success și apoi accesat prin data.
Metodele SIUIBridge din documentația legacy următoare păstrează comportamentul
cu excepții pentru compatibilitate; folosește objWSMSIUI pentru noul contract.

O ghilimea lipsă produce SyntaxError înainte de apelarea funcției și nu poate
fi capturată de wrapper. În JavaScript exit nu este instrucțiune de ieșire;
folosește if/else sau return în interiorul unei funcții.

## Ajustări opționale, numai în pagina curentă

Fără niciun apel de configurare se folosesc valorile implicite. La nevoie:

~~~js
objWSMSIUI.setTimeouts({
  connectTimeout: 4000,
  requestTimeout: 135000,
  waitTimeout: 180000,
  downloadTimeout: 180000,
  totalTimeout: 300000
});
objWSMSIUI.setReconnectOptions({connectAttempts:3, connectTimeout:4000});
objWSMSIUI.setPollingInterval(1000);
objWSMSIUI.selectBridge({equipmentID:'18'});

// Opțional: folosește alt obiect WSM în această pagină.
objWSMSIUI.setWSM(altObjWSM);
objWSMSIUI.selectBridge({equipmentID:'19'});

// Revino la objWSM detectat automat / selecție automată.
objWSMSIUI.useAutomaticWSM();
objWSMSIUI.selectBridge({});

// Consultă sau resetează ajustările locale.
const options = objWSMSIUI.getOptions(); // {success:true,data:{...}}
objWSMSIUI.resetOptions();
~~~

Toate duratele sunt milisecunde pozitive; connectAttempts este întreg pozitiv.
Metodele sunt sincrone și întorc success:true sau success:false/error (string).
Opțiunile invalide nu modifică setările existente. Modificările se aplică apelurilor
următoare; cele deja pornite își păstrează configurația. Nu se scrie în localStorage,
configurația bridge-ului sau pe server și nu se deconectează obiectele WSM.
setWSM/useAutomaticWSM șterg selecția echipamentului anterior; selectBridge înlocuiește
toți selectorii. getOptions nu returnează obiectul WSM, credențiale sau funcții.
resetOptions restaurează valorile implicite și detectarea automată a objWSM.

## API legacy SIUIBridge — compatibilitate, cu excepții

Secțiunile legacy de mai jos folosesc alte forme de răspuns. Pentru cod nou,
folosește objWSMSIUI și exemplele din ghidul practic.

Încarcă cele două fișiere servite de WSM:

```html
<script src="https://wslocal.wisemed.eu/sdk/wsm.js"></script>
<script src="https://wslocal.wisemed.eu/sdk/siui-bridge.js"></script>
```

Wrapperul reutilizează obiectul WSM existent sau îl creează prin wsmFactory. Nu cere
JWT local SIUI și nu execută fetch/HTTP către SIUI Bridge. Toate cererile,
inclusiv descoperirea, polling-ul și descărcările, folosesc socketul lui objWSM.

```js
const siui = await SIUIBridge.create(objWSM, {equipmentID:'18'});
if (siui === null) {
  afiseazaMesaj('SIUI Bridge nu este conectat.');
  return;
}
const status = await siui.getStatus();
```

`equipmentID`, `clientID`, `connectionID` sunt selectoare opționale. Fără selector
este acceptat numai un singur bridge online; mai multe produc `ambiguous_bridge`.
Fără clienți potriviți, `create()` returnează null. Conexiunea WSM este refăcută cu un număr limitat de încercări; epuizarea lor
respinge Promise-ul. Alternativ: `const siui = SIUIBridge(objWSM, options)`
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

## Cele 40 de operații CNAS

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
| `general.generatePrescriptionSeries` | `categoryCode`, `orgUnitCode`, `uic`, `subUnitCode`, `validFrom`, `isOnLine`, `quantity` | Da |
| `general.getPrescriptionSeriesInfo` | `categoryCode`, `orgUnitCode`, `uic`, `subUnitCode` | Nu |
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
| `clinice.processIssuedPrescription` | `reportXml` | Da |
| `clinice.cancelReleasedPrescription` | `providerCode`, `workplaceCode`, `insuranceHouseCode`, `series`, `no`, `fractionNo`, `cancellationReason` | Da |
| `clinice.updateInvoices` | `requestXml` | Da |
| `clinice.getReleasedPrescription` | `providerCode`, `workplaceCode`, `insuranceHouseCode`, `series`, `no`, `fractionNo` | Nu |
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

`operationId` este obligatoriu pentru cele 40 de operații. Generează-l o singură
dată pentru o intenție de lucru și păstrează-l pentru reconcilierea acelei intenții.
Nu genera un ID nou la fiecare retry al unei operații cu efecte.

Metodele de operație trimit POST o singură dată, apoi interoghează GET calls/{id}
până la finalizare. Default: polling la 1s, timeout de așteptare 180s după
submit. Configurabile prin `pollInterval`, `timeout`; timeoutul fiecărei cereri
este separat (`requestTimeout`). Nu există retry automat de POST.

- `completed`: descarcă automat fișierul dacă există, aplică procesarea cerută și întoarce call cu download și, opțional, processed. Interpretează separat rezultatul CNAS.
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

## Reconectare automată cu limite

Nu pune await objWSM.ready înainte de wrapper. Fiecare cerere verifică conexiunea;
dacă este căzută, apelează reconnect() (sau connect()) de maximum 3 ori, cu
timeout de 5000 ms pentru fiecare încercare. Un ready vechi/blocat nu este așteptat
nelimitat. Încercările concurente pe același obiect împart reconectarea.
Eroarea finală are code=wsm_disconnected, attempts și cause (de exemplu connect_timeout).

~~~js
const siui = await SIUIBridge.create(objWSM, {
  equipmentID: '18',
  connectAttempts: 3,
  connectTimeout: 4000,
  requestTimeout: 135000,
  downloadTimeout: 180000,
  // Folosit dacă objWSM lipsește sau nu este un obiect WSM valid:
  wsmFactory: async () => {
    objWSM = WSMServer(wsmURL, '', {tokenProvider: obtineJWTWSM});
    return objWSM; // nu aștepta ready aici
  }
});
~~~

wsmFactory este obligatoriu dacă obiectul WSM lipsește; wrapperul nu poate inventa
URL-ul sau acreditările. Pe obiectul existent tokenProvider furnizează JWT nou la
reconectare. connectAttempts/connectTimeout pot fi suprascrise per apel.
AbortSignal oprește așteptarea apelantului; reconectarea partajată poate continua
pentru ceilalți apelanți. După un POST trimis nu există retransmitere automată.

## Flux complet: execuție, fișier, procesare

~~~js
const call = await siui.getProviderInfo({
  partnerCategory: 'CLIN',
  start: '2026-10-01T00:00:00',
  stop: '2026-10-31T23:59:59',
  uic: codFiscal
}, {
  operationId: idStabil,
  file: {unzip:true, entry:'personalizare.xml', format:'json'},
  downloadTimeout: 180000,
  onDownloadProgress: p => afiseazaOcteti(p.receivedBytes),
  processResult: async call => transformaPersonalizarea(call.download.data)
});
// entry trebuie să fie numele real din ZIP; omite-l dacă există un singur fișier.
const arboreXML = call.download.data;
const dateAplicatie = call.processed;
~~~

Metodele numite și executeOperation așteaptă implicit descărcarea și processResult.
download.bytes este Uint8Array, download.data este Uint8Array pentru bytes,
string pentru xml sau obiect pentru json. download.source descrie fișierul original.
Fără fișier, call rămâne rezultatul normal; processResult se poate aplica și lui.
wait:false întoarce doar confirmarea inițială; download:false oprește descărcarea.
waitCall întoarce starea CNAS; pentru reluarea fluxului folosește
await siui.completeCall(await siui.waitCall(id), options).
Erorile fluxului combinat păstrează callId și operationId.

Opțiunile file (sau aceleași câmpuri direct la downloadCallFile):

| Opțiune | Comportament |
| --- | --- |
| unzip | Implicit false: arhiva rămâne intactă. true: serverul extrage în memorie un singur fișier. |
| entry | Numele exact din ZIP; obligatoriu când există mai multe fișiere. |
| format | bytes (implicit), xml (verifică XML și întoarce text), json (arbore XML generic). |
| validateXSD | Implicit true; false dezactivează numai XSD. Schema este aleasă exclusiv de server. |
| maxBytes | Limită locală de descărcare, implicit 64 MiB. |

Ordinea pe server: unzip → XSD implicit activ → XML/JSON → fragmente WSM.
Serverul alege XSD-ul din operație și categoria raportării; clientul nu trimite schema.
Dacă schema nu este disponibilă/neambiguă, validarea produce eroare. PDF-ul nu devine XML.
XSD folosește motorul existent, limita sa de 1 MiB și necesită validatorul platformei.
Procesarea XML/ZIP are limită 64 MiB, adâncime XML 40 și interzice DTD.
Nicio intrare ZIP nu este scrisă ca fișier pe disc.
La JSON, nodurile au name, namespace, attributes (name/namespace/value) și content,
o listă ordonată de texte și noduri. Repetițiile și valorile numerice rămân intacte
ca șiruri; nu se presupune o structură medicală comună tuturor operațiilor.
processResult permite transformarea în modelul aplicației și este așteptat cu
processingTimeout (implicit waitTimeout). Codul sincron lung nu poate fi întrerupt
de un timer JavaScript; pentru el folosește un Web Worker.

## Actualizarea WiseMED din server: callwmapi

~~~js
const call = await siui.getInsured({pid:cnp, requestDate:dataCerere}, {
  operationId:idStabil,
  callwmapi:{
    lt:loginTokenWiseMED, // opțional; nu este JWT-ul WSM
    apipath:'/pacient/{cnp_pacient}',
    method:'POST',
    transform_path:[{from_field:'cnp',to_field:'cnp_pacient'}],
    body:{
      verificare_la_data:dataVerificarii,
      transform_result:[
        {from_field:'cid',to_field:'cid_pacient'},
        {from_field:'state',to_field:'stare_asigurare_cas'}
      ]
    }
  }
});
~~~

Wrapperul transmite callwmapi către SIUI Bridge. După salvarea rezultatului CNAS,
bridge-ul face apelul WiseMED independent de browser, inclusiv după închiderea lui
sau repornirea bridge-ului. lt este opțional, specific cererii și trimis în claim-ul
lt al JWT-ului WiseMED, fără login preliminar. Fără lt se încearcă apelul fără token
de utilizator; nu se folosește sesiunea globală a ultimului utilizator.
Utilizatorii obișnuiți autentificați pot solicita callback-ul. WiseMED verifică
drepturile; restricțiile operației CNAS și izolarea pe proprietarul apelului rămân.

call.status=completed și returnarea wrapperului nu confirmă livrarea callback-ului.
Citește (await siui.getCall(call.id)).call.callwmapi pentru status, attempts,
http_status și error: waiting, pending, sending, delivered, failed sau skipped.
401/403 opresc livrarea fără reîncercare automată. Erorile de transport, 429 și 5xx
au maximum 5 încercări, cu întârzieri 30/60/120/240 secunde.
await siui.retryCallWMAPI(call.id) reia doar callback-ul eșuat cu body pregătit,
păstrând tokenul și Idempotency-Key; nu repetă CNAS.

WiseMED trebuie să deduplice atomic Idempotency-Key: siui-<call.id>.
Tokenul este criptat în coada persistentă și nu apare în status/loguri.
Schimbarea callwmapi (inclusiv lt) pentru același operationId produce 409.
Transformările se aplică rezultatului CNAS salvat și opțional file_json pregătit
prin callwmapi.file pe server; nu folosesc processResult din browser. Contractul complet:
[Callback SIUI Bridge](../../readersv3/apps/siui-bridge/docs/API.md#callback-persistent-către-wisemed-callwmapi).

## Trei exemple complete pentru aplicația WiseMED

Codul reutilizabil este în
[examples/browser/siui-workflows.js](../examples/browser/siui-workflows.js).
Publică-l din aplicația ta și încarcă-l după wsm.js și siui-bridge.js (nu este un
endpoint /sdk suplimentar). Funcțiile se pot copia și direct în aplicație.

Include siui-bridge.js și siui-workflows.js în pagina care are objWSM.
Singletonul objWSMSIUI se inițializează automat; nu apela create/configure.
Exemplele următoare sunt în interiorul unei funcții async (return oprește funcția).

ID-urile operationId de mai jos trebuie generate și păstrate o singură dată per
intenție de lucru de aplicația WiseMED. Refolosește exact aceleași date la retry,
inclusiv datele calendaristice fixe din callback; altfel bridge-ul răspunde 409.
loginTokenWiseMED provine din autentificarea WiseMED; nu îl pune în loguri sau
localStorage. Endpointurile WiseMED sunt exemple de contract, nu sunt create de
acest repository.

### 1. Calitatea de asigurat în browser și update automat pacient

~~~js
const response = await SIUIWorkflows.checkInsurance(objWSMSIUI, {
  cnp:cnpPacient,
  requestDate:dataVerificariiISO,
  lt:loginTokenWiseMED,
  operationId:idOperatieAsigurat
});
if (!response.success) {
  afiseazaEroare(response.error);
  return;
}
const call = response.data;
const persoana = call.result.insured;
// persoana.pid/cid sunt șiruri, state este șirul SIUI ("0", "1", "3"),
// categories și states sunt liste complete.
afiseazaCalitateaAsiguratului(persoana);
~~~

După răspuns, bridge-ul trimite POST /pacient/<cnp> cu verificare_la_data,
cnp_pacient, cid_pacient, stare_asigurare_cas (număr), categorii_cas și stari_cas.
CNAS state=0 înseamnă neasigurat, 1 asigurat, 3 decedat; nu trata orice valoare
nenulă drept asigurat. Dacă XML-ul nu poate fi interpretat, callback-ul eșuează,
fără update parțial inventat. Livrarea continuă dacă browserul se închide.

### 2. Personalizare în browser și POST /cnas/personalisation

~~~js
const response = await SIUIWorkflows.downloadPersonalisation(objWSMSIUI, {
  partnerCategory:'CLIN',
  start:'2026-10-01T00:00:00',
  stop:'2026-10-31T23:59:59',
  uic:codFiscal,
  // entry:'numele-real-din-arhiva.xml', // obligatoriu doar pentru ZIP cu mai multe fișiere
  // validateXSD:false, // doar când XSD-ul CNAS este mai nou decât cel din bridge
  lt:loginTokenWiseMED,
  operationId:idOperatiePersonalizare
});
if (!response.success) {
  afiseazaEroare(response.error);
  return;
}
const call = response.data;
const personalizareJSON = call.download.data;
afiseazaPersonalizarea(personalizareJSON);
~~~

callwmapi.file cere bridge-ului să dezarhiveze și să convertească XML în JSON
înainte de pregătirea livrării. transform_result copiază file_json în
body.personalisation. Body-ul efectiv către endpointul virtual este de forma:
~~~json
{
  "partnerCategory":"CLIN",
  "start":"2026-10-01T00:00:00",
  "stop":"2026-10-31T23:59:59",
  "uic":"123456",
  "personalisation":{
    "name":"provider",
    "namespace":"http://www.cnas.ro/siui/2.0",
    "attributes":[{"name":"code","value":"000123"}],
    "content":[{"name":"service","attributes":[{"name":"code","value":"A"}]}]
  }
}
~~~
Arborele de mai sus este ilustrativ; numele și conținutul provin integral din XML-ul
CNAS real. Repetițiile și atributele nu se pierd și CNP/CID/codurile nu devin numere.
Body-ul final este persistat pentru retry. Această procesare nu depinde de
processResult din browser. Dacă răspunsul nu conține fișier sau ZIP-ul este ambiguu,
callback-ul devine failed și nu trimite date incomplete.

### 3. Validare 72h și update /file/{file_id}/

~~~js
const response = await SIUIWorkflows.validate72h(objWSMSIUI, {
  fileId:idFisa,
  reportXml:xmlGeneratDeWiseMED,
  reportType:'PARA',
  requestType:'RQ_PARA_SRV',
  requestedAt:dataValidariiISO, // păstrată fixă pentru acest operationId
  lt:loginTokenWiseMED,
  operationId:idOperatieValidare
});
if (!response.success) {
  afiseazaEroare(response.error);
  return;
}
const call = response.data;
afiseazaRaspunsulValidarii(call.result.values[0]);
~~~

Exemplul folosește contractul de validare PARA și necesită dreptul administrativ
al operației CNAS. Pentru CLIN furnizează reportType și requestType potrivite
contractului/documentului clinic. Nu se generează XML fictiv de raportare.
Bridge-ul trimite POST /file/<fileId>/ cu:
~~~json
{
  "file_id":"123",
  "report_type":"PARA",
  "requested_at":"2026-10-18T12:00:00.000Z",
  "cnas_validation_response_xml":"<raspunsul XML real returnat de CNAS>"
}
~~~
Backendul WiseMED trebuie să parseze răspunsul și să coreleze AppId cu serviciile
salvate ale fișei. completed înseamnă răspuns primit, nu validare fără erori.
Nu se setează automat un flag validat=true. fileId este metadata WiseMED,
inclus direct în apipath și body; nu este trimis ca parametru WSDL.

### Verificarea livrării pentru toate cele trei exemple

~~~js
const latest = await objWSMSIUI.getCall(call.id);
if (!latest.success) { afiseazaEroare(latest.error); return; }
const delivery = latest.data.call.callwmapi;
afiseazaStareaActualizariiWiseMED(delivery);
// Numai după tratarea cauzei unei livrări failed, cu body deja pregătit:
await objWSMSIUI.retryCallWMAPI(call.id);
~~~
Nu apela retry automat la fiecare interogare. delivered înseamnă HTTP 2xx; WiseMED
trebuie să întoarcă un cod non-2xx la refuz și să deduplice Idempotency-Key.


### Alegerea XSD pe server și compatibilitatea cu actualizările CNAS

Clientul nu transmite numele XSD. Procesarea XML validează implicit cu schema
selectată de bridge din operația salvată și parametrii ei (ex. partnerCategory).
Pentru personalizare sunt incluse definițiile MF, CLIN, PARA, STOM, AMB, MD, HC,
SPT, NHP, DIA, SICK, CBRET. PHM și REC au variante diferite: fără o identificare
neambiguă serverul refuză validarea, nu ghicește. Fișierele vechi fără contextul
schemei produc aceeași eroare. Descărcarea originalului ZIP/PDF fără procesare
rămâne neschimbată.

Dacă CNAS actualizează XML/XSD înaintea bridge-ului, se poate cere explicit:
~~~js
file: {unzip:true, format:'json', validateXSD:false}
~~~
Pentru callback: callwmapi.file:{unzip:true,validateXSD:false}.
False dezactivează doar verificarea XSD; XML-ul malformat, DTD și limitele de
procesare rămân verificate. Nu există fallback automat după o eroare XSD.
Această opțiune privește procesarea fișierelor de răspuns, nu dezactivează
validările existente ale XML-ului de raportare trimis către CNAS.

## Status și progress bar

Înregistrează opțional un callback pentru toate apelurile din pagină:
~~~html
<span id="siui-status"></span>
<progress id="siui-progress" max="100"></progress>
~~~
~~~js
objWSMSIUI.setStatusHandler(event => {
  document.getElementById('siui-status').textContent = event.message;
  const bar = document.getElementById('siui-progress');
  if (event.percent === null) bar.removeAttribute('value'); // nedeterminat
  else bar.value = event.percent;
});
// Eliminare:
objWSMSIUI.setStatusHandler(null);
~~~

Sau numai pentru o operație:
~~~js
const result = await objWSMSIUI.general.getInsured(parameters, {
  operationId: idStabil,
  onStatus: event => actualizeazaStatus(event)
});
~~~

Callback-urile global și per apel primesc aceleași evenimente. Câmpuri:
requestId (ID local unic în instanță), operation, operationId dacă există,
stage, message, elapsedMs, percent (0–100 sau null). Suplimentar pot exista
attempt/maxAttempts, callId, status, receivedBytes/totalBytes sau code.
Pentru mai multe apeluri simultane, păstrează câte un indicator per requestId.

Etape: starting, connecting, discovering, requesting, waiting, downloading,
processing, completed, error, aborted. Descoperirea/cererile pot reapărea la
polling sau la fiecare fragment. percent reprezintă progresul etapei, nu o estimare
a duratei întregii operații. La CNAS și la procesarea server-side nu există procent
disponibil: folosește un indicator nedeterminat. Pentru download-ul original,
procentul se calculează din octeți; pentru fișierul transformat totalul poate lipsi,
iar procentul devine 100 numai la EOF.

completed/100 confirmă încheierea apelului wrapperului; cu wait:false confirmă
numai submit-ul. Nu confirmă validarea medicală fără erori sau livrarea callwmapi.
Nu mai sunt emise evenimente după completed/error/aborted pentru acel apel.
Callback-urile onStatus nu sunt așteptate; excepțiile/rejecțiile lor sunt izolate
și nu schimbă rezultatul operației. Nu executa procesare sincronă lungă în UI.
onProgress și onDownloadProgress existente rămân disponibile separat.


### Selecția implicită a singletonului
objWSMSIUI alege primul client siui-bridge conectat din lista returnată de WSM,
fără configurare obligatorie. Ordinea listei nu reprezintă o prioritate garantată.
După selecție păstrează identitatea echipamentului: dacă acesta dispare, returnează
bridge_offline, fără să mute automat apelurile pe alt echipament.
selectBridge({}) reface selecția automată la următorul apel; selectBridge({equipmentID:'18'})
alege explicit. Selectorii expliciți ambigui rămân erori. API-ul legacy SIUIBridge
păstrează comportamentul strict cu ambiguous_bridge când nu există selector.

## Prescripții: carnete, prescriere și eliberare

Catalogul include toate cele 13 operații din `EPrescriptionWS`, cele două operații de carnete din `SiuiWS` și `getPrescription` / `validatePrescription` din `SiuiValidateWS`. Metodele adăugate:

| Metodă singleton | Parametri WSDL, în ordine | Modifică CNAS |
| --- | --- | --- |
| `general.generatePrescriptionSeries` | `categoryCode` (string), `orgUnitCode` (string), `uic` (string), `subUnitCode` (string), `validFrom` (dateTime), `isOnLine` (boolean), `quantity` (int) | Da |
| `general.getPrescriptionSeriesInfo` | `categoryCode` (string), `orgUnitCode` (string), `uic` (string), `subUnitCode` (string) | Nu |
| `clinice.processIssuedPrescription` | `reportXml` (string) | Da |
| `clinice.cancelReleasedPrescription` | `providerCode` (string), `workplaceCode` (string), `insuranceHouseCode` (string), `series` (string), `no` (string), `fractionNo` (int), `cancellationReason` (string) | Da |
| `clinice.updateInvoices` | `requestXml` (string) | Da |
| `clinice.getReleasedPrescription` | `providerCode` (string), `workplaceCode` (string), `insuranceHouseCode` (string), `series` (string), `no` (string), `fractionNo` (int) | Nu |

Toți parametrii se trimit ca șiruri JSON, cu excepția parametrilor declarați nullable. Pentru cele două operații de carnete, `subUnitCode` acceptă și JSON `null`, transmis SOAP ca `xsi:nil="true"`. Acesta este cazul unității cu personalitate juridică și contract direct cu CAS. Pentru o subunitate, se transmite `partnerCode` din personalizare. Nu folosi textul `"null"`. Un câmp nullable gol în Debug este trimis ca null; API-ul distinge între null și șirul gol. `isOnLine` acceptă exact `"true"`, `"false"`, `"1"`, `"0"`; `quantity` și `fractionNo` sunt șiruri de întregi pe 32 de biți. API-ul carnetelor rămâne în GENERAL; în Debug, preluarea și generarea carnetelor apar și în CLINICE. Nu există parametru `stencilNo` la aceste două operații: `categoryCode`, `orgUnitCode`, `uic` și `subUnitCode` trebuie completate conform identității contractuale CNAS, fără a presupune că `subUnitCode` este parafa medicului.

`getPrescriptionSeriesInfo` interoghează carnetele existente la CNAS pentru parametrii furnizați, inclusiv cele generate prin altă aplicație dacă sunt accesibile aceleiași identități. WSDL-ul nu garantează că aceasta este lista tuturor carnetelor unui medic, indiferent de furnizor/contract. `generatePrescriptionSeries` generează carnete cu `validFrom`, `isOnLine` și `quantity`. Conform specificației PIAS §5.20, cele două valori din `result.data.result.values` sunt URL-ul fișierului de răspuns și dimensiunea lui. Backend-ul descarcă fișierul de pe aceeași origine HTTPS CNAS; singletonul returnează implicit conținutul ca `result.data.download.data` / `bytes` (Uint8Array), cu metadate în `result.data.result.file`. `download:false` oprește transferul browserului, fără a anula descărcarea backend-ului. Conținutul fișierului trebuie interpretat pentru a extrage carnetele; valorile SOAP nu sunt lista de carnete.

- Prescriere/validare PE: `clinice.processPrescribedPrescription({reportXml}, options)`, cu verificare locală `PhysicianDrugPERequest.xsd` înainte de trimitere.
- Preluare/anulare prescriere: `clinice.getPrescribedPrescription` / `clinice.cancelPrescribedPrescription`.
- Eliberare la farmacie: `clinice.processIssuedPrescription`, `clinice.getReleasedPrescription`, `clinice.cancelReleasedPrescription`. „Issued/released” desemnează eliberarea, nu prescrierea de către medic.
- Spitalizare: `clinice.processHospitalPrescription`, `clinice.getReleasedHospitalPrescription`, `clinice.cancelReleasedHospitalPrescription`.
- Consultare/rapoarte: `clinice.getStatusForPrescriptions`, `clinice.getPrescribedPrescriptionsForCid`, `clinice.downloadNotReportedPrescriptionsReport`; `clinice.updateInvoices` actualizează facturile în serviciul PE.
- Prescripții SIUI clasice: `clinice.getPrescription` și `clinice.validatePrescription` (distincte de PE).

### Online și offline

`isOnLine:"false"` generează un carnet pentru fluxul offline; apelul de generare în sine necesită conexiune CNAS. În schema PE inclusă, atributul rețetei `reportedOnline="0"` înseamnă prescriere offline de medic și validare online de farmacie; `reportedOnline="1"` înseamnă prescriere și validare online de medic. Schema cererii permite și `senderType="M"` / `"F"`, precum și `isCancelled` pe rețetă. Aceste câmpuri nu înlocuiesc operațiile SOAP de anulare și nu garantează acceptarea de CNAS a oricărei combinații.

Emiterea fără conexiune, alegerea și consumul unui număr din carnet, stocarea rețetei, tipărirea și sincronizarea ulterioară sunt responsabilitatea aplicației WiseMED. Bridge-ul nu implementează registrul local de carnete și nu rezervă automat numere pentru mai multe stații. Nu există validare de business CNAS fără conexiune și nici retrimitere automată a rețetelor la reconectare.

`processIssuedPrescription` și `updateInvoices` verifică XML-ul sintactic; în resursele incluse nu există XSD-uri de cerere pentru aceste două operații. Nu se aplică schema medicului la eliberarea farmaciei. Regulile de business se evaluează de CNAS. `success:true` / `completed` confirmă primirea răspunsului, nu acceptarea rețetei/carnetului; verifică valorile/XML-ul CNAS.

Folosește un `operationId` stabil pentru fiecare intenție de modificare. După timeout sau `unknown`, urmărește apelul existent și reconciliază prin operațiile de preluare înainte de a iniția altă modificare. Nu genera un ID nou doar pentru a repeta generarea carnetului.

Exemplele cu spinner, callback și `$.aceOverWatch` sunt în `examples/browser/siui-prescriptions.js` din pachetul WSM (în repository: `wsm-server/examples/browser/siui-prescriptions.js`).

```js
const parameters = {categoryCode: "...", orgUnitCode: "...", uic: "...", subUnitCode: null};
await SIUIPrescriptions.getPrescriptionSeriesSIUI(target, parameters,
    function(responseReceived, values, errorMsg, call) {
        if (!responseReceived) return;
        console.log(values); // interpretează răspunsul real CNAS
    });

// Creează și păstrează împreună acest ID și acești parametri pentru aceeași intenție.
const generationId = crypto.randomUUID();
const generationParameters = {...parameters,
    validFrom: moment().format("YYYY-MM-DDTHH:mm:ssZ"),
    isOnLine: "false", quantity: "1"
};
await SIUIPrescriptions.generatePrescriptionSeriesSIUI(target, generationParameters,
    generationId, function(responseReceived, values, errorMsg, call) {
        if (!responseReceived) return;
        console.log(values); // verifică acceptarea CNAS înainte de mesajul de succes
    });
```
