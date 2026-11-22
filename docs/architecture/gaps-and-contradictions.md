# Discrepanțe, necunoscute și întrebări rămase

- **Scop:** registrul consolidat al contradicțiilor dintre cod, teste, documentație, memorie
  (`aimemory/`) și contractele externe, plus zonele neinvestigate.
- **Data:** 2026-11-14. **Surse:** rapoartele `_reports/01`–`06` și verificările directe ale
  orchestratorului (marcate **[O]**).
- **Limite:** rapoartele 01–05 sunt lectură de cod, fără execuție. Numai raportul 06 a rulat
  build/vet/teste. Nimic de aici nu confirmă hardware, CNAS real, backendul WiseMED din
  producție sau instalări pe Windows/Linux.
- Documentul descrie starea existentă. Nu conține remedieri; întrebările pentru proprietarul
  produsului sunt în §7.

Etichete: `CDC` = CONFIRMAT DIN COD, `CTL` = CONFIRMAT PRIN TEST LOCAL, `DN` = DOCUMENTAT,
NEVERIFICAT, `INF` = INFERENȚĂ, `NEC` = NECUNOSCUT.

## 1. Nepotriviri de interfață între subsisteme

Acestea sunt cele mai importante: fiecare parte e coerentă intern, dar cele două capete nu se
potrivesc.

| # | Interfață | Partea A | Partea B | Stare |
|---|---|---|---|---|
| I1 | WiseMED AI → WSM (evenimente) | `readersv3/modules/wisemedai/events.go:28` apelează `PublishEventToConnection`, care trimite `event` cu `target.mode="connection"` (`readersv3/modules/ws/events.go:12`); aplicația rulează cu rol `reader` (`readersv3/modules/ws/module.go:707-712`) | WSM respinge pentru rolul `reader` orice `event` care nu e pe topic `results:<reader_id>` / `logs:<reader_id>` (`wsm-server/internal/server/server.go:475`) | CDC pe ambele părți **[O]**; efectul cap-coadă (evenimentele `wisemed-ai.*` nu ajung la browser) e INF, neverificat la runtime |
| I2 | Reader → WSM în mod `device_key` | readerul își semnează JWT cu `exp = now+5min` (`ws/module.go:754`) și nu îl reînnoiește pe conexiunea existentă; pauză fixă 30 s la reconectare (`ws/module.go:319`) | WSM tratează autentificarea ca lease și închide socketul la `exp` (`wsm-server/internal/server/server.go:265`) | CDC pe fiecare fapt **[O]**; ciclul „deconectare la ~5 min + ~30 s offline” e INF |
| I3 | SDK browser SIUI ↔ bridge | ruta `report/package` există în bridge (`readersv3/modules/siui/module.go:307`) | lipsește din catalogul SDK; testul `wsm-server/internal/server/sdk/siui-bridge.test.cjs:16` pică (1 din 51) | CTL (eșec preexistent) |
| I4 | Reader → backend WiseMED (token WSS) | readerul face `POST <token_path>` (`readersv3/modules/wisemedapi/wss.go:15`) | stratul PHP local expune `GET /wsm/token` (`apis/wisemed-api/apiv2/implementation/wsm_wsmtoken.php:4`) | CDC; comportamentul producției NEC |
| I5 | Reader → backend WiseMED (bootstrap) | `Module.Bootstrap` așteaptă un array brut (`readersv3/modules/wisemedapi/module.go:715-731`); `BootstrapClient` (wizardul CLI) tratează învelișul | PHP-ul local întoarce `{success, rows}` (`apis/wisemed-api/apiv2/implementation/administrative.php:140`) | CDC; față de acest backend `GET /api/wisemed/bootstrap` ar da liste goale (INF); producția NEC |
| I6 | Șablon config ↔ modul Labnovation | `readersv3/apps/labnovation-ld-560/deployments/config.install.yaml:79` folosește `protocol-labnovation-ld-560` | modulul are ID `protocol-labnovation-ld560` (`readersv3/modules/protocols/labnovationld560/module.go:73`) | CDC **[O]**; setările din șablon nu sunt citite |
| I7 | Imagine Docker WSM ↔ consola de control | `wsm-server/Dockerfile:16` copiază 4 asset-uri | `app.js` importă static și `docmed-forms.js`, `siui-invoice.js` (`build.sh` copiază 6) | CDC; „consola nu se încarcă în imagine” e INF |
| I8 | Unități de serviciu împachetate ↔ `-headless` | `readersv3/apps/*/build/linux/*.service` și plist-urile `build/macos` pornesc binarul cu `-headless` | în `-headless` părintele lansează copilul și iese (`readersv3/apps/runner/runner.go:125-138`); unit-ul generat de `-installservice` nu folosește `-headless` (`readersv3/apps/runner/service_linux.go:20-36`) | CDC **[O]**; efectul sub systemd/launchd e INF |
| I9 | CI ↔ `releasectl` | workflow-ul urcă `readersv3/release` | installerele Windows sunt scrise în `dist/installers` | CDC; artefact gol pe runner e INF |
| I11 | Scope-uri implicite reader ↔ WSM | scope-urile implicite ale readerului în `device_key` (`readersv3/modules/ws/module.go:754`) și `DeviceScopes()` (`wsm-server/internal/config/devices.go:43-45`) nu conțin `api:invoke` și `wisemed:proxy` | WSM cere `api:invoke` pentru `control.ticket` și `wisemed:proxy` pentru `wisemed.*` (`wsm-server/internal/server/server.go:423-434`) | CDC **[O]**; cu valorile implicite, „Deschide” din reader e refuzat și worklist-ul prin WSS cade pe HTTP direct — INF; scope-urile reale din instalări NEC |
| I12 | Callback SIUI → backend WiseMED | bridge-ul trimite `Idempotency-Key: siui-<id>` (`readersv3/modules/wisemedapi/callback.go:30-74`) | antetul nu e citit nicăieri în `apis/` | CDC pentru stratul PHP local; producția NEC |
| I13 | Limite de dimensiune și timp pe lanțul WSS | `apibridge` permite 4 MiB și 30 s implicit (610 s pentru render) | `max_message_bytes` implicit al WSM e 1 MiB (depășirea închide socketul); wrapperul HTTP plafonează la 300 s; apelul OpenAI are 45 s | CDC pe fiecare limită; detalii în [communication-map.md](communication-map.md) §14 |
| I10 | `protocol.md` WSM ↔ readere | documentul trimite la handleri `orders.*`, `results.activate`, `logs.*` în `readersv3` | nu există; readerele nu emit pe `results:` / `logs:` | CDC |

## 2. Comportamente din cod care contrazic așteptările documentate

| # | Zonă | Ce spune documentația / cerința | Ce face codul | Etichetă |
|---|---|---|---|---|
| B1 | Runtime | lifecycle unic | `InitModules` rulează de două ori pe darwin/linux (`readersv3/apps/runner/runner.go:143` și `:264`); prima instanță `App` e abandonată fără `Start` | CDC **[O]**; resurse scurse / migrări duble — INF |
| B2 | Runtime | serviciu Windows echivalent | serviciul Windows folosește prima instanță, fără `process-control` → fără restart complet din UI | CDC |
| B3 | Runtime | transporturi decuplate de protocol | `transport-serial` și `transport-tcpip` sunt no-op; I/O-ul e în fiecare protocol; ASTM generic nu suportă serial | CDC |
| B4 | Runtime | module `login`, `dashboard`, `stats`, `analytes`, `events` ca funcționalitate | sunt meniu + `/meta`; logica e în `modules/localhttp` și `modules/storage/sqlite`; nu există event bus (UI face polling) | CDC |
| B5 | Laborator | `generic-test-reader` importă fișiere | `protocol-generic-file` e placeholder; importul manual răspunde 400 | CDC |
| B6 | Laborator | o nouă recepție retransmite rezultatele (auditul din 2026-09-16) | re-recepția șterge `wisemed_sm_id`/`wisemed_fsm_id`; comenzile cu `wisemed_match_flag=4` sunt sărite de sincronizare → rezultatul nou nu mai poate fi trimis | CDC pe verigi; efectul cap-coadă INF — de reprodus prin test |
| B7 | Laborator | repeat per analizor la recepție | `repeat_mode` afectează doar selecția manuală; ultimul rezultat devine mereu implicit | CDC |
| B6a | Laborator | un eșec de trimitere lasă starea neschimbată (`_reports/03 §6.3`) | adevărat doar pentru `PATCH` eșuat; ramura `no_entries` rescrie comanda la `none` și analizele la `pending` (`readersv3/modules/protocols/fileimportbase/module.go:658,706-758`) | CDC; coborârea unei comenzi `full` după re-recepție e INF |
| B6b | Utilitare | helper Java la `./java-helper/signing-pad-helper.jar` (șablon `signing-pad-utility`) | JAR-ul e în `readersv3/apps/signing-pad-utility/java/`; `deployments/` nu îl conține | CDC; cum ajunge la calea din șablon — NEC |
| B8 | Laborator | ASTM cu verificare | ACK necondiționat (ASTM, Pentra, miniVIDAS): checksum doar pentru trace, fără NAK/retransmisie; ACK înaintea persistenței | CDC |
| B9 | Laborator | `frame_timeout_ms: 2000` | plafonat la 100 ms (`readersv3/modules/protocols/astm/module.go:565-568`) | CDC |
| B10 | Laborator | coliziune ID „în aceeași zi/rundă” | verificată doar pe aceeași dată **și** aceeași rundă; nu există unicitate pe `orders(order_date, round_no, sample_id)` | CDC |
| B11 | SIUI | XML-ul cererii nu se păstrează | parametrii compleți sunt în `siui_history_outbox.parameters`, fără ștergere | CDC |
| B12 | SIUI | sesiune OCSP proaspătă per operație; test 25 s | reutilizare 150 s; timeout 120 s | CDC |
| B13 | SIUI | Linux nesuportat (mesaj 503, memorie) | PKCS#11 pe Linux e implementat | CDC |
| B14 | SIUI | „reconcilierea” stării `unknown` | nu există cod de reconciliere și nici rută de anulare | CDC |
| B15 | SIUI | `completed` = succes | la operațiile clasice răspunsul CNAS nu e interpretat; doar DocMED transformă erorile de business în `failed` | CDC |
| B16 | WSM | chei device doar rol `reader` | generate cu `reader` + `service`; chei browser cu `browser` + `service` | CDC |
| B17 | WSM | `reconnect_delay_ms`, `heartbeat_ms` configurabile | necitite; 30 s / 15 s hard-codate | CDC |
| B18 | WSM | revocare limitată la tenant | `authEpoch` e global: o modificare admin într-un tenant invalidează lease-urile și tichetele tuturor | CDC (disponibilitate, nu confidențialitate) |
| B19 | Update | actualizare verificată | checksum SHA-256 verificat doar dacă serverul îl trimite; fără semnătură, backup sau rollback; verificare doar la pornire | CDC |
| B20 | Update | toate utilitarele au `app-updates` (`readersv3/AGENTS.md`) | `signing-pad-utility` și `update-server` nu au secțiunea; `update-server` nu are self-update; URL-ul „standard” e `http://127.0.0.1:19090` | CDC |
| B21 | AI | STT pe telefon (`docs/WisemedAI.md`) | există și transcriere pe server | CDC |
| B22 | Utilitare | `anaf-docsmart` comunică cu ANAF / cere login | completează local un PDF prin Adobe Acrobat, doar pe macOS; rutele proprii nu cer sesiune | CDC |
| B23 | Utilitare | `signing-pad-utility` funcțional | helperul Java e stub; `esignature-server` folosește DLL Signotec nativ, Windows 32 biți | CDC |

## 3. Documentație și memorie depășite

- **Numărul de aplicații:** 32 în cod (`go list ./apps/...`, CTL); memoria spune 26,
  `readersv3/docs/wss-control.md` 28, `readersv3/README.md` „patru readere”,
  `readersv3/build/apps.json` 28 (lipsesc cele trei aplicații SIUI și `wisemed-ai`).
- **`aimemory/wisemed-labreaders.md`:** prima jumătate descrie generația `Readers Last/…`
  (tabel `event_logs`, `result_available`, `/api/stats`), inexistentă în repo.
- **`Readme.md`, `Service.md`, `InstallPI.md` din rădăcină:** istorice (`implementation/`
  nu mai există).
- **`wsm-server/docs/verification.md`:** raportează „9 + 12 teste”; azi sunt 75 + 51, cu un
  eșec (I3).
- **OpenAPI SIUI:** omite `validations*`, `insured`, `personalization`, `ocsp-test`,
  `certificates/refresh`, `siui-history-sync/*`; nu declară tagul `docmed`.
- **OpenAPI WiseMED AI:** omite 5 rute folosite de UI și codurile 402/410/413/502.
- **WSDL vs expus:** 73 de operații în WSDL, 63 expuse (41 + 22 DocMED); 10 neexpuse, din
  care documentația menționează doar 2.
- Lista completă pe subsistem: `_reports/01 §8`, `02 §15`, `03 „Contradicții”`,
  `04 „Contradicții”`, `05 §9`, `06 „Contradicții”`.

## 4. Distincții de terminologie rezolvate

- **`unknown` vs `execution_unknown`:** `unknown` este stare de job SIUI.
  `timeout_execution_unknown`, `client_disconnected_execution_unknown` și
  `request_interrupted_execution_unknown` sunt coduri de eroare ale wrapperului HTTP WSM
  (`wsm-server/internal/server/http_commands.go`) și flagul `executionUnknown` din
  `wsm-server/examples/php/WSMHttpClient.php`. **[O]** Niciuna nu are reconciliere automată.
- **„OCSP” în SIUI:** este autentificarea de sesiune CNAS, nu verificarea stării unui
  certificat.
- **`modules/login`:** placeholder; autentificarea locală e în `modules/localhttp`.
- **`shared/bindguard`:** tratează doar conflictul de port; nu e control de securitate.
- **Modulul Go din rădăcină:** orfan (fără fișiere `.go`); `go list` găsește un singur
  pachet, `output/wsm-browser-sdk/examples/reader`, care nu compilează în acel modul.

## 5. Observații despre conținutul repository-ului

- Binare și backup-uri urmărite în git: `wsm-server/output/bin/WSM_Server.backup-*` (5),
  `readersv3/zonci-xl1000i-reader.exe`, `output/wsm-browser-sdk/bin/*`.
- 15 fișiere `config.yaml` de runtime urmărite, inclusiv
  `readersv3/apps/shimatzu-generic-reader/deployments/config.yaml`.
- `docs/signotec/.../serverkey.pem` este urmărit (conținut neinspectat).
- `cfg_wisemed_key` este pre-completat în șabloanele `config.install.yaml` și apare identic în
  `apis/mp-api/index.php` (valoarea nu e reprodusă aici).

## 6. Necunoscute și zone neinvestigate

**Externe repository-ului (nu pot fi verificate de aici):**

- backendul WiseMED din producție: logica de business, emiterea JWT pentru browser, endpointul
  de token WSS, respectarea `Idempotency-Key` la callback;
- comportamentul real CNAS;
- aplicația Flutter completă (în repo e doar clientul Dart subțire);
- URL-urile de producție pentru update-server și WSM.

**Interne, necitite sau citite doar structural:**

- parserele mari (Cary60, CFX96, TOC-L, HL7 LD-560) și motorul de formule;
- handlerii `localhttp` în afara sincronizării, trimiterii și corecției ID-ului; `app.js`;
- WiseMED AI: billing/abonamente, comunitatea vocală, modelele vocale, prompturile; schemele
  OpenAPI nu au fost comparate câmp cu câmp cu structurile Go;
- SIUI: facturare/CMS, construcția XML din formularele DocMED, interogările SIUI Data,
  drumul Windows (WinHTTP) în detaliu;
- apelurile DLL Signotec, layouturile ZPL, UI-ul update-server;
- sursele Delphi și PDF/DOCX din `analyzers-comm-protocols/`, PDF-urile din `docs/CNAS`;
- configurațiile și bazele de date reale din `readersv3/output/` și `wsm-server/output/`
  (neatinse intenționat); binarele din `output/` nu au fost comparate cu sursa.

**Neexecutate:**

- teste care cer PostgreSQL (56; raportul 06 scrie 57, dar outputul salvat dă 60 de teste
  sărite în total), MySQL (2), token/TLS CNAS (2), `jsdom` (12 teste UI);
- Playwright, testele Dart și C#;
- `wsm-server/scripts/test-update-wsm.py` (ar apela `sudo` și `ssh-agent` reale) și
  `readersv3/docker/test-images.py` (Docker);
- orice build de installer, pachet de update sau imagine Docker.

## 7. Întrebări pentru proprietarul produsului

1. Evenimentele `wisemed-ai.*` prin WSM (I1) sunt folosite în producție? Dacă da, cu ce rol
   se conectează aplicația?
2. Modul `device_key` (I2) este cel folosit de readerele instalate, sau `token_endpoint`?
3. Care este URL-ul de producție al update-server și cine publică release-urile?
4. Backendul de producție întoarce pentru bootstrap un array brut sau `{success, rows}` (I5),
   și expune tokenul WSS pe `POST` sau `GET` (I4)?
5. Comportamentul din B6 (rezultat re-recepționat care nu se mai trimite) este cunoscut și
   acceptat?
6. Fișierele sensibile urmărite în git (§5) sunt intenționate?
