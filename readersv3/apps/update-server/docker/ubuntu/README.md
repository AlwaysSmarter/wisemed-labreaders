# Docker Ubuntu pentru WiseMED Update Server

Acest setup pornește un container `ubuntu:24.04` care:

- are Go `1.24.0` instalat în container
- are NSIS (`makensis`) pentru build de installer Windows
- are MinGW binutils pentru `windres`
- montează repo-ul host-ului în `/opt/wmlr`
- montează runtime-ul persistent în `/opt/wmlr-runtime`
- compilează la fiecare pornire ultima versiune `./apps/update-server`
- scrie runtime-ul în `/opt/wmlr-runtime/update-server`
- sincronizează `apps/update-server/deployments` în `/opt/wmlr-runtime/update-server/deployments` fără să suprascrie `config.yaml`
- pornește serverul și îl expune pe portul `19090`

## Cerințe

- pe host există o clonă `wisemed-labreaders` montată în `/opt/wmlr`
- Docker și Docker Compose sunt instalate

Dacă pe host clona este în alt loc, modifică volumul și variabila `WMLR_REPO` din `compose.yaml`.

Dacă vrei altă locație pentru datele persistente, setează `WMLR_RUNTIME_ROOT` înainte de `docker compose up`.

## Pornire rapidă

Din [readersv3/apps/update-server/docker/ubuntu/compose.yaml](/Users/raduichim/work/gowork/wisemed-labreaders/readersv3/apps/update-server/docker/ubuntu/compose.yaml):

```bash
docker compose up --build -d
```

Serverul va fi disponibil pe:

```text
http://<ip-ul-masinii>:19090
```

## Configurare utilă

Setează URL-ul public înainte de pornire dacă vrei linkuri de download corecte generate de update-server:

```bash
export PUBLIC_BASE_URL="http://<ip-sau-dns>:19090"
docker compose up --build -d
```

Dacă vrei alt port pe host:

```bash
export WMLR_PORT=29090
export PUBLIC_BASE_URL="http://<ip-sau-dns>:29090"
docker compose up --build -d
```

## Volume persistente

- `/opt/wmlr:/opt/wmlr`: repo-ul sursă de pe host
- `${WMLR_RUNTIME_ROOT:-/opt/wmlr-runtime}:/opt/wmlr-runtime`: runtime persistent separat de worktree-ul Git
- `wisemed-update-server-gomod:/go/pkg/mod`: cache module Go
- `wisemed-update-server-gobuild:/root/.cache/go-build`: cache build Go

Datele runtime rămân direct în:

- `${WMLR_RUNTIME_ROOT:-/opt/wmlr-runtime}/update-server/Update_Server`
- `${WMLR_RUNTIME_ROOT:-/opt/wmlr-runtime}/update-server/deployments`

## Ce se întâmplă la startup

1. containerul validează repo-ul montat în `/opt/wmlr`
2. creează sau reutilizează `${WMLR_RUNTIME_ROOT:-/opt/wmlr-runtime}/update-server`
3. copiază template-ul `apps/update-server/deployments` în runtime
4. aplică `UPDATE_SERVER_BIND` și opțional `PUBLIC_BASE_URL`
5. rulează `go build -o /opt/wmlr-runtime/update-server/Update_Server ./apps/update-server`
6. pornește binarul cu `-config /opt/wmlr-runtime/update-server/deployments/config.yaml`

La fiecare startup, logurile containerului afișează explicit:

- `Repo root`
- `Runtime dir`
- `Deployments dir`
- `Binary path`
- `Config path`
- `Start command`

## Observații

- `config.yaml` este păstrat între restarturi și nu este suprascris
- `app-update-server.db`, `files/` și release-urile rămân în afara clonei Git, deci supraviețuiesc la `git reset --hard` și `git pull`
- dacă lipsește `config.yaml`, aplicația îl recreează automat din `config.install.yaml`
- la prima pornire este necesar acces la internet pentru descărcarea modulelor Go, dacă cache-ul este gol
