# Docker Ubuntu pentru WiseMED Update Server

Acest setup pornește un container `ubuntu:24.04` care:

- are Go `1.24.0` instalat în container
- montează repo-ul host-ului în `/opt/wmlr`
- compilează la fiecare pornire ultima versiune `./apps/update-server`
- scrie runtime-ul în `output/update-server`
- sincronizează `apps/update-server/deployments` în `output/update-server/deployments` fără să suprascrie `config.yaml`
- pornește serverul și îl expune pe portul `19090`

## Cerințe

- pe host există o clonă `wisemed-labreaders` montată în `/opt/wmlr`
- Docker și Docker Compose sunt instalate

Dacă pe host clona este în alt loc, modifică volumul și variabila `WMLR_REPO` din `compose.yaml`.

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
- `wisemed-update-server-gomod:/go/pkg/mod`: cache module Go
- `wisemed-update-server-gobuild:/root/.cache/go-build`: cache build Go

Datele runtime rămân direct în:

- `/opt/wmlr/readersv3/output/update-server/Update_Server`
- `/opt/wmlr/readersv3/output/update-server/deployments`

## Ce se întâmplă la startup

1. containerul validează repo-ul montat în `/opt/wmlr`
2. creează sau reutilizează `output/update-server`
3. copiază template-ul `apps/update-server/deployments` în runtime
4. aplică `UPDATE_SERVER_BIND` și opțional `PUBLIC_BASE_URL`
5. rulează `go build -o output/update-server/Update_Server ./apps/update-server`
6. pornește binarul cu `-config output/update-server/deployments/config.yaml`

## Observații

- `config.yaml` este păstrat între restarturi și nu este suprascris
- dacă lipsește `config.yaml`, aplicația îl recreează automat din `config.install.yaml`
- la prima pornire este necesar acces la internet pentru descărcarea modulelor Go, dacă cache-ul este gol
