# Docker pe EC2: SIUI Bridge, History și SIUI Data

Din rădăcina repository-ului:

```sh
./make-siui-bridge-docker.sh
./make-siui-bridge-history-docker.sh
./make-wisemed-siui-data-docker.sh
```

Fiecare comandă construiește implicit `linux/amd64`, inclusiv de pe Apple Silicon,
și exportă atomic `~/dockers/<app>-YYYYMMDD-HHMMSS.tar.gz` (UTC). Tagul imaginii
este `<app>:YYYYMMDD-HHMMSS`, identic cu versiunea embedded. Compilează nativ pe
builder cu GOOS/GOARCH pentru destinație; nu compilează AMD64 prin emulare.
Imaginea include executabilul, Help, template-ul fără secrete, CA-uri, timezone,
xmllint, OpenSC și biblioteca PC/SC. Nu include configul activ, tokenuri sau date.

Opțiuni build: `SIUI_DOCKER_PLATFORM=linux/arm64` și
`SIUI_DOCKER_OUTPUT_DIR=/cale/export`. Scripturile de update se generează automat
în `readersv3/output/<app>/update-<app>.sh` și se copiază lângă arhivă în `~/dockers`.
`scripts/prepare-reader-output.sh <app>` le regenerează și la refresh-ul output-ului
local, păstrând configurația și datele existente.

Publicați arhivele în repository-ul privat `AlwaysSmarter/dockers`, ca la WSM.
Build-ul local nu face commit/push. Copiați scriptul de update corespunzător pe EC2;
acesta este standalone și nu necesită checkout-ul surselor.

| Aplicație | Date persistente pe EC2 | UI local implicit |
| --- | --- | --- |
| siui-bridge | /opt/siui-bridge | https://127.0.0.1:19112 |
| siui-bridge-history | /opt/siui-bridge-history | https://127.0.0.1:19113 |
| wisemed-siui-data | /opt/wisemed-siui-data | https://127.0.0.1:19114 |

Containerele folosesc **host networking pe Linux**. Astfel `127.0.0.1` din
config rămâne serverul EC2, inclusiv pentru MySQL, PostgreSQL, WSM și comunicarea
Bridge → History. Porturile nu sunt remapate prin Docker. Păstrați UI pe numeric
loopback; configurați Nginx/SSH tunnel dacă este necesar accesul din exterior.
Porturile WSM (8090) și cele trei utilitare sunt distincte.

## Prima instalare

Rulați ca `ec2-user`, fără sudo în fața întregului script:

```sh
chmod +x update-siui-bridge.sh
./update-siui-bridge.sh --setup
# După configurare și pornirea interactivă, Ctrl+C, apoi:
./update-siui-bridge.sh

chmod +x update-siui-bridge-history.sh
./update-siui-bridge-history.sh --setup
./update-siui-bridge-history.sh

chmod +x update-wisemed-siui-data.sh
./update-wisemed-siui-data.sh --setup
./update-wisemed-siui-data.sh
```

Cheia implicită este `~/.ssh/github_dockers`; scriptul reutilizează agentul dacă
poate și cheia este încărcată. Dacă este necesar, pornește un agent temporar și
cere passphrase prin `ssh-add`; oprește numai agentul creat de el. Verificarea
known_hosts este activă. Accesul Git rămâne al utilizatorului curent; Docker și
pregătirea directoarelor folosesc sudo dacă este necesar.

`--setup` încarcă imaginea și rulează wizardul existent cu terminal interactiv.
Completează WiseMED și identitatea echipamentului; SIUI Data cere PostgreSQL și
WSM conform wizardului propriu. După salvare aplicația rulează în prim-plan;
Ctrl+C o oprește, apoi comanda fără `--setup` creează containerul persistent.
Setup-ul refuză să ruleze dacă acel container este deja pornit.

Alternativ, copiați o configurație completă în
`/opt/<app>/deployments/config.yaml` înainte de update. În container aceeași cale
este `/data/deployments/config.yaml`. **Configurația se poate modifica din UI**;
volumul este inscriptibil și proprietar UID/GID 65532. La instalările noi,
SQLite/outbox, History și upload-urile sunt sub `/opt/<app>/data/` (în config,
căi `../data/...`). Configurile existente păstrează căile operatorului. Logurile
și materialul TLS comun rămân lângă configurație în `deployments/`. Scriptul pregătește automat
permisiunile directoarelor și fișierelor din deployments. Căile relative ale
modulelor continuă să se rezolve față de directorul `deployments`.
Pentru migrarea datelor existente, copiați și SQLite, cheile, TLS, history,
upload-urile și directoarele referite de configurație; nu copiați doar YAML-ul.
Adaptați căile absolute vechi la `/data/...` sau montați explicit directorul.

MySQL/PostgreSQL și bazele necesare se pregătesc separat, conform README-ului
fiecărei aplicații; imaginile nu includ serverele DB. Păstrați backup-uri ale DB
și fișierelor înainte de upgrade-uri care modifică schema. S3 se configurează în
History; accesul efectiv la DB/S3/CNAS nu este demonstrat de health check-ul UI.

## Actualizări și rollback

```sh
./update-siui-bridge.sh                 # cea mai nouă arhivă pentru această app
./update-siui-bridge.sh 20261027-120000 # versiune explicită, inclusiv downgrade
```

Updaterul clonează repository-ul, selectează numai arhive tracked pentru app,
verifică gzip, arhitectura, identitatea imaginii și versiunea executabilului.
Validează lizibilitatea/YAML-ul, protocolul aplicației și listenerul loopback
înainte să oprească vechiul container. Verificările complete ale modulelor și
conexiunilor DB se execută la pornire; nu există un check-config offline complet.

Containerul nou rulează ca 65532:65532, cu root filesystem read-only, fără
capabilități, tmpfs pentru /tmp și rotație de log Docker. Template-ul și Help se
actualizează la pornire; runtime-ul păstrează valorile existente din config.yaml
și adaugă numai setările implicite lipsă. Datele persistente nu sunt în imagine.
Directorul `/opt/<app>/` aparține utilizatorului care rulează updaterul, astfel
încât poate păstra și rula scriptul acolo. Subdirectoarele `deployments/` și
`data/` aparțin utilizatorului containerului; pentru administrarea lor pe host
folosiți sudo. Numai aceste două subdirectoare sunt montate în container.

Health check-ul validează `/api/session`, protocolul și versiunea binarului.
Pentru HTTPS folosește CA-ul local persistent, cu verificare TLS activă. Un eșec
restaurează containerul vechi și verifică sănătatea lui. La prima instalare,
containerul nereușit este eliminat. Configurația/datele/schema DB nu se restaurează
automat la rollback. Containerul vechi sănătos rămâne oprit ca `<app>-backup-...`.
O imagine identică deja sănătoasă nu este repornită.

Mecanismul comun app-updates rămâne disponibil cu setările operatorului. În
container verifică/descarcă actualizările, dar aplicarea automată a unei arhive
native este oprită: imaginea se actualizează prin scriptul Docker de pe host.

Opțiuni runtime, separat pentru fiecare invocare:
`SIUI_DATA_DIR` (implicit `/opt/<app>`), `SIUI_CONTAINER_NAME` (`<app>`),
`SIUI_UPDATE_CACHE` (`~/.cache/<app>-update`), `SIUI_HEALTH_TIMEOUT` (`120`),
`SIUI_SSH_KEY` (`~/.ssh/github_dockers`). Folosiți același cache la fiecare update
al aceleiași instanțe; flock previne două actualizări simultane în acel cache.

## Mediu și token Linux

Opțional, `/opt/<app>/docker.env` conține variabile Docker `NUME=valoare`, de
exemplu cheia persistentă `WISEMED_SIUI_DATA_JWT_SECRET` (minimum 32 bytes).
Păstrați fișierul privat; updaterul îl citește cu sudo fără a-i afișa conținutul.

Pentru drivere/tokenuri, `/opt/<app>/docker.args` acceptă câte un argument complet
pe linie: `--mount=...`, `--device=...` sau `--group-add=...`. Exemplu PC/SC,
numai dacă serviciul și socketul există efectiv pe EC2:

```text
--mount=type=bind,src=/run/pcscd,dst=/run/pcscd,readonly
```

Configurați `modules.siui.pkcs11_module` cu biblioteca Linux OpenSC sau a
furnizorului. Bibliotecile suplimentare și dependențele lor trebuie să fie
compatibile cu arhitectura imaginii și montate la calea configurată. Un certificat
din Windows/macOS nu apare automat în containerul EC2; operațiile semnate necesită
tokenul accesibil și PIN-ul configurat. Nu se folosește `--privileged`.

Verificări locale: `python3 readersv3/docker/test-update.py` și, din readersv3,
`go test ./tools/docker-runtime ./apps/runner`. Testele orchestrării folosesc Git,
SSH și Docker simulate; build-ul real se verifică separat cu cele trei scripturi.
