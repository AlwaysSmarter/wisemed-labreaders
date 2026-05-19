# WSM Server Last

Implementare noua, separata, pentru serverul central:

- API HTTP pentru diagnostic si integrare ulterioara cu WiseMed
- hub WebSocket cu registry explicit de conexiuni
- write pump separat pe conexiune, backpressure si ping/pong
- autentificare WS obligatorie prin JWT semnat cu o cheie din `security.accepted_keys`
- pagini HTML de test servite direct de server

## Structura

- `cmd/wsm-server` - entrypoint
- `internal/config` - config YAML + env overrides
- `internal/server` - API + WebSocket hub
- `web` - pagini de test

## Run

```bash
cd "Server Last/wsm-server"
GOPROXY=off GOSUMDB=off GOCACHE=/tmp/gocache go run ./cmd/wsm-server -config deployments/config.yaml
```

## Test

Deschide:

- `http://127.0.0.1:8090/test/test-a.html`
- `http://127.0.0.1:8090/test/test-b.html`
- optional `http://127.0.0.1:8090/test/test-reader.html`

Inspectie:

- `http://127.0.0.1:8090/api/connections`
- `http://127.0.0.1:8090/api/debug/state`

## Observatii de scalare

Configuratia implicita este gandita pentru cel putin 50 de conexiuni simultane:

- coada per conexiune: `send_queue_size=128`
- ping/pong automat
- timeout de scriere/citire
- registry explicit + statistici de mesaje dropped

## Securitate WS

Conexiunile WebSocket sunt validate obligatoriu:

- tokenul se trimite in `Authorization: Bearer ...` sau `?token=...`
- semnatura trebuie sa se potriveasca cu una dintre cheile din `security.accepted_keys`
- `sub` din token trebuie sa fie exact cheia logica din lista acceptata
- pentru `role=reader`, `hello.reader_id` trebuie sa corespunda cu `reader_id` sau `sub` din token
