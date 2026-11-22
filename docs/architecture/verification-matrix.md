# Matricea de verificare

| | |
|---|---|
| **Scop** | Inventarul verificărilor EXECUTATE în această investigație (comandă exactă, director, rezultat cu cifre, ce demonstrează și ce NU demonstrează), al eșecurilor preexistente, al verificărilor neexecutate, al acoperirii cu teste pe zone și al testelor sărite. Include verificările făcute de orchestrator prin lectură încrucișată a codului. |
| **Data** | 2026-11-14 (commit de bază `d53088e`, branch `main`, cu modificările locale din `git status`) |
| **Surse** | [`_reports/06-qa-build-ops.md`](_reports/06-qa-build-ops.md) partea B (singura sursă de execuții); outputurile salvate ale acelor rulări (citite, nu regenerate); verificările orchestratorului prin lectură directă; rapoartele 01–05 pentru inventarul de teste existente. |
| **Limite** | Nimic nu a fost rulat din nou la redactarea acestui document. Rulările au avut loc pe o singură mașină: macOS darwin/arm64 (Darwin 23.5.0), `go1.25.5`, `node v24.10.0`, Python 3.14 (Homebrew), `/bin/bash` 3.2.57, cu `GOPROXY=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly`. |

**Etichete**: **[COD]** = `CONFIRMAT DIN COD` · **[TEST]** = `CONFIRMAT PRIN TEST LOCAL` · **[DOC]** = `DOCUMENTAT, NEVERIFICAT` · **[INF]** = `INFERENȚĂ` · **[NEC]** = `NECUNOSCUT`. Prescurtare: `R/` = `readersv3/`.

**Documente înrudite**: [`data-and-state-machines.md`](data-and-state-machines.md) · [`build-deploy-update.md`](build-deploy-update.md) · [`gaps-and-contradictions.md`](gaps-and-contradictions.md) · [`security-and-identity.md`](security-and-identity.md) · [`communication-map.md`](communication-map.md) · [`end-to-end-flows.md`](end-to-end-flows.md) · [`api-catalog.md`](api-catalog.md) · [`application-catalog.md`](application-catalog.md) · [`runtime-and-modules.md`](runtime-and-modules.md) · [`agent-context.md`](agent-context.md).

---

## 0. Ce trebuie reținut înainte de a citi cifrele

1. **Rapoartele 01–05 nu au rulat nimic** — niciun test, niciun binar, niciun serviciu. Toate afirmațiile lor sunt `CONFIRMAT DIN COD` prin lectură (sau `INFERENȚĂ` / `NECUNOSCUT`); acolo unde citează un test, este „test existent, neexecutat de acel raport”. Eticheta `CONFIRMAT PRIN TEST LOCAL` aparține exclusiv rulărilor din raportul 06, listate mai jos.
2. **Niciun test simulat nu confirmă**: hardware (analizoare, porturi seriale, imprimante, tablete Signotec, token-uri criptografice), **CNAS real** (SIUI / SIPE / DocMED / eFactura), **backendul WiseMED real**, OpenAI, sau **instalarea și rularea pe Windows ori Linux**. Toate testele trecute folosesc backend-uri false (`httptest`, `net.Pipe`, loopback, `node:vm`, `fetch` simulat) și au rulat numai pe macOS darwin/arm64.
3. Un pachet „`ok`” înseamnă că testele lui **care nu au fost sărite** au trecut. Un test numit într-un raport 01–05 este „confirmat prin test” doar la nivel de pachet; testele individuale nu au fost izolate și verificate nominal.
4. Rulările nu au modificat arborele de lucru: `git status --porcelain` identic înainte și după (38 de linii, diff gol). [TEST]

---

## 1. Verificări executate (raportul 06, partea B)

Verificare de siguranță prealabilă [COD]: niciun `*_test.go` nu referă `output/` sau `dist/` (fișierele se scriu în `t.TempDir()`); URL-urile reale (`www.siui.ro`, `*.e-pias.ro`, `api.openai.com`) apar doar ca șiruri cu backend-uri false; testele live sunt condiționate de variabile de mediu, toate nesetate (`WAI_TEST_DATABASE_URL`, `WAI_BROWSER_TEST`, `SIUI_DATA_TEST_DATABASE_URL`, `SIUI_DATA_REAL_*`, `HISTORY_TEST_MYSQL_*`, `SIUI_TEST_PKCS11_*`, `SIUI_TEST_CONNECTED_TOKEN`, `SIUI_TEST_CNAS_TLS`, `REQUIRE_CHROME_TESTS`).

| # | Comandă exactă | Director | Rezultat | Ce demonstrează | Ce NU demonstrează |
|---|---|---|---|---|---|
| 1 | `go build ./...` | `readersv3/` | **OK**, 24,3 s | toate cele 101 pachete (inclusiv 32 de aplicații) compilează pentru darwin/arm64 | cross-compilarea Windows/Linux; build-ul prin `releasectl`; codul sub build tag-uri `windows` / `linux` (`*_windows.go`, `*_linux.go`) |
| 2 | `go vet ./...` | `readersv3/` | **OK**, 4,2 s, zero avertismente | lipsa problemelor detectabile de `vet` pe darwin | fișierele specifice Windows/Linux |
| 3 | `go test ./... -count=1 -timeout 15m` | `readersv3/` | **OK**: 33 pachete `ok`, 68 fără teste, 0 eșecuri; 18,9 s; 294 teste de nivel superior trecute, 243 subteste trecute, **60 sărite** | logica acoperită de teste unitare cu backend-uri simulate: SIUI (joburi, idempotency, callback, arhivare), clientul WSS, parsere de protocol, storage SQLite, config, runner | cele 60 de teste sărite (vezi §5); nimic despre CNAS real, analizoare, imprimante, tablete de semnătură, PostgreSQL/MySQL reale |
| 4 | `go test -race ./... -count=1 -timeout 20m` | `readersv3/` (tot modulul) | **OK**: 33 pachete `ok`, 0 curse raportate; 61,0 s | absența curselor de date **pe căile exercitate de teste** | curse pe căi neacoperite (27 din 51 de pachete `modules` nu au teste); concurența reală a gorutinelor per conexiune TCP față de SQLite |
| 5 | `go build ./...` | `wsm-server/` | **OK**, 1,4 s | serverul, `wsmctl`, exemplul și copiile din `dist/` compilează | build Linux / Docker |
| 6 | `go vet ./...` | `wsm-server/` | **OK**, 0,3 s | lipsa problemelor `vet` | — |
| 7 | `go test ./... -count=1` | `wsm-server/` | **OK**: 3 pachete `ok` (`internal/config`, `internal/logstore`, `internal/server`), 8 fără teste; 4,8 s; 58 teste + 70 subteste, 0 sărite | validarea configurației, stocarea logurilor, serverul (autentificare, rutare, admin, SDK) cu upstreamuri simulate | backend WiseMED real, Nginx, încărcare de producție, reload prin SIGHUP izolat |
| 8 | `go test -race ./... -count=1` | `wsm-server/` | **OK**: 3 pachete `ok`, 0 curse; 7,4 s | absența curselor pe căile testate ale hubului | căile netestate (`cmd/wsm-server`, `cmd/wsmctl`) |
| 9 | `go vet ./...` | `wsm-server/integration/` | **OK**, 0,2 s | — | — |
| 10 | `go test ./... -count=1 -v` | `wsm-server/integration/` | **OK**: 1 pachet, 30,3 s | server WSM cu TLS real pe loopback + modulul reader din `readersv3`: apel API, tichet de control și **reconectarea reală după ~30 s** | certificat public, DNS, proxy, mai multe readere, rețea reală; ciclul de expirare a JWT-ului de 5 min; **nu a rulat cu `-race`** (varianta din `wsm-server/verify.sh`) |
| 11 | `go list ./...` | rădăcină (`go.mod`, go 1.19) | **OK**: 1 pachet (`output/wsm-browser-sdk/examples/reader`) | modulul din rădăcină nu mai conține cod sursă propriu | — |
| 11b | `go vet ./...` | rădăcină | **FAIL** (preexistent, cod — E3) | singurul pachet al modulului rădăcină nu compilează în acest modul | nu afectează `readersv3` sau `wsm-server` |
| 12 | `node --test readersv3/modules/localhttp/ui/*.test.cjs readersv3/apps/esignature-server/tests/*.test.cjs` | rădăcină | **OK**: 75 teste, 63 trecute, **12 sărite**, 0 eșecuri; 0,5 s | logica UI locală (WSS, SIUI, formatter, history, wisemed-ai) și compatibilitatea clientului eSignature, executate în `node:vm` cu date simulate | cele 12 teste DOM sărite (cer `jsdom`); comportamentul într-un browser real |
| 13 | `node --test wsm-server/internal/server/adminui/app.test.cjs wsm-server/internal/server/sdk/*.test.cjs` | rădăcină | **FAIL**: 51 teste, 50 trecute, **1 eșuat** (E1); 0,3 s | SDK-ul de browser și consola de admin, cu transport simulat (50 de teste) | ruta `report/package` în catalogul SDK-ului (E1) |
| 14 | `./node_modules/.bin/vitest run` | `readersv3/apps/wisemed-ai/web/` | **OK**: 3 fișiere, 10 teste; 1,2 s | clientul API, facturarea și dictarea din frontend, cu `fetch` simulat | integrarea cu backendul |
| 15 | `./node_modules/.bin/vue-tsc --noEmit` | `readersv3/apps/wisemed-ai/web/` | **OK**, 2,4 s | typecheck-ul frontendului Vue/TypeScript | build-ul Vite (nerulat — ar scrie în `R/modules/wisemedai/web/dist`) |
| 16 | `python3 -m unittest -v test_icd10_crosswalk` | `readersv3/scripts/` | **OK**: 4 teste | parsarea expresiilor și potrivirea codurilor ICD-10 | rularea pe cataloagele reale |
| 17 | `python3 readersv3/docker/test-update.py -v` | rădăcină | **FAIL**: 8 teste, 16 erori (toate subtestele); 20,7 s (E2) | nimic pe acest host | comportamentul updaterului Docker pe Linux cu bash ≥ 4 rămâne neverificat |

Toate rândurile: [TEST] (raport 06 B.1). Outputul complet al fiecărei rulări a fost păstrat de raportul 06 în directorul de lucru temporar al sesiunii (nu în repository).

**Precizare la rândul 3 (rezoluție de inconsecvență internă a raportului 06).** Raportul enumeră cele 60 de teste sărite ca „31 `siuidata` + 26 `wisemedai` + 2 `siuihistory` + 2 `siui`” (suma ar fi 61) și, în B.3, „57 de teste PostgreSQL”. Outputul JSON salvat al rulării (citit la redactare, fără rerulare) dă: `siuidata` **31**, `wisemedai` **25**, `siuihistory` **2**, `siui` **2** = **60**; deci **56** de teste PostgreSQL. Totalurile 294 / 243 / 60 se confirmă din același fișier. [TEST]

---

## 2. Eșecuri preexistente

Niciun eșec nu a fost introdus de investigație (arbore neschimbat). Clasificare: **cod** = defect reproductibil în sursa comisă; **mediu** = depinde de mașina de rulare.

| ID | Rulare | Clasificare | Ce eșuează | Cauză | Efect |
|---|---|---|---|---|---|
| **E1** | 13 | **cod** | `wsm-server/internal/server/sdk/siui-bridge.test.cjs:16-22` („catalog covers every explicit SIUI auxiliary route”): `AssertionError: missing auxiliary path report/package` | `R/modules/siui/module.go:307` definește ruta `report/package` (documentată și în `R/modules/siui/openapi.go:111`), dar `SIUIBridge.endpoints` din `wsm-server/internal/server/sdk/siui-bridge.js` nu o conține; fișierele sunt curate în `git status`, deci desincronizarea este în codul comis | [TEST] pentru eșec; SDK-ul de browser nu expune `report/package` prin catalog — [INF] (nu s-a verificat dacă ruta poate fi apelată pe altă cale) |
| **E2** | 17 | **mediu**, cu o fragilitate reală în script | toate cazurile din `readersv3/docker/test-update.py`: `update-siui-bridge.sh: line 78: sshkey\xe2: unbound variable`, apoi `UnicodeDecodeError` în test | `R/docker/update.sh:78` conține `"…using $sshkey…"` cu caracterul `…` (U+2026) lipit de numele variabilei; `/bin/bash` 3.2.57 (macOS) tratează primul octet ca parte din nume și, sub `set -u`, raportează variabilă nedefinită | [TEST] pe bash 3.2. Scriptul a ieșit totuși cu cod 0 după eroare (observat cu un harness), deci eșecul nu ar fi vizibil în codul de ieșire. Pe bash ≥ 4 / Linux (ținta reală, EC2): [NEC] |
| **E3** | 11b | **cod** | `go vet ./...` în modulul rădăcină | `output/wsm-browser-sdk/examples/reader/main.go:14-15` importă `github.com/gorilla/websocket` și `…/wsm-server/internal/server`, inexistente în modulul `wisemed-labreaders` (go 1.19) | [TEST]; este o copie de exemplu livrată în `output/`, nu cod activ |

Observație [TEST + COD]: `wsm-server/docs/verification.md` (22 septembrie 2026) raportează toate verificările PASS; pe starea curentă suita Node a WSM are un eșec (E1). Vezi [`gaps-and-contradictions.md`](gaps-and-contradictions.md).

---

## 3. Ce NU s-a executat și de ce

| Verificare | Motiv | Consecință |
|---|---|---|
| Orice build real: `releasectl build/package/release/sync`, `make`, `build-all-*`, `package-all-*`, `R/scripts/*.sh`, `R/docker/build.sh`, `make-*-docker.sh`, `wsm-server/build.sh`, `build-docker.sh`, `prepare-local.sh` | interzise și/sau scriu în `dist/`, `output/`, `build/` | installerele NSIS, pachetele `.deb/.rpm/.pkg/.dmg`, arhivele de update și imaginile Docker **nu au fost construite sau instalate**: [NEC] dacă se instalează și pornesc corect |
| `python3 wsm-server/scripts/test-update-wsm.py` | **sărit ca riscant**: `wsm-server/update-wsm.sh:13-24` rulează `ssh-agent`, `ssh-add` și `sudo chown/chmod` reale înainte de orice validare, iar testul nu le simulează | updaterul WSM neverificat |
| `R/docker/update.sh`, `wsm-server/update-wsm.sh` pe un host real | acționează asupra containerelor și a repository-ului de arhive | rollback-ul de container neverificat |
| `R/docker/test-images.py` | cere Docker și imagini construite | — |
| `wsm-server/verify.sh` ca script | componentele lui au fost rulate separat (rândurile 1–10, 12–13), cu două diferențe: integrarea fără `-race`; `go build ./apps/...` acoperit de `go build ./...` | — |
| Playwright (`npm run test:e2e`, `R/tests/ui/order-id-click.cjs`), testul Go `TestBrowser*` | cer Chrome și un server pornit pe `127.0.0.1:19120` (`WAI_BROWSER_TEST`, `REQUIRE_CHROME_TESTS`) | 10 fișiere `*.e2e.ts` neexecutate |
| Testele PostgreSQL / MySQL | fără `WAI_TEST_DATABASE_URL`, `SIUI_DATA_TEST_DATABASE_URL`, `HISTORY_TEST_MYSQL_*` | vezi §5 |
| Testele cu token PKCS#11 / Keychain și TLS CNAS | fără `SIUI_TEST_PKCS11_*`, `SIUI_TEST_CONNECTED_TOKEN`, `SIUI_TEST_CNAS_TLS`; nu s-a contactat CNAS | vezi §5 |
| `vite build` | ar scrie în `R/modules/wisemedai/web/dist` | — |
| `go test -race` pe `wsm-server/integration` | nerulat în această variantă | — |
| Teste C# (`R/apps/esignature-server/tests/sigstring-oracle`), oracolul Java DocMED (`R/apps/siui-bridge/tests/docmed-contract-oracle`), testele Dart (`clients/flutter/test/client_test.dart`) | nu au făcut parte din planul de rulare al raportului 06 | compatibilitatea `sigenc` cu Topaz și contractul DocMED față de JAXB rămân [DOC] |
| `wsm-server -check-config` pe `deployments/config.http.yaml` | nerulat | [NEC] dacă fișierul trece validarea |
| Reproducerea lanțului „re-recepție → pierderea `wisemed_fsm_id` → comandă cu `wisemed_match_flag=4` nesincronizabilă” | nu există test; nu s-a scris unul | rămâne [INF] (vezi [`data-and-state-machines.md`](data-and-state-machines.md) §3.7) |

---

## 4. Acoperirea cu teste pe zone

Sursă: `go list -f '{{.ImportPath}} {{len .TestGoFiles}} {{len .XTestGoFiles}}' ./...` [TEST].

| Zonă | Pachete | Cu teste | Fără teste |
|---|---|---|---|
| `readersv3/apps` | 35 | 1 (`apps/runner`, 6 fișiere de test) | 34 (toate `main` + pachetele interne ale aplicațiilor) |
| `readersv3/core` | 3 | 2 | 1 (`core/module`) |
| `readersv3/modules` | 51 | 24 | 27 |
| `readersv3/shared` | 10 | 4 | 6 |
| `readersv3/tools` | 2 | 2 | 0 |
| **`readersv3` total** | **101** | **33** | **68** |
| `wsm-server` | 11 | 3 (`internal/config`, `internal/logstore`, `internal/server`) | 8 (`cmd/wsm-server`, `cmd/wsmctl`, `examples/reader`, 5 copii din `dist/`) |
| `wsm-server/integration` | 1 | 1 | 0 |

### Zone critice fără niciun test Go

| Zonă | Pachete / fișiere | De ce contează |
|---|---|---|
| Transporturi către analizoare | `modules/transports/serial`, `modules/transports/tcpip`, `modules/transports/file` | (serial/TCP sunt oricum stuburi; I/O-ul real este în protocoale) |
| Protocoale de analizor (10 pachete) | `anafdocsmart`, `analytikjenaplasmaquantmselite`, `anatoliageneworks`, `beoslcsv`, `biosanhipompp96`, `cary60uvvis`, `gammavision`, `genericfile`, `irbiotyper`, `tricarb5110tr`; plus `modules/astmspecimen` | parsere care scriu rezultate de pacient; `beoslcsv` este singurul cu `auto_confirm_wisemed: true` în șablon |
| Flux de laborator | `analytes`, `analytemanagement`, `qc`, `stats`, `dailyorders`, `dailydetails`, `dashboard`, `events` | (majoritatea sunt stuburi de meniu; logica este în `localhttp` și `storage/sqlite`, care au teste) |
| QC / Westgard | `classifyWestgard` în `R/modules/storage/sqlite/module.go:2313-2378` | nu există test pentru reguli (raport 03 §7.2) [COD] |
| Securitate și acces local | `modules/login`, `shared/bindguard` | |
| Update | `shared/appupdates` (clientul de update, semnarea/verificarea JWT-ului HS256); `apps/runner/update_apply.go` fără test dedicat | aplicarea unui update peste instalare, fără rollback |
| Imprimare | `modules/barcodeprinter` | rute neautentificate, inclusiv `PUT /api/barcode/settings` |
| Diagnostic | `shared/commtrace`, `shared/debugreplay`, `shared/analyzeractivity` | |
| Instalarea ca serviciu | `apps/runner/service_*.go` fără teste; `tools/releasectl` are 3 fișiere de test pentru 2467 de linii | divergența `-headless` (vezi §6) |
| WSM | `cmd/wsm-server` (pornire, reload SIGHUP), `cmd/wsmctl` (generare de chei și tokenuri) | |
| CI | `.github/workflows/readersv3-release.yml` **nu rulează** `go test`, `go vet`, teste Node sau Python; nu există workflow pentru `wsm-server` | nicio verificare automată la push [COD] |

Toate rândurile de pachete: [TEST] (raport 06 B.3). Nota privind rolul de stub al unor pachete: [COD] (raport 03 §0).

### Teste existente pentru comportamentele-cheie (pachetul a trecut; testul individual nu a fost izolat)

| Comportament | Teste numite în rapoartele 01–05 | Pachet | Statut |
|---|---|---|---|
| Corecția ID-ului probei: atomicitate, coliziune, actor autentificat, ID manual | `TestIDChangeIsAtomicAndRejectsCollision`, `TestIDChangeUsesAuthenticatedActor`, `TestManualFileIDBypassesAnalyzerNormalization`, `TestSendUsesCorrectedFileID` | `storage/sqlite`, `localhttp`, `resultsync`, `fileimportbase` | pachet `ok` la rularea 3 [TEST la nivel de pachet] |
| Păstrarea punctuației în TAG | `analyte_tag_test.go`, `astm/analyte_identity_test.go`, `resultsync/analyte_tag_test.go` | `modules/core/model`, `protocols/astm`, `resultsync` | idem |
| SIUI: restart fără replay, fără retry după scriere ambiguă, idempotency | `TestServiceRestartNeverReplaysPendingCalls`, `TestNoRetryAfterAmbiguousWrite`, `TestJobIdempotencyAndRestart` | `modules/siui` | idem; backend CNAS simulat |
| SIUI: callback și arhivare | `TestWMCallbackQueueAndIdempotency`, `TestWMDeliveryRestartAndStableID`, `TestHistoryOutageRestartAndResumption` | `modules/siui` | idem |
| SIUI: paritate catalog ↔ WSDL | `TestCatalogMatchesBundledWSDL`, `TestEveryRequestedSOAPOperation`, `TestCompletePrescriptionWSDLCoverage`, `TestDocMedAllWSDLContractsAndSchemas` | `modules/siui` | idem |
| WSM: suprascrierea câmpurilor `sender_*` | `server_test.go:570` | `wsm-server/internal/server` | pachet `ok` la rularea 7 |
| WiseMED AI: izolare tenant, antete nefalsificabile | `integration_test.go:262-290` | `modules/wisemedai` | **neconfirmat**: testele de integrare cer PostgreSQL și fac parte din cele 25 sărite [NEC care anume] |

---

## 5. Teste sărite

| Grup | Număr | Pachet / suită | Condiție nesatisfăcută | Ce rămâne neverificat |
|---|---|---|---|---|
| **PostgreSQL** | **56** (31 + 25) | `R/modules/siuidata` (31), `R/modules/wisemedai` (25) | `SIUI_DATA_TEST_DATABASE_URL`, `SIUI_DATA_REAL_*`, `WAI_TEST_DATABASE_URL` nesetate | migrările 1–5 `siui_data_*` și 001–017 `wai_*` pe o bază reală; importurile și activarea nomenclatoarelor; tranzacțiile de sesiune AI, advisory lock-urile, rezervarea de buget, outbox-ul, izolarea tenant/utilizator |
| **MySQL** | **2** | `R/modules/siuihistory` | `HISTORY_TEST_MYSQL_*` nesetate | indexul History pe MySQL real (manifest, commit, interogări) |
| **Token / TLS CNAS** | **2** | `R/modules/siui` | `SIUI_TEST_PKCS11_*`, `SIUI_TEST_CONNECTED_TOKEN`, `SIUI_TEST_CNAS_TLS` nesetate | semnarea cu token real (PKCS#11 / Keychain / magazin Windows), handshake-ul mTLS TLS 1.2 cu CNAS |
| **`jsdom`** | **12** | `node --test` pe `R/modules/localhttp/ui/*.test.cjs` | `jsdom` neinstalat și nedeclarat în niciun `package.json` | comportamentul DOM al UI-ului local |
| Playwright | 10 fișiere `*.e2e.ts` (neexecutate, nu „sărite” de runner) | `R/apps/wisemed-ai/web/tests/` | cer Chrome + server pe `127.0.0.1:19120` | fluxurile UI cap-coadă ale WiseMED AI |

Cifrele 31 / 25 / 2 / 2 / 12: [TEST] (rulările 3 și 12; defalcarea pe pachete din outputul salvat — vezi precizarea din §1). Suita `wsm-server` nu a sărit niciun test (rularea 7).

---

## 6. Verificări prin lectură încrucișată (orchestrator)

Verificări făcute de orchestrator prin **lectura directă a codului**, pentru a rezolva puncte în care rapoartele se bazau unul pe altul sau lăsau o inferență deschisă. Nimic de aici nu a fost executat: partea „confirmat” este `CONFIRMAT DIN COD`; efectele la runtime rămân `INFERENȚĂ` acolo unde este indicat.

| # | Punct verificat | Referințe | Confirmat din cod | Rămâne inferență / neverificat |
|---|---|---|---|---|
| O1 | Dublu `InitModules` | `R/apps/runner/runner.go:143` și `:264` | da — două apeluri | efectul real (handle-uri SQLite/pgx scurse, migrări `wisemed-ai` rulate de două ori, `initNative` al `signing-pad` apelat de două ori) [NEC] |
| O2 | Cheia HMAC a cookie-ului local este derivată din `ReaderID` pentru readere | `R/modules/localhttp/module.go:2660-2665` | da — nu este un secret aleator; utilitarele `siui*` folosesc în schimb o cheie aleatorie per proces (`:202-203`) | exploatabilitatea practică [INF]; vezi [`security-and-identity.md`](security-and-identity.md) |
| O3 | Cheie de configurare greșită la LD-560 | `R/apps/labnovation-ld-560/deployments/config.install.yaml:79` (`protocol-labnovation-ld-560`) vs `R/modules/protocols/labnovationld560/module.go:73` (`protocol-labnovation-ld560`) | da — setările din șablon stau sub o cheie pe care modulul nu o citește | comportamentul unei instalări reale (dacă `config.yaml` a fost corectat manual) [NEC] |
| O4 | Evenimentele WSM ale WiseMED AI | `R/modules/wisemedai/events.go:28` → `R/modules/ws/events.go:12` (`event`, `mode:"connection"`); rol `reader` (`R/modules/ws/module.go:707-712`); respingere în `wsm-server/internal/server/server.go:475` | da, **pe ambele părți**: clientul trimite `event` către o conexiune cu rolul `reader`, iar WSM refuză pentru acest rol orice `event` în afara topicurilor `results:<reader_id>` / `logs:<reader_id>` | **neverificat la runtime**; niciun test rulat nu exercită acest drum |
| O5 | Lease WSM la `exp` + JWT `device_key` de 5 min | `wsm-server/internal/server/server.go:265`; `R/modules/ws/module.go:754` | da — serverul închide socketul la `exp`; readerul își semnează singur un JWT cu `exp = now + 5 min` | **ciclul de reconectare** (deconectare la ~5 min, ~30 s offline) este [INF]; rularea 10 a exercitat doar reconectarea de ~30 s |
| O6 | `-headless` în unitățile de serviciu | `R/apps/runner/service_linux.go:20-36` (unit-ul generat de `-installservice` **nu** folosește `-headless`); unitățile împachetate `R/apps/*/build/linux/*.service` și plist-urile `build/macos` **folosesc** `-headless` | da — cele două căi de instalare generează definiții diferite | **efectul** (sub systemd `Type=simple`, părintele headless iese și serviciul se oprește; sub launchd `KeepAlive` repornește la nesfârșit) este [INF], neverificat pe Linux/macOS |
| O7 | `execution_unknown` vs `unknown` | `execution_unknown` apare numai în `wsm-server/internal/server/http_commands.go`, `wsm-server/examples/php/WSMHttpClient.php`, `wsm-server/docs/http-commands.md` | da — starea jobului SIUI este `unknown`; `*_execution_unknown` sunt coduri de eroare ale wrapperului HTTP de comenzi al WSM | — |

Rezoluțiile folosite în celelalte documente:

- **O4** închide diferența dintre raportul 05 (evenimentul descris ca livrat inițiatorului) și raportul 02 (respingere marcată ca inferență): pe codul actual evenimentul este refuzat de WSM.
- **O7** închide diferența dintre raportul 04 (șirul negăsit în zona SIUI) și raportul 02 (coduri `*_execution_unknown`): ambele observații sunt corecte, pentru componente diferite.
- **O5** și **O6** rămân deliberat împărțite în „fapt din cod” și „efect inferat”.

Verificări suplimentare prin lectură făcute la redactare (fără execuție), folosite în [`data-and-state-machines.md`](data-and-state-machines.md):

| # | Punct | Referințe | Rezultat |
|---|---|---|---|
| L1 | `RecordImportedResult` este netranzacțional și nu transmite ID-urile WiseMED | `R/modules/storage/sqlite/module.go:1380-1419` | [COD] trei scrieri independente; `OrderAnalysis` construit fără `WiseMEDSMID`/`WiseMEDFSMID` |
| L2 | `SaveOrderAnalysis` suprascrie `wisemed_sm_id`, `wisemed_fsm_id`, `flags_json` la analiza existentă; `UpsertOrder` îmbină meta | `R/modules/storage/sqlite/module.go:1080-1090,925-936` | [COD]; efectul cap-coadă asupra comenzilor cu `wisemed_match_flag=4` rămâne [INF] |
| L3 | Starea de trimitere este rescrisă și pe ramura `no_entries` | `R/modules/protocols/fileimportbase/module.go:655-679,706-758` | [COD] — precizează afirmația din raportul 03 §6.3 (valabilă doar pentru eșecul PATCH-ului) |
| L4 | Joburile SIUI neterminate devin `unknown` la deschiderea bazei | `R/modules/siui/service_jobs.go:48,394` | [COD] |
| L5 | Apelul AI sub advisory lock per organizație; ID de sesiune determinist din `Idempotency-Key` | `R/modules/wisemedai/extract.go:202`, `sessions.go:543-569` | [COD] |
| L6 | „Semnătura HMAC” din `shared/appupdates` este a JWT-ului, nu a artefactului; checksum comparat doar dacă există | `R/shared/appupdates/appupdates.go:236-262`, `R/apps/runner/updates.go:142-153` | [COD] — împacă rapoartele 05 §7 și 06 B.3 |

---

## 7. Sinteză: ce este și ce nu este acoperit de dovezi executate

| Afirmație | Nivel de dovadă |
|---|---|
| Codul Go din `readersv3` și `wsm-server` compilează și trece `vet` pe darwin/arm64 | [TEST] |
| Testele unitare existente trec (cu excepția E1) și nu raportează curse | [TEST] |
| WSM + clientul reader comunică pe TLS loopback și se reconectează după ~30 s | [TEST] (rularea 10) |
| Idempotency-ul, restartul și callbackul SIUI se comportă conform descrierii | [TEST] la nivel de pachet, cu CNAS și WiseMED simulate; [COD] pentru detalii |
| Schema și tranzacțiile PostgreSQL / MySQL funcționează pe o bază reală | **neverificat** — 58 de teste sărite |
| Comunicarea cu analizoare, imprimante, tablete Signotec, token-uri criptografice | **neverificat** — niciun test, nici simulat pentru hardware real |
| Comunicarea cu CNAS real și cu backendul WiseMED real | **neverificat** |
| Build, installer, serviciu și update pe Windows / Linux | **neverificat** — nimic construit sau instalat; codul sub build tag-uri `windows`/`linux` nici măcar compilat |
| Updaterele Docker (`update.sh`, `update-wsm.sh`) | **neverificat** — E2 pe macOS; testul WSM sărit ca riscant |
| Toate afirmațiile rapoartelor 01–05 | `CONFIRMAT DIN COD` prin lectură, sau `INFERENȚĂ` / `NECUNOSCUT` — niciuna prin execuție |
