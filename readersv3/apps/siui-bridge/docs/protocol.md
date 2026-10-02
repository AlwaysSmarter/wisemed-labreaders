# Validare 72h - API HTTP/HTTPS și WSM

Un singur handler HTTP autentificat este invocat local sau in-process prin
adaptorul WSM `api.request`. Nu există proxy către IP-ul local și nici port nou
pentru transportul WSM.

## Date și surse

Contractul SOAP este preluat din `docs/CNAS/DefinitiiServiciiWeb/SiuiValidateWS.wsdl`:
SOAP 1.1 RPC/encoded, SOAPAction gol, namespace
`http://siuiValidate.webservices.utils.svapnt.siveco.ro`.
Parametrii sunt, în ordine: `reportXml`, `reportType=PARA`, `requestType=RQ_PARA_SRV`.
XML-ul cererii nu se semnează CMS și nu se arhivează pentru această metodă.
Semnăturile cu cardul pacientului, dacă sunt necesare, trebuie să existe în XML-ul
furnizat de WiseMED; utilitarul nu le generează sau modifică.

Referințe locale: Anexa 001 v3.7.32, Anexa 008 v3.7.31 și documentul principal
PIAS v3.7.32, secțiunile 5.1 și 5.10. XSD-urile de cerere/răspuns și tipurile comune
sunt incluse în binar, copiate din documentația furnizată. WSDL-ul este păstrat
pentru trasabilitate. Verificările pasive folosesc și SiuiInsuredWS/SiuiWS.

OCSP: GET `/OCSP/validator?username=...` cu certificat client și Basic
(utilizator/cheie de activare). Antetul **OSCP_RESPONSE** este numele literal din
PIAS. Sesiunea nouă este utilizată la o singură validare și nu este persistată sau
returnată clientului. Nu se ghicește durata ei de valabilitate.

## Rute

| Metodă și rută | Rol | Rezultat |
|---|---|---|
| GET `/api/siui/status` | autentificat | configurat/platformă/joburi în așteptare |
| GET `/api/siui/settings` | admin | utilizator, origine CNAS, magazin, thumbprint, licence_configured, licence_hint |
| PUT `/api/siui/settings` | admin | persistă configurația; refuză schimbarea în timpul joburilor |
| GET `/api/siui/certificates?store=CurrentUser` | admin | certificate Windows și metadate; fără export |
| POST `/api/siui/validations` | autentificat | acceptă un lot și returnează jobul |
| GET `/api/siui/validations` | autentificat | ultimele 50 de joburi ale solicitantului |
| GET `/api/siui/validations/{id}` | autentificat | stare, răspuns XML și rezultat structurat |

PUT acceptă și `licence` (maximum 4096 caractere), valoare write-only: absentă
sau goală păstrează licența existentă; o valoare nouă o înlocuiește. GET returnează
`licence_configured` și `licence_hint` (primele/ultimele 3 caractere,
separate prin `…`; valorile de maximum 6 caractere sunt mascate integral). Configurarea poate fi salvată fără certificat pe Mac;
trimiterea validărilor cere utilizator, licență și certificatul selectat.

Store poate fi `CurrentUser` sau `LocalMachine`. API-ul de setări nu primește PIN,
cheie privată, fișier `.cer`, cale de fișier sau URL arbitrar pentru un apel.
Originile personalizate se configurează local; UI permite numai originea deja
configurată sau originile oficiale www/testsiui.siui.ro pe porturile 443/444.

### HTTP/HTTPS local

Autentifică-te prin login-ul WiseMED al utilitarului și folosește cookie-ul acelei
sesiuni. Testele curl trebuie să folosească CA-ul certificatului local, fără `-k`.

```sh
curl --cacert local-ca.pem --cookie cookies.txt \
  -H 'Content-Type: application/json' \
  --data-binary @validation.json \
  https://127.0.0.1:19112/api/siui/validations
```

Corpul `validation.json`:

```json
{"operation_id":"validation-72h-unique-001","xml":"<request xmlns=\"http://www.cnas.ro/siui/2.0\" ...>...</request>"}
```

Folosește XML complet după XSD, nu exemplul prescurtat de mai sus. Există un
[exemplu structural sintetic](validate-request.xml) care trece XSD; codurile sale
TEST nu reprezintă date valide pentru CNAS și nu trebuie trimise în producție.

### WSM

Browserul/service-ul se autentifică pe WSM și așteaptă `hello_ack`. Tokenul său
necesită `route:command,api:invoke`; pentru configurare/certificate, și `api:admin`.
Echipamentul necesită identitate reader, equipment_id și cel puțin `route:reply`.

```json
{
  "type":"command",
  "request_id":"transport-unique-001",
  "target":{"mode":"equipment","equipment_id":"42"},
  "payload":{
    "command":"api.request",
    "args":{
      "method":"POST",
      "path":"/api/siui/validations",
      "body":{"operation_id":"validation-72h-unique-001","xml":"<request ...>...</request>"}
    }
  }
}
```

Se primește ACK-ul de rutare separat de reply. Reply-ul conține
`payload.kind=api.response`, `status=202`, `body.ok=true` și `body.job.id`.
202 înseamnă acceptat local, nu validat CNAS. Interoghează cu un nou request_id:

```json
{"type":"command","request_id":"poll-unique-002","target":{"mode":"equipment","equipment_id":"42"},"payload":{"command":"api.request","args":{"method":"GET","path":"/api/siui/validations/JOB_ID"}}}
```

## Rezultate și reluare

- `queued` / `running`: în așteptare / în lucru. Un singur job folosește tokenul la un moment dat.
- `completed`: răspuns CNAS primit și verificat după XSD. **Nu implică validitate.**
  Citește `result.validated` și starea fiecărui serviciu: `0=INVALID`, `1=VALID`,
  `2=PARTIAL VALID`. Răspunsurile pentru AppID necunoscut/duplicat sunt respinse;
  absența unui serviciu nu poate produce `validated=true`.
- `failed`: eroare înainte de trimiterea SOAP (configurare, XSD, autentificare).
- `unknown`: SOAP a fost început, dar rezultatul nu poate fi confirmat, sau
  procesul s-a oprit cu un job neterminat. Reconciliază înainte de retrimitere.

`operation_id` este stabil pentru aceeași cerere și același utilizator/tenant.
Repetarea lui cu același XML întoarce același job fără alt apel CNAS; cu XML diferit
întoarce 409. `request_id` este separat, pentru corelarea fiecărui mesaj WSM.
La corectarea unui serviciu folosește un nou operation_id și **același AppID** al
serviciului, conform specificației CNAS. Nu există retry automat sau programare
la 72h. Joburile neterminate devin `unknown` după restart; XML-ul cererii nu este
stocat pe disc. Rezultatul rămâne accesibil după reconectare/restart.

Limite: XML 1 MiB, 1–1000 servicii per cerere, 20 de loturi în coadă și unul activ,
timeout 90 secunde pentru procesarea unui job. Depășirea cozii refuză trimiterea.
Asincronia evită depășirea timeoutului de 30s al adaptorului WSM. Păstrează setarea
WSM `max_message_bytes: 8388608` din configurația de producție pentru payloaduri
mari. Baza de date este locală; la mutarea echipamentului trebuie mutată și ea.

## Verificări pasive: fără validateReport

Aceleași rute sunt disponibile prin HTTP/HTTPS și WSM `api.request`, folosind POST
cu JSON (CNP nu apare în URL). Autentificarea utilizatorului este obligatorie.

| Rută | Corp | Rol |
|---|---|---|
| `/api/siui/ocsp-test` | `{}` | autentificat |
| `/api/siui/insured` | `{"mode":"local"}` | autentificat; test fictiv fără rețea |
| `/api/siui/insured` | `{"mode":"cnas","cnp":"CNP_13_CIFRE","date":"2026-10-03"}` | autentificat |
| `/api/siui/personalization` | `{"start":"2026-10-01","stop":"2026-10-03"}` | admin / api:admin |

OCSP: HTTP 200 cu `authenticated:true` numai după primirea sesiunii valide, fără
expunerea ei. Fiecare operație reală obține propria sesiune OCSP înainte de SOAP.
Eșecul OCSP oprește operația. Nu este necesar un test OCSP manual înainte de CNP.

Insured: `result.state` este -1=eroare, 0=inexistent, 1=asigurat, 2=neasigurat,
3=decedat. `result.insured` este true doar pentru 1, false doar pentru 2, null pentru
celelalte; `result.xml` păstrează răspunsul verificat XSD. `mode:local` returnează
`simulated:true,cnas_contacted:false` și o etichetă explicită; nu contactează CNAS și
folosește exclusiv o înregistrare fictivă, indiferent de un CNP furnizat.

Personalizare: SOAP document/literal `getProviderInfo(partnerCategory=PARA,start,
stop,uic)` conform SiuiWS.wsdl; CUI derivat din utilizator. Se verifică URL-ul de
răspuns (aceeași origine HTTPS), apoi se descarcă fișierul fără redirecturi.
Răspunsul JSON conține `file.filename,content_type,size,data_base64`. Nu se scrie
pe server; browserul salvează conținutul original ZIP/XML. XML-ul trebuie să aibă
rădăcina PIAS `provider`; nu se afirmă validarea completă a personalizării prin XSD.
Limita este 1 MiB (comprimat și decomprimat). Acest format încape în limita WSM.

Operațiile au timeout 25s, returnează 409 dacă tokenul este ocupat, 503 pe platforme
fără transport nativ, 502 la eșec upstream. Timeoutul este o eroare, nu un rezultat
medical. Nu există retry automat și nici persistența CNP-ului/răspunsului în SQLite.
