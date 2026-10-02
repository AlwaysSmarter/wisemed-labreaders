# WiseMED SIUI Bridge - verificări CNAS și validare paraclinice 72h

Utilitar Go din readersv3, cu aceeași infrastructură ca barcodeprinter: HTTP/HTTPS,
login WiseMED, WSM, loguri zilnice, configurare, reconfigurare, serviciu/daemon,
update și installer. Include autentificare OCSP, verificare CNP (`getInsured`),
descărcare personalizare PARA (`getProviderInfo`) și validarea paraclinicelor
`SiuiValidateWS.validateReport(reportXml, "PARA", "RQ_PARA_SRV")`.

Trimite imediat cererea primită. „72h” denumește fluxul operațional solicitat, nu
un temporizator: nu așteaptă 72 de ore și nu impune local reguli calendaristice sau
excepții CNAS. WiseMED generează cererile și decide momentul trimiterii; CNAS
stabilește eligibilitatea. Nu sunt implementate raportarea lunară, eCard/PIN de
pacient, rețete sau facturi.

## Pornire și configurare

1. Instalează pe Windows driverul tokenului furnizorului. Certificatul trebuie să
   apară în magazinul `MY` al contului care rulează utilitarul și să aibă asociată
   cheia privată. Cheia rămâne pe token. Nu se exportă/salvează fișier `.cer`.
2. Prima pornire copiază `deployments/config.install.yaml` în
   `deployments/config.yaml`. Fișierul existent se păstrează. Toate căile relative
   din module se rezolvă față de `deployments/`.
3. Configurează conexiunea WiseMED prin interfața comună sau `-reconfigure`.
   Inițializarea HTTP WiseMED trebuie să atribuie `echipament_id` înainte de WSM.
4. În **Setări → CNAS - validare 72h**, un administrator completează utilizatorul
   CNAS, selectează magazinul și apasă **Reîncarcă certificatele**. Alege după
   titular, emitent, expirare și ultimele caractere din thumbprint, apoi salvează.
   Certificatele expirate/fără asociere cu o cheie privată nu pot fi selectate.
   Certificatul înregistrat poate apărea și când tokenul este scos; prezența reală
   și PIN-ul sunt verificate de driver la conectare.
5. Introdu **Seria de licență CNAS** în același formular. Este un câmp mascat,
   salvat în `modules.siui.licence` din `deployments/config.yaml`, fără fișier
   separat. API-ul returnează `licence_configured` și indiciul `licence_hint` (`ABC…XYZ`),
   niciodată valoarea completă. Licențele cu maximum 6 caractere sunt mascate integral.
   La modificări ulterioare, câmpul gol păstrează licența salvată. Poți salva
   utilizatorul și licența pe macOS/Linux înainte de alegerea tokenului pe Windows.
   Configurația conține acest secret local; salvarea setează permisiuni 0600 pe Unix.
6. Setările HTTP/HTTPS, adresa și limba sunt cele comune. Implicit:
   `https://127.0.0.1:19112`, cu certificatul HTTPS local generat de infrastructura
   comună. Pentru test HTTP setează `local_http.tls: false` și
   `modules.local-http.tls: false`; păstrează bindul pe loopback.
7. Configurează WSM în **Debug → WSS**: cheie de echipament, tenant, issuer și URL,
   apoi activează conexiunea. Vezi `../../docs/wss-control.md`.

PIN-ul tokenului este gestionat de driverul Windows. Nu există câmp PIN în WSM,
API, YAML sau baza de date. Transportul CNAS folosește WinHTTP/Schannel și
certificatul selectat, cu validarea certificatului serverului și TLS 1.2.
Nu se urmează redirecturi și nu se retransmit automat cereri de validare.
Validarea XSD folosește `System.Xml` prin Windows PowerShell existent în sistem;
scriptul este fix, XML-ul este transmis ca date pe stdin. Nu este necesar un
proiect/runtime .NET nou. Pe macOS/Linux, interfața și WSM rulează; XSD folosește
`xmllint`, iar conectarea reală cu tokenul este disponibilă doar pe Windows.

## Testare fără validare de servicii

În Setări → CNAS, secțiunea **Verificări fără raportare de servicii**:

1. **Test local fără CNAS** merge și pe Mac, fără token/licență/CNP. Verifică schema
   și interpretarea unui răspuns fictiv fix, marcat explicit TEST LOCAL. Nu testează
   conexiunea sau autentificarea OCSP și nu indică asigurarea reală a unei persoane.
2. **Testează autentificarea OCSP** (Windows cu token configurat) face numai GET
   la `/OCSP/validator` și verifică prezența sesiunii OSCP_RESPONSE. Sesiunea nu
   apare în răspuns, în loguri sau în baza de date. Nu introduce CNP sau XML.
3. **Verificare calitate de asigurat** primește CNP și data consultării. Obține
   automat o sesiune OCSP nouă, apoi apelează `SiuiInsuredWS.getInsured`. Stările
   sunt distincte: Eroare CNAS, Inexistent, Asigurat, Neasigurat, Decedat.
4. **Descarcă personalizare PARA** (administrator) primește intervalul de date,
   extrage CUI din utilizatorul `CUI_CAS-CODE`, autentifică OCSP, apelează
   `SiuiWS.getProviderInfo` și descarcă XML/ZIP-ul returnat, numai de pe aceeași
   origine HTTPS CNAS. Fișierul ajunge în descărcările browserului, inclusiv prin WSM.

Niciunul dintre aceste butoane nu apelează validateReport. Operațiile reale se
execută pe Windows; pot fi controlate din browserul Mac prin WSM. Nu se rulează
simultan cu alte operații pe token. Timeout-ul este de 25 secunde; la expirare nu
se afișează „neasigurat”, ci o eroare. Nu există retry automat. Rezultatele/CNP-urile
nu se persistă în istoricul validărilor. Personalizarea este limitată la 1 MiB,
atât arhiva cât și XML-ul decomprimat; fișierele mai mari sunt refuzate explicit.

Formularul de **validare a serviciilor** este separat, într-o secțiune închisă
implicit; deschiderea lui nu trimite date. Trimiterea cere apăsarea propriului buton.

## Comenzi și instalare

Din directorul runtime:

```powershell
.\siui-bridge.exe -config deployments\config.yaml -showlog
.\siui-bridge.exe -config deployments\config.yaml -reconfigure
.\siui-bridge.exe -config deployments\config.yaml -headless
# Terminal elevat pentru serviciu:
.\siui-bridge.exe -config deployments\config.yaml -installservice
.\siui-bridge.exe -version
```

Serviciul Windows trebuie să ruleze sub contul care poate accesa certificatul și
cheia tokenului. Implicit installerul comun folosește LocalSystem: configurează
contul în Services → Log On, sau provisionarea LocalMachine, apoi reselectează
certificatul din interfața instanței care rulează efectiv ca serviciu. Un driver
care solicită obligatoriu dialog PIN interactiv nu poate afișa acel dialog în
Session 0; pentru el folosește rularea interactivă/headless în sesiunea utilizatorului,
sau modul noninteractiv suportat oficial de driver. Nu se copiază cheia privată.

Logurile zilnice și UI de loguri sunt cele comune. Jurnalul SIUI scrie doar ID-ul
intern al jobului și starea, fără XML medical, licență sau sesiune OCSP.
`deployments/siui-jobs.db` conține istoricul și răspunsurile CNAS; trebuie protejat
ca date medicale locale. Nu se păstrează XML-ul cererii. Nu șterge baza de date
pentru a reîncerca: ea protejează împotriva trimiterilor duplicate. Rularea simultană
a două instanțe pe aceeași bază de date este refuzată.

Build din `readersv3/`:

```sh
./scripts/prepare-reader-output.sh siui-bridge
go run ./tools/releasectl build --app siui-bridge --target windows-amd64
go run ./tools/releasectl release --app siui-bridge --target windows-amd64
```

Sunt incluse wrapper-ele standard pentru build/install/service pe platformele
comune. Windows amd64 este ținta pentru token; suportul macOS/Linux este pentru
UI/WSM/test, nu reprezintă un driver USB multiplatformă.

## API și verificare

Vezi [protocol.md](docs/protocol.md), exemplele HTTP/WSM și datele sintetice din
[validate-request.xml](docs/validate-request.xml). Interfața CNAS include și un
formular de test XML și istoricul joburilor. Conturile obișnuite pot trimite și
consulta propriile validări; configurarea și lista certificatelor cer administrator.

```sh
go test -race ./modules/siui ./modules/localhttp ./apps/runner
go vet ./modules/siui ./apps/siui-bridge
node --test modules/localhttp/ui/siui-ui.test.cjs modules/localhttp/ui/wss-ui.test.cjs modules/localhttp/ui/wss-remote.test.cjs
```

Testele CNAS folosesc un server TLS simulat și date sintetice. Un build Windows
verificat prin cross-compilare nu dovedește funcționarea driverului USB sau
acceptarea reală CNAS. Testul final se face pe Windows, cu tokenul, contul și
cererile reale autorizate ale furnizorului.

## HTTPS local: browserul afișează „Your connection is not private”

SIUI Bridge generează aceeași autoritate locală ca celelalte readere, în
`deployments/tls/wisemed-local-root-ca.pem`; certificatul serverului include
`localhost`, `127.0.0.1` și `::1`. Aceasta este autoritatea interfeței HTTPS locale,
independentă de certificatul CNAS de pe token; tokenul nu se exportă.

Pe macOS, la pornire se verifică încrederea în Keychain și se încearcă importul
în keychain-ul utilizatorului. Acceptă autorizarea cerută de macOS. Dacă sistemul
refuză schimbarea sau aplicația rulează fără sesiune interactivă, logul indică
problema. Poți deschide `deployments/tls/wisemed-local-root-ca.pem` în **Keychain
Access**, apoi la **WiseMED Local Root CA → Trust** alegi **Always Trust**.
Pentru daemon folosește keychain-ul **System**, cu autorizare de administrator.
Închide complet și redeschide browserul după schimbarea încrederii.

Pe Linux sunt suportate `update-ca-certificates` și `update-ca-trust`. Instalarea
în magazinul sistemului cere root; fără aceste drepturi, logul indică fișierul,
directorul și comanda necesare. Browserele cu magazin separat pot necesita importul
aceleiași autorități publice în setările lor. Nu se dezactivează verificarea TLS.

Pe Windows, utilitarul verifică/importă autoritatea prin API-ul nativ în
`LocalMachine\Root`. La pornire manuală fără drepturi de administrator poate folosi
`CurrentUser\Root`, valabil numai pentru contul care rulează aplicația. Pentru un
serviciu este necesară încrederea la nivelul calculatorului. Logul precizează
magazinul folosit sau eroarea de import, fără a opri interfața HTTPS.

Pentru instalările existente, dintr-un terminal **Administrator**, deschis în
folderul utilitarului (adaptează calea dacă folosești alt fișier de configurare):

```powershell
certutil -addstore -f Root ".\deployments\tls\wisemed-local-root-ca.pem"
```

Închide complet și redeschide browserul, apoi accesează
`https://localhost:19112/settings/siui`. Nu șterge folderul `tls`: regenerarea
schimbă autoritatea care trebuie instalată. Dacă avertismentul persistă, verifică
codul exact al erorii și emitentul certificatului prezentat de browser. Un browser
pe alt calculator are nevoie de încredere în propria sa instalare.

## Actualizări

SIUI Bridge folosește mecanismul comun din runner și local-http, inclusiv la rularea
ca serviciu. Implicit, actualizările și descărcarea automată sunt activate, canalul
este `stable`, `app_id` este `siui-bridge`, iar serverul implicit este
`http://127.0.0.1:19090`, ca la celelalte utilitare. Configurează adresa serverului
real în **Server și actualizări** dacă serverul nu rulează pe același calculator.
Verificarea folosește cheia WiseMED `cfg_wisemed_key`, nu licența CNAS.

În panoul CNAS sunt disponibile **Server și actualizări** și **Verifică actualizări**;
aceeași verificare este disponibilă prin click pe versiunea din interfața comună.
Serverul trebuie să aibă publicat pachetul pentru `siui-bridge` și platforma curentă.
Configurațiile existente sunt păstrate la instalare/update; dacă provin din prima
versiune cu update dezactivat, activează-l din setări. Comportamentul de descărcare,
verificare checksum și aplicare la pornire/ca serviciu este cel comun tuturor readerelor.

## OCSP: Windows error 12175

12175 indică un eșec de verificare/negociere HTTPS WinHTTP, nu un răspuns de
licență greșită primit de la CNAS. Versiunea curentă afișează și flags/cauza din
callback-ul WinHTTP: certificat expirat, CA necunoscută, nume diferit, revocare sau
Schannel. Nici sesiunea OCSP, nici licența/PIN-ul nu apar în diagnostic.

Portul este explicit în formular: implicit 443; 444 este disponibil dacă endpointul
folosit îl cere. Se persistă în `base_url` (ex. `https://www.siui.ro:444`). Schimbarea
portului nu reprezintă o soluție generală pentru erori de certificat.

La verificarea TLS directă din 3 octombrie 2026 (ora României), www.siui.ro:443
prezenta certificatul *.siui.ro emis de certSIGN Web CA, valabil până la
9 aprilie 2026 08:20:01 UTC. Aceasta este o observație la acel moment, nu o valoare
fixată în cod. Verifică data Windows și certificatul endpointului folosit.
Vechiul C# folosea o politică ce accepta orice certificat de server; noul transport
păstrează verificarea TLS. Un certificat expirat la server trebuie reînnoit de
operatorul endpointului; instalarea autorității HTTPS locale nu remediază acest caz.
