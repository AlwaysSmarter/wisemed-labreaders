# 01 — Runtime `readersv3` și catalogul aplicațiilor

Investigație read-only, 2026-11-14. Căile sunt relative la rădăcina repository-ului. Etichete: `CONFIRMAT DIN COD` (CDC), `CONFIRMAT PRIN TEST LOCAL` (CTL — doar `go list ./apps/...` a fost rulat), `DOCUMENTAT, NEVERIFICAT` (DN), `INFERENȚĂ` (INF), `NECUNOSCUT` (NEC). Nu s-a pornit niciun binar; nu există confirmări la runtime.

---

## 1. Ordinea reală de inițializare

### 1.1 Secvența standard (`main` → `runner.Run`)

| # | Pas | Referință | Etichetă |
|---|---|---|---|
| 1 | `main` parsează 7 flag-uri, tratează `-version` (tipărește `appmeta.CurrentVersion()` și iese), construiește lista **hardcodată** de module și apelează `runner.Run(cfgPath, modules, RunOptions{...})`; eroarea → `log.Fatal` | `readersv3/apps/generic-test-reader/main.go:12-28` (identic structural în 31 de aplicații) | CDC |
| 2 | `config.Ensure(configPath)`: creează `config.yaml` din `config.install.yaml` sau face merge non-distructiv | `readersv3/apps/runner/runner.go:52`, `readersv3/core/config/install.go:19` `Ensure` | CDC |
| 3 | `config.Load` (YAML → `Config`, `ApplyDefaults`) + `normalizeLegacyConfig` (signing-pad `shared_http`, `app_id` shimatzu-generic) | `runner.go:56-60`, `runner.go:183` | CDC |
| 4 | `cfg.EnabledModules = defaultModules` (+ `runner-startup-updates` dacă `BackgroundStartupUpdates`) — lista de module NU vine din YAML (`EnabledModules` are tag `yaml:"-"`) | `runner.go:70-73`, `core/config/config.go:19` | CDC |
| 5 | Bootstrap interactiv `ensureBootstrap(cfg, reconfigure)` dacă `!SkipBootstrap || Reconfigure`. Eroarea e **non-blocantă** (warning + `ApplyDefaults`), cu excepția protocolului `siui-data` unde e fatală (salvează progresul dacă `changed`) | `runner.go:74-92`, `bootstrap.go:15` | CDC |
| 6 | `setupLogging`: arhivează săptămânile încheiate în zip, deschide `<configDir>/<reader-id>-YYYYMMDD.log`, redirecționează `log` global; `-showlog` → `MultiWriter(stdout, fișier)` | `runner.go:93`, `logging.go:18` | CDC |
| 7 | Dacă bootstrap-ul a modificat ceva: `confirmSaveAndStart` (prompt `[Y/n]`), apoi `cfg.Save()`; refuz → `return nil` | `runner.go:103-112`, `bootstrap.go:336` | CDC |
| 8 | `-installservice` → `installService(cfg)` și ieșire (fără a porni module) | `runner.go:113-115` | CDC |
| 9 | Verificare update sincronă `checkForUpdates` (dacă nu e `BackgroundStartupUpdates`); `errUpdateStarted` → ieșire curată; altă eroare → warning | `runner.go:116-124`, `updates.go:22` | CDC |
| 10 | `-headless` (părinte): `launchHeadlessProcess` relansează binarul cu `-config X -headless-child`, așteaptă rezultatul pornirii (max 1 min), tipărește PID/log/pidfile și iese | `runner.go:125-138`, `headless_unix.go:18`, `headless_windows.go:23`, `headless_startup.go:16` | CDC |
| 11 | **Prima** instanță runtime: `module.NewRegistry` + `registerRunnerModules` + `runtime.New` + `app.InitModules(cfg.EnabledModules)` | `runner.go:139-145` | CDC |
| 12 | `runServiceManager(cfg, app)`: pe Windows, dacă procesul e serviciu SCM → `svc.Run` cu `app.Start` și return; pe darwin/linux întoarce mereu `(false, nil)` | `runner.go:146-148`, `service_windows.go:66`, `service_unix.go:10` | CDC |
| 13 | `signal.NotifyContext(SIGINT, SIGTERM)`; copilul headless scrie `<reader-id>.pid` (șters prin `defer`) | `runner.go:149-162` | CDC |
| 14 | `runAppLoop`: **reîncarcă** config, construiește un **al doilea** registry + `runtime.App`, înregistrează `process-control`, rulează din nou `InitModules`, apoi `app.Start(runCtx)` într-o goroutine | `runner.go:247-282` | CDC |
| 15 | `App.Start`: câte o goroutine per modul care implementează `Starter`; mux-ul HTTP e servit de `local-http.Start` (`http.Server{Handler: rt.Mux()}`) | `core/runtime/runtime.go:124-149`, `modules/localhttp/module.go:299-332` | CDC |
| 16 | Shutdown: semnal → `ctx.Done` → `cancel()` → `<-errCh` → `return nil`; `closeLog(runErr)` prin `defer` | `runner.go:283-287`, `runner.go:97` | CDC |

### 1.2 Observații critice despre secvență

- **Dublu `Init` pe darwin/linux.** Pașii 11 și 14 inițializează modulele de două ori; prima instanță `App` este abandonată fără `Start`, deci resursele deschise în `Init` nu sunt închise: handle SQLite din `storage-sqlite` (`modules/storage/sqlite/module.go:56` `Open`), DB-ul de log din `barcode-printer` (`modules/barcodeprinter/module.go:66` `openDB`), istoricul `signing-pad` (`modules/signingpad/module.go:78` `openHistory`), pool-ul pgx + migrările din `wisemed-ai` (`modules/wisemedai/module.go:51-78`), DB-ul `app-update-server` nu e afectat (nu folosește `runner`). CDC pentru existența dublei inițializări; efectul (handle-uri scurse, migrări rulate de două ori la fiecare pornire) este INF — neverificat la runtime.
- **Serviciul Windows folosește prima instanță** (`service_windows.go:78,92`), care nu are serviciul `process-control` și nu trece prin `runAppLoop` → restart-ul complet cerut din UI (`modules/localhttp/module.go:972,2792`) nu e disponibil ca serviciu Windows; `processControl()` întoarce `nil` (`modules/localhttp/module.go:3092-3099`). CDC.
- **Copilul headless repetă pașii 2–9** (Ensure, bootstrap, logging, update-check), deci verificarea de update se face de două ori la `-headless` (părinte + copil). CDC (`headless_unix.go:33` argumente; `runner.go:52-124` fără ramură pe `HeadlessChild`).
- **Bootstrap-ul interactiv citește `os.Stdin`** (`bootstrap.go:24`); copilul headless are stdin pe `/dev/null` (`headless_unix.go:36`), deci un config incomplet produce prompturi cu EOF → valorile implicite. INF.
- **Logul zilnic nu se rotește în timpul rulării**: fișierul e ales o singură dată în `setupLogging` (`logging.go:30`); arhivarea săptămânală rulează numai la pornire (`logging.go:27`). CDC.

### 1.3 Ordonarea modulelor, erori, context

- **Ordine**: strict ordinea slice-ului din `main.go`; nu există declarații de dependențe, sortare topologică sau priorități (`runtime.go:110-122` `InitModules`). CDC.
- **Dependențe implicite la `Init`** (ordinea contează): `barcode-printer` și `signing-pad` cer `ws-action-dispatcher` (`barcodeprinter/module.go:84`, `signingpad/module.go:81`) → `wisemed-ws` trebuie să fie înainte (lipsa e tolerată tăcut); `signing-pad` (cu `shared_http`), `siui`, `siui-data` cer `local-http-control` și **eșuează** dacă lipsește (`signingpad/module.go:93-99`, `siui/module.go:123-127`, `siuidata/module.go:97-101`); `siui-history` îl ia opțional (`siuihistory/module.go:76`). Restul consumurilor de servicii sunt leneșe (la request sau în `Start`). CDC.
- **Eroare la `Init`**: `InitModules` se oprește la primul eșec și întoarce eroarea; modulele deja inițializate nu sunt curățate; `Run` întoarce eroarea → `[ERROR]` pe stdout + log + raportare către părintele headless (`runner.go:44-51`) → `log.Fatal` în `main`. Modul neînregistrat → `module %q is not registered` (`core/module/registry.go:22`). Un pattern HTTP duplicat ar produce `panic` în `http.ServeMux.Handle`, fără `recover` (INF; nu există conflict în compozițiile actuale datorită `normalizeLegacyConfig`, vezi §5.3).
- **`Start`**: toate `Starter`-ele pornesc **în paralel**, fără ordine (`runtime.go:129-139`). Primul `Start` care se întoarce — **inclusiv cu `nil`** — declanșează `cancel()` pentru toate; `running.Wait()` așteaptă fără timeout terminarea tuturor; se întoarce doar prima valoare citită din `errCh` (`runtime.go:141-148`). Consecință: orice `Starter` trebuie să blocheze până la `ctx.Done()`; un modul blocat blochează oprirea. CDC; comportament acoperit de `core/runtime/lifecycle_test.go:28` `TestStartWaitsForShutdown` (test existent, nerulat aici).
- **Propagare context**: `signal.NotifyContext` → `ctx` (`runner.go:149`) → `runCtx` per iterație (`runner.go:277`) → `context.WithCancel` în `App.Start` (`runtime.go:125`) → fiecare `Starter`. Excepție: `result-sync` rulează sincronizarea cu `context.Background()` (`modules/resultsync/module.go:103,109`), deci o sincronizare în curs nu e anulată la oprire. CDC.
- **Ieșire din `runAppLoop`**: semnal → `nil`; cerere de restart → buclă nouă; `Start` întors spontan → `return err` (poate fi `nil` → proces încheiat cu cod 0) (`runner.go:283-300`). Sub systemd (`Restart=on-failure`, `service_linux.go:29`) o ieșire cu cod 0 nu repornește serviciul — INF.

### 1.4 Ce face pachetul `runner` (pe fișiere)

| Fișier | Rol | Etichetă |
|---|---|---|
| `runner.go` | `Run`, `RunOptions`, `normalizeLegacyConfig`, `processControl`, `runAppLoop` | CDC |
| `bootstrap.go` | Wizard interactiv: setări `wisemed-api`, identitate reader, selecție unitate medicală / tip echipament prin `wisemedapi.BootstrapClient`, `local_http`, `wisemed_ws`, comm/protocol, secțiuni `siui`/`siui-history` la `-reconfigure`, `app-updates`, `storage-sqlite`, `result-sync`, transport; `registerEquipment` în WiseMED (`bootstrap.go:441`); `needsBootstrap` (`:289`); derivarea protocoalelor/transporturilor suportate din `EnabledModules` (`:721`, `:776`) | CDC |
| `siui_data_setup.go` | Bootstrap separat pentru `siui-data`: PostgreSQL, identitate WiseMED obligatorie (`echipament_id > 0`), WSM cu scope-uri fixe (`siuiDataWSSScopes`, `:16`), parole citite cu `golang.org/x/term` | CDC |
| `headless_startup.go`, `headless_unix.go`, `headless_windows.go` | Lansare proces detașat (`Setsid` / `DETACHED_PROCESS`), raportare pornire printr-un fișier temporar indicat de env `WISEMED_HEADLESS_STARTUP_FILE`, timeout 1 min | CDC |
| `logging.go` | Log zilnic în directorul configului, arhivare zip săptămânală, colorare `[ERROR]` pe TTY | CDC |
| `updates.go`, `update_apply.go` | `checkForUpdates` (check → download → checksum → auto-aplicare doar dacă arhivă și (`Mandatory` sau rulează ca serviciu Windows) și `WISEMED_CONTAINER != 1`, `updates.go:157`); aplicare prin script PowerShell / `/bin/sh` | CDC |
| `background_updates.go` | Modul intern `runner-startup-updates`: aceeași verificare, dar în `Start`, ca să nu întârzie servirea; `errUpdateStarted` → `Start` întoarce `nil` → runtime-ul se oprește pentru updater | CDC |
| `service_common.go` | Nume serviciu `wisemed-<token>` (excepție: `wisemed-ai`) | CDC |
| `service_linux.go` | Unit systemd în `/etc/systemd/system`, `Restart=on-failure`; cont de sistem dedicat pentru `html-print-formatter` și `wisemed-ai` | CDC |
| `service_darwin.go` | LaunchDaemon `eu.wisemed.readersv3.<token>`, `KeepAlive` | CDC |
| `service_windows.go` | `sc.exe create/config/failure/start`; `runServiceManager` + `windowsService.Execute` | CDC |
| `service_detect_*.go` | `currentRunsAsService()` — real doar pe Windows | CDC |

---

## 2. Contracte și registry de servicii

### 2.1 Contracte (`readersv3/core/module/types.go`)

- `Module` (`:30`): `ID() string`, `Init(rt Runtime) error`.
- `Starter` (`:35`): `Start(ctx context.Context) error` — opțional, detectat prin type assertion (`runtime.go:130`).
- `Runtime` (`:16`): `ConfigPath`, `ConfigDir`, `ReaderID`, `Logf`, `ModuleSettings(id)`, `ResolvePath`, `AddMenu`, `Handle`, `Mux`, `RegisterService`, `Service`. Implementat de `runtime.App` (`runtime.go:16`).
- `MenuEntry` (`:8`): `ID, Group, Label, Path, Order`; sortare stabilă pe `(Group, Order)` la fiecare `AddMenu` (`runtime.go:81-91`); expus la `GET /api/menu`.
- `Registry` (`registry.go:7`): `map[string]Factory`, fără mutex, `Register` suprascrie tăcut un ID existent.
- Nu există `Stop`/`Close` în contract: curățarea se face la ieșirea din `Start` după `ctx.Done()`. Modulele fără `Starter` nu au niciun hook de oprire. CDC.
- Serviciile sunt `interface{}` netipizate, rezolvate prin nume + type assertion la consumator; rutele expuse de runtime însuși: `/healthz`, `/api/menu`, `/api/modules` (`runtime.go:50-52`). CDC.

### 2.2 Tabel servicii: nume → furnizor → consumatori

| Serviciu | Furnizor (înregistrare) | Consumatori (`Service(...)`) |
|---|---|---|
| `reader-config` | `runtime.New` (`runtime.go:36`) — map instantaneu din config | `localhttp/module.go:3082`, `wisemedapi/module.go:1079` |
| `analyzer-config` | `runtime.New` (`runtime.go:43`) | `dailydetails:157`, `localhttp:3157`, `erbamannheimlaura:145`, `labnovationld560:1015,1031`, `wisemedapi:208,1091`, `ws:696`, `astm:1203` (prin `moduleServiceValue`, `astm/module.go:1568`) |
| `app-config` | `runtime.New` (`runtime.go:47`) | `localhttp/module.go:3716` |
| `process-control` | `runner.runAppLoop` (`runner.go:263`) | `localhttp/module.go:3093` |
| `local-http-ready` | `runner.runAppLoop` (`runner.go:271`), doar copil headless | `localhttp/module.go:369` |
| `local-http-control` | `local-http` (`localhttp/module.go:215`) | `barcodeprinter:374`, `htmlprintformatter/auth.go:87`, `signingpad/module.go:93,191`, `signingpad/websocket.go:42`, `siui/module.go:123`, `siuidata/module.go:97`, `siuihistory/module.go:76` |
| `daily-analysis-send-filters` | `local-http` (`localhttp/module.go:216`) | `horibaabxpentra400:607`, `zoncixl1000i:491` |
| `http-server-options` | `html-print-formatter` (`htmlprintformatter/module.go:46`), `wisemed-ai` (`wisemedai/module.go:104`) | `localhttp/module.go:326` |
| `storage` | `storage-sqlite` (`storage/sqlite/module.go:70`) | `localhttp` (≥13 locuri, ex. `:1307`, `order_identity.go:30`), `resultsync:203,301,509`, `transports/file:82`, `wisemedapi:793`, `fileimportbase` (6), `astm` (3), `cary60uvvis` (3), `shimatzutocl` (3), `labnovationld560:938,1110`, `horibaabxpentra400:590,658`, `zoncixl1000i:475,499`, `biomerieuxminividas:330`, `cfx96quantitation:547`, `erbamannheimlaura:283` |
| `storage-meta` | `storage-sqlite` (`:71`) | niciun consumator găsit |
| `analyzer-activity` | `events` (`events/module.go:17`) | `localhttp:1339`, `astm:1407`, `erbamannheimlaura:248`, `fileimportbase:935`, `labnovationld560:1131,1143` |
| `events` | `events` (`:18`) | niciun consumator găsit |
| `wisemed-api` | `wisemed-api` (`wisemedapi/module.go:153`) | `appupdateserver:2057`, `localhttp:3176`, `astm:999`, `beoslcsv:67,221`, `cary60uvvis:349`, `fileimportbase:486,573`, `horibaabxpentra400:599`, `labnovationld560:947`, `shimatzutocl:348`, `zoncixl1000i:483`, `resultsync:518`, `siui/callback.go:281`, `wisemedai/auth.go:113`, `ws:345` |
| `ws-action-dispatcher` | `wisemed-ws` (`ws/module.go:140`) | `barcodeprinter:84`, `signingpad:81` |
| `wisemed-ws-status` | `wisemed-ws` (`:141`) | `localhttp:3235`, `wisemedai/runtime_settings.go:85` |
| `wisemed-ws-client` | `wisemed-ws` (`:142`) | `localhttp/wss.go:28`, `siuidata/bridge_sync.go:37`, `wisemedai/events.go:17`, `wisemedai/runtime_settings.go:59,186`, `wisemedapi:802` |
| `result-sync` | `result-sync` (`resultsync/module.go:86`) | `localhttp:3194`, `fileimportbase:582` |
| `result-publisher` | `protocol-beosl-csv` (`beoslcsv/module.go:53`) | `localhttp:3203` |
| `file-importer` | `fileimportbase` (`:180`, toate protocoalele derivate), `protocol-cary60-uvvis` (`:63`), `protocol-shimatzu-tocl` (`:68`) | `localhttp:3559` |
| `transport-file` | `transport-file` — **în `Start`**, nu în `Init` (`transports/file/module.go:36`) | `fileimportbase:399`, `cary60uvvis:296`, `shimatzutocl:295` |
| `daily-details` | `daily-details` (`dailydetails/module.go:28`) | `localhttp:3532` |
| `signing-pad` | `signing-pad` (`signingpad/module.go:109`) | `localhttp:591` |
| `debug-replay-runner` | `protocol-astm` (`astm/module.go:170`), `protocol-labnovation-ld560` (`:84`) | `localhttp/debug_replay.go:117` |
| `protocol-astm-status` | `protocol-astm` (`:169`) | niciun consumator găsit |
| `labnovation-ld560-status` | `protocol-labnovation-ld560` (`:83`) | niciun consumator găsit |
| `dashboard`, `analytes`, `analyte-management`, `qc`, `stats`, `help` | modulele omonime (map-uri descriptive `features`) | niciun consumator găsit (apar doar în `/api/modules`) |

Toate rândurile: CDC (grep exhaustiv pe `RegisterService(` și `.Service("` în `apps core modules shared`, fără `_test.go`). Consumul prin nume construit dinamic există într-un singur loc (`astm/module.go:1568`).

Note: `transport-file` e înregistrat în `Start`, iar `Start`-urile rulează în paralel → importatoarele pot vedea serviciul lipsă la primul scan; se autocorectează la tick-ul următor deoarece `fileTransport()` e recitit la fiecare scan (`fileimportbase/module.go:226,397`). INF. Serviciile `reader-config`/`analyzer-config`/`app-config` sunt instantanee la `runtime.New` — modificările de config devin vizibile doar după restartul complet din `runAppLoop`. CDC.

---

## 3. Registry de module

Toate ID-urile sunt înregistrate într-un singur loc: `readersv3/modules/builtin/register.go:54` `RegisterAll` (47 ID-uri), plus `runner-startup-updates` în `apps/runner/background_updates.go:17`. CDC.

| ID | Pachet (`readersv3/modules/…`) | `Init` | `Start` | Rute (prefixe) |
|---|---|---|---|---|
| `local-http` | `localhttp` | sesiuni, CORS, meniu; servicii `local-http-control`, `daily-analysis-send-filters` | listener HTTP/HTTPS pe mux-ul comun, rebind la schimbare adresă, `bindguard` la port ocupat | `/`, `/settings*`, `/orders`, `/qc`, `/history`, `/operations`, `/debug`, asset-uri JS/CSS, `/api/session*`, `/api/preferences*`, `/api/reader-settings`, `/api/wisemed/{setup,bootstrap}`, `/api/app-update/*`, `/api/wss/`, `/api/status`, `/api/dashboard`, `/api/logs`, `/api/analytes*`, `/api/orders*`, `/api/order-image`, `/api/daily-details*`, `/api/daily-analysis-filters*`, `/api/qc*`, `/api/results/default`, `/api/result-sync/*`, `/api/debug/tests*` |
| `storage-sqlite` | `storage/sqlite` | deschide SQLite (`modernc.org/sqlite`), servicii `storage`, `storage-meta` | așteaptă ctx, închide DB | `/api/storage/meta` |
| `events` | `events` | servicii `analyzer-activity`, `events` | — | `/api/events/meta` |
| `wisemed-ws` | `ws` | stare, nonce, dispatcher; 3 servicii | buclă conectare/reconectare (retry 30 s), `wake` | — |
| `wisemed-api` | `wisemedapi` | client HTTP, serviciu `wisemed-api` | `EnsureEquipmentInitialized` dacă setup complet, apoi așteaptă | `/api/wisemed/meta` |
| `login` | `login` | placeholder | — | `/api/login/meta` |
| `help` | `help` | servește `help_dir` | — | `/help`, `/help/` |
| `html-print-formatter` | `htmlprintformatter` | setări, `http-server-options` (suprascrie portul din env `PORT`) | `NewRenderer` (Chromium headless), închide la ctx | `/render*`, `/pdf`, `/health`, `/api/html-print-formatter/*` |
| `wisemed-ai` | `wisemedai` | cheie criptare, pool pgx + migrări/bootstrap dacă `database_url`, `http-server-options` | ticker 10 s (mirror modele, billing, outbox, retenție) | `/ai`, `/ai/`, `/ai/voice`, `/v1/`, `/api/wisemed-ai/v1/`, `/health/ai`, `/ready/ai` |
| `dashboard` | `dashboard` | meniu + serviciu descriptiv | — | `/api/dashboard/meta` |
| `analytes` | `analytes` | meniu + serviciu descriptiv | — | `/api/analytes/meta` |
| `analyte-management` | `analytemanagement` | meniu + serviciu descriptiv | — | `/api/analytes/management/meta` |
| `qc` | `qc` | meniu, niveluri QC | — | `/api/qc/meta` |
| `result-sync` | `resultsync` | serviciu `result-sync` | ticker `interval_minutes` (implicit 5), `runOnce` | — |
| `signing-pad` | `signingpad` | driver nativ, istoric SQLite opțional, înregistrare în dispatcher WS | cu `shared_http`: așteaptă; altfel listener propriu | `/api/esignature/*`, `/ws`, `/ws/demo`, `/esignature`, `/health`, `/signing-pad/*`, opțional `/api/signing-pad/*`, `/` doar fără `shared_http` |
| `siui` | `siui` | validare URL-uri CNAS, profile certificat, cere `local-http-control` | lock de fișier, SQLite joburi, worker istoric, ticker callback | `/api/siui/`, `/api/siui-history-sync/`, `/history-sync-ui` |
| `siui-data` | `siuidata` | setări, cheie JWT, cere `local-http-control` | watcher local + metrici la 2 s; închide pool PostgreSQL | `/api/siui-data/`, `/upload`, `/siui-data-*` |
| `siui-history` | `siuihistory` | setări MySQL/storage | așteaptă ctx, închide DB | `/api/history/`, `/history-ui` |
| `stats` | `stats` | meniu + serviciu descriptiv | — | `/api/stats/meta` |
| `daily-details` | `dailydetails` | definiții din config, serviciu `daily-details` | — | `/api/daily-details/meta` |
| `daily-orders` | `dailyorders` | meniu | — | `/api/orders/meta` |
| `astm-specimen-settings` | `astmspecimen` | — | doar așteaptă ctx | `/api/settings/astm-specimens` |
| `transport-file` | `transports/file` | — | creează directoarele, înregistrează serviciul `transport-file` | — |
| `transport-serial` | `transports/serial` | — | doar log + așteaptă (portul e deschis de protocol) | — |
| `transport-tcpip` | `transports/tcpip` | — | doar log + așteaptă (socketul e deschis de protocol) | — |
| `protocol-generic-file` | `protocols/genericfile` | **placeholder (22 linii), fără importator** | — | `/api/protocol/meta` |
| `protocol-cary60-uvvis` | `protocols/cary60uvvis` | meniu, `file-importer` | scan la `poll_seconds` (2) | `/api/protocol/meta` |
| `protocol-shimatzu-tocl` | `protocols/shimatzutocl` | meniu, `file-importer` | scan periodic | `/api/protocol/meta` |
| `protocol-cfx96-quantitation`, `-analytikjena-plasmaquantms-elite`, `-seegene-excel`, `-biosan-hipo-mpp96`, `-gammavision`, `-shimatzu-generic`, `-tricarb-5110-tr`, `-anatolia-geneworks` | `protocols/<nume>` peste `protocols/fileimportbase` (`Spec`) | meniu, `file-importer` | scan la `poll_seconds` (2) | `/api/protocol/meta` |
| `protocol-beosl-csv` | `protocols/beoslcsv` (wrapper peste `fileimportbase`) | în plus `result-publisher` | delegă la bază | `/api/protocol/meta` |
| `protocol-anaf-docsmart` | `protocols/anafdocsmart` | — | scan la `poll_seconds` (5) | `/api/protocol/meta`, `/api/docsmart/parsed-files` |
| `protocol-ir-biotyper` | `protocols/irbiotyper` | doar intrare de meniu | — | — |
| `protocol-astm` | `protocols/astm` | meniu, servicii status/replay | buclă TCP server/client, retry 3 s; inactiv dacă `comm_type != tcpip` | `/api/protocol/meta`, `/api/protocol/astm/status`, `/settings/protocol/astm` |
| `protocol-erba-mannheim-laura` | `protocols/erbamannheimlaura` | — | serial sau TCP după `comm_type`, retry 3 s | — |
| `protocol-horiba-abx-pentra400` | `protocols/horibaabxpentra400` | meniu | serial, retry 3 s | `/api/protocol/horiba-abx-pentra400/query-status`, `/protocol/horiba-abx-pentra400/query` |
| `protocol-biomerieux-minividas` | `protocols/biomerieuxminividas` | meniu | serial, retry 3 s | — |
| `protocol-labnovation-ld560` | `protocols/labnovationld560` | meniu, servicii status/replay, `restartCh` | TCP server/client, reconfigurare la cald | `/api/protocol/meta`, `/api/protocol/labnovation-ld560/*`, `/settings/protocol/labnovation-ld560` |
| `protocol-zonci-xl1000i` | `protocols/zoncixl1000i` | — | TCP server sau serial, retry 3 s | — |
| `barcode-printer` | `barcodeprinter` | SQLite log joburi, înregistrare în dispatcher WS | așteaptă ctx, închide DB | `/barcode/*`, `/api/barcode/*` |
| `app-update-server` | `appupdateserver` | SQLite, sesiuni | **listener propriu** pe adresa din `local-http` | `/`, `/settings`, `/help-ui`, `/app-updates/*`, `/api/session*`, `/api/update-server/*`, `/api/public/*` |
| `runner-startup-updates` | `apps/runner` | reîncarcă config | verificare update în fundal | — |

Toate rândurile: CDC (funcțiile `Init`/`Start` din `module.go` al fiecărui pachet; liniile exacte sunt în §2.2 și în `register.go:55-101`). Erorile de comunicație ale protocoalelor nu se propagă niciodată în afara `Start` (retry intern), deci nu opresc runtime-ul.

---

## 4. Configurare

- **Structură YAML** (`core/config/config.go:11` `Config`): `reader{id,label,analyzer_name,analyzer_code,db_name}`, `logging{verbose_level}`, `results{auto_confirm_wisemed}`, `local_http{enabled,address,language,tls,cors_allowed_origins}`, `wisemed_ws{enabled,url,heartbeat_ms,reconnect_delay_ms}`, `analyzer{comm_type,protocol}`, `modules: map[id]map[string]interface{}`. Setările de modul sunt netipizate și citite cu `rt.ModuleSettings(id)`. CDC.
- **Oglinzi top-level ↔ `modules.*`** (`ApplyDefaults`, `config.go:79`): `local_http` ↔ `modules.local-http`, `logging` ↔ `modules.logging`, `results` ↔ `modules.results`, `wisemed_ws` ↔ `modules.wisemed-ws` (valorile din modul au prioritate; `syncWiseMedWSMirror`, `config.go:260`). `modules.logging` și `modules.results` nu sunt module reale, doar secțiuni de setări. CDC.
- **Valori forțate**: `local_http.enabled` devine mereu `true` (`config.go:106-108`, apoi a doua sincronizare o scrie în modul) → HTTP-ul local nu poate fi dezactivat din config; `reconnect_delay_ms` e forțat la 30000 (`config.go:293-294`); `verbose_level` limitat la 1..5; CORS implicit `https://ldse.wisemed.eu`; adresă implicită `127.0.0.1:18080`. CDC.
- **`config.install.yaml` → `config.yaml`** (`install.go:19` `Ensure`): șablonul e căutat lângă config (`InstallTemplatePath`, `:71`). Lipsă config + șablon → copie a șablonului; lipsă ambele → `Default()` salvat; ambele prezente → `migrateSIUIDataMap` → `preserveLegacyConnectionSettings` → `mergeMissing` (adaugă doar cheile absente sau `nil`, recursiv pe map-uri; listele nu sunt îmbinate), scriere doar dacă s-a schimbat ceva. CDC.
- **Efect secundar al scrierii**: `writeMap` și `Config.Save` re-serializează prin `yaml.Marshal` pe map/struct → comentariile și ordinea cheilor din `config.yaml` se pierd (INF din utilizarea `yaml.v3` pe `map[string]interface{}`). Permisiuni `0644`, dar `0600` când `analyzer.protocol == siui-data` (`install.go:142-149`, `config.go:135-142`). CDC.
- **Migrări**: `siu-data` → `siui-data` (protocol, listă module, secțiune de modul) atât pe struct (`siui_data_migration.go:8`), cât și pe map-ul brut înainte de merge (`:32`); WSS: copierea cheilor `wisemed_ws` vechi în `modules.wisemed-ws` și mutarea `address/tls` din `modules.signing-pad` în `local_http` cu `shared_http: true` (`install.go:193` `preserveLegacyConnectionSettings`, dublat la încărcare în `runner.go:183`); `app_id` `shimatzu-generic-v3` → `shimatzu-generic-reader` (`runner.go:209-224`). Teste existente: `core/config/siui_data_migration_test.go`, `core/config/wss_test.go`, `apps/runner/wss_config_test.go` (nerulate aici). CDC.
- **Actualizări punctuale din UI**: `config.Update(path, map["a.b.c"]valoare)` (`install.go:75`), folosit de ex. în `modules/astmspecimen/module.go` (PUT `/api/settings/astm-specimens`). CDC.
- **Path-uri relative**: `App.ResolvePath` le rezolvă față de directorul fișierului de config; șir gol → directorul configului (`runtime.go:67-75`); echivalent în runner: `resolveConfigPath` (`updates.go:183`). Logurile și fișierul PID stau tot în directorul configului. CDC.
- **`reader_id`**: `reader.id` din YAML, expus prin `Runtime.ReaderID()` (`runtime.go:58`); promptat la bootstrap cu implicit derivat din `analyzer_code`/label (`bootstrap.go:59-62,664`); folosit ca prefix de log/PID (`logging.go:57`), nume de serviciu OS (`service_common.go:32`) și fallback pentru `app_id` la update (`updates.go:39`). CDC.
- **Versiune**: `readersv3/shared/appmeta/appmeta.go` — variabile `Version` (implicit `0.0.0-dev`), `Commit`, `BuildTime`, `VersionMarker`, injectate cu `-ldflags -X` de `readersv3/tools/releasectl/main.go:554-562`; `CurrentVersion()` elimină prefixul `v`. Configul nu conține versiunea. CDC.
- **Server de update implicit**: toate șabloanele au `modules.app-updates.base_url: http://127.0.0.1:19090`, iar wizardul propune aceeași valoare (`bootstrap.go:189`). CDC. Care este URL-ul de producție: NEC.

---

## 5. Catalogul aplicațiilor (32, fără `runner`)

`go list ./apps/...` din `readersv3/` listează 32 de pachete `main` de aplicație + `runner` + 2 utilitare `apps/wisemed-ai/tools/{catalog,import-postgres}` (CTL).

Seturi de module (ordinea exactă din cod):
- **R** (bază reader, 14): `local-http, storage-sqlite, events, wisemed-api, wisemed-ws, login, help, dashboard, analytes, analyte-management, qc, result-sync, stats, daily-orders`
- **U** (bază utilitar, 6): `local-http, events, wisemed-api, wisemed-ws, login, help`

Flag-uri CLI: toate cele 31 de aplicații bazate pe `runner` au exact `-config` (implicit `deployments/config.yaml`), `-headless`, `-headless-child`, `-installservice`, `-reconfigure`, `-showlog`, `-version`. `update-server` are doar `-config`, `-reconfigure`, `-version`. CDC (`apps/*/main.go:13-19`).

### 5.1 Readere de analizoare (22)

Toate: entrypoint `readersv3/apps/<app>/main.go`, `runner.Run` cu `RunOptions` standard, SQLite prin `storage-sqlite` la `./<db>.db` (relativ la `deployments/`), HTTP local `127.0.0.1:18080` fără TLS. Sursă: `main.go:25` și `deployments/config.install.yaml` ale fiecărei aplicații. CDC.

| Aplicație | Module | Transport / protocol implicit | Port analizor implicit | WSS implicit | Particularități |
|---|---|---|---|---|---|
| `analytikjena-plasmaquantms-elite-reader` | R + `transport-file`, `protocol-analytikjena-plasmaquantms-elite` | file / `analytikjena-plasmaquantms-elite`, `*.csv` | — | off | — |
| `anatolia-geneworks-reader` | R + `transport-file`, `protocol-anatolia-geneworks` | file / `anatolia-geneworks`, `*.xls` | — | off | — |
| `autobio-autolumo-s900-reader` | R + `astm-specimen-settings`, `transport-tcpip`, `protocol-astm` | tcpip server / `astm` | `0.0.0.0:11119` | on | — |
| `beosl-reader` | R + `transport-file`, `protocol-beosl-csv` | file / `beosl-csv`, `*.csv` | — | on | publică dozimetrie în WiseMED (`result-publisher`) |
| `biomerieux-minividas-reader` | R + `transport-serial`, `protocol-biomerieux-minividas` | serial 9600 / `biomerieux-minividas` | port serial gol | on | — |
| `biosan-hipo-mpp-96-reader` | R + `transport-file`, `protocol-biosan-hipo-mpp96` | file / `biosan-hipo-mpp96`, `*.csv` | — | off | — |
| `cary60-uvvis-reader` | R (cu `daily-details` inserat înainte de `daily-orders`) + `transport-file`, `protocol-cary60-uvvis` | file / `cary60-uvvis`, `*.csv` | — | off | singura aplicație cu `daily-details` |
| `cfx96-reader` | R + `transport-file`, `protocol-cfx96-quantitation` | file / `cfx96-quantitation`, `*Quanti* Plate View Results.xlsx` | — | on | — |
| `erba-mannheim-laura-reader` | R + `transport-serial`, `transport-tcpip`, `protocol-erba-mannheim-laura` | serial 9600 (implicit) sau tcpip / `erba-mannheim-laura` | TCP `127.0.0.1:5150` | on | două transporturi; are `README.md`, `docs/` |
| `gammavision-reader` | R + `transport-file`, `protocol-gammavision` | file / `gammavision`, `*.txt` | — | off | — |
| `gemini-reader` | R + `astm-specimen-settings`, `transport-tcpip`, `protocol-astm`, `protocol-ir-biotyper` | tcpip server / `astm` | `127.0.0.1:9000` | on | două module de protocol (`ir-biotyper` e doar meniu) |
| `generic-test-reader` | R + `transport-file`, `protocol-generic-file` | file / `generic-file`, `*.txt` | — | on | modulul de protocol e placeholder — fără `file-importer` (vezi §8) |
| `horiba-abx-pentra400-reader` | R + `astm-specimen-settings`, `transport-serial`, `protocol-horiba-abx-pentra400` | serial 9600 / `horiba-abx-pentra400` | port serial gol | on | șablonul are secțiune `protocol-astm` deși modulul nu e activat |
| `horiba-yumizen-h500-reader` | R + `astm-specimen-settings`, `transport-tcpip`, `protocol-astm` | tcpip server / `astm` | `127.0.0.1:5150` | on | — |
| `labnovation-ld-560` | R + `transport-tcpip`, `protocol-labnovation-ld560` | tcpip server / `simple` (și `hl7`) | `0.0.0.0:8000` | on | fără `deployments/help`; are `docs/`; `reader.id` fără sufix `-v3`, `app_id` diferit (`labnovation-ld-560-reader-v3`); cheie de config greșită (vezi §8) |
| `lifotronic-h8-reader` | R + `astm-specimen-settings`, `transport-tcpip`, `protocol-astm` | tcpip server / `astm` | `127.0.0.1:8000` | on | — |
| `seegene-reader` | R + `transport-file`, `protocol-seegene-excel` | file / `seegene-excel`, `*.csv` | — | on | — |
| `shimatzu-generic-reader` | R + `transport-file`, `protocol-shimatzu-generic` | file / `shimatzu-generic`, `*.txt` | — | off | `deployments/config.yaml` este **versionat în git**; normalizare `app_id` în runner |
| `shimatzu-tocl-reader` | R + `transport-file`, `protocol-shimatzu-tocl` | file / `shimatzu-tocl`, `*.txt` | — | off | — |
| `snibe-maglumi-x3` | R + `astm-specimen-settings`, `transport-tcpip`, `protocol-astm` | tcpip server / `astm` | `127.0.0.1:9000` | on | — |
| `tricarb-5110-tr-reader` | R + `transport-file`, `protocol-tricarb-5110-tr` | file / `tricarb-5110-tr`, `*.csv` | — | off | — |
| `zonci-xl1000i-reader` | R + `transport-tcpip`, `transport-serial`, `protocol-zonci-xl1000i` | tcpip server (implicit) sau serial 115200 / `zonci-xl1000i` | `0.0.0.0:3125` | on | **fără director `build/`**; binar `readersv3/zonci-xl1000i-reader.exe` versionat în rădăcina `readersv3/` |

### 5.2 Utilitare și servere (10)

| Aplicație | Module (ordine) | Rol / protocol | Bază de date | Port implicit | `RunOptions` / particularități |
|---|---|---|---|---|---|
| `anaf-docsmart` | `local-http, wisemed-api, wisemed-ws, login, help, transport-file, protocol-anaf-docsmart` | utility / `anaf-docsmart`, `*.xml` | niciuna (fără `storage-sqlite`, **fără `events`**) | `127.0.0.1:18120` | standard; directoare `inbox/outbox/processed/failed/work/assets` în sursă; `internal/docsmart` este **director gol** |
| `barcodeprinter` | U + `barcode-printer` | utility / `barcodeprinter`; `GET/POST /barcode/print` + WS `barcode.print` | SQLite propriu `./barcodeprinter-log.db` (`log_db_path`) | `127.0.0.1:8010` | șablonul conține secțiuni `storage-sqlite` și `transport-file` neutilizate |
| `esignature-server` | U + `signing-pad` | utility / `signing-pad` | SQLite opțional (`log_db_path`; niciuna dacă e gol) | `127.0.0.1:19111` **TLS** | `SkipBootstrap: true`; `bin/eSignature` (conținut neverificat), `build-windows.sh` cu țintă `windows-386`, `tests/`, doar `build/windows` |
| `signing-pad-utility` | U + `signing-pad` | utility / `signing-pad` (aceleași module ca `esignature-server`) | SQLite opțional | `0.0.0.0:19110`, `device_port 10000` | bootstrap activ; `java/` (helper JAR + surse + `build-helper.sh`); șablon **fără secțiune `app-updates`** |
| `html-print-formatter` | U + `html-print-formatter` | utility / `html-print-formatter` (Chromium headless prin `chromedp`) | niciuna | `127.0.0.1:19113` (suprascris de env `PORT`) | `SkipBootstrap: true`; `docker/`, `examples/`, `generate-help.py`; cont de sistem dedicat la instalare pe Linux |
| `siui-bridge` | U + `siui` | utility / `siui` (CNAS SIUI/SIPE/DocMed) | SQLite `./siui-jobs.db` + fișier `.lock` | `127.0.0.1:19112` **TLS** | standard; `tests/`, `docs/`; secțiune `siui-history-sync` |
| `siui-bridge-history` | U + `siui-history` | utility / `siui-history` | MySQL (`go-sql-driver/mysql`, deschis leneș) + stocare `local`/S3 | `127.0.0.1:19113` **TLS** | standard; **același port implicit ca `html-print-formatter`** |
| `wisemed-siui-data` | U + `siui-data` | utility / `siui-data` | PostgreSQL (`pgxpool`) | `127.0.0.1:19114` **TLS** | `BackgroundStartupUpdates: true`; bootstrap dedicat `siui_data_setup.go`; config `0600` |
| `wisemed-ai` | U + `wisemed-ai` | utility / `wisemed-ai` | PostgreSQL (`pgxpool`, conectare + migrare în `Init` dacă `database_url`) | `127.0.0.1:19120` | `SkipBootstrap: true, BackgroundStartupUpdates: true`; `web/` (Vite + Playwright), `clients/flutter`, `tools/{catalog,import-postgres}`, `docker/` |
| `update-server` | `wisemed-api, login, help, app-update-server` (**fără `local-http`**) | utility / `app-update-server` | SQLite `./app-update-server.db` + `./files` | `0.0.0.0:19090` | **NU folosește `runner.Run`** (vezi §5.3); `bootstrap.go`, `Dockerfile`, `docker/ubuntu`; șablon fără `app-updates` |

Toate rândurile: CDC din `apps/<app>/main.go` și `apps/<app>/deployments/config.install.yaml`; driverele din `sql.Open`/`pgxpool` (`modules/barcodeprinter/module.go:528`, `signingpad/history.go:21`, `siui/jobs.go:42`, `siuihistory/module.go:204`, `siuidata/postgres.go:75`, `wisemedai/module.go:60`, `appupdateserver/module.go:871`).

### 5.3 Aplicații cu compunere atipică

- **`update-server`** (`apps/update-server/main.go:19-65`): compune direct `config.Ensure` → `config.Load` → `ensureUpdateServerBootstrap` (wizard propriu, `bootstrap.go:12`) → `builtin.RegisterAll` → `runtime.New` → `InitModules([...])` → `app.Start`. Consecințe: fără log în fișier (stdout), fără `-headless`/`-installservice`/`-showlog`, fără verificare de update, fără buclă de restart, fără `process-control`, un singur `Init`. Listenerul e deschis de modulul `app-update-server` pe adresa din `modules.local-http` (`appupdateserver/module.go:165-170`). CDC.
- **`SkipBootstrap`**: `esignature-server`, `html-print-formatter`, `wisemed-ai` — wizardul rulează doar cu `-reconfigure` (`runner.go:75`). CDC.
- **`BackgroundStartupUpdates`**: `wisemed-ai`, `wisemed-siui-data` — adaugă modulul `runner-startup-updates` (`runner.go:71-73`). CDC.
- **`signing-pad` + `local-http`**: ambele ar înregistra `/`; conflictul e evitat deoarece `normalizeLegacyConfig` forțează `shared_http: true` pentru protocolul `signing-pad` (`runner.go:189-207`, `signingpad/module.go:110-112`). CDC.
- **`anaf-docsmart`**: singura aplicație pe `runner` fără `events` și fără `storage-sqlite`. CDC.
- **Fișierul generat `readersv3/build/apps.json`** (ignorat de git) listează 28 de aplicații; lipsesc `siui-bridge`, `siui-bridge-history`, `wisemed-ai`, `wisemed-siui-data`. CDC pentru conținutul curent de pe disc; semnificația (livrare Docker separată?) — INF.

---

## 6. Concurență la nivel de runtime/runner și recuperare după restart

| Primitivă | Loc | Rol |
|---|---|---|
| `sync.RWMutex` `App.mu` | `core/runtime/runtime.go:22` | protejează `menu` și `services`; `modules` e scris doar în `InitModules` (fără lock) și citit fără lock în `handleModules` (`:168`) |
| `http.ServeMux` | `runtime.go:19` | sincronizare internă a `ServeMux`; rutele sunt înregistrate în `Init` |
| goroutine per `Starter` + `sync.WaitGroup` + `errCh` (buffer `len(modules)`) | `runtime.go:127-139` | lifecycle paralel; fără timeout la `Wait` |
| `restartCh chan string` (buffer 1) | `runner.go:248` | cerere de restart complet; `RequestRestart` e non-blocant și înlocuiește o cerere în așteptare (`runner.go:231-245`) |
| goroutine `app.Start` + `errCh` (buffer 1) | `runner.go:278-281` | o instanță runtime per iterație |
| `sync.Once` | `runner.go:38` | raportare unică a rezultatului pornirii către părintele headless |
| goroutine `cmd.Wait` + `ticker` 50 ms + `timer` 1 min | `headless_startup.go:27-33` | așteptarea copilului headless |
| goroutine `app.Start` + `errCh` | `service_windows.go:90-93` | serviciu Windows; `Stop/Shutdown` → `cancel()` → `<-errCh` |

Toate: CDC.

**Restart complet în proces** (`runAppLoop`): reîncarcă `config.yaml` (`config.Load`, fără `Ensure`, fără bootstrap, fără update-check, fără redeschiderea logului), construiește registry + `App` + mux noi, reinițializează toate modulele; starea în memorie a modulelor (sesiuni HTTP, nonce WS, cozi) se pierde; persistă doar ce e în SQLite/fișiere. Declanșat din UI la schimbarea setărilor readerului sau ale update-ului (`localhttp/module.go:972,2792`). CDC.

**Restart de proces**: nu există supervizor intern. Recuperarea depinde de OS: systemd `Restart=on-failure`, `RestartSec=5` (`service_linux.go:29-30`); launchd `KeepAlive` (`service_darwin.go:36-37`); SCM `restart/60000/restart/120000/restart/300000`, reset 86400 s (`service_windows.go:38`). În mod `-headless` (fără serviciu) nu există repornire automată; fișierul PID rămâne dacă procesul e ucis brutal (ștergerea e prin `defer`). CDC pentru configurație; comportamentul efectiv — DN.

**Reconectări interne** (fără restart de runtime): WSS la 30 s (`ws/module.go:319`), protocoale TCP/serial la 3 s (ex. `astm/module.go:211`), rebind HTTP la schimbarea adresei (`localhttp/module.go:386-404`), port ocupat → `bindguard.HandleAddressInUse` propune altă adresă și o scrie în config (`localhttp/module.go:413-423`). CDC.

Recuperarea stării de business după restart (fișiere pe jumătate importate, cozi de trimitere, joburi SIUI) nu a fost investigată aici — NEC (ține de module, nu de runtime).

---

## 7. Rădăcina repository-ului

- **`go.mod` din rădăcină**: `module wisemed-labreaders`, `go 1.19`, `replace wisemed-labreaders => ./`, o singură dependență `github.com/hooklift/gowsdl v0.5.0`. **Nu există niciun fișier `.go` în pachetul rădăcină** și nicio referință la `gowsdl` în cod/scripturi (singura mențiune e în `ARCHITECTURE.md:15`). Modulul e orfan; `readersv3/go.mod` (`wisemed-labreaders/readersv3`, `go 1.24.0`) și `wsm-server/go.mod` sunt module independente. CDC. Motivul păstrării (probabil generare istorică de clienți SOAP) — INF.
- **`old-src/`**: ignorat de git (`.gitignore`: `/old-src/`); conține surse C#/.NET istorice: `MedicalPAGES.SIUIBridge`, `WiseMEDeCARDManager`, `WiseMEDeSIGNATURE`. Zero fișiere Go. Rol: referință pentru portările `siui` și `signing-pad` — INF.
- **`DocSMART/`**: zero fișiere Go; în git sunt doar 6 fișiere de date `work/Df1102X35-*/{import.xml,template.pdf}`; `cmd/docsmart-importer` și `internal` există pe disc doar ca **directoare goale** (neversionate în git), plus `inbox/outbox/processed/failed`. Implementarea reală este modulul `readersv3/modules/protocols/anafdocsmart` + aplicația `readersv3/apps/anaf-docsmart` (care are aceeași structură de directoare de lucru și, la rândul ei, un `internal/docsmart` gol). `DocSMART/` este rămășița unui proiect standalone anterior — CDC pentru conținut, INF pentru istoric.
- **`output/`** (rădăcină): `siui-data-research/` (2 fișiere de audit ICD10) și `wsm-browser-sdk/` (binare `wsm-server`/`wsmctl` cu backup-uri, consolă `control/`, config-uri, documentație, exemple; singurul `.go`: `examples/reader/main.go`). 46 de fișiere versionate. Ține de `wsm-server`, nu de `readersv3`. `.gitignore` mai conține reguli pentru `/output/<analizor>/certs/` ale unor readere vechi (cobas, dirui, maglumi, sysmex…) care nu mai există pe disc. CDC.
- **`tmp/`**: ignorat de git; conține doar `tmp/pdfs/` cu un element. CDC.
- **Directoare șterse**: `comm/`, `implementation/`, `protocols/`, `server/` apar doar ca ștergeri de `.DS_Store` în indexul git — structura readerelor pre-v3 a fost eliminată. CDC (git status).

---

## 8. Contradicții cod ↔ documentație/memorie

| # | Documentație / memorie | Cod | Verdict |
|---|---|---|---|
| 1 | `readersv3/README.md` („Stadiu”): „aplicații v3 pentru cele patru readere” | 32 de aplicații în `readersv3/apps/` (CTL prin `go list`) | README depășit |
| 2 | `aimemory/wisemed-labreaders.md:7-8`: „all 26 equipment apps” | 32 de aplicații; `build/apps.json` are 28 | Memorie depășită |
| 3 | `aimemory/wisemed-labreaders.md:10-14,48-70,402-440`: proiectul activ e `Readers Last/generic-test-reader` cu `cmd/`, `internal/reader`, `internal/webui`; secțiuni YAML `wisemed_api`, `communication`, `layout`, `capabilities`; `serial`/`network` neimplementate | Directorul `Readers Last/` nu există; arhitectura reală este `readersv3/{core,modules,apps}`; secțiunile YAML sunt cele din `core/config/config.go:11-21`; serial și TCP sunt implementate în modulele de protocol | Jumătatea de început a memoriei descrie o generație anterioară, inexistentă în repo |
| 4 | `README.md` („Model”): toate aplicațiile standard activează 12 module, fără `local-http` și `result-sync` în listă | Toate readerele activează 14 (`local-http` și `result-sync` incluse), ex. `apps/generic-test-reader/main.go:25` | Listă incompletă în README |
| 5 | `README.md`: `generic-test-reader` = „file transport + generic file protocol” | `modules/protocols/genericfile/module.go` are 22 de linii: doar `/api/protocol/meta`, fără parser, fără `file-importer`, fără `Start`; `localhttp` răspunde 400 „file import backend is not available” când `importer == nil` (`localhttp/module.go:1694-1697`) | Protocolul generic nu importă fișiere în v3 (CDC pentru lipsa importatorului; efectul în UI — INF) |
| 6 | `README.md`: `seegene-reader` = „seegene excel protocol”; wizardul propune `*.xlsx` pentru seegene (`bootstrap.go:246-248`) | Șablonul are `pattern: "*.csv"`, iar modulul răspunde `SEEGENE_VIEWER_CSV` (`protocols/seegeneexcel/module.go:22`) | Denumire/implicit inconsistente |
| 7 | `README.md`: `gemini-reader`/`snibe-maglumi-x3` = „tcp/ip transport + astm protocol”, sugerând că transportul face comunicația | `transport-tcpip` și `transport-serial` sunt no-op (`transports/tcpip/module.go:14-18`); socketul/portul serial sunt deschise de modulul de protocol | Decuplarea transport/protocol din README nu există pentru TCP/serial |
| 8 | `README.md`: modulele `login`, `dashboard`, `stats`, `analytes`, `analyte-management` prezentate ca funcționalitate shared | Aceste module înregistrează doar meniu + endpoint `/meta` (ex. `login/module.go:13-22` „login module placeholder”); logica reală (sesiuni, dashboard, analize, QC) este în `modules/localhttp/module.go` (3954 linii) și `storage/sqlite` | Modularitatea e parțial nominală |
| 9 | `readersv3/AGENTS.md`: orice utilitar trebuie să configureze `app-updates` („Do not omit updates”) | `apps/signing-pad-utility/deployments/config.install.yaml` și `apps/update-server/deployments/config.install.yaml` nu au secțiunea `app-updates`; `update-server` nu apelează deloc `checkForUpdates` | Regulă nerespectată în 2 șabloane (pentru signing-pad wizardul o va cere interactiv: `bootstrap.go:326-332`) |
| 10 | `AGENTS.md`: „standard update-server URL” | Implicit peste tot `http://127.0.0.1:19090` (șabloane + `bootstrap.go:189`) | URL-ul „standard” nu e definit în cod |
| 11 | `AGENTS.md`: fiecare reader are scripturi de build/installer și help | `apps/zonci-xl1000i-reader` nu are `build/`; `apps/labnovation-ld-560/deployments` nu are `help/` | Layout incomplet |
| 12 | `README.md`: `-headless` „păstrează WS și local HTTP/HTTPS active” fără alte efecte | Părintele și copilul rulează fiecare Ensure + bootstrap + update-check (`runner.go:52-124`) | Omisiune în documentație |
| 13 | `README.md`: lifecycle prezentat ca unic | `Init` rulează de două ori pe darwin/linux (`runner.go:143` și `:264`) | Nedocumentat; posibilă scurgere de resurse |
| 14 | `apps/labnovation-ld-560/deployments/config.install.yaml:79`: secțiune `protocol-labnovation-ld-560` | Modulul citește `ModuleSettings("protocol-labnovation-ld560")` (`labnovationld560/module.go:650`; ID la `:73`) | Setările din șablon sub cheia greșită nu sunt citite de modul |
| 15 | `.gitignore`: `/readersv3/output/*/deployments/config.yaml` ignorat; README: `config.yaml` nu intră în payload | `readersv3/apps/shimatzu-generic-reader/deployments/config.yaml` este versionat (în `apps/`, nu în `output/`) | Config runtime în sursă |
| 16 | `ARCHITECTURE.md:15`: rolul modulului rădăcină „de stabilit” | Fără fișiere `.go`, fără utilizări `gowsdl` | Modul orfan (clarificare, nu contradicție) |

Conflicte de porturi implicite (nu contradicție cu documentația, dar relevant): toate cele 22 de readere folosesc `127.0.0.1:18080`; `html-print-formatter` și `siui-bridge-history` folosesc ambele `19113`; `horiba-yumizen-h500` și `erba` folosesc ambele TCP `5150`; `gemini` și `snibe` `9000`; `labnovation` și `lifotronic` `8000`. Pentru HTTP există `bindguard`; pentru porturile de analizor, `net.Listen` eșuează și protocolul reîncearcă la 3 s. CDC.

---

## 9. Necunoscute / zone neinvestigate

- **Efectul real al dublei inițializări** (handle-uri SQLite/pgx scurse, migrări `wisemed-ai` rulate de două ori, `initNative` al `signing-pad` apelat de două ori): dedus din cod, neconfirmat la runtime. NEC.
- **Comportamentul ca serviciu Windows** (timeout SCM în timpul `Init`, lipsa `process-control`): neverificat. NEC.
- **Importul de fișiere în `generic-test-reader`**: importul manual din UI răspunde 400 fără `file-importer` (`localhttp/module.go:1694-1697`); nu am urmărit dacă există altă cale (ex. scan automat) pentru fișierele din `inbox`. NEC.
- **Interiorul modulelor mari**: `localhttp` (sesiuni, WSS remote, debug replay), `ws.runSession`, `resultsync.runOnce`, `storage/sqlite` (schema, migrări), `siui*`, `wisemedai`, `appupdateserver` — citite doar `Init`/`Start` și înregistrările de rute/servicii. Inventarul detaliat de endpoint-uri e în sarcina altui raport.
- **`shared/*`** (`bindguard`, `localtls`, `appupdates`, `apibridge`, `commtrace`, `debugreplay`, `history`, `legacy`, `astm`): necitite, cu excepția `appmeta` și a semnăturii `appupdates.ResolveBaseURL`.
- **`update_apply.go`**: citite doar semnăturile; scripturile de înlocuire a binarului nu au fost analizate.
- **`tools/releasectl`**, `Makefile`, scripturile `build-*/package-*`, `installer/`, `docker/`, `scripts/prepare-reader-output.sh`: doar injectarea versiunii a fost verificată.
- **`readersv3/output/`** (config-uri runtime reale) și bazele de date: deliberat neatinse; valorile efective din instalări pot diferi de șabloane.
- **`wsm-server/`**: în afara scopului.
- **`apps/esignature-server/bin/eSignature`**, `apps/signing-pad-utility/java/`, `apps/wisemed-ai/{web,clients}`: doar existența a fost constatată.
- **Testele** (`core/runtime/lifecycle_test.go`, `core/config/*_test.go`, `apps/runner/*_test.go`) nu au fost rulate; nicio afirmație de aici nu e `CONFIRMAT PRIN TEST LOCAL` în afara listei de pachete din `go list`.
- **URL-ul de producție al serverului de update și al WSM** (`wss://wslocal.wisemed.eu/ws` e doar implicitul din șabloane): NEC.
