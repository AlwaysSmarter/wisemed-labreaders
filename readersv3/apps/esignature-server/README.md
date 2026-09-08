# eSignature server

Utilitar local Signotec Omega / Evolis Sig200. App ID de build și update:
`esignature-server`. Target Windows: `windows-386`, inclusiv pe Windows x64,
deoarece DLL-ul furnizat este x86. Modelul comercial este o etichetă opțională;
SDK-ul deschide primul pad USB detectat. Conectați un singur pad.

## Build, installer și update server

Din `readersv3`, același flux ca barcodeprinter:

```sh
make build APP=esignature-server TARGET=windows-386
make update APP=esignature-server TARGET=windows-386
make installer APP=esignature-server TARGET=windows-386
make release APP=esignature-server TARGET=windows-386
```

În update server, configurați aplicația cu App ID `esignature-server`, OS `windows`,
arhitectură `386`, canal `stable`. Serverul descoperă sursa prin
`modules.app-updates.app_id`. Recompilați/reporniți update serverul cu aceste
schimbări pentru validarea și selectorul x86. Nu a fost publicat automat un release.

Fluxul comun include `STPadLib.dll` din `docs/signotec/WebSocketPadServer` în
`deployments/signotec`, atât în arhiva ZIP de update, cât și în payload-ul NSIS.
Build-urile cu arhitectură incompatibilă sunt respinse explicit. Build-all selectează
Windows x86 pentru acest utilitar. Scriptul `build-windows.sh` folosește tot
`releasectl`; există și scripturi PowerShell în `build`.

Runtime: `dist/windows-386/esignature-server/runtime`. Arhivele de update sunt în
`dist/updates`, installerele în `dist/installers`. Doar `config.install.yaml` se
livrează: configurația de lucru, certificatele, istoricul și logurile rămân locale.

## Pornire și administrare

Porniți `esignature-server.exe` din directorul runtime. Pagina de test:
`https://localhost:19111/esignature`, administrare: `/settings`, diagnostic:
`/debug`, ajutor: `/help/`. Sunt comune cu barcodeprinter: serverul local-http,
certificatele/CA Windows, autentificarea WiseMED, setările de server și update,
help, evenimente, logurile și ciclul de serviciu/restart.

CLI: `-config`, `-headless`, `-installservice`, `-showlog`, `-version`,
`-reconfigure`. Pornirea normală a utilitarului nu cere configurarea unui analizor;
`-reconfigure` pornește explicit wizardul comun. `-installservice` folosește
aceeași implementare de serviciu ca barcodeprinter.

Update-urile se verifică la pornire, folosind mecanismul comun și cheia din
`modules.wisemed-api.cfg_wisemed_key`. Configurați URL-ul serverului și cheia
potrivită mediului în administrare. Conexiunea WiseMED WS este disponibilă, dar
implicit dezactivată până la configurarea echipamentului.

Driverele USB Signotec și runtime-urile Microsoft cerute de DLL trebuie instalate
pe Windows din kitul vendor. Nu sunt necesare Java, COM sau STPadServer.exe.
Captura nu trebuie folosită simultan cu alt program care deține pad-ul.

## API local WSS

Originile se configurează în administrarea comună HTTP/HTTPS
(`modules.local-http.cors_allowed_origins`). Originea paginii locale este permisă.
WSS acceptă origini exacte; wildcard-ul nu autorizează orice website.

```javascript
const pad = new WebSocket('wss://localhost:19111/ws');
pad.onopen = () => pad.send(JSON.stringify({id:'1', action:'devices'}));
pad.onmessage = ({data}) => {
  const result = JSON.parse(data);
  if (result.imageBase64) {
    document.querySelector('img').src = 'data:image/png;base64,' + result.imageBase64;
  }
};
// Trimiteți comenzile succesiv, prin acțiuni ale utilizatorului:
// pad.send(JSON.stringify({id:'2', action:'start'}));
// pad.send(JSON.stringify({id:'3', action:'retry'}));
// pad.send(JSON.stringify({id:'4', action:'confirm'}));
// alternativ: action:'cancel'
```

Comenzi: `health`, `devices`, `start`, `retry`, `confirm`, `cancel`. Răspunsuri:
`id`, `action`, `ok`, iar la eroare `message`. Confirmarea returnează `imageBase64`
și `mimeType: image/png`. Semnătura nu este salvată pe disc. Istoricul SQLite
păstrează doar data, operația, rezultatul și eroarea, fără imagine sau date pacient.

O singură sesiune deține pad-ul, indiferent de transport. Confirmarea, anularea,
deconectarea WSS, oprirea serverului sau timeout-ul eliberează dispozitivul.
Durata maximă a sesiunii este `session_timeout_seconds`, implicit 180, între 1–600.

## HTTP și WiseMED WS

`POST /api/esignature/command`, JSON cu `id`, `action`, `session_id`, utilizează
aceleași comenzi. Pentru captura în mai mulți pași, generați un `session_id` aleator
nou (de exemplu UUID) și folosiți-l la start/retry/confirm/cancel. Același protocol
este disponibil prin dispecerul WiseMED cu acțiunile `esignature.health`,
`esignature.devices`, `esignature.start`, `esignature.retry`, `esignature.confirm`,
`esignature.cancel`; folosiți anvelopa comună `type: "command"`, cu
`payload.command` (de exemplu `esignature.start`) și `payload.args`
(de exemplu `{ "session_id": "UUID", "id": "1" }`). Spre deosebire de WSS local,
pierderea conexiunii HTTP/WiseMED nu închide instantaneu sesiunea: folosiți cancel,
altfel o eliberează timeout-ul absolut.

Setări pad: `GET/POST/PUT /api/esignature/settings`.
Istoric (ultimele 100 operații): `GET /api/esignature/jobs`.
Statistici UTC (ultimele 30 zile cu activitate): `GET /api/esignature/stats/daily`.

## Verificare față de barcodeprinter

| Funcție comună | eSignature |
| --- | --- |
| Server HTTP/HTTPS și CA locală | Aceleași module local-http/localtls |
| Administrare, login, setări update și diagnostic | Aceeași interfață comună |
| Headless, serviciu, restart, loguri, versiune embedded | Același runner |
| Config install/merge fără suprascrierea valorilor locale | Același config/releasectl |
| Build/update/release în update server | App ID esignature-server, windows-386, DLL inclus |
| Installer/dezinstalare/shortcut-uri | Același NSIS, cu director x86 |
| Comenzi HTTP și WiseMED WS | Operații esignature, aceeași integrare cu dispecerul |
| Test dispozitiv, istoric și statistici | Captură PNG, istoric operații și statistici confirmări |

ZPL, imprimante și profile poștale rămân funcții exclusiv barcodeprinter.
Captura Signotec nu implementează încă butoane desenate pe pad, previzualizare
live sau compatibilitate cu JSON-ul serverului vendor. DLL-ul Windows nu poate
fi folosit pentru USB din macOS/Linux.

Teste: `go test -race ./modules/signingpad ./modules/localhttp ./shared/localtls
./modules/appupdateserver ./tools/releasectl ./apps/runner`.
Build-ul Windows x86 și ZIP-ul de update au fost generate. Testele automate
verifică WSS/TLS, concurența local/remote, timeout-ul, istoricul și maparea de release.
Captura fizică, instalarea/rularea serviciului pe Windows și login-ul cu cont real
rămân teste pe mediul țintă. NSIS local macOS 3.12 se prăbușește cu std::bad_alloc
inclusiv pe un installer gol; generarea executabilului installer nu a fost validată.
