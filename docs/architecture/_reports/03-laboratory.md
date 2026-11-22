# 03 — Laborator: transporturi, protocoale, SQLite, QC, sincronizare WiseMED

Investigație read-only, 2026-11-14, checkout local `main` (HEAD `d53088e`). Prescurtare: `R/` = `readersv3/`.
Etichete: **[COD]** = CONFIRMAT DIN COD · **[TEST]** = CONFIRMAT PRIN TEST LOCAL · **[DOC]** = DOCUMENTAT, NEVERIFICAT · **[INF]** = INFERENȚĂ · **[NEC]** = NECUNOSCUT.

**Limite de metodă.** Nu am rulat `go test`, binare sau servicii (interdicție explicită). Prin urmare **nicio afirmație nu poartă [TEST]**; testele sunt inventariate din surse („test existent, nerulat”). Toate testele existente sunt simulate (parsere pe fixture-uri, `net.Pipe`/loopback, `httptest`): chiar rulate, **nu confirmă funcționarea cu analizoare reale, porturi seriale reale sau serverul WiseMED real**.

---

## 0. Constatări structurale care schimbă lectura întregului subiect

| # | Constatare | Dovadă | Etichetă |
|---|---|---|---|
| S1 | Modulele `transport-serial` și `transport-tcpip` sunt **stuburi** (un `Logf` + `<-ctx.Done()`). Tot I/O-ul serial/TCP este implementat **în fiecare modul de protocol**. `transport-file` doar creează directoarele și publică un `map` de căi. | `R/modules/transports/serial/module.go:14-18`, `tcpip/module.go:14-18`, `file/module.go:26-57` | [COD] |
| S2 | Modulele `events`, `dashboard`, `stats`, `qc`, `analytes`, `analyte-management`, `daily-orders` sunt **stuburi de meniu + rută `/meta`**. Logica reală (rute, QC, dashboard, loguri) este în `R/modules/localhttp/module.go` și `R/modules/storage/sqlite/module.go`. | `events/module.go:15-26`, `dashboard/module.go:14-25`, `stats/module.go:14-27`, `qc/module.go:16-40` | [COD] |
| S3 | Nu există un „event bus”: nici canal de publicare, nici SSE/WS către UI. UI-ul face **polling HTTP** (`setInterval` pe `/api/logs`, `/api/status`, `/api/dashboard`). | `events/module.go:17-20` (doar un `map` de „features”); `R/modules/localhttp/ui/app.js:3573,3587` | [COD] |
| S4 | `R/shared/legacy/{contracts,model}` sunt directoare **goale**. `R/shared/history` nu ține de laborator (protocol de arhivă SIUI Bridge ↔ History). | `ls R/shared/legacy`; `R/shared/history/protocol.go:1` | [COD] |
| S5 | `apis/wisemed-api/apiv2` și `apis/mp-api` conțin **implementări PHP ale rutelor REST** consumate de readere (strat subțire peste obiecte de business `$ACE_System_obj`/`medicalpages.lab` care **nu** sunt în repo). Deci contractul nu este integral „extern”: stratul de rutare este în repo, logica de business nu. | `apis/wisemed-api/apiv2/implementation/analyzers.php:2`, `file_services.php:218,249`, `administrative.php:71,94,366`, `logins_user.php:2`; `apis/mp-api/api_implementation.php:140-462` | [COD] |

---

## 1. Transporturi

### 1.1 Ce face efectiv fiecare modul de transport

| Modul | ID | Config citit | Comportament real | Predare către protocol |
|---|---|---|---|---|
| file | `transport-file` | `modules.transport-file.{import_dir,processed_dir,failed_dir,export_dir,pattern}`; implicit `./inbox`, `./processed`, `./failed`, `./outbox`, `*` | `MkdirAll` + fallback pe director implicit dacă cel configurat nu se poate crea; scrie un audit log; **nu scanează nimic** | `RegisterService("transport-file", map{import_dir,…,glob})` — `file/module.go:36-43`; citit de `fileimportbase.fileTransport()` (`R/modules/protocols/fileimportbase/module.go:398`) [COD] |
| serial | `transport-serial` | nimic | log „serial transport configured” | Niciuna. Protocoalele citesc direct `rt.ModuleSettings("transport-serial")` [COD] |
| tcpip | `transport-tcpip` | nimic | log „tcp/ip transport configured” | Niciuna. Protocoalele citesc direct `rt.ModuleSettings("transport-tcpip")` [COD] |

Selectorul serial/TCP este `analyzer.comm_type`, expus ca serviciu `analyzer-config` (`R/core/runtime/runtime.go:43`).

### 1.2 Polling de fișiere (`fileimportbase`, plus copiile proprii din `cary60uvvis` și `shimatzutocl`)

| Aspect | Comportament | Dovadă |
|---|---|---|
| Declanșare | `time.Ticker` la `poll_seconds` (implicit 2 s; `<=0` dezactivează), plus o scanare imediată la pornire | `fileimportbase/module.go:188-211` [COD] |
| Selecție | `filepath.Glob(import_dir/pattern)`, secvențial, într-o singură goroutine | `:229-256` [COD] |
| Fișier în curs | `map running` protejat de `sync.Mutex` (`begin/end`) — protejează doar față de importul manual concurent | `:992-1006` [COD] |
| Stabilitate fișier | **Nu există** verificare de dimensiune stabilă / lock / „stable wait”. Un fișier încă în scriere de către analizor este parsat așa cum e. | absența oricărei verificări în `:229-347` [COD]; consecința (import parțial) [INF] |
| Succes | `os.Rename` în `processed_dir` | `:253`, `:979-990` [COD] |
| Eșec parsare/DB | `os.Rename` în `failed_dir`; rândurile deja scrise în SQLite **rămân** (importul nu e tranzacțional) | `:248-251`, `:290-307` [COD] |
| Eroarea de arhivare | ignorată (`_ = m.archive(...)`); fișierul rămas în inbox este reimportat la fiecare tick | `:250,253` [COD]; reimportul repetat [INF] |
| Import manual | `POST /api/orders/import` → copiere în `.tmp-imports/` → `ImportFileNow` (fără arhivare) | `R/modules/localhttp/module.go:1689-1731` [COD] |
| Trace | `logInboundFile`, `logParsedPreview`, `logIgnored`, pe niveluri `logging.verbose_level` 4–5 | `fileimportbase/module.go:877-968` [COD] |
| Excepție | `protocol-generic-file` **nu importă nimic** (doar `/api/protocol/meta`) | `R/modules/protocols/genericfile/module.go:13-22` [COD] |

### 1.3 Serial (RS-232) — implementat per protocol, biblioteca `go.bug.st/serial`

| Protocol | Parametri (din `transport-serial`) | Implicit în cod | Read timeout | Reconectare | Dovadă |
|---|---|---|---|---|---|
| miniVIDAS | `port, baud, data_bits, stop_bits, parity` | 38400/8/N/1 (template-ul de instalare pune 9600) | 500 ms | buclă: eroare → `Logf` → 3 s → redeschidere; port neconfigurat → reîncercare la 3 s | `biomerieuxminividas/module.go:71-93,95-103,286-300` [COD] |
| Pentra 400 | idem + `sender_name`, `order_lookup_days` | [NEC] valorile implicite din `readConfig` (`:723`) necitite integral | 500 ms | idem, 3 s; audit `horiba_serial_error` | `horibaabxpentra400/module.go:95-121` [COD] |
| LAURA | `port, baud, data_bits, stop_bits, parity` | 9600/8/N/1 | 500 ms | buclă 3 s | `erbamannheimlaura/module.go:142-196` [COD] |
| Zonci | `port, baud` | 8/N/1 fix; baud din config (template 115200) | 500 ms | buclă 3 s; audit `communication_error` | `zoncixl1000i/module.go:71-93,116-128` [COD] |
| ASTM generic | — | **Nu suportă serial**: dacă `comm_type != tcpip` intră în „idle” | — | — | `astm/module.go:177-181` [COD] |

### 1.4 TCP

| Protocol | Mod | Bind implicit | Client: dial / pauză | Server: per conexiune | Port ocupat | Dovadă |
|---|---|---|---|---|---|---|
| ASTM | server sau client (`transport-tcpip.mode`) | `127.0.0.1:9000` | `DialTimeout` 10 s; eșec → 5 s; după închidere → 2 s | `go handleConn` nelimitat | `bindguard`: propune alt port și **rescrie `config.yaml`** | `astm/module.go:431-498,1199-1210` [COD] |
| LD-560 | server/client, repornire la schimbarea setărilor (`restartCh`) | [NEC] valoarea implicită exactă | dial 10 s; 5 s / 2 s | goroutine per conexiune | [NEC] | `labnovationld560/module.go:88-150,151-250,1170-1180` [COD] |
| LAURA | server/client | `127.0.0.1:5150` | `Dialer{Timeout:5s}`; reconectare prin bucla externă (3 s) | goroutine per conexiune | eroare → bucla de 3 s (fără bindguard) | `erbamannheimlaura/module.go:197-245` [COD] |
| Zonci | doar server | din config (template: `0.0.0.0:3125`) | — | goroutine per conexiune; cadrele serializate cu `m.mu` | eroare → 3 s | `zoncixl1000i/module.go:94-115,193-197` [COD] |

Niciun listener de analizor nu are autentificare, TLS, allow-list de IP sau limită de conexiuni [COD — absență în fișierele de mai sus].

### 1.5 Trace / diagnoză

- ASTM: `tracePacket` scrie **o linie de log per octet primit** chiar la `verbose_level` 1 (`astm/module.go:589`, `:1471-1493`); nivel 2 adaugă text, ≥3 hexdump + starea checksumului (`:1495-1505`). [COD]
- `R/shared/commtrace/commtrace.go:9` — formatator comun (`==> IN` / `<== OUT`, text + hex la nivel >4); folosit de LAURA (`erbamannheimlaura/module.go:262`) și LD-560. [COD]
- `R/shared/analyzeractivity/analyzeractivity.go:18-62` — tracker în memorie (sesiuni conectate, ultimul pachet); „connected” rămâne true 5 s după ultimul pachet. Înregistrat de modulul `events` ca serviciu `analyzer-activity` (`events/module.go:17`). [COD]
- `R/shared/debugreplay` + `RunDebugReplay` (ASTM `astm/module.go:290`, LD-560 `:513`): rulează scripturi de test printr-un listener loopback; expus prin `/api/debug/tests[/run]` doar pentru `UserType <= 0` (`R/modules/localhttp/debug_replay.go:21-34`). Este simulare, nu hardware. [COD]

---

## 2. Protocoale

### 2.1 Inventar (21 pachete în `R/modules/protocols`)

`fileimportbase` este bibliotecă (nu se înregistrează singură). `irbiotyper` și `genericfile` sunt stuburi. `anafdocsmart` nu este protocol de laborator (declarații ANAF XML → PDF).

| Pachet | ID modul | Transport | Format | Direcție | ID probă | TAG | Valoare / flag | QC | Teste (nerulate) |
|---|---|---|---|---|---|---|---|---|---|
| `astm` | `protocol-astm` | TCP server/client | ASTM E1394 (înregistrări H/P/O/R/Q/L); framing tip E1381 configurabil | **Bidirecțional**: rezultate + host query (`Q`) → comandă `O` | căi configurabile `sample_id_paths` (impl. `O.3.1`,`O.2.1`), trim stânga/dreapta, fallback ID pacient | `result_id_paths` (impl. `R.2.4`,`R.2.1`) → `analyte_mappings` → `NormalizeAnalyteTag` | `R.3.1`; unitate `R.4.1`; flag `R.6.1`/`R.8.1` stocat în `interpreted_value` | prefix ID (`QC`,`CTRL`,`CONTROL`) → `qc_records`; nivel din `L1..L5/LEVELn`; lot fix `-` | `module_test.go` (10), `analyte_identity_test.go` (1) |
| `horibaabxpentra400` | `protocol-horiba-abx-pentra400` | Serial | ASTM E1394 peste ENQ/STX/ETX/EOT | **Bidirecțional** (host query + comenzi grupate pe tip probă) | `O.2`, fallback `P.3` | `R.2` componentele `^^^cod^tag` (`pentraAnalyteIdentifier`), fără normalizare | `R.3`, unitate `R.4`; timestamp `R.12` | nu | 1 test (`buildOrderRecord`) |
| `biomerieuxminividas` | `protocol-biomerieux-minividas` | Serial | Proprietar bioMérieux: câmpuri `\|` cu prefix de 2 litere (`ci,rt,rn,ql,qn,td,tt`), RS/GS | Unidirecțional (reader doar ACK) | `ci`, fără zerouri de început | `rt` → `NormalizeAnalyteTag` | `qn`, sau `ql` dacă analitul e în `qualitative_analytes`/`qn` gol; `ql` → `interpreted_value` | nu | 2 teste parser |
| `erbamannheimlaura` | `protocol-erba-mannheim-laura` | Serial sau TCP | Raport text STX…ETX „DEKAPHAN LAURA” | Unidirecțional, **fără ACK** | linia `ID:` (zerouri păstrate) | listă fixă de 10; `pH`→`PH` | col. 2; unitate rest; `*` → flag `abnormal` | nu | 3 teste (chunking, respingere incomplet/corupt, flux TCP) |
| `zoncixl1000i` | `protocol-zonci-xl1000i` | TCP server sau Serial | Proprietar STX `<cmd> <id> …` ETX; cmd `1` query, `2` comandă, `3` rezultate, `4` QC | **Bidirecțional** | al doilea token | codul din linie ↔ `flags.zonci_code` / TAG | col. 2 (`normalizeValue`), unitate col. 3; separator configurabil | cadrul `4` este **doar logat, nu stocat** (`:231`) | 4 teste parser |
| `labnovationld560` | `protocol-labnovation-ld560` | TCP server/client | „Simple” (`<TRANSMIT>…<I>…<R>…`) sau HL7 v2 (MLLP sau raw CR) | Unidirecțional; **nu trimite ACK HL7** | Simple: câmpuri `<I>`; HL7: `OBR.3.1` (+fallback) | Simple: cheie → `analyte_mappings`; HL7: `OBX.3.1`; fără normalizare comună | Simple: perechi; HL7 `OBX.5.1`, unitate `OBX.6.1`, flag `OBX.8.1` | Simple: `sample_mode_qc_values`; HL7: `OBR.11.1` ∈ {Q,QC,1} | 6 teste parser |
| `seegeneexcel` | `protocol-seegene-excel` | Fișier | CSV Seegene Viewer (în ciuda numelui) | Unidirecțional | `deriveSampleID(patientID,name,sampleNo,well)` | antet țintă → `NormalizeAnalyteTag` | Ct | după tipul rândului | 1 test |
| `cfx96quantitation` | `protocol-cfx96-quantitation` | Fișier | XLSX „Plate View Results” (+ fișier frate Ct) | Unidirecțional | `deriveSampleID(sampleRaw, well)` | fluorofor/țintă → `NormalizeAnalyteTag` | Cq + calitativ pe prag | `isQCContent` | 2 teste |
| `beoslcsv` | `protocol-beosl-csv` | Fișier | CSV dozimetrie | Unidirecțional + push dozimetrie | `DOSIMETER ID` | fix `HP007_CALC_DOSE`, `HP10_CALC_DOSE` | numeric | nu | fără |
| `cary60uvvis` | `protocol-cary60-uvvis` | Fișier | CSV secționat Cary WinUV | Unidirecțional (+ worklist tipărit) | `normalizeCary60SampleID` | din secțiune/nume fișier | concentrație × factor | rând care începe cu `PC` | fără |
| `shimatzutocl` | `protocol-shimatzu-tocl` | Fișier | TXT tabular TOC-L | Unidirecțional | normalizare proprie + reguli prefix/sufix | din coloane | numeric | `PC*`, `ETALON*` | 3 teste |
| `shimatzugeneric` | `protocol-shimatzu-generic` | Fișier | TXT GC cu secțiuni | Unidirecțional | din secțiunea de eșantion | nume compus → `NormalizeAnalyteTag` | concentrație | `PC*` | 1 test |
| `analytikjenaplasmaquantmselite` | `protocol-analytikjena-plasmaquantms-elite` | Fișier | CSV `;` | Unidirecțional | `deriveSampleID(label)` | element → `NormalizeAnalyteTag` | `pickResult` | `isQCRow` | fără |
| `anatoliageneworks` | `protocol-anatolia-geneworks` | Fișier | XLS (BIFF) | Unidirecțional | `Case ID` → primul cuvânt din `Label` → nr. rând | `Target` → `NormalizeAnalyteTag` | Ct („No Ct” → gol) + `Conclusion` | `Type != UNKNOWN` (NTC→negativ, POSITIVE→pozitiv) | fără |
| `biosanhipompp96` | `protocol-biosan-hipo-mpp96` | Fișier | CSV `;` (≥15 coloane) | Unidirecțional | col. 4, fallback cod slot | col. 14 → `NormalizeAnalyteTag` | col. 5; calitativ col. 6 | slot care **nu** începe cu `T` | fără |
| `gammavision` | `protocol-gammavision` | Fișier | TXT raport GammaVision | Unidirecțional | `Spectrum name` → descriere → nume fișier | nuclid, **cu `#` și spații eliminate** (`:156-162`) | activitate Bq/L; incertitudine, 2σ, MDA în flags | nu | fără |
| `tricarb5110tr` | `protocol-tricarb-5110-tr` | Fișier | CSV după antetul `S#,Count Time,CPMA,SIS` | Unidirecțional | col. `S#` | fix `CPMA`, `SIS` | numeric | nu | fără |
| `genericfile` | `protocol-generic-file` | (fișier) | — | **neimplementat** | — | — | — | — | fără |
| `irbiotyper` | `protocol-ir-biotyper` | — | — | **neimplementat** (doar meniu) | — | — | — | — | fără |
| `anafdocsmart` | `protocol-anaf-docsmart` | Fișier | XML ANAF (non-laborator) | — | — | — | — | — | fără |
| `fileimportbase` | (bibliotecă) | Fișier | — | — | `NormalizeImportedRecord` | — | — | — | 1 test (`TestSendUsesCorrectedFileID`) |

Dovezi principale [COD]: `astm/module.go:881-928,1199-1232,1744-1763`; `horibaabxpentra400/module.go:480-566`; `biomerieuxminividas/module.go:170-284`; `erbamannheimlaura/module.go:19-84`; `zoncixl1000i/module.go:193-404`; `labnovationld560/parser.go:100-147,180-230,332-400`, `module.go:768-910`; fișierele de parser indicate în tabel.

### 2.2 Protocol → aplicații

| Protocol | Aplicații (`R/apps/*/main.go:25`) | Transport activat | Port/pattern din `config.install.yaml` |
|---|---|---|---|
| `protocol-astm` | `autobio-autolumo-s900-reader`, `gemini-reader` (+`protocol-ir-biotyper`), `horiba-yumizen-h500-reader`, `lifotronic-h8-reader`, `snibe-maglumi-x3` | `transport-tcpip` | 11119 / 9000 / 5150 / 8000 / 9000, mod server |
| `protocol-horiba-abx-pentra400` | `horiba-abx-pentra400-reader` | `transport-serial` (+ `astm-specimen-settings`) | baud 9600 |
| `protocol-biomerieux-minividas` | `biomerieux-minividas-reader` | `transport-serial` | baud 9600 |
| `protocol-erba-mannheim-laura` | `erba-mannheim-laura-reader` | `transport-serial` + `transport-tcpip` | 9600 / 5150; `repeat_mode: grouped` |
| `protocol-zonci-xl1000i` | `zonci-xl1000i-reader` | `transport-tcpip` + `transport-serial` | 3125 / 115200 |
| `protocol-labnovation-ld560` | `labnovation-ld-560` | `transport-tcpip` | 8000 |
| `protocol-seegene-excel` | `seegene-reader` | `transport-file` | `*.csv` |
| `protocol-cfx96-quantitation` | `cfx96-reader` | `transport-file` | `*Quanti* Plate View Results.xlsx` |
| `protocol-beosl-csv` | `beosl-reader` | `transport-file` | `*.csv`; singurul cu `auto_confirm_wisemed: true` |
| `protocol-cary60-uvvis` | `cary60-uvvis-reader` (+`daily-details`) | `transport-file` | `*.csv` |
| `protocol-shimatzu-tocl` | `shimatzu-tocl-reader` | `transport-file` | `*.txt` |
| `protocol-shimatzu-generic` | `shimatzu-generic-reader` | `transport-file` | `*.txt` |
| `protocol-analytikjena-plasmaquantms-elite` | `analytikjena-plasmaquantms-elite-reader` | `transport-file` | `*.csv` |
| `protocol-anatolia-geneworks` | `anatolia-geneworks-reader` | `transport-file` | `*.xls` |
| `protocol-biosan-hipo-mpp96` | `biosan-hipo-mpp-96-reader` | `transport-file` | `*.csv` |
| `protocol-gammavision` | `gammavision-reader` | `transport-file` | `*.txt` |
| `protocol-tricarb-5110-tr` | `tricarb-5110-tr-reader` | `transport-file` | `*.csv` |
| `protocol-generic-file` | `generic-test-reader` | `transport-file` | `*.txt` (fără import) |

Toate cele 22 de readere activează în plus: `local-http, storage-sqlite, events, wisemed-api, wisemed-ws, login, help, dashboard, analytes, analyte-management, qc, result-sync, stats, daily-orders`. [COD]

### 2.3 ASTM generic — nivel de legătură (`R/modules/protocols/astm/module.go`)

| Element | Implementare | Linie | Etichetă |
|---|---|---|---|
| Recepție | automat pe octeți, `bufio.Reader.ReadByte`, deadline de citire ≤100 ms (deși `frame_timeout_ms` implicit 2000, este plafonat la 100 ms) | `:563-570` | [COD] |
| ENQ | răspunde ACK dacă `send_ack` (implicit true); golește cadrele acumulate | `:630-637` | [COD] |
| Cadru | STX … ETX/ETB + trailer așteptat: 2 caractere checksum (dacă `checksum_mode != none`) + CR LF (dacă `trailer_mode == crlf`) | `:593-628,1268-1277` | [COD] |
| Checksum | calculat ca sumă mod 256 peste payload + terminator, **doar pentru trace**. Un checksum invalid **nu produce NAK**: cadrul este acceptat și confirmat cu ACK. | `:1297-1320`, ACK necondiționat `:601-604,622-625` | [COD] |
| NAK emis de reader | **niciodată** (constanta `ctrlNAK` este folosită doar la recepție) | `:677-682` | [COD] |
| Număr de secvență | eliminat (prima cifră) fără verificarea continuității; ETB tratat ca ETX (fără reasamblare specială) | `:715-720,1148-1153` | [COD] |
| EOT | concatenează cadrele; dacă există `Q` → host query asincron; altfel `processBatch` | `:643-654` | [COD] |
| Timeout inactiv | la timeout de citire, cu cadre acumulate și fără cadru deschis → `processBatch` (lot fără EOT, după ≤100 ms de liniște) | `:572-581` | [COD] |
| Mod „linie brută” | linii în afara cadrelor; `L\|` declanșează procesarea | `:683-700` | [COD] |
| Host query — răspuns | reader trimite ENQ, așteaptă ACK, trimite cadru(e); mod `full_package` (un cadru + EOT lipit) sau `record_frames` (un cadru per înregistrare, EOT după ultimul ACK) | `:531-540,655-676,1042-1069` | [COD] |
| Retransmisie la NAK | **nu există**: la NAK răspunsul este abandonat | `:677-682` | [COD] |
| Timeout pe ACK-ul analizorului | **nu există**: `replyActive` rămâne true până la un ACK/NAK; răspunsurile ulterioare se pun în coadă | `:549-562` | [COD] |
| Contenție ENQ | netratată (fără back-off conform E1381) | — | [COD — absență] |
| Persistență vs ACK | ACK-ul este trimis **înaintea** persistenței; o eroare de DB în `processBatch` nu ajunge la analizor → rezultatul se pierde | `:601-604` vs `:649-651` | [COD]; pierderea [INF] |

Aceleași proprietăți (ACK necondiționat, fără verificare checksum, fără NAK/retransmisie) la Pentra (`horibaabxpentra400/module.go:137-186`) și miniVIDAS (`biomerieuxminividas/module.go:117-136`). La Pentra, căutarea WiseMED pentru host query rulează **sincron în bucla de citire serială** (până la 20 s WS + 20 s HTTP) (`:156-163,201-216`). [COD]

### 2.4 Host query (worklist)

| Protocol | Căutare locală | Fallback WiseMED | Răspuns | Filtru zilnic |
|---|---|---|---|---|
| ASTM | ultimele 7 zile, potrivire pe `sample_id`/`file_id`/`patient_id` (`:970-992`) | `FetchFileForAnalyzer` → creează comandă `scheduled` + analize cu `wisemed_sm_id/fsm_id` (`:994-1040`) | `H`, `P`, `O` (coduri `^^^cod` din `flags.analyzer_code` → `analytes.code` → TAG; specimen `O.16` = `specimen_code_default`), `L` (`:1042-1084`) | nu |
| Pentra | `order_lookup_days` (impl. 7) (`:568`) | idem (`:358-452`); upsertează și catalogul de analite | un `O` per cod de specimen (`ResolveSpecimenCode`, `R/shared/astm/specimen.go:12`); marchează `analyzer_send_status=sent` (`:307-333`) | da (`IsAnalysisSendBlocked`) |
| Zonci | `findOrder` | `loadOrder` (`:242-309`) | `STX 2 <id> <coduri> ETX` | da |

Fără comandă găsită, ASTM și Pentra trimit un răspuns gol (`H`+`L`) (`astm/module.go:955-958`, `horibaabxpentra400/module.go:346-352`). [COD]

**Defect condiționat la Pentra:** `stringValue(nil)` întoarce `"<nil>"` (`horibaabxpentra400/module.go:629-631`), deci dacă răspunsul WiseMED nu are `o_file_date`/`o_file_id`, comanda se salvează cu `order_date="<nil>"`/`file_id="<nil>"` (`:374-384`). ASTM și Zonci tratează `nil` corect (`astm/module.go:1102-1107`, `zoncixl1000i/module.go:515-520`). Dacă backendul omite vreodată câmpurile: [NEC] (copia PHP locală le completează mereu, `analyzers.php:41-55`).

---

## 3. Modelul de date SQLite (`R/modules/storage/sqlite`)

Driver `sqlite` (pur Go), fișier `modules.storage-sqlite.path` (implicit `reader-v3.db`), pragme `busy_timeout=5000`, `journal_mode=WAL`, `synchronous=NORMAL` (`module.go:94,144-147`). Serviciul `storage` este obiectul `*Store`; modulele îl consumă prin interfețe locale (type assertion). [COD]

### 3.1 Tabele (`module.go:148-326`)

| Tabelă | Coloane-cheie | Unicitate / index | Scrisă de |
|---|---|---|---|
| `rounds` | `order_date, round_no` | `unique(order_date, round_no)`; `idx_rounds_date_round` | `CurrentRoundNo`, `CreateNextRound`, `UpsertOrder` |
| `analytes` | `tag` (unic), `code`, `result_formula_code`, `result_type`, `transform_rules_json`, `protocol_options_json`, `active` | `tag unique` | UI (`/api/analytes`), auto-creare din orice protocol |
| `orders` | `round_no, order_date, sample_id, file_id, patient_id, patient_name, rack_*, sample_no, status, source_file, meta_json` | **nicio constrângere de unicitate**; `idx_orders_date_round` | protocoale, `result-sync`, `CorrectOrderID`, host query |
| `order_analyses` | `order_id, analyte_tag, wisemed_sm_id, wisemed_fsm_id, status, default_result_id`, valori curente + `source_*`, `flags_json` | fără unicitate; `idx_order_analyses_order` | protocoale, `result-sync`, transformări, trimitere |
| `order_analysis_results` | `order_analysis_id`, valori + `source_*`, `flags_json` (`panel_id`), `created_at` | `idx_order_results_analysis` | protocoale (append-only) |
| `qc_targets` | `analyte_tag, control_level, lot_no, expires_at, target_mean, target_sd, target_cv` | `unique(analyte_tag, control_level, lot_no)`; `idx_qc_targets_lookup` | UI; auto-creare cu medie/SD 0 la import (`fileimportbase/module.go:1022-1049`) |
| `qc_records` | `run_date, control_label, control_level, lot_no, file_id, status, source_file` | fără unicitate SQL (dedup aplicativ pe `run_date+label+level+lot`, `:1479`); `idx_qc_records_run_date` | protocoale, QC manual |
| `qc_analyses` | `qc_record_id, analyte_tag, lot_no, numeric_value, manual_entered_by/at, flags_json` | `idx_qc_analyses_record` | protocoale, QC manual |
| `audit_logs` | `level, event_type, actor, message, meta_json, created_at` | `idx_audit_logs_created_at` | `AppendAuditLog`, `CorrectOrderID` |
| `daily_detail_definitions` | `key` (unic), `scope`, `field_type`, … | `key unique` | UI / modulul `daily-details` |
| `daily_detail_values` | `definition_key, scope_date, round_no, analyte_tag, value_text` | `unique(definition_key, scope_date, round_no, analyte_tag)` | UI |
| `daily_analysis_send_filters` | `scope_date, analyte_tag, mode, specimen_codes_json` | index pe `(scope_date, analyte_tag)` | UI; citit de Pentra/Zonci înainte de a trimite comenzi |

Nu există chei străine declarate, nici `on delete cascade`; integritatea este menținută de cod (`DeleteOrder`, `DeleteOrderAnalysis`, `DeleteQCRecord`). [COD]

### 3.2 Migrări

- Fără tabel de versiuni. La fiecare pornire: `create table if not exists` + `alter table … add column` cu ignorarea erorii `duplicate column name` (`module.go:343-408`), urmate de backfill-uri QC (`:409-460`). [COD]
- **Atenție:** `update qc_analyses set control_level = '' where control_level <> ''` rulează la fiecare pornire (`:417`), imediat după backfill-ul din `:409-416`; `UpsertQCAnalysis` scrie și el mereu `''` (`:1570`). Nivelul de control efectiv vine din `qc_targets`/`qc_records` prin `coalesce` (`:1781`). [COD]
- Sidecar `<db>.ui.json` (analite + ținte QC) importat o singură dată și redenumit `.migrated` (`:464-500`). [COD]
- Erori de deschidere (read-only, I/O cu WAL/SHM) → mesaj explicit care interzice ștergerea WAL-ului (`:110-136`). [COD; test existent `open_errors_test.go`, nerulat]

### 3.3 Tranzacții

| Operație | Tranzacțională? | Dovadă |
|---|---|---|
| `RecordImportedPanel` (LAURA) | **Da** — comandă + analize + rezultate într-un `tx`, cu `panel_id` aleator | `panels.go:16-101` |
| `CorrectOrderID` | **Da** | `order_identity.go:26-142` |
| `DeleteOrder`, `DeleteOrderAnalysis`, `DeleteQCRecord`, compactarea rundelor | Da | `module.go:994-1028,1105-1143,1180-1205,1615-1632` |
| `SetDefaultResult` (mod `grouped`) | Da pentru actualizările grupului | `:1311-1334` |
| `RecordImportedResult` (toate celelalte protocoale) | **Nu** — trei scrieri independente: `UpsertOrder`, `SaveOrderAnalysis`, `UpsertOrderResultForAnalysis` | `:1380-1419` |
| `UpsertOrder` | Nu — `select` apoi `insert/update`, fără `tx` și fără constrângere unică | `:883-949` |
| Un fișier / un lot ASTM întreg | Nu — fiecare rezultat este o serie separată de scrieri | `fileimportbase/module.go:290-307`, `astm/module.go:750-844` |

Consecință [INF]: o eroare la mijlocul unui lot lasă un import parțial; două goroutine care inserează simultan același `(order_date, round_no, sample_id)` pot crea comenzi duble (posibil la serverele TCP cu goroutine per conexiune).

### 3.4 Identitatea probei, runde, repetări

| Subiect | Regulă | Dovadă |
|---|---|---|
| Cheie logică a comenzii | `(order_date, round_no, sample_id)`; căutare secundară după `meta_json.$.id_correction.original_sample_id`, astfel încât rezultatele noi ale unui ID corectat ajung la aceeași comandă | `module.go:903-912`; `panels.go:48-61` [COD] |
| Data | din mesaj (ASTM `O.7/O.6/H.13`, miniVIDAS `td`, LAURA data raportului, fișiere: câmpul de dată); altfel ziua curentă. Pentra folosește **mereu ziua curentă** la rezultate (`horibaabxpentra400/module.go:531`) | [COD] |
| Runda | importul scrie mereu în `max(round_no)` al zilei (`CurrentRoundNo`, `:832-845`); runde noi doar din UI (`CreateNextRound`); ștergerea unei runde goale renumerotează rundele și `daily_detail_values` (`:1180-1205`) | [COD] |
| Rezultat repetat | fiecare recepție **adaugă** un rând în `order_analysis_results` (fără deduplicare) și devine automat rezultatul implicit (`UpsertOrderResultForAnalysis` → `SetDefaultResult(…,"individual")`, `:1241-1255`) | [COD] |
| Politica de repeat | `modules.local-http.repeat_mode`: `individual` (toate aplicațiile) sau `grouped` (doar LAURA). Afectează **doar selecția manuală** (`PUT /api/results/default`): în `grouped`, alegerea unui rezultat comută întregul panou cu același `panel_id` (sau același `created_at` la secundă pentru date vechi) | `module.go:1257-1335`; `R/modules/localhttp/module.go:2486-2510,2897-2910` [COD; test existent `panels_test.go`, nerulat] |
| Valori sursă vs transformate | `source_*` păstrează valoarea analizorului; `result_value/raw_value/interpreted_value` pot fi rescrise de regulile `transform_rules_json` (value-map, formule Go-AST) la `ReapplyOrderTransformations` | `transformations.go:75-162,218-255,329-441` [COD] |
| **Efect secundar la re-recepție** | `RecordImportedResult` apelează `SaveOrderAnalysis` cu `WiseMEDSMID/FSMID` goale și `Flags` = flagurile noii recepții → **șterge `wisemed_sm_id`, `wisemed_fsm_id` și starea de trimitere** ale analizei existente (`update … wisemed_sm_id=?,wisemed_fsm_id=?,…,flags_json=?`). Tot atunci `UpsertOrder` suprascrie `file_id`, `patient_id`, `patient_name` cu valorile analizorului (excepție: comenzile cu `id_revision>0`). | `module.go:1399-1413` + `:1089-1090`; `:925-936` [COD] |

Zonci nu este afectat (salvează structura existentă, `zoncixl1000i/module.go:383-390`), iar LAURA nici ea (`RecordImportedPanel` nu atinge coloanele WiseMED, `panels.go:96`). Sunt afectate: ASTM, Pentra, miniVIDAS, LD-560 și toate importurile de fișiere. Vezi §6.5 pentru consecința asupra trimiterii.

### 3.5 Normalizarea TAG-urilor

- `NormalizeAnalyteTag` (`R/modules/core/model/analyte_tag.go:10-17`): `TrimSpace`, spațiu → `_`, majuscule; **punctuația se păstrează** (`NEU#` ≠ `NEU%`, `P-LCR`, `RDW-CV`). [COD; teste existente `analyte_tag_test.go`, `astm/analyte_identity_test.go`, `resultsync/analyte_tag_test.go`, nerulate]
- Folosit de: ASTM, miniVIDAS, Seegene, CFX96, Analytik Jena, Anatolia, Biosan, Shimadzu Generic. [COD — `grep NormalizeAnalyteTag`]
- **Nu** îl folosesc: Pentra (TAG brut), Zonci (cod brut), LD-560 (`mapAnalyte`), LAURA (`ToUpper` pe listă fixă), GammaVision (elimină `#` și spații — normalizare proprie care contrazice regula generală, dar pentru nuclizi), TriCarb/BEOSL (TAG-uri fixe). [COD]
- Mapări explicite: `protocol-astm.analyte_mappings` (`astm/module.go:756,1555`), `hl7/simple.analyte_mappings` la LD-560. [COD]
- Potrivirea cu WiseMED este pe `ToUpper(TrimSpace(tag))` == `ToUpper(t_tag)` (`resultsync/module.go:749-790`). [COD]

### 3.6 Istoric

`order_analysis_results` (toate recepțiile), `audit_logs` (acțiuni operator + evenimente de protocol), `orders.meta_json.id_correction_history` (toate corecțiile de ID). Jurnalul afișat în UI este un `UNION` calculat la citire peste `order_analysis_results`, `qc_analyses`, `audit_logs` (`module.go:1720-1770`) — nu există tabel `event_logs`. [COD]

---

## 4. Corectarea ID-ului primit de la analizor

| Aspect | Implementare | Dovadă |
|---|---|---|
| Rută | `PUT /api/orders/id`, `requireSession` + verificare explicită a sesiunii; corp ≤ 8192 octeți | `R/modules/localhttp/module.go:251`; `order_identity.go:15-51` [COD] |
| Payload | `{order_id, expected_id, expected_revision, new_id, confirmation, reason}` | `R/modules/core/model/order_identity.go:8-15` [COD] |
| Validare | `new_id` nevid, ≤200, fără caractere de control, diferit de `expected_id`; `reason` ≤1000; `confirmation == "deacord"` exact; actor nevid → altfel `ErrOrderIdentityInvalid` (400) | `R/modules/storage/sqlite/order_identity.go:19-23` [COD] |
| Actor | `auditActor(r)` din sesiunea de pe server (nume complet → username → e-mail); nu din corpul cererii | `R/modules/localhttp/module.go:2637-2644` [COD; test existent `TestIDChangeUsesAuthenticatedActor`, nerulat] |
| Concurență optimistă | `sample_id` curent și `meta.id_revision` trebuie să corespundă valorilor așteptate → altfel 409 | `order_identity.go:40-43` [COD] |
| Coliziune | respinge dacă altă comandă din **aceeași `order_date` + `round_no`** are `sample_id == new_id` sau `id_correction.original_sample_id == new_id` → 409. Runde diferite ale aceleiași zile **nu** se verifică. | `:44-50` [COD] |
| Atomicitate | un singur `tx`: `orders` (ID, `file_id=new_id`, pacient golit, `status='received'`, meta), toate `order_analyses` (`wisemed_sm_id/fsm_id=''`, flaguri `wisemed_*` șterse), `audit_logs` (`event_type='order-id-change'`) | `:26-141` [COD; test existent `TestIDChangeIsAtomicAndRejectsCollision`, nerulat] |
| Resetarea asocierii | șterge din meta cheile `sync_*`, `sample_code_*`, `wisemed_*`, `file_id`; setează `wisemed_match_flag=1`, `wisemed_send_status="pending"`, `id_revision+1`; rezultatele și istoricul lor rămân neatinse | `:68-78,107-113` [COD] |
| Excludere reciprocă | `identityMu` (RWMutex): corecția ia `Lock`; sincronizarea și trimiterea iau `RLock` pe toată durata (`BeginOrderIdentityUse`) → corecția așteaptă operațiunile în curs | `:13-16,24-25`; `resultsync/module.go:203-207,301-305`; `fileimportbase/module.go:592-595` [COD] |
| Copii învechite | `UpsertOrder` cu `ID>0` respinge un `meta.id_revision` diferit; trimiterea reverifică `sample_id`, `ManualFileID`, `id_revision` | `order_identity.go:155-160`; `module.go:921-924`; `fileimportbase/module.go:596-608` [COD] |
| Utilizarea ID-ului nou | `ManualFileID()` este folosit direct ca ID de fișă, fără prefixe/separatori | `resultsync/module.go:376-378`; `fileimportbase/module.go:618-620` [COD; teste existente `TestManualFileIDBypassesAnalyzerNormalization`, `TestSendUsesCorrectedFileID`, nerulate] |
| Ce nu face | nu retrage nimic din WiseMED; nu există apel de ștergere/anulare către API | absența oricărui apel în `order_identity.go` [COD] |

Observație [COD]: backendul PHP local convertește ID-ul cu `intval()` (`apis/wisemed-api/apiv2/implementation/analyzers.php:5-6`, `file_services.php:251`), deci un ID corectat nenumeric devine 0 și este respins; readerul acceptă orice șir ≤200.

---

## 5. Integrarea WiseMED API (`R/modules/wisemedapi`)

### 5.1 Configurare, autentificare

- Setări `modules.wisemed-api`: `cfg_wisemed_protocol/ip/port/path/key`, `unitate_medicala_id`, `tip_de_echipament_id`, `cod_echipament`, `numar_serial_echipament`, `echipament_id`, `api_key_echipament`, `login_token`, `login_user_id`, `reagent_set_name`, `skip_setup_completion`; fallback din `base_url` (`module.go:148-152,1161-1219`). [COD]
- `IsConfigured` = cele 5 chei `cfg_*`; `SetupComplete` adaugă unitate, tip, cod, serie (`:216-242`). [COD]
- **JWT per cerere**, HS256 semnat cu `cfg_wisemed_key`; claims `caller_id="WM-Lab-Reader"`, `caller_type`, `exp=+5 min`, opțional `lt` (login token) și `login_user_id` (`:1465-1493`). Nu există refresh, cache sau rotație; cheia este un secret simetric partajat. [COD]
- `login_token` și `login_user_id` sunt **persistate în `config.yaml`** după login (`:322-327,607`). [COD]
- Client HTTP: timeout total 20 s; mod „legacy” (protocol `http` sau cale ce conține `mp-api`): fără keep-alive, fără compresie, fără proxy (`:1138-1159`). [COD]
- Erori: non-2xx → `message` din JSON, altfel corpul brut, altfel codul (`:1262-1271`). Fără retry, fără backoff, fără circuit breaker. [COD]
- Trace la `verbose_level ≥4` (cerere/răspuns, cu `maskSecrets`), nivel 5: corp răspuns ≤1200 caractere și previzualizare JWT (`:675-709,1340-1396`) — la nivel 5 logurile pot conține date de pacient din `fileforanalyzer`. [COD]
- Șabloanele `config.install.yaml` conțin o valoare pre-completată pentru `cfg_wisemed_key` (de ex. `R/apps/snibe-maglumi-x3/deployments/config.install.yaml`, secțiunea `wisemed-api`); aceeași valoare apare ca `_api_key` în `apis/mp-api/index.php:7,13`. Valoarea nu este reprodusă aici. [COD]

### 5.2 Endpointuri consumate (lista exactă din cod)

| # | Metodă + cale (relativ la `cfg_wisemed_path`) | Apelant | Payload | Răspuns așteptat de reader | Implementare PHP locală |
|---|---|---|---|---|---|
| 1 | `GET /administrative/medicalunits` | `Bootstrap` (`:281`), `BootstrapClient.ListMedicalUnits` (`:1107`) | — | array, sau `{rows\|data\|items\|result:[…]}` doar în `BootstrapClient` | `administrative.php:94` → `{success, rows}`; fără login |
| 2 | `GET /administrative/wmanalyzertypes` | `Bootstrap` (`:285`), `ListEquipmentTypes` (`:1115`) | — | idem | `administrative.php:366` |
| 3 | `PUT /administrative/analyzer` | `ensureEquipmentHTTP` (`:354`), `RegisterEquipment` (`:1123`) | JSON: `cod_echipament, nume_echipament, producator_echipament, tip_analizor, numar_serial_echipament, online, nr_rackuri, pozitii_pe_rack, nume_pe_raport_final, unitate_medicala_id, tip_de_echipament_id, [echipament_id], analize:[{pe_tag, pe_codificare, pe_formatare, pe_tip_rezultat, pe_ponderare_um, pe_um, pe_set_reactivi_buletin, pe_activ, pe_transformare,…}]` (`:733-790`) | obiect cu `echipament_id` (>0 obligatoriu), `api_key_echipament`, `cod_echipament`, … → persistate în config (`:358-399`) | `administrative.php:71` |
| 4 | `PUT /administrative/login` | `Login` (`:312`) | `{username, password, medical_unit_id, device_id, device_name}` | `login_token` (sau `token`/`lt`), `user_id`, nume, `user_type` (`:1516-1545`) | `logins_user.php:2` |
| 5 | `GET /fileforanalyzer/{file_id}/{equipment_id}/?XDEBUG_TRIGGER=debug` | `FetchFileForAnalyzer` (`:402-423`), `OverrideClient` (`:452`) | — | `o_file_id, o_file_date, o_file_code, o_file_code2, o_patient_id, o_patient_name, o_patient_birthday, o_patient_sex, o_specimen_code, o_tests:[{t_tag, t_name, t_code, t_sm_id, t_fsm_id, t_sample_code}]` | `analyzers.php:2-131` |
| 6 | `PATCH /file/services/results/{file_id}?XDEBUG_TRIGGER=debug` | `SaveFileServiceResults` (`:565-599`), `OverrideClient` (`:472`) | form-urlencoded: `[reagent_set_name]`, `srv[i][fsmid]`, `srv[i][result]`, `srv[i][interpretation]`, `srv[i][conclusion]` | orice JSON (nevalidat de reader) | `file_services.php:249-307` → `{success, results:[{fsmid, success, error}]}` |
| 7 | `PATCH /file/services/dosimetry/?XDEBUG_TRIGGER=debug` | `SaveDosimetry` (`:543-563`) — doar BEOSL | form: `dos[i][serial]`, `dos[i][hp_10]`, `dos[i][hp_007]` | `{success, results:[{success, serial, error}]}` | `file_services.php:218-247` |
| 8 | `POST <token_path configurat>` | `WSSAccessToken` (`wss.go:15-51`) | JSON liber | `{token}`; doar 200; fără redirect; răspuns ≤16 KiB | PHP-ul local are `GET /wsm/token` (`wsm_wsmtoken.php:4`) — metodă diferită; ruta efectivă e configurabilă [NEC] |
| 9 | `POST\|PUT <cale callback>` | `SendCallback` (`callback.go:30-74`) | JSON opac; antet `Idempotency-Key`; JWT cu `lt` furnizat pe job | doar codul HTTP | [NEC] — folosit de module utilitare, nu de fluxul de laborator |
| WS | acțiunea `wisemed.fetch_file_for_analyzer` prin `wisemed-ws` (timeout 20 s), încercată **înaintea** HTTP-ului #5 | `fetchFileForAnalyzerViaWS` (`:525-541`) | `{file_id, equipment_id}` | același obiect ca #5 | serverul WSM — în afara acestui raport |

`ensureEquipmentOnlineViaWS` (`:508-523`) este **cod mort** (niciun apelant). `?XDEBUG_TRIGGER=debug` este trimis în producție pe #5–#7. [COD]

Rute PHP existente dar **neconsumate** de reader: `PUT /fileforanalyzer/results/` (`analyzers.php:137`), `PUT /file/mplabresults` (`files.php:266`). [COD]

### 5.3 Bootstrap, înregistrare, login

1. Wizard CLI (`R/apps/runner/bootstrap.go`, `BootstrapClient`) sau UI: `GET /api/wisemed/bootstrap`, `PUT /api/wisemed/setup`. [COD pentru rute; fluxul wizardului necitit în detaliu → [NEC]]
2. La pornire, dacă `SetupComplete`: `EnsureEquipmentInitialized` → `PUT /administrative/analyzer` o singură dată per proces (`initializeMu` + `initialized`), cu tot catalogul de analite; un răspuns fără `echipament_id` valid nu suprascrie configul (`module.go:172-180,332-400`). Eșecul este doar logat; nu se reîncearcă până la următorul login/import. [COD; teste existente `wss_initialization_test.go`, nerulate]
3. Login operator: `POST /api/session/login` → #4 → `EnsureEquipmentOnline` (#3 din nou) → cookie de sesiune local (`R/modules/localhttp/module.go:1106-1171`). [COD]
4. Catalogul de analite se retrimite (#3) ori de câte ori un import creează/modifică un analit (`fileimportbase/module.go:335-340,485-496`; LD-560 `:946`). [COD]

### 5.4 Local vs extern

| Element | Unde |
|---|---|
| Client HTTP, JWT, formarea payloadurilor, persistența setărilor | în repo, Go [COD] |
| Rutele REST și forma răspunsurilor | în repo, PHP (`apis/wisemed-api/apiv2/implementation`, `apis/mp-api/api_implementation.php`) [COD] |
| Validarea JWT pe server, autorizarea (ce echipament poate citi ce fișă), blocarea rezultatelor validate, logica `import_dosimetry_data`, `updateAnalyzer` | obiecte externe (`$ACE_System_obj`, `$thriftServer`, `medicalpages.lab`) — **nu sunt în repo** [NEC] |
| Dacă versiunea PHP din repo este cea rulată în producție | [NEC] |

### 5.5 Conținutul `apis/`

| Director | Conținut |
|---|---|
| `apis/wisemed-api/*.thrift` | 14 IDL-uri Thrift (analyzers, files, patients, …) cu adnotări `//yaml:` din care se generează OpenAPI (`yamlgenerate.sh`, `yaml/*.yaml`, `yaml/final.yaml`) — **specificații** |
| `apis/wisemed-api/gen-php`, `gen-html`, `lib`, `php/` | cod generat Thrift + client PHP (`WiseMEDAPIClient.php`) + helper DB |
| `apis/wisemed-api/apiv2/` | **server REST PHP** (`index.php`, `api_implementation.php`, `implementation/*.php` ~110 rute), `docs/wmapi.json`, colecții Postman, `openapi/generated/` |
| `apis/mp-api/` | adaptor PHP care implementează **același contract de reader** (`/administrative/login`, `medicalunits`, `wmanalyzertypes`, `analyzer`, `/fileforanalyzer/{f}/{a}/`, `PATCH /file/services/results/{f}`, `PATCH /file/services/dosimetry/`) peste aplicația „Medical Pages”; include verificări de blocare a rezultatelor (`mp_api_is_file_results_locked`, `:1012-1075`), `php-jwt`, Postman. Cheie API hard-codată în `index.php` |

Niciun client Go nu importă aceste directoare; legătura este doar contractul HTTP. [COD]

---

## 6. Sincronizare și confirmare (`R/modules/resultsync`, „Synchro Match”, trimitere în buletin)

### 6.1 Componente

| Componentă | Rol | Dovadă |
|---|---|---|
| Buclă periodică | `Start`: o rulare imediată, apoi `Ticker` la `interval_minutes` (impl. 5); activă implicit (`enabled` impl. true) | `resultsync/module.go:90-114,484-506` [COD] |
| `runOnce` | **doar comenzile zilei curente**, toate rundele; nu trimite rezultate | `:202-288` [COD] |
| `RunOrders` / `RunOrdersWithSettings` | sincronizare țintită pe ID-uri + dată + rundă | `:163-173,300-371` [COD] |
| `saveOrderBundlesToWiseMED` | PATCH rezultate + persistarea stării de trimitere | `fileimportbase/module.go:590-695` [COD] |
| `AutoSaveResultsToWiseMED` | sincronizare → reîncărcare → PATCH, dacă `results.auto_confirm_wisemed` | `:498-535` [COD] |
| Manual | `POST /api/orders/send-to-bulletin` (alias `/send-to-wisemed`): sincronizare apoi PATCH, cu `login_token` al operatorului conectat | `R/modules/localhttp/module.go:1802-1941` [COD] |

### 6.2 Normalizarea codului probei (`postprocessSampleCode`, `:526-621`)

1. prefix QC configurat (`result-sync.qc_prefixes`) → `qc`, fără lookup;
2. elimină primul prefix/sufix configurat (`sample_prefixes/suffixes`);
3. doar cifre → `file_id`;
4. altfel împarte după un separator (impl. `-`) în `fișă[-probă[-eșantion]]`, toate părțile numerice; altfel `invalid_code`;
5. un ID corectat manual ocolește totul (`:376-378`). [COD]

Importurile de fișiere aplică o normalizare similară **la recepție** (`NormalizeImportedRecord`, `fileimportbase/module.go:1173-1265`): `sample_id` devine codul normalizat, iar `patient_id`/`patient_name` primesc **codul de probă / eșantion** (`:1188-1189`) — câmpurile sunt reutilizate semantic. [COD]

### 6.3 Mașina de stări

**Potrivire (per comandă, în `orders.meta_json`):**

| `wisemed_match_flag` | Semnificație | `sync_status` posibile |
|---|---|---|
| 1 | neîncercat / resetat după corecția ID | —, `qc`, `invalid_code`, `lookup_error` |
| 2 | încercat, nicio analiză potrivită | `matched` sau `file_found` |
| 3 | parțial | `matched` / `file_found` |
| 4 | complet (analize potrivite ≥ analize cu rezultat) → **comanda este sărită la toate sincronizările ulterioare** | `matched` / `file_found` |

Tranziții: `processOrder` (`:373-443`), `deriveMatchFlag` (`:878-889`), `orderHasCompleteWiseMEDMatch` (`:891-893`). Per analiză: `flags.wisemed_match_flag` ∈ {2,3,4}, `wisemed_match_attempted[_at]` (`:792-829`). O potrivire scrie `wisemed_sm_id`/`wisemed_fsm_id`, numele analizei și datele pacientului/fișei (`:720-790`). [COD]

Răspunsul complet WiseMED este salvat în `orders.meta_json.sync_lookup` (`:422`), inclusiv nume, dată de naștere, sex — date de pacient în SQLite-ul local. [COD]

**Trimitere:**

| Nivel | Câmp | Valori | Setat de |
|---|---|---|---|
| analiză | `flags.wisemed_send_status` | `pending` → `sent` | `persistWiseMEDSendStates` (`fileimportbase/module.go:706-736`), `CorrectOrderID` |
| analiză | `wisemed_sent_at`, `wisemed_send_count` | timestamp, contor | idem |
| comandă | `meta.wisemed_send_status` | `pending` (după corecție), `none`, `partial`, `full` | `:737-758` |
| comandă | `wisemed_sent_analyses`, `wisemed_total_sendable_analyses`, `wisemed_send_count`, `wisemed_sent_at` | — | idem |

Nu există stare „în curs”, „eșuat” sau „necunoscut”; un eșec lasă starea anterioară neschimbată. [COD]

### 6.4 Ce se trimite

- ID fișă: `ManualFileID()` → `orders.file_id` → `meta.file_id`; lipsă → `skipped/missing_file_id` (`:617-633`).
- Doar analizele cu `wisemed_fsm_id` nevid și valoare; valoarea trimisă este **`raw_value`**, cu fallback `result_value` (`:636-655`); `interpretation` și `conclusion` sunt mereu goale.
- Un PATCH per comandă, cu **toate** analizele eligibile, indiferent dacă au mai fost trimise (`:634-669`).
- Fără `Idempotency-Key`, fără identificator de cerere; reluarea este sigură doar pentru că serverul suprascrie după `fsmid` (PHP local: `file_services.php:271-284`; comportamentul din producție [NEC]).
- Filtrul zilnic `daily_analysis_send_filters` **nu** se aplică aici (doar comenzilor către analizor). [COD]

### 6.5 Erori, timeout, retry, idempotency

| Situație | Comportament | Dovadă |
|---|---|---|
| Sincronizare deja în curs | `result sync already running` → eroare; trimiterea automată se abandonează ca „warning”, **fără reluare** | `resultsync/module.go:209-213,307-311`; `fileimportbase/module.go:341-344` [COD] |
| Lookup eșuat pentru o comandă | `sync_status=lookup_error` în meta; **nu** produce eroare Go; `AutoSaveResultsToWiseMED` ignoră sumarul și continuă → analizele fără `fsm_id` sunt omise tăcut | `resultsync/module.go:415-421`; `fileimportbase/module.go:519-521` [COD] |
| Timeout / eroare la PATCH | funcția se oprește la prima eroare; comenzile anterioare rămân `sent`, cea curentă și următoarele rămân nemarcate; dacă serverul a aplicat totuși modificarea, starea locală rămâne „netrimis” | `:669-677` [COD]; divergența [INF] |
| Retry automat / coadă persistentă | **nu există**; bucla periodică nu face PATCH | `resultsync/module.go:202-288` [COD] |
| Repornire | nimic nu reia trimiterile neterminate; doar operatorul, manual | [COD — absență] |
| Trimitere manuală | 502 la eșecul sincronizării sau al PATCH-ului; ambele rezultate ajung în `audit_logs` (`results-send-bulletin`) | `R/modules/localhttp/module.go:1874-1940` [COD] |
| Comenzi din alte zile | bucla periodică nu le atinge; se pot sincroniza/trimite doar manual, cu `order_date` explicit | `resultsync/module.go:236` [COD] |
| **Rezultat re-recepționat pentru o comandă deja potrivită complet** | analiza își pierde `wisemed_fsm_id` (§3.4), dar comanda păstrează `wisemed_match_flag=4` (meta se îmbină, `module.go:933`) → toate sincronizările o sar → trimiterea omite analiza (`no_entries` sau trimitere parțială). Ieșire: corecția ID-ului (resetează flagul) sau ștergerea comenzii. | lanț: `storage/sqlite/module.go:1089-1090,1399-1413` → `resultsync/module.go:267-270,351-354` → `fileimportbase/module.go:637-640,656-667` [COD pe fiecare verigă]; efectul cap-coadă [INF], neverificat prin rulare |

### 6.6 Automat vs manual; acoperire per protocol

| Traseu | Protocoale | Dovadă |
|---|---|---|
| Automat (`AutoSaveResultsToWiseMED`) | toate cele bazate pe `fileimportbase` (Seegene, CFX96, BEOSL, Analytik Jena, Anatolia, Biosan, GammaVision, TriCarb, Shimadzu Generic), `cary60uvvis` (`:243`), `shimatzutocl` (`:242`), `labnovationld560` (`:905`) | [COD] |
| Doar manual | ASTM (5 aplicații), Pentra, miniVIDAS, LAURA, Zonci — recepția se termină după salvare + transformări | `astm/module.go:845-857`; `horibaabxpentra400/module.go:548-553`; `biomerieuxminividas/module.go:223-231`; `erbamannheimlaura/module.go:303-311`; `zoncixl1000i/module.go:397-403` [COD] |
| Fără import | `generic-test-reader`, IR Biotyper | [COD] |
| BEOSL | două căi la același import: `AfterImport` → `PATCH /file/services/dosimetry/` **și** fluxul generic sync + `PATCH results`; plus serviciul `result-publisher` pentru retrimitere manuală | `beoslcsv/module.go:44,53,66-98,216-244`; `fileimportbase/module.go:329-344` [COD] |

### 6.7 Ce nu se retrage din WiseMED

Nu există niciun apel de anulare/ștergere a rezultatelor. Ștergerea locală a unei comenzi, corecția ID-ului, schimbarea rezultatului implicit sau reaplicarea transformărilor **nu** modifică ce s-a trimis deja; noua valoare ajunge în WiseMED doar la o nouă trimitere reușită, care suprascrie. [COD — absența oricărei metode `DELETE` în `R/modules/wisemedapi`]

---

## 7. QC, Westgard, statistici, dashboard

### 7.1 Date

- Clasificarea ca QC se face **în protocol**, la recepție (prefix ID la ASTM; indicator la LD-560; tipul rândului la fișiere) → `qc_records` + `qc_analyses`. `result-sync` are propria listă `qc_prefixes`, folosită doar pentru a nu căuta în WiseMED comenzi care au ajuns totuși în `orders` (§6.2). Cele două liste sunt independente. [COD]
- `numeric_value` extras din `raw_value`/`result_value` (`parseNumericQCValue`, `module.go:2202`); `created_at` = `flags.measured_at` dacă există (`:1577-1580`). [COD]
- QC manual: `SaveManualQCRecord` cu `manual_entered_by/at` (`:767-830`), prin `POST /api/qc-records`. [COD]
- Ținte: `qc_targets` (medie, SD, CV, lot, expirare); importurile creează automat ținte cu valori 0, de completat de operator (`fileimportbase/module.go:1036-1047`). [COD]
- Rundele QC sunt dezactivate: `ListQCRoundNumbers` întoarce mereu `[1]` (`module.go:1634-1640`). [COD]

### 7.2 Reguli Westgard implementate (`classifyWestgard`, `module.go:2313-2378`)

z = (valoare − `target_mean`) / `target_sd`, pe seria ordonată cronologic a unui analit (filtre opționale nivel/lot/interval; ultimele 500 de puncte).

| Regulă | Condiție în cod |
|---|---|
| `1_2s` | \|z\| > 2 |
| `1_3s` | \|z\| > 3 |
| `2_2s` | două puncte consecutive, aceeași parte, ambele \|z\| > 2 |
| `R_4s` | \|z_i − z_{i−1}\| > 4 |
| `4_1s` | patru consecutive, aceeași parte, toate \|z\| > 1 |
| `10x` | zece consecutive de aceeași parte a mediei |
| `7T` | șapte consecutive strict monotone |

Limitări [COD]: regulile se evaluează **într-o singură serie** (nu între niveluri de control din aceeași rulare); `R_4s` compară puncte consecutive, nu două controale ale aceleiași rulări; dacă `target_sd <= 0` nu se evaluează nimic; toate regulile sunt raportate ca „probleme de validare”, fără distincție avertizare/respingere (`validateWestgardDataset`, `:2380-2397`); rezultatul este **doar informativ** — nu blochează recepția, sincronizarea sau trimiterea rezultatelor pacienților. Nu există teste pentru `classifyWestgard`.

Join-ul cu ținta este pe `analyte_tag + lot_no` (fără nivel) (`:1784`); dacă există mai multe ținte active pentru același lot, ultima citită câștigă (`:1821-1829`). [COD]

### 7.3 Statistici și dashboard

- `QCPerformance` întoarce: `count, mean, sd, cv, median, min, max, outliers, repeatability` (media diferențelor absolute consecutive), `target_*`, `robust_mean` (identic cu media — ramura `>10` nu schimbă nimic, `:1856-1859`), `points`, `westgard`, `is_valid`, `validation_issues` (`:1772-1888`). [COD]
- `DashboardSnapshot` (`:1642-1718`): analize cu/fără rezultat azi; QC azi (total, numerice, în afara 2SD/3SD față de țintă); serie pe ultimele 14 zile (comenzi, analize, analize cu rezultat). Folosește data locală a mașinii. [COD]
- Modulul `stats` nu are logică; **nu există** rute `/api/stats` sau `/api/stats/series` în v3. [COD]

### 7.4 Rute

`GET/POST /api/qc-records`, `POST /api/qc-records/delete`, `GET/POST /api/qc-targets`, `…/qc-targets/{id}`, `GET /api/qc/metrics?analyte_tag=&control_level=&lot_no=&date_from=&date_to=`, `GET /api/dashboard` — toate cu sesiune (vezi §9).

---

## 8. Evenimente și loguri

| Canal | Persistență | Producători | Consumatori |
|---|---|---|---|
| `audit_logs` (SQLite) | permanentă, fără retenție | `AppendAuditLog`: acțiuni UI (`orders-import`, `orders-delete`, `result-default-change`, `result-sync-run-orders`, `results-send-bulletin`), protocoale (`astm-import`, `horiba_*`, Zonci, LD-560, `transport-file`), `order-id-change` | `GET /api/logs` (polling din UI) |
| Jurnal UI | calculat (`UNION` rezultate + QC + audit, ultimele N, impl. 40) | — | `GET /api/logs` (`R/modules/localhttp/module.go:1432-1458`) |
| Log text zilnic | `<bază>-YYYYMMDD.log`; săptămânile încheiate sunt arhivate ZIP și fișierele originale șterse | `rt.Logf` din toate modulele | fișier; consolă cu `-showlog` (`R/apps/runner/logging.go:18-56,82-199`) |
| `analyzer-activity` | memorie | protocoale | `/api/status` |
| WSS (`wisemed-ws`) | — | comenzi `reader.status`, `ws.status`, `device.ping`, `ws.reconnect`, `debug.message`, `api.request` (punte către rutele HTTP locale) | serverul WSM (`R/modules/ws/module.go:503-563`) |

Nu există topic `results:<reader_id>`, eveniment `result_available`, `logs.activate` sau tabel `event_logs` în v3 (vezi Contradicții). [COD]

---

## 9. Rute HTTP înregistrate de modulele de laborator

Serverul: `R/modules/localhttp/module.go` (`http.Server` peste mux-ul comun, implicit `127.0.0.1:18080`, HTTP simplu; TLS opțional). **Nu există middleware global de autentificare**: fiecare rută este protejată doar dacă este înfășurată explicit cu `requireSession` (`:568-577`). Sesiunea = cookie HMAC sau principal WSM (`currentSession`, `:2566-2593`). Pentru readerele standard, cheia HMAC a cookie-ului este derivată din `ReaderID` (`sessionSecret`, `:2660-2665`) — nu este un secret aleator. [COD]

### 9.1 Rute cu logică (`localhttp/module.go`)

| Metodă | Rută | Handler (linie) | Auth |
|---|---|---|---|
| GET | `/api/session` | `handleSessionStatus` (:235/:626) | nu |
| POST | `/api/session/login` | `handleLogin` (:241/:1106) | nu (creează sesiunea prin WiseMED) |
| POST | `/api/session/logout` | `handleLogout` (:242/:1252) | sesiune |
| GET, PUT | `/api/wisemed/setup` | `handleWiseMEDSetup` (:239/:1172) | **nu** pentru readere (sesiune doar la utilitarele login-only deja configurate) |
| GET | `/api/wisemed/bootstrap` | `handleWiseMEDBootstrap` (:240/:1233) | **nu** |
| GET/PUT | `/api/reader-settings` | `handleReaderSettings` (:238/:699) | sesiune |
| GET | `/api/status` | `handleStatus` (:246/:1290) | sesiune |
| GET | `/api/dashboard` | `handleDashboard` (:247/:1376) | sesiune |
| GET | `/api/logs?limit=` | `handleLogs` (:248/:1432) | sesiune |
| GET, POST | `/api/analytes` | `handleAnalytes` (:249/:1459) | sesiune |
| GET, PUT, DELETE | `/api/analytes/{id}` | `handleAnalyteByID` (:250/:1490) | sesiune |
| PUT | `/api/orders/id` | `handleOrderIDChange` (:251; `order_identity.go:15`) | sesiune |
| GET | `/api/orders?order_date=&round_no=&search=` | `handleOrders` (:252/:1535) | sesiune |
| GET | `/api/order-image` | `handleOrderImage` (:253/:1670) | sesiune |
| GET | `/api/orders/worklist` | `handleOrdersWorklist` (:254/:1966) — doar protocolul `cary60-uvvis` | sesiune |
| GET, POST | `/api/orders/rounds` | `handleOrderRounds` (:255/:1618) | sesiune |
| POST | `/api/orders/import` (multipart) | `handleOrdersImport` (:256/:1689) | sesiune |
| POST | `/api/orders/export` | `handleOrdersExport` (:257/:1732) | sesiune |
| POST, DELETE | `/api/orders/delete` | `handleOrdersDelete` (:258/:1736) | sesiune |
| POST | `/api/orders/reapply-transformations` | `handleOrdersReapplyTransformations` (:259/:1771) | sesiune |
| POST | `/api/orders/send-to-wisemed`, `/api/orders/send-to-bulletin` | `handleOrdersSendToBulletin` (:260-261/:1806) | sesiune |
| GET, POST | `/api/daily-details/definitions`, `…/{id}` | (:262-263/:2205,:2237) | sesiune |
| GET, PUT | `/api/daily-details` | `handleDailyDetails` (:264/:2008) | sesiune |
| GET | `/api/daily-details/worksheet` | (:265/:2147) | sesiune |
| GET, POST, DELETE | `/api/daily-analysis-filters`, `…/{id}` | (:266-267/:2045,:2079) | sesiune |
| GET, POST | `/api/qc-records` | `handleQCRecords` (:268/:2276) | sesiune |
| POST | `/api/qc-records/delete` | (:269/:2348) | sesiune |
| GET, POST | `/api/qc-targets`; GET/PUT/DELETE `…/{id}` | (:270-271/:2388,:2415) | sesiune |
| GET | `/api/qc/metrics` | `handleQCMetrics` (:272/:2456) | sesiune |
| PUT | `/api/results/default` | `handleDefaultResult` (:273/:2486) | sesiune |
| GET | `/api/result-sync/status` | (:274/:984) | sesiune |
| GET, PUT | `/api/result-sync/settings` | (:275/:999) | sesiune |
| POST | `/api/result-sync/run` | `handleResultSyncRun` (:276/:1041) | sesiune |
| POST | `/api/result-sync/orders` | `handleResultSyncOrders` (:277/:1059) | sesiune |
| POST | `/api/result-sync/reset` | (:278/:1092) | sesiune |
| GET | `/api/debug/tests` | `handleDebugTests` (:279; `debug_replay.go:36`) | sesiune + `UserType <= 0` |
| POST | `/api/debug/tests/run` | `handleDebugRun` (:280; `debug_replay.go:55`) | sesiune + `UserType <= 0` |

Metodele pentru rutele cu „(:linie)” fără handler citit integral sunt deduse din nume și din UI → [INF] pentru metoda exactă; înregistrarea și protecția sunt [COD]. Nu există roluri/permisiuni per rută în afara verificării de debug: orice utilizator autentificat poate șterge comenzi, corecta ID-uri și trimite în buletin. [COD]

### 9.2 Rute înregistrate direct de alte module (toate **fără autentificare**)

| Metodă | Rută | Modul : linie | Observație |
|---|---|---|---|
| GET | `/healthz`, `/api/menu`, `/api/modules` | `R/core/runtime/runtime.go:50-52` | |
| GET | `/api/storage/meta` | `storage/sqlite/module.go:75` | |
| GET | `/api/events/meta` | `events/module.go:21` | |
| GET | `/api/analytes/meta`, `/api/analytes/management/meta` | `analytes/module.go:24`, `analytemanagement/module.go:31` | pattern exact, ocolește `requireSession` de pe `/api/analytes/` |
| GET | `/api/orders/meta`, `/api/dashboard/meta`, `/api/qc/meta`, `/api/stats/meta`, `/api/login/meta`, `/api/daily-details/meta` | `dailyorders:18`, `dashboard:20`, `qc:29`, `stats:21`, `login:16`, `dailydetails:30` | |
| GET | `/api/wisemed/meta` | `wisemedapi/module.go:154` | |
| GET, **PUT** | `/api/settings/astm-specimens` | `astmspecimen/module.go:24,33-63` | **PUT neautentificat rescrie `config.yaml`** (`protocol-astm.specimen_code_*`) |
| GET | `/api/protocol/meta` | fiecare protocol (ex. `astm/module.go:166`, `fileimportbase/module.go:181`) | |
| GET | `/api/protocol/astm/status`, `/settings/protocol/astm` | `astm/module.go:167-168` | expune host/port |
| GET | `/api/protocol/horiba-abx-pentra400/query-status`, `/protocol/horiba-abx-pentra400/query` | `horibaabxpentra400/module.go:90-91` | starea interogărilor per ID de probă |
| GET, **PUT** | `/api/protocol/labnovation-ld560/settings` | `labnovationld560/module.go:80,298` | **PUT neautentificat** schimbă modul/host/port și repornește comunicația |
| GET | `/api/protocol/labnovation-ld560/status`, `/settings/protocol/labnovation-ld560` | `:79,81` | |
| GET | `/help`, `/help/*` | `help/module.go:28-41` | fișiere statice |

Aceleași rute `/api/*` sunt accesibile și prin WSM `api.request` (`R/shared/apibridge/bridge.go:51-63`), unde identitatea vine din principalul WSM; detaliile autorizării WSM nu țin de acest raport. [COD parțial]

---

## 10. Concurență, backpressure, recuperare, retenție

### 10.1 Goroutine și sincronizare

| Componentă | Goroutine | Sincronizare | Backpressure |
|---|---|---|---|
| Runtime | una per modul `Starter`; **prima care iese din `Start` oprește tot procesul** (`R/core/runtime/runtime.go:124-148`) | `WaitGroup`, `errCh` | — |
| Import fișiere | o goroutine (ticker); importul manual rulează pe goroutine-a HTTP | `mu` + `running` per cale | scanare secvențială; un import lent întârzie următoarele; ticker-ul Go nu acumulează tick-uri |
| ASTM | `Start` + una per conexiune + una per host query (`:648`) | `mu` pentru status; canal `queryReplyCh` (buffer 8) per conexiune | conexiuni nelimitate; dacă bufferul de 8 e plin, goroutine-ele de query se blochează până la `ctx.Done` |
| LD-560 | `Start` + una per conexiune; `restartCh` (buffer 1) | `mu` | `readSimpleMessage` acumulează fără limită până la `</TRANSMIT>` (`parser.go:216-230`) |
| LAURA | una per conexiune TCP + watcher de închidere | — | cadru limitat la 64 KiB (`:102-105`) |
| Zonci | una per conexiune | `m.mu` serializează toate cadrele (inclusiv lookup-ul WiseMED) | — |
| Pentra, miniVIDAS | o singură goroutine serială | Pentra: `RWMutex` pentru status | lookup WiseMED sincron în bucla de citire |
| `result-sync` | buclă cu ticker | `mu` + flag `running` (o singură rulare; a doua primește eroare, nu așteaptă) | — |
| Storage | `database/sql` cu pool implicit (nelimitat) pe SQLite WAL; `busy_timeout` 5 s | `identityMu` (RWMutex) doar pentru identitatea comenzii | scriitori concurenți → `SQLITE_BUSY` după 5 s [INF] |
| `wisemedapi` | — | `mu` (setări), `initializeMu` (înregistrare) | timeout 20 s per cerere |

Risc [INF]: `identityMu` este un `sync.RWMutex` ținut în `RLock` pe durata apelurilor HTTP către WiseMED (până la zeci de secunde per comandă). O corecție de ID în așteptare (`Lock`) blochează, prin semantica RWMutex din Go, toți cititorii noi (sincronizare, trimitere) până se termină cititorii în curs.

### 10.2 Recuperare după repornire

| Situație | Comportament | Etichetă |
|---|---|---|
| Fișier în inbox neprocesat sau procesat pe jumătate la oprire | rămâne în inbox și este reimportat integral la pornire → rânduri duplicate în `order_analysis_results`, analize suprascrise (cu efectul din §3.4/§6.5) | [COD] pentru mecanism; [INF] pentru duplicate |
| Fișier parțial scris de analizor | poate fi parsat incomplet și mutat în `processed`/`failed`; restul nu mai este citit | [INF] |
| Lot ASTM / pachet serial în memorie la oprire | pierdut (analizorul a primit deja ACK pe cadre); nu există jurnal de recepție pe disc | [COD] |
| Rezultate salvate dar netrimise | rămân în SQLite; nu sunt reluate automat (bucla periodică doar potrivește, și doar pentru ziua curentă) | [COD] |
| Trimitere întreruptă | starea `sent` există doar pentru comenzile confirmate înainte de întrerupere | [COD] |
| Sesiuni operator | cookie semnat stateless → rămân valide după repornire la readerele standard; `login_token`-ul WiseMED al sesiunii se ține în memorie (`sessionAuth`), deci după repornire trimiterea manuală folosește tokenul din `config.yaml` | `R/modules/localhttp/module.go:2595-2610`, `:1858-1867` [COD]; efectul [INF] |
| SQLite | WAL + `synchronous=NORMAL`: tranzacțiile comise pot fi pierdute la pană de curent, fără corupere | [INF — semantica SQLite] |

### 10.3 Păstrarea și ștergerea datelor

- **Nu există retenție automată** pentru `orders`, rezultate, QC sau `audit_logs` (niciun `delete` programat, niciun `vacuum`) → baza crește nelimitat. [COD]
- Ștergeri doar la cererea operatorului: `/api/orders/delete` (șterge și rezultatele, și runda goală, auditat), `/api/qc-records/delete`, analite, ținte, filtre. [COD]
- Fișiere procesate/eșuate: păstrate nelimitat în `processed/` și `failed/`. Imaginile LD-560 se salvează pe disc (`labnovationld560/module.go:1214`). `.tmp-imports/` nu este curățat (`R/modules/localhttp/module.go:1706-1722`). [COD]
- Loguri text: arhivare săptămânală ZIP, fără ștergerea arhivelor. [COD]
- Date personale locale: nume/ID pacient în `orders`, răspunsul WiseMED complet în `meta_json.sync_lookup`, valori în loguri la nivel 1 (`astm rezultat primit sample_id=… patient_id=…`, `astm/module.go:751`) și pachetul ASTM integral (`PARSED ASTM PACKAGE`, `:732`). SQLite-ul nu este criptat. [COD]

---

## 11. Matrice de comunicații

### 11.1 Analizor ↔ reader

| Sursă → destinație | Transport | Comandă / mesaj | Autentificare | Autorizare | Payload | Răspuns | Persistență | Timeout | Retry / idempotency | Erori |
|---|---|---|---|---|---|---|---|---|---|---|
| Software analizor → reader | Fișier în `import_dir` | fișier nou care corespunde `pattern` | niciuna (drepturi OS) | niciuna | CSV/TXT/XLS/XLSX | mutare în `processed`/`failed` | `orders`, `order_analyses`, `order_analysis_results`, `qc_*`, `analytes` | poll 2 s | reimport la fiecare apariție; fără dedup | `failed_dir` + log |
| Analizor → reader (ASTM) | TCP (server impl.) | ENQ / cadre / EOT | niciuna | niciuna | H,P,O,R,L | ACK per ENQ și per cadru | idem | citire 100 ms; lot închis la EOT sau la liniște | reader nu cere retransmisie; fiecare retransmitere a analizorului = rânduri noi | `LastError` + log; fără NAK |
| Analizor → reader (ASTM query) | TCP | `Q\|1\|^<id>` + EOT | niciuna | niciuna | ID probă | ENQ → (ACK) → H,P,O,L → EOT | comandă `scheduled` + analize (dacă vine din WiseMED) | lookup 20 s WS + 20 s HTTP; fără timeout pe ACK | fără retransmisie la NAK | răspuns gol |
| Pentra ↔ reader | RS-232 | ASTM rezultate + host query | niciuna | niciuna | idem; `O` per tip de probă | ACK; comenzi cu checksum + CRLF | idem + `analyzer_send_*` | citire 500 ms | idem | audit `horiba_*`; redeschidere port la 3 s |
| miniVIDAS → reader | RS-232 | ENQ / text+ETX / EOT | niciuna | niciuna | câmpuri `ci,rt,rn,ql,qn,td,tt` | ACK | `orders…` | 500 ms | — | „packet rejected” în log |
| LAURA → reader | RS-232 sau TCP | STX raport ETX | niciuna | niciuna | panou fix 10 analite | **niciun răspuns** | `RecordImportedPanel` (atomic) | 500 ms (serial) | panou repetat = `panel_id` nou | raport incomplet/corupt respins integral |
| Zonci ↔ reader | TCP server sau RS-232 | `1` query → `2` comandă; `3` rezultate; `4` QC | niciuna | niciuna | text cu separator configurabil | doar la `1` | `orders…`; QC nepersistat | 500 ms (serial); fără deadline TCP | — | log + audit |
| LD-560 → reader | TCP | `<TRANSMIT>` sau HL7 (MLLP) | niciuna | niciuna | rezultate HbA1c, opțional imagine | **niciun ACK** | `orders…`, `qc_*`, fișier imagine | fără deadline | — | log; conexiunea continuă |

### 11.2 Reader ↔ WiseMED API

| Sursă → destinație | Transport | Endpoint | Autentificare | Autorizare | Payload | Răspuns | Persistență | Timeout | Retry / idempotency | Erori |
|---|---|---|---|---|---|---|---|---|---|---|
| Reader → WiseMED | HTTPS (sau HTTP „legacy”) | `GET /administrative/medicalunits`, `/wmanalyzertypes` | JWT HS256 (`cfg_wisemed_key`), 5 min | server: permise fără login (PHP local) | — | liste | nimic | 20 s | fără; citire idempotentă | 502 către UI |
| Reader → WiseMED | idem | `PUT /administrative/analyzer` | JWT | server [NEC] | echipament + catalog analite | `echipament_id`, `api_key_echipament`,… | `config.yaml` | 20 s | la pornire, la fiecare login și la schimbări de catalog; upsert după `echipament_id` | log; `initialized` rămâne false |
| Reader → WiseMED | idem | `PUT /administrative/login` | JWT + utilizator/parolă în corp | server | credențiale + device | `login_token`, profil | `config.yaml` (`login_token`, `login_user_id`) + sesiune locală | 20 s | fără | 401 către UI |
| Reader → WiseMED | WSS `wisemed.fetch_file_for_analyzer`, apoi HTTP `GET /fileforanalyzer/{f}/{e}/` | — | identitatea WSS / JWT (+`lt` dacă există) | server: fișa × echipament [NEC] | — | fișă + teste (`t_tag`, `t_fsm_id`,…) | `orders.meta_json.sync_*`, `order_analyses.wisemed_*` | 20 s + 20 s | periodic la 5 min pentru comenzile zilei cu flag < 4; idempotent | `sync_status=lookup_error` |
| Reader → WiseMED | HTTPS | `PATCH /file/services/results/{f}` | JWT (+`lt` al operatorului la trimiterea manuală) | server [NEC] | `srv[i][fsmid|result|interpretation|conclusion]` | JSON nevalidat | flaguri `wisemed_send_*` | 20 s | **fără retry, fără cheie de idempotency**; retrimiterea suprascrie | 502 către UI / „warning” în log |
| Reader → WiseMED | HTTPS | `PATCH /file/services/dosimetry/` | JWT | server [NEC] | `dos[i][serial|hp_10|hp_007]` | `{success, results}` | nimic local | 20 s | fără | „after-import warning” |
| Reader → WiseMED | HTTPS | `POST <token_path>` | JWT | server | JSON | `{token}` | memorie | context apelant | fără; fără redirect | eroare generică |

Sensul invers (WiseMED → reader) nu folosește HTTP: comenzile vin prin conexiunea WSS inițiată de reader (`api.request`). Nu există webhook-uri primite de reader în fluxul de laborator. [COD]

---

## Contradicții cod ↔ documentație / memorie

| # | Sursă | Afirmație | Ce arată codul |
|---|---|---|---|
| C1 | `R/README.md` („transporturile către analizor: fișier, serial, TCP/IP… decuplate”) | transporturile sunt module funcționale | serial/TCP sunt stuburi; I/O-ul este duplicat în fiecare protocol; ASTM generic nu suportă serial deloc (§0 S1, §1.3) |
| C2 | `R/README.md` („event/log bus pentru UI și diagnoză”) | există un bus | modulul `events` are 26 de linii, fără publicare/abonare; UI face polling (§0 S3) |
| C3 | `aimemory/wisemed-labreaders.md` (secțiunile despre `generic-test-reader`) | căi `Readers Last/…`, tabel `event_logs`, comenzi WS `orders.list`/`results.activate`, eveniment `result_available`, rute `/api/stats`, `/api/order-analysis`, `/api/config`, „file fully operational” | descrie generația anterioară (în afara `readersv3`); nimic din acestea nu există în v3; `generic-test-reader` v3 nu importă nimic |
| C4 | `R/README.md` (protocoale: „generic file, Seegene Excel, … IR Biotyper”) | sunt protocoale utilizabile | `genericfile` și `irbiotyper` sunt stuburi; „Seegene Excel” parsează CSV |
| C5 | `R/docs/audit-trimitere-automata-buletin-2026-09-16.md` („O nouă recepție poate retransmite rezultate existente”) | re-recepția duce la retransmitere | analizele re-recepționate își pierd `wisemed_fsm_id` și, dacă comanda era potrivită complet, nu mai pot fi resincronizate → **nu se mai trimit**; se retrimit doar analizele neatinse ale aceleiași comenzi (§3.4, §6.5). Restul auditului (10 fără flux automat / 12 cu) este în continuare valabil pe HEAD-ul curent |
| C6 | `R/README.md` (corecția ID: coliziuni „în aceeași zi/rundă”) | formulare ambiguă | verificarea este strict pe aceeași dată **și** aceeași rundă; același ID în altă rundă a aceleiași zile este acceptat (§4) |
| C7 | `R/README.md` („politica de repeat per analizor”, AGENTS.md „analyzer-specific repeat behavior”) | comportament de repetare per analizor la recepție | `repeat_mode` influențează doar selecția manuală a rezultatului implicit; la recepție ultimul rezultat devine mereu implicit, pentru toate analizoarele (§3.4) |
| C8 | Șabloane `config.install.yaml` vs cod | miniVIDAS: `baud: 9600` | implicitul din cod este 38400 (`biomerieuxminividas/module.go:294`); fără conflict cât timp cheia există în config |
| C9 | `R/modules/wisemedapi/module.go:715-731` vs `apis/wisemed-api/apiv2/implementation/administrative.php:140` | — | `Module.Bootstrap` așteaptă un array brut; PHP-ul local întoarce `{success, rows}` → `GET /api/wisemed/bootstrap` ar întoarce liste goale față de acest backend. `BootstrapClient` (wizardul CLI) tratează corect învelișul. Dacă producția întoarce array brut: [NEC] |
| C10 | `R/modules/wisemedapi/wss.go:15` vs `apis/wisemed-api/apiv2/implementation/wsm_wsmtoken.php:4` | — | readerul face `POST <token_path>`; PHP-ul local expune `GET /wsm/token` |
| C11 | Enunțul sarcinii („backendul WiseMED nu e în repo”) | — | stratul REST PHP este în `apis/`; lipsește doar logica de business (§5.4) |
| C12 | `frame_timeout_ms: 2000` în toate șabloanele ASTM | timeout de cadru 2 s | valoarea este plafonată la 100 ms și folosită doar ca deadline de citire (`astm/module.go:565-568`) |

---

## Necunoscute / zone neinvestigate

1. **Niciun test rulat** (restricție). Nicio afirmație nu este confirmată prin execuție; în special lanțul din §6.5 (pierderea `wisemed_fsm_id` la re-recepție) trebuie reprodus printr-un test.
2. **Hardware real**: nicio captură de pe analizoare în afara celor două fișiere LAURA; conformitatea ASTM (checksum, secvență, contenție) cu fiecare analizor este [NEC]. Parametrii seriali reali (LAURA 9600/8N1, miniVIDAS, Pentra) sunt neconfirmați.
3. **Backendul WiseMED real**: validarea JWT, autorizarea fișă × echipament, blocarea rezultatelor validate, comportamentul la `PATCH` repetat, forma exactă a răspunsurilor în producție, dacă `apis/` reflectă versiunea rulată.
4. Parserele citite doar structural (nu linie cu linie): `cary60uvvis` (1124 linii), `cfx96quantitation` (803), `shimatzutocl` (1040), `seegeneexcel`, `analytikjenaplasmaquantmselite`, `shimatzugeneric`, HL7 din `labnovationld560/parser.go:231-330`. Regulile lor exacte de ID/QC din tabelul §2.1 provin din semnături și fragmente.
5. `R/modules/storage/sqlite/transformations.go` (motorul de formule, 1049 linii) — citit doar fluxul de persistență, nu semantica formulelor.
6. `R/modules/localhttp/module.go`: citite înregistrarea rutelor, autentificarea și handler-ele de sincronizare/trimitere/ID; necitite integral: `handleOrders`, `handleQCRecords` (POST), `handleDashboard`, `handleStatus`, `handleReaderSettings`, export CSV, tipărire, CORS (`currentCORSAllowedOrigins`, `:464`). Metodele HTTP exacte ale unor rute din §9.1 sunt deduse.
7. `R/modules/ws/module.go` (WSS), `R/shared/apibridge` (autorizarea `api.request`), `R/apps/runner/bootstrap.go` (wizard) — doar consultate tangențial.
8. `R/modules/dailydetails/module.go` (225 linii) și UI-ul (`app.js`, ~7000 linii) — necitite.
9. Documentația per analizor: citite `R/apps/erba-mannheim-laura-reader/{README.md,docs/protocol.md}`; **necitite**: sursele Delphi `.pas` și PDF/DOCX din `analyzers-comm-protocols/` și `R/apps/labnovation-ld-560/docs/` (nu am comparat implementările Go cu ele).
10. Configurațiile reale ale clienților (`output/*/deployments/config.yaml`) și bazele de date — neatinse intenționat; valorile din raport sunt din șabloanele `config.install.yaml`.
11. Valorile implicite exacte din `horibaabxpentra400.readConfig` (`:723`) și host/port implicit LD-560.
12. Comportamentul `go.bug.st/serial` la deconectarea fizică a cablului (eroare vs citiri goale) — determină dacă bucla de reconectare pornește efectiv.
