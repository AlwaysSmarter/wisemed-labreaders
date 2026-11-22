# API PIAS eCard

## Autentificare și transport

API local: aceeași sesiune autentificată WiseMED ca shell-ul comun. Toate rutele
`/api/ecard/` cer autentificare; modificările cer `application/json` și, când există
Origin, aceeași origine. PIN, date medicale și fișiere TDX nu se trimit în URL.
Folosiți HTTPS când serviciul este accesat în afara loopback.

WSM: acțiunea comună `api.request`, cu `method`, `path`, `body` în payload,
în cadrul routing-ului autentificat existent. Principalul și tenantul sunt
atestate de server, nu acceptate din body. Drepturi `api:invoke`; pentru
administrare `api:admin`. Joburile sunt accesibile numai proprietarului din
transportul care le-a creat; identitatea locală nu se confundă cu cea WSM.

Exemplu payload WSM (în envelope-ul comun al clientului):

```json
{"method":"POST","path":"/api/ecard/sign/batch?async=true","body":{"operation_id":"346c04d6-675a-44f8-92af-172159720364","request":{"contract":{"cif":"COD_FURNIZOR","cui":"CUI","number":"CONTRACT","date":"2026-01-01","house":"CAS-B","provider_type":"PARA"},"expected":{"cnp":"CNP_ASTEPTAT","cid":"CID_ASTEPTAT"},"encoding":"ascii"},"items":[{"id":"serviciu-1","text":"TEXTUL EXACT"}]}}
```

## Apel unic batch

`POST /api/ecard/sign/batch` primește întreg batch-ul și returnează întreg rezultatul
la terminare. JSON: `operation_id` UUID recomandat, `request` și `items` ca în
exemplul de mai sus. Operația este întotdeauna `sign`; nu este necesară în request.
`encoding`: `ascii` implicit, respinge caracterele non-ASCII în loc să le înlocuiască
silențios cu `?`; alternativ `utf-8`. Fără trim, normalizare Unicode sau newline.
`certificate`: `insured` implicit, alternativ `mai` dacă SDK/cardul îl suportă.

`operation_id` poate fi ales de client înainte de apel, inclusiv pentru polling
și PIN în paralel cu apelul sincron. Aceeași identitate + același conținut + același
ID returnează același job; alt conținut/proprietar produce 409. Un ID șters/expirat
nu este reutilizat. Fără operation_id serverul generează unul; după pierderea
conexiunii NU retrimiteți orbește un batch fără ID stabil.

Headerul `X-ECard-Job-ID` este trimis înaintea așteptării pe HTTP local. Clientul
poate interoga jobul și răspunde solicitărilor PIN în timp ce apelul original
rămâne deschis. HTTP 200 al apelului sincron înseamnă că jobul a fost admis;
rezultatul se decide din `job.state`, nu doar din codul HTTP. Răspuns:

```json
{"job":{"id":"...","state":"completed","operation":"sign","sdk":"2.0.0.4","total":2,"completed":2,"created":0},"items":[{"id":"1","ok":true,"signature":"BASE64_CMS"},{"id":"2","ok":true,"signature":"BASE64_CMS"}]}
```

`?async=true` păstrează încărcarea într-un singur call și returnează imediat 202 cu
ID/stare. Folosiți-l pentru UI, PIN de la PC prin WSM și batch-uri mari. WSM are
limite comune de 4 MiB request/response; apelul sincron WSM admite maximum 100
itemi și nu extinde limitele serverului/clientului intermediar. Pentru volume
mari folosiți paginare. Apelul local unic acceptă până la 32 MiB JSON. Batch-uri
mai mari se încarcă în chunk-uri prin API-ul de joburi de mai jos.

În caz de disconnect, jobul continuă până la final/deadline/anulare explicită.
Nu există replay automat. La o eroare, elementele deja semnate rămân accesibile;
restul nu se semnează. La restart, joburile în curs devin `interrupted`. Dacă
hostul se pierde după expedierea unei comenzi de semnare/scriere, rezultatul poate
fi `unknown`; nu se retrimite pe alt SDK.

## Identitatea pacientului

`request.expected` trebuie să conțină CNP și/sau CID. Se citește identitatea de pe
card înaintea primei semnări:

- CNP normal: dacă există CNP așteptat, trebuie să coincidă; dacă există și CID
  așteptat, trebuie să coincidă și acesta.
- CNP pe card vid sau `0000000000000`: CID așteptat este obligatoriu și trebuie
  să coincidă cu CID citit de pe card. Nu se calculează CID din CNP anonimizat.
- CNP normal diferit: respingere chiar dacă CID coincide.

Hostul păstrează generația cardului și certificatul din aceeași sesiune, verifică
schimbarea înainte/după semnare și verifică CMS-ul rezultat cu certificatul
capturat. Conținutul CMS trebuie să fie identic byte cu byte cu textul encodat.
Nu verifică online lanțul/revocarea pentru a pretinde acceptare CNAS; verifică
corespondența criptografică a rezultatului.

## Joburi și operații

| Rută | Metodă | Rol |
|---|---|---|
| `/ui` | GET | formular eCard, transportabil și prin WSM |
| `/sign/verify` | POST | verificare CMS cu text și semnatar așteptat, fără card |
| `/health` | GET | stare serviciu, disponibilitate platformă |
| `/settings` | GET / PUT | setări active / salvare admin cu aplicare imediată |
| `/jobs` | POST | creează job în `uploading` |
| `/jobs/{id}/items` | POST | adaugă chunk semnare `{offset,items}` |
| `/jobs/{id}/start` | POST | pornește jobul o singură dată |
| `/jobs/{id}` | GET | stare, progres, cerere PIN activă |
| `/jobs/{id}/pin` | POST | răspuns `{challenge,value}`, o singură utilizare |
| `/jobs/{id}/cancel` | POST | anulează, fără a șterge rezultatele confirmate |
| `/jobs/{id}/results?offset=0&limit=20` | GET | rezultate în ordine; `next_offset` |
| `/jobs/{id}` | DELETE | șterge datele unui job încheiat |
| `/tdx/preview` | POST | inspectează TDX și destinația, admin |
| `/tdx/migrate` | POST | migrare explicită confirmată, admin |

Prefixul tuturor rutelor este `/api/ecard`. `POST /jobs` primește obiectul Request:
`operation`, `contract`, `fields`, `data`, `cid`, `expected`, `encoding`,
`certificate`, `confirm`. Operații:

- `readers`, `status`: fără contract; listează dispozitivele/starea.
- `read`: `fields` opțional, implicit A1,B1,B2,B4,C1; returnează cod→valoare.
- `certificates`: certificate publice DER Base64.
- `sign`: încărcare itemi, identitate obligatorie; același mecanism ca batch unic.
- `activate`, `change-pin`, `reset-pin`: operațiile SDK documentate, `confirm:true`.
- `write`: `confirm:true`, `cid`, `fields`, `data` obiect CardData (structura SDK).
- `enroll`: `confirm:true`, administrator; cere explicit înrolarea prin CNAS.

Schimbarea/resetarea PIN și activarea pentru CEI sunt limitate de capabilitățile
SDK/cardului; documentația le descrie pentru cardul de sănătate. Resetarea PIN
necesită și autorizarea externă CNAS. Nu există endpoint pentru exportul cheii
private; G2/G3/G4 nu sunt expuse ca operații de citire.

PIN este cerut de evenimentul SDK; pe cititor cu tastatură se introduce fizic.
Pe cititor fără tastatură: interogați jobul, preluați `pin_request.challenge` și
`prompt`, trimiteți `{challenge,value}`. PIN-ul nu este memorat între operații.
Timeout implicit per comandă/interacțiune: 180 secunde; job: 7200 secunde.

Limite implicite: 100.000 itemi/job, 64 KiB UTF-8/item, 256 MiB texte/job, 32 joburi
păstrate, 500 itemi/chunk, 1 MiB/chunk JSON, rezultate maximum 20/pagină. Joburile
folosesc un singur worker, fără folosirea simultană a cardului. Un mutex Windows
împiedică două hosturi ale acestei aplicații să lucreze simultan.

## Fallback

Ordinea configurată este încercată la pregătirea citirii/certificatelor/semnării,
înaintea primei semnături. Erori eligibile: SDK indisponibil/incompatibil și coduri
SDK -6,-17,-33,-34,-51,-68,-69, numai dacă nu a existat interacțiune PIN.
Nici PIN greșit, nici card blocat/retras, nici timeout, eroare UM sau operație de
scriere nu declanșează replay. După solicitarea PIN sau începerea semnării nu se
schimbă biblioteca automat. Biblioteca aleasă rămâne fixă pentru întreg batch-ul.
Erorile necunoscute se opresc; nu se inventează succes sau semnături.

## Migrare TDX

`preview` primește:

```json
{"filename":"HASH_ORIGINAL.tdx","data_base64":"TDX_CRIPTAT_BASE64","source":{"contract":{"cif":"...","cui":"...","number":"...","date":"2026-01-01","house":"...","provider_type":"..."},"host":"NUME_VECHI","um_endpoint":"IP_REZOLVAT:443"},"confirm_digest":""}
```

Se verifică numele original față de identitate și structura criptată. Răspunsul
conține metadate fără chei, numele destinației și `confirm_digest`. `migrate`
primește același body cu digestul confirmat. Noul host este calculatorul serviciului;
UM este configurația curentă rezolvată. Rezolvarea DNS ambiguă este refuzată:
configurați IP-ul concret. Conținutul criptat rămâne identic; doar numele local
al fișierului se adaptează. Contractul original rămâne neschimbat.

Scriere numai pe Windows în ProgramData/Novensys.eCard.SDK/TerminalData, fără
job activ, cu backup înainte de înlocuire. Nu există generare automată de TDX
la pornire/card inserat. Fișierul este verificat înainte de ObtineToken pentru a
împiedica auto-înrolarea SDK atunci când lipsește sau este corupt.

## Verificarea unei semnături existente

`POST /api/ecard/sign/verify` primește:

```json
{"text":"textul exact","encoding":"ascii","signature_base64":"CMS_BASE64","certificate_base64":"X509_DER_BASE64","public_key_pem":"","detached":false}
```

Este obligatoriu certificatul așteptat sau `public_key_pem` (SPKI / RSA PKCS#1).
Cu cheie publică, CMS trebuie să conțină certificatul semnatarului. Se acceptă un
singur semnatar. `encoding` este ASCII strict sau UTF-8, fără normalizarea textului.
Pentru CMS detașat utilizați `detached:true`; conținutul inclus trebuie altfel să
coincidă byte-cu-byte. Verificarea nu accesează SDK/card/CNAS și funcționează și pe
macOS/Linux. Rezultatul `valid` reflectă verificarea CMS; biblioteca verifică și
încadrarea atributului signingTime în intervalul certificatului, dacă este prezent.
Nu validează lanțul CA, revocarea sau acceptarea CNAS, marcate explicit `false`.
Datele nu se persistă. Parserul și verificatorul sunt din
[digitorus/pkcs7](https://github.com/digitorus/pkcs7/tree/e76b763bdc49).

## Interfață de configurare și debug

- **Setări → Cititor și terminal eCard**: Detectează cititoarele creează un job
  `readers`, îl pornește, citește rezultatul și șterge jobul reușit. Selectați
  cititorul din select și salvați pentru aplicare imediată. Opțiunea automată lasă alegerea SDK-ului.
  Detectarea reflectă cititoarele accesibile contului serviciului Windows.
- **Debug eCard**: operații card și înrolare, migrare TDX, verificare CMS și
  consola tuturor rutelor eCard. Crearea jobului și pornirea sunt pași separați.
  Mutațiile necesită confirmare în consolă, iar operațiile SDK păstrează validările
  `confirm`, identitate și rol de administrator. PIN-ul nu se salvează în browser.
- Formularele folosesc transportul autentificat al paginii principale inclusiv
  prin WSM. API-urile comune server/login/WSM/actualizări rămân în paginile lor.


## Conexiune UM-CEAS configurabilă

În interfața eCard → Biblioteci și terminal, completați Producție/Test sau un
host și port personalizate, apoi salvați; setările se aplică imediat.
Butoanele completează doar formularul; nu salvează automat configurația.
Configurația existentă nu este înlocuită la încărcarea interfeței.

```yaml
modules:
  ecard:
    um_host: umceas.siui.ro
    um_port: 443
```

Producție implicită: `tcp://umceas.siui.ro:443`; test: `tcp://testumceas.siui.ro:443`.
`um_host` este un nume DNS sau IP, fără schemă, port sau cale; `um_port` este
între 1 și 65535. Sunt acceptate și adrese personalizate.
Aceste câmpuri sunt disponibile prin `GET /api/ecard/settings` și
`PUT /api/ecard/settings` (administrator), împreună cu `sdk_order` și `reader_name`.
PUT salvează în config.yaml și întoarce `restart_required: false`; GET reflectă
imediat setările salvate. Salvarea este refuzată cât există joburi
active sau în coadă.

UM-CEAS folosește eCard.SDK, care primește separat hostul și portul prin
`SetAdresaUnitateManagement`, înainte de crearea sesiunii de card. Portul 443
nu înseamnă că endpointul este HTTPS. SIUI/SIPE sunt servicii web separate,
configurate în siui-bridge, nu în eCard.

Surse: [portalul CNAS](https://portal.cnas.ro/cnas/pias/specificatii) și
`docs/CNAS/Specificatie Interfatare PIAS - Anexa 102 - Specificatii_eCard_SDK2.0.0.3-v.pdf`,
secțiunea de configurare UM (exemplul testumceas.siui.ro, port 443).


## Ordine SDK și detectarea cititoarelor

Gridul de configurare oferă câte un select pe rând, Adaugă bibliotecă, Elimină
și mutare sus/jos. Fiecare SDK apare o singură dată; rămâne cel puțin un rând.
Catalogul disponibil este expus prin `GET /api/ecard/health`, `available_sdks`.

Selectul Cititor păstrează opțiunea de selecție automată și cititorul configurat.
Detectarea folosește Windows PC/SC `SCardListReadersW`, nu catalogul SDK
`GetSupportedTerminalNames`. Rezultatul conține dispozitivele conectate și
vizibile serviciului Windows pe calculatorul țintă, inclusiv când UI este
accesată prin WSM. O selecție absentă din detectare este păstrată și marcată
„nedetectat”; detectarea nu modifică automat alegerea operatorului.
Erorile PC/SC sunt raportate ca erori, nu drept listă goală.

SDK-ul selectează un model de cititor. Numele PC/SC cu sufix de instanță este
asociat celui mai lung nume de model compatibil. Dacă sunt conectate mai multe
instanțe ale acelui model, selecția explicită este refuzată pentru a evita
folosirea altui dispozitiv. Configurațiile vechi cu numele modelului SDK își
păstrează comportamentul.

Ordinea SDK, cititorul și hostul/portul UM se persistă înainte de aplicarea în
memorie. Nu necesită restart; salvarea este refuzată cu HTTP 409 cât există
joburi active sau în coadă. Fiecare job folosește o copie stabilă a setărilor.
Această aplicare imediată privește câmpurile din formularul eCard, nu setările
generale ale serviciului (de exemplu portul HTTP).

Enumerare Windows: https://learn.microsoft.com/en-us/windows/win32/api/winscard/nf-winscard-scardlistreadersw
