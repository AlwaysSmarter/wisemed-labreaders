# WiseMED SIUI Data

Serviciu central pentru nomenclatoare CNAS, cu PostgreSQL, API autentificat, dashboard pe `partnerCategory`, import ZIP/local și sincronizare prin SIUI Bridge conectat la WSM.

## Categorii și modele

Dashboard-ul și filtrul de versiuni includ MF, PHM, CLIN, PARA, STOM, AMB, MD, HC, REC, SPT, NHP, DIA, SICK și CBRET. Sunt implementate importul și sincronizarea pentru **CLIN, PARA și PHM**. Celelalte categorii sunt marcate explicit „Planificat”; nu pretind că au date importate.

- CLIN: `ClinicService` → `ClinicServiceGroup` → `ClinSrvGrpSpecialty` → `Speciality`; pachetele provin din `ClinSrvMedPack` → `MedSrvPack`.
- PARA: `LaboratoriesService` și `LabSrvMedPack` → `MedSrvPack`. Specialitățile existente în sursă sunt păstrate, dar nu sunt asociate artificial serviciilor PARA.
- Toate celelalte înregistrări din XML sunt păstrate cu atributele, eventualii copii XML și valabilitatea lor, inclusiv medicamentele și listele de compensare din CLIN. Sunt accesibile prin API-ul generic `records/{kind}`.
- Nomenclatoarele comune aparțin versiunii sursei lor: un import PARA nu modifică datele CLIN. Nu există un catalog global suprascris după „ultimul import”.

Aplicația și artefactele se numesc `wisemed-siui-data`. Modulul/protocolul și prefixul API sunt `siui-data` și `/api/siui-data/`. JWT issuer: `wisemed-siui-data`; audience: `siui-data-api`. Tokenurile existente trebuie reemise pentru audience-ul corectat. Configurațiile cu vechea scriere a modulului/protocolului sunt migrate automat, păstrând PostgreSQL și cheile per CUI. Numele variabilelor de mediu configurate explicit rămân valabile; instalările noi folosesc `WISEMED_SIUI_DATA_JWT_SECRET`.

## Pornire și setup

Din rădăcina `readersv3`:

```sh
./scripts/prepare-reader-output.sh wisemed-siui-data
cd output/wisemed-siui-data
./wisemed-siui-data -showlog
# Reconfigurare interactivă:
./wisemed-siui-data --reconfigure
```

Adresă implicită: `https://127.0.0.1:19114/`. După login WiseMED, Overview afișează dashboard-ul pe categorii. Pagini directe:

- `/siui-data-dashboard`: categorii, căutare și upload.
- `/siui-data-versions`: istoric tabelar, statistici și sincronizare WSM.
- `/siui-data-debug`: explorator pentru toate endpointurile, filtre simple/array, URL final și răspuns JSON formatat.
- `/siui-data-settings`: setup PostgreSQL, director local și limite (administrator).

Prima pornire cu o configurație incompletă și `--reconfigure` folosesc același setup dedicat: PostgreSQL, limite, director monitorizat, WiseMED API, alegerea unității și a tipului de echipament, înregistrarea prin API și salvarea `echipament_id`, conexiunea WSM și actualizările. WSM poate fi activat/dezactivat explicit; la activare se cer URL-ul și autentificarea (`device_key`, `token_file` sau `token_endpoint`). Enter păstrează valorile existente, inclusiv parolele. O înregistrare fără un ID valid oprește setup-ul. Identitatea și conexiunea pot fi administrate și din paginile comune Reader / WSS, accesibile din Setup. Nu se solicită SQLite.

PostgreSQL 14+; baza de date se creează înainte de conectare. La prima utilizare, aplicația creează tabelele și indexurile sale, plus `pg_trgm`. Administratorul poate instala extensia în prealabil. Rolul aplicației trebuie să aibă drepturile de DDL pentru schema sa și drepturile de citire/scriere pentru importuri.

Setări în `modules.siui-data`:

```yaml
postgres_host: 127.0.0.1
postgres_port: 5432
postgres_database: cnas
postgres_user: siui_data
postgres_password: "" # se completează local, nu în repository
postgres_sslmode: prefer
postgres_root_cert: ""
postgres_max_conns: 20
upload_dir: ./catalogue-uploads
max_upload_mib: 4096
max_xml_mib: 16384
import_timeout_minutes: 120
local_import_dir: "" # gol = monitorizare dezactivată
local_poll_seconds: 30
jwt_secret_env: WISEMED_SIUI_DATA_JWT_SECRET
```

Căile relative se rezolvă față de directorul `deployments/`. Parola PostgreSQL este write-only în API; un câmp gol o păstrează. Configurația salvată de utilitar are permisiuni 0600 pe Unix. Pentru PostgreSQL la distanță configurați `verify-full` și CA-ul corespunzător. `postgres_root_cert` folosește o cale de pe serverul aplicației; căile relative se rezolvă față de `deployments/`.

API-ul de setup și dashboard-ul rămân disponibile fără o bază configurată. Rutele de date returnează 503 până la configurarea bazei/importul tipului cerut. Nu este necesar un certificat CNAS pe serverul de date; preluarea CNAS se execută de bridge.

## Upload ZIP și activare atomică

`POST /upload` prin HTTPS, autentificat ca administrator, primește `type=CLIN|PARA|PHM` și `file` în multipart. Alternativ: corp ZIP brut cu `?type=CLIN`. Aliasuri: `/api/siui-data/upload` și `/api/siui-data/import` (acum primesc ZIP, nu vechiul corp XML).

Upload-ul este copiat incremental într-un fișier temporar privat; nu se folosește `ParseMultipartForm` și nu se încarcă întreaga arhivă în RAM. ZIP-ul poate conține maximum 128 de intrări și fișiere XML `Catalogues`. Nu se extrag căi din arhivă pe disc. Limitele ZIP/XML și timeoutul sunt configurabile; o singură înregistrare XML poate avea maximum 8 MiB.

După upload se returnează **202 + importId**. Workerul parsează XML incremental și folosește PostgreSQL COPY într-o tranzacție de import. Construiește relațiile/răspunsurile pentru servicii, statisticile și indexurile necesare; apoi schimbă versiunea activă în aceeași tranzacție. Cititorii continuă să folosească versiunea anterioară. O eroare, o arhivă greșită sau lipsa serviciilor active produce rollback și nu modifică datele active.

Blocarea PostgreSQL este separată pe categorie și funcționează și între procese. Este permis un singur import per categorie (CLIN, PARA, PHM), cu maximum două operații de upload/import/sync per proces. Cererile normale nu sunt serializate în spatele importului. Replicile aplicației trebuie să folosească aceeași bază; fișierele temporare sunt locale fiecărei instanțe.

Datele brute istorice și viitoare se păstrează. Serviciile publicate se filtrează la `asOf`, în Europe/Bucharest. Fișierele reale CNAS conțin și perioade istorice cu `validTo < validFrom`: acestea rămân păstrate, inactive și numărate în `invalidPeriods`. Referințele lipsă din graful serviciilor active, codurile active duplicate și asocierile suprapuse sunt respinse. Aceasta este validare structurală și de domeniu pentru modelele implementate, nu validare integrală XSD a tuturor nomenclatoarelor.

`asOf` rămâne data importului. După schimbarea zilei, statusul marchează `stale`; un import reevaluează valabilitatea. Nu există activare automată zilnică a înregistrărilor viitoare.

## Import din director local

Configurați `local_import_dir` din Setup sau `--reconfigure`. Monitorizarea citește fișierele XML din directorul configurat și, pentru medici, din subdirectorul `getPhysicianCatalogues`:

- `NomenclatoareClinice_YYYYMMDD.xml` sau `NomenclatoareCliniceYYYYMMDD.xml` → CLIN.
- `NomenclatoareParaclinice_YYYYMMDD.xml` sau `NomenclatoareParacliniceYYYYMMDD.xml` → PARA.
- `NomenclatoareFarmacii_YYYYMMDD.xml` sau `NomenclatoareFarmaciiYYYYMMDD.xml` → PHM.

Pentru fiecare categorie este selectat cel mai nou fișier după data validă din nume. Dimensiunea și mtime trebuie să fie stabile la două scanări consecutive. Fișierul este copiat într-o sursă temporară și se verifică dacă s-a modificat în timpul copierii. Pentru livrare atomică, scrieți întâi sub un nume temporar și redenumiți fișierul complet în numele final.

Data din nume trebuie să fie strict mai nouă decât cea mai mare dată a unei surse importate cu succes pentru aceeași categorie, indiferent de restart sau dacă importul anterior a fost local, upload sau WSM. Nu se compară cu ora upload-ului și nici cu mtime. Dacă XML-ul conține `issueDate`, trebuie să corespundă datei din nume. Un fișier eșuat neschimbat nu se reîncearcă la fiecare scanare; schimbarea lui permite o nouă încercare. Importul manual prin upload poate fi folosit pentru o reimportare explicită.

## Versiuni și WSM

„Versiuni nomenclatoare” oferă filtrul cu toate cele 14 valori `partnerCategory`, ultima versiune activă și ultimele 100 de încercări (API: până la 1000). Fiecare încercare are data sursei, originea, momentele de pornire/finalizare, `running` / `success` / `fail`, eroarea și statistici pe tip de înregistrare: total, active la import, perioade inversate. CLIN, PARA și PHM au date/istoric independent. Tabelul de categorii afișează și ultima verificare WSM reușită, inclusiv când rezultatul este „fără actualizări”; fără verificări apare „—”.

Dacă utilitarul este conectat la WSM, pagina listează conexiunile `client_type=siui-bridge` din tenantul propriu. Selectați categoria și bridge-ul, apoi „Preia ultimul nomenclator”. Cu un singur bridge selecția poate fi automată.

Fluxul folosește `api.request`: lansează `POST /api/siui/general/getCatalogues`, urmărește call-ul și descarcă fișierul în bucăți prin `/api/siui/calls/{id}/file`. Verifică strict offseturile, dimensiunea și progresul. Nu se transportă un fișier de sute de MiB într-un singur mesaj WSM. ZIP, XML și GZIP primite de la bridge sunt procesate cu limitele utilitarului.

`start` este transmis ca reper pentru actualizare (data/ora ultimei verificări WSM reușite pentru categoria respectivă, sau 1 ianuarie din anul precedent la prima sincronizare; în 2026: 2025-01-01). O verificare eșuată nu avansează acest reper. Folosim ora pornirii verificării reușite pentru a nu omite modificările apărute în timpul descărcării. CNAS poate returna ultima versiune disponibilă chiar dacă `start` este în trecut; nu îl tratăm ca selector al unei versiuni istorice. Decizia de import folosește data fișierului efectiv primit, comparată cu sursele importate cu succes pentru categoria respectivă. De exemplu, un `start=2026-01-21T18:21:07Z` poate returna `NomenclatoareParaclinice_20260602`; versiunea sursei este 2026-06-02.

Data descărcării vine din `Catalogues.issueDate` sau din numele CNAS datat. O dată lipsă nu este inventată. O sursă cu aceeași dată sau mai veche produce `status=unchanged`, afișat ca `success`, fără activarea altei versiuni. Operațiile CNAS nu sunt retrimise automat după o eroare/deconectare. Un restart marchează încercările întrerupte ca eșuate numai după ce blocarea PostgreSQL demonstrează că nu mai au un worker activ.

Cheia WSM a utilitarului necesită `connections:read`, `route:command` și `api:invoke`, plus permisiunile obișnuite de răspuns. Aceste drepturi trebuie acordate și pe WSM; aplicația nu le acordă singură. `api:admin` nu este necesar pentru `getCatalogues`. Bridge-ul are propria configurare CNAS/token. Conexiunile multiple cer alegerea explicită a bridge-ului. Limita actuală de download CNAS a bridge-ului este 64 MiB comprimat; ea se aplică înaintea limitelor acestui utilitar.

## Autentificare și performanță

Rutele cer sesiune WiseMED, identitate WSM verificată sau Bearer JWT HS256 pe HTTPS. Upload, sincronizare, setup și eliminarea versiunilor inactive cer administrator. JWT este emis pentru 15 minute exclusiv din sesiunea locală de administrator WiseMED; un JWT sau o identitate WSM nu poate emite alt JWT. Tokenurile cu chei per CUI sunt limitate la propriile servicii țintite. Cheia aleatorie per pornire poate fi stabilizată cu variabila de mediu `WISEMED_SIUI_DATA_JWT_SECRET` (minimum 32 octeți).

Căutarea serviciilor folosește documente JSONB pregătite la import, indexuri trigram pentru text, indexuri pentru relații/atribute și paginare `start`/`limit` cu total filtrat. Nu există cache Redis/Memcached în această etapă. Nu este încărcat întregul catalog în memoria Go. PostgreSQL și sistemul de operare asigură cache-ul paginilor; un cache distribuit poate fi adăugat după măsurarea traficului real.

Versiunile precedente sunt păstrate pentru istoric. Endpointul administrativ `DELETE /api/siui-data/imports/{id}` poate elimina o versiune inactivă și datele ei; nu poate elimina versiunea activă sau o operație în curs. Planificați spațiul PostgreSQL inclusiv pentru versiunile păstrate și pentru WAL. Vechile snapshoturi JSON locale nu sunt migrate automat și nu sunt șterse.

## Validare și distribuție

```sh
go test -race ./modules/siuidata ./apps/runner ./core/config ./modules/localhttp ./modules/ws
# Bază dedicată de test; rolul trebuie să poată crea baze temporare:
SIUI_DATA_TEST_DATABASE_URL='postgres://user@localhost/postgres?sslmode=disable' go test -race ./modules/siuidata
# Test real, opțional, pe arhivele CNAS furnizate:
SIUI_DATA_TEST_DATABASE_URL='postgres://user@localhost/postgres?sslmode=disable' \
SIUI_DATA_REAL_CATALOGUES=/cale/Nomenclatoare go test ./modules/siuidata -run TestRealCatalogues -v -timeout 20m
make release APP=wisemed-siui-data TARGET=linux-amd64
```

Pe arhivele din 2026-10-01 au fost verificate 1.394.704 înregistrări CLIN / 272 servicii active și 437.945 înregistrări PARA / 302 servicii active, la `asOf=2026-10-21`. Aceste numere sunt rezultate pe sursele furnizate, nu o promisiune de capacitate în producție. Fluxul WSM are teste cu bridge simulat; acceptanța CNAS reală necesită un bridge conectat și configurat.

Actualizările comune rămân activate implicit, stable, `app_id=wisemed-siui-data`, cu download automat și verificare la pornire în fundal. Windows/Linux/macOS păstrează build, installere și servicii WinSW/systemd/launchd. Configurația, secretele, baze de date și upload-urile nu se includ în release.

Contractul API: [docs/API.md](docs/API.md).

API-ul folosește `success/data` pentru obiecte, `success/totalCount/rows` pentru liste și `success:false/error` pentru erori. Toate filtrele de căutare acceptă string-uri sau array-uri (`specialties[]=CARD&specialties[]=DERM`); OR în același filtru, AND între filtre. `limit=50&start=0`, apoi `start=50` pentru pagina următoare. `/api/siui-data/simple-services` returnează rânduri plate pentru combinațiile serviciu × pachet × specialitate. `simple-drugs` returnează rânduri plate pentru combinațiile medicament × asociere CNAS × diagnostic DCI. Fără array-uri sau obiecte imbricate; `totalCount` și paginarea numără combinațiile filtrate. Vezi contractul și exemplele din API.md.


## Medicamente și PHM

Importurile construiesc acum și proiecția de medicamente: Drug → DCI, concentrație, formă farmaceutică, țară, ATC și ierarhia lui; liste de compensare, prețuri, PNS, categorii de boală, diagnostice și aprobări. PHM permite importul `NomenclatoareFarmacii_YYYYMMDD.xml` / ZIP și sincronizare WSM. Sunt păstrate separat regulile pe medicament, pe substanță activă, protocoalele și încadrările NHP_DRUG (DRG/ACS).

Endpointuri: `drugs`, `simple-drugs`, `drugs/{code}`, `drug-compensations`, `drug-relations`, `drug-catalogues`, `drug-records/{kind}`. Filtre simple/array, intervale de preț inclusive, `limit`/`start` și total filtrat. Prețul, lista, programul și aprobarea trebuie să se potrivească pe aceeași asociere. Codul de boală G15 este o categorie DiseaseCategory; ICD10 este tratat separat. Detalii și exemple: [API.md](docs/API.md).

Schema migrează la versiunea 2 fără ștergerea datelor. Importurile vechi trebuie reîncărcate o dată manual pentru proiecția de medicamente; serviciile existente rămân disponibile. `drugProjectionReady` și numărul `drugs` apar în status/versiuni.

Exploratorul Debug are un combobox de parametri pentru fiecare endpoint, cu explicații sub fiecare selecție, inclusiv codurile de aprobare și limitele intervalelor de preț.

Validarea grafului pe arhivele reale din 2026-10-01 a importat 117.572 de medicamente active pentru fiecare sursă: CLIN (1.394.704 înregistrări brute) și PHM (1.915.307 înregistrări brute). Rulare separată: `SIUI_DATA_TEST_DATABASE_URL=... SIUI_DATA_REAL_CATALOGUES=... go test ./modules/siuidata -run TestRealMedicineGraph -v -timeout 20m`. Testele de migrare și import concurent verifică păstrarea serviciilor/medicamentelor vechi până la activarea noii versiuni. Aceste rezultate nu reprezintă un test de capacitate în producție.


## Medici CNAS și OPSNAJ

Importă arhiva `getPhysicianCatalogues` prin Upload cu tipul intern `PHYS`. Cele două fișiere `MediciCNAS_YYYYMMDD.xml` și `MediciOPSNAJ_YYYYMMDD.xml` se publică atomic. Pentru directorul local se așteaptă perechea stabilă, cu aceeași dată în nume, mai nouă decât ultimul import. Se păstrează separat datele reale `issueDate` ale fiecărei surse. WSM folosește operația `getPhysicianCatalogues` cu parametri goi, disponibilă din Versiuni selectând PHYS.

API: `physicians`, `simple-physicians`, `physicians/{stencil}`, `physician-catalogues`, `physician-records/{kind}`. Căutare după nume, parafă, specialități, tip/număr contract, tip medic și casa de asigurări; filtre string/array și `limit`/`start`. Lista simplificată conține rânduri plate per asociere medic–specialitate–contract. Identitatea este sursa CNAS/OPSNAJ împreună cu parafa. [Contractul API și exemple](docs/API.md#medici-getphysiciancatalogues).


## Personalizări confidențiale și profiluri de clienți

Pagina locală `/siui-data-clients` este disponibilă numai cu sesiune WiseMED de administrator. Importă XML/ZIP de personalizare, cu categoria aleasă (CLIN, PARA, SPT etc.), apoi caută clientul după CUI/nume. Pagina arată profilul la o dată, contractele și convențiile cu valabilitățile lor, medicii, specialitățile și istoricul versiunilor. Portofoliul poate fi filtrat după categoria contractului activ și dată. Cheile JWT per CUI se creează, rotesc și revocă din aceeași pagină; se salvează cu permisiuni private și nu se returnează după salvare.

Personalizările folosesc tabele PostgreSQL separate de nomenclatoarele publice. Nu sunt accesibile prin API-ul public, nici prin WSM sau JWT-ul global. Listele de pacienți din exporturile MF nu sunt reținute de această bază de contracte. Datele clienților nu apar în `records`, `versions`, `status` sau `imports` publice.

`GET /api/siui-data/targeted_services/{CUI}` și `GET /api/siui-data/targeted-simple-services/{CUI}` aplică profilul obligatoriu la `atDate=YYYY-MM-DD` (implicit azi). Ele întorc numai servicii CLIN pentru specialitățile medicilor activi, cu contract CLIN activ în personalizare. Un contract CLIN inclus într-o personalizare SPT este luat în calcul; convențiile nu acordă această eligibilitate. Contractele PARA sunt importate și vizibile intern, însă aceste două endpointuri filtrează momentan serviciile CLIN.

Configurare opțională în `modules.siui-data`: `client_jwt_secrets: {"7991962": "<secret distinct, minimum 32 bytes>"}`. Recomandat: generează secretul din pagina internă, copiază-l înainte de salvare, apoi distribuie clientului numai propria cheie/token. JWT HS256: header `kid` egal cu CUI-ul numeric; claim `cui` egal cu același CUI, `admin:false`, issuer `wisemed-siui-data`, audience `siui-data-api`, `sub`, `iat`, `exp` cu durată maximă 900 secunde. Cheia globală rămâne în variabila `WISEMED_SIUI_DATA_JWT_SECRET` de pe server; browserul primește doar un token temporar. [Contractul detaliat](docs/API.md#personalizări-interne-și-servicii-țintite).

Filtrele de text au și variante cu sufix `All` pentru chip-uri obligatorii (AND), de exemplu `nameAll[]=consulta&nameAll[]=repetata` sau `presentationModeAll[]=cutie&presentationModeAll[]=20`. Convenția este comună serviciilor, medicamentelor, medicilor și înregistrărilor brute; Exploratorul API afișează parametrii disponibili și explicațiile lor. Array-urile fără `All` păstrează OR.
