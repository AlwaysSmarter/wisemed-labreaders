# Consola administratorului WSM

Deschideți `https://<domeniu-wsm>/admin/`. Consola oferă comenzi pentru aparate, browsere/backenduri și setările tenantului;
nu este un shell al sistemului de operare și nu execută programe sau comenzi OS.

Autentificarea se face cu utilizatorul și parola WiseMED pentru tenantul selectat.
WSM trimite login-ul doar la API-ul HTTPS configurat pentru acel tenant. Accesul
este permis exclusiv dacă răspunsul autentificat conține `user_type == -1` și un
token de login; rolul nu este acceptat din browser. Utilizatorii
cu tip 0, alte valori sau tip lipsă sunt refuzați. WSM nu salvează parola sau
tokenul de login WiseMED. Sesiunea locală este temporară și are cookie HttpOnly;
în producție se folosește HTTPS, inclusiv când Nginx termină TLS.

## Browsere, backenduri PHP și setări tenant

După autentificare, comenzile se aplică exclusiv tenantului selectat:

| Comandă | Efect |
| --- | --- |
| `tenant` | Afișează issuer, URL-ul API WiseMED, originile permise și cheia API mascată `abc***xyz`. |
| `tenant edit` | Deschide formularul de modificare issuer/API URL/cheie API. Cheia goală păstrează valoarea curentă. |
| `browsers` / `browser list` | Listează cheile browser, inclusiv cele din YAML (read-only), cu secrete mascate. |
| `browser add wisemed-web https://php.example.ro` | Generează o cheie browser/backend și adaugă originea în tenant. |
| `browser key wisemed-web` | Afișează separat cheia browser din tenantul curent (consolă sau YAML) și configurația pentru PHP. |
| `browser edit wisemed-web https://new.example.ro` | Schimbă originea înregistrată și permite noua origine, păstrând cheia. |
| `browser rotate wisemed-web` | Generează un secret nou; cel vechi este revocat imediat. |
| `browser rights wisemed-web` | Deschide editorul de drepturi; folosește `toggle`, `info`, `save`, `cancel`. |
| `browser delete wisemed-web` | Revocă cheia gestionată; originea rămâne pentru celelalte chei. |
| `origin add https://php.example.ro` | Adaugă o origine HTTPS exactă, fără cale, query sau fragment. |
| `origin delete https://php.example.ro` | Elimină originea, inclusiv dacă provenea din YAML. |

Cheile generate permit rolurile `browser` și `service`; PHP păstrează secretul și
emite JWT-uri pentru utilizatorii autorizați. JavaScript primește numai JWT-ul.
`api:admin` nu este acordat implicit: activează-l explicit prin `browser rights`
pentru operații administrative precum validarea SIUI și emite-l în PHP numai
utilizatorilor autorizați. Drepturile și originile sunt politici ale tenantului;
originea înregistrată pe browser este metadata, nu o restricție suplimentară pe
cheie. Schimbarea ei nu elimină automat originea veche: folosește `origin delete`.
Cheile statice existente sunt vizibile; modificarea/revocarea lor rămâne în YAML.

În liste și în `tenant`, cheile apar numai cu primele și ultimele trei caractere,
separate de `***`; secretele prea scurte apar doar ca `***`. Cheia completă a unui
browser este disponibilă numai în panoul temporar: add/rotate pentru cheile gestionate și browser key pentru orice cheie browser a tenantului.
Cheia API WiseMED se introduce prin formularul `tenant edit`, niciodată ca argument
al unei comenzi păstrate în istoric. Ea trebuie să corespundă celei acceptate de
backendul PHP; WSM nu modifică automat configurația PHP. La schimbarea issuer-ului,
actualizează și emitentul JWT din PHP/readere, deoarece valoarea `iss` trebuie să
coincidă exact cu noua valoare.

Setările se aplică imediat și sunt salvate atomic, cu permisiuni `0600`, în
`security.device_keys_file`, alături de aparate: `browsers`, `tenant_origins`,
`tenant_settings`. Nu trebuie editat YAML-ul principal sau repornit WSM la fiecare
modificare. Registrul trebuie să fie într-un volum persistent, inscriptibil de WSM;
modificările rămân valabile la reload/restart și suprascriu setările tenantului din
YAML. Versiunile WSM anterioare nu înțeleg secțiunile noi ale registrului.

Eliminarea unei origini, revocarea/rotirea unei chei și salvarea drepturilor
închid conexiunile tenantului și invalidează tichetele sale pentru reautentificare.
Modificarea setărilor prin `tenant edit` invalidează și sesiunile administrative
ale acelui tenant: autentifică-te din nou, folosind API-ul WiseMED actualizat.

## Comenzi

| Comandă | Efect |
| --- | --- |
| `help` | Afișează comenzile disponibile. |
| `add 1` | Înregistrează ID-ul de echipament 1 și generează cheia sa WSS. |
| `add 1 horiba-yumizen-h500-reader-v3` | Înregistrează și restricționează cheia la ID-ul readerului indicat. |
| `list` | Afișează aparatele înregistrate prin consolă în tenant, inclusiv cele offline. |
| `key 1` | Afișează cheia aparatului 1 pentru copiere în reader/utilitar. |
| `edit 1 horiba-yumizen-h500-reader-v3` | Adaugă sau înlocuiește restricția reader-id, păstrând cheia. |
| `edit 1 --remove-reader` | Elimină restricția reader-id; tenantul și equipment_id rămân obligatorii. |
| `delete 1` | Șterge înregistrarea WSS și revocă cheia aparatului. |
| `rights 1` | Afișează drepturile numerotate, cu explicații și bife. |
| `info 3 7` | Afișează explicația extinsă și exemplul pentru unul sau mai multe numere din lista `rights`, fără a modifica selecția. |
| `toggle 7` | Bifează/debifează dreptul 7 în selecția curentă. Acceptă mai multe numere. |
| `save` / `cancel` | Salvează / abandonează selecția drepturilor. |
| `history` | Afișează istoricul comenzilor, numerotat cronologic. |
| `clear` | Curăță afișajul consolei; păstrează istoricul comenzilor și aparatele. |
| `logout` | Închide sesiunea de administrator. |

### Chei pentru SIUI/service

În consola WSM din `/admin/`, comenzile sunt aceleași pentru un reader și un
service de echipament. WSM actualizat permite rolurile JWT `reader` și `service`
pentru fiecare cheie generată; rolul efectiv este trimis de aplicație (`service`
pentru SIUI Bridge). Nu există o comandă separată `add-service`.

```text
add 18 siui-bridge
```

Dacă echipamentul 18 există deja, nu îl ștergeți și nu regenerați cheia:

```text
key 18
rights 18
```

Rezultatul `key` afișează și rolurile JWT permise. O cheie creată înaintea
actualizării primește suportul `service` la încărcarea registrului în WSM nou;
secretul și drepturile rămân aceleași. Cheile statice din configurație necesită
`service` în `security.keys.<key-id>.roles` și reload/restart.

### Diagnostic din CLI și loguri

Porniți WSM și bridge-ul cu `--showlog` pentru afișarea logurilor în terminal.
Refuzurile JWT au HTTP 401, corp JSON cu `error` și `message`, iar logul WSM
conține același cod și instrucțiunile de remediere. Bridge-ul actualizat le afișează
în log și `last_error`; este necesară actualizarea ambelor binare. Pentru un
WSM/proxy vechi care întoarce doar `unauthorized`, bridge-ul afișează pașii generali.

| Cod | Acțiune |
| --- | --- |
| `wsm_role_denied` | SIUI necesită service; actualizați WSM pentru chei generate sau corectați roles la cheia statică. |
| `wsm_scope_denied` | Mesajul numește permisiunea lipsă. Verificați rights 18; eliminați-o din bridge sau acordați-o prin toggle și save, apoi copiați scopes salvate. |
| `wsm_tenant_mismatch` / `wsm_issuer_mismatch` / `wsm_audience_mismatch` | Copiați valorile din key 18 în bridge/emitentul JWT. |
| `wsm_subject_mismatch` / `wsm_reader_id_mismatch` / `wsm_equipment_id_mismatch` | Corectați câmpul indicat; edit 18 siui-bridge modifică restricția reader-id, păstrând cheia. |
| `wsm_token_expired` / `wsm_token_not_yet_valid` | Reînnoiți JWT-ul și verificați sincronizarea ceasurilor. |
| `wsm_invalid_credentials` | Verificați key_id, secretul, cheia activă și formatul JWT. |

Identitatea și drepturile sunt diagnosticate numai după verificarea semnăturii.
Pentru un secret greșit sau key_id necunoscut, răspunsul nu confirmă existența
cheii. Logurile și răspunsurile nu includ JWT-uri sau secrete.

Comenzile `add`, `key`, `rights`, `toggle` și `save` aparțin consolei WSM din
browser, nu executabilului `wsmctl`. `wsmctl keygen -out FILE` generează offline
un secret pentru o cheie statică; operatorul trebuie să configureze separat
cheia, tenantul, rolurile, identitatea și drepturile în WSM. Nu înregistrează
un echipament prin API și nu modifică registrul consolei.

ID-ul echipamentului este cel din WiseMED. Comanda `add` configurează autorizarea
WSS; nu creează un echipament medical în baza de date WiseMED. Readerul efectuează
în continuare inițializarea HTTP WiseMED înainte să se conecteze la WSS.

După `add` sau `key`, interfața afișează separat cheia și identitatea necesară:
tenant, ID cheie, emitent, audiență, subiect și ID echipament. Copiați datele în
Setări → Debug WSS ale readerului, cu autentificare **Cheie echipament (JWT)** și
câmpul **Cheie secretă WSS**. Cheia se salvează în YAML-ul readerului; nu este
nevoie să creați manual un fișier `.key` pe calculatorul acestuia.

Lista nu conține secrete. `key` dezvăluie doar cheia unui aparat administrat în
tenantul sesiunii, nu cheile principale de backend. Secretul este afișat într-un
panou separat și nu este păstrat în istoricul comenzilor, localStorage sau loguri.
Un administrator autorizat îl poate solicita din nou cu `key`.

## Configurare și persistență

Adăugați în configurația WSM (păstrând restul configurației):

```yaml
admin:
  enabled: true
  public_origin: https://ws.example.ro
  session_ttl_seconds: 900
security:
  device_keys_file: /var/lib/wsm-server/device-keys.yaml
tenants:
  clinic-a:
    issuer: https://clinic-a.example.ro
    allowed_origins: ["https://clinic-a.example.ro"]
    wisemed:
      base_url: https://clinic-a.example.ro/api
      api_key_file: /etc/wsm-server/keys/clinic-a-api.key
```

`public_origin` este originea browserului, fără path, și include portul dacă este
nestandard. În spatele Nginx este adresa HTTPS publică, nu adresa HTTP internă.
Pentru mai multe adrese ale aceleiași console, folosiți lista `public_origins`:

```yaml
admin:
  enabled: true
  public_origins:
    - https://wslocal.wisemed.eu
    - https://wslocalhost.wisemed.eu
  session_ttl_seconds: 900
```

`public_origin` rămâne compatibil și se adaugă listei dacă este configurat.
Fiecare cerere trebuie să aibă Host și Origin pentru aceeași adresă permisă;
lista nu permite cereri cross-origin între aliasuri. Cookie-urile sunt limitate
la hostul accesat, deci autentificarea se face separat pe fiecare adresă.
După actualizarea binarului pentru suportul listei, reporniți WSM.

`base_url` și cheia API trebuie să corespundă instanței WiseMED reale; adresele din
exemplu nu reprezintă un backend funcțional.

Intrările generate sunt salvate automat în `security.device_keys_file`, un YAML
extern cu permisiuni restrictive. WSM îl încarcă împreună cu configurația principală
la pornire. Configurația principală poate rămâne read-only. Fișierul generat conține
chei secrete și trebuie inclus în backup-ul protejat, nu în Git sau imaginea Docker.
Cheile sunt salvate înainte de confirmarea operației și se aplică fără restart.

Pentru Docker, păstrați mount-ul configurației read-only și adăugați un director
persistent inscriptibil pentru registry:

```sh
--mount type=bind,src=/srv/wsm-server/state,dst=/var/lib/wsm-server
```

Directorul de pe host trebuie deținut de UID/GID 65532, cu permisiuni restrictive,
pentru imaginea furnizată. Montați directorul, nu doar fișierul: salvarea atomică
folosește un fișier temporar și rename. Un container cu root filesystem read-only
poate scrie în acest mount separat. Pentru systemd folosiți directorul de stare
autorizat de unitate. Registry-ul nu adaugă suport multi-replică: folosiți o singură
instanță WSM activă pentru acest fișier.

## Modificare și ștergere

`equipment_id` este ID-ul echipamentului din WiseMED (de exemplu `1`). `reader_id`
este identificatorul text trimis de aplicație, de exemplu
`horiba-yumizen-h500-reader-v3`, vizibil în rezumatul readerului. Nu este un ID
numeric de tip echipament din WiseMED. Restricția opțională verifică exact acest text.

Modificarea reader-id păstrează secretul, ID-ul cheii și subiectul JWT. Conexiunile
acelui aparat sunt închise pentru reautentificare; celelalte echipamente rămân
conectate. Eliminarea restricției nu elimină autentificarea cu cheia proprie sau
asocierea la tenant și equipment_id. `delete` revocă cheia și închide conexiunile
aparatului, dar nu șterge aparatul sau configurația lui din WiseMED/reader.

API-ul consolei folosește sesiunea admin, Origin și X-CSRF-Token:
- `PATCH /admin/api/devices/{keyID}` cu `{"reader_id":"identificator-text"}`;
  `{"reader_id":""}` elimină restricția. La PATCH este necesar cel puțin unul dintre câmpurile reader_id și scopes.
- `DELETE /admin/api/devices/{keyID}` șterge accesul WSS.

Comenzile consolei primesc equipment_id și rezolvă keyID din lista tenantului.
Schimbările sunt persistate înainte de aplicare și nu necesită restart WSM.

## Drepturile cheii

`rights 1` citește drepturile reale ale cheii, nu doar valorile implicite ale
readerului. Fiecare rând are număr, `[x]` sau `[ ]`, numele dreptului și titlul. După `rights`, comanda `info 3 7` afișează explicațiile extinse și exemplele pentru numerele selectate; acceptă unul sau mai multe numere și nu modifică selecția.
`toggle 1 2` inversează selecția pentru numerele respective. Nimic nu se modifică
pe server până la `save`; `cancel` abandonează selecția.

Catalogul este:

1. `route:reply`: răspunsuri la comenzi; păstrați pentru controlul remote al aparatului.
2. `route:event`: trimiterea evenimentelor.
3. `route:command`: trimiterea comenzilor.
4. `devices:debug`: ping/reconnect/mesaje către alte echipamente.
5. `connections:read`: lista și prezența echipamentelor.
6. `server:status`: starea serverului.
7. `api:invoke`: „Deschide” și apelarea API-urilor echipamentelor din tenant,
   inclusiv operații care modifică date; necesită dreptul 3.
8. `api:admin`: și operațiile administrative ale API-urilor remote; necesită 3 și 7.
   Nu acordă acces la consola admin WSM.
9. `route:broadcast`: destinatari multipli; se combină cu dreptul pentru tipul mesajului.
10. `topics:subscribe`: abonare și dezabonare la topicuri și primirea mesajelor adresate acestora, în același tenant. Se acordă explicit, nu implicit.
11. `wisemed:proxy`: comenzi WiseMED prin API-ul configurat pentru tenant; necesită upstream WiseMED configurat.

Pentru o cheie nouă implicită, exemplu de activare „Deschide”:

```text
rights 1
toggle 7
save
```

Verificați bifele înainte: toggle inversează starea existentă. După salvare, consola
arată lista separată prin virgule. Copiați-o în reader → Debug WSS → **Permisiuni
solicitate**, apoi **Salvează și aplică**. Drepturile din WSM sunt limita autorizată;
JWT-ul readerului trebuie să solicite drepturile pe care le folosește. Un JWT ce
solicită un drept eliminat va fi refuzat. Cheia secretă rămâne aceeași.

API: `PATCH /admin/api/devices/{keyID}` cu `{"scopes":["route:reply", ...]}`.
Nu acceptă drepturi necunoscute, duplicate, lista goală sau dependențe lipsă.
Persistă în `security.device_keys_file`. Înregistrările vechi fără scopes păstrează
setul implicit. Modificarea închide conexiunile și invalidează tichetele emise din
acea identitate; alte chei rămân neafectate. Drepturile nu sunt limitate automat la
propriul aparat: api:invoke permite control în tenant, conform regulilor de rutare.

## Istoricul comenzilor

Săgețile ↑ și ↓ din câmpul de comandă navighează prin istoricul comenzilor.
La revenirea după cea mai recentă comandă se restaurează textul neterminat.
Comanda `history` afișează comenzile cronologic, inclusiv comanda `history` curentă.
Sunt păstrate ultimele 1.000 de comenzi cu sintaxă validă, inclusiv cele pentru
care serverul a raportat o eroare. Parolele, răspunsurile și cheile afișate nu se
salvează în istoric. Reamintirea unei comenzi nu o execută; este necesar Enter.

Persistența este în localStorage, pe același browser/profil și aceeași origine,
separat după tenant și utilizator. Istoricul rezistă la refresh, logout, închiderea
browserului și repornirea WSM; nu se sincronizează între browsere/calculatoare.
Ștergerea datelor site-ului îl elimină. Dacă stocarea nu este disponibilă, consola
anunță acest lucru și păstrează istoricul doar în sesiunea curentă.

## Loguri zilnice și urmărire live

WSM scrie simultan în stderr/journal și în fișiere private `wsm-YYYY-MM-DD.log`,
cu zile și timestamps UTC. În consola autentificată:

```text
logs                    # ziua curentă UTC, ultimele mesaje
logs 2026-10-28          # ziua aleasă
logs dates              # zilele disponibile
logs live               # actualizare la fiecare secundă; urmărește și schimbarea zilei
logs live 2026-10-28     # urmărește ziua explicită
logs stop
```

Panoul oferă selector de zi, Încărcare, În timp real și Închide. Implicit este azi.
Accesul folosește sesiunea administrativă WiseMED creată numai pentru `user_type`
canonic `-1`; un JWT browser sau rolul `api:admin` nu oferă acces la fișiere.
Fiecare citire live revalidează sesiunea și se oprește la logout/expirare/reload.
Sunt afișate mesajele marcate cu tenantul curent și diagnosticele generale fără
marcaj tenant, nu mesajele altor tenanturi. Fișierele de pe disc conțin jurnalul
complet al procesului; interfața afișează o pagină recentă și păstrează cel mult
1 MiB de text în memorie.

Directorul se poate configura prin `server.log_dir` (relativ la configurație).
Implicit este `logs/` lângă `security.device_keys_file`; dacă registrul nu este
configurat, este `../logs` față de directorul configurației. Pentru Docker/systemd,
registrul din `/var/lib/wsm-server` duce automat la `/var/lib/wsm-server/logs`, în
volumul persistent existent. Directorul trebuie să fie inscriptibil de WSM.
Fișierele sunt create cu `0600`, directorul cu `0700`; nu sunt servite ca resurse
statice. Nu există ștergere automată a zilelor vechi. Schimbarea `log_dir` cere
restart. O eroare de scriere la runtime este raportată în stderr, care rămâne activ.
Fișierele se deschid după validarea configurației; erorile care împiedică pornirea
sau inițializarea directorului se văd în stderr.

Refuzurile JWT includ codul, peer-ul și o origine validată, fără JWT/secret/query.
Refuzurile de origine și de identitate hello sunt acum logate explicit. Nu sunt
logate corpurile API medicale sau textul erorilor de parsare a mesajelor WSS.
