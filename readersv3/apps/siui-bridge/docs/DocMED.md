# DocMED — API separată în SIUI Bridge

Implementarea acoperă cele 22 de operații din cele patru WSDL-uri CNAS DocMED 1.0 (20.10.2026). Originile SIUI/SIPE și metodele existente rămân separate. Nu se generează serii în ambele sisteme automat și nu se retrimit automat scrierile după erori ambigue.

## Configurare și autentificare

`modules.siui.docmed_base_url: https://docmed.e-pias.ro` este separată de `base_url` (SIUI/OCSP), `pe_base_url` și `einvoice_base_url`. Se salvează per certificat în Setări → CNAS. DocMED folosește același certificat/token și aceleași credențiale SIUI; autentificarea OCSP se face la `base_url + /OCSP/validator`. Rutele sunt disponibile local cu sesiune/JWT și prin WSM, cu aceleași reguli de proprietar/tenant și administrator ca restul bridge-ului.

## Contractele disponibile

Toate rutele de mai jos sunt **POST `/api/siui/docmed/<operație>`**. Interfața Debug → DOCMED oferă formulare pentru toate operațiile, fără editare XML. Corpul JSON trimis de formular are `operation_id` și `fields`, cu variabilele completate. Bridge-ul construiește XML-ul în namespace-ul `http://www.cnas.ro/pias/2.0`, îl validează local și adaugă anvelopa SOAP document/literal. API-ul avansat existent cu `parameters.requestXml` rămâne compatibil; nu se combină `fields` și `parameters` în aceeași cerere.

| Operație | Serviciu / cale SOAP | Acces |
| --- | --- | --- |
| `MedicalLeave` | `DocMedService` — `/loader/ws` | Administrator; scriere |
| `MedicalLetter` | `DocMedService` — `/loader/ws` | Administrator; scriere |
| `PhysicianElectronicPrescription` | `DocMedService` — `/loader/ws` | Administrator; scriere |
| `RecoveryMedicalPlan` | `DocMedService` — `/loader/ws` | Administrator; scriere |
| `LaboratoryMFReferral` | `DocMedService` — `/loader/ws` | Administrator; scriere |
| `LaboratoryAGReferral` | `DocMedService` — `/loader/ws` | Administrator; scriere |
| `LaboratoryCTReferral` | `DocMedService` — `/loader/ws` | Administrator; scriere |
| `LaboratorySGReferral` | `DocMedService` — `/loader/ws` | Administrator; scriere |
| `PalliativeRecommendation` | `DocMedService` — `/loader/ws` | Administrator; scriere |
| `MedicalDeviceRecommendation` | `DocMedService` — `/loader/ws` | Administrator; scriere |
| `HomeCareMedicalPlan` | `DocMedService` — `/loader/ws` | Administrator; scriere |
| `ClinicalReferral` | `DocMedService` — `/loader/ws` | Administrator; scriere |
| `LaboratoryRMNReferral` | `DocMedService` — `/loader/ws` | Administrator; scriere |
| `LaboratoryReferral` | `DocMedService` — `/loader/ws` | Administrator; scriere |
| `HomeCareRecommendation` | `DocMedService` — `/loader/ws` | Administrator; scriere |
| `ConsumeDocument` | `DocStatusChangerService` — `/status/ws` | Administrator; scriere |
| `UnconsumeDocument` | `DocStatusChangerService` — `/status/ws` | Administrator; scriere |
| `GetDocumentStatus` | `DocStatusChangerService` — `/status/ws` | Citire; utilizator autentificat |
| `GenDocumentSeries` | `DocumentSeries` — `/seq/ws` | Administrator; scriere |
| `GetDocumentSeries` | `DocumentSeries` — `/seq/ws` | Citire; utilizator autentificat |
| `GetMedicalDocumentXml` | `MedicalDocumentDownloader` — `/download/ws` | Citire; utilizator autentificat |
| `GetMedicalDocument` | `MedicalDocumentDownloader` — `/download/ws` | Citire; utilizator autentificat |

Catalogul complet este disponibil în `GET /api/siui/catalog`, grupul `docmed`; conține elementele Request/Response, fișierele XSD, calea SOAP, modelul `form` și un `request_example` sintetic pentru clienții avansați. `docmed_defaults` precompletează codul fiscal și CAS-ul configurat. OpenAPI include toate cele 63 de operații: 41 SIUI/SIPE și 22 DocMED. Exemplele nu reprezintă documente reale și nu se trimit fără înlocuirea datelor cu cele corespunzătoare operației dorite.

## Verificarea unui document vechi fără modificarea lui

1. Repornește executabilul actualizat și reîncarcă pagina.
2. În **Debug → DOCMED**, metoda inițială este `GetMedicalDocumentXml`.
3. Completează câmpurile furnizorului și cele patru date ale documentului: `personCID`, `issueDate`, `serialCode`, `serialNo`. Folosește un document vechi cunoscut.
4. Execută cererea cu un ID nou și urmărește execuția. Răspunsul XML și rezultatul structurat sunt disponibile fără a consuma documentul.
5. Dacă dorești PDF-ul, folosește separat `GetMedicalDocument`, cu aceleași date de identificare. Răspunsul XML și PDF-ul arhivat ZIP au butoane de descărcare distincte.

Nu există o metodă DocMED de listare a tuturor documentelor pacientului/furnizorului. `GetMedicalDocumentXml` și `GetMedicalDocument` cer identificatorii documentului. `GetDocumentStatus` permite CID opțional și cere serie, număr și dată. `GetDocumentSeries` listează serii de formulare, nu documentele medicale generate.

Un document vechi găsit confirmă disponibilitatea acelui document în DocMED. `DOC_NOT_FOUND` nu dovedește absența tuturor documentelor vechi; verifică identificatorii, tipul, drepturile și mediul. `DOC_NOT_FINALIZED` și `DOC_CONSUMED` se tratează distinct. Compară tipul și identificatorii documentului returnat cu cei așteptați înainte de importul datelor în aplicația clinică.

### Exemplu de corp JSON pentru citire

Datele sunt exclusiv sintetice; înlocuiește-le înainte de o cerere reală.

```json
{
  "operation_id": "docmed-read-old-001",
  "fields": {
    "providerCode": "123",
    "providerName": "FURNIZOR EXEMPLU",
    "providerInsuranceHouse": "CAS-DJ",
    "MedicalDocument": {
      "personCID": "00000000000000000000",
      "issueDate": "2026-11-02",
      "serialCode": "SERIE",
      "serialNo": "1"
    }
  }
}
```

HTTP 202 întoarce `call.id`; citește apoi `GET /api/siui/calls/{id}`. Folosirea aceluiași ID cu aceiași parametri urmărește execuția existentă; schimbarea conținutului cu același ID produce 409. Un test independent folosește un ID nou.

### SDK browser / WSM

```js
// objWSMSIUI utilizează conexiunea objWSM a aplicației.
const response = await objWSMSIUI.docmed.GetMedicalDocumentXml(
  { requestXml },
  { operationId: crypto.randomUUID(), file: { format: 'json' } }
);
// response.success=false: inspectează response.error / execuția indicată.
// response.success=true: response.data.result.docmed și response.data.download.data.

// Pentru o execuție GetMedicalDocument reușită:
const zip = await objWSMSIUI.downloadDocMedAttachment(callId);
// zip.success=true -> zip.data conține Uint8Array cu arhiva ZIP.
```

SDK-ul direct obținut prin `SIUIBridge.create(...)` oferă aceleași metode în `bridge.docmed`. `bridge.downloadDocMedAttachment(callId)` descarcă ZIP-ul în fragmente fără o nouă cerere către CNAS. Opțiunile `unzip:true`, `entry:'nume.pdf'`, `validateXSD:false` pot selecta un PDF din arhivă; numele intrării se ia din ZIP, nu se presupune.

## Răspunsuri și fișiere

- `result.docmed.attributes` păstrează atributele răspunsului ca șiruri, inclusiv `DocID`, `documentType`, `status` unde există.
- `result.docmed.messages` păstrează până la 100 de mesaje (cod, nivel, text); `message_count` indică numărul total. XML-ul descărcabil păstrează toate mesajele.
- `result.docmed.data` include reprezentarea JSON a XML-ului pentru răspunsuri mici; pentru rezultate mari se descarcă fișierul.
- `result.file` este XML-ul răspunsului DocMED, cu namespace-urile necesare pentru utilizare independentă de anvelopa SOAP. Descărcare în fragmente prin `/api/siui/calls/{id}/file?offset=0`; opțiunile existente `format=xml|json`, `validateXSD` funcționează cu schema DocMED selectată de server.
- Pentru `GetMedicalDocument`, `result.docmed.attachment` descrie arhiva ZIP. Calea `/api/siui/calls/{id}/file?docmed_attachment=true&offset=0` extrage atașamentul din XML-ul păstrat local, fără apel CNAS. Păstrează parametrul la fiecare fragment. După eliminarea copiei locale, răspunsul original arhivat în History conține încă atașamentul Base64; proiecția locală ZIP necesită restaurarea/prelucrarea originalului.

Fișierele și execuțiile sunt accesibile numai proprietarului autorizat; identificatorii medicali rămân șiruri. Cererile și răspunsurile nu se scriu în logurile operaționale. Rezultatele și fișierele folosesc mecanismele existente de istoric și callback WiseMED.

## Reguli de stare și erori

- `GenDocumentSeries` / `GetDocumentSeries`: mesajele E blochează preluarea seriilor; W, I și cele fără nivel sunt păstrate fără să blocheze.
- Restul operațiilor: orice mesaj în `errors` înseamnă operație eșuată. `status=failed` păstrează mesajele și XML-ul răspunsului, inclusiv codul `DOC_NOT_FOUND`. Nu se tratează automat ca succes doar pentru că HTTP a fost 200.
- Timeout, SOAP fault sau răspuns invalid după o scriere pot produce `unknown`; nu se retrimite automat.
- Înainte de `ConsumeDocument`, bridge-ul verifică `GetDocumentStatus` și cere FINALIZED. Înainte de `UnconsumeDocument` cere CONSUMED. Verificarea nu înlocuiește controlul concurenței efectuat de CNAS.
- Documentele se transmit/modifică prin metoda lor dedicată. Ștergerea, tipărirea/finalizarea și anularea folosesc atributele `deleted`, `printed`, `canceled` ale aceluiași document, conform specificației. Finalizarea include `attachedFile` cu PDF arhivat ZIP. Aplicația păstrează `DocID` primit și gestionează local starea; bridge-ul nu fabrică serii/DocID și nu execută aceste scrieri implicit.

## Limite și particularități ale schemelor CNAS

DocMED permite XML de până la 32 MiB pentru cerere și răspuns; răspunsurile mari se descarcă în fragmente, nu în payload-ul de polling. XML-ul nu poate conține DTD/directive, rădăcini multiple sau adâncime peste 40. SOAP SIUI existent păstrează limita de 1 MiB, iar descărcările de fișiere 64 MiB. Limitele WSM/API-bridge existente pot fi mai mici pentru cereri mari (4 MiB decodat în adapter); nu sunt extinse implicit de această integrare. PDF-ul arhivat ZIP este validat ca Base64 și ZIP, fără extracție pe disc.

Arhiva CNAS locală referă `fpidType` fără să îl definească în `GetDocumentStatusRequest.xsd`, `ConsumeDocumentRequest.xsd`, `UnconsumeDocumentRequest.xsd`. Copiile încorporate în aplicație elimină numai referința nedefinită din union și validează ramura `cidType` numerică documentată. Nu se inventează o definiție pentru identificatori străini. Originalele din `docs/CNAS` sunt păstrate intacte. Cererile pentru această ramură străină necesită clarificarea/publicarea tipului de către CNAS.

Validarea este efectuată pe Windows prin System.Xml și pe macOS/Linux prin xmllint, cu scheme locale. Verificarea locală acoperă toate cele 22 de cereri/răspunsuri, erorile de business, preflight-ul de stare, izolarea tenant/proprietar, deduplicarea și atașamente peste limita SOAP veche. Nu s-au efectuat operații reale în CNAS în timpul dezvoltării.


## Formulare și liste de valori (3 noiembrie 2026)

Toate cele 22 de formulare sunt generate din aceleași XSD-uri încorporate folosite de validarea serverului. Modelul este în `assets/docmed_forms.json`; se regenerează prin `python3 readersv3/scripts/generate-docmed-forms.py` din rădăcina repository-ului.

- Tipul documentului, valorile enumerate CNAS și opțiunile Da/Nu sunt liste derulante cu denumire și cod. CAS-urile folosesc nomenclatorul încorporat. Nu se presupun coduri pentru nomenclatoare externe absente.
- Datele se aleg prin calendar, iar câmpurile numerice au limitele CNAS. Identificatorii, CID/CNP și seriile rămân șiruri, inclusiv zerourile inițiale.
- Câmpurile opționale sunt grupate separat; secțiunile și listele repetabile se adaugă/elimină din formular. Nu se completează XML sau JSON în interfață.
- Codul fiscal și CAS-ul se precompletează din configurație. Datele comune completate se păstrează numai în memoria panoului cât este deschis.
- Pentru atașamente se selectează PDF sau ZIP (maximum 24 MiB în formular); PDF-ul este arhivat automat de bridge. API-ul `fields` acceptă pentru `attachedFile` fie Base64, fie `{filename, data_base64}`. Arhivarea PDF este deterministă, pentru a păstra deduplicarea.
- Variabilele JSON sunt șiruri; obiectele reprezintă secțiuni, iar listele reprezintă înregistrări repetate. Serverul respinge câmpurile necunoscute, valorile nepermise și datele obligatorii lipsă, cu indicarea câmpului. Escape-ul XML este efectuat de encoder, nu de utilizator.

Exemplu de obținere a seriilor, fără XML:

```json
{"operation_id":"docmed-series-001","fields":{"providerCode":"123","providerName":"FURNIZOR EXEMPLU","providerInsuranceHouse":"CAS-DJ","getDocumentSeries":{"insuranceHouseCode":"CAS-DJ","documentType":"ML","validFrom":"2026-11-02","validTo":"2026-11-02"}}}
```

Erorile disting `DocMED request validation` (cererea locală) de `DocMED response validation` (răspunsul CNAS), cu numele schemei. Lipsa validatorului XSD este raportată separat. Un răspuns XML primit, dar neconform XSD, este păstrat pentru diagnostic și marcat `result.docmed.validated=false`; nu este oferit ca rezultat medical validat. O citire eșuează cu `failed`, iar o scriere cu răspuns neconform rămâne `unknown`, fără retrimitere automată. XML-ul primit este descărcabil și în aceste cazuri.


## Diagnostic SOAP Fault

Un SOAP Fault este păstrat separat de răspunsurile medicale validate. `result.docmed.fault` conține `code`, `reason`, `actor`, `detail`, `detail_xml` și `http_status`, cu limite de dimensiune pentru afișare. `SOAPFault.xml` este descărcabil prin fișierul execuției, inclusiv când starea este `unknown`. Licența, antetul de autentificare și tokenul OCSP cunoscute sunt mascate înainte de persistare/afișare. Detaliile nu se scriu în logurile operaționale.

Pentru o scriere, `write_may_have_been_applied=true` arată că fault-ul nu constituie dovada neexecutării. `GenDocumentSeries` rămâne `unknown`, fără retrimitere automată; verifică întâi `GetDocumentSeries` pentru același tip de document. Un apel vechi pentru care s-a păstrat numai eroarea generică nu poate recupera retroactiv corpul SOAP Fault. Nu se generează alte serii numai pentru a obține un mesaj de diagnostic.
