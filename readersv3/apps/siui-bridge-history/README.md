# SIUI Bridge History

Utilitar Go separat pentru arhivarea asincronă a apelurilor CNAS efectuate de SIUI Bridge.
Folosește runtime-ul comun WiseMED: serviciu Windows/macOS/Linux, HTTPS, autentificare,
loguri, configurare/reconfigurare, WSM și actualizări prin update server.

## Pornire

```sh
cd readersv3
bash scripts/prepare-reader-output.sh siui-bridge-history
cd output/siui-bridge-history
./siui-bridge-history -config deployments/config.yaml -showlog
```

Interfață implicită: `https://localhost:19113/settings/history`.
Autentificarea și configurarea WiseMED/WSS folosesc paginile comune ale utilitarelor.
Completează inițializarea WiseMED (unitate medicală, tip și identificarea echipamentului).
History se înregistrează prin API-ul WiseMED și salvează `echipament_id` și cheia echipamentului
în `modules.wisemed-api`; acest ID permite generarea/asocierea cheii WSM.
Dacă ai configurat versiunea inițială fără echipament, reia inițializarea din pagina de
autentificare sau pornește cu `-reconfigure`, apoi configurează conexiunea WSS.
În **Setări → History**, configurează organizația (`tenant`), MySQL și stocarea.
Serverul MySQL trebuie să aibă deja baza creată; History creează cele trei tabele proprii.
Exemplu administrativ SQL, de adaptat și rulat de administratorul DB:

```sql
CREATE DATABASE siui_history CHARACTER SET utf8mb4 COLLATE utf8mb4_bin;
CREATE USER 'siui_history'@'%' IDENTIFIED BY 'replace-with-strong-password';
GRANT SELECT, INSERT, UPDATE, CREATE, INDEX ON siui_history.* TO 'siui_history'@'%';
```

Restrânge hostul utilizatorului la rețeaua serverului History în configurația finală.
MySQL poate rula în rețeaua privată; numai API-ul History trebuie accesibil bridge-urilor.
MySQL TLS `true` validează certificatul; `false` permite conectarea fără TLS în rețeaua configurată.

1. Salvează setările, apoi **Testează MySQL**.
2. Selectează **local** și calea de bază sau **s3**, endpoint, bucket, regiune,
   prefix, access key, secret key și eventual session token. Bucketul există deja.
3. **Testează scrierea și ștergerea**: rezultatul conține separat `write`, `deleted`
   și `delete_error`. Lipsa dreptului de ștergere nu transformă scrierea reușită în eșec.
4. Introdu un ID unic pentru fiecare bridge și apasă **Emite cheie Bridge**.
   Copiază cheia afișată o singură dată; History păstrează numai hashul ei.
5. În fiecare SIUI Bridge: **Setări → History**, activează sincronizarea, configurează
   URL-ul HTTPS al History și cheia emisă. Pentru o CA privată, indică un fișier PEM
   în `ca_file`, accesibil contului sub care rulează serviciul. Verificarea TLS rămâne activă.
6. Salvează și apasă **Testează conexiunea**. **Starea cozii** arată pending/archived și
   ultima eroare; **Reîncearcă acum** devansează următoarea încercare.

Setările se salvează în `deployments/config.yaml`; cele History se aplică imediat.
Câmpurile secrete goale păstrează valoarea salvată. Configurația cu parole se scrie cu
permisiuni 0600 pe sisteme POSIX. Accesul la setări, interogările globale și emiterea/revocarea
cheilor necesită administrator. Cheia unui bridge permite numai propriile apeluri.
Mai multe bridge-uri ale aceleiași organizații folosesc același `tenant` și ID-uri distincte.

## Cum funcționează

Bridge salvează cererea în outbox SQLite în aceeași tranzacție cu rezervarea operației.
Nu așteaptă rețeaua History în fluxul CNAS. Un worker separat comprimă în gzip cererea
și rezultatul, apoi trimite manifestul și fișierele în fragmente de 256 KiB.
Fișierele ZIP CNAS sunt păstrate ca atare: nu sunt dezarhivate sau convertite automat în JSON.
Persistența locală are un cost de disc, iar arhivarea consumă I/O în fundal; nu promite latență zero.

History verifică dimensiunile și SHA-256, salvează originale + manifest, indexează detaliile,
apoi confirmă `archived`. Bridge șterge numai copia temporară de arhivare după confirmare;
fișierele originale deja folosite de API-ul SIUI rămân disponibile.
O retransmitere cu aceeași cheie `(tenant, bridge, call_id)` și același manifest este idempotentă;
un conținut diferit produce HTTP 409.
După întrerupere se reia de la offsetul persistat. Reîncercările folosesc backoff până la o oră.
Nu se repetă niciodată apelul CNAS pentru a reface arhivarea.

Sunt arhivate apelurile operațiilor catalogului SIUI, validările legacy `/api/siui/validations`
și apelurile legacy `/insured` (mod CNAS) și `/personalization`,
create după instalarea acestei versiuni, inclusiv cele eșuate/necunoscute. Nu se reconstruiesc
retroactiv cereri vechi și nu se arhivează PIN-uri, credențiale, login-uri, tokenurile OCSP sau `lt`.
Testele locale simulate, testele administrative OCSP, gestionarea certificatelor și cererile de configurare nu sunt
apeluri de business arhivate. Se păstrează parametrii originali și răspunsurile de business,
nu capturi ale întregului trafic TLS sau ale tuturor headerelor SOAP.

Coada persistă și când sincronizarea este dezactivată; activarea ulterioară livrează apelurile acumulate.
`created_at`, `started_at`, `completed_at` vin de la Bridge, iar `received_at` de la History,
în format ISO 8601. La oprirea brutală a procesului, operațiile neterminate sunt `unknown`;
pentru acestea timpul final reprezintă constatarea la repornire, nu un timp CNAS inventat.
`source_ip` este adresa transportului văzut de Bridge; prin WSM este identificatorul conexiunii,
nu adresa IP publică a browserului. `owner` este identitatea deja autentificată în Bridge.

Arhivele sunt organizate astfel (zi UTC a creării operației):

```text
<base sau prefix S3>/<tenant>/YYYY/MM/DD/<bridge>/<call_id>/
  manifest.json
  record-gzip         # JSON cu parameters/result/error, comprimat gzip
  download            # fișier CNAS original, dacă operația catalogului a descărcat unul
```

Pentru ruta legacy `/personalization`, fișierul mic este păstrat în `record-gzip`,
în `result.legacy_file.data_base64`, exact în formatul returnat de acea rută.

Locația fiecărei arhive se păstrează separat în DB. Schimbarea destinației pentru operațiile
noi nu redirecționează citirea arhivelor vechi. Pentru arhive S3 vechi, credențialele curente
trebuie să aibă în continuare acces la bucketul vechi. Mutarea fizică a datelor nu este automată.
Un Bridge care a început livrarea către o identitate History nu o redirecționează automat
către alt URL/tenant/bridge; rotația cheii pentru aceeași identitate este permisă.

## Date și căutare

- `siui_history_calls`: identificatori, operație, timpi, stare de arhivare și manifest.
- `siui_history_details`: detalii extrase, PID/CID indexabile și eroare de parsare.
- `siui_history_storage`: locația originalelor, fără credențiale S3.

Pentru `getInsured` și `getInsuredByCID`, History parsează XML-ul original din răspuns:
PID/CID rămân stringuri, `state` este numeric, categoriile și stările sunt păstrate integral.
Pentru celelalte operații indexează deocamdată operația și starea; originale și rezultate sunt
păstrate integral, fără parsere specifice fiecărui nomenclator/decont. O eroare de parsare
nu pierde originalul și nu împiedică arhivarea; apare ca `parse_error`.

## Operare și build

- `-installservice`, `-headless`, `-reconfigure`, `-showlog`, `-version`: mecanismele runtime comune.
- **Setări → Server și actualizări**: update server, stable, descărcare și aplicare.
- **Setări → WSS**: autentificare și conectare; **Debug → WSM**: diagnostic WSM.
- Identitate WSM: `client_type=siui-bridge-history`, `role=service`.
- Scripturi în `build/` pentru Windows, macOS și Linux; pictograme PNG/ICO în resources.
- Nu se șterg automat apeluri/fișiere vechi: dimensionează discul/coada și configurează
  retenția/backupul MySQL separat. S3 nu înlocuiește backupul bazei MySQL.
- Rulează o singură instanță de scriere History pentru aceeași configurație/staging;
  scalarea orizontală cu locking distribuit nu este inclusă.

[Contract API și exemple HTTP/WSM](docs/API.md)

Meniul **Istoric** permite filtrarea după interval de date și ore, Bridge și operație,
cu detalii despre apelant și descărcarea fișierelor arhivate. **Debug → Cereri Bridge**
afișează cererile de transfer și testele de identitate, răspunsurile HTTP și erorile,
chiar dacă baza MySQL este indisponibilă. Jurnalul Debug păstrează ultimele 1000 de
cereri de la pornire; sumarul este scris și în logul serviciului.

Pagina principală este un dashboard al arhivei: spațiu stocat, bridge-uri autorizate
și observate, coada centrală, trafic recent și grafice pentru volum, rezultate SIUI
și distribuție pe Bridge. Perioade: azi, ieri, săptămâna curentă, ultimele 7 zile,
luna curentă și interval personalizat. Activitatea este măsurată la primirea în
History; spațiul și coada sunt totaluri curente, independente de perioada aleasă.
