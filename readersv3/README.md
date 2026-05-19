# readersv3

`readersv3` este fundația modulară pentru următoarea generație de readere.

Obiectivul este:

- un nucleu comun pentru pornire, configurare, meniu și lifecycle
- module activabile per reader
- setări separate per modul în YAML
- protocoale și transporturi decuplate de UI și de storage
- funcționalitate comună care se propagă în toate readerele doar prin recompilare

## Structură

- `core/config`: configurare comună
- `core/module`: contracte, registry, meniu
- `core/runtime`: runtime-ul aplicației și lifecycle-ul modulelor
- `modules/*`: module built-in
- `apps/*`: aplicații concrete care aleg modulele active
- `apps/<reader>/deployments`: template-ul sursă pentru runtime payload (`config.install.yaml`)
- `output/<reader>`: workspace local de dezvoltare/test, nu sursa pentru installer/update

## Layout output

Pentru `readersv3`, layout-ul de runtime este:

- `output/<reader>/<binary>`
- `output/<reader>/deployments/config.yaml`
- `output/<reader>/deployments/config.install.yaml`

Binarele pornesc implicit cu `-config deployments/config.yaml`, deci pot rula direct din rădăcina directorului `output/<reader>`.

## Installer și Update

Versionarea aplicației este embedded direct în binar la build. Configul nu mai este sursă de adevăr pentru versiune.

Fluxul de config este:

- installer/update livrează doar binarul și `deployments/config.install.yaml`
- la prima pornire, dacă lipsește `deployments/config.yaml`, aplicația îl creează automat din `config.install.yaml`
- la pornirile următoare, `config.yaml` nu este suprascris
- dacă `config.install.yaml` aduce chei noi, ele sunt adăugate prin merge non-distructiv în `config.yaml`
- valorile deja configurate de client au prioritate

Packaging:

- `make installer APP=<app> TARGET=<goos-goarch>` produce installerul nativ
- `make update APP=<app> TARGET=<goos-goarch>` produce arhiva de update
- `make release APP=<app> TARGET=<goos-goarch>` produce release-ul complet
- pachetul de update conține doar binarul nou și fișierele statice din `deployments/`, în special `config.install.yaml`
- pentru țintele Windows, `make release` produce și installerul NSIS nativ cu `makensis`, fără Docker și fără Wine
- artefactele finale sunt scrise în `dist/updates`, `dist/installers` și `dist/releases`
- `config.yaml`, bazele de date, logurile, cache-ul și artefactele runtime nu intră în payload

Versiunea embedded se poate verifica cu:

- `<binar> -version`

Opțiunea `-headless` nu dezactivează modulele aplicației. Ea pornește același reader în fundal, ca proces de tip daemon/service, păstrează WS și local HTTP/HTTPS active, afișează PID-ul și logul folosit, apoi predă promptul înapoi shell-ului. Pe Windows procesul este detașat fără a rămâne atașat de fereastra de consolă.

Toate path-urile relative din configurile de modul se rezolvă față de directorul configului, adică `deployments/`. Asta înseamnă:

- `help_dir: ./help` => `output/<reader>/deployments/help`
- `path: ./reader.db` => `output/<reader>/deployments/reader.db`
- `import_dir: ../inbox` => `output/<reader>/inbox`

## Ce este comun în v3

Pe baza implementărilor din `readerslast`, următoarele zone trebuie tratate ca shared și mutate în module comune:

- HTTP/HTTPS local și shell-ul UI
- storage SQLite
- event/log bus pentru UI și diagnoză
- integrarea WiseMED API
- conexiunea WiseMED WS
- login și sesiune locală
- dashboard și statistici zilnice
- catalogul de analize
- managementul analizelor
- QC și Westgard
- help runtime
- managementul probelor zilnice
- transporturile către analizor: fișier, serial, TCP/IP
- protocoalele: generic file, Seegene Excel, BEOSL CSV, ASTM, IR Biotyper

`management analize` este separat de simplul catalog de analize tocmai ca schimbările funcționale din acest modul să fie vizibile în toate aplicațiile care îl folosesc.

## Model

Un reader declară doar:

1. ce module activează
2. ce setări are fiecare modul
3. ce protocol specific folosește

Toate aplicațiile v3 standard activează aceleași module shared de bază:

- `storage-sqlite`
- `events`
- `wisemed-api`
- `wisemed-ws`
- `login`
- `help`
- `dashboard`
- `analytes`
- `analyte-management`
- `qc`
- `stats`
- `daily-orders`

Exemplu de diferențiere:

- `generic-test-reader`: file transport + generic file protocol
- `seegene-reader`: file transport + seegene excel protocol + QC avansat
- `beosl-reader`: file transport + beosl csv protocol
- `gemini-reader`: tcp/ip transport + astm protocol
- `barcodeprinter`: utilitar de tipărire etichete, compatibil `GET/POST /barcode/print`, plus control prin WS (`barcode.print`)

## Stadiu

În acest pas există:

- runtime modular funcțional
- registry de module
- injecție automată în meniu
- registry de servicii între module
- module built-in pentru UI local, storage, evenimente, WiseMED API/WS, login, help, dashboard, analytes, management analize, QC, stats, orders și transporturi
- aplicații v3 pentru cele patru readere

Migrarea logicii complete din `readerslast` se poate face incremental, modul cu modul.

## Propagare schimbări

Dacă adaugi funcționalitate nouă în `modules/analytemanagement`, toate readerele care activează `analyte-management` o vor primi după recompilare, fără copiere manuală de cod între aplicații. Același model se aplică pentru QC, dashboard, WS, storage și celelalte module shared.
