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
pentru trasabilitate; numai `validateReport` este expus de acest utilitar.

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
