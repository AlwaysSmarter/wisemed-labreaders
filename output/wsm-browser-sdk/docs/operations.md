# Instalare și operare WSM Server

## Arhitectura de deployment

Un singur proces central, mai multe instanțe WiseMED izolate logic. Starea hubului
este în RAM. Pentru mai multe replici ar trebui implementat un bus/registry comun;
nu puneți două replici independente în load balancer presupunând că vor comunica.

TLS poate fi terminat direct de server folosind certificat + cheie PEM externe.
Certificatul conține fullchain și numele DNS public. Cheia privată nu are parolă
interactivă și trebuie protejată prin permisiuni de fișier. Nu sunt generate sau
înlocuite automat certificatele operatorului.

Alternativ, proxy TLS local, cu serverul pe loopback numeric:

```yaml
server:
  address: 127.0.0.1
  port: 8090
  allow_insecure_loopback: true
```

În acest mod omiteți `tls`. Pentru Nginx într-un container separat, folosiți
`deployments/config.http.yaml`: `address: "0.0.0.0"`, `port: 8090`,
`allow_insecure_http: true`, fără bloc `tls`. Acest opt-in permite HTTP/WS pe
interfața containerului; portul trebuie accesibil doar proxy-ului în rețeaua privată.
Nu publicați `8090` pe interfețele publice. Dacă Nginx este pe host și WSM în Docker,
publicați numai `-p 127.0.0.1:8090:8090`. Dacă ambele sunt containere, conectați-le
la aceeași rețea privată și nu publicați portul WSM; upstream-ul Nginx devine
`http://wsm-server:8090`.

`allow_insecure_http` este implicit false; simpla eliminare a certificatului nu
dezactivează protecția TLS. Dacă blocul `tls` este prezent, certificatul și cheia
trebuie în continuare să fie valide și serverul folosește TLS.

Exemplu Nginx complet pentru secțiunea `http {}`: `deployments/nginx-wsm.conf`.
Adaptați hostname-ul, certificatul și cheia la domeniul real și validați cu `nginx -t`.
Configurația păstrează Host cu portul public și transmite Upgrade/Connection conform
[documentației oficiale Nginx](https://nginx.org/en/docs/http/websocket.html).
Clienții folosesc în continuare `wss://domeniul-public/ws`; doar legătura
Nginx → WSM folosește HTTP/WS. Issuer-ul JWT nu trebuie schimbat automat după
adresa backendului; allowed_origins trebuie să conțină originile HTTPS reale ale
interfețelor care se conectează cu JWT. Fereastra cu tichet trebuie servită de pe
originea HTTPS publică WSM.

Serverul nu se bazează pe `X-Forwarded-Proto` pentru a permite plaintext. Proxy-ul
public trebuie să valideze TLS, să transmită Upgrade/Connection și subprotocolurile
WebSocket, să păstreze headerul Host public (inclusiv portul) pentru verificarea
originii consolei `/control/`, să aibă timeout peste heartbeat și să aplice limite la conexiuni/handshake
neautentificate. Nu logați Authorization, Sec-WebSocket-Protocol sau query-uri JWT.

## Instalare Linux/systemd

Construiți pe Linux, sau cross-compile pentru arhitectura serverului:

```sh
cd wsm-server
go test -race ./...
go vet ./...
GOOS=linux GOARCH=amd64 WSM_OUTPUT_DIR=dist/linux-amd64 ./build.sh
```

`./build.sh` fără override produce `dist/`; exemplul de mai sus produce
`dist/linux-amd64/`. În comenzile de instalare de mai jos, `dist/` desemnează
pachetul construit pentru serverul țintă. Pachetul nu include chei, certificate, vechea configurație sau UI de test. Pe server,
creați utilizatorul dedicat și copiați fișierele (comenzi pentru administrator):

```sh
sudo useradd --system --home /nonexistent --shell /usr/sbin/nologin wsm-server
sudo install -d -o root -g wsm-server -m 0750 /etc/wsm-server /etc/wsm-server/keys /etc/wsm-server/tls
sudo install -d -o root -g root -m 0755 /opt/wsm-server/bin
sudo install -m 0755 dist/bin/wsm-server dist/bin/wsmctl /opt/wsm-server/bin/
sudo cp -R dist/control /opt/wsm-server/control
sudo install -o root -g wsm-server -m 0640 dist/deployments/config.production.yaml /etc/wsm-server/config.yaml
sudo install -m 0644 dist/deployments/wsm-server.service /etc/systemd/system/wsm-server.service
```

Adaptați instanțele/DNS/porturile în config înainte de pornire. Copiați certificatul
și cheia existente la căile din config; procesul `wsm-server` trebuie să le poată
citi. Pentru fișierele din `/etc/wsm-server/tls`, folosiți root:wsm-server, 0640 și
directoare 0750. Pentru symlink-uri de la un client ACME, verificați accesul pe tot
traseul sau folosiți un deploy hook care copiază fișierele și reîncarcă serviciul.
Nu dezactivați ProtectSystem/ProtectHome doar pentru a evita configurarea accesului.

## Docker fără repository în runtime

Interfața nu este încorporată în binarul Go. Pachetul `build.sh` include `control/`,
iar Dockerfile copiază aceleași patru fișiere în imagine la `/opt/wsm-server/control`.
Sursele readersv3 sunt necesare doar la build. Calea relativă din configurația
locală GoLand este exclusiv pentru dezvoltare și nu se folosește în container.

Din rădăcina repository-ului:

```sh
docker build -f wsm-server/Dockerfile -t wisemed-wsm:local .
```

Pregătiți pe host `/srv/wsm-server/config.yaml` pornind de la
`deployments/config.production.yaml`, plus certificatele și cheile referite de el.
Valorile relevante pentru container sunt:

```yaml
server:
  address: "0.0.0.0"
  port: 8443
  control_ui_dir: /opt/wsm-server/control
  tls:
    cert_file: /etc/wsm-server/tls/fullchain.pem
    key_file: /etc/wsm-server/tls/privkey.pem
```

Acesta este doar fragmentul `server`; păstrați și configurați `security` și
`tenants`. Toate căile de certificat/secret trebuie să fie valabile în container.
Imaginea rulează ca UID/GID 65532; fișierele montate trebuie să fie citibile de
acest utilizator, de exemplu directoare root:65532 cu 0750 și fișiere cu 0640.

```sh
docker run --rm --read-only \
  --mount type=bind,src=/srv/wsm-server,dst=/etc/wsm-server,readonly \
  wisemed-wsm:local -config /etc/wsm-server/config.yaml -check-config

docker run -d --name wsm-server --restart unless-stopped --read-only \
  -p 8443:8443 \
  --mount type=bind,src=/srv/wsm-server,dst=/etc/wsm-server,readonly \
  wisemed-wsm:local
```

Nu montați repository-ul sau directorul UI. Imaginea include interfața și CA-urile
publice pentru conexiunile HTTPS către WiseMED; configurația și cheile vin din
volumul extern. Contextul de build exclude explicit output, dist și secretele.
Reload chei/certificat: `docker kill --signal=HUP wsm-server`; modificările de
listener/limite cer restart. Actualizarea interfeței cere rebuild și înlocuirea
imaginii. Folosiți o singură replică, deoarece registrul WSS este în memorie.

## Provisionarea și managementul cheilor

Provisionarea din browser este disponibilă la `/admin/`, cu login WiseMED și
`user_type == -1`. Comenzile `add`, `list`, `key` folosesc un registry YAML extern
inscriptibil; configurația principală și secretele de backend rămân read-only.
Vedeți [consola administratorului](admin-console.md) pentru activare și mount-ul
suplimentar `/var/lib/wsm-server`. Fără acel mount persistent, administrarea din
browser nu poate salva cheile într-un container cu filesystem read-only.

O cheie HMAC nouă = 32 bytes aleatori, stocați ca text base64url. Se folosește
**textul exact** drept secret HMAC; emitentul JWT nu îl decodează base64.
`wsmctl keygen` creează fișier 0600 și refuză suprascrierea:

```sh
sudo /opt/wsm-server/bin/wsmctl keygen -out /etc/wsm-server/keys/clinic-a.key
sudo /opt/wsm-server/bin/wsmctl keygen -out /etc/wsm-server/keys/clinic-b.key
sudo chown root:wsm-server /etc/wsm-server/keys/clinic-a.key /etc/wsm-server/keys/clinic-b.key
sudo chmod 0640 /etc/wsm-server/keys/clinic-a.key /etc/wsm-server/keys/clinic-b.key
```

Distribuiți secretul fiecărei instanțe numai backendului ei, prin mecanismul privat
de deployment/secrets. Nu puneți cheia principală în JavaScript, browser, URL,
comandă shell, git sau reader. Backendul emite tokenuri pentru utilizatori/readere.
Configurația poate avea mai multe `kid` pentru același tenant (rotație ori roluri
separate). Pentru un key dedicat readerelor, limitați `roles: [reader]` și scopes
la cele necesare. Cheia de backend reprezintă autoritatea de semnare pentru acele roluri și nu se
distribuie readerelor. Modul `device_key` folosește o cheie diferită per echipament,
cu `subject`, `reader_id`, `equipment_id`, roluri și scope-uri restricționate în WSM;
vezi [provisionarea echipamentelor](remote-control.md). Nu reutilizați cheia
principală a tenantului pentru acest mod.

Fiecare intrare activă cere exact unul dintre:

```yaml
secret_file: /etc/wsm-server/keys/clinic-a.key
# sau:
secret_env: CLINIC_A_WSM_SECRET
```

Căile relative sunt rezolvate față de directorul configului. Valorile secrete sunt
curățate de whitespace la capete; nu sunt acceptate chei mai scurte de 32 bytes.
Cheile identice între tenanturi diferite sunt respinse. Nu există chei hardcodate,
fallback global sau generator de tokenuri accesibil prin HTTP.

Exemplu complet cu două instanțe: `deployments/config.production.yaml`.
`tenants.<id>.disabled: true` dezactivează o instanță; `security.keys.<kid>.disabled:
true` dezactivează cheia. Configurația trebuie să păstreze cel puțin o cheie activă;
pentru oprirea tuturor clienților, opriți serviciul.

## Validare, pornire, loguri

```sh
sudo -u wsm-server /opt/wsm-server/bin/wsm-server -config /etc/wsm-server/config.yaml -check-config
sudo systemctl daemon-reload
sudo systemctl enable --now wsm-server
sudo systemctl status wsm-server
sudo journalctl -u wsm-server -f
```

`-check-config` încarcă schema strictă, secretele și certificatul/cheia. Respinge
câmpuri necunoscute, URL-uri WiseMED non-HTTPS și origini browser neexplicite.
Fișierele TLS nu sunt emise/renewate de WSM. Operatorul verifică lanțul public și
expirarea, DNS/firewall, încrederea clienților și hostname-ul.

Logurile merg la stderr/journal (UTC); nu se mai scriu automat lângă config și nu
se loghează payloaduri medicale, JWT-uri sau query-uri. `-showlog` este păstrat doar
ca flag compatibil, fără efect asupra destinației. Retenția este gestionată de
journald/platformă. `GET /healthz` verifică procesul, nu upstreamurile.

## Rotație, revocare, reîncărcare certificat

1. Generați o cheie nouă într-un fișier nou; adăugați un `kid` nou la același tenant.
2. Validați configul cu `-check-config` ca utilizatorul serviciului.
3. `sudo systemctl reload wsm-server` (SIGHUP).
4. Backendul WiseMED începe să emită tokenuri cu noul kid/secret.
5. După expirarea tokenurilor vechi, eliminați/dezactivați vechiul kid și reload.

La compromitere: dezactivați cheia imediat și reload; nu așteptați expirarea.
La revocarea întregii instanțe: disabled pe tenant și reload.

Orice reload reușit înlocuiește atomic snapshotul și **deconectează toate socketurile**,
inclusiv din celelalte instanțe și cele aflate înainte de hello. Astfel nu există
sesiuni cu drepturi revocate rămase deschise. Clienții trebuie să se reconecteze cu
JWT valid și backoff. Rotația cu suprapunere păstrează validitatea tokenurilor vechi,
dar nu păstrează socketurile deschise. Acesta este un compromis operațional explicit.

La înlocuirea certificatului/fullchain/key, validați fișierele și folosiți tot
reload; noile handshakes primesc noul certificat. Conținutul fișierelor nu este citit
la fiecare handshake. Config invalid/certificat invalid = reload respins, configurația
anterioară continuă. Schimbarea listenerului, modului TLS sau a limitelor din
`server` necesită restart și este respinsă la reload; certificatele pot fi schimbate.

Environment-ul procesului este cel de la pornire. Schimbarea EnvironmentFile din
systemd necesită restart, nu doar reload. Pentru rotație fără restart folosiți
fișiere externe. Pe Windows nu există SIGHUP în acest program: aplicați prin restart.

## Emiterea unui token pentru verificare

`wsmctl token` este unealtă offline de operator, nu un serviciu de autentificare.
În producție, backendul WiseMED îndeplinește acest rol după autentificare/autorizare.
Nu transmiteți tokenul rezultat în loguri sau capturi publice.

```sh
/opt/wsm-server/bin/wsmctl token \
  -secret-file /etc/wsm-server/keys/clinic-a.key \
  -kid clinic-a-2026-09 -tenant clinic-a \
  -issuer https://clinic-a.example.ro \
  -subject user-123 -client browser-session-456 -role browser \
  -scopes route:command,route:broadcast,topics:subscribe,connections:read,server:status \
  -ttl 5m
```

`-audience` implicit `wsm-server`; readerul mai cere `-role reader -reader reader-1 -equipment 42`.
CLI validează forma locală, dar nu cunoaște scopes/roluri/TTL maxime ale serverului;
serverul face validarea finală. Cheia se citește din fișier, nu din argument.

## Limite și monitorizare

Default: 2000 conexiuni globale, 200/tenant, 128 mesaje în coadă/conexiune, 1 MiB
mesaj primit, 64 topicuri/conexiune, token bucket 50 mesaje/secundă cu burst 100,
hello 10s, ping 25s, read timeout 60s, write timeout 5s. Limitele de conexiuni includ
socketurile autentificate care încă nu au făcut hello.

Dacă se umple coada, crește `total_dropped` și clientul lent este închis. Contoarele
`total_accepted`, `total_closed`, `total_dropped` și conexiunile sunt accesibile cu
JWT `connections:read`, numai pentru tenantul lui. Se resetează la restart.
Ajustați limitele după RAM și traficul real: limitele implicite nu constituie o
promisiune de capacitate pentru 2000 clienți cu mesaje mari simultan.
Template-ul de producție setează explicit `max_message_bytes: 8388608` (8 MiB),
spre deosebire de defaultul de 1 MiB atunci când câmpul lipsește. Valoarea de 8 MiB
acoperă corpul maxim de 4 MiB al adaptorului API și overhead-ul base64/JSON.

Testele includ 50 de socketuri WSS, nu un benchmark de producție. Faceți un test de
încărcare cu volumele reale înainte de lansare. Edge proxy/firewall trebuie să limiteze
handshake-urile neautentificate și traficul abuziv înainte de aplicație. Dacă aveți
cerințe de audit persistent sau livrare garantată, adăugați stocare/broker și
idempotency în aplicațiile care execută comenzile.

## Referințe tehnice

Validarea JWT utilizează API-urile documentate în
[golang-jwt/jwt v5](https://pkg.go.dev/github.com/golang-jwt/jwt/v5).
Shutdown-ul socketurilor este explicit deoarece
[net/http Shutdown](https://pkg.go.dev/net/http#Server.Shutdown) nu închide conexiunile
WebSocket hijacked. TLS folosește
[crypto/tls](https://pkg.go.dev/crypto/tls#Config) și certificatul reîncărcat atomic.
