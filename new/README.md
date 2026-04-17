# WiseMED Next - Implementation Layout

Structura urmeaza modelul proiectului initial:

- `implementation/<service-or-analyzer>` - aplicatii compilabile separat
- `output/<service-or-analyzer>` - binare rezultate
- `internal/*` - surse comune partajate intre implementari (control, storage, protocol, modele)

## Implementari create

- `implementation/wisemedws` - webservice central WS+API (rulabil separat / Docker)
- `implementation/maglumi-800` - reader pentru test, bazat pe nucleul comun

## Arhitectura curenta

1. Reader (`maglumi-800`) porneste din config YAML.
2. Reader se conecteaza outbound la `wisemedws` pe WebSocket folosind `X-Reader-API-Key`.
3. Userii WiseMED (JWT admin) controleaza reader-ul prin API-ul `wisemedws`.
4. Reader pastreaza local in SQLite:
- nomenclator analize (`name`, `tag`)
- rezultate in outbox (offline-first)
- evenimente de comunicatie
5. Cand conexiunea revine, reader sincronizeaza automat rezultatele buffered.

## Share cod intre implementari

Sursele comune sunt in:

- `internal/readeragent/*`
- `internal/shared/*`

Astfel poti adauga usor implementari noi in `implementation/*` fara duplicare de logic.

## Build

### WiseMED WS

```bash
cd new/implementation/wisemedws
cp deployments/wisemedws.example.yaml deployments/wisemedws.yaml
export WISEMEDWS_ADMIN_JWT_SECRET='change-me'
./build.sh
```

Binar: `new/output/wisemedws/wisemedws`

### Reader Maglumi 800

```bash
cd new
cp deployments/reader-agent.example.yaml deployments/reader-agent.yaml
export WMR_READER_APIKEY='reader-key-demo-001'
cd implementation/maglumi-800
./build.sh
```

Binar: `new/output/maglumi-800/reader-maglumi-800`

## Run

### 1) wisemedws

```bash
cd new
./output/wisemedws/wisemedws -config implementation/wisemedws/deployments/wisemedws.yaml
```

### 2) maglumi-800 reader

```bash
cd new
./output/maglumi-800/reader-maglumi-800 -config deployments/reader-agent.yaml
```

## API wisemedws

- `GET /healthz`
- `GET /api/readers` (Bearer JWT role=admin)
- `POST /api/readers/{readerID}/commands` (Bearer JWT role=admin)
- `GET /api/readers/{readerID}/results` (Bearer JWT role=admin)

Comenzi suportate initial:

- `ping`
- `get_status`
- `restart_comm`
- `test_comm`
- `set_analytes`
- `list_analytes`
- `enqueue_demo_result`
