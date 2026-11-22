# Runtime și module (`readersv3`)

| | |
|---|---|
| **Scop** | Descrie lifecycle-ul real al aplicațiilor `readersv3`: ordinea de inițializare, contractele `Module`/`Runtime`/`Starter`/`MenuEntry`, registry-ul de module și de servicii, configurarea, concurența la nivel de runtime și restartul. |
| **Data** | 2026-11-14 (commit de bază `d53088e`) |
| **Surse** | `_reports/01-runtime-and-apps.md` (principală); `_reports/02-wsm-security.md §6` (clientul `wisemed-ws`); `_reports/03-laboratory.md §0–§1` (transporturi, module nominale); `_reports/06-qa-build-ops.md` B.1, B.3 (ce a fost rulat) |
| **Limite** | Niciun binar nu a fost pornit: toate efectele la runtime (handle-uri scurse, comportament sub systemd/SCM) sunt inferențe din cod. Interiorul modulelor mari (`localhttp`, `storage/sqlite`, `siui*`, `wisemedai`) este tratat aici doar la nivel de `Init`/`Start`/rute; vezi documentele de domeniu. `wsm-server` nu folosește acest runtime și nu este acoperit aici. |

Etichete: **CDC** = `CONFIRMAT DIN COD` · **CTL** = `CONFIRMAT PRIN TEST LOCAL` (numai rulările din `_reports/06-qa-build-ops.md` B.1) · **DN** = `DOCUMENTAT, NEVERIFICAT` · **INF** = `INFERENȚĂ` · **NEC** = `NECUNOSCUT`. Căile fără prefix sunt relative la `readersv3/`.

Documente înrudite: [application-catalog.md](application-catalog.md) (ce module activează fiecare aplicație), [build-deploy-update.md](build-deploy-update.md) (serviciu OS, update), [communication-map.md](communication-map.md), [api-catalog.md](api-catalog.md), [data-and-state-machines.md](data-and-state-machines.md), [security-and-identity.md](security-and-identity.md), [verification-matrix.md](verification-matrix.md), [gaps-and-contradictions.md](gaps-and-contradictions.md).

Stare de verificare generală — CTL: `go build ./...`, `go vet ./...`, `go test ./...` și `go test -race ./...` trec în `readersv3/` pe darwin/arm64 (`_reports/06` B.1 rândurile 1–4). `apps/runner` (6 fișiere de test) și `core/runtime`, `core/config` au teste care au rulat în această suită; `core/module` și `apps/runner/service_*.go` nu au teste (`_reports/06` B.3). Testele nu acoperă fișierele `*_windows.go`/`*_linux.go`.

---

## 1. Ordinea reală de inițializare

### 1.1 Secvența standard (`main` → `runner.Run`)

Valabilă pentru 31 de aplicații. `update-server` are o secvență scurtată (§1.4).

| # | Pas | Referință |
|---|---|---|
| 1 | `main` parsează 7 flag-uri, tratează `-version`, construiește lista **hardcodată** de module, apelează `runner.Run(cfgPath, modules, RunOptions{...})`; eroarea → `log.Fatal` | `apps/generic-test-reader/main.go:12-28` |
| 2 | `config.Ensure(configPath)`: creează `config.yaml` din `config.install.yaml` sau face merge non-distructiv | `apps/runner/runner.go:52`, `core/config/install.go:19` |
| 3 | `config.Load` (YAML → `Config`, `ApplyDefaults`) + `normalizeLegacyConfig` | `runner.go:56-60`, `runner.go:183` |
| 4 | `cfg.EnabledModules = defaultModules` (+ `runner-startup-updates` dacă `BackgroundStartupUpdates`). Lista de module **nu vine din YAML** (`EnabledModules` are tag `yaml:"-"`) | `runner.go:70-73`, `core/config/config.go:19` |
| 5 | Bootstrap interactiv `ensureBootstrap` dacă `!SkipBootstrap \|\| Reconfigure`. Eroarea e non-blocantă (warning + `ApplyDefaults`), **cu excepția** protocolului `siui-data`, unde e fatală | `runner.go:74-92`, `apps/runner/bootstrap.go:15` |
| 6 | `setupLogging`: arhivează săptămânile încheiate, deschide `<configDir>/<reader-id>-YYYYMMDD.log`, redirecționează `log` global | `runner.go:93`, `apps/runner/logging.go:18` |
| 7 | Dacă bootstrap-ul a modificat ceva: prompt `[Y/n]` (`confirmSaveAndStart`), apoi `cfg.Save()`; refuz → `return nil` | `runner.go:103-112`, `bootstrap.go:336` |
| 8 | `-installservice` → `installService(cfg)` și ieșire, fără a porni module | `runner.go:113-115` |
| 9 | Verificare de update sincronă `checkForUpdates` (dacă nu e `BackgroundStartupUpdates`); `errUpdateStarted` → ieșire curată; altă eroare → warning | `runner.go:116-124`, `apps/runner/updates.go:22` |
| 10 | `-headless` (părinte): relansează binarul cu `-config X -headless-child`, așteaptă rezultatul pornirii (max 1 min), tipărește PID/log/pidfile și **iese** | `runner.go:125-138`, `headless_unix.go:18`, `headless_windows.go:23`, `headless_startup.go:16` |
| 11 | **Prima** instanță runtime: `module.NewRegistry` + `registerRunnerModules` + `runtime.New` + `app.InitModules(cfg.EnabledModules)` | `runner.go:139-145` (`InitModules` la `:143`) |
| 12 | `runServiceManager(cfg, app)`: pe Windows, dacă procesul e serviciu SCM → `svc.Run` cu `app.Start` și return; pe darwin/linux întoarce mereu `(false, nil)` | `runner.go:146-148`, `service_windows.go:66`, `service_unix.go:10` |
| 13 | `signal.NotifyContext(SIGINT, SIGTERM)`; copilul headless scrie `<reader-id>.pid` (șters prin `defer`) | `runner.go:149-162` |
| 14 | `runAppLoop`: **reîncarcă** config, construiește un **al doilea** registry + `runtime.App`, înregistrează `process-control`, rulează din nou `InitModules`, apoi `app.Start(runCtx)` într-o goroutine | `runner.go:247-282` (`InitModules` la `:264`) |
| 15 | `App.Start`: câte o goroutine per modul care implementează `Starter`; mux-ul HTTP e servit de `local-http.Start` | `core/runtime/runtime.go:124-149`, `modules/localhttp/module.go:299-332` |
| 16 | Oprire: semnal → `ctx.Done` → `cancel()` → `<-errCh` → `return nil`; `closeLog(runErr)` prin `defer` | `runner.go:283-287`, `runner.go:97` |

Toate rândurile: CDC (`_reports/01 §1.1`); liniile `:143` și `:264` au fost reverificate direct în cod.

### 1.2 Dublul `Init`

Pe darwin și linux, pașii 11 și 14 inițializează **toate modulele de două ori** — CDC (`runner.go:143` și `:264`). Prima instanță `runtime.App` este abandonată fără `Start`, deci:

- resursele deschise în `Init` nu au cine să le închidă (contractul nu are `Stop`/`Close`, iar curățarea se face doar la ieșirea din `Start`): handle-ul SQLite din `storage-sqlite` (`modules/storage/sqlite/module.go:56`), DB-ul de jurnal din `barcode-printer` (`modules/barcodeprinter/module.go:66`), istoricul `signing-pad` (`modules/signingpad/module.go:78`), pool-ul pgx și migrările din `wisemed-ai` (`modules/wisemedai/module.go:51-78`);
- efectul concret (handle-uri scurse, migrări rulate de două ori la fiecare pornire, `initNative` al `signing-pad` apelat de două ori) este **INF** — neconfirmat la runtime (`_reports/01 §1.2, §9`);
- `update-server` nu este afectat (un singur `Init`, §1.4);
- nicio documentație din repository nu menționează dubla inițializare (`_reports/01 §8 #13`).

**Serviciul Windows folosește prima instanță** (`service_windows.go:78,92`): aceasta nu are serviciul `process-control` și nu trece prin `runAppLoop`, deci restartul complet cerut din UI (`modules/localhttp/module.go:972,2792`) nu este disponibil; `processControl()` întoarce `nil` (`modules/localhttp/module.go:3092-3099`) — CDC. Comportamentul efectiv ca serviciu Windows (inclusiv timeout SCM în timpul `Init`): NEC.

### 1.3 Alte observații despre secvență

- **Copilul headless repetă pașii 2–9** (Ensure, bootstrap, logging, update-check): nu există ramură pe `HeadlessChild` în `runner.go:52-124`, deci verificarea de update se face de două ori la `-headless` — CDC.
- **Bootstrap-ul citește `os.Stdin`** (`bootstrap.go:24`); copilul headless are stdin pe `/dev/null` (`headless_unix.go:36`), deci un config incomplet produce prompturi cu EOF → valori implicite — INF.
- **Logul zilnic nu se rotește în timpul rulării**: fișierul e ales o dată în `setupLogging` (`logging.go:30`), arhivarea săptămânală rulează doar la pornire (`logging.go:27`) — CDC pentru cod; efectul la procese de lungă durată — INF (vezi [build-deploy-update.md §7](build-deploy-update.md#7-observabilitate-și-loguri)).
- **Raportarea pornirii către părintele headless**: prin fișierul temporar din env `WISEMED_HEADLESS_STARTUP_FILE`; succesul e semnalat de `local-http` prin serviciul `local-http-ready` (`runner.go:271`, `modules/localhttp/module.go:369`) — CDC.

### 1.4 Secvența `update-server`

`apps/update-server/main.go:19-65`: `config.Ensure` → `config.Load` → `ensureUpdateServerBootstrap` → `builtin.RegisterAll` → `runtime.New` → `InitModules([wisemed-api, login, help, app-update-server])` → `app.Start`. Fără logging în fișier, fără headless/serviciu, fără update-check, fără buclă de restart, fără `process-control`, un singur `Init` — CDC.

---

## 2. Ordonarea modulelor, erori, context

- **Ordine**: strict ordinea slice-ului din `main.go`. Nu există declarații de dependențe, sortare topologică sau priorități (`core/runtime/runtime.go:110-122`) — CDC.
- **Dependențe implicite la `Init`** (ordinea listei le satisface) — CDC:
  - `barcode-printer` și `signing-pad` cer `ws-action-dispatcher` (`barcodeprinter/module.go:84`, `signingpad/module.go:81`) → `wisemed-ws` trebuie să fie înainte; lipsa e tolerată tăcut;
  - `signing-pad` (cu `shared_http`), `siui`, `siui-data` cer `local-http-control` și **eșuează** dacă lipsește (`signingpad/module.go:93-99`, `siui/module.go:123-127`, `siuidata/module.go:97-101`);
  - `siui-history` îl ia opțional (`siuihistory/module.go:76`);
  - restul consumurilor de servicii sunt leneșe (la request sau în `Start`).
- **Eroare la `Init`**: `InitModules` se oprește la primul eșec; modulele deja inițializate nu sunt curățate; `Run` întoarce eroarea → `[ERROR]` pe stdout + log + raportare către părintele headless (`runner.go:44-51`) → `log.Fatal`. Modul neînregistrat → `module %q is not registered` (`core/module/registry.go:22`) — CDC. Un pattern HTTP duplicat ar produce `panic` în `http.ServeMux.Handle`, fără `recover` — INF (nu există conflict în compozițiile actuale).
- **Semantica `Start`** — CDC (`runtime.go:129-148`):
  - toate `Starter`-ele pornesc **în paralel**, fără ordine;
  - primul `Start` care se întoarce — **inclusiv cu `nil`** — declanșează `cancel()` pentru toate;
  - `running.Wait()` așteaptă **fără timeout** terminarea tuturor; se întoarce doar prima valoare citită din `errCh`;
  - consecință: orice `Starter` trebuie să blocheze până la `ctx.Done()`; un modul blocat blochează oprirea. Comportament acoperit de `core/runtime/lifecycle_test.go:28` `TestStartWaitsForShutdown` (pachet rulat cu succes în suita `go test ./...` — CTL la nivel de pachet, `_reports/06` B.1 rândul 3).
  - Mecanismul e folosit intenționat de `runner-startup-updates`: întoarce `nil` după lansarea updaterului, ceea ce oprește tot runtime-ul (`apps/runner/background_updates.go:41-44`).
- **Erorile de comunicație ale protocoalelor** nu ies niciodată din `Start` (retry intern), deci nu opresc runtime-ul — CDC.
- **Propagarea contextului**: `signal.NotifyContext` → `ctx` (`runner.go:149`) → `runCtx` per iterație (`runner.go:277`) → `context.WithCancel` în `App.Start` (`runtime.go:125`) → fiecare `Starter`. Excepție: `result-sync` rulează sincronizarea cu `context.Background()` (`modules/resultsync/module.go:103,109`), deci o sincronizare în curs nu e anulată la oprire — CDC.
- **Ieșirea din `runAppLoop`**: semnal → `nil`; cerere de restart → iterație nouă; `Start` întors spontan → `return err` (poate fi `nil` → cod de ieșire 0) (`runner.go:283-300`) — CDC. Sub systemd cu `Restart=on-failure` o ieșire cu cod 0 nu repornește serviciul — INF.

---

## 3. Contracte (`core/module/types.go`)

| Contract | Definiție | Observații |
|---|---|---|
| `Module` (`:30`) | `ID() string`, `Init(rt Runtime) error` | obligatoriu |
| `Starter` (`:35`) | `Start(ctx context.Context) error` | opțional, detectat prin type assertion (`runtime.go:130`) |
| `Runtime` (`:16`) | `ConfigPath`, `ConfigDir`, `ReaderID`, `Logf`, `ModuleSettings(id)`, `ResolvePath`, `AddMenu`, `Handle`, `Mux`, `RegisterService`, `Service` | implementat de `runtime.App` (`runtime.go:16`) |
| `MenuEntry` (`:8`) | `ID, Group, Label, Path, Order` | sortare stabilă pe `(Group, Order)` la fiecare `AddMenu` (`runtime.go:81-91`); expus la `GET /api/menu` |
| `Registry` (`core/module/registry.go:7`) | `map[string]Factory` | fără mutex; `Register` suprascrie tăcut un ID existent |

CDC. De reținut:

- **Nu există `Stop`/`Close`** în contract. Curățarea se face la ieșirea din `Start` după `ctx.Done()`; modulele fără `Starter` nu au niciun hook de oprire.
- **Serviciile sunt `interface{}` netipizate**, rezolvate prin nume + type assertion la consumator; nu există verificare la compilare a existenței furnizorului.
- **Un singur `http.ServeMux` per `App`**: modulele înregistrează rute cu `rt.Handle` în `Init`; mux-ul nu are middleware global de autentificare (fiecare handler își aplică propriul `requireSession`/CORS — vezi [security-and-identity.md](security-and-identity.md)). Același mux este invocat in-process de `api.request` prin WSS (`shared/apibridge`).
- **Rute expuse de runtime însuși**: `/healthz`, `/api/menu`, `/api/modules` (`runtime.go:50-52`).

---

## 4. Registry de module

Toate ID-urile sunt înregistrate într-un singur loc: `modules/builtin/register.go:54` `RegisterAll` — **47 de ID-uri** (numărul de apeluri `Register(` a fost reverificat) —, plus `runner-startup-updates` în `apps/runner/background_updates.go:17`. CDC.

Legendă coloana „Tip”: **L** = logică reală; **N** = nominal (meniu + rută `/meta`, vezi §7); **S** = stub.

### 4.1 Infrastructură și integrare

| ID | Pachet (`modules/…`) | Tip | `Init` | `Start` | Rute (prefixe) |
|---|---|---|---|---|---|
| `local-http` | `localhttp` | L | sesiuni, CORS, meniu; servicii `local-http-control`, `daily-analysis-send-filters` | listener HTTP/HTTPS pe mux-ul comun, rebind la schimbarea adresei, `bindguard` la port ocupat | UI (`/`, `/settings*`, `/orders`, `/qc`, `/history`, `/operations`, `/debug`), `/api/session*`, `/api/reader-settings`, `/api/wisemed/{setup,bootstrap}`, `/api/app-update/*`, `/api/wss/`, `/api/status`, `/api/dashboard`, `/api/logs`, `/api/analytes*`, `/api/orders*`, `/api/qc*`, `/api/result-sync/*`, `/api/debug/tests*` ș.a. |
| `storage-sqlite` | `storage/sqlite` | L | deschide SQLite; servicii `storage`, `storage-meta` | așteaptă ctx, închide DB | `/api/storage/meta` |
| `events` | `events` | N | servicii `analyzer-activity`, `events` | — | `/api/events/meta` |
| `wisemed-ws` | `ws` | L | stare, nonce, dispatcher; 3 servicii | buclă conectare/reconectare (30 s) | — |
| `wisemed-api` | `wisemedapi` | L | client HTTP, serviciu `wisemed-api` | `EnsureEquipmentInitialized` dacă setup complet, apoi așteaptă | `/api/wisemed/meta` |
| `login` | `login` | S | placeholder | — | `/api/login/meta` |
| `help` | `help` | L | servește `help_dir` | — | `/help`, `/help/` |
| `result-sync` | `resultsync` | L | serviciu `result-sync` | ticker `interval_minutes` (implicit 5), `runOnce` | — |
| `runner-startup-updates` | `apps/runner` | L | reîncarcă config | verificare de update în fundal | — |

### 4.2 Laborator (module nominale și setări)

| ID | Pachet | Tip | `Init` | `Start` | Rute |
|---|---|---|---|---|---|
| `dashboard` | `dashboard` | N | meniu + serviciu descriptiv | — | `/api/dashboard/meta` |
| `analytes` | `analytes` | N | meniu + serviciu descriptiv | — | `/api/analytes/meta` |
| `analyte-management` | `analytemanagement` | N | meniu + serviciu descriptiv | — | `/api/analytes/management/meta` |
| `qc` | `qc` | N | meniu, niveluri QC | — | `/api/qc/meta` |
| `stats` | `stats` | N | meniu + serviciu descriptiv | — | `/api/stats/meta` |
| `daily-orders` | `dailyorders` | N | meniu | — | `/api/orders/meta` |
| `daily-details` | `dailydetails` | L | definiții din config, serviciu `daily-details` | — | `/api/daily-details/meta` |
| `astm-specimen-settings` | `astmspecimen` | L | — | doar așteaptă ctx | `/api/settings/astm-specimens` |

### 4.3 Transporturi și protocoale

| ID | Pachet | Tip | `Init` | `Start` | Rute |
|---|---|---|---|---|---|
| `transport-file` | `transports/file` | L (minim) | — | creează directoarele, înregistrează serviciul `transport-file` | — |
| `transport-serial` | `transports/serial` | S | — | doar log + așteaptă | — |
| `transport-tcpip` | `transports/tcpip` | S | — | doar log + așteaptă | — |
| `protocol-generic-file` | `protocols/genericfile` | S | placeholder (22 de linii), fără importator | — | `/api/protocol/meta` |
| `protocol-ir-biotyper` | `protocols/irbiotyper` | S | doar intrare de meniu | — | — |
| `protocol-cary60-uvvis` | `protocols/cary60uvvis` | L | meniu, `file-importer` | scan la `poll_seconds` (2) | `/api/protocol/meta` |
| `protocol-shimatzu-tocl` | `protocols/shimatzutocl` | L | meniu, `file-importer` | scan periodic | `/api/protocol/meta` |
| `protocol-cfx96-quantitation`, `protocol-analytikjena-plasmaquantms-elite`, `protocol-seegene-excel`, `protocol-biosan-hipo-mpp96`, `protocol-gammavision`, `protocol-shimatzu-generic`, `protocol-tricarb-5110-tr`, `protocol-anatolia-geneworks` (8 ID-uri) | `protocols/<nume>` peste `protocols/fileimportbase` | L | meniu, `file-importer` | scan la `poll_seconds` (2) | `/api/protocol/meta` |
| `protocol-beosl-csv` | `protocols/beoslcsv` (wrapper peste `fileimportbase`) | L | în plus `result-publisher` | delegă la bază | `/api/protocol/meta` |
| `protocol-anaf-docsmart` | `protocols/anafdocsmart` | L | — | scan la `poll_seconds` (5) | `/api/protocol/meta`, `/api/docsmart/parsed-files` |
| `protocol-astm` | `protocols/astm` | L | meniu, servicii status/replay | buclă TCP server/client, retry 3 s; inactiv dacă `comm_type != tcpip` | `/api/protocol/meta`, `/api/protocol/astm/status`, `/settings/protocol/astm` |
| `protocol-erba-mannheim-laura` | `protocols/erbamannheimlaura` | L | — | serial sau TCP după `comm_type`, retry 3 s | — |
| `protocol-horiba-abx-pentra400` | `protocols/horibaabxpentra400` | L | meniu | serial, retry 3 s | `/api/protocol/horiba-abx-pentra400/query-status`, `/protocol/horiba-abx-pentra400/query` |
| `protocol-biomerieux-minividas` | `protocols/biomerieuxminividas` | L | meniu | serial, retry 3 s | — |
| `protocol-labnovation-ld560` | `protocols/labnovationld560` | L | meniu, servicii status/replay, `restartCh` | TCP server/client, reconfigurare la cald | `/api/protocol/meta`, `/api/protocol/labnovation-ld560/*`, `/settings/protocol/labnovation-ld560` |
| `protocol-zonci-xl1000i` | `protocols/zoncixl1000i` | L | — | TCP server sau serial, retry 3 s | — |

### 4.4 Utilitare

| ID | Pachet | `Init` | `Start` | Rute |
|---|---|---|---|---|
| `html-print-formatter` | `htmlprintformatter` | setări, `http-server-options` (port din env `PORT`) | `NewRenderer` (Chromium headless), închide la ctx | `/render*`, `/pdf`, `/health`, `/api/html-print-formatter/*` |
| `wisemed-ai` | `wisemedai` | cheie de criptare, pool pgx + migrări dacă `database_url`, `http-server-options` | ticker 10 s (mirror modele, billing, outbox, retenție) | `/ai`, `/ai/`, `/ai/voice`, `/v1/`, `/api/wisemed-ai/v1/`, `/health/ai`, `/ready/ai` |
| `signing-pad` | `signingpad` | driver nativ, istoric SQLite opțional, înregistrare în dispatcher WS | cu `shared_http`: așteaptă; altfel listener propriu | `/api/esignature/*`, `/ws`, `/ws/demo`, `/esignature`, `/health`, `/signing-pad/*`, opțional `/api/signing-pad/*` |
| `barcode-printer` | `barcodeprinter` | SQLite jurnal, înregistrare în dispatcher WS | așteaptă ctx, închide DB | `/barcode/*`, `/api/barcode/*` |
| `siui` | `siui` | validare URL-uri CNAS, profile de certificat, cere `local-http-control` | lock de fișier, SQLite joburi, worker, ticker callback | `/api/siui/`, `/api/siui-history-sync/`, `/history-sync-ui` |
| `siui-data` | `siuidata` | setări, cheie JWT, cere `local-http-control` | watcher local + metrici la 2 s; închide pool PostgreSQL | `/api/siui-data/`, `/upload`, `/siui-data-*` |
| `siui-history` | `siuihistory` | setări MySQL/stocare | așteaptă ctx, închide DB | `/api/history/`, `/history-ui` |
| `app-update-server` | `appupdateserver` | SQLite, sesiuni | **listener propriu** pe adresa din `local-http` | `/`, `/settings`, `/help-ui`, `/app-updates/*`, `/api/session*`, `/api/update-server/*`, `/api/public/*` |

Toate tabelele: CDC (`_reports/01 §3`; `Init`/`Start` din `module.go` al fiecărui pachet; `register.go:55-101`). Numărătoare: 9 − 1 (`runner-startup-updates`) + 8 + 3 + 20 + 8 = 47. Catalogul complet de rute: [api-catalog.md](api-catalog.md).

**Nepotrivire ID ↔ cheie de configurare** — CDC: ID-ul `protocol-labnovation-ld560` (`modules/protocols/labnovationld560/module.go:73`) nu corespunde cheii `protocol-labnovation-ld-560` din `apps/labnovation-ld-560/deployments/config.install.yaml:79`; `ModuleSettings` caută după ID, deci secțiunea din șablon nu e citită.

---

## 5. Registry de servicii (nume → furnizor → consumatori)

Toate rândurile: CDC (grep exhaustiv pe `RegisterService(` și `.Service("` în `apps core modules shared`, fără `_test.go`; `_reports/01 §2.2` conține liniile exacte ale fiecărui consumator).

| Serviciu | Furnizor (loc de înregistrare) | Consumatori |
|---|---|---|
| `reader-config` | `runtime.New` (`core/runtime/runtime.go:36`) — instantaneu din config | `localhttp`, `wisemedapi` |
| `analyzer-config` | `runtime.New` (`runtime.go:43`) | `dailydetails`, `localhttp`, `erbamannheimlaura`, `labnovationld560`, `wisemedapi`, `ws`, `astm` (prin nume construit dinamic, `astm/module.go:1568`) |
| `app-config` | `runtime.New` (`runtime.go:47`) | `localhttp` |
| `process-control` | `runner.runAppLoop` (`apps/runner/runner.go:263`) | `localhttp/module.go:3093` |
| `local-http-ready` | `runner.runAppLoop` (`runner.go:271`), doar copil headless | `localhttp/module.go:369` |
| `local-http-control` | `local-http` (`localhttp/module.go:215`) | `barcodeprinter`, `htmlprintformatter`, `signingpad`, `siui`, `siuidata`, `siuihistory` |
| `daily-analysis-send-filters` | `local-http` (`localhttp/module.go:216`) | `horibaabxpentra400`, `zoncixl1000i` |
| `http-server-options` | `html-print-formatter` (`htmlprintformatter/module.go:46`), `wisemed-ai` (`wisemedai/module.go:104`) | `localhttp/module.go:326` |
| `storage` | `storage-sqlite` (`storage/sqlite/module.go:70`) | `localhttp` (≥ 13 locuri), `resultsync`, `transports/file`, `wisemedapi`, `fileimportbase`, `astm`, `cary60uvvis`, `shimatzutocl`, `labnovationld560`, `horibaabxpentra400`, `zoncixl1000i`, `biomerieuxminividas`, `cfx96quantitation`, `erbamannheimlaura` |
| `storage-meta` | `storage-sqlite` (`:71`) | niciun consumator |
| `analyzer-activity` | `events` (`events/module.go:17`) | `localhttp`, `astm`, `erbamannheimlaura`, `fileimportbase`, `labnovationld560` |
| `events` | `events` (`:18`) | niciun consumator |
| `wisemed-api` | `wisemed-api` (`wisemedapi/module.go:153`) | `appupdateserver`, `localhttp`, `astm`, `beoslcsv`, `cary60uvvis`, `fileimportbase`, `horibaabxpentra400`, `labnovationld560`, `shimatzutocl`, `zoncixl1000i`, `resultsync`, `siui/callback.go`, `wisemedai/auth.go`, `ws` |
| `ws-action-dispatcher` | `wisemed-ws` (`ws/module.go:140`) | `barcodeprinter`, `signingpad` |
| `wisemed-ws-status` | `wisemed-ws` (`:141`) | `localhttp`, `wisemedai/runtime_settings.go` |
| `wisemed-ws-client` | `wisemed-ws` (`:142`) | `localhttp/wss.go`, `siuidata/bridge_sync.go`, `wisemedai/events.go`, `wisemedai/runtime_settings.go`, `wisemedapi` |
| `result-sync` | `result-sync` (`resultsync/module.go:86`) | `localhttp`, `fileimportbase` |
| `result-publisher` | `protocol-beosl-csv` (`beoslcsv/module.go:53`) | `localhttp` |
| `file-importer` | `fileimportbase` (`:180`, toate protocoalele derivate), `protocol-cary60-uvvis`, `protocol-shimatzu-tocl` | `localhttp/module.go:3559` |
| `transport-file` | `transport-file` — **în `Start`, nu în `Init`** (`transports/file/module.go:36`) | `fileimportbase`, `cary60uvvis`, `shimatzutocl` |
| `daily-details` | `daily-details` (`dailydetails/module.go:28`) | `localhttp` |
| `signing-pad` | `signing-pad` (`signingpad/module.go:109`) | `localhttp/module.go:591` |
| `debug-replay-runner` | `protocol-astm` (`astm/module.go:170`), `protocol-labnovation-ld560` (`:84`) | `localhttp/debug_replay.go:117` |
| `protocol-astm-status`, `labnovation-ld560-status` | protocoalele omonime | niciun consumator |
| `dashboard`, `analytes`, `analyte-management`, `qc`, `stats`, `help` | modulele omonime (map-uri descriptive `features`) | niciun consumator (apar doar în `/api/modules`) |

Observații:

- **`local-http` este consumatorul central**: rezolvă ~15 servicii și conține logica pe care o expun, deci modulele „furnizor” sunt adesea doar puncte de acces la date (§7).
- **`transport-file` se înregistrează în `Start`**, iar `Start`-urile rulează în paralel → un importator poate vedea serviciul lipsă la primul scan; se autocorectează la tick-ul următor, deoarece `fileTransport()` e recitit la fiecare scan (`fileimportbase/module.go:226,397`) — INF.
- **Serviciile de config sunt instantanee** create la `runtime.New`: modificările din `config.yaml` devin vizibile doar după restartul complet din `runAppLoop` — CDC.
- **Un serviciu înregistrat de doi furnizori** (`http-server-options`, `debug-replay-runner`, `file-importer`) nu produce conflict în compozițiile actuale, pentru că furnizorii nu coexistă în aceeași aplicație; `RegisterService` ar suprascrie tăcut — INF din absența verificării.

---

## 6. Configurare

### 6.1 Structură

`core/config/config.go:11` `Config`: `reader{id,label,analyzer_name,analyzer_code,db_name}`, `logging{verbose_level}`, `results{auto_confirm_wisemed}`, `local_http{enabled,address,language,tls,cors_allowed_origins}`, `wisemed_ws{enabled,url,heartbeat_ms,reconnect_delay_ms}`, `analyzer{comm_type,protocol}`, `modules: map[id]map[string]interface{}`. Setările de modul sunt **netipizate** și citite cu `rt.ModuleSettings(id)` — CDC. Configul nu conține lista de module (hardcodată în `main.go`) și nici versiunea (injectată la build, vezi [build-deploy-update.md §2.3](build-deploy-update.md#23-embed-versiune)).

### 6.2 `config.install.yaml` → `config.yaml` (`config.Ensure`, `core/config/install.go:19`)

Șablonul e căutat lângă config (`InstallTemplatePath`, `install.go:71`) — CDC:

| Situație | Rezultat |
|---|---|
| lipsă `config.yaml`, există șablon | copie a șablonului |
| lipsesc ambele | `Default()` salvat |
| ambele prezente | `migrateSIUIDataMap` → `preserveLegacyConnectionSettings` → `mergeMissing`; scriere doar dacă s-a schimbat ceva |

- **`mergeMissing`** (`install.go:152-170`): adaugă doar cheile absente sau `nil`, recursiv pe map-uri; **listele nu sunt îmbinate**; valorile existente ale operatorului nu sunt suprascrise — CDC. Acesta este și mecanismul prin care un update aduce chei noi de configurare.
- **Efect secundar al scrierii**: `writeMap` și `Config.Save` re-serializează prin `yaml.Marshal` → comentariile și ordinea cheilor din `config.yaml` se pierd — INF.
- **Permisiuni**: `0644`, dar `0600` când `analyzer.protocol == siui-data` (`install.go:142-149`, `config.go:135-142`) — CDC.
- **Actualizări punctuale din UI**: `config.Update(path, map["a.b.c"]valoare)` (`install.go:75`), folosit de ex. de `modules/astmspecimen/module.go` — CDC. Alte module rescriu configul direct (ex. `PUT /api/barcode/settings`, `ws.SaveSettings`, `bindguard` la conflict de port).

### 6.3 Oglinzi și valori forțate (`ApplyDefaults`, `config.go:79`)

- **Oglinzi top-level ↔ `modules.*`**: `local_http` ↔ `modules.local-http`, `logging` ↔ `modules.logging`, `results` ↔ `modules.results`, `wisemed_ws` ↔ `modules.wisemed-ws`; valorile din modul au prioritate (`syncWiseMedWSMirror`, `config.go:260`). `modules.logging` și `modules.results` nu sunt module reale, doar secțiuni de setări — CDC.
- **Valori forțate** — CDC:

| Cheie | Comportament | Referință |
|---|---|---|
| `local_http.enabled` | devine mereu `true` → HTTP-ul local nu poate fi dezactivat din config | `config.go:106-108` |
| `wisemed_ws.reconnect_delay_ms` | forțat la 30000; în plus modulul nu îl citește (pauza e hard-codată) | `config.go:293-294`, `modules/ws/module.go:319` |
| `wisemed_ws.heartbeat_ms` | scris de wizard, necitit de modul (ping hard-codat la 15 s) | `apps/runner/bootstrap.go:369`, `modules/ws/module.go:463` |
| `logging.verbose_level` | limitat la 1..5 | `config.go` |
| `local_http.cors_allowed_origins` | implicit `https://ldse.wisemed.eu` | `config.go` |
| `local_http.address` | implicit `127.0.0.1:18080` | `config.go` |
| `modules.signing-pad.shared_http` | forțat `true` pentru protocolul `signing-pad` | `apps/runner/runner.go:189-207` |
| portul listenerului la `html-print-formatter` | suprascris de env `PORT` | `modules/htmlprintformatter/module.go:38-50` |
| `modules.wisemed-ai.database_url` | suprascris de env `WISEMED_AI_DATABASE_URL` | `modules/wisemedai/config.go:39` |

### 6.4 Migrări — CDC

| Migrare | Loc |
|---|---|
| `siu-data` → `siui-data` (protocol, listă de module, secțiune de modul), pe struct și pe map-ul brut înainte de merge | `core/config/siui_data_migration.go:8`, `:32` |
| chei `wisemed_ws` vechi → `modules.wisemed-ws`; `address/tls` din `modules.signing-pad` → `local_http` cu `shared_http: true` | `install.go:193` `preserveLegacyConnectionSettings`, dublat la încărcare în `runner.go:183` |
| `app_id` `shimatzu-generic-v3` → `shimatzu-generic-reader` | `runner.go:209-224` |

Teste existente: `core/config/siui_data_migration_test.go`, `core/config/wss_test.go`, `apps/runner/wss_config_test.go` — pachetele au trecut în suita `go test ./...` (CTL la nivel de pachet, `_reports/06` B.1 rândul 3).

### 6.5 Path-uri, `reader_id`, bootstrap

- **Path-uri relative**: `App.ResolvePath` le rezolvă față de **directorul fișierului de config**; șir gol → directorul configului (`runtime.go:67-75`); echivalent în runner: `resolveConfigPath` (`updates.go:183`). Logurile și fișierul PID stau tot acolo — CDC. Consecință: baza SQLite, `inbox/outbox`, logurile și configul sunt toate sub `deployments/`.
- **`reader.id`**: expus prin `Runtime.ReaderID()` (`runtime.go:58`); folosit ca prefix de log/PID (`logging.go:57`), nume de serviciu OS (`service_common.go:32`), fallback pentru `app_id` la update (`updates.go:39`), `sub` implicit în JWT-ul WSM — CDC.
- **Wizardul de bootstrap** (`apps/runner/bootstrap.go`): setări `wisemed-api`, identitate reader, selecția unității medicale / tipului de echipament prin `wisemedapi.BootstrapClient`, `local_http`, `wisemed_ws`, comm/protocol, `app-updates`, `storage-sqlite`, `result-sync`, transport; `registerEquipment` în WiseMED (`bootstrap.go:441`); protocoalele/transporturile oferite sunt derivate din `EnabledModules` (`:721`, `:776`). `siui-data` are un wizard separat (`siui_data_setup.go`) — CDC.
- **Server de update implicit**: toate șabloanele au `modules.app-updates.base_url: http://127.0.0.1:19090` (și wizardul, `bootstrap.go:189`) — CDC; URL-ul de producție: NEC.

---

## 7. Transporturi stub și module nominale

Modularitatea declarată în `readersv3/README.md` este parțial nominală — CDC (`_reports/01 §8 #7–8`, `_reports/03 §0` S1–S3).

**Transporturi**:

| Modul | Ce face efectiv | Unde e I/O-ul real |
|---|---|---|
| `transport-serial` | un `Logf` + `<-ctx.Done()` (`modules/transports/serial/module.go:14-18`) | în fiecare modul de protocol, cu `go.bug.st/serial`; setările sunt citite direct cu `rt.ModuleSettings("transport-serial")` |
| `transport-tcpip` | un `Logf` + `<-ctx.Done()` (`modules/transports/tcpip/module.go:14-18`) | `net.Listen`/`DialTimeout` în fiecare modul de protocol; setări citite direct din `transport-tcpip` |
| `transport-file` | `MkdirAll` + fallback pe director implicit + publică un `map` de căi (`modules/transports/file/module.go:26-57`); **nu scanează nimic** | pollingul e în `fileimportbase` (și copiile proprii din `cary60uvvis`, `shimatzutocl`) |

Selectorul serial/TCP este `analyzer.comm_type`, expus ca serviciu `analyzer-config`. Consecință: logica de reconectare, timeouturile și tratamentul portului ocupat sunt duplicate și diferă între protocoale (`_reports/03 §1.3–§1.4`). Niciun pachet de transport nu are teste Go (`_reports/06` B.3).

**Module nominale** (meniu + rută `/meta`, fără logică): `login` („login module placeholder”, `modules/login/module.go:13-22`), `events` (26 de linii, fără publicare/abonare), `dashboard`, `stats`, `qc`, `analytes`, `analyte-management`, `daily-orders` (`events/module.go:15-26`, `dashboard/module.go:14-25`, `stats/module.go:14-27`, `qc/module.go:16-40`).

**Unde este logica reală**:

- `modules/localhttp/module.go` (3954 de linii): sesiuni și autentificare locală, dashboard, comenzi, analize, QC/Westgard, loguri, corecția ID-ului, sincronizarea manuală, setări, starea update-ului, WSS remote;
- `modules/storage/sqlite`: schema, migrările, tranzacțiile, identitatea probei, normalizarea TAG-urilor, motorul de transformări.

Detalii: `_reports/03-laboratory.md §3, §7, §9`, [data-and-state-machines.md](data-and-state-machines.md).

**Nu există event bus**: niciun canal de publicare și niciun SSE/WS către UI-ul local; UI-ul face polling HTTP (`setInterval` pe `/api/logs`, `/api/status`, `/api/dashboard`, `modules/localhttp/ui/app.js:3573,3587`) — CDC. Singurul SSE din repo este `GET /v1/sessions/{id}/events` din `wisemed-ai`.

**Protocoale stub**: `protocol-generic-file` (nu înregistrează `file-importer`; `localhttp` răspunde 400 „file import backend is not available”, `localhttp/module.go:1694-1697`) și `protocol-ir-biotyper` (doar meniu) — CDC. Dacă există altă cale de import pentru `generic-test-reader`: NEC.

---

## 8. Clientul `wisemed-ws`

Modulul `modules/ws` (ID `wisemed-ws`) este inclus în 31 de aplicații (toate cele pe `runner`) — CDC. Rezumat relevant pentru runtime; protocolul complet este în `_reports/02-wsm-security.md §6` și [communication-map.md](communication-map.md).

- **Condiții de conectare** (`Start`/`runSession`, `modules/ws/module.go:255-461`): `enabled != "false"` și `url` nevid (doar `wss://`); serviciul `wisemed-api` trebuie să reușească `EnsureEquipmentInitialized()`; `echipament_id` nevid și ≠ `"0"`. **Nu există mod WSS fără WiseMED** — CDC. Aplicațiile login-only (`html-print-formatter`, `wisemed-ai`) sar peste înregistrare, dar tot au nevoie de `echipament_id`; proveniența lui: NEC.
- **Autentificare**: `auth_mode` = `device_key` (implicit; readerul își semnează singur un JWT HS256 de 5 min), `token_file` sau `token_endpoint` (`module.go:717-761`) — CDC. Vezi [security-and-identity.md](security-and-identity.md).
- **Reconectare**: pauză **fixă de 30 s** după orice sfârșit de sesiune, fără backoff sau jitter (`module.go:319`); ping la 15 s, read deadline 60 s — CDC. Reconectarea reală după ~30 s este acoperită de testul de integrare `wsm-server/integration` — CTL (`_reports/06` B.1 rândul 10).
- **Ciclul de 5 minute în `device_key`** — INF din două fapte CDC: tokenul expiră la 5 min, WSM închide socketul la `exp` (`wsm-server/internal/server/server.go:265`), iar readerul nu reînnoiește tokenul pe conexiunea existentă → deconectare periodică + ~30 s offline.
- **Execuția comenzilor**: o goroutine worker per sesiune, **serială**, coadă de 16; coadă plină → sesiunea e închisă. `api.request` invocă `apibridge.Invoke` pe `rt.Mux()` — același mux ca HTTP-ul local, in-process (`module.go:512-533`). Coada de trimitere are 128 de poziții; plină → reply pierdut (`module.go:627-645`) — CDC.
- **Servicii oferite runtime-ului**: `ws-action-dispatcher` (modulele `barcode-printer` și `signing-pad` își înregistrează acțiunile), `wisemed-ws-status`, `wisemed-ws-client` — CDC.
- La restartul complet din `runAppLoop` sesiunea WSS, nonce-urile și cererile în așteptare se pierd (`failPending`, `module.go:617-626`) — CDC.

---

## 9. Concurență la nivel de runtime

| Primitivă | Loc | Rol |
|---|---|---|
| `sync.RWMutex` `App.mu` | `core/runtime/runtime.go:22` | protejează `menu` și `services`; `modules` e scris doar în `InitModules` (fără lock) și citit fără lock în `handleModules` (`:168`) |
| `http.ServeMux` | `runtime.go:19` | sincronizare internă; rutele sunt înregistrate în `Init` |
| goroutine per `Starter` + `sync.WaitGroup` + `errCh` (buffer `len(modules)`) | `runtime.go:127-139` | lifecycle paralel; fără timeout la `Wait` |
| `restartCh chan string` (buffer 1) | `apps/runner/runner.go:248` | cerere de restart complet; `RequestRestart` e non-blocant și înlocuiește o cerere în așteptare (`runner.go:231-245`) |
| goroutine `app.Start` + `errCh` (buffer 1) | `runner.go:278-281` | o instanță runtime per iterație |
| `sync.Once` | `runner.go:38` | raportare unică a rezultatului pornirii către părintele headless |
| goroutine `cmd.Wait` + `ticker` 50 ms + `timer` 1 min | `apps/runner/headless_startup.go:27-33` | așteptarea copilului headless |
| goroutine `app.Start` + `errCh` | `apps/runner/service_windows.go:90-93` | serviciu Windows; `Stop/Shutdown` → `cancel()` → `<-errCh` |

Toate: CDC (`_reports/01 §6`). `go test -race ./...` nu raportează curse pe căile exercitate de teste — CTL (`_reports/06` B.1 rândul 4); 27 din 51 de pachete `modules` nu au teste, deci căile lor nu sunt acoperite. Concurența din interiorul modulelor (worker SIUI, semaforul Chromium, `deviceMu` la signing-pad, hubul WSM) este descrisă în documentele de domeniu și în `_reports/02 §12`, `_reports/03 §10`, `_reports/04 §11`, `_reports/05`.

---

## 10. Restart și recuperare

| Nivel | Mecanism | Ce se păstrează / pierde | Etichetă |
|---|---|---|---|
| **Restart complet în proces** (`runAppLoop`) | declanșat din UI la schimbarea setărilor readerului sau ale update-ului (`modules/localhttp/module.go:972,2792`) prin `process-control`. Reîncarcă `config.yaml` (`config.Load`, **fără** `Ensure`, bootstrap, update-check sau redeschiderea logului), construiește registry + `App` + mux noi, reinițializează toate modulele | starea în memorie (sesiuni HTTP, nonce WS, cozi) se pierde; persistă ce e în SQLite/fișiere. Indisponibil ca serviciu Windows (§1.2) | CDC |
| **Reconectări interne** (fără restart de runtime) | WSS la 30 s (`ws/module.go:319`); protocoale TCP/serial la 3 s (ex. `astm/module.go:211`); rebind HTTP la schimbarea adresei (`localhttp/module.go:386-404`); port ocupat → `bindguard.HandleAddressInUse` propune altă adresă și o scrie în config (`localhttp/module.go:413-423`); LD-560 se reconfigurează la cald prin `restartCh` propriu | — | CDC |
| **Restart de proces** | nu există supervizor intern. Depinde de OS: systemd `Restart=on-failure`, `RestartSec=5` (`service_linux.go:29-30`); launchd `KeepAlive` (`service_darwin.go:36-37`); SCM restart la 60/120/300 s, reset 86400 s (`service_windows.go:38`). În `-headless` fără serviciu nu există repornire automată; fișierul PID rămâne dacă procesul e ucis brutal | — | CDC pentru configurație; comportamentul efectiv DN |
| **Stare de business** | fișiere pe jumătate importate, cozi de trimitere, joburi SIUI | ține de module: vezi `_reports/03 §10.2`, `_reports/04 §6.8`, [data-and-state-machines.md](data-and-state-machines.md) | — |

---

## 11. Lacune

- Efectul real al dublului `Init` și comportamentul ca serviciu Windows: neconfirmate la runtime (NEC).
- `shared/*` a fost citit doar parțial de rapoarte (`bindguard`, `localtls`, `commtrace`, `debugreplay`); `bindguard` gestionează doar conflictul de port, nu este o protecție a bind-ului (`_reports/02 §15 #10`).
- Importul de fișiere în `generic-test-reader` pe altă cale decât `file-importer`: NEC.
- Comportamentul `go.bug.st/serial` la deconectarea fizică a cablului (determină dacă bucla de reconectare pornește): NEC (`_reports/03` necunoscuta 12).
