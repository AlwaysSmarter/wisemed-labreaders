# API WiseMED SIUI Data

Prefix: `/api/siui-data/`. Autentificare: sesiune WiseMED, identitate WSM verificată sau Bearer JWT pe HTTPS. Upload, sincronizare, setup și ștergere de versiuni cer administrator. Mutările din browser cer aceeași origine. Personalizările sunt exclusiv în pagina internă; tokenurile per CUI sunt acceptate doar la endpointurile targeted ale propriului CUI. `Cache-Control: no-store`.

## Formatul răspunsurilor

Obiect unic (status, detaliu serviciu/import, configurație, token, pornire upload/sync):

```json
{"success":true,"data":{"importId":"...","status":"queued"}}
```

Liste (inclusiv categorii, nomenclatoare, înregistrări, versiuni, conexiuni bridge):

```json
{"success":true,"totalCount":125,"rows":[{"code":"001","name":"Consultație"}],"limit":50,"start":0}
```

`totalCount` este totalul după aplicarea filtrelor, înaintea paginării. `rows` este întotdeauna un array, inclusiv când este gol. Listele pot avea metadate suplimentare la nivelul rădăcinii: `importId`, `type`, `asOf`, `versions`, `asOfByType`, `nextStart`, `lastCheckedAt`, `connected`. Detaliile sunt întotdeauna în `data`. Acest contract înlocuiește răspunsurile vechi fără envelope și câmpurile `items` / `codes`.

Erori, cu status HTTP corespunzător:

```json
{"success":false,"error":"Error txt"}
```

`HEAD services/{code}` nu are body, conform HTTP; statusul și headerele indică rezultatul.

## Paginare

Toate listele acceptă `limit` (implicit 50, interval 1–1000) și `start` (implicit 0, offset nenegativ):

```http
GET /api/siui-data/simple-services?type=CLIN&limit=50&start=0
GET /api/siui-data/simple-services?type=CLIN&limit=50&start=50
```

`limit=50` returnează cel mult 50 de rânduri. `start >= totalCount` returnează `rows:[]`, păstrând totalul. Ordinea serviciilor este după cod și versiune; înregistrările brute după ordinal și versiune; versiunile după data pornirii descrescător și ID. În `/services`, un serviciu asociat cu mai multe valori cerute apare o singură dată în aceeași categorie. În `/simple-services` și `/simple-drugs`, `totalCount`, `start`, `limit` și `nextStart` se referă la rândurile plate rezultate după expandarea și filtrarea relațiilor.

Compatibilitate: `after` / `nextAfter` rămân disponibile pentru `/services` și înregistrări dintr-un singur tip; `after` nu se combină cu `start > 0` sau cu mai multe tipuri. `simple-services` și endpointurile de medicamente acceptă numai `start`/`limit`, fără `after`. `offset` este respins; utilizați `start`. `totalCount` rămâne totalul filtrat și când se folosește `after`.

## Filtre simple și array-uri

Fiecare filtru de căutare acceptă un string simplu sau mai multe valori. Valorile aceluiași filtru se combină prin **OR**, iar filtrele diferite prin **AND**.

```http
GET /api/siui-data/services?type=CLIN&specialty=CARD
GET /api/siui-data/services?type=CLIN&specialties[]=CARD&specialties[]=DERM&limit=50&start=0
GET /api/siui-data/simple-services?type=CLIN&specialties[]=CARD&specialties[]=DERM&package[]=BAZA&package[]=EXTINS&isExam=true
GET /api/siui-data/records/Drug?type=CLIN&attr.atc[]=G02CX&attr.atc[]=N02BE
GET /api/siui-data/versions?partnerCategory[]=CLIN&partnerCategory[]=PARA&limit=50&start=0
```

Array-uri acceptate: `key[]=A&key[]=B`, `key[0]=A&key[1]=B`, parametri repetați `key=A&key=B`, sau un array JSON de string-uri în valoarea parametrului (`key=["A","B"]`, codificat URL). `specialties` și `specialities` sunt aliasuri pentru `specialty`. Maximum 100 de valori per filtru, 512 octeți per valoare, 32 filtre `attr.*`. Nu se separă implicit valorile prin virgulă.

### Chips cu toate fragmentele obligatorii

Filtrele de text acceptă și sufixul `All`: `nameAll[]=consulta&nameAll[]=repetata` cere **ambele fragmente (AND)**, indiferent de ordine, majuscule sau diacritice. Fără sufix, array-ul păstrează OR. Un string simplu funcționează în ambele variante. Sunt acceptate aceleași forme de array descrise mai sus, inclusiv array JSON. Fragmentele `%` și `_` sunt text literal.

| Endpointuri | Filtre cu variantă `All` |
|---|---|
| `services`, `simple-services`, `targeted_services/{CUI}`, `targeted-simple-services/{CUI}` | `name`, `q`, `specialtyName` |
| `physicians`, `simple-physicians` | `name`, `q`, `specialtyName`, `insuranceHouseName` |
| `drugs`, `simple-drugs`, `drug-compensations`, `drug-relations` | `name`, `q`, `activeSubstance`, `concentration`, `pharmaceuticalForm`, `presentationMode`, `company`, `diseaseName`, `nationalProgramName`, `copaymentListName`, `commissionName`, `diagnosisName` |
| `records/{kind}`, `drug-records/{kind}`, `physician-records/{kind}` | `name`, `q` |

`nameAll` caută toate fragmentele în aceeași denumire. `specialtyNameAll` cere toate fragmentele în **aceeași specialitate**; nu împarte fragmentele între specialități diferite. La medici și medicamente, filtrele de relație se aplică în continuare aceleiași asocieri; diagnosticele cerute trebuie să corespundă aceleiași înregistrări. Filtrele pentru un CUI păstrează obligatoriu restricțiile profilului său.

`qAll` caută toate chip-urile în textul general al elementului; acestea pot corespunde unor câmpuri diferite. La servicii, medicamente și medici, fiecare chip păstrează regula `q`: maximum 8 cuvinte, minimum 2 caractere pe cuvânt, toate cuvintele obligatorii. În înregistrările brute `qAll` caută fragmente în denumire/descriere. Codurile exacte, flag-urile și limitele numerice păstrează filtrele lor existente.

```http
GET /api/siui-data/services?nameAll[]=consultatie&nameAll[]=repetata
GET /api/siui-data/simple-services?specialtyNameAll[]=cardio&specialtyNameAll[]=logie
GET /api/siui-data/physicians?nameAll[]=popescu&nameAll[]=stefan
GET /api/siui-data/records/Speciality?qAll[]=cardio&qAll[]=logie
GET /api/siui-data/simple-drugs?type=CLIN&presentationModeAll[]=cutie&presentationModeAll[]=20&activeSubstanceAll[]=paracet&activeSubstanceAll[]=molum
```

În Exploratorul API selectează parametrul cu sufix `All`, apoi introdu chip-urile ca array JSON: `["cutie", "20"]`. Explicația afișează AND, iar URL-ul final conține valorile `[]`.

Filtre servicii complete și simplificate:

- `type` / `partnerCategory`: CLIN (implicit), PARA, PHM sau array cu mai multe categorii; fiecare tip cerut trebuie să aibă date importate.
- `code`: cod exact sau mai multe coduri; `name` / `nameAll`: fragmente de denumire.
- `q`: căutare fără diferențe de majuscule/diacritice în cod, denumire, grupă și specialități. Un string poate conține până la 8 cuvinte de minimum 2 caractere; toate cuvintele din acel string sunt obligatorii. Un array oferă expresii alternative.
- `specialty`: coduri exacte; `specialtyName`: fragmente de denumire normalizate.
- `group`, `package`, `serviceType`: valori exacte.
- `isExam`, `opRoom`, `isConnectedService`, `highLevel`, `recByPC`, `formula`, `inClinic`: true/false, inclusiv array-uri. Sunt recunoscute și valorile XML 1/0.
- `attr.{nume}`: egalitate pe atributele originale; numele și valorile sunt parametrizate SQL.

`records/{kind}` acceptă `type`, `code`, `name`, `nameAll`, `q`, `qAll`, `attr.*`, flag-uri și paginare. `includeInactive=true` include istoricul, datele viitoare și perioadele inversate păstrate. Implicit sunt returnate numai înregistrările active la import. `catalogues` acceptă `type`, `kind` și paginare. `categories` acceptă `code` / `partnerCategory`, `name`, `group`, `supported` și paginare.

Parametrii de control (`limit`, `start`, `after`, `includeInactive`), identificatorii din cale și selectoarele unei operații de scriere cer o singură valoare. Array-urile nu lansează importuri sau ștergeri în masă.

## Endpointuri

| Metodă / rută | Rezultat |
|---|---|
| `GET categories` | Listă: cele 14 categorii, nume, grup, `supported` |
| `GET status` | Obiect: versiunile active și metricile procesului |
| `GET services` | Listă: servicii cu toate atributele și relațiile |
| `GET simple-services` | Listă: servicii simplificate, aceleași filtre și paginare |
| `GET/HEAD services/{code}?type=CLIN` | Obiect: un serviciu dintr-un singur tip |
| `GET catalogues` | Listă: tipuri de înregistrări, total și active |
| `GET records/{kind}` | Listă: înregistrări brute, atribute și eventual copii XML |
| `GET versions` | Listă: istoric importuri și `lastCheckedAt` per categorie |
| `GET imports/{id}` | Obiect: progres și rezultat import |
| `DELETE imports/{id}` | Obiect: elimină doar versiuni inactive și finalizate |
| `POST /upload` | Obiect, HTTP 202: upload ZIP direct pe HTTPS |
| `POST upload` / `POST import` | Aliasuri de upload ZIP sub prefixul API |
| `GET/PUT settings` | Obiect: PostgreSQL, director local și limite; parola nu este returnată |
| `GET bridge/connections` | Listă: bridge-uri SIUI și metadată `connected` |
| `POST bridge/sync` | Obiect, HTTP 202: download prin WSM și import dacă sursa e mai nouă |
| `POST auth/token` | Obiect: `access_token`, `token_type`, `expires_in` (900 secunde) |
| `GET debug` | Obiect: HTML-ul exploratorului, pentru consola WSM |

Categoriile neimplementate au istoric gol și nu acceptă upload/sync. CLIN, PARA și PHM au date independente. Nomenclatoarele brute comune nu sunt suprascrise între categorii.

## Servicii simplificate

`GET /api/siui-data/simple-services` returnează câte un obiect plat pentru fiecare combinație serviciu × pachet × specialitate. De exemplu, 2 pachete și 3 specialități produc 6 rânduri pentru același serviciu.

```json
{
  "code":"001",
  "kind":"ClinicService",
  "name":"Consultație",
  "type":"CLIN",
  "isExam":true,
  "opRoom":false,
  "isConnectedService":true,
  "group_code":"G1",
  "package_code":"BAZA",
  "package_name":"Pachet de bază",
  "package_points":"12.3500",
  "package_value_lei":"42.00",
  "package_points_srg":"7.5",
  "speciality_code":"CARD",
  "speciality_name":"Cardiologie"
}
```

Nu există array-uri sau obiecte imbricate în rânduri. Filtrele `package`, `specialty`/`specialties`/`specialities` și `specialtyName` restrâng combinațiile returnate. Ordinea este cod serviciu, versiune, cod pachet, cod specialitate și pozițiile asocierilor din sursă. Paginarea se aplică după expandare; același serviciu poate ocupa mai multe pagini. Dacă lipsesc pachetele ori specialitățile, serviciul rămâne prezent cu câmpurile relației lipsă `null`. PARA are `kind=LaboratoriesService`, cu specialități `null`. Punctajele și sumele păstrează string-urile XML; flag-urile lipsă sunt `false`. `/services` păstrează graful complet.

Fiecare pagină de servicii/înregistrări și calculul totalului folosesc același snapshot PostgreSQL. `versions` / `asOfByType` identifică versiunile pentru fiecare tip; la un singur tip sunt furnizate și `importId`, `asOf`, plus headerele `X-Catalogue-Version` și `X-Catalogue-As-Of`. Dacă un import schimbă versiunea între pagini, comparați metadatele și reluați paginarea pentru o listă coerentă.

## Upload și WSM

```sh
curl --fail --cacert ca.pem -H "Authorization: Bearer $SIUI_ADMIN_JWT" \
  -F type=CLIN -F file=@NomenclatoareClinice_20261001.xml.zip \
  https://localhost:19114/upload
```

Alternativ `POST /upload?type=PARA`, `Content-Type: application/zip`, corpul ZIP. `X-Catalogue-Filename` transmite numele sursei. Succesul HTTP 202 înseamnă acceptare; urmăriți `data.importId` prin `GET imports/{id}` până la `data.status=ready` sau `failed`. Limite implicite: 4096 MiB ZIP, 16384 MiB XML, 120 minute. Importul nu întrerupe accesul la datele active.

Sincronizare: `POST bridge/sync` cu body `{"partnerCategory":"CLIN","connection_id":"..."}`. Cu un singur bridge, `connection_id` poate lipsi. Necesită WSM conectat, `connections:read`, `route:command`, `api:invoke` și un bridge configurat CNAS. `start` pentru CNAS este ora pornirii ultimei verificări WSM reușite pe categorie (`ready` sau `unchanged`), altfel 1 ianuarie din anul precedent în UTC. Erorile nu avansează reperul. CNAS poate returna ultima versiune disponibilă; numai data fișierului primit decide activarea. O dată egală/mai veche produce `unchanged`, fără reimport.

`GET versions` expune `lastCheckedAt` pe întreg istoricul, independent de pagină. Stări: `queued`, `downloading`, `parsing`, `ready`, `failed`, `unchanged`; `state`: `running`, `success`, `fail`. Statisticile au `total`, `active`, `invalidPeriods` pe tip de înregistrare.

JSON poate fi apelat și prin WSM `api.request`, cu același contract și aceleași drepturi. Limita comună este 4 MiB per mesaj, iar downloadul din bridge se transferă în bucăți. Uploadurile mari cer HTTPS direct. Limita fișierului CNAS comprimat în bridge este 64 MiB. Nu se retrimite automat o operație CNAS după eroare.

## Debug

În meniul Debug și la `/siui-data-debug`: alegerea fiecărui endpoint, query-uri simple/array, `start`/`limit`, URL final complet cu valorile codificate, copiere URL, body JSON sau fișier ZIP și răspuns JSON indentat, status HTTP și durată. Nicio operație nu este lansată la simpla selectare a endpointului; cererile se trimit numai cu „Execută cererea”. Drepturile sunt verificate de endpoint. Tokenurile generate sunt afișate doar în rezultatul cererii și nu sunt salvate de explorator.


## Medicamente simplificate

`GET /api/siui-data/simple-drugs` păstrează atributele simple ale medicamentului (`code`, `kind`, `type`, `name`, `activeSubstance`, `concentration`, `pharmaceuticalForm`, `presentationMode`, `company`, `country`, `atc`, flag-uri, `qtyPerPackage`, `pricePerPackage`, `prescriptionMode`). Fiecare asociere CNAS devine un rând plat, combinat cu fiecare diagnostic DCI asociat. Listele de relații sunt reunite prin rânduri identificate prin `sourceKind`: `CopaymentListDrug`, `CopaymentListActiveSubst`, `CopaymentListProtocolTherapeutic`, `NHP_DRUG`. Lista, programul, boala, aprobarea și prețurile de pe un rând provin din aceeași asociere CNAS.

Câmpuri suplimentare, toate scalare sau `null`:

| Câmpuri | Conținut |
| --- | --- |
| `active_substance_name`, `concentration_name`, `pharmaceutical_form_name`, `country_name`, `atc_name` | Denumirile definițiilor asociate |
| `copayment_list_code`, `copayment_list_name`, `copayment_list_percent` | Lista de compensare |
| `national_program_code`, `national_program_name` | Programul național al asocierii |
| `disease_code`, `disease_name` | Categoria bolii din asociere |
| `approval_code`, `approval_name` | Aprobarea/comisia |
| `maxPrice`, `referencePrice`, `maxPriceUT` | Prețurile din asociere, ca string-uri XML |
| `protocol_code`, `protocol_name` | Protocolul terapeutic |
| `relation_diagnosis_code`, `relation_diagnosis_name` | Diagnosticul restricției CNAS |
| `diagnosis_code`, `diagnosis_name`, `diagnosis_disease_code`, `diagnosis_disease_name` | Diagnosticul asociat DCI și categoria sa |
| `relation_<atributXML>` | Toate atributele originale ale asocierii, inclusiv flag-uri, cantități și perioade |

Exemplu de câmpuri dintr-un rând: `{"code":"D1","name":"Paracetamol","sourceKind":"CopaymentListDrug","copayment_list_code":"A","copayment_list_name":"Lista A","national_program_code":"P1","disease_code":"G1","approval_code":"P","maxPrice":"10.0000","referencePrice":"2.000001","diagnosis_code":"994"}`.

Cinci asocieri CNAS și două diagnostice produc zece rânduri. Fără asocieri sau diagnostice, medicamentul rămâne prezent cu câmpurile lipsă `null`. Filtrele de relație și diagnostic restrâng rândurile returnate; celelalte filtre restrâng medicamentele. Ordinea este cod medicament, versiune, ordinal asociere și poziție diagnostic. `totalCount` și paginarea se calculează după expandare și filtrare. Nu este necesar reimportul pentru schimbarea formatului unui import care are deja proiecția de medicamente. `/drugs` și `/drugs/{code}` păstrează documentele complete.

## Medicamente: import și relații

Importul CLIN/PARA construiește și medicamentele prezente în sursă. PHM este acum implementat pentru nomenclatoarele Farmacii, prin upload, WSM și director local (`NomenclatoareFarmacii_YYYYMMDD.xml`). PHM necesită containerul `Drugs`, fără să pretindă că are servicii clinice. Aceleași limite și maximum două importuri simultane per proces se aplică; blocarea PostgreSQL este independentă pe cele trei categorii.

Schema PostgreSQL se migrează la versiunea 2, fără ștergerea importurilor. Versiunile existente păstrează serviciile accesibile; **trebuie reimportate o dată** pentru proiecția de medicamente. `drugProjectionReady=false` indică această situație, iar endpointurile de medicamente răspund 503 cu instrucțiunea de reimport. Nu se interpretează lipsa proiecției ca un catalog gol. Pentru prima reconstrucție folosiți upload manual; watcherul/WSM continuă să evite reimportarea automată a aceleiași date.

La import sunt preasamblate medicamentele active și relațiile active la `asOf`, în aceeași tranzacție cu serviciile și activarea versiunii. Datele istorice/viitoare rămân în `records` și nu participă la proiecția curentă. Medicamentele sau definițiile active duplicate, referințele fundamentale lipsă și ciclurile ATC resping importul; versiunea anterioară rămâne accesibilă. Lipsa unei definiții opționale pe o relație păstrează codul original în atribute și îl marchează în `unresolvedReferences`.

Relații verificate pe XSD și arhivele CNAS furnizate:

| Origine | Legătură / reprezentare |
|---|---|
| `Drug.activeSubstance` | `ActiveSubstance.code`; în sursa reală, codul este chiar denumirea DCI |
| `Drug.concentration` | `Concentration.concentration`; această definiție nu are atribut `code` |
| `Drug.pharmaceuticalForm` | `PharmaceuticalForm.code` |
| `Drug.country` | `Country.code` |
| `Drug.atc` | `ATC.code`, apoi `ATC.parentATC`; documentul conține `atcHierarchy` |
| `Drug.presentationMode`, `company`, `prescriptionMode` | Texte/coduri originale; nu inventăm dicționare pentru ambalaje, producători sau echivalențe cu `PrescriptionType` |
| `CopaymentListDrug.drug` | `Drug.code`; fiecare asociere are propriile prețuri, program, boală și aprobare |
| `copaymentListType` | `CopaymentListType.code`, inclusiv procentul de bază și regulile `CpmetLstTypePersState → PersonState` în `personStateRules` |
| `nhpCode` / `NHP_DRUG.nhp` | `NHP.code`, cu descriere, `programCode` și celelalte atribute |
| `CopaymentListDrug.diseaseCode` | `DiseaseCategory.code` (de exemplu G15), **distinct de ICD10** |
| `hgIcd10`, `CopaymentListProtocolTherapeutic.icd10` | `ICD10.code` |
| `ActSubstICD10` | DCI → ICD10 → DiseaseCategory, expus separat în `diagnoses` |
| `CopaymentListActiveSubst` | DCI + `aTC` → medicamente cu aceeași DCI și același ATC; păstrează `diseasecategory`, lista, programul și aprobarea |
| `CopaymentListProtocolTherapeutic` | DCI + `atc` → medicamentele corespunzătoare; păstrează protocolul, lista, boala, diagnosticul și programul |
| `NHP_DRUG` | `type=DRG`: cod de medicament; `type=ACS`: cod de substanță activă. Păstrează `NHP_DRUG_TYPE` și programul. Bunurile/serviciile din celelalte tipuri nu sunt transformate în medicamente |

`TherapeuticalSchema`, `MedicalFormType`, `NHPCategory` și celelalte nomenclatoare conexe sunt accesibile prin endpointurile brute filtrate pe atribute. Nu inferăm indicații terapeutice numai pentru că două înregistrări au același program național.

### Endpointuri medicamente

| Rută | Rezultat |
|---|---|
| `GET drugs` | Medicamente unice, cu definiții și graful complet de relații |
| `GET simple-drugs` | Rânduri plate: medicament × asociere CNAS × diagnostic DCI, fără obiecte/array-uri imbricate |
| `GET/HEAD drugs/{code}?type=CLIN` | Un medicament, dintr-o singură categorie |
| `GET drug-compensations` | Asocierile `CopaymentListDrug` filtrate, fiecare cu prețurile și definițiile sale |
| `GET drug-relations` | Toate cele patru tipuri de relații; `sourceKind` distinge regulile pe medicament, DCI, protocol și NHP |
| `GET drug-catalogues` | Tipurile de înregistrări relevante pentru medicamente, număr total și active |
| `GET drug-records/{kind}` | Alias restrâns la nomenclatoare medicamentoase al `records/{kind}` |

Toate folosesc același format `success/data` sau `success/totalCount/rows`, autentificarea existentă și paginarea `limit`/`start`. Pentru medicamente nu se acceptă `after` sau `offset`. Selectarea implicită este CLIN; pentru sursa Farmacii folosiți `type=PHM`. `status` și istoricul importului expun `drugs` și `drugProjectionReady`.

Documentul complet conține `attributes`, definițiile `activeSubstance`, `concentration`, `pharmaceuticalForm`, `country`, `atc`, `atcHierarchy`, și array-urile `compensations`, `substanceCompensations`, `therapeuticProtocols`, `nationalProgramDrugs`, `diagnoses`. Fiecare relație păstrează `sourceKind`, atributele originale și definițiile asociate. Datele monetare și cantitățile sunt string-uri cu precizia XML; comparațiile de preț folosesc PostgreSQL `numeric`, fără rotunjiri float.

### Filtre medicamente

String simplu sau array; OR între valorile aceluiași filtru, AND între filtre. Pentru toate fragmentele în același câmp folosește varianta `All` din tabelul de mai sus (inclusiv aliasurile `drugAll`, `medicineAll`, `substanceAll`, `formAll`, `packagingAll`):

- `code` / `drugCode`: cod exact; `name` / `drug` / `medicine`: fragment de denumire; `q`: căutare pe cod, denumire, DCI, concentrație, formă, prezentare și producător.
- `activeSubstance` / `substance`, `concentration`, `pharmaceuticalForm` / `form`, `presentationMode` / `packaging`, `company`: fragmente normalizate. Pentru egalitate exactă: `activeSubstanceCode`, `concentrationCode`, `pharmaceuticalFormCode`.
- `country`, `atc`, `prescriptionMode`: coduri exacte; `atcPrefix`: prefix ATC.
- `isSpecial`, `isFractional`, `isBrand`, `hasBioEchiv`: boolean sau array de booleeni. `isNarcotic` păstrează clasificarea originală ca string, nu o transformă arbitrar în boolean.
- `copaymentListType` / `copaymentList`, `nhpCode` / `nationalProgram`, `diseaseCode`: coduri exacte. `copaymentListName`, `nationalProgramName`, `diseaseName`: fragmente din denumiri.
- `needApproval` / `commission`: codul aprobării, inclusiv 0, M, P, S, N, J, A; `commissionName`: fragment de denumire. Codurile necunoscute rămân păstrate. Nu există un nomenclator separat de comisii în aceste surse.
- `maxPrice`, `referencePrice`, `maxPriceUT`, `pricePerPackage`: egalitate numerică sau array de valori alternative. Fiecare acceptă sufixele `Min` și `Max` pentru interval **inclusiv**, de exemplu `referencePriceMin=1.25&referencePriceMax=5`. Limitele sunt scalare, nenegative, cu separator punct; minimul nu poate depăși maximul. Un preț absent nu este tratat ca zero.
- `specialLaw`, `needSpecialty`, `overValue`, `openCircuit`: flag-uri ale relației, true/false; `contractCv`, `hgIcd10`, `classifInsulin`: valori exacte.
- `diagnosisCode`, `diagnosisName`: diagnosticul legat explicit de DCI prin `ActSubstICD10`. Pentru diagnosticul dintr-o compensare utilizați `hgIcd10` / `diseaseName`.
- `sourceKind`: CopaymentListDrug, CopaymentListActiveSubst, CopaymentListProtocolTherapeutic, NHP_DRUG.
- `attr.*`: atribut original al medicamentului; `compensation.attr.*`: atribut original al relației. Maximum 32 de filtre de atribute în total. Parametrii necunoscuți ai căutării de medicamente sunt respinși cu 400 pentru a evita ignorarea accidentală a unui filtru.

**Filtrele listei, programului, bolii, aprobării și prețurilor se aplică pe aceeași relație.** Nu se combină prețul din lista A cu programul din lista C1. O căutare în `drugs` selectează medicamentele potrivite, dar returnează întregul lor graf; pentru numai asocierile potrivite utilizați `drug-compensations` sau `drug-relations`. `totalCount` numără medicamente la `/drugs`, relații la endpointurile de relații și combinații plate la `/simple-drugs`.

```http
GET /api/siui-data/drugs?type=CLIN&name=paracetamol&limit=50&start=0
GET /api/siui-data/simple-drugs?type=PHM&activeSubstance[]=PARACETAMOLUM&activeSubstance[]=IBUPROFENUM&isSpecial=false
GET /api/siui-data/drugs?type=CLIN&copaymentListType=C1&diseaseCode=G15&needApproval=P&maxPriceMin=10&maxPriceMax=100
GET /api/siui-data/drug-compensations?type=PHM&nhpCode=3&referencePriceMin=1&referencePriceMax=20
GET /api/siui-data/drug-relations?type=PHM&sourceKind=NHP_DRUG&limit=50&start=0
GET /api/siui-data/drug-records/Concentration?type=PHM&code=500%20mg
```

Exploratorul API oferă pentru fiecare endpoint un **combobox cu parametrii acceptați** și explicația discretă sub parametrul selectat. La alegerea `attr.*` / `compensation.attr.*` apare câmpul pentru numele atributului original. Explicațiile indică array-urile, intervalele și semnificația codurilor; URL-ul final reflectă fiecare modificare.


## Medici: getPhysicianCatalogues

Arhiva se importă prin `POST /upload` (sau `/api/siui-data/upload`) cu `type=PHYS` și conține ambele fișiere `MediciCNAS_YYYYMMDD.xml` și `MediciOPSNAJ_YYYYMMDD.xml`. PHYS este un tip intern de set de date, separat de categoriile de furnizori CNAS. Importul este atomic: ambele surse se validează înainte de activare, iar versiunea anterioară rămâne accesibilă. Un ZIP incomplet eșuează fără înlocuirea datelor active.

Fișierele au dicționare proprii `Speciality`, `InsuranceHouse`, `InsuranceHouseType`; legătura este `Physician.stencil` → `PhysicianSpeciality.stencil`, apoi `specialityCode` → `Speciality.code`, `insuranceHouse` → `InsuranceHouse.code`, `InsuranceHouse.type` → `InsuranceHouseType.code`. Identitatea unui medic este `(source, stencil)`, unde `source` este `CNAS` sau `OPSNAJ`. O parafă poate exista în ambele surse. Numele și contractele se păstrează pentru fiecare sursă, fără amestecarea definițiilor.

| Endpoint, cu prefixul `/api/siui-data/` | Rezultat |
| --- | --- |
| `GET physicians` | Un rând per medic și sursă; `attributes` și `specialities` cu asocierile complete |
| `GET physicians/{stencil}?source=CNAS` | Un medic; dacă parafa este ambiguă și sursa lipsește, HTTP 409 |
| `HEAD physicians/{stencil}?source=CNAS` | Verificare existență fără body |
| `GET simple-physicians` | Rânduri plate per medic × asociere activă specialitate–contract |
| `GET physician-catalogues` | Nomenclatoarele și statisticile din export |
| `GET physician-records/{kind}` | Înregistrări originale, inclusiv `source`; `includeInactive=true` include istoricul |

Tipul implicit pentru aceste endpointuri este PHYS. Listele folosesc `success`, `totalCount`, `rows`, `limit`, `start`, `nextStart`; nu acceptă `after` la căutarea medicilor. `physicians` numără medicii pe sursă; `simple-physicians` numără asocierile filtrate. Fără asocieri active, medicul rămâne în lista simplificată cu câmpuri ale relației `null`. Datele active se determină din perioadele XML la data importului; înregistrările istorice rămân disponibile prin API-ul brut. Înregistrările `PhysicianSpeciality` fără un medic activ corespunzător rămân în datele brute și în statistici; nu se inventează medici pentru acestea.

Filtrele acceptă string sau array, OR în cadrul câmpului și AND între câmpuri:

| Filtru | Semnificație |
| --- | --- |
| `source` | `CNAS` / `OPSNAJ` |
| `name` | Fragment din nume, fără diferențe de majuscule sau diacritice |
| `stencil` (aliasuri `code`, `parafa`) | Parafa exactă |
| `q` | Cuvinte în nume și parafă |
| `specialty`, `specialties`, `specialities`, `specialityCode` | Coduri exacte de specialitate |
| `specialtyName`, `specialityName` | Fragment din denumirea specialității |
| `contractType` | Cod exact SIUI: CLN, SPT, CMG, STM, PAR, PNS etc. |
| `contractNo` | Număr exact de contract |
| `physicianType` | Cod exact SIUI: A, H, F, P, S |
| `insuranceHouse`, `insuranceHouseName`, `insuranceHouseType` | Cod, fragment din numele casei și tipul casei |
| `attr.<nume>` | Atribut original al medicului |
| `association.attr.<nume>` | Atribut original al asocierii medic–specialitate–contract |

Filtrele specialității, contractului și casei se verifică pe **aceeași asociere**. `/physicians` returnează toate asocierile active ale medicului selectat; `/simple-physicians` returnează numai rândurile potrivite.

```http
GET /api/siui-data/physicians?name=popescu&limit=50&start=0
GET /api/siui-data/physicians?stencil[]=A72003&stencil[]=D85112
GET /api/siui-data/simple-physicians?source=CNAS&specialties[]=CARDIOLOGIE&specialties[]=MEDICINA%20INTERNA&contractType[]=CLN&contractType[]=SPT&limit=50&start=0
GET /api/siui-data/physicians/A72003?source=CNAS
GET /api/siui-data/physician-records/PhysicianSpeciality?source=OPSNAJ&includeInactive=true
```

Rândul simplificat conține numai valori scalare sau `null`:

```json
{
  "code":"P1", "stencil":"P1", "name":"Ștefan Popescu",
  "kind":"Physician", "type":"PHYS", "source":"CNAS",
  "validFrom":"2020-01-01", "validTo":null,
  "speciality_code":"CARD", "speciality_name":"Cardiologie",
  "contractNo":"100", "contractType":"CLN", "physicianType":"A",
  "insurance_house_code":"CAS-BV", "insurance_house_name":"CAS Brașov",
  "insurance_house_type_code":"CJAS", "insurance_house_type_name":"Casă județeană",
  "association_valid_from":"2020-01-01", "association_valid_to":null
}
```

În plus, `association_<atributXML>` păstrează toate atributele originale ale asocierii. Dicționarele lipsă produc denumiri `null`, păstrând codul din asociere.

Directorul local monitorizează perechi cu aceeași dată în nume, în `local_import_dir` sau în subdirectorul său `getPhysicianCatalogues`. Se așteaptă ambele fișiere stabile și se importă numai o pereche cu dată mai nouă decât ultimul import reușit. Datele `issueDate` ale CNAS și OPSNAJ pot diferi de data numelor: se păstrează separat în `sourceDates`, afișate în Versiuni. În sursa verificată, CNAS este 2026-10-21, OPSNAJ este 2026-10-08, ambele fișiere fiind numite cu 20261021.

WSM: `POST bridge/sync` cu `{"partnerCategory":"PHYS","connection_id":"..."}` apelează bridge-ul `general/getPhysicianCatalogues` cu `parameters:{}` și transferă ZIP-ul în bucăți. Operația SIUI nu primește `start` sau `partnerCategory`. Datele `issueDate` se compară separat pentru CNAS și OPSNAJ: se importă dacă măcar una avansează; dacă ambele sunt egale, rezultatul este `unchanged`. O sursă mai veche decât versiunea activă produce eșec pentru a preveni regresia celeilalte surse. Data maximă este afișată ca data generală a exportului. Dashboardul include PHYS, numărul de medici și acțiunea de import/sincronizare. Exploratorul API oferă endpointurile, filtrele, explicațiile și URL-ul final.

Validare pe arhiva furnizată: 372.080 înregistrări, 73.785 medici activi pe surse și 98.776 asocieri active în XML. 51 de asocieri active nu au medic activ în aceeași sursă și sunt accesibile în datele brute. Trei perioade inversate sunt păstrate în istoric și raportate ca inactive.


## Personalizări interne și servicii țintite

### Interfața confidențială

`/siui-data-clients` este o pagină HTTP locală, accesibilă numai administratorului autentificat prin sesiunea WiseMED. Importul, istoricul, portofoliul, contractele, medicii și administrarea cheilor folosesc rute auxiliare sub aceeași pagină, **în afara `/api/`**. Aceste rute resping orice Bearer JWT (inclusiv global), identitatea WSM și utilizatorii fără rol de administrator. Mutările cer HTTPS și header intern de protecție CSRF; nicio rută nu permite CORS. Răspunsurile sunt `no-store`. Rutele private nu apar în Exploratorul API.

Pagina importă un XML sau un ZIP cu personalizări din aceeași categorie, maximum 64 MiB la upload și 128 MiB XML expandat. Categoria se selectează în interfață; pentru nume precum `PARA.xml` se selectează automat. Sunt acceptate cele 14 categorii de furnizori, fără PHYS. CUI-ul se citește din `provider.uic`, nu se introduce separat; prefixul RO și zerourile inițiale sunt normalizate. Un ZIP cu mai multe personalizări se publică într-o singură tranzacție: un document invalid anulează întregul import. Datele vechi rămân disponibile.

Datele păstrate sunt atributele furnizorului, contractele/convențiile, personalul și specialitățile acestuia. Listele de pacienți și celelalte secțiuni fără legătură cu profilul de contracte nu sunt reținute. Documentele confidențiale sunt în tabele `siui_client_*`, fără amestec cu `siui_data_records` sau istoricul public al nomenclatoarelor.

### Istoric și eligibilitate

Identitatea unei versiuni este `(CUI, categorie importată, cod furnizor, issueDate)`. `issueDate` devine momentul de aplicare; o valoare fără fus orar se interpretează în Europe/Bucharest. Reimportul aceluiași document este idempotent. Alt conținut la exact același issueDate este respins, pentru a nu modifica retroactiv o versiune; se importă o personalizare cu issueDate nou.

La o dată aleasă se selectează ultima versiune cu issueDate până la sfârșitul zilei respective, separat pe categorie și cod furnizor. Versiunile mai noi nu modifică profilul anterior. În lipsa unei versiuni aplicabile, profilul este gol. Valabilitățile `validFrom`/`validTo` sunt inclusive; perioadele inversate sau invalide sunt respinse. Personalizarea care elimină un medic/specialitate îl elimină din profil de la noul issueDate. Nu există editare manuală a eligibilității.

Pentru servicii CLIN este necesar, în aceeași personalizare:

1. Un contract cu `categoryCode=CLIN` (alias CLN), activ la data cerută. O convenție nu este contract de servicii.
2. Un medic din lista de personal, cu parafă nenulă și perioadă de activitate care include data.
3. O specialitate a acelui medic, valabilă la aceeași dată.

Categoria importată nu înlocuiește verificarea contractului: SPT poate conține și un contract CLIN. Specialitățile eligibile se reunesc din toate versiunile aplicabile care au contract CLIN activ. Contractul PARA/PAR este importat, afișat și disponibil pentru filtrarea internă a portofoliului, dar nu acordă acces la servicii CLIN.

Pagina arată toate contractele/convențiile din versiunea aplicabilă, cu stare activ/inactiv la data cerută, pe fiecare categorie/furnizor. Portofoliul se poate filtra după CUI/nume și categoria unui contract activ la o dată. Istoricul afișează ultimele 100 de versiuni ale CUI-ului.

### Endpointuri publice țintite

```http
GET /api/siui-data/targeted_services/7991962?atDate=2026-10-22&limit=50&start=0
GET /api/siui-data/targeted-simple-services/7991962?atDate=2026-10-22&limit=50&start=0
```

`targeted_services` întoarce servicii complete, iar `targeted-simple-services` păstrează forma plată serviciu × pachet × specialitate. Ambele folosesc envelope-ul `success/totalCount/rows` și paginarea `limit/start/nextStart`, fără `after`.

`atDate` este scalar, YYYY-MM-DD, implicit data curentă în Europe/Bucharest (`date` este alias; nu se combină cu `atDate`). Profilul este evaluat la această dată. Definițiile serviciilor provin din **nomenclatorul CLIN activ**, identificat prin metadatele `importId`, `asOf`, `versions`; endpointul nu reconstruiește un nomenclator istoric. Perioada serviciului este verificată suplimentar la data cerută.

Filtre acceptate, aceleași string/array ca în `services`: `code`, `q`, `kind`, `specialty`/`specialties`/`specialities`, `specialtyName`, `group`, `package`, `isExam`, `opRoom`, `isConnectedService`, `attr.*`, `limit`, `start`. `type`/`partnerCategory`, dacă sunt furnizate, pot fi numai CLIN. Alte filtre de control sunt respinse. Filtrele utilizatorului se intersectează cu eligibilitatea; nu o pot lărgi, nici prin array-uri. Serviciile complete păstrează numai specialitățile eligibile în graful returnat; lista simplificată întoarce numai combinațiile eligibile. Totalul se calculează după toate restricțiile, în același snapshot PostgreSQL ca profilul și pagina de servicii.

Fără profil, fără contract activ ori fără medici eligibili: HTTP 200, `totalCount:0`, `rows:[]`, inclusiv înainte de primul import al nomenclatorului CLIN. Nu există fallback la lista completă. Răspunsul nu divulgă nume de client/medic, parafe, contracte, personalizări sau ID-uri de profil. CUI-ul din path nu poate fi înlocuit prin query.

### Chei JWT pe CUI

Configurație în `modules.siui-data`:

```yaml
jwt_secret_env: WISEMED_SIUI_DATA_JWT_SECRET # global, numai pe server
client_jwt_secrets: {} # populat din pagina internă, sau map CUI -> secret distinct
```

Fiecare secret are 32–4096 bytes, diferă de cheia globală și de cheile celorlalte CUI-uri. Cheile din map sunt CUI-uri numerice canonice. Nu se expun în GET settings, în istoricul API, în browser sau în răspunsul la salvare. În pagina internă, cheia nouă poate fi copiată înainte de salvare; după salvare se poate numai înlocui/revoca. Rotirea sau ștergerea cheii invalidează imediat tokenurile semnate anterior.

Un JWT al clientului folosește HS256 și exact acest domeniu de acces (timestampurile exemplului se înlocuiesc cu timpul curent):

```json
{"alg":"HS256","typ":"JWT","kid":"7991962"}
```

```json
{
  "iss":"wisemed-siui-data",
  "aud":"siui-data-api",
  "sub":"integrare-client",
  "cui":"7991962",
  "admin":false,
  "iat":1700000000,
  "exp":1700000900
}
```

Headerul `kid` selectează o singură cheie, iar claimul `cui` trebuie să corespundă exact. Un token cu o cheie CUI nu poate pretinde `admin:true`, alt CUI sau cheia globală. Tokenurile lipsă, expirate, modificate, cu alt algoritm ori semnate cu cheia altui CUI sunt respinse cu 401. Un token valid CUI care solicită alt CUI sau alte endpointuri este respins cu 403, chiar dacă în aceeași cerere există o sesiune/identitate administrativă.

Tokenurile CUI pot apela exclusiv GET/HEAD pe cele două endpointuri targeted pentru CUI-ul propriu. Nu pot accesa `/services` fără filtrare, medicamente, medici, configurări, importuri sau emite tokenuri. Cheia globală rămâne pe server; browserul intern de administrator poate emite un token global temporar, valabil pentru mai multe CUI-uri, prin `POST auth/token` cu `{}`. Prin `{"cui":"7991962"}` emite un token cu cheia acelui CUI. Emiterea se permite exclusiv din sesiunea locală de administrator, pe HTTPS; WSM sau alte JWT-uri nu pot emite tokenuri. Tokenurile globale vechi fără `kid` rămân validate cu cheia globală; pentru targeted au nevoie de `admin:true`.

Exemplu de apel al clientului:

```sh
curl --fail --cacert ca.pem \
  -H "Authorization: Bearer $CLIENT_JWT" \
  'https://localhost:19114/api/siui-data/targeted-simple-services/7991962?atDate=2026-10-22&limit=50&start=0'
```

Validare pe exemplele furnizate: CLIN (1 contract și 1 convenție, 27 angajați), PARA (1 contract, 4 angajați), SPT (2 contracte, inclusiv CLIN, și 1 convenție, 13 angajați), AMB (1 convenție, fără angajați), SICK (1 convenție, 32 angajați). La data personalizărilor, profilul reunește 12 specialități eligibile CLIN. Validarea a rulat într-o bază PostgreSQL temporară izolată.


Medicamentele din nomenclatoarele clinice sunt importate împreună cu serviciile: `drugs?type=CLIN` și `simple-drugs?type=CLIN`. Nu este necesară o versiune PHM pentru a le interoga. Dashboardul afișează și numărul medicamentelor active în cardurile CLIN/PARA; `simple-drugs` din Explorator API pornește cu sursa CLIN, fără filtrul implicit `isSpecial=true`. Sursa PHM rămâne selectabilă explicit. Prefixul local `NomenclatoarePHM_YYYYMMDD.xml` este acceptat ca alias al `NomenclatoareFarmacii_YYYYMMDD.xml`.

Validarea strictă a înregistrărilor `Physician`/`PhysicianSpeciality` se aplică numai exportului PHYS (`getPhysicianCatalogues`). În CLIN/PHM, înregistrările auxiliare incomplete ale medicilor se păstrează în sursă fără să blocheze importul medicamentelor.
