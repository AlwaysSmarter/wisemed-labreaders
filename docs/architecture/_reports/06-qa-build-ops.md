# 06 — QA, build, distribuție și operare

Data investigației: 2026-11-14. Mediu de execuție: macOS (darwin/arm64, Darwin 23.5.0), `go1.25.5`, `node v24.10.0`, Python 3.14 (Homebrew), `/bin/bash` 3.2.57. Commit de bază: `d53088e` (branch `main`), cu modificările locale ale utilizatorului din `git status`.

Etichete folosite: `CONFIRMAT DIN COD`, `CONFIRMAT PRIN TEST LOCAL`, `DOCUMENTAT, NEVERIFICAT`, `INFERENȚĂ`, `NECUNOSCUT`.

Reguli respectate: nu a fost modificat cod, configurații, `output/`, `dist/`, baze de date sau certificate; nu a fost rulat niciun script de build/deploy/update/docker/installer; nu a fost apelat niciun sistem extern (testele Go au rulat cu `GOPROXY=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly`). `git status --porcelain` înainte și după execuție este identic (38 de linii, diff gol). Outputul complet al fiecărei rulări este în scratchpad (`/private/tmp/claude-501/-Users-raduichim--claude-spares-spare-wKjv88/5cf1d680-26df-44a8-ba4d-1933f6607b68/scratchpad/`).

---

## PARTEA A — Build, distribuție, operare

### A.0 Harta generală

| Familie | Sursă | Unealtă de build | Artefact | Canal de livrare |
| --- | --- | --- | --- | --- |
| Readere de analizoare + utilitare desktop (32 de `apps/*/main.go`) | `readersv3/apps/*` | `readersv3/tools/releasectl` (Go) | `dist/<target>/<app>/runtime`, `dist/updates/*.zip`, `dist/installers/*-Setup.exe`, `release/<app>/<target>/*.{tar.gz,deb,rpm,pkg,dmg}` | installer nativ + `update-server` (modul `app-updates`) |
| Servicii Docker SIUI (`siui-bridge`, `siui-bridge-history`, `wisemed-siui-data`) | `readersv3/apps/*` + `readersv3/docker/` | `readersv3/docker/build.sh` (prin `make-*-docker.sh` din rădăcină) | `~/dockers/<app>-YYYYMMDD-HHMMSS.tar.gz` + `update-<app>.sh` | repository privat de arhive Docker, updater standalone pe EC2 |
| Alte imagini Docker (`wisemed-ai`, `html-print-formatter`, `update-server`) | `readersv3/apps/<app>/docker/` | `docker compose` | imagine locală | compose pe host |
| WSM Server | `wsm-server/` | `build.sh`, `build-docker.sh`, `Dockerfile` | `dist/` (binare + deployments + docs + control), `wisemed-wsm-<versiune>.tar.gz` | systemd sau Docker + `update-wsm.sh` |

### A.1 Readere și utilitare (`readersv3`)

**Puncte de intrare build** — `CONFIRMAT DIN COD`
- `readersv3/Makefile:11-27`: ținte `build`, `installer`, `update`, `release`, `build-all`, `package-all`; toate deleagă la `go run ./tools/releasectl …` cu `APP`, `TARGET` (implicit GOOS-GOARCH al hostului) și `VERSION` opțional. Cache-ul Go este forțat în `/tmp/wisemed-releasectl-go{cache,modcache}` (`Makefile:5`).
- Wrapper-e generate: `build-all-platforms.{sh,ps1}`, `package-all-platforms.{sh,ps1}`, `sync-release-scripts.{sh,ps1}` (3–5 linii fiecare, doar `go run ./tools/releasectl <cmd>`); `build-all-targets-macos.sh` și `package-all-targets-macos.sh` refuză să ruleze în afara macOS.
- Per aplicație: `apps/<app>/build/{build-all,build-linux,build-macos,build-windows}.*` plus `linux/` (unit systemd, `install.sh`, `uninstall.sh`, `build-installer.sh`), `macos/` (plist, install/uninstall), `windows/` (`.nsi`, `.wxs`, `.bundle.wxs`, `.winsw.xml`, `install.ps1`, `uninstall.ps1`). Sunt generate de `releasectl sync` (`tools/releasectl/main.go:154-195`, `:984-1015`).

**Comenzi `releasectl`** — `CONFIRMAT DIN COD` (`tools/releasectl/main.go:117-150`): `sync`, `build`, `build-all`, `package`, `package-update`, `release`, `package-all`.

**Platforme țintă** — `CONFIRMAT DIN COD` (`main.go:82-90`): `windows-386`, `windows-amd64`, `windows-arm64`, `linux-amd64`, `linux-arm64`, `darwin-amd64`, `darwin-arm64`.
- Excepție: `esignature-server` se construiește numai pentru `windows-386` (DLL Signotec x86), iar celelalte aplicații nu se construiesc pentru `windows-386` (`main.go:243-252`, `:444-446`). DLL-ul este copiat din `docs/signotec/WebSocketPadServer/STPadLib.dll` (`main.go:523-527`).
- `package-all` împachetează doar țintele sistemului gazdă: Windows → trei ținte Windows, Linux → `linux-amd64/arm64`, macOS → `darwin-amd64/arm64` (`main.go:395-405`).

**Compilare** — `CONFIRMAT DIN COD` (`main.go:476-515`): `go build -a -trimpath -buildvcs=false -mod=readonly`, `GOWORK=off`, `SOURCE_DATE_EPOCH` = data ultimului commit; prima încercare fără `CGO_ENABLED` explicit, la eșec reîncearcă cu `CGO_ENABLED=1`. Înainte de fiecare build directorul `dist/<target>/<app>/runtime` este șters complet (`main.go:464-467`).

**Embed versiune** — `CONFIRMAT DIN COD`
- Variabile: `shared/appmeta/appmeta.go:5-10` (`Version`, `Commit`, `BuildTime`, `VersionMarker`; implicit `0.0.0-dev`).
- ldflags: `main.go:552-564` (`-s -w -buildid= -X …appmeta.Version/Commit/BuildTime/VersionMarker`).
- Sursa versiunii: `--version` → variabila de mediu `VERSION` → `git describe --tags --always --match 'v*'` → `0.1.0` (`main.go:1786-1795`). Normalizare: prefixul `v` eliminat, `-`, `_`, `+`, `/` devin `.` (`main.go:1808-1820`).
- După build, versiunea este recitită din binar prin markerul `WISEMED_APP_VERSION=` (`main.go:566-586`), deci numele pachetelor reflectă ce e efectiv în executabil.
- Toate aplicațiile acceptă `-version` (ex. `apps/generic-test-reader/main.go:19-24`).

**Layout `dist/` și `release/`** — `CONFIRMAT DIN COD` + observat pe disc
- `dist/<target>/<app>/runtime/` = binar + `deployments/` filtrat + `manifest.json` (`main.go:531-546`).
- Filtrul `deployments` exclude `config.yaml`, `.db/.sqlite*`, `.log`, `.zip`, `.pid`, `.bak`, `.tmp`, plus directoarele `logs/`, `cache/`, `updates/`, `processed/`, `failed/`, `outbox/`, `inbox/` (`main.go:1413-1446`). Doar `config.install.yaml` și asset-urile statice ajung în pachet.
- `dist/updates/<app>-<versiune>-<target>.zip` — pachetul de update (fără `manifest.json`), cu SHA-256 logat (`main.go:1225-1260`). Pe disc: 62 de arhive.
- `dist/installers/<slug>-<versiune>[-arch]-Setup.exe` — installer NSIS (`main.go:653-700`, `:1101-1114`). Pe disc: 43 de fișiere.
- `dist/releases/<app>/<target>/<versiune>.json` — metadate de release (`main.go:609`).
- `release/<app>/<target>/` — pachete Linux (`.tar.gz`, `.deb`, `.rpm`) și macOS (`.pkg`, `.dmg`) (`main.go:705`, `:743`, `:782`, `:815`, `:846`).
- `dist/wss-preview/<timestamp>/` — preview-uri izolate produse de `scripts/build-wss-preview.sh` (nu atinge `output/`, refuză o destinație existentă: `scripts/build-wss-preview.sh:8-11`).

**Installer Windows (NSIS)** — `CONFIRMAT DIN COD`
- Script comun: `installer/windows/installer.nsi` (85 de linii), apelat de `scripts/build-installer.sh` care face un self-test `makensis`, copiază binarul + `deployments/`, șterge explicit `config.yaml`, `*.db*`, `*.log` din payload (`scripts/build-installer.sh:75-95`) și generează un wrapper `.nsi` cu `!define`-uri.
- Installerul cere admin (`installer.nsi:1`), instalează în `$PROGRAMFILES64\<DisplayName>` (sau `$PROGRAMFILES` pentru 386, `:37-41`), scrie cheile de registry pentru Uninstall (`:54-58`), creează scurtături în Start Menu și pe Desktop (`:60-62`) și oferă pornirea aplicației la final (`:81-85`).
- Installerul **nu instalează un serviciu Windows**. Uninstall-ul șterge numai binarul, `config.install.yaml` și `help/`, deci `config.yaml`, bazele de date și logurile operatorului rămân pe disc (`:67-79`).
- Dacă `makensis` lipsește, `release` continuă fără installer (`main.go:597-603`).
- `apps/wisemed-ai/docker/build-windows-release.sh` rulează `makensis` dintr-un container Debian când NSIS-ul local nu funcționează.

**Pachete Linux** — `CONFIRMAT DIN COD` (`main.go:704-779`): rootfs `/opt/<app>/` + unit `/etc/systemd/system/<serviceName>.service`; întotdeauna `.tar.gz`; `.deb`/`.rpm` prin `fpm` dacă există, altfel `dpkg-deb`/`rpmbuild`. Post-install: `systemctl daemon-reload`, `enable`, `restart` (`main.go:2353-2360`). Pentru `wisemed-ai` și `html-print-formatter` se creează un cont de sistem dedicat și unitul rulează sub acel utilizator; restul rulează ca `root` (`main.go:1543-1558`, `:2312`).

**Pachete macOS** — `CONFIRMAT DIN COD` (`main.go:781-851`): payload în `/usr/local/<app>/`, LaunchDaemon `/Library/LaunchDaemons/<bundleId>.plist` (`RunAtLoad`, `KeepAlive`, log în `/var/log/<app>.log`, `main.go:2369-2391`), `.pkg` prin `pkgbuild` + `productbuild`, `.dmg` prin `hdiutil`. Nu există semnare sau notarizare în cod — `CONFIRMAT DIN COD` (nicio referință la `codesign`/`notarytool`).

**Instalare ca serviciu per OS (din aplicație, `-installservice`)** — `CONFIRMAT DIN COD`
- Nume serviciu: `wisemed-<token>` unde tokenul vine din `reader.id` → `analyzer_code` → `label` din configurație (`apps/runner/service_common.go:32-41`).
- Windows (`apps/runner/service_windows.go:16-52`): `sc.exe create/config` cu `start= auto`, `sc.exe failure` (restart la 60s/120s/300s, reset 86400s), `sc.exe start`. Procesul detectează rularea ca serviciu prin `svc.IsWindowsService()` și intră în `svc.Run` (`:66-79`), tratând `Stop`/`Shutdown` (`:85-115`).
- Linux (`apps/runner/service_linux.go:14-78`): scrie `/etc/systemd/system/<serviciu>.service` (`Type=simple`, `Restart=on-failure`, `RestartSec=5`, jurnal), apoi `systemctl daemon-reload` și `enable --now`. Pentru `html-print-formatter` și `wisemed-ai` creează utilizator de sistem și schimbă proprietarul directorului.
- macOS (`apps/runner/service_darwin.go:15-67`): scrie `/Library/LaunchDaemons/eu.wisemed.readersv3.<token>.plist`, apoi `launchctl bootout/bootstrap/enable/kickstart`.
- Linux/macOS nu au manager de serviciu în proces (`service_unix.go:10-12`); oprirea se face prin `SIGTERM`/`os.Interrupt` (`runner.go:149`).

**Mod headless** — `CONFIRMAT DIN COD`: `-headless` pornește un proces copil cu `-headless-child` într-o sesiune nouă (`Setsid`), așteaptă confirmarea pornirii (maxim 1 minut), scrie un fișier `<reader>.pid` lângă configurație, afișează PID-ul și iese (`apps/runner/runner.go:126-139`, `headless_unix.go:18-54`, `headless_startup.go:28-36`).

**Update automat (modul `app-updates`)** — `CONFIRMAT DIN COD`
- La pornire: `checkForUpdates` (`apps/runner/updates.go:22-60`) folosește `modules.app-updates.{enabled,base_url,app_id,channel}` și `cfg_wisemed_key` din `wisemed-api`. Erorile de conectare sunt neblocante (`runner.go:116-123`). Unele aplicații verifică în fundal (`RunOptions.BackgroundStartupUpdates`, `background_updates.go`).
- Descărcarea calculează SHA-256 (`shared/appupdates/appupdates.go:171-196`) și îl compară cu `checksum_sha256` din răspunsul serverului numai dacă acesta este prezent (`updates.go:142-143`). La nepotrivire update-ul nu se aplică; pentru un update obligatoriu aplicația continuă cu avertisment, iar în mod headless întoarce eroare (`updates.go:144-150`). Un răspuns fără checksum este acceptat fără verificare de integritate.
- Aplicare: arhivele `.zip` se extrag într-un director de staging, apoi un script detașat (PowerShell pe Windows, `/bin/sh` pe Unix) așteaptă ieșirea procesului, copiază fișierele peste instalare și repornește aplicația — pe Windows prin `sc.exe start` dacă rula ca serviciu (`update_apply.go:17-44`, `:96-181`). Alte fișiere (installer `.exe`) se lansează ca installer.
- Aplicarea automată are loc pentru update obligatoriu sau când procesul rulează ca serviciu, și este dezactivată în container (`WISEMED_CONTAINER=1`, `updates.go:157`).
- Nu există backup al versiunii anterioare și nici rollback la aplicarea unei arhive — `CONFIRMAT DIN COD` (scriptul doar copiază peste, `update_apply.go:138-146`, `:171`).

**Observabilitate readere** — `CONFIRMAT DIN COD`
- Log zilnic `<reader>-YYYYMMDD.log` lângă `config.yaml` (`apps/runner/logging.go:18-35`), cu microsecunde; `-showlog` dublează în consolă, cu `[ERROR]` colorat pe terminal interactiv (`:37-44`, `:198-215`).
- La pornire, logurile săptămânilor încheiate sunt arhivate în `<reader>-<de la>-<până la>.zip` și fișierele sursă sunt șterse (`logging.go:82-131`).
- Numele fișierului de log se stabilește o singură dată, la pornire (`logging.go:30-32`); nu am găsit rotație la miezul nopții în `core/` sau `shared/`. Un proces care rulează mai multe zile continuă să scrie în fișierul zilei de pornire, iar arhivarea săptămânală are loc doar la următoarea pornire — `INFERENȚĂ` (din citirea codului, neverificat prin rulare îndelungată).
- Niveluri: nu există un nivel de log configurabil (`log_level`) în `core/` sau `shared/`; se folosesc prefixe textuale precum `[ERROR]` — `CONFIRMAT DIN COD` (căutare fără rezultate).
- Health: `GET /healthz` pe HTTP-ul local întoarce `{"ok":true,"reader":…}` (`core/runtime/runtime.go:50`, `:151-156`); verifică doar că procesul răspunde. Alte endpointuri: `/api/status` (cu sesiune, `modules/localhttp/module.go:246`), `/health` (`htmlprintformatter`, `signingpad`), `/health/ai` și `/ready/ai` (`wisemedai`).
- Diagnostic: pachetele `shared/commtrace`, `shared/debugreplay`, `shared/analyzeractivity` există, dar nu au teste (vezi B.3).

**CI (GitHub Actions)** — `CONFIRMAT DIN COD` (`.github/workflows/readersv3-release.yml`, generat de `releasectl sync`, `main.go:189-192`)
- Declanșare: `workflow_dispatch` (cu `version` opțional) și push de tag `v*.*.*` (`:3-12`).
- Job `sync`: rulează `releasectl sync` și cere arbore curat cu `git diff --exit-code` (`:18-30`).
- Job `package`: matrice `ubuntu-24.04`, `windows-2022`, `macos-14`; instalează `fpm`/`rpm` pe Linux și `wixtoolset` + `nsis` pe Windows; rulează `releasectl package-all`; urcă `readersv3/release` ca artefact (`:32-73`).
- Job `release`: pe tag, publică artefactele ca GitHub Release (`:75-86`).
- Workflow-ul nu rulează `go test`, `go vet`, teste Node sau Python. Nu există niciun workflow pentru `wsm-server`. Singurul fișier din `.github/` este acesta.
- Fișierul cerut în brief, `readersv3/tools/readersv3-release.yml`, nu există; workflow-ul este doar în `.github/workflows/`.

### A.2 Servicii Docker SIUI (`siui-bridge`, `siui-bridge-history`, `wisemed-siui-data`)

**Build** — `CONFIRMAT DIN COD`
- `make-siui-bridge-docker.sh`, `make-siui-bridge-history-docker.sh`, `make-wisemed-siui-data-docker.sh` (rădăcină) → `readersv3/docker/build.sh <app>`.
- `docker/build.sh:5-24`: acceptă doar cele trei aplicații; versiune = timestamp UTC `YYYYMMDD-HHMMSS`; platformă implicită `linux/amd64` (`SIUI_DOCKER_PLATFORM`); export în `~/dockers` (`SIUI_DOCKER_OUTPUT_DIR`); refuză suprascrierea unei arhive existente; export atomic (`mktemp` + `mv`).
- Înainte de build apelează `scripts/prepare-docker-output.sh`, care **scrie în `readersv3/output/<app>/`** (`update-<app>.sh` și `docker/config.install.yaml`) — motiv pentru care nu a fost rulat.
- `docker/Dockerfile`: build nativ pe `$BUILDPLATFORM` cu `golang:1.26.8` și cross-compile `CGO_ENABLED=0` (`:1-21`); runtime `debian:bookworm-slim` cu `ca-certificates`, `tzdata`, `libxml2-utils`, `opensc`, `libpcsclite1` (`:24-30`); utilizator `65532:65532` (`:40`); `ENV WISEMED_CONTAINER=1` (`:33`); label `eu.wisemed.app` (`:34`).
- Versiunea este injectată numai prin `appmeta.Version` (`Dockerfile:17`); `Commit`, `BuildTime` și `VersionMarker` rămân la valorile implicite.
- Contextul de build este o listă albă (`docker/Dockerfile.dockerignore`): numai `core`, `modules`, `shared`, `apps/runner`, `main.go` + `config.install.yaml` + `help/` ale celor trei aplicații și `tools/docker-runtime`.

**Rulare** — `CONFIRMAT DIN COD` (`docker/update.sh:122-125`, `:160-161`)
- `--network host`, `--read-only`, `--cap-drop ALL`, `--security-opt no-new-privileges:true`, tmpfs `/tmp`, `--restart unless-stopped`, rotație log Docker (`max-size=10m`, `max-file=3`).
- Două bind mount-uri: `<root>/deployments → /data/deployments` și `<root>/data → /data/data`; `root` implicit `/opt/<app>`.
- Host networking este intenționat: `127.0.0.1` din configurație rămâne hostul EC2 (baze de date, WSM, Bridge → History). `docker-runtime` refuză o adresă `local_http` care nu este loopback numeric (`tools/docker-runtime/main.go:44-48`).

**Păstrarea configurației** — `CONFIRMAT DIN COD`
- `docker/entrypoint.sh:5-12` rescrie la fiecare pornire doar `config.install.yaml` (cu căile de date mutate în `../data/…`) și `help/`; `config.yaml` al operatorului nu este atins. Runtime-ul completează doar cheile lipsă din template (`config.Ensure`, `apps/runner/runner.go:49-64`).
- Opțional: `<root>/docker.env` (variabile) și `<root>/docker.args` (numai `--mount=`, `--device=`, `--group-add=`; orice altceva oprește scriptul, `update.sh:127-137`).

**Updater standalone** (`docker/update.sh`, generat per aplicație prin înlocuirea `@APP@`) — `CONFIRMAT DIN COD`
- Nu are nevoie de checkout-ul surselor; cere `git ssh ssh-agent ssh-add ssh-keygen docker flock python3 gzip` (`:17-19`).
- Pași: blocare `flock` (`:22-23`) → clonare superficială a repository-ului de arhive cu cheia dedicată, `IdentitiesOnly=yes`, `BatchMode=yes` (`:78-81`) → selecția arhivei tracked, cu refuz la duplicate și symlink-uri (`:82-97`) → `gzip -t`, `docker load`, verificarea arhitecturii, a labelului `eu.wisemed.app` și a versiunii binarului (`:100-107`) → `docker-runtime check-config` înainte de oprirea containerului vechi (`:149-150`) → redenumire + oprire container vechi (`:155-157`) → pornire container nou → așteptare `healthy` (implicit 120 s).
- Rollback automat la eșec: șterge containerul nou, redenumește și repornește pe cel vechi (`:45-63`). La succes, containerul vechi rămâne oprit ca `<app>-backup-…`. Datele și migrările de schemă nu sunt restaurate (`:170`).
- `--setup` rulează wizardul interactiv (`-reconfigure`) și refuză dacă există un container pornit (`:112-114`, `:138-143`).

**Health** — `CONFIRMAT DIN COD`: `HEALTHCHECK` la 5 s rulează `docker-runtime health` (`Dockerfile:41-42`), care cere `GET /api/session` pe loopback (HTTPS cu CA-ul local persistent dacă `local_http.tls`), și verifică protocolul și versiunea (`tools/docker-runtime/main.go:55-92`). Nu verifică baza de date, S3 sau CNAS.

**Alte imagini** — `CONFIRMAT DIN COD`
- `wisemed-ai` (`apps/wisemed-ai/docker/`): build în trei etape (Node 22 pentru frontend, `golang:1.24-bookworm`, `debian:bookworm-slim`), utilizator `wisemed-ai` (UID 10001), healthcheck `curl /ready/ai`; `compose.yaml` adaugă `pgvector/pgvector:pg16`, publică implicit pe `127.0.0.1:19120`, `cap_drop: [ALL]`. `make-wisemed-ai-docker.sh` rulează implicit `docker compose up -d --build`. Entrypoint-ul creează `config.yaml` doar dacă lipsește și schimbă adresa de ascultare la `0.0.0.0:19120` (`entrypoint.sh:4-7`).
- `html-print-formatter` (`apps/html-print-formatter/docker/ubuntu/`): `golang:1.25-bookworm` → `ubuntu:24.04`, UID 10001, `tini`, port 19113, healthcheck la 30 s.
- `update-server` (`apps/update-server/docker/ubuntu/`): `ubuntu:24.04` cu Go 1.24.0 instalat în imagine, port 19090; `restart` este comentat în compose.

### A.3 WSM Server

**Build** — `CONFIRMAT DIN COD`
- `wsm-server/build.sh`: `go build -trimpath -ldflags "-X main.buildVersion=<timestamp UTC>"` pentru `cmd/wsm-server` și `cmd/wsmctl`; copiază `deployments/*`, `README.md`, `docs/*.md`, exemple și **șase** asset-uri de control UI din `readersv3/modules/localhttp/ui/` în `dist/` (sau `WSM_OUTPUT_DIR`). Platforma se alege prin `GOOS`/`GOARCH` din mediu.
- `wsm-server/Dockerfile`: `golang:1.26.8` → imagine `scratch` cu `ca-certificates.crt`, `wsm-server`, `wsmctl` și `control/`; utilizator `65532:65532`; expune 8443 și 8090; `CMD -config /etc/wsm-server/config.yaml`. Copiază **patru** asset-uri de control (`Dockerfile:16`).
- `wsm-server/build-docker.sh` + `make-wsm-docker.sh`: imagine `wisemed-wsm:<timestamp>`, export `wisemed-wsm-<timestamp>.tar.gz`, platformă implicită `linux/amd64`.
- `wsm-server/prepare-local.sh`: pregătește `wsm-server/output/` (generează o cheie locală cu `openssl rand` dacă lipsește, copiază `config.yaml` doar dacă lipsește, recopiază asset-urile de control). **Scrie în `wsm-server/output/`** — nerulat.
- Versiune: `main.buildVersion` (implicit `dev`), afișată cu `--version` și în log la pornire (`cmd/wsm-server/main.go:18`, `:27-33`, `:42`).

**Instalare ca serviciu** — `CONFIRMAT DIN COD` (`deployments/wsm-server.service`): utilizator `wsm-server`, `ExecReload=kill -HUP`, `Restart=on-failure`, hardening (`NoNewPrivileges`, `ProtectSystem=strict`, `ProtectHome`, `PrivateTmp`, `RestrictAddressFamilies`), `StateDirectory=wsm-server`, `LimitNOFILE=16384`. Pașii manuali sunt în `docs/operations.md:68-76` — `DOCUMENTAT, NEVERIFICAT`. Nu există installer sau pachet `.deb`/`.rpm` pentru WSM. Pe Windows nu există SIGHUP (`docs/operations.md:237`).

**Docker + update** — `CONFIRMAT DIN COD` (`update-wsm.sh`)
- Rulare: `--read-only`, `--cap-drop ALL`, `no-new-privileges`, `-p 127.0.0.1:<port>:8090`, `deployments` montat read-only în `/etc/wsm-server`, `state` montat inscriptibil în `/var/lib/wsm-server` (`:128-134`). Nu folosește host networking.
- Fluxul de selecție, verificare, `-check-config` înainte de oprire, redenumire, health și rollback este același ca la updaterul SIUI (`:86-143`). Health: `curl http://127.0.0.1:<port>/healthz` trebuie să întoarcă `status=ok`, `service=wsm-server` (`:68-77`).
- Nginx: `deployments/nginx-wsm.conf` (TLS 1.2/1.3, HSTS, `access_log off`, `proxy_read_timeout 310s`, păstrează `Host` cu port).

**Observabilitate WSM** — `CONFIRMAT DIN COD`
- Loguri la stderr/journal și în fișiere zilnice UTC `wsm-YYYY-MM-DD.log` (`internal/logstore/store.go:52`, `cmd/wsm-server/main.go:67-73`); director implicit `logs/` lângă registrul de chei sau `../logs` față de configurație (`internal/config/config.go:202-209`). Fără retenție automată (`docs/operations.md:207`).
- `-showlog` este păstrat doar pentru compatibilitate (`cmd/wsm-server/main.go:30`).
- `GET /healthz` → `{"status":"ok","service":"wsm-server"}` (`internal/server/server.go:164-165`); nu verifică upstreamurile.
- `-check-config` validează schema strictă, secretele și fișierele TLS (`cmd/wsm-server/main.go:29`).
- Reload prin SIGHUP pe Unix (`cmd/wsm-server/reload_unix.go:17-18`); conform documentației deconectează toate socketurile — `DOCUMENTAT, NEVERIFICAT` în această sesiune (există teste de server care au trecut, dar nu am izolat testul de reload).
- Contoare de conexiuni disponibile cu scope `connections:read`, resetate la restart (`docs/operations.md:266-268`) — `DOCUMENTAT, NEVERIFICAT`.

### A.4 Fișiere binare, backup-uri și material sensibil urmărite în git (observație)

`CONFIRMAT DIN COD` (`git ls-files`, 1994 de fișiere urmărite):

| Cale | Mărime | Observație |
| --- | --- | --- |
| `wsm-server/output/bin/WSM_Server` | 11,3 MB | binar urmărit |
| `wsm-server/output/bin/Last_ServerWS` | 11,8 MB | binar urmărit |
| `wsm-server/output/bin/WSM_Server.backup-20261016T094920Z`, `…T095326Z`, `.backup-before-cnas-20261102`, `.backup-before-docmed-20261102`, `.backup-before-docmed-forms-20261103` | 11,0–12,1 MB fiecare | cinci backup-uri, adăugate în index (`A` în `git status`) |
| `output/wsm-browser-sdk/bin/wsm-server` + trei `.backup-*`, `wsmctl` | 6,5–11,3 MB | binare + backup-uri; și `control/app.js.backup-*`, `docs/*.backup-*` |
| `readersv3/zonci-xl1000i-reader.exe` | 23,1 MB | executabil Windows în rădăcina `readersv3` |
| `readersv3/output/update-server/deployments/files/**` | — | arhive `.zip` și installere `.exe` (barcodeprinter 1.0.10/1.0.11, cary60 1.0.10) + `app-update-server.db` |
| `readersv3/release/barcodeprinter/linux-amd64/barcodeprinter-1.2.3-amd64.tar.gz`, `readersv3/dist/linux-amd64/barcodeprinter/**` (inclusiv `.db` și `config.yaml`) | — | urmărite deși `.gitignore` exclude `readersv3/dist/` și `readersv3/release/` |
| `docs/signotec/**` (`STPadLib.dll`, `STPadServer.exe`, drivere WinUSB) | — | binare de la furnizor; `STPadLib.dll` este dependență de build pentru `esignature-server` |
| `docs/signotec/WebSocketPadServer/server.pem`, `serverkey.pem` | — | un fișier de cheie privată este urmărit (conținut neinspectat) |
| `readersv3/apps/signing-pad-utility/java/signing-pad-helper.jar` | — | dependență runtime |
| 15 fișiere `config.yaml` din `output/` și `deployments/` (ex. `readersv3/output/*/deployments/config.yaml`, `wsm-server/output/deployments/config.yaml`, `output/wsm-browser-sdk/deployments/config.yaml`, `readersv3/apps/shimatzu-generic-reader/deployments/config.yaml`) | — | configurații runtime urmărite, deși `.gitignore` exclude `/readersv3/output/*/deployments/config.yaml`; conținut neinspectat pentru secrete |

`wsm-server` include în modulul Go directoarele `dist/*/examples/reader` (cinci copii ale aceluiași `main.go`), deci `go build ./...` și `go vet ./...` le compilează și pe acestea.

---

## PARTEA B — Validare

### B.0 Verificare de siguranță înainte de rulare

`CONFIRMAT DIN COD` (grep pe toate `*_test.go` din `readersv3` și `wsm-server`):
- Nicio referință la `output/` sau `dist/` în teste. Fișierele se scriu în `t.TempDir()`.
- URL-urile reale (`www.siui.ro`, `sipe.siui.ro`, `*.e-pias.ro`, `api.openai.com`) apar doar ca șiruri de configurare/aserțiune cu backend-uri false (ex. `modules/siui/connection_error_test.go:22`, `modules/wisemedai/openai_test.go:32`).
- Testele live sunt condiționate de variabile de mediu, toate nesetate în această sesiune: `WAI_TEST_DATABASE_URL`, `WAI_BROWSER_TEST`, `SIUI_DATA_TEST_DATABASE_URL`, `SIUI_DATA_REAL_*`, `HISTORY_TEST_MYSQL_*`, `SIUI_TEST_PKCS11_*`, `SIUI_TEST_CONNECTED_TOKEN`, `SIUI_TEST_CNAS_TLS`, `REQUIRE_CHROME_TESTS`.
- `wsm-server/integration/roundtrip_test.go`: `t.TempDir()` (`:132`), port efemer pe `127.0.0.1:0` (`:137`), certificat TLS autogenerat valabil o oră (`:92-102`), server pe `127.0.0.1` (`:144`, `:164`).
- Niciun pachet Go nu a fost sărit.

### B.1 Matrice de verificare

| # | Comandă | Director | Rezultat | Demonstrează | NU demonstrează |
| --- | --- | --- | --- | --- | --- |
| 1 | `go build ./...` | `readersv3/` | **OK**, 24,3 s | Toate cele 101 pachete (inclusiv 32 de aplicații) compilează pentru darwin/arm64 | Cross-compilarea pentru Windows/Linux, build-ul prin `releasectl`, cod sub build tag-uri `windows`/`linux` |
| 2 | `go vet ./...` | `readersv3/` | **OK**, 4,2 s, fără avertismente | Lipsa problemelor detectabile de `vet` pe darwin | Fișierele `*_windows.go`, `*_linux.go` |
| 3 | `go test ./... -count=1 -timeout 15m` | `readersv3/` | **OK**: 33 pachete `ok`, 68 fără teste, 0 eșecuri; 18,9 s. 294 teste de nivel superior trecute, 243 subteste trecute, **60 sărite** | Logica acoperită de teste unitare cu backend-uri simulate (SIUI, WSS, parsere de protocol, config, runner) | Cele 60 de teste sărite: 31 `siuidata` și 26 `wisemedai` (cer PostgreSQL), 2 `siuihistory` (cer MySQL), 2 `siui` (token conectat, TLS CNAS). Nimic despre CNAS real, analizoare, imprimante, tablete de semnătură |
| 4 | `go test -race ./... -count=1 -timeout 20m` | `readersv3/` (tot modulul, nu doar subsetul cerut) | **OK**: 33 pachete `ok`, 0 curse raportate; 61,0 s | Absența curselor de date pe căile exercitate de teste | Curse pe căi neacoperite (27 din 51 de pachete `modules` nu au teste) |
| 5 | `go build ./...` | `wsm-server/` | **OK**, 1,4 s | Serverul, `wsmctl`, exemplul și copiile din `dist/` compilează | Build Linux/Docker |
| 6 | `go vet ./...` | `wsm-server/` | **OK**, 0,3 s | — | — |
| 7 | `go test ./... -count=1` | `wsm-server/` | **OK**: 3 pachete `ok` (`internal/config`, `internal/logstore`, `internal/server`), 8 fără teste; 4,8 s; 58 teste + 70 subteste, 0 sărite | Validarea configurației, stocarea logurilor, serverul (autentificare, rutare, admin, SDK) cu upstreamuri simulate | Backend WiseMED real, Nginx, încărcare de producție |
| 8 | `go test -race ./... -count=1` | `wsm-server/` | **OK**: 3 pachete `ok`, 0 curse; 7,4 s | Absența curselor pe căile testate | — |
| 9 | `go vet ./...` | `wsm-server/integration/` | **OK**, 0,2 s | — | — |
| 10 | `go test ./... -count=1 -v` | `wsm-server/integration/` | **OK**: 1 pachet, 30,3 s | Server WSM cu TLS real pe loopback + modulul reader din `readersv3`: apel API, tichet de control și reconectarea reală după ~30 s | Certificat public, DNS, proxy, mai multe readere, rețea reală. Nu a fost rulat cu `-race` (varianta din `verify.sh`) |
| 11 | `go list ./...` | rădăcină (`go.mod`, go 1.19) | **OK**: 1 pachet (`output/wsm-browser-sdk/examples/reader`) | Modulul din rădăcină nu mai conține cod sursă propriu | — |
| 11b | `go vet ./...` | rădăcină | **FAIL** (preexistent, cod): lipsesc `github.com/gorilla/websocket` și pachetul `…/wsm-server/internal/server` din modulul rădăcină | Singurul pachet al modulului rădăcină nu compilează în acest modul | Nu afectează `readersv3` sau `wsm-server` |
| 12 | `node --test readersv3/modules/localhttp/ui/*.test.cjs readersv3/apps/esignature-server/tests/*.test.cjs` | rădăcină | **OK**: 75 teste, 63 trecute, **12 sărite**, 0 eșecuri; 0,5 s | Logica UI locală (WSS, SIUI, formatter, history, wisemed-ai) executată în `node:vm` cu date simulate | Cele 12 teste DOM sărite (cer `jsdom`, neinstalat și nedeclarat în niciun `package.json`); comportament în browser real |
| 13 | `node --test wsm-server/internal/server/adminui/app.test.cjs wsm-server/internal/server/sdk/*.test.cjs` | rădăcină | **FAIL**: 51 teste, 50 trecute, **1 eșuat**; 0,3 s | SDK-ul de browser și consola de admin, cu transport simulat | Vezi eșecul E1 |
| 14 | `./node_modules/.bin/vitest run` | `readersv3/apps/wisemed-ai/web/` | **OK**: 3 fișiere, 10 teste; 1,2 s | Clientul API, facturarea și dictarea din frontend, cu `fetch` simulat | Integrarea cu backendul |
| 15 | `./node_modules/.bin/vue-tsc --noEmit` | `readersv3/apps/wisemed-ai/web/` | **OK**, 2,4 s | Typecheck-ul frontendului Vue/TypeScript | Build-ul Vite (ar scrie în `modules/wisemedai/web/dist`, nerulat) |
| 16 | `python3 -m unittest -v test_icd10_crosswalk` | `readersv3/scripts/` | **OK**: 4 teste | Parsarea expresiilor și potrivirea codurilor ICD-10 | Rularea pe cataloagele reale |
| 17 | `python3 readersv3/docker/test-update.py -v` | rădăcină | **FAIL**: 8 teste, 16 erori (toate subtestele); 20,7 s | Nimic pe acest host — vezi E2 | Comportamentul updaterului pe Linux cu bash ≥ 4 rămâne neverificat |

Neexecutate (cu motiv) — vezi și secțiunea finală:
- `python3 wsm-server/scripts/test-update-wsm.py` — **sărit ca riscant**: `update-wsm.sh:13-24` rulează `ssh-agent`, `ssh-add` și `sudo chown/chmod` reale înainte de orice validare, iar testul nu le simulează (simulează doar `docker git curl flock uname`).
- `readersv3/docker/test-images.py` — cere Docker și imagini construite.
- `wsm-server/verify.sh` — citit; este local (teste Go, `node --test`, integrare) și nu atinge `output/` sau `deployments/`. Nu a fost rulat ca script, pentru că toate componentele lui au fost rulate separat (rândurile 1–10, 12–13), cu două diferențe: integrarea a rulat fără `-race`, iar `go build ./apps/...` a fost acoperit de `go build ./...`.
- Playwright (`npm run test:e2e`, `readersv3/tests/ui/order-id-click.cjs`) — cer un browser Chrome și un server pornit pe `127.0.0.1:19120`.

### B.2 Eșecuri

**E1 — test SDK WSM: catalogul SIUI nu acoperă ruta `report/package`** — preexistent, **cod** (nu mediu).
- Test: `wsm-server/internal/server/sdk/siui-bridge.test.cjs:16-22` („catalog covers every explicit SIUI auxiliary route”).
- Output: `AssertionError [ERR_ASSERTION]: missing auxiliary path report/package`.
- Cauză: `readersv3/modules/siui/module.go:307` definește `route == "report/package"` (documentat și în `modules/siui/openapi.go:111`), dar `SIUIBridge.endpoints` din `wsm-server/internal/server/sdk/siui-bridge.js` nu conține această cale. Fișierele sunt curate în `git status`, deci desincronizarea este în codul comis.
- Efect: SDK-ul de browser nu expune endpointul `report/package` prin catalog — `INFERENȚĂ` (nu am verificat dacă poate fi apelat pe altă cale).

**E2 — `readersv3/docker/test-update.py`: toate cazurile eșuează pe macOS** — preexistent, **mediu**, cu o fragilitate reală în script.
- Output (extras): `update-siui-bridge.sh: line 78: sshkey\xe2: unbound variable`, urmat în test de `UnicodeDecodeError: 'utf-8' codec can't decode byte 0xe2`.
- Cauză: `readersv3/docker/update.sh:78` conține `"…using $sshkey…"`, cu caracterul `…` (U+2026) lipit de numele variabilei. `/bin/bash` 3.2.57 de pe macOS tratează primul octet al caracterului ca parte din nume și, sub `set -u`, raportează variabilă nedefinită. Confirmat cu un harness din scratchpad care a afișat stderr-ul brut.
- Pe acest host scriptul a ieșit totuși cu cod 0 după eroare (afișat de harness ca `RC 0`), deci eșecul nu ar fi vizibil în codul de ieșire.
- Nu am putut verifica pe bash ≥ 4 (nu există alt `bash` pe host). Ținta reală este Linux/EC2, unde comportamentul rămâne `NECUNOSCUT`. Folosirea `${sshkey}` ar elimina dependența de versiunea de bash.
- `docker/README.md:148` recomandă rularea acestui test local; pe un Mac cu bash-ul de sistem nu trece.

**E3 — `go vet ./...` în modulul rădăcină** — preexistent, cod: `output/wsm-browser-sdk/examples/reader/main.go:14-15` importă pachete care nu există în modulul `wisemed-labreaders` (go 1.19). Este o copie de exemplu livrată în `output/`, nu cod activ.

### B.3 Acoperire cu teste

`CONFIRMAT PRIN TEST LOCAL` (`go list -f '{{.ImportPath}} {{len .TestGoFiles}} {{len .XTestGoFiles}}' ./...`):

| Zonă | Pachete | Cu teste | Fără teste |
| --- | --- | --- | --- |
| `readersv3/apps` | 35 | 1 (`apps/runner`, 6 fișiere de test) | 34 (toate `main` + pachetele interne ale aplicațiilor) |
| `readersv3/core` | 3 | 2 | 1 (`core/module`) |
| `readersv3/modules` | 51 | 24 | 27 |
| `readersv3/shared` | 10 | 4 | 6 |
| `readersv3/tools` | 2 | 2 | 0 |
| **`readersv3` total** | **101** | **33** | **68** |
| `wsm-server` | 11 | 3 (`internal/config`, `internal/logstore`, `internal/server`) | 8 (`cmd/wsm-server`, `cmd/wsmctl`, `examples/reader`, 5 copii din `dist/`) |
| `wsm-server/integration` | 1 | 1 | 0 |

Zone critice fără niciun test Go:
- **Transporturi către analizoare**: `modules/transports/serial`, `modules/transports/tcpip`, `modules/transports/file`.
- **Protocoale de analizor** (10 pachete): `anafdocsmart`, `analytikjenaplasmaquantmselite`, `anatoliageneworks`, `beoslcsv`, `biosanhipompp96`, `cary60uvvis`, `gammavision`, `genericfile`, `irbiotyper`, `tricarb5110tr`; plus `modules/astmspecimen`.
- **Flux de laborator**: `analytes`, `analytemanagement`, `qc`, `stats`, `dailyorders`, `dailydetails`, `dashboard`, `events`.
- **Securitate și acces local**: `modules/login`, `shared/bindguard`.
- **Update**: `shared/appupdates` (clientul de update și verificarea semnăturii HMAC); aplicarea update-ului din `apps/runner/update_apply.go` nu are test dedicat.
- **Imprimare**: `modules/barcodeprinter`.
- **Diagnostic**: `shared/commtrace`, `shared/debugreplay`, `shared/analyzeractivity`.
- **Instalarea ca serviciu**: `apps/runner/service_*.go` nu are teste; `tools/releasectl` are 3 fișiere de test pentru 2467 de linii.
- **WSM**: `cmd/wsm-server` (pornire, reload SIGHUP) și `cmd/wsmctl` (generare de chei și tokenuri) nu au teste proprii.

Teste care există dar nu rulează fără infrastructură: 57 de teste PostgreSQL (`siuidata`, `wisemedai`), 2 MySQL (`siuihistory`), 2 de token/TLS CNAS (`siui`), 12 teste DOM (`jsdom`), 10 fișiere Playwright `*.e2e.ts`.

---

## Contradicții cod ↔ documentație

1. **Asset-uri de control UI lipsă din imaginea Docker WSM.** `wsm-server/Dockerfile:16` și `Dockerfile.dockerignore:14-17` copiază patru fișiere (`index.html`, `app.js`, `styles.css`, `wss-remote.js`). `build.sh:15` și `prepare-local.sh:18` copiază șase (în plus `docmed-forms.js`, `siui-invoice.js`). `app.js:1-2` importă static `/siui-invoice.js` și `/docmed-forms.js`, iar serverul întoarce 404 pentru un fișier absent (`internal/server/control.go:116-124`). `docs/operations.md:87-88` afirmă că Dockerfile copiază „aceleași patru fișiere” ca `build.sh`. Efect probabil: în imaginea Docker, consola de control nu se încarcă, pentru că importurile de modul eșuează — `INFERENȚĂ` (neverificat într-un container).
2. **`config.http.yaml` contrazice ghidul.** `docs/operations.md:24-26` descrie fișierul cu `allow_insecure_http: true`; fișierul real are `allow_insecure_http: false` (`deployments/config.http.yaml:8`), cu `address: 0.0.0.0` și fără bloc `tls`. Dacă `-check-config` îl respinge în această formă — `NECUNOSCUT` (nerulat).
3. **Loguri WSM.** `docs/migration.md:23` spune că logurile au trecut „în stderr/journald”; `docs/operations.md:199-205` și codul (`internal/logstore/store.go:52`) scriu și fișiere zilnice.
4. **`update-wsm.sh` față de `docs/docker-update.md` și față de propriul test.** Documentul spune că se ignoră cheile din agent (`:7-8`) și că o cheie lipsă oprește scriptul înainte de orice operație (`:9-10`). Scriptul pornește însă un `ssh-agent`, face `ssh-add` și rulează `sudo chown/chmod` pe `deployments` și `state` în liniile `13-24`, înainte de `set -Eeuo pipefail` (`:27`) și de validarea argumentelor și a cheii (`:30-33`). Agentul pornit nu este oprit nicăieri. Testul (`scripts/test-update-wsm.py:1`) se declară „no Docker daemon or SSH required”, dar nu simulează `ssh-agent`, `ssh-add` sau `sudo`.
5. **`make-wsm-docker.sh`: cod mort după `exec`.** Linia 7 face `exec ./wsm-server/build-docker.sh`; liniile 8–12 (`cp update-wsm.sh`, `git add .`, `git commit -m "Latest"`, `git push`) nu se execută niciodată. `docker/README.md:25` spune explicit că build-ul local nu face commit/push — adevărat doar datorită acestui `exec`.
6. **CI încarcă un director care nu conține installerele Windows.** Workflow-ul urcă `readersv3/release` (`readersv3-release.yml:69-73`), dar `packageWindows` scrie în `dist/installers/` (`main.go:665`). Pe runnerul Windows artefactul ar fi gol — `INFERENȚĂ`. În plus, `package-all` nu produce arhivele de update (`dist/updates`), iar `build/README.md:10` spune că artefactele finale sunt în `dist/updates`, `dist/installers` și `dist/releases`.
7. **CI: matrice și unelte nefolosite.** Câmpul `packages` din matrice (`:39-43`) nu este folosit; pe Windows omite `windows-386`, pe care `package-all` îl construiește pentru `esignature-server`. Se instalează `wixtoolset` (`:62`), dar codul folosește numai NSIS; fișierele `.wxs` și `.winsw.xml` sunt generate fără a fi consumate, iar `ensureWinSW` (`main.go:1380-1410`, descărcare de pe GitHub) nu este apelată de nicăieri.
8. **`build/apps.json` este în urmă.** Conține 28 de aplicații; există 32 de `apps/*/main.go`. Lipsesc `siui-bridge`, `siui-bridge-history`, `wisemed-ai`, `wisemed-siui-data`. `zonci-xl1000i-reader` este în manifest, dar nu are director `apps/…/build`. Fișierul este urmărit în git deși `.gitignore` îl exclude. Jobul `sync` din CI (`git diff --exit-code`) ar eșua pe starea curentă — `INFERENȚĂ` (`sync` nu a fost rulat, pentru că scrie în repo).
9. **Două definiții diferite ale serviciului pentru aceeași aplicație.** Pachetele `releasectl` folosesc `serviceName` din id-ul aplicației și bundle id cu puncte (ex. `eu.wisemed.readersv3.generic.test.reader`); `-installservice` folosește `wisemed-<reader.id din config>` și eticheta `eu.wisemed.readersv3.<token>` cu cratime (`service_darwin.go:20`). Cele două căi pot instala servicii cu nume diferite pentru același binar.
10. **`-headless` în unitățile de serviciu împachetate.** Unitățile generate rulează `… -config deployments/config.yaml -headless` (`apps/generic-test-reader/build/linux/generic-test-reader.service:10`, plist-ul macOS, `.winsw.xml`), cu `Type=simple`, respectiv `KeepAlive`. În modul `-headless` procesul părinte pornește un copil și iese cu succes (`runner.go:126-139`). Sub systemd, ieșirea procesului principal oprește serviciul și, cu `KillMode` implicit, copilul este terminat; sub launchd, `KeepAlive` repornește părintele la nesfârșit. Calea `-installservice` nu adaugă `-headless` (`service_linux.go:28`). Probabil pachetele `.deb/.rpm/.pkg` nu țin serviciul pornit — `INFERENȚĂ`, neverificat pe Linux/macOS; merită testat înainte de orice livrare prin aceste pachete.
11. **`docs/verification.md`** (datat 22 septembrie 2026) raportează toate verificările PASS, cu „9 teste WSS + 12 teste transport remote”. Azi suita Node are 75 + 51 de teste, cu un eșec (E1). Documentul menționează și calea `wsm-server/discu ct/` (`:28`), care nu există. Timpul de integrare (~32 s) se confirmă (30,3 s).
12. **Documentația din rădăcină este istorică.** `Readme.md:50-60` descrie `./implementation/<analizor>` și `config.db`; directorul `implementation/` nu mai există (apare doar ca ștergeri în `git status`). `Service.md` și `InstallPI.md` sunt șabloane generice (unit systemd cu `Restart=always`, instalare `golang` pe Raspberry Pi) fără legătură cu `releasectl` sau `-installservice`.
13. **Versiuni Go neuniforme.** `go.mod` declară `go 1.24.0`; Dockerfile-urile folosesc `golang:1.26.8` (SIUI, WSM), `1.24-bookworm` (wisemed-ai), `1.25-bookworm` (html-print-formatter) și Go 1.24.0 instalat manual (update-server); CI folosește versiunea din `go.mod`.
14. **Port 19113 folosit de două aplicații.** `html-print-formatter` expune 19113 (`apps/html-print-formatter/docker/ubuntu/Dockerfile:57`), același port ca UI-ul implicit `siui-bridge-history` (`docker/README.md:31`), deși README afirmă că porturile utilitarelor sunt distincte (`:38`, afirmație limitată la WSM și cele trei utilitare SIUI). Conflict doar dacă rulează pe același host.

---

## Necunoscute / neexecutate

Neexecutate deliberat:
- Orice build real: `releasectl build/package/release/sync`, `make`, `build-all-*`, `package-all-*`, `scripts/*.sh`, `docker/build.sh`, `make-*-docker.sh`, `wsm-server/build.sh`, `build-docker.sh`, `prepare-local.sh` — interzise și/sau scriu în `dist/`, `output/`, `build/`.
- `update-wsm.sh`, `docker/update.sh` pe un host real; `wsm-server/scripts/test-update-wsm.py` (apelează `sudo`, `ssh-agent`, `ssh-add` reale); `docker/test-images.py` (Docker).
- Teste Playwright și testul Go `TestBrowser*` (`WAI_BROWSER_TEST`), testele PostgreSQL/MySQL, testele cu token PKCS#11/Keychain și TLS CNAS.
- `vite build` (scrie în `modules/wisemedai/web/dist`), `go test -race` pe `wsm-server/integration`.

Necunoscute:
- Dacă installerele NSIS, pachetele `.deb/.rpm/.pkg/.dmg` și update-urile `.zip` produse azi se instalează și pornesc corect pe Windows/Linux/macOS — nimic din acestea nu a fost construit sau instalat.
- Comportamentul serviciilor împachetate cu `-headless` (contradicția 10) și al serviciului Windows la update automat.
- Dacă arhiva de update este semnată criptografic; în cod am găsit doar compararea opțională a SHA-256 primit de la același server.
- Comportamentul `docker/update.sh` și `update-wsm.sh` pe bash ≥ 4 / Linux; starea reală a containerelor și a repository-ului de arhive de pe EC2.
- Dacă imaginea Docker WSM aflată în producție are consola de control funcțională (contradicția 1).
- Dacă `config.http.yaml` trece `-check-config` (contradicția 2).
- Dacă fișierele `config.yaml` și `serverkey.pem` urmărite în git conțin secrete reale — conținutul nu a fost inspectat sau copiat.
- Dacă workflow-ul GitHub a rulat vreodată cu succes (nu am acces la istoricul Actions).
- Rotația logului zilnic al readerelor la procese de lungă durată (A.1) — dedusă din cod, neconfirmată prin rulare.
- Niciun test nu confirmă comunicarea cu analizoare, imprimante, tablete Signotec, token-uri CNAS sau cu serviciile CNAS/SIUI reale: toate testele trecute folosesc backend-uri simulate.
