# Catalogul aplicațiilor

| | |
|---|---|
| **Scop** | Inventarul tuturor executabilelor din repository: cele 32 de aplicații din `readersv3/apps/` și `wsm-server` (`cmd/wsm-server`, `cmd/wsmctl`), cu entrypoint, module, transport, persistență, porturi, dependențe externe; plus modulele Go și zonele auxiliare ale repository-ului. |
| **Data** | 2026-11-14 (commit de bază `d53088e`, branch `main`, cu modificări locale necomise) |
| **Surse** | `_reports/01-runtime-and-apps.md` §5, §7, §8; `_reports/03-laboratory.md` §1–§2, §5.5; `_reports/04-cnas-siui.md` §0, §4, §9; `_reports/05-utilities-ai.md` §0–§6; `_reports/02-wsm-security.md` §4–§6, §12; `_reports/06-qa-build-ops.md` A.0, A.4, B.1 |
| **Limite** | Niciun binar nu a fost pornit. Valorile de port/protocol provin din șabloanele `deployments/config.install.yaml`, nu din instalări reale (`readersv3/output/*/deployments/config.yaml` nu a fost citit). Nicio afirmație de aici nu confirmă comunicarea cu analizoare, imprimante, tablete de semnătură, CNAS sau WiseMED real. |

Etichete: **CDC** = `CONFIRMAT DIN COD` · **CTL** = `CONFIRMAT PRIN TEST LOCAL` (numai rulările din `_reports/06-qa-build-ops.md` B.1) · **DN** = `DOCUMENTAT, NEVERIFICAT` · **INF** = `INFERENȚĂ` · **NEC** = `NECUNOSCUT`. Căile sunt relative la rădăcina repository-ului.

Documente înrudite: [runtime-and-modules.md](runtime-and-modules.md) (lifecycle, registry de module și servicii), [build-deploy-update.md](build-deploy-update.md) (build, instalare, update), [communication-map.md](communication-map.md), [api-catalog.md](api-catalog.md), [security-and-identity.md](security-and-identity.md), [data-and-state-machines.md](data-and-state-machines.md), [gaps-and-contradictions.md](gaps-and-contradictions.md).

---

## 1. Privire de ansamblu

| Familie | Număr | Compunere | Livrare (detalii în [build-deploy-update.md](build-deploy-update.md)) |
|---|---|---|---|
| Readere de analizoare | 22 | `runner.Run` + setul **R** + transport + protocol | installer nativ + `update-server` |
| Utilitare desktop/servicii | 9 | `runner.Run` + setul **U** (cu o excepție) + modul propriu | installer nativ / Docker |
| `update-server` | 1 | compunere proprie, fără `runner` | Docker / binar |
| WSM Server | 2 binare | modul Go separat (`wsm-server/`) | systemd sau Docker |

- **Numărul de aplicații este 32** (fără pachetul `runner`). `go build ./...` în `readersv3/` compilează 101 pachete, din care 35 în `apps/` = 32 aplicații + `runner` + 2 utilitare `apps/wisemed-ai/tools/{catalog,import-postgres}` — CTL (`_reports/06` B.1 rândul 1, B.3). Alte numere din repository sunt depășite: memoria `aimemory/wisemed-labreaders.md:7-8` spune 26, `readersv3/docs/wss-control.md` 28, `readersv3/build/apps.json` 28 (lipsesc `siui-bridge`, `siui-bridge-history`, `wisemed-ai`, `wisemed-siui-data`), `readersv3/README.md` „patru readere” — CDC (`_reports/01` §8 #1–2, `_reports/06` contradicția 8).
- **31 de aplicații** au structura `main` → `runner.Run(cfgPath, modules, RunOptions{...})`, cu lista de module **hardcodată** în `main.go` (`readersv3/apps/generic-test-reader/main.go:12-28`, identic structural) — CDC. `update-server` compune runtime-ul direct (§3.10).
- **Flag-uri CLI** (31 de aplicații): `-config` (implicit `deployments/config.yaml`), `-headless`, `-headless-child`, `-installservice`, `-reconfigure`, `-showlog`, `-version`. `update-server`: doar `-config`, `-reconfigure`, `-version` — CDC (`readersv3/apps/*/main.go:13-19`).
- **`RunOptions`** (`readersv3/apps/runner/runner.go:22-32`): `BackgroundStartupUpdates`, `SkipBootstrap`, `Headless`, `HeadlessChild`, `InstallService`, `Reconfigure`, `ShowLog`. Ultimele cinci oglindesc flag-urile; primele două sunt fixate în cod per aplicație — CDC:

| Opțiune fixată în `main.go` | Aplicații | Efect |
|---|---|---|
| `SkipBootstrap: true` | `esignature-server`, `html-print-formatter`, `wisemed-ai` | wizardul interactiv rulează doar cu `-reconfigure` (`runner.go:75`) |
| `BackgroundStartupUpdates: true` | `wisemed-ai`, `wisemed-siui-data` | verificarea de update rulează în modulul intern `runner-startup-updates`, nu înainte de pornire (`runner.go:71-73`) |
| (standard) | celelalte 27 | bootstrap interactiv la config incomplet, update-check sincron |

### Seturi de module (ordinea exactă din cod)

- **R** — bază reader, 14 module: `local-http, storage-sqlite, events, wisemed-api, wisemed-ws, login, help, dashboard, analytes, analyte-management, qc, result-sync, stats, daily-orders`.
- **U** — bază utilitar, 6 module: `local-http, events, wisemed-api, wisemed-ws, login, help`.

CDC (`readersv3/apps/<app>/main.go:25`). Ordinea contează: modulele sunt inițializate strict în ordinea listei (vezi [runtime-and-modules.md §2](runtime-and-modules.md#2-ordonarea-modulelor-erori-context)). `readersv3/README.md` listează doar 12 module pentru readere (fără `local-http` și `result-sync`) — README incomplet (`_reports/01` §8 #4).

---

## 2. Readere de analizoare (22)

Comun tuturor — CDC (`main.go:25` + `deployments/config.install.yaml` ale fiecărei aplicații):

- **Entrypoint**: `readersv3/apps/<app>/main.go`, `RunOptions` standard.
- **Persistență**: SQLite prin `storage-sqlite` (`modernc.org/sqlite`), fișier `./<db>.db` relativ la directorul configului (`deployments/`); loguri zilnice și PID tot acolo. Readerele pe fișier folosesc în plus directoarele `inbox/processed/failed/outbox` ale `transport-file`. Schema: [data-and-state-machines.md](data-and-state-machines.md), `_reports/03-laboratory.md §3`.
- **HTTP local**: `127.0.0.1:18080`, fără TLS — **același port în toate cele 22 de șabloane** (vezi §5).
- **Dependențe externe**: WiseMED API (HTTP, modul `wisemed-api`) pentru înregistrarea echipamentului, comenzi și trimiterea rezultatelor; WSM (WSS, modul `wisemed-ws`, `wss://wslocal.wisemed.eu/ws` implicit în șabloane) unde coloana WSS e „on”; `update-server` (`modules.app-updates.base_url`, implicit `http://127.0.0.1:19090`). URL-urile de producție: NEC.
- **Transport**: pentru `tcpip` și `serial`, modulul `transport-*` este stub; socketul/portul serial e deschis de modulul de protocol (vezi [runtime-and-modules.md §7](runtime-and-modules.md#7-transporturi-stub-și-module-nominale)).

| Aplicație | Module (peste R) | Transport / protocol implicit | Format (din `_reports/03` §2.1) | Port analizor implicit | WSS | Particularități |
|---|---|---|---|---|---|---|
| `analytikjena-plasmaquantms-elite-reader` | `transport-file`, `protocol-analytikjena-plasmaquantms-elite` | file, `*.csv` | CSV `;` | — | off | — |
| `anatolia-geneworks-reader` | `transport-file`, `protocol-anatolia-geneworks` | file, `*.xls` | XLS (BIFF) | — | off | — |
| `autobio-autolumo-s900-reader` | `astm-specimen-settings`, `transport-tcpip`, `protocol-astm` | tcpip server / `astm` | ASTM E1394, bidirecțional (host query) | `0.0.0.0:11119` | on | — |
| `beosl-reader` | `transport-file`, `protocol-beosl-csv` | file, `*.csv` | CSV dozimetrie | — | on | publică dozimetrie în WiseMED (serviciu `result-publisher`); singurul șablon cu `auto_confirm_wisemed: true` |
| `biomerieux-minividas-reader` | `transport-serial`, `protocol-biomerieux-minividas` | serial 9600 | proprietar bioMérieux, unidirecțional | port serial gol | on | implicitul din cod e 38400 (`modules/protocols/biomerieuxminividas/module.go:294`), șablonul pune 9600 |
| `biosan-hipo-mpp-96-reader` | `transport-file`, `protocol-biosan-hipo-mpp96` | file, `*.csv` | CSV `;` | — | off | — |
| `cary60-uvvis-reader` | R cu `daily-details` inserat înainte de `daily-orders` + `transport-file`, `protocol-cary60-uvvis` | file, `*.csv` | CSV secționat Cary WinUV | — | off | singura aplicație cu `daily-details`; importator propriu (nu `fileimportbase`) |
| `cfx96-reader` | `transport-file`, `protocol-cfx96-quantitation` | file, `*Quanti* Plate View Results.xlsx` | XLSX | — | on | — |
| `erba-mannheim-laura-reader` | `transport-serial`, `transport-tcpip`, `protocol-erba-mannheim-laura` | serial 9600 (implicit) sau tcpip | raport text STX…ETX, fără ACK | TCP `127.0.0.1:5150` | on | două transporturi; are `README.md`, `docs/` |
| `gammavision-reader` | `transport-file`, `protocol-gammavision` | file, `*.txt` | TXT raport GammaVision | — | off | — |
| `gemini-reader` | `astm-specimen-settings`, `transport-tcpip`, `protocol-astm`, `protocol-ir-biotyper` | tcpip server / `astm` | ASTM E1394 | `127.0.0.1:9000` | on | `protocol-ir-biotyper` este doar intrare de meniu (neimplementat) |
| `generic-test-reader` | `transport-file`, `protocol-generic-file` | file, `*.txt` | — | — | on | modulul de protocol e placeholder (22 de linii): **nu importă fișiere** (`modules/protocols/genericfile/module.go:13-22`); importul manual răspunde 400 (`modules/localhttp/module.go:1694-1697`) |
| `horiba-abx-pentra400-reader` | `astm-specimen-settings`, `transport-serial`, `protocol-horiba-abx-pentra400` | serial 9600 | ASTM E1394 peste ENQ/STX/ETX/EOT, bidirecțional | port serial gol | on | șablonul are secțiune `protocol-astm` deși modulul nu e activat |
| `horiba-yumizen-h500-reader` | `astm-specimen-settings`, `transport-tcpip`, `protocol-astm` | tcpip server / `astm` | ASTM E1394 | `127.0.0.1:5150` | on | — |
| `labnovation-ld-560` | `transport-tcpip`, `protocol-labnovation-ld560` | tcpip server / `simple` (și `hl7`) | „Simple” sau HL7 v2 (MLLP/raw); nu trimite ACK HL7 | `0.0.0.0:8000` | on | fără `deployments/help`; `reader.id` fără sufix `-v3`, `app_id` `labnovation-ld-560-reader-v3`; **cheie de config greșită** (vezi mai jos) |
| `lifotronic-h8-reader` | `astm-specimen-settings`, `transport-tcpip`, `protocol-astm` | tcpip server / `astm` | ASTM E1394 | `127.0.0.1:8000` | on | — |
| `seegene-reader` | `transport-file`, `protocol-seegene-excel` | file, `*.csv` | CSV Seegene Viewer (în ciuda numelui „excel”) | — | on | wizardul propune `*.xlsx` (`apps/runner/bootstrap.go:246-248`), șablonul `*.csv` |
| `shimatzu-generic-reader` | `transport-file`, `protocol-shimatzu-generic` | file, `*.txt` | TXT GC cu secțiuni | — | off | `deployments/config.yaml` este **versionat în git**; `app_id` normalizat în runner (`runner.go:209-224`) |
| `shimatzu-tocl-reader` | `transport-file`, `protocol-shimatzu-tocl` | file, `*.txt` | TXT tabular TOC-L | — | off | importator propriu (nu `fileimportbase`) |
| `snibe-maglumi-x3` | `astm-specimen-settings`, `transport-tcpip`, `protocol-astm` | tcpip server / `astm` | ASTM E1394 | `127.0.0.1:9000` | on | — |
| `tricarb-5110-tr-reader` | `transport-file`, `protocol-tricarb-5110-tr` | file, `*.csv` | CSV | — | off | — |
| `zonci-xl1000i-reader` | `transport-tcpip`, `transport-serial`, `protocol-zonci-xl1000i` | tcpip server (implicit) sau serial 115200 | proprietar STX…ETX, bidirecțional | `0.0.0.0:3125` | on | **fără director `build/`**; binar `readersv3/zonci-xl1000i-reader.exe` (23,1 MB) versionat în git |

Toate rândurile: CDC (`_reports/01` §5.1, `_reports/03` §2.1–§2.2). Detaliile de parsare (ID probă, TAG, QC) sunt în `_reports/03-laboratory.md §2.1`.

**Cheia de configurare `labnovation-ld-560`** — CDC, verificat direct: `readersv3/apps/labnovation-ld-560/deployments/config.install.yaml:79` definește secțiunea `protocol-labnovation-ld-560`, dar ID-ul modulului este `protocol-labnovation-ld560` (`readersv3/modules/protocols/labnovationld560/module.go:73`), iar setările sunt citite cu `ModuleSettings("protocol-labnovation-ld560")` (`module.go:650`). Setările din șablon aflate sub cheia greșită nu sunt citite de modul; modulul rulează cu valorile implicite din cod.

**Reutilizarea protocoalelor** — CDC: `protocol-astm` deservește 5 aplicații (`autobio`, `gemini`, `horiba-yumizen-h500`, `lifotronic-h8`, `snibe-maglumi-x3`), diferențiate doar prin configurare; ASTM generic nu suportă serial (intră în „idle” dacă `comm_type != tcpip`, `modules/protocols/astm/module.go:177-181`). Nouă protocoale pe fișier sunt construite peste biblioteca `modules/protocols/fileimportbase` (opt direct + wrapperul `beoslcsv`); `cary60uvvis` și `shimatzutocl` au importatoare proprii.

**Securitatea listenerelor de analizor** — CDC (absență): niciun listener TCP de analizor nu are autentificare, TLS, allow-list de IP sau limită de conexiuni (`_reports/03` §1.4); trei șabloane ascultă pe `0.0.0.0` (`autobio`, `labnovation`, `zonci`). Vezi [security-and-identity.md](security-and-identity.md).

---

## 3. Utilitare și servere (10)

### 3.0 Sumar

| Aplicație | Module (ordine) | Protocol (`analyzer.protocol`) | Bază de date / fișiere | Port implicit | `RunOptions` |
|---|---|---|---|---|---|
| `anaf-docsmart` | `local-http, wisemed-api, wisemed-ws, login, help, transport-file, protocol-anaf-docsmart` | `anaf-docsmart` | niciun DB; fișiere `inbox/work/outbox/processed/failed` | `127.0.0.1:18120` | standard |
| `barcodeprinter` | U + `barcode-printer` | `barcodeprinter` | SQLite `./barcodeprinter-log.db` | `127.0.0.1:8010` | standard |
| `esignature-server` | U + `signing-pad` | `signing-pad` | SQLite `./esignature-log.db` (opțional) | `127.0.0.1:19111` **TLS** | `SkipBootstrap` |
| `signing-pad-utility` | U + `signing-pad` | `signing-pad` | SQLite istoric (opțional) + PNG în `./signatures` | `0.0.0.0:19110` | standard |
| `html-print-formatter` | U + `html-print-formatter` | `html-print-formatter` | niciuna (memorie) | `127.0.0.1:19113` (suprascris de env `PORT`) | `SkipBootstrap` |
| `siui-bridge` | U + `siui` | `siui` | SQLite `./siui-jobs.db` + `.lock` + `siui-token-pins.json` + `siui-downloads/` | `127.0.0.1:19112` **TLS** | standard |
| `siui-bridge-history` | U + `siui-history` | `siui-history` | MySQL (index) + stocare `local`/S3 (obiecte) | `127.0.0.1:19113` **TLS** | standard |
| `wisemed-siui-data` | U + `siui-data` | `siui-data` | PostgreSQL (`pgxpool`) | `127.0.0.1:19114` **TLS** | `BackgroundStartupUpdates` |
| `wisemed-ai` | U + `wisemed-ai` | `wisemed-ai` | PostgreSQL (`wai_*`) + fișiere cheie/audio/model | `127.0.0.1:19120` | `SkipBootstrap`, `BackgroundStartupUpdates` |
| `update-server` | `wisemed-api, login, help, app-update-server` | `app-update-server` | SQLite `./app-update-server.db` + `./files` | `0.0.0.0:19090` | — (fără `runner`) |

CDC: `readersv3/apps/<app>/main.go` + `deployments/config.install.yaml`; drivere din `sql.Open`/`pgxpool` (`modules/barcodeprinter/module.go:528`, `signingpad/history.go:21`, `siui/jobs.go:42`, `siuihistory/module.go:204`, `siuidata/postgres.go:75`, `wisemedai/module.go:60`, `appupdateserver/module.go:871`).

Toate cele 9 aplicații pe `runner` includ `wisemed-api` și `wisemed-ws`, deci pot depinde de WiseMED API și WSM; care dintre ele le folosesc efectiv depinde de configurare (ex. `wisemed-ai` are în șablon `wisemed_ws.enabled: false`). Rolul anunțat la WSM: `service` pentru protocoalele `siui`/`siui-history`, `reader` pentru rest, inclusiv `wisemed-siui-data` (`modules/ws/module.go:694-712`) — CDC.

### 3.1 `anaf-docsmart`

- **Entrypoint**: `readersv3/apps/anaf-docsmart/main.go:27-36`. Singura aplicație pe `runner` **fără `events` și fără `storage-sqlite`** — CDC.
- **Funcție**: polling `inbox/*.xml` la `poll_seconds` (5) → completează local un PDF XFA (formular `f1102`) → `outbox/<nume>.filled.pdf`; XML mutat în `processed/` sau `failed/` (`modules/protocols/anafdocsmart/module.go:58-189`). **Nu există comunicare de rețea cu ANAF** — CDC.
- **Dependență externă**: **Adobe Acrobat pe macOS**, condus prin `osascript` + un script JavaScript scris în `~/Library/Application Support/Adobe/Acrobat/DC/JavaScripts/` (`acrobat_darwin.go:15-43`). Pe Linux și Windows funcția întoarce eroare, deci orice XML ajunge în `failed/` (`xfa_linux.go`, `acrobat_other.go`) — CDC.
- **Particularități**: `apps/anaf-docsmart/internal/docsmart/` și directoarele de lucru din sursă sunt goale; rutele proprii (`/api/protocol/meta`, `/api/docsmart/parsed-files`) nu cer sesiune la nivel de modul — CDC. Detalii: `_reports/05-utilities-ai.md §5`.

### 3.2 `barcodeprinter`

- **Funcție**: generare ZPL și trimitere RAW la imprimantă; `GET/POST /barcode/print`, `/api/barcode/*`, acțiuni WSM `barcode.print` / `barcode.printers` — CDC.
- **Dependențe externe**: spoolerul sistemului — `lp` → `lpr` pe Unix (`printer_unix.go`), spooler nativ pe Windows (`printer_windows.go`); WiseMED web ca apelant — CDC.
- **Particularități**: rutele HTTP sunt înregistrate doar cu `withCORS`, **fără autentificare**, inclusiv `PUT /api/barcode/settings` care rescrie `config.yaml` (`modules/barcodeprinter/module.go:71-82`); ZPL-ul complet (cu numele pacientului) este scris în log (`module.go:509-517`); șablonul conține secțiuni `storage-sqlite` și `transport-file` neutilizate; `app_id` de update `barcodeprinter-v3` — CDC. Detalii: `_reports/05 §3`, [security-and-identity.md](security-and-identity.md).

### 3.3 `esignature-server` și 3.4 `signing-pad-utility`

Ambele încarcă **același modul** `signing-pad`; diferă doar configurarea — CDC (`_reports/05 §4`).

| | `esignature-server` | `signing-pad-utility` |
|---|---|---|
| Cale hardware | nativ: `STPadLib.dll` Signotec prin syscall, **doar Windows** (`native_windows.go`, `legacy_windows.go`); pe alte OS eroare explicită (`native_other.go`) | helper extern `java -jar ./java-helper/signing-pad-helper.jar` (stdin/stdout JSON) |
| Stare | implementat | **stub**: `apps/signing-pad-utility/java/src/.../HelperMain.java:48` răspunde „SDK bridge not implemented yet” |
| Țintă de build | numai `windows-386` (DLL pe 32 de biți); DLL copiat din `docs/signotec/WebSocketPadServer/STPadLib.dll` (`tools/releasectl/main.go:243-252`, `:523-527`) | toate țintele standard |
| Port | `127.0.0.1:19111` TLS | `0.0.0.0:19110`, `device_port 10000` |
| Bootstrap | `SkipBootstrap: true` | activ |
| Update | `app_id: esignature-server` | șablon **fără secțiune `app-updates`** |
| Conținut suplimentar | `build-windows.sh`, `tests/` (oracol C#, test JS), doar `build/windows`; `bin/eSignature` = executabil Mach-O arm64 netracked (artefact local) | `java/` (JAR, surse, `build-helper.sh`) |

- Protocoale expuse: WS `/ws` (protocol nou + protocol legacy compatibil cu aplicația C# `old-src/WiseMEDeSIGNATURE`), `POST /api/esignature/command`, acțiuni WSM `esignature.*` — CDC.
- Conflictul pe ruta `/` dintre `signing-pad` și `local-http` este evitat prin `normalizeLegacyConfig`, care forțează `shared_http: true` (`runner.go:189-207`, `signingpad/module.go:110-112`) — CDC.
- Șablonul `signing-pad-utility` indică JAR-ul la `./java-helper/signing-pad-helper.jar`, dar în sursă el se află în `apps/signing-pad-utility/java/` și `deployments/` conține doar `config.install.yaml` și `help/` — CDC (verificat prin listare). Cum ajunge JAR-ul la calea din șablon la instalare: NEC.
- Comportamentul cu hardware Signotec real: NEC (niciun test nu îl acoperă).

### 3.5 `html-print-formatter`

- **Funcție**: randare HTML → DOM serializat / PDF. `POST /render*`, `/pdf`, `/api/html-print-formatter/{render,pdf,token}` — CDC.
- **Dependență externă**: **Chromium headless** prin `chromedp` — un proces persistent, context incognito per cerere, pornire ≤ 30 s (`modules/htmlprintformatter/renderer.go:40-87`); cale din `chrome_path` / env `CHROME_PATH`. La căderea Chromium `/health` întoarce 503 și nu există repornire automată a browserului — CDC.
- **Autentificare**: JWT HS256 cu `kid` (`auth.go:18-46`); orice principal WSS autentic trece fără JWT — CDC.
- **Particularități**: portul listenerului comun e suprascris de env `PORT` (`module.go:38-50`); cont de sistem dedicat la instalarea pe Linux; `docker/ubuntu`, `examples/`, `generate-help.py`. **Același port implicit 19113 ca `siui-bridge-history`** — CDC.

### 3.6 `siui-bridge`

- **Funcție**: integrare CNAS (SIUI / SIPE / eFactura / DocMED), 63 de operații asincrone (HTTP 202 + polling) — CDC; detalii în `_reports/04-cnas-siui.md §1–§7` și [end-to-end-flows.md](end-to-end-flows.md).
- **Dependențe externe**: serverele CNAS (implicit `https://siui.e-pias.ro`, `https://sipe.e-pias.ro`, `https://docmed.e-pias.ro`, `modules/siui/endpoints.go:7-9`) prin mTLS TLS 1.2; **certificat/token criptografic**: magazinul de certificate Windows, Security.framework/CryptoTokenKit pe macOS 12+, PKCS#11 încărcat dinamic pe Linux (`modules.siui.pkcs11_module`); nu există variantă cu certificat din fișier (`_reports/04 §4.1`); WiseMED API pentru callback; `siui-bridge-history` pentru arhivă — CDC. Comportamentul real al CNAS: DN/NEC.
- **Persistență**: SQLite `siui-jobs.db`, `flock` exclusiv pe `<database>.lock` (o singură instanță, `modules/siui/module.go:143-161`), depozit de PIN criptat `siui-token-pins.json` — CDC.
- **Livrare**: și ca imagine Docker (`readersv3/docker/`).

### 3.7 `siui-bridge-history`

- **Funcție**: arhiva apelurilor bridge-ului; API `/api/history/*`, UI `/history-ui` — CDC (`_reports/04 §8`).
- **Dependențe externe**: MySQL (`go-sql-driver/mysql`, deschis leneș), stocare de obiecte `local` sau S3 — CDC.
- **Particularități**: ia `local-http-control` opțional (`modules/siuihistory/module.go:76`); port implicit 19113 identic cu `html-print-formatter`.

### 3.8 `wisemed-siui-data`

- **Funcție**: nomenclatoare CNAS (CLIN, PARA, PHM, PHYS, CIM-10) + personalizări per furnizor; achiziție prin `siui-bridge` via WSM (`api.request`) — CDC (`_reports/04 §9`).
- **Dependențe externe**: PostgreSQL (necesar `pg_trgm`), WSM, WiseMED API (identitate de echipament obligatorie, `echipament_id > 0`) — CDC.
- **Particularități**: bootstrap dedicat `apps/runner/siui_data_setup.go` (eroarea de bootstrap este **fatală** doar pentru acest protocol, `runner.go:74-92`); `config.yaml` scris cu permisiuni `0600` (`core/config/install.go:142-149`); migrarea ortografiei vechi `siu-data` → `siui-data` (`core/config/siui_data_migration.go`) — CDC.

### 3.9 `wisemed-ai`

- **Entrypoint**: `readersv3/apps/wisemed-ai/main.go:25-26` — CDC.
- **Dependențe externe**: PostgreSQL (în Docker: `pgvector/pgvector:pg16`) — singura dependență dură; **OpenAI** (`/v1/chat/completions`, `/v1/audio/transcriptions`) când `provider: openai` (implicit `mock`); conectori HIS allowlisted; surse de modele vocale; WiseMED API/WSM opționale (mod standalone: `wisemed_login_enabled: false`, `wsm_identity_enabled: false`, `wisemed_ws.enabled: false` în șablon) — CDC (`_reports/05 §1`).
- **Particularități**: conectare la PostgreSQL + migrări în `Init` dacă există `database_url` (afectat de dublul `Init`, vezi [runtime-and-modules.md §1.2](runtime-and-modules.md#12-dublul-init)); fără `database_url` pornește, dar `/v1/*` răspunde 503; `WISEMED_AI_DATABASE_URL` suprascrie configul; conține `web/` (Vue/Vite + Playwright), `clients/flutter`, `tools/{catalog,import-postgres}`, `docker/`; verificarea de update cere `cfg_wisemed_key`, deci o instalare standalone nu primește update-uri — CDC.

### 3.10 `update-server`

- **Entrypoint**: `readersv3/apps/update-server/main.go:19-65` — **nu folosește `runner.Run`**: `config.Ensure` → `config.Load` → `ensureUpdateServerBootstrap` (wizard propriu, `apps/update-server/bootstrap.go:12`) → `builtin.RegisterAll` → `runtime.New` → `InitModules` → `app.Start` — CDC.
- **Consecințe**: fără log în fișier (stdout), fără `-headless`/`-installservice`/`-showlog`, fără verificare de update (fără self-update), fără buclă de restart, fără `process-control`, **un singur `Init`** — CDC.
- **Listener**: propriu, deschis de modulul `app-update-server` pe adresa din `modules.local-http` (`modules/appupdateserver/module.go:165-227`), deși modulul `local-http` nu este încărcat; TLS opțional auto-generat — CDC.
- **Dependențe externe**: WiseMED API (login admin, `user_type` ∈ `allowed_user_types`); pentru `make-release` — **sursele `readersv3` și toolchain-ul Go** pe mașina serverului (`go run ./tools/releasectl release`, `module.go:1322-1446`) — CDC.
- **Persistență**: SQLite (tabele `applications`, `app_versions`), artefacte în `files_dir/<app_id>/<version>/`; sesiunile admin și tokenurile de download sunt doar în memorie — CDC.
- Protocolul client ↔ server: [build-deploy-update.md §6](build-deploy-update.md#6-protocolul-de-actualizare-client--update-server).

---

## 4. WSM Server (`wsm-server/`)

| Binar | Entrypoint | Rol |
|---|---|---|
| `wsm-server` | `wsm-server/cmd/wsm-server/main.go` | hub WSS multi-tenant, exclusiv în memorie, cu JWT HS256 per cheie (`kid`) legată de un tenant; fără coadă persistentă, replay sau livrare garantată |
| `wsmctl` | `wsm-server/cmd/wsmctl/main.go` | utilitar **offline** (nu contactează serverul): `keygen -out FILE`, `token …` (semnează JWT HS256, TTL implicit 5 min) |

CDC (`_reports/02-wsm-security.md §0, §5`).

- **Nu folosește runtime-ul `readersv3`** (fără `Module`/`Runtime`/`runner`): pachete proprii `internal/config`, `internal/server`, `internal/logstore`. Modul Go separat (§6).
- **Flag-uri**: `-config` (implicit `deployments/config.yaml`), `-check-config`, `-version`, `-showlog` (păstrat doar pentru compatibilitate) (`cmd/wsm-server/main.go:27-30`) — CDC.
- **Transport**: `GET /ws` (WSS), API HTTP (`/api/connections`, `/api/equipment/{id}`, `POST /api/connections/{id}/commands`), SDK de browser (`/sdk/wsm.js`, `/sdk/siui-bridge.js`), consolă `/control`, consolă `/admin` (`internal/server/server.go:131-173`) — CDC. Catalog complet: [api-catalog.md](api-catalog.md), `_reports/02 §4`.
- **Porturi**: imaginea Docker expune 8443 și 8090; `update-wsm.sh` publică `127.0.0.1:<port>:8090` în spatele Nginx (`deployments/nginx-wsm.conf`); `WSM_SERVER_PORT` suprascrie portul din config — CDC (`_reports/06` A.3, `_reports/02 §5`). Portul public de producție: NEC.
- **Persistență**: config YAML strict (tratat read-only), registrul scriibil `security.device_keys_file` (chei de device/browser, setări de tenant, origini — scriere atomică), loguri zilnice `wsm-YYYY-MM-DD.log` fără retenție automată. La restart se pierd conexiunile, abonamentele, tichetele, sesiunile admin — CDC (`_reports/02 §5, §12`).
- **Dependențe externe**: WiseMED API al fiecărui tenant (`PUT <base_url>/administrative/login` pentru consola admin, proxy pentru comenzile `mode:"server"`); Nginx ca terminator TLS (opțional) — CDC pentru partea din repo, DN pentru backend.
- **Clienți**: modulul `wisemed-ws` din 31 de aplicații `readersv3` ([runtime-and-modules.md §8](runtime-and-modules.md#8-clientul-wisemed-ws)), SDK-ul JS de browser, wrapperul PHP `wsm-server/examples/php/WSMHttpClient.php`.
- **Verificare** — CTL (`_reports/06` B.1 rândurile 5–10): `go build`, `go vet`, `go test` și `go test -race` trec în `wsm-server/` (3 pachete cu teste); testul de integrare `wsm-server/integration` (server WSM cu TLS pe loopback + modulul reader real, reconectare după ~30 s) trece. Testul Node al SDK-ului are 1 eșec (catalogul `siui-bridge.js` nu acoperă ruta `report/package`, `_reports/06` E1).
- **Particularități**: `wsm-server/web/` (UI de test vechi) nu este servit de nicio rută; `wsm-server/dist/*/examples/reader` (cinci copii ale aceluiași `main.go`) fac parte din modulul Go și sunt compilate de `go build ./...`; `wsm-server/output/bin/` conține binare și cinci backup-uri urmărite în git (`_reports/06` A.4) — CDC.

---

## 5. Porturi implicite și coliziuni

Valorile sunt implicitele din șabloane (CDC); valorile din instalări reale sunt NEC.

| Port | Aplicații | Observație |
|---|---|---|
| `127.0.0.1:18080` (HTTP local) | toate cele 22 de readere | două readere pe aceeași stație se ciocnesc; `bindguard.HandleAddressInUse` propune altă adresă și o scrie în config (`modules/localhttp/module.go:413-423`) |
| `19113` | `html-print-formatter` și `siui-bridge-history` | conflict doar pe același host (`_reports/06` contradicția 14) |
| TCP `5150` | `horiba-yumizen-h500-reader`, `erba-mannheim-laura-reader` | port de analizor |
| TCP `9000` | `gemini-reader`, `snibe-maglumi-x3` | port de analizor |
| TCP `8000` | `labnovation-ld-560`, `lifotronic-h8-reader` | port de analizor |
| `8010`, `18120`, `19090`, `19110`, `19111`, `19112`, `19114`, `19120` | `barcodeprinter`, `anaf-docsmart`, `update-server`, `signing-pad-utility`, `esignature-server`, `siui-bridge`, `wisemed-siui-data`, `wisemed-ai` | unice |
| `8443`, `8090` | `wsm-server` (Docker) | — |

Pentru porturile de analizor: ASTM folosește `bindguard` (propune alt port și **rescrie `config.yaml`**, `modules/protocols/astm/module.go:431-498`); LAURA și Zonci doar reîncearcă la 3 s (`_reports/03 §1.4`) — CDC.

---

## 6. Module Go (4 fișiere `go.mod`)

| `go.mod` | Modul | Go | Rol | Stare |
|---|---|---|---|---|
| `go.mod` (rădăcină) | `wisemed-labreaders` | 1.19 | **orfan**: niciun fișier `.go` în pachetul rădăcină; singura dependență `github.com/hooklift/gowsdl v0.5.0`, nereferită nicăieri; `replace wisemed-labreaders => ./` | CDC (`_reports/01 §7`). `go list ./...` găsește un singur pachet (`output/wsm-browser-sdk/examples/reader`), iar `go vet ./...` **eșuează** pe el (importuri inexistente în acest modul) — CTL (`_reports/06` B.1 rândurile 11, 11b). Motivul păstrării: INF (generare istorică de clienți SOAP) |
| `readersv3/go.mod` | `wisemed-labreaders/readersv3` | 1.24.0 | toate cele 32 de aplicații, `core/`, `modules/`, `shared/`, `tools/` (101 pachete) | `go build`, `go vet`, `go test`, `go test -race` trec pe darwin/arm64 (33 de pachete cu teste, 68 fără, 60 de teste sărite din lipsă de infrastructură) — CTL (`_reports/06` B.1 rândurile 1–4) |
| `wsm-server/go.mod` | `wisemed-labreaders/serverlast/wsm-server` | 1.24.0 | `cmd/wsm-server`, `cmd/wsmctl`, `internal/*`, `examples/reader`, copiile din `dist/` (11 pachete) | CTL (rândurile 5–8) |
| `wsm-server/integration/go.mod` | `wisemed-labreaders/serverlast/wsm-server/integration` | 1.24.0 | test de integrare WSM ↔ modulul reader din `readersv3` | CTL (rândurile 9–10; fără `-race`) |

Căile de modul și versiunile Go: CDC (verificat direct în cele patru fișiere). Nu există `go.work`; `releasectl` compilează cu `GOWORK=off`. Versiunile de Go din Dockerfile-uri diferă de `go.mod` (`_reports/06` contradicția 13).

Structura internă a `readersv3/`: `apps/` (aplicații + `runner`), `core/{config,module,runtime}`, `modules/` (51 de pachete; registru unic în `modules/builtin/register.go`), `shared/` (10 pachete: `appmeta`, `appupdates`, `apibridge`, `bindguard`, `localtls`, `commtrace`, `debugreplay`, `analyzeractivity`, `history`, `astm`; `shared/legacy/{contracts,model}` sunt directoare goale), `tools/{releasectl,docker-runtime}` — CDC (`_reports/03` S4, `_reports/06` B.3).

---

## 7. Zone auxiliare ale repository-ului

| Zonă | Conținut | Rol | Etichetă |
|---|---|---|---|
| `apis/wisemed-api/` | 14 IDL-uri Thrift cu adnotări `//yaml:` → OpenAPI (`yamlgenerate.sh`, `yaml/final.yaml`); cod generat (`gen-php`, `gen-html`, `lib`, `php/`); `apiv2/` = **server REST PHP** (~110 rute, `docs/wmapi.json`, Postman) | specificația și stratul de rutare al API-ului WiseMED consumat de readere. Logica de business (`$ACE_System_obj`, `medicalpages.lab`) **nu** este în repo. Niciun cod Go nu importă directorul; legătura este doar contractul HTTP | CDC (`_reports/03 §5.4–§5.5`); dacă e versiunea din producție: NEC |
| `apis/mp-api/` | adaptor PHP care implementează același contract de reader peste aplicația „Medical Pages”; conține o cheie API hard-codată în `index.php` (necopiată aici) | backend alternativ pentru aceleași rute | CDC |
| `analyzers-comm-protocols/` | documentație de protocol de la producători și surse Delphi `.pas` pentru `autobio-autolumo-s900`, `biomerieux-minividas`, `horiba-abx-pentra400`, `horiba-yumizen-h500`, `lifotronic-h8`, `yumizen-h500`, `zonci-xl1000i` | material de referință; implementările Go nu au fost comparate cu el | CDC (existență); conformitatea: NEC (`_reports/03` necunoscuta 9) |
| `docs/` | `CNAS/` (PDF, WSDL/XSD oficiale), `DRG/`, `signotec/` (SDK furnizor: `STPadLib.dll`, `STPadServer.exe`, drivere, plus `server.pem`/`serverkey.pem` — conținut neinspectat), `erba-mannheim-laura/`, `WisemedAI.md`, `cnas-*.md`, `architecture/` (acest set) | documentație și binare de furnizor; `docs/signotec/WebSocketPadServer/STPadLib.dll` este **dependență de build** pentru `esignature-server` | CDC (`_reports/06` A.1, A.4) |
| `aimemory/` | `wisemed-labreaders.md`, `wsm-server.md` | note de memorie pentru agenți; prima jumătate a `wisemed-labreaders.md` descrie o generație anterioară (`Readers Last/generic-test-reader`) inexistentă în repo | CDC (`_reports/01 §8 #3`) |
| `old-src/` | surse C#/.NET istorice: `MedicalPAGES.SIUIBridge`, `WiseMEDeCARDManager`, `WiseMEDeSIGNATURE`; ignorat de git; zero fișiere Go | referință pentru portările `siui` și `signing-pad` (protocolul WS legacy e emulat de `modules/signingpad/legacy.go`); `WiseMEDeCARDManager` și `CeasMedicalDataWS` nu au echivalent Go | CDC conținut; INF rol (`_reports/01 §7`, `_reports/04 §0`, `_reports/05 §4`) |
| `DocSMART/` | în git doar 6 fișiere de date `work/Df1102X35-*/{import.xml,template.pdf}`; `cmd/docsmart-importer` și `internal` sunt directoare goale neversionate | rămășița unui prototip standalone; implementarea reală este `readersv3/modules/protocols/anafdocsmart` + `readersv3/apps/anaf-docsmart` | CDC conținut; INF istoric |
| `output/` | `siui-data-research/` (2 fișiere de audit ICD10); `wsm-browser-sdk/` (binare `wsm-server`/`wsmctl` cu backup-uri, consolă `control/`, config-uri, documentație, `examples/reader/main.go`) | pachet de livrare al WSM pentru integratori; ține de `wsm-server`, nu de `readersv3`. Conține `config.yaml` urmărit în git (conținut neinspectat) | CDC (`_reports/01 §7`, `_reports/06` A.4) |
| `tmp/` | `tmp/pdfs/` cu un element; ignorat de git | scratch | CDC |
| `readersv3/output/` | config-uri runtime reale și baze de date per aplicație; `update-server/deployments/files/**` cu arhive și installere | stare de rulare locală; deliberat necitită | NEC (conținut) |
| Directoare șterse | `comm/`, `implementation/`, `protocols/`, `server/` | structura readerelor pre-v3; apar doar ca ștergeri de `.DS_Store` în indexul git. `Readme.md`, `Service.md`, `InstallPI.md` din rădăcină le descriu încă | CDC (`_reports/06` contradicția 12) |

---

## 8. Divergențe între rapoarte, rezolvate la redactare

| Subiect | Raport A | Raport B | Rezoluție |
|---|---|---|---|
| SQLite la `esignature-server` | `_reports/01 §5.2`: „opțional (`log_db_path`; niciuna dacă e gol)” | `_reports/05 §0`: `esignature-log.db` | Ambele corecte: șablonul setează `log_db_path: ./esignature-log.db` (`apps/esignature-server/deployments/config.install.yaml:55`), iar modulul deschide istoricul doar dacă valoarea e nevidă — CDC |
| Locul JAR-ului `signing-pad-utility` | `_reports/01`: `java/` | `_reports/05`: `./java-helper/signing-pad-helper.jar` | Sursa e în `apps/signing-pad-utility/java/`; șablonul indică `./java-helper/…` (`config.install.yaml:42`), cale inexistentă în `deployments/` din sursă — CDC; mecanismul de copiere: NEC |
| `apps/esignature-server/bin/eSignature` | `_reports/01`: conținut neverificat | `_reports/05`: Mach-O arm64 netracked | Se reține `_reports/05` (apare ca `??` în `git status`) |
| „backendul WiseMED nu e în repo” | `_reports/02 §16`, `_reports/04` | `_reports/03 §5.4` | Stratul REST PHP este în `apis/`; lipsește doar logica de business (se reține `_reports/03`) |

Lacune cunoscute ale acestui catalog: valorile implicite exacte ale `horibaabxpentra400.readConfig` și host/port implicit LD-560 din cod (NEC, `_reports/03`); proveniența `echipament_id` pentru utilitarele login-only (NEC, `_reports/02 §6`); conținutul `apps/wisemed-ai/{web,clients}` și `tools/import-postgres` (doar existența constatată).
