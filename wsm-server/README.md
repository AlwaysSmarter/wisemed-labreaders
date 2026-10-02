# WSM Server

Server central HTTPS/WSS pentru mai multe instanțe WiseMED. Fiecare conexiune este
asociată unei singure instanțe (`tenant_id`) printr-un JWT verificat. Cheile,
conexiunile, broadcasturile, topicurile, statisticile și upstreamurile WiseMED sunt
separate între instanțe.

- TLS 1.2+ direct, certificat PEM/fullchain și cheie privată din fișiere externe.
- JWT HS256 cu `kid`, `tenant_id`, `iss`, `aud`, `sub`, `client_id`, `role`,
  `scopes`, `iat`, `exp`; expirarea tokenului închide și conexiunea.
- Rutare 1:1, către liste de destinatari, tipuri de clienți, topicuri și broadcast
  **în interiorul instanței autentificate**.
- Chei externe, rotație/revocare prin configurație și SIGHUP pe Unix.
- Limite de conexiuni/trafic/mesaj/topicuri/coadă, timeout hello, ping/pong,
  închidere a socketurilor la oprire și reload.
- Diagnostic autentificat; fără generator public de tokenuri sau UI de test.

## Documentație

- [Consolă admin WiseMED: add / list / key / edit / delete / rights](docs/admin-console.md)

0. [Control remote, echipamente și adaptorul API–WSS](docs/remote-control.md)
1. [Conectare, JWT, protocol și procesare 1:1 / 1:N](docs/protocol.md)
2. [Instalare, certificate, chei, rotație și operare](docs/operations.md)
3. [Migrare din versiunea veche și integrarea readersv3](docs/migration.md)
4. [Reader demonstrativ în Go](examples/reader/main.go)
5. [Punct de pornire AI](AGENTS.md) și [AI memory](../aimemory/wsm-server.md)
6. [Rezultatele verificării și pachetele de test](docs/verification.md)

## Build și verificare

Pentru terminarea TLS în Nginx, folosiți `deployments/config.http.yaml` și
`deployments/nginx-wsm.conf`. WSM poate asculta HTTP/WS pe portul 8090 cu
`server.allow_insecure_http: true` și fără bloc `server.tls`; limitați accesul
la rețeaua privată a proxy-ului. Pentru Nginx pe același host, preferați
`address: 127.0.0.1` cu `allow_insecure_loopback: true`. Clienții folosesc WSS la
adresa publică Nginx. Detalii în [operare](docs/operations.md).

Pentru Docker, imaginea include binarele și interfața comună; nu necesită sursele
readersv3 la rulare. Din rădăcina repository-ului:
`docker build -f wsm-server/Dockerfile -t wisemed-wsm:local .`.
Configurația/certificatele/cheile se montează separat; vedeți
[instrucțiunile Docker](docs/operations.md#docker-fără-repository-în-runtime).

### Pornire locală din GoLand

Rulați `./prepare-local.sh` din `wsm-server`. Pregătește:

- `output/deployments/config.yaml`: configurația activă (nu se suprascrie la rerulare).
- `output/deployments/keys/local-backend.key`: cheia externă, exclusă din Git.
- `output/control/`: interfața comună copiată din surse.

În configurația GoLand `Last ServerWS`:

- Directory: `$PROJECT_DIR$/wsm-server/cmd/wsm-server`
- Working directory: `$PROJECT_DIR$/wsm-server/output`
- Output directory: `$PROJECT_DIR$/wsm-server/output/bin`
- Program arguments: `-config deployments/config.yaml --showlog`

Configurația implicită este HTTP/WS pe portul 8090, fără bloc `tls`, destinată
unui backend privat în spatele Nginx. Health: `http://localhost:8090/healthz`.
Serverul afișează calea absolută a configurației la pornire. Nu există căutare
sau fallback automat în `deployments/local/`; acel folder este un setup vechi de
TLS pentru dezvoltare și nu mai este activ. Calea `-config` este relativă la
Working directory, nu la executabil. Configurați issuer/origins pentru frontendul
HTTPS real; cheile readerelor se provisionează separat.

`deployments/config.yaml` din surse este template-ul pentru output; fișierul din
`output/deployments/config.yaml` este cel editat pentru rularea locală. Template-ul
nu include secrete. `config.production.yaml` rămâne alternativa cu TLS direct.

### Build distribuție

Din rădăcina repository-ului:

```sh
cd wsm-server
go test -race ./...
go vet ./...
./build.sh
# Toate modulele, JavaScript și integrarea cu retry real de 30 secunde:
./verify.sh
# Pachet separat pentru server Linux x86_64:
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 WSM_OUTPUT_DIR=dist/linux-amd64 ./build.sh
```

Rezultatul este `dist/`: `bin/wsm-server`, `bin/wsmctl`, configurație exemplu,
unitate systemd, interfața comună în `control/` și documentație. Nu conține secrete
sau certificate. `WSM_OUTPUT_DIR` permite un director de pachet separat.

După configurarea fișierelor externe și a instanțelor:

```sh
./dist/bin/wsm-server -config /etc/wsm-server/config.yaml -check-config
./dist/bin/wsm-server -config /etc/wsm-server/config.yaml
```

Template: `deployments/config.production.yaml`. `deployments/config.yaml` folosește acum schema nouă HTTP; configurațiile vechi
cu `accepted_keys` necesită migrare.

## Stadiu verificat

Teste automate pentru WSS cu certificat extern și validarea CA, 50 de socketuri,
rutare și răspunsuri, izolare între instanțe, JWT/roluri/scopes, expirare/revocare,
limite, proxy WiseMED către servere TLS simulate și shutdown. `go vet` este curat.

Nu a fost instalat pe serverul central și nu a fost conectat la o instanță WiseMED
reală. Readerele/utilitarele v3 au acum transportul nou; instalările cu executabile vechi trebuie actualizate și provisionate. Hubul este un proces unic,
în memorie: nu are coadă persistentă, replay, livrare garantată sau execuție exact
o dată. Un ACK confirmă introducerea în coada de trimitere, nu procesarea comenzii.

## Imagine Docker versionată automat

Scriptul din rădăcină `./make-wsm-docker.sh` folosește același build și păstrează
exportul în `~/dockers/wisemed-wsm-<data-ora-UTC>.tar.gz`.

Din rădăcina repository-ului, cu Docker pornit:

```sh
./wsm-server/build-docker.sh
```

Scriptul construiește implicit `linux/amd64` (pentru EC2 x86_64, inclusiv de pe
Mac M2), încarcă imaginea local și exportă arhiva în `wsm-server/dist/docker/`.
Versiunea este data și ora UTC `YYYYMMDD-HHMMSS`, fără commit Git, aceeași în
binar, tagul Docker și numele arhivei. Exemplu: `wisemed-wsm:20260924-143000` și
`wisemed-wsm-20260924-143000.tar.gz`. Output-ul scriptului afișează valorile exacte.
Arhivele nu includ configurații sau chei.

Opțional: `WSM_DOCKER_PLATFORM=linux/arm64`, `WSM_DOCKER_IMAGE=registry/nume`,
`WSM_DOCKER_OUTPUT_DIR=/cale/export`. Pentru această comandă alegeți platforma
mașinii destinație. Exportul este finalizat atomic doar dacă docker save și gzip
reușesc. Scriptul refuză suprascrierea unei arhive cu aceeași versiune.

Pe server: `sudo docker load -i <arhiva.tar.gz>`, apoi folosiți tagul afișat în
comanda docker run. Verificare: `docker run --rm <imagine:versiune> --version`.
Versiunea se afișează și în logul de pornire. Comanda simplă `docker build` nu
alege automat tagul/arhiva; folosiți scriptul pentru întregul flux.

Actualizare automată pe EC2: [update-wsm.sh — instalare, verificare și rollback](docs/docker-update.md).

Pe Apple Silicon, etapa de compilare Docker rulează pe BUILDPLATFORM (ARM64),
iar GOOS/TARGETOS și GOARCH/TARGETARCH produc binarele pentru platforma selectată
(de exemplu linux/amd64). Astfel compilatorul Go nu rulează sub emulare AMD64.
