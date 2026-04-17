# Generic Test Reader

Reader generic nou pentru `WiseMedWS`, cu bootstrap prin WiseMed API si stocare locala in SQLite.

## Ce face

- bootstrap interactiv al configului la prima pornire
- apeleaza WiseMed API pentru lista de unitati medicale
- salveaza config YAML persistent
- salveaza datele locale in SQLite
- expune control si administrare prin WebSocket
- suporta configurare pentru:
  - `file`
  - `serial`
  - `network`
- `file` este operational in aceasta versiune
- `serial` si `network` sunt configurabile si raportate prin WS, dar runtime-ul lor este lasat pentru implementari specifice de analizor

## Run

```bash
cd "/Users/raduichim/work/gowork/wisemed-labreaders/Readers Last/generic-test-reader"
GO111MODULE=on GOPROXY=off GOSUMDB=off GOCACHE=/tmp/gocache go run ./cmd/generic-test-reader -config deployments/config.yaml
```

Reconfigurare:

```bash
GO111MODULE=on GOPROXY=off GOSUMDB=off GOCACHE=/tmp/gocache go run ./cmd/generic-test-reader -config deployments/config.yaml --reconfigure
```

Afisare log in consola:

```bash
GO111MODULE=on GOPROXY=off GOSUMDB=off GOCACHE=/tmp/gocache go run ./cmd/generic-test-reader -config deployments/config.yaml --showlog
```

Implicit, fiecare rulare scrie logul si in fisierul zilnic:

- `<reader-id>-YYYYMMDD.log`

Fisierul este creat langa configul readerului.

## SQLite

Tabele principale:

- `analytes`
- `orders`
- `order_analyses`
- `order_analysis_results`
- `event_logs`

## WS commands

Readerul raspunde la comenzi:

- `reader.status`
- `logs.list`
- `logs.tail`
- `logs.activate`
- `logs.deactivate`
- `results.activate`
- `results.deactivate`
- `analytes.list`
- `analytes.get`
- `analytes.create`
- `analytes.update`
- `analytes.delete`
- `orders.list`
- `orders.get`
- `orders.create`
- `orders.update`
- `orders.delete`
- `results.list`
- `comm.get`
- `comm.set`
- `imports.run_file`

Compatibilitate:

- comenzile vechi (`get_logs`, `list_orders`, `upsert_analyte` etc.) raman acceptate

In plus, trimite evenimente catre clienti browser pentru:

- `log`
- `tick`
- `result_available`

Pentru loguri in timp real:

1. clientul se aboneaza in `WiseMedWS` pe topicul `logs:<reader_id>`
2. clientul trimite readerului comanda `activate_real_time_logs`
3. readerul incepe sa emita evenimente `log` catre topicul respectiv

Pentru notificari de rezultat nou:

1. clientul se aboneaza in `WiseMedWS` pe topicul `results:<reader_id>`
2. clientul trimite readerului comanda `activate_real_time_results`
3. readerul emite evenimente `result_available` de fiecare data cand este persistat un rezultat nou, indiferent de canalul de comunicare folosit de reader
