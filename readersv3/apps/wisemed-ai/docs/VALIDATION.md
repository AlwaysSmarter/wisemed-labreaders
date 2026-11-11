# Verificare — 2026-11-03

Versiune: **wisemed-ai 1.0.0**. Verificare locală pe macOS arm64 și în Docker Linux, numai cu date sintetice.

| Verificare | Rezultat |
| --- | --- |
| `go test ./...` în readersv3 | Trecut pentru toate pachetele. |
| `go vet ./...` în readersv3 | Trecut. |
| `go test -race ./modules/wisemedai` cu PostgreSQL real | Trecut în scheme temporare izolate și eliminate la final. |
| Migrații, seed repetat | Trecut; 52 șabloane / 46 specialități, fără rescrierea versiunilor personalizate. |
| RBAC, tenant, proprietarul sesiunii, mapping WSM | Trecut; headerele falsificate și identitățile din alt tenant sunt respinse; administratorii organizației nu primesc administrarea instanței. |
| Transcript duplicat, out-of-order, revizie veche/corecție | Trecut; precizie timestamp normalizată conform PostgreSQL. |
| Finalizare, aprobarea exactei revizii și idempotency | Trecut; aprobarea prematură, veche sau neconfirmată este respinsă. |
| Criptare și domeniu tenant/entitate, sanitizare HTML, dovezi | Trecut; scripturile/atributele executabile, valori inventate și fragmente numerice incorecte sunt respinse. |
| Range validation, condiții, câmpuri repetabile, mapări tehnice | Trecut; câmpurile inactive nu devin obligatorii; mapările nu inventează valori implicite. |
| QR one-time, expirare logică, reutilizare | Tokenul este limitat la 120s; consumul/reutilizarea sunt testate. |
| OpenAI cu transport mock, Structured Outputs și usage | Trecut; schema strictă, costul independent de rollback și blocarea peste buget sunt verificate. |
| Întrerupere OpenAI cu facturare incertă | Trecut; rezervarea persistentă rămâne în buget. |
| HTTPS conector allowlisted, retry și Idempotency-Key | Trecut cu transport mock, inclusiv eroare temporară urmată de succes și lipsa retrimiterii după succes. |
| Memorie privată, propuneri neaplicate automat, versiuni înghețate | Trecut. |
| Vector: dimensiuni, finitudine, normă nenulă | Trecut; pgvector provisionat și verificat în baza Docker dedicată. |
| `npm run build` / TypeScript | Trecut; frontend împărțit în bundle principal și module editor/configurare încărcate la nevoie. |
| Vitest | 4 teste trecute pentru contract client browser și exemple sintetice conforme șablonului. |
| Playwright / Chromium | Flux clinic și editor, propuneri cu acceptare, configurare, audit, desktop și mobil; trecut atât nativ, cât și pe instanța Docker. |
| Client Dart | `dart analyze` fără probleme; 3 teste trecute. |
| OpenAPI | `openapi-spec-validator`: OK, OpenAPI 3.1.0. |
| Docker Compose app + PostgreSQL/pgvector | Build multi-stage și readiness trecute; aplicația rulează ca UID 10001, fără Node runtime. |
| Binare | Go darwin-arm64, linux-amd64 și windows-amd64, versiune embedded 1.0.0. |
| Instalație macOS | PKG și DMG generate. Nu au fost instalate în LaunchDaemons pe calculatorul utilizatorului. |
| Instalație Linux | Arhivă tar.gz cu executabil, config.install, help, scripturi și systemd; generată. Nu sunt prezente uneltele native DEB/RPM pe host; nu se pretinde generarea acestor două formate. |
| Instalație Windows | Setup.exe NSIS real și update ZIP generate. NSIS de pe Mac are o eroare reproductibilă `std::bad_alloc` chiar la self-test; compilarea a reușit cu NSIS Linux în Docker, prin helperul livrat. |
| Icon | PNG plus ICO real cu rezoluții 16/32/48/64/128/256; folosit și la installerul Windows. |
| Secrete | Scanarea surselor livrabile nu găsește parola PostgreSQL reală. Fișierele locale de conexiune, bootstrap și criptare sunt protejate; nu intră în update. |

Conexiunea PostgreSQL a fost importată din configurația locală SIUI Data exclusiv în `output/wisemed-ai/deployments/config.yaml`, ignorat de Git. Migrațiile instalării locale creează numai obiecte `wai_`. Nu s-a modificat schema operațională WiseMED/SIUI existentă.

Artefactele sunt disponibile în `readersv3/dist/updates`, `readersv3/dist/installers` și `readersv3/release/wisemed-ai`. Manifestele și checksum-urile release-urilor sunt în `readersv3/dist/releases/wisemed-ai`.

Nu au fost efectuate apeluri OpenAI facturabile, login/înregistrare pe un WiseMED real, conectare la WSM de producție, livrare către HIS/Odoo real sau teste pe hardware mobil. Acestea cer chei, mapping-uri și contracte de mediu reale. Flutter este un client API Dart verificat, nu o aplicație mobilă completă cu STT. Recuperarea semantică acceptă embedding-uri furnizate explicit de client; generarea lor automată nu este activată. Nu se declară validare clinică, aprobare de producție sau certificare de conformitate.

Arhivele ZIP au fost inspectate: conțin config.install.yaml și nu includ config.yaml, chei, parole bootstrap, baze de date sau loguri. Arhiva Linux include install.sh/uninstall.sh executabile și serviciul systemd; sintaxa scripturilor a fost verificată cu bash -n. Mediul Docker de smoke test a fost separat de instalarea locală și este curățat după verificare.


## Pachete de servicii — 1.1.0

Verificare la 2026-11-03, pe date sintetice. Migrația aditivă `007_monetization.sql` a fost aplicată și pe instalarea locală; configurația PostgreSQL și fișierele secrete existente au fost păstrate.

- `go test ./...` și `go vet ./...`: trecute.
- Suita completă `go test -race ./modules/wisemedai -v -timeout 120s`, cu PostgreSQL real și scheme temporare: trecută, inclusiv cele trei teste noi pentru monetizare.
- Verificate Free implicit, drafturi Pro/Premium, izolarea rolurilor -1/0/1, protecția împotriva escaladării, proprietatea șabloanelor, limita de specialități/șabloane/câmpuri și versiunile comerciale păstrate la alocare.
- Verificate rezervări concurente de tokenuri, eliberarea diferenței față de usage-ul efectiv și refuzul depășirii alocării chiar dacă există credit configurat. Testul fluxului unui medic păstrează tranzacția clinică deschisă, înregistrează independent usage-ul exact și verifică faptul că un fragment duplicat produce un singur apel provider.
- `npm run build`: trecut, inclusiv TypeScript. Vitest: 7 teste trecute, inclusiv conversia exactă a sumelor monetare.
- Playwright/Chromium: 2 scenarii trecute pe executabilul nativ 1.1.0, pentru catalog/limite/tarife, lista utilizatorilor, pachetul propriu, fluxul clinic și afișarea mobilă. Captura catalogului a fost inspectată vizual.
- OpenAPI 3.1.0: validator trecut după adăugarea contractelor billing.
- Release standard 1.1.0 pentru darwin-arm64, linux-amd64 și windows-amd64: ZIP-uri și metadate generate, checksum-uri verificate. Payload-urile nu conțin configurația runtime sau secrete.
- PKG/DMG macOS, arhiva Linux cu serviciu și scripturi executabile, respectiv Setup.exe Windows NSIS: generate local. Sintaxa scripturilor Linux a fost verificată. Acest pas nu instalează servicii pe sistem și nu execută installerul Windows.
- Executabilul local a fost înlocuit și repornit, raportează 1.1.0 și servește dashboardul la `http://127.0.0.1:19120/ai/`. Niciun artefact nu a fost publicat pe update-server.

Pachetele configurează oferta comercială. Abonamentele, facturile, cumpărarea suplimentelor, cheltuirea creditului și integrarea EuPlătesc sunt etapa următoare; nu sunt declarate plăți reale sau teste de încasare. Detaliile funcționale sunt în [MONETIZATION.md](MONETIZATION.md).


## Abonamente, credit și context per șablon — 1.2.0

- `go test ./...` și `go vet ./...`: trecute după modificările finale.
- Suita PostgreSQL completă cu `-race`: trecută (56,551 s), în scheme temporare. Sunt verificate limita exactă de cinci zile, reînnoiri fără duplicate, plăți parțiale/idempotente, blocare și reactivare, imposibilitatea confirmării plății de către utilizator, rezervări concurente de credit și păstrarea rezervărilor la schimbarea lunii.
- Contextul fiecărui șablon este verificat pentru criptare, izolare între utilizatori, versiuni înghețate, limite de dimensiune și încărcarea efectivă în cererea providerului. Datele din exemple nu înlocuiesc validarea dovezilor clinice.
- Compilarea păstrează textul clinic și toate câmpurile; fixture sintetic 562 → 212 octeți. Parserul local funcționează fără cheie OpenAI și înregistrează zero tokenuri facturabile. Receipt-ul de generare se leagă numai de un șablon publicat de autorul autorizat. Usage-ul de cache raportat de provider este verificat cu transport sintetic.
- Build Vue/TypeScript și cele 7 teste Vitest: trecute. OpenAPI 3.1.0: validator trecut, inclusiv contractele de abonamente și context.
- Playwright nativ: catalogul și fluxul clinic/editorul cu context persistent au trecut, inclusiv mobil. Scenariul cu plăți se omite intenționat pe baza runtime; separat, pe o schemă PostgreSQL temporară, catalogul și recuperarea unui utilizator suspendat prin plată verificată au trecut (2 teste). Captura stării suspendate a fost inspectată vizual după terminarea animațiilor.
- Executabilul nativ 1.2.0 rulează local cu migrațiile 008–010; conexiunea și secretele existente sunt păstrate. Nu s-au creat abonamente sau plăți de test în schema runtime. Testul clinic nativ folosește numai date sintetice și șterge sesiunea creată.

Limite de livrare: plățile sunt înregistrări administrative ale încasărilor verificate extern. Nu sunt implementate EuPlătesc live, debitarea cardului, facturi fiscale/e-Factura, rambursări, proratări, schimbarea ori anularea abonamentului din UI. Contextul extern nu este context gratuit pentru model. Regăsirea definițiilor este locală și lexicală; nu se declară un RAG semantic extern sau economii garantate în tokenuri/bani. Nu au fost efectuate apeluri OpenAI facturabile.

Livrabilele finale 1.2.0 sunt generate local pentru macOS arm64, Linux amd64 și Windows amd64: update ZIP + metadate, PKG/DMG, tar.gz cu scripturi/serviciu și Setup.exe NSIS real. Checksum-urile din metadate și payload-urile fără secrete au fost verificate. Executabilul nativ care rulează este identic cu cel din update-ul macOS. Nu s-au instalat servicii și nu s-a publicat pe update-server.


## Model general și adaptarea completă a catalogului —1.3.0

Toate52 de exemple /46 de specialități sunt adaptate și verificate în runtime:1028 de secțiuni și5124 de variante demonstrative. Sunt variante inițiale de redactare, nu o bibliotecă validată de patologii.

- Go: suita PostgreSQL completă cu race trecută (84,691s), inclusiv versiuni și upgrade idempotent fără rescrierea personalizărilor. `go test ./...` și `go vet ./...` trecute. Verificările țintite ulterioare pentru work_context/idempotency, câmpuri invizibile și revizii de prezentare au trecut.
- Validare: lipsa datelor nu activează normalitatea; câmpurile obligatorii rămân verificate chiar dacă secțiunea goală este ascunsă; variantele insuficient susținute/ambigue păstrează forma de bază; subsecțiunile sunt păstrate; datele necunoscute nu sunt ascunse de variante Standard/Detaliat; câmpurile invizibile nu pot fi introduse în formulări.
- Vue/TypeScript build și7 teste Vitest: trecute. OpenAPI3.1 final: validat.
- Playwright nativ:2 scenarii trecute, catalog și flux clinic complet cu stil Scurt general /Ficat Detaliat, revizie nouă, revizuire/aprobare, editor cu context și aspect mobil. Scenariul financiar nu este rulat pe runtime; separat,2 scenarii de catalog/recuperare după plată au trecut pe schema temporară.
- Captura raportului mixt a fost inspectată vizual. Sesiunea reușită este ștearsă de test; trei sesiuni sintetice rămase din verificările UI eșuate au fost eliminate explicit, fără alte ștergeri.
- Livrabile1.3 macOSarm64/Linuxamd64/Windowsamd64: ZIP-uri +metadate și instalatoare locale (PKG/DMG, tar.gz cu serviciu/scripturi, Setup.exe NSIS). Checksum-uri și payload fără runtime/secrete verificate. Binarul local1.3 este identic cu update-ul macOS. Nu s-a publicat nimic pe update-server și nu s-au instalat servicii în sistem.

Flutter este în proiectul separat; nu confunda aceste verificări backend/Vue cu validarea STT pe dispozitive mobile. Urmărește checkpointul de lucru și validarea proiectului Flutter pentru rezultatele sale reale.

## Corecție utilitar / pagina principală — 1.3.1

Pagina `/` deschide acum `/ai/`; linkurile vechi de laborator se redirecționează la AI sau la setările utilitarului. Consola comună păstrează numai server/actualizări și WSM/WSS, fără meniuri, câmpuri, indicatori sau apeluri API de laborator. WiseMED API este folosit administrativ, fără înregistrare de analizor, păstrând identitatea WSM configurată.

- `go test ./modules/localhttp ./modules/wisemedapi ./apps/runner` și `go vet` pentru aceleași pachete: trecute. Regresii pentru rutele AI și păstrarea rutelor cititoarelor ASTM, plus lipsa înregistrării medicale pentru AI.
- JavaScript UI comun:61 teste trecute,12 omise explicit;4 regresii noi AI pentru navigație, mod detectat prin protocol/ID, montare fără apeluri de laborator și polling.
- Playwright pe binarul nativ1.3.1:2 teste trecute. Redirecționare reală `/`→`/ai/`; setări după o sesiune administrativă sintetică, fără autentificare externă și fără modificări de configurare. Verifică meniurile ascunse, actualizările vizibile, schimbarea limbii, WSM și absența apelurilor API de laborator. Captura WSM a fost inspectată vizual. Acest test nu pretinde autentificare reală la un server WiseMED extern.
- Runtime local1.3.1 pe19120: readiness PostgreSQL200, root307 spre `/ai/`. Executabilul local și cel folosit de IDE sunt actualizate; procesul deja pornit în debugger trebuie repornit pentru a încărca noul cod.
- Pachete1.3.1 macOSarm64/Linuxamd64/Windowsamd64 regenerate: update ZIP-uri/metadate, PKG/DMG, tar.gz și Setup.exe NSIS real. Checksum-urile, binarele incluse și lipsa config.yaml/databases/logurilor/fișierelor cu secrete din update au fost verificate. Nu s-a publicat nimic și nu s-au instalat servicii.

## Alegerea autentificării și acces administrator — 1.3.2

Formularul afișează explicit Tip de autentificare: cont AI prin email sau cont WiseMED prin nume de utilizator. Providerul WiseMED dezactivat este explicat și blochează trimiterea prin acel provider, în loc să ascundă opțiunea. În runtime-ul local, loginul WiseMED a fost activat la cererea utilizatorului; installerul păstrează opțiunea implicită configurabilă. Asocierea verificată external_subject/tenant și autorizarea nu au fost schimbate.

- Vue/TypeScript build trecut;7 teste Vitest trecute.
-5 teste Playwright trecute pe binarul1.3.2: username WiseMED `medic.test` fără @ ajunge la endpoint cu wisemed=true și tenantul explicit (API simulat); provider dezactivat afișează motivul; contul bootstrap local se autentifică efectiv și vede rolul Super admin, apoi sesiunea este revocată; cele2 regresii de utilitar trec. Nu se pretinde validarea credențialelor la un server WiseMED extern.
- Credentialele bootstrap existente au fost verificate și prin API local; numai identitatea/rolul au fost afișate. Parola nu a fost copiată în surse, loguri sau documentație; fișierul protejat existent este păstrat, fără resetare.
- Runtime pe19120 și binarul de IDE sunt1.3.2; auth/providers confirmă standalone=true și wisemed=true. Instanța IDE deja pornită trebuie repornită. Pachetele1.3.2 și checksum-urile sunt generate local, fără publicare.
