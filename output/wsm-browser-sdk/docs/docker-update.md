# Actualizare Docker pe EC2 din repository-ul de arhive

Copiați o singură dată scriptul `update-wsm.sh` din rădăcina proiectului pe EC2.
Nu sunt necesare sursele WSM. Rulați ca utilizatorul care are cheia SSH pentru
`git@github.com:AlwaysSmarter/dockers.git` (de exemplu ec2-user), nu cu sudo în fața
întregului script. Scriptul folosește sudo pentru Docker dacă este necesar.

Prerechizite: Docker pornit, git, python3, curl, gzip și flock (util-linux), acces SSH
la repository și cheia host GitHub verificată în known_hosts. Nu dezactivează
verificarea SSH. Git LFS necesită clientul git-lfs dacă arhivele sunt stocate în LFS.

Configurația inițială rămâne responsabilitatea operatorului:
- `/srv/wsm/deployments/config.yaml` și fișierele de chei/certificate referite;
- `/srv/wsm/state/` persistent, inscriptibil de UID/GID 65532;
- WSM ascultă în container pe 0.0.0.0:8090 HTTP, fără TLS, cu explicit allow_insecure_http;
- device_keys_file: `/var/lib/wsm-server/device-keys.yaml`;
- control_ui_dir: `/opt/wsm-server/control`;
- Nginx pe host cu HTTPS și upstream 127.0.0.1:8090, conform ghidului Docker.

```sh
chmod +x update-wsm.sh
./update-wsm.sh
# Sau o versiune explicită, pentru upgrade ori downgrade:
./update-wsm.sh 20260924-160000
```

Implicit selectează cea mai nouă arhivă `wisemed-wsm-YYYYMMDD-HHMMSS.tar.gz` din
branch-ul implicit al repository-ului (inclusiv subdirectoare). Necesită tagul
intern `wisemed-wsm:YYYYMMDD-HHMMSS`, produs de scriptul standard make-wsm-docker.sh.
Refuză nume duplicate pentru aceeași versiune, arhive invalide, arhitectură greșită
și versiune binar diferită. Nu face build pe server.

Încarcă imaginea, o fixează după image ID și validează configurația înainte să
oprească serverul vechi. Redenumește și oprește containerul vechi, pornește noul
container cu aceleași căi standard și verifică /healthz timp de maximum 60 secunde.
La eșec încearcă automat revenirea la containerul vechi și verifică sănătatea lui.
Dacă și rollback-ul eșuează, afișează explicit eroarea. La prima instalare eșuată
elimină noul container. O versiune identică deja sănătoasă nu este repornită.

După succes, containerul precedent rămâne oprit sub numele afișat
`wsm-server-backup-...`, pentru revenire manuală. Nginx și fișierele de configurare
nu sunt modificate. Datele nu sunt șterse sau restaurate automat; păstrați separat
backup-uri pentru configurație și registry, în special înainte de migrări viitoare.
Imaginile/containerele vechi nu sunt eliminate automat. Verificarea healthz este
locală și nu înlocuiește verificarea domeniului public sau a unui reader real.

Opțiuni prin mediu: WSM_DATA_DIR (implicit /srv/wsm), WSM_CONTAINER_NAME
(wsm-server), WSM_HOST_PORT (8090), WSM_HEALTH_TIMEOUT (60), WSM_UPDATE_CACHE
(~/.cache/wsm-update). Folosiți un singur utilizator/cache pentru deployment;
flock previne actualizări simultane în acel cache. Arhivele temporare clonate sunt
șterse după operație. Rulați cu aceleași opțiuni la fiecare actualizare.

Verificări locale ale scriptului: `python3 wsm-server/scripts/test-update-wsm.py`.
Folosesc Git/Docker simulate, fără acces la AWS sau la repository-ul privat.
