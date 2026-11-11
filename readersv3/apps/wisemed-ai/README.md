# wisemed-ai

Utilitar WiseMED Lab Readers pentru documentare clinică, implementat în Go + PostgreSQL și Vue 3 / TypeScript / PrimeVue / TipTap. Interfața compilată este inclusă în executabil; Node nu este necesar în producție. Standalone funcționează fără WiseMED, WSM sau OpenAI.

## Pachete de servicii (1.2.0)

WiseMED AI Dashboard include super admin (-1), admin (0), utilizator normal, Free implicit și două niveluri plătite configurabile. Limitele, tarifele suplimentelor, creditul configurabil, alocarea manuală și consumul AI pe utilizator/șablon sunt descrise în [MONETIZATION.md](docs/MONETIZATION.md). Abonamentele, facturile și EuPlătesc rămân etapa următoare; nu se fac încasări sau avansuri de credit acum.

## Pornire locală

Din `readersv3/`:

```sh
cd apps/wisemed-ai/web
npm ci
npm run build
cd ../../..
./scripts/prepare-reader-output.sh wisemed-ai
```

Configurați `WISEMED_AI_DATABASE_URL` cu un DSN PostgreSQL URL-encoded sau `modules.wisemed-ai.database_url` în **configurația locală** `output/wisemed-ai/deployments/config.yaml`. Exemplul distribuit nu conține credențiale. Pentru reutilizarea conexiunii utilitarului SIUI Data:

```sh
go run ./apps/wisemed-ai/tools/import-postgres \
  ./output/wisemed-siui-data/deployments/config.yaml \
  ./output/wisemed-ai/deployments/config.yaml
cd output/wisemed-ai
./wisemed-ai -config deployments/config.yaml -showlog
```

Utilitarul de import copiază exclusiv conexiunea PostgreSQL, nu identități, API keys sau configurații WSM. Fișierul rezultat are permisiuni `0600`. Nu modificați instalarea SIUI sursă. Configurația locală existentă este păstrată la rebuild/update.

WiseMED AI Dashboard: **http://127.0.0.1:19120/** (redirecționează la `/ai/`). Administrarea comună a utilitarului este la `/settings/reader`, iar WSM/WSS la `/settings/wss`. Meniurile și statisticile de laborator nu sunt folosite de WiseMED AI. Health: `/health/ai`; readiness PostgreSQL: `/ready/ai`.

Dashboardul AI păstrează autentificarea standalone sau WiseMED opțională, conform configurării. Consola comună de administrare folosește autentificarea administrativă WiseMED API, fără bootstrap/înregistrare de analizor; identitatea WSM deja configurată este păstrată. După actualizarea binarului reporniți procesul; un proces deja pornit din IDE continuă să ruleze versiunea încărcată anterior.

La login, **Tip de autentificare** separă contul WiseMED AI (email) de contul WiseMED (nume de utilizator, fără obligația caracterului `@`). Dacă loginul WiseMED este dezactivat, formularul explică motivul și nu trimite credențiale prin providerul greșit. Administratorul îl activează în Configurare → WiseMED API; aplicarea necesită repornire. Pentru acces WiseMED sunt necesare codul organizației AI și o identitate verificată `wisemed:<user_id>` asociată explicit utilizatorului în **Utilizatori**. Nu se creează conturi sau drepturi automat pe baza unui login extern.

La prima pornire sunt create tabelele `wai_`, organizația demonstrativă și contul `admin@wisemed.local`. Parola inițială aleatoare se găsește în `deployments/ai-bootstrap-password` (nu este afișată în loguri). Folosiți conținutul fișierului ca parolă, apoi schimbați-o din **Contul meu**. Fișierul reprezintă parola inițială, nu parola nouă după schimbare.

Primul cont este administrator de instanță. Conturile cu rol `admin` create din interfață administrează organizația și membrii; nu primesc drepturi de configurare globală. Rolurile `doctor` și `viewer` au acces doar la propriile sesiuni. Memoria este privată per medic și organizație. Dezactivarea unei apartenențe nu dezactivează apartenențele utilizatorului în alte organizații.

## Biblioteca medicală

La pornire sunt instalate idempotent **52 de șabloane pentru 46 de specialități**, inclusiv ecografie abdominală, neurologie, cardiologie, medicină internă, medicină de familie, pediatrie, geriatrie, psihiatrie, endocrinologie, diabetologie, pneumologie, gastroenterologie, nefrologie, urologie, obstetrică-ginecologie, dermatologie, ORL, oftalmologie, reumatologie, ortopedie, specialități chirurgicale, oncologie, hematologie, boli infecțioase, alergologie, recuperare, medicina muncii, urgență, ATI, neonatologie, imagistică și medicină dentară.

Lista completă și câmpurile se află în [SPECIALTIES.md](docs/SPECIALTIES.md) și [specialty-templates.json](docs/fixtures/specialty-templates.json). Catalogul poate fi regenerat fără PostgreSQL:

```sh
go run ./apps/wisemed-ai/tools/catalog > apps/wisemed-ai/docs/fixtures/specialty-templates.json
```

Câmpurile sunt inițial goale. Fragmentele normal/patologic sunt opționale, introduse explicit de operator în editor. Lipsa examinării nu produce constatări normale. Publicarea creează o versiune nouă, imuabilă. Sesiunile păstrează versiunea selectată la creare. Copierea produce un șablon independent.

Editorul oferă noduri placeholder reale, câmpuri vizibile/invizibile, tipuri, unități, aliasuri, limite numerice, semantic tags, secțiuni și reguli declarative. Secțiunile sunt ierarhice; atributele HTML `data-section` controlează vizibilitatea condițională. Câmpurile repetabile produc liste de observații documentate (de exemplu formațiuni). Configurațiile avansate sunt editabile în panoul JSON validat. Variabile globale: `{{patient_ref}}`, `{{doctor_name}}`. Propunerile dintr-un raport existent folosesc providerul configurat; în modul mock sunt extrase etichetele liniilor `câmp: valoare`. Propunerile nu sunt aplicate sau publicate automat.

Acestea sunt șabloane inițiale de documentare, personalizabile și supuse revizuirii clinice locale; nu reprezintă protocoale de diagnostic sau tratament.

## Flux clinic

1. Selectați șablonul și introduceți o referință minimă de pacient. Contextul standalone este declarat de medic; nu reprezintă validare automată a accesului într-un HIS.
2. Trimiteți textul obținut prin STT pe telefon sau introduceți text sintetic. Modul mock acceptă câte o linie `cheie: valoare`; butonul „Exemplu sintetic” oferă un exemplu.
3. Fiecare segment are UUID, secvență, revizie, indicator final și timestamp. Duplicate identice sunt idempotente, reviziile vechi sunt ignorate, conflictele sunt respinse, iar corecțiile recalculează rezultatul din toate segmentele curente ordonate. Timestampurile sunt normalizate la microsecunde UTC, conform PostgreSQL.
4. Revizuiți HTML-ul, JSON-ul, câmpurile lipsă și citatele exacte. Valorile neacoperite de citate sunt respinse. Se păstrează stările `present`, `unknown`, `negated`, `not_examined`; un câmp fără informație lipsește din date. Dovezile includ segmentul, revizia și secvența.
5. Finalizați după ce toate segmentele sunt finale. Aprobarea cere confirmare explicită, versiunea curentă, câmpurile obligatorii și validări trecute. Rezultatul clinic aprobat este creat în aceeași tranzacție cu aprobarea și auditul. Transcrierea și schițele sunt persistate criptat înainte de aprobare pentru reconectare; nu sunt rezultate aprobate și nu sunt exportate automat în HIS.
6. După aprobare puteți descărca JSON sau solicita explicit livrarea către un conector configurat. Outbox-ul folosește cheie de idempotency stabilă și maximum cinci încercări cu backoff; statusul și erorile sunt vizibile la Conectori. Receptorul trebuie să respecte `Idempotency-Key`.

SSE `/v1/sessions/{id}/events` transmite numai metadate start/progress/status. Conexiunea este limitată la aproximativ 55 secunde; clientul reconectează cu `Last-Event-ID` și citește rezultatul curent. Snapshotul complet validat este patch-ul de înlocuire; aplicați datele numai pentru revizii mai noi și reconciliați separat schimbările de statut semnalate de ID-ul evenimentului. QR-ul conține un token opac one-time, 120 secunde; revendicarea cere același medic și tenant autentificat, fără a conferi acces suplimentar.

„Informare” este exclusiv on demand. Fără un HIS cu acces verificat configurat, răspunde explicit că nu există istoric autorizat atașat; nu inventează un istoric și nu caută automat în memoria medicului.

## API, OpenAI, WSM și configurare

[OpenAPI 3.1](docs/openapi.json), [client Dart pentru Flutter](clients/flutter/lib/wisemed_ai_client.dart), [fixture transcriere](docs/fixtures/abdominal-chunk.json) și [audit integrare](docs/AUDIT.md).

În **Configurare**, administratorul instanței poate salva providerul/modelul, cheia OpenAI write-only, tarifele de calcul, bugetul, retenția, conexiunea PostgreSQL, setările WiseMED API, WSM și actualizări. Cheile goale păstrează valoarea existentă. Secretele nu sunt returnate prin API. Setările persistate care schimbă infrastructura sau modelele cer restart, indicat în UI. Configurația WSM folosește validarea și reconectarea clientului comun. Nu activați cookie Secure până când accesați aplicația prin HTTPS.

OpenAI utilizează HTTPS backend, `store:false` și Structured Outputs cu JSON Schema strict, pe contractul [oficial](https://developers.openai.com/api/docs/guides/structured-outputs?api-mode=chat). Cheia vine din `OPENAI_API_KEY` sau din fișierul protejat `ai-openai.key`. Nu există chei în frontend. Modelul este configurabil; tarifele implicite sunt estimări configurabile de operator, nu un tarif garantat. Verificarea bugetului este serializată per tenant și rezervă conservator intrarea și ieșirea maximă; usage este contabilizat chiar dacă validarea ulterioară eșuează. O rezervare persistentă precedă apelul; la timeout sau stare necunoscută rămâne în buget, vizibilă separat în Consum și costuri, până la reconcilierea facturării de către operator. Nu se reîncearcă automat o cerere OpenAI facturabilă cu stare necunoscută.

WiseMED login folosește clientul real `PUT /administrative/login`, protocolul JWT existent și mapping de identitate `wisemed:<user_id>`. Trebuie completată configurarea reală cerută de clientul comun. Tokenul WiseMED nu devine token AI. Nu sunt inventate endpointuri pentru autorizarea contextului pacientului; un context HIS de lansare trebuie integrat doar după existența contractului verificabil al HIS.

WSM este dezactivat implicit. Se folosesc WSS, cheie de echipament/fișier JWT/endpoint JWT, tenant, issuer, audience și permisiunile efectiv provisionate pe server. Clientul comun cere înregistrarea reală și ID-ul WiseMED înainte de WSS. API-ul JSON are alias `/api/wisemed-ai/v1/...`, accesibil prin `api.request`; sunt necesare `api:invoke` și mapping `external_subject` în tenantul AI. Nici `api:admin`, nici un header HTTP nu conferă automat acces la date clinice. `wsm_identity_enabled` trebuie activat explicit. Evenimentele `wisemed-ai.start/progress/result` sunt trimise numai conexiunii WSM atestate a apelantului, cu ID instanță/sesiune și revizie, fără PHI. În HTTP direct se utilizează SSE. Nu există broadcast de rezultate clinice.

Conectorii HIS/Odoo sunt adaptoare HTTPS generice, opționale, cu bearer token criptat și gazde allowlisted în `connector_allowed_hosts` (`host[:port]` exact). Redirecturile nu sunt urmate. Mapările semantice ale șablonului apar în `canonical_data`. Mapările tehnice sunt salvate separat la Conectori → connector-mappings, sub forma `{"fields":{"external_field":"local_key"}}`; conectorul poate indica `mapping_id` pentru a livra `mapped_data` în locul snapshotului complet. Câmpurile lipsă nu primesc valori implicite. Nu există endpointuri fabricate de Odoo/HIS, scăderi de stoc sau prescripții automate.

Memoria aprobată este administrată separat în preferințe, exemple și feedback. Căutarea textuală privată funcționează fără extensii. Pentru recuperare semantică selectivă, cu pgvector instalat, API-ul permite atașarea explicită a unui embedding de 1536 dimensiuni la un exemplu propriu și căutare cosine prin `/v1/memory/search`. Embedding-urile sunt furnizate de clientul autorizat; nu se generează automat din istoricul clinic, iar exemplele nu sunt injectate automat ca observații ale pacientului.

## Docker și instalare

Din `readersv3/apps/wisemed-ai/docker/`, creați `.env` după `../.env.example`, cu o parolă hex aleatoare (`openssl rand -hex 24`), apoi:

```sh
docker compose up -d --build --wait
```

Sau utilizați scriptul din rădăcina repository-ului `./make-wisemed-ai-docker.sh`. Compose pornește PostgreSQL 16 + pgvector și aplicația fără privilegii root (UID 10001), cu volume persistente. Numai portul aplicației este expus implicit pe loopback. Extensia vector este creată automat exclusiv în baza Docker dedicată; în baza comună existentă se verifică extensia fără instalare sau modificarea tabelelor străine.

Build/installer/update folosesc infrastructura comună:

```sh
go run ./tools/releasectl sync-app wisemed-ai
go run ./tools/releasectl release --app wisemed-ai --target darwin-arm64 --version 1.2.0
go run ./tools/releasectl release --app wisemed-ai --target windows-amd64 --version 1.2.0
go run ./tools/releasectl release --app wisemed-ai --target linux-amd64 --version 1.2.0
# Installere native macOS/Linux se generează separat:
go run ./tools/releasectl package --app wisemed-ai --target darwin-arm64 --version 1.2.0
go run ./tools/releasectl package --app wisemed-ai --target linux-amd64 --version 1.2.0
```

Dacă NSIS nativ eșuează la self-test pe macOS, `./apps/wisemed-ai/docker/build-windows-release.sh 1.2.0` folosește NSIS Linux în Docker, păstrând aceeași infrastructură de release și generând un Setup.exe real.

Arhiva Linux se extrage într-un director de lucru, apoi se rulează `bash ./install.sh`; include și `uninstall.sh`, care păstrează datele și secretele.

Sunt incluse scripturi Windows/NSIS/WinSW, Linux/systemd și macOS/launchd, icon PNG și ICO multi-rezoluție. Pe Linux serviciul rulează ca utilizator dedicat `wisemed-ai`; în Windows/macOS se aplică identitatea de serviciu configurată de operator. `-installservice` instalează serviciul platformei; `-headless` pornește în fundal; `-version` afișează versiunea embedded. Nu folosiți `-headless` în spatele unui supervisor care urmărește PID-ul procesului principal.

Actualizările sunt activate implicit, `app_id: wisemed-ai`, canal stable, URL comun și auto_download true. Alegerile explicite existente sunt păstrate. Verificarea reală a update-serverului cere cheia WiseMED provisionată. Pachetele nu includ configurații active, parole, chei de criptare, transcrieri sau baze de date.

## Stocare și operare

Toate tabelele și indexurile proprii au prefix `wai_`. Migrațiile SQL sunt versionate, tranzacționale și serializate prin advisory lock. Relațiile clinice verifică tenantul și referințele. Identificatorii sunt UUID. Datele clinice, rezultatele, memoria și configurațiile conectorilor sunt criptate AES-256-GCM cu AAD tenant/entitate. Identitățile, denumirile șabloanelor, auditul fără PHI și usage sunt metadate operaționale.

Păstrați în backup securizat atât PostgreSQL, cât și `deployments/ai-encryption.key`. Pierderea cheii face datele criptate irecuperabile. Nu copiați cheia în Git sau în arhive de update. Parolele sunt bcrypt; tokenurile de sesiune și pairing sunt stocate doar ca SHA-256. Cookie-ul este HttpOnly/SameSite Strict; mutațiile cross-origin sunt respinse. Autentificarea este limitată la 15 încercări/IP/10 minute. Limite: 1 MiB JSON, 16 KB/segment, 100 KB și 500 segmente/sesiune, timeout cerere 55s, OpenAI 45s, limite de conexiuni PostgreSQL și shutdown prin runtime-ul comun.

Retenția clinică implicită este 30 zile, verificată periodic. Ștergerea unei sesiuni elimină prin FK segmentele, reviziile, extragerile, rezultatele, aprobările și livrările asociate. Auditul operațional fără transcrieri și memoria explicit aprobată au cicluri separate; memoria poate fi exportată/listată și ștearsă de proprietar. Nu există training implicit pe date și nu sunt logate transcrieri, referințe pacient sau secrete de acest modul.

## Verificare

```sh
# din readersv3/
go test ./...
go vet ./...
WAI_TEST_CONFIG="$PWD/output/wisemed-ai/deployments/config.yaml" go test -race -v ./modules/wisemedai
cd apps/wisemed-ai/web
npm ci
npm run build
npm test
npx playwright install chromium
npm run test:e2e
```

Testele PostgreSQL creează o schemă temporară `wai_test_<uuid>` și o elimină la final; nu șterg datele instalației. Alternativ utilizați `WAI_TEST_DATABASE_URL`. E2E folosește instanța locală, parola inițială protejată și exclusiv referințe sintetice; `WAI_BASE_URL` și `WAI_PASSWORD_FILE` permit o instanță dedicată. Nu rulați E2E cu date de pacient reale.

Rezultatele efective și limitele verificării sunt în [VALIDATION.md](docs/VALIDATION.md). Apelurile reale OpenAI/WiseMED/HIS, Flutter pe dispozitiv, instalarea ca serviciu pe fiecare sistem și acceptarea clinică cer mediile/identitățile respective; testele cu mock nu demonstrează acele integrări de producție.


## Abonamente, credit și context compact (1.2.0)

Abonamentele plătite se activează cu acceptare explicită și se reînnoiesc lunar în UTC. Soldul este vizibil în cele cinci zile de grație, apoi restanțele suspendă lucrul. Consumul suplimentar AI folosește limita de credit, cu rezervare concurentă și blocare la epuizare. Plata verificată se poate înregistra de super admin, cu idempotency și reactivare automată. Documentele sunt evidențe interne de plată, nu facturi fiscale. EuPlătesc, debitarea cardului și schimbările/anularea abonamentului nu sunt încă implementate. Vezi [regulile complete de abonament și credit](docs/SUBSCRIPTIONS.md).

Definițiile compacte, parserul local și regăsirea autorizată a definițiilor reduc contextul și apelurile inutile. Statisticile păstrează tokenurile efective și cele din cache, separat de rezervări, pe generare și versiune de șablon. Vezi [optimizarea contextului](docs/CONTEXT_OPTIMIZATION.md).


## Șabloane generale și catalog adaptat (1.3.0)

Toate52 de exemple au secțiuni și subsecțiuni generale, formulări scurte/standard/detaliate și condiții bazate pe date documentate. Medicul poate alege stilul general și excepții pe secțiuni; schimbarea stilului refolosește faptele validate și creează o revizie nouă, fără alt apel AI. Versiunile aprobate sunt blocate. Upgrade-ul păstrează versiunile vechi și șabloanele personalizate. Vezi [modelul general](docs/SECTION_MODEL.md).

Specificația mobilă este în proiectul separat `/Users/raduichim/work/flutter/wisemed-ai/WisemedAIFlutter.md`. Contractul comun actualizat rămâne [OpenAPI](docs/openapi.json). Pentru reluarea dezvoltării și starea verificărilor, citește [checkpointul](docs/WORK_STATUS.md).
