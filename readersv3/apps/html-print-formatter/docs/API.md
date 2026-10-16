# HTML Print Formatter — ghid de utilizare

Utilitar intern WiseMED pentru documente de încredere. Returnează DOM-ul final
serializat sau generează PDF folosind Chromium, cu endpointuri separate.

## Pornire și configurare

Instalați Google Chrome sau Chromium pe calculatorul serviciului. Chromium nu
este inclus în binar sau în update. Pe Linux sunt necesare bibliotecile sistemului,
fonturile raportului și suportul sandbox al distribuției. Nu rulați Chromium ca
root și nu dezactivați sandbox-ul. Pe Windows instalați Chrome pentru contul
serviciului sau setați calea absolută către instalarea pentru toți utilizatorii.

Din rădăcina `readersv3`:

```sh
scripts/prepare-reader-output.sh html-print-formatter
cd output/html-print-formatter
./html-print-formatter -showlog
```

Pe Windows, executabilul este `html-print-formatter.exe`. Flags comune:
`-config deployments/config.yaml`, `-showlog`, `-version`, `-headless`,
`-installservice`, `-reconfigure`. Pornirea simplă nu cere configurarea unui
echipament WiseMED; `-reconfigure` permite configurarea serverului de login și a setărilor locale,
fără înregistrare medicală.
`-headless` detașează utilitarul ca daemon; Chromium este headless în ambele moduri.

La prima pornire, runner-ul creează `deployments/config.yaml` din
`deployments/config.install.yaml`. Actualizările completează doar cheile lipsă;
valorile operatorului se păstrează. `modules.html-print-formatter` conține:

```yaml
chrome_path: ""
max_concurrent_renders: 2
max_body_bytes: 4194304
default_render_timeout_ms: 120000
```

Variabilele de mediu au prioritate față de YAML și nu sunt salvate în acesta:

| Variabilă | Implicit | Semnificație |
| --- | --- | --- |
| PORT | 19113 în configul livrat | Înlocuiește portul listenerului comun; păstrează adresa de bind și TLS din YAML. 1–65535. |
| CHROME_PATH | autodetecție | Calea executabilului Chrome/Chromium. |
| MAX_CONCURRENT_RENDERS | 2 | 1–64; număr maxim de randări active. |
| MAX_BODY_BYTES | 4194304 | Corp JSON HTTP maxim, 1–67108864 bytes. |
| DEFAULT_RENDER_TIMEOUT_MS | 120000 | 1–600000 ms; timeout general implicit. |

Reporniți utilitarul după modificarea acestor setări. Pentru o altă adresă de bind
folosiți `local_http.address` și `modules.local-http.address` în config. Implicit
serviciul ascultă numai pe `127.0.0.1`, fără TLS. HTTP/HTTPS și setările WSS rămân
cele comune WiseMED. Timeout-urile HTTP sunt 10s pentru headers, 30s pentru citirea
cererii, 660s pentru scriere și 60s idle. Timeout-ul de randare începe după
decodarea/validarea cererii și include așteptarea semaforului, încărcarea,
JavaScript, fonturile, imaginile, cadrele și extragerea.

Actualizările sunt activate implicit: app_id `html-print-formatter`, canal
`stable`, server `http://127.0.0.1:19090`, auto_download `true`. Configurați serverul
real în Setări; controalele de stare/verificare sunt cele comune WiseMED.

## JWT pentru API și chei multiple

În `deployments/config.yaml`, configurați secrete independente. Generați fiecare
secret cu `openssl rand -hex 32`; copiați rezultatul ca text, fără decodare hex.
Nu folosiți textele demonstrative de mai jos ca secrete reale.

```yaml
modules:
  html-print-formatter:
    jwt_keys:
      app-one: "<secret aleator unic de minimum 32 bytes>"
      app-two: "<alt secret aleator unic de minimum 32 bytes>"
    jwt_signing_key_id: app-one
```

Secretele sunt citite numai din YAML, nu din variabile de mediu. Protejați accesul
la fișier. Reporniți serviciul după adăugarea/eliminarea unei chei. Eliminarea unei
chei invalidează tokenurile ei după restart. Toate cheile permit ambele operații;
nu reprezintă roluri diferite. Nu există secret implicit în distribuție.

Trimiteți `Authorization: Bearer <JWT>`. Header JWT obligatoriu: `alg=HS256`,
`kid` identic cu un ID din `jwt_keys`. Claims obligatorii: `iss` și `aud` egale cu
`html-print-formatter`, `sub` nevid (identificator client), `iat` și `exp` ca timestamp
Unix în secunde. `exp` trebuie să fie în viitor; `iat` nu poate fi în viitor.
`nbf` este verificat dacă există. Recomandăm tokenuri de 5 minute și ceasuri sincronizate.
Tokenul este verificat la începutul cererii; expirarea nu întrerupe o conversie în curs.

JWT absent/invalid/expirat produce HTTP 401, cod `unauthorized`. Lipsa cheilor produce
HTTP 503, cod `auth_not_configured`. Sunt protejate și `/render/html`, `/render/url`,
`/api/html-print-formatter/render`, `/api/html-print-formatter/pdf`.

Interfața și testul din helper cer login administrativ și obțin un JWT valabil 5 minute
prin `POST /api/html-print-formatter/token` (JSON, sesiune obligatorie). `jwt_signing_key_id`
alege cheia folosită; lipsa ei blochează emiterea tokenurilor pentru browser, dar nu
verificarea tokenurilor externe. Secretul nu este trimis în browser. Folosiți HTTPS
pentru acces în rețea. WSS păstrează identitatea verificată de puntea WSM și nu cere
acest Bearer suplimentar; un header HTTP nu poate simula identitatea WSS.

## Postman și PHP

Importați `examples/html-print-formatter.postman_collection.json` în Postman.
Colecția conține exact două cereri: `/render` (HTML final) și `/pdf` (PDF direct).
Completați local `base_url`, `jwt_key_id` și `jwt_secret` cu cheia din config.yaml.
Scriptul pre-request semnează automat HS256, cu Web Crypto, și generează corpul JSON.
Variabila `html` acceptă HTML cu ghilimele și linii multiple; pentru un link completați
`html_link` (are prioritate). Linkul folosit pentru `/render` trebuie să seteze `printReady`.
Folosiți Send and Download pentru salvarea răspunsului. Nu distribuiți colecția cu
secrete completate. Exemplul PHP complet este `examples/pdf.php`: setați
`HTML_PRINT_JWT_SECRET`, `HTML_PRINT_JWT_KEY_ID`, opțional `HTML_PRINT_URL`, apoi
serviți scriptul prin PHP cu `report.html` în același director. Scriptul semnează JWT-ul,
apelează `/pdf` și trimite PDF-ul direct în browser, fără salvare pe disc. Pentru
descărcare înlocuiți `inline` cu `attachment` în Content-Disposition. Din CLI,
`php examples/pdf.php report.html` scrie PDF-ul binar la stdout. Variabilele acestea sunt ale clientului PHP.
Pentru comenzile curl de mai jos, setați `JWT_TOKEN` la un token semnat conform acestui contract.

## Utilitar independent și autentificare administrativă

Conversiile HTTP (`/render`, `/pdf` și variantele JSON) cer JWT Bearer. `/health`
rămâne public. Nu sunt necesare unitate medicală, tip de echipament sau echipament ID.
Utilitarul poate fi apelat de orice aplicație prin contractul HTTP documentat.
Rămâne destinat documentelor de încredere și unei rețele interne controlate.

Accesul la configurările administrative cere login valid la server. Configurați
în `modules.wisemed-api` protocolul, hostul, portul, path-ul și cheia API a
serverului de autentificare. Aceste date apar în formularul de inițializare;
nu se mai cer unitate medicală, cod/serie/tip de echipament sau echipament ID.
Serverul trebuie să implementeze contractul existent `PUT /administrative/login`
cu username/password și să accepte autentificarea fără medical_unit_id, returnând
login_token și datele utilizatorului. Nu este un adaptor OAuth/OIDC generic.

Modul este activat automat de protocolul html-print-formatter, inclusiv pentru
config.yaml existent. Nu se fac cereri pentru liste medicale sau înregistrarea
`/administrative/analyzer` la pornire ori după login. Un login eșuat nu deschide
accesul la setări. Sesiunile administrative sunt semnate cu o cheie aleatoare
per proces și expiră la restart.

Prima configurare a serverului este permisă din pagina de login cât timp
conexiunea nu este complet configurată. După aceea, modificarea acestei conexiuni
prin API cere o sesiune autentificată. Cheile și tokenul de login nu sunt expuse
în răspunsurile publice. Pentru corectarea unei adrese greșite care împiedică
loginul, editați config.yaml local și reporniți serviciul.

**WSS rămâne disponibil și configurabil**, inclusiv transportul api.request pentru
HTML și PDF. Activarea conexiunii este opțională și nu blochează funcționarea HTTP.
Protocolul WSM existent cere o identitate de rutare (equipment_id) provisionată
pe server și credențiale WSS; aceasta nu este o condiție pentru pornire/login.
Pentru WSS setați `modules.wisemed-api.echipament_id` la identitatea provisionată
și configurați URL-ul, cheia/tokenul, issuer, tenant și key_id în
`modules.wisemed-ws`, apoi activați conexiunea. Nu este necesară o unitate medicală.
Identitățile/cheile WSS existente sunt păstrate; formatterul nu încearcă să le
reînregistreze prin endpointul medical. Fără identitate WSM, WSS raportează eroarea
în status/loguri, iar conversiile HTTP și administrarea continuă să funcționeze.

## API HTTP

| Metodă și rută | Funcție |
| --- | --- |
| POST /render | Acceptă exact unul dintre `html` și `html_link`; răspuns HTML. |
| POST /render/html | Același contract, dar cere `html`. |
| POST /render/url | Același contract, dar cere `html_link`. |
| POST /api/html-print-formatter/render | Același renderer; răspuns JSON cu `html`, pentru HTTP și adaptorul WSS. |
| POST /pdf | HTML2PDF direct: acceptă `html` sau `html_link`, fără printReady implicit; răspuns application/pdf. |
| POST /api/html-print-formatter/pdf | Aceeași conversie PDF; JSON cu `html` și `pdf_base64`, disponibil și prin WSS. |
| GET /health | 200 când Chromium este disponibil; 503 la pornire/oprire sau dacă a căzut. |
| GET /api/html-print-formatter/health | Echivalent disponibil prin WSS. |

POST cere `Content-Type: application/json`. Corpul poate fi:

```json
{
  "html": "<!DOCTYPE html><html><body><script>document.body.id='ready';window.printReady=true;</script></body></html>",
  "base_url": "https://app.wisemed.eu/",
  "timeout_ms": 120000,
  "viewport_width": 1280,
  "viewport_height": 900
}
```

Sau înlocuiți `html` cu `"html_link": "https://example.org/report.html"`.
Nu trimiteți ambele. `base_url` lipsește implicit. Pentru HTML inline, fără un
`base` sau `base_url`, URL-urile relative se rezolvă pe serverul intern loopback
și de regulă vor eșua. Pentru link se rezolvă față de URL-ul paginii finale după
redirecturi. `html_link` și `base_url` acceptă numai HTTP(S) absolut, fără user/parolă.
Documentele trebuie să fie HTML complet; pentru inserarea unui base în pagini
legate folosiți UTF-8 (nu HTML UTF-16). Câmpurile necunoscute sunt respinse.

Valorile lipsă sau zero folosesc timeout-ul configurat, viewport 1280×900.
Dimensiunile explicite sunt 1–8192 px; timeout explicit 1–600000 ms.

La succes `/render*` returnează documentul direct cu
`Content-Type: text/html; charset=utf-8`, `Cache-Control: no-store` și
`X-Request-ID`. Nu este învelit în JSON. Ruta `/api/.../render` returnează:

```json
{"ok":true,"request_id":"UUID","html":"<!DOCTYPE html>\n<html>...</html>"}
```

Exemplu cu fișierul demonstrativ jQuery livrat în helper:

```sh
curl --fail-with-body -sS http://127.0.0.1:19113/help/request.json -o request.json
curl --fail-with-body -sS http://127.0.0.1:19113/render \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -H 'Content-Type: application/json' \
  --data-binary @request.json -o rendered.html
```

Dacă schimbați portul, adaptați `base_url` în `request.json`.
Pentru un HTML propriu, fără a scăpa manual ghilimelele:

```sh
python3 -c 'import json,pathlib; print(json.dumps({"html":pathlib.Path("report.html").read_text(),"base_url":"https://app.wisemed.eu/"}))' > request.json
curl --fail-with-body -sS http://127.0.0.1:19113/render \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -H 'Content-Type: application/json' --data-binary @request.json -o rendered.html
```

Exemplu link (înlocuiți adresa raportului):

```sh
curl --fail-with-body -sS http://127.0.0.1:19113/render/url \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -H 'Content-Type: application/json' \
  --data '{"html_link":"https://example.org/report.html","timeout_ms":120000}' \
  -o rendered.html
```

La eroare răspunsul este JSON, chiar și pe `/render`:

```json
{"ok":false,"request_id":"UUID","error":{"code":"page_error","message":"Mesajul paginii"}}
```

| Cod stabil | HTTP | Explicație |
| --- | --- | --- |
| invalid_json | 400 | JSON invalid, câmp necunoscut sau mai multe obiecte. |
| invalid_request | 400 | Sursă absentă/ambiguă, URL sau parametri nevalizi. |
| method_not_allowed | 405 | Metodă nepotrivită. |
| unsupported_media_type | 415 | Lipsește application/json. |
| body_too_large | 413 | Corp peste limită; și documentul legat atunci când se injectează base. |
| page_error | 422 | `window.printError` are o valoare diferită de null/undefined, inclusiv șir gol sau false. |
| render_timeout | 504 | Deadline general depășit; `printReady` lipsă sau resurse nefinalizate. |
| render_canceled | 408 | Client deconectat, context anulat sau shutdown; clientul închis nu mai poate primi răspunsul. |
| source_load_failed | 502 | Documentul legat a răspuns cu status HTTP de eroare. |
| render_failed | 502 | Navigare, CDP, JavaScript de extragere sau browser eșuat. |
| pdf_failed | 502 | Comanda Chromium PrintToPDF a eșuat. |
| browser_unavailable | 503 | Browser nepornit/oprit; verificați health și logurile. |

## HTML2PDF separat: POST /pdf

`/pdf` acceptă aceleași câmpuri de sursă, base_url, viewport și timeout ca
`/render`, dar **nu așteaptă window.printReady implicit**. Este potrivit și pentru
HTML static sau pagini HTTP(S) obișnuite. Scripturile paginii se execută normal;
serviciul așteaptă document.readyState complete, fonturile, imaginile și două
cadre. Nu așteaptă automat operații AJAX/timere care continuă după încărcare.
Pentru acestea, pagina trebuie să semnalizeze explicit finalizarea.

Opțional, `wait_for_print_ready: true` activează așteptarea obligatorie a
`window.printReady === true` pentru PDF. Valoarea implicită este false, numai
pentru endpointurile PDF. Endpointurile `/render` cer întotdeauna printReady,
indiferent de acest câmp. `window.printError` produce eroare în ambele moduri.

```sh
curl --fail-with-body -sS http://127.0.0.1:19113/pdf \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -H 'Content-Type: application/json' \
  --data '{"html":"<!DOCTYPE html><html><body><h1>Raport PDF</h1></body></html>","timeout_ms":120000}' \
  -o rendered.pdf

curl --fail-with-body -sS http://127.0.0.1:19113/pdf \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -H 'Content-Type: application/json' \
  --data '{"html_link":"https://example.org/","timeout_ms":120000}' \
  -o rendered.pdf
```

Pentru fișiere HTML cu ghilimele, apostrofuri și rânduri noi, folosiți un encoder:

```sh
python3 -c 'import json,pathlib; print(json.dumps({"html":pathlib.Path("report.html").read_text()}))' > pdf-request.json
curl --fail-with-body -sS http://127.0.0.1:19113/pdf \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -H 'Content-Type: application/json' --data-binary @pdf-request.json -o rendered.pdf
```

Răspunsul de succes are Content-Type application/pdf, Content-Disposition
attachment; filename="rendered.pdf", X-Request-ID și Cache-Control no-store.
Erorile rămân JSON, cu aceleași coduri ca randarea; `pdf_failed` (502) indică
un eșec al comenzii de imprimare Chromium. Timeout-ul include generarea PDF.

Varianta HTTP/WSS `/api/html-print-formatter/pdf` întoarce:

```json
{"ok":true,"request_id":"UUID","html":"<html>...</html>","pdf_base64":"JVBERi0...","filename":"rendered.pdf"}
```

Clientul decodează pdf_base64 în bytes application/pdf. Limita comună WSS este
4 MiB pentru răspunsul JSON complet (DOM + PDF în Base64, care adaugă aproximativ
33%); pentru rapoarte mari folosiți direct `/pdf` prin HTTP. Timeout-ul adaptorului
și al clientului de control WSS este 610 secunde pentru rutele formatterului;
limita efectivă a lucrării rămâne timeout_ms, cel mult 600000.

În pagina principală și în helper există câmpuri separate HTML, Link HTML și
Timeout (implicit 120000). Butonul **Randează → PDF** așteaptă printReady și
produce DOM-ul final plus linkul Descarcă PDF. Butonul **HTML → PDF direct**
folosește același endpoint PDF cu wait_for_print_ready false. Se completează
exact una dintre surse. JSON-ul este construit automat, fără escapare manuală.

PDF-ul se generează cu chromedp/Page.printToPDF din aceeași pagină vie după
stabilizarea resurselor. DOM-ul nu este încărcat din nou; scripturile documentului
nu sunt reexecutate printr-o a doua navigare. Chromium poate declanșa evenimentele
standard beforeprint/afterprint; acestea și timerele paginii pot modifica DOM-ul,
așa că raportul trebuie să rămână stabil după semnalul de finalizare.

Implicit se folosește A4 portret, margini 0, fundaluri incluse, fără antet/subsol
Chromium. CSS `@page { size: A4; margin: 12mm; }` poate stabili formatul și
marginile (preferCSSPageSize activ). Încărcarea și procesarea inițială folosesc
media **screen**. Înainte de exportul PDF se activează explicit media **print**:
regulile `@media print` se aplică, iar elementele ascunse pentru imprimare nu apar
în PDF. Se așteaptă reacția la schimbarea media, fonturile, imaginile și cadrele
necesare stabilizării, în limita aceluiași timeout. `/render` rămâne în screen.
Calculele de paginare făcute anterior în JavaScript nu sunt refăcute automat. Pentru paginare folosiți @page și break-before /
break-after / break-inside. PDF-ul păstrează aspectul randat, inclusiv canvas,
nu codul JavaScript interactiv. Nu este automat PDF/A sau document semnat.

### Alegerea motorului

Pentru acest utilitar alegem Chromium + chromedp: browserul există deja în
procesul de randare, execută jQuery și suportă CSS-ul browserului; PrintToPDF
exportă direct pagina procesată. Nu este necesar un al doilea serviciu.
Gotenberg oferă API-uri HTML/URL → PDF tot prin Chromium, util când se dorește
un serviciu separat. wkhtmltopdf folosește un motor QtWebKit învechit, nepotrivit
pentru a înlocui Chromium în aceste rapoarte. Surse consultate:
https://chromedevtools.github.io/devtools-protocol/tot/Page/#method-printToPDF
https://gotenberg.dev/docs/convert-with-chromium/convert-html-to-pdf
https://wkhtmltopdf.org/status

## WiseMED WebSocket

Se folosește transportul standard WSM, comanda `api.request`, fără un WebSocket
suplimentar. Configurați identitatea provisionată WSM și setările WSS, activați
`modules.wisemed-ws.enabled`, provisionați identitatea/cheia echipamentului pe WSM.
WSS este oprit în template până sunt disponibile credențiale valide. Nu există
chei hardcodate. Clientul trebuie să aibă scope `api:invoke` și drepturi asupra
echipamentului. Vezi și documentația comună `readersv3/docs/wss-control.md`.

Comanda trimisă de client după autentificare și hello_ack (înlocuiți equipment_id):

```json
{
  "type": "command",
  "request_id": "render-001",
  "target": {"mode": "equipment", "equipment_id": "ID-ECHIPAMENT"},
  "payload": {
    "command": "api.request",
    "args": {
      "method": "POST",
      "path": "/api/html-print-formatter/render",
      "body": {
        "html": "<!DOCTYPE html><html><body><script>document.body.id='done';window.printReady=true</script></body></html>",
        "timeout_ms": 120000
      }
    }
  }
}
```

Reply-ul standard are `correlation_id: "render-001"`, iar în payload:
`kind: "api.response"`, `status: 200`, `content_type: "application/json; charset=utf-8"`
și `body: {"ok":true,"request_id":"UUID","html":"..."}`. La eroare se păstrează
statusul HTTP și obiectul `error` în `payload.body`. Erorile transportului apar
în envelope-ul standard WSM, separat de erorile rendererului.

Ruta JSON folosește exact același renderer și aceleași validări; adaptorul WSS
permite pentru această rută până la 610s, fără limita generică de 30s. Clientul
trebuie să aibă un timeout propriu suficient. Anularea sesiunii WSS a utilitarului
anulează cererea în curs; protocolul comun nu oferă anulare individuală a unui
render la simpla închidere a ferestrei clientului remote. Deadline-ul rămâne activ.
Comenzile WSS ale unei sesiuni sunt executate secvențial de transportul comun.

Adaptorul are limita proprie de 4 MiB pentru request și response JSON; HTML-ul
escapat și envelope-ul consumă spațiu suplimentar. Readerul acceptă frame-uri până
la 8 MiB, iar WSM are implicit `server.max_message_bytes: 1048576`. Configurați
limitele WSM pentru documentele utilizate, fără a depăși limita adaptorului.
Pentru documente mari folosiți HTTP direct. Ruta `/render` cu răspuns text/html
nu se transportă prin adaptor; folosiți ruta JSON de mai sus.

## Încărcare, procesare și serializare

HTML-ul inline este servit ca răspuns HTTP real pe un listener intern
`127.0.0.1:<port-aleator>/documents/<UUID>`. Chromium navighează la acel document;
parserul HTML execută normal scripturile inline/externe și jQuery document.ready.
Documentul temporar este ținut numai în memorie și eliminat la finalul cererii.
Nu încărcăm sursa prin `innerHTML`, nu înlocuim scripturile și nu folosim `eval`
pentru a reexecuta scripturile furnizate.

Pentru `html_link`, Chromium navighează direct, cu redirecturi, cookies și
politicile browserului în contextul izolat. Când este dat `base_url`, răspunsul
documentului este interceptat prin CDP Fetch înainte de parsare și este inserat
un `<base>` numai dacă documentul nu are deja unul. Se păstrează restul sursei,
scripturile și headers precum CSP. Pentru inline se face aceeași inserare înainte
de a servi documentul. Un tokenizer localizează punctul de inserție, fără
reserializarea documentului sursă; textele `<base>` din scripturi/comentarii nu
sunt confundate cu elemente. Un base existent are prioritate.

Viewport-ul este setat înaintea navigării, deviceScaleFactor=1, media **screen**.
Pentru `/render` și PDF cu `wait_for_print_ready: true`, finalizarea cere strict
`window.printReady === true`. PDF direct folosește document.readyState complete. O valoare în
`window.printError` are prioritate și devine `page_error` cu mesajul paginii.
Interogarea periodică de 20ms verifică semnalul; nu este un timp fix după care
se presupune succes. Erorile JavaScript neinterceptate sunt logate, dar numai
`printError` este semnalul explicit de eșec al raportului; fără `printReady`,
cererea expiră. Nu folosiți `window.print()`, dialoguri sau interacțiune manuală.

După semnal se așteaptă `document.fonts.ready`, toate elementele `document.images`
din acel moment (load sau error) și două cadre `requestAnimationFrame`, în același
deadline. Imaginile lazy sunt activate pentru încărcare. O imagine stricată este
considerată terminată, nu generează automat page_error. Așteptarea nu include
imagini CSS background, iframe-uri separate, video sau cereri AJAX arbitrare.
Raportul trebuie să termine propriile operații asincrone înainte de printReady.

**Raportul trebuie să aștepte fonturile și imaginile înainte de propriile calcule
de paginare dacă influențează dimensiunile. Așteptarea de la final nu repară
calculele făcute deja cu dimensiuni greșite.** Exemplul `example.html` din helper
arată explicit această ordine, apoi rulează sincron `compact_sections()`,
`normalize_content()` și `move_content()`, înainte de a seta printReady.

Rezultatul este `document.documentElement.outerHTML`, precedat de DOCTYPE dacă
există (serializat cu XMLSerializer). Include elementele mutate/generate și
atributele/stilurile inline. Nu include stilurile calculate, ascultătorii de
evenimente, starea JavaScript, DOM-ul intern al iframe-urilor/shadow roots sau
bitmap-urile canvas. Nu este o arhivă autonomă: resursele externe nu sunt inline.

Scripturile rămân în HTML. Serviciul nu le reexecută după extragere, dar ele se
pot executa din nou dacă deschideți rezultatul în browser. Pentru a vedea sursa
exactă salvați fișierul și inspectați-l ca text. Pentru un raport legat fără base,
URL-urile relative din rezultat necesită publicarea la URL-ul original sau un
base potrivit; serializarea nu le transformă automat în URL-uri absolute.

## Origine, securitate și operare

`<base>` schimbă rezolvarea URL-urilor, **nu originea documentului și nu CORS**.
Originea unui document inline este listenerul loopback intern, indiferent de
base_url. AJAX/fetch către aplicația WiseMED rămâne cross-origin și are nevoie
de permisiunile CORS, de autentificare și de regulile cookies/SameSite corespunzătoare.
Pentru link, originea este URL-ul paginii navigate. Restricțiile CSP, mixed content
și Local Network Access rămân active; nu dezactivăm securitatea browserului.

Un singur proces Chromium este reutilizat. Fiecare cerere primește un browser
context incognito separat și un tab propriu. Cookies/localStorage/cache de context
nu sunt partajate între cereri. Contextul este închis inclusiv după eșec la crearea
tabului, timeout și anulare. Dacă anularea apare în timpul alocării contextului,
se recuperează întâi ID-ul acestuia pentru închidere (buget separat de cel mult 5s).
Eliberarea contextului are cel mult 3s; o alocare/închidere CDP eșuată oprește
browserul pentru a nu lăsa contexte active și necesită restartul utilitarului.
Aceste bugete sunt pentru cleanup, fără continuarea procesării documentului.
Procesul browser este oprit la shutdown; runtime-ul
așteaptă închiderea modulelor. Un crash al procesului afectează randările active;
health devine 503, iar serviciul trebuie repornit. Nu reîncercăm automat rapoartele.

Chromium rulează sandboxed; inclusiv comportamentul chromedp care ar adăuga
automat `--no-sandbox` pentru root este dezactivat. Restricțiile sistemului privind
sandbox-ul trebuie rezolvate în instalare, nu prin dezactivarea protecției.

Endpointurile de randare HTTP sunt interne și protejate prin JWT HS256. Login-ul WiseMED protejează administrarea și WSS cere identitate
verificată. Nu publicați serviciul direct pe internet: HTML-ul execută JavaScript
cu acces la rețeaua serverului, inclusiv servicii interne. Limita de corp și
semaforul nu sunt limite de memorie/CPU pentru Chromium și nu fac sigur codul
neîncredere. Linkurile și subresursele pot descărca date suplimentare; limita
MAX_BODY_BYTES nu limitează totalul traficului browserului.

Mesajele consolei, erorile JavaScript și mesajele de rețea sunt în logurile
standard WiseMED, cu `request_id` (același din răspuns). Conținutul logat de pagina
însăși poate include date sensibile; protejați accesul la logurile serviciului.

## Build, distribuție și teste

```sh
go build ./apps/html-print-formatter
REQUIRE_CHROME_TESTS=1 go test -race ./modules/htmlprintformatter
go test ./core/runtime ./core/config ./shared/apibridge ./modules/localhttp ./modules/ws ./apps/runner ./tools/releasectl
make build APP=html-print-formatter TARGET=windows-amd64
make update APP=html-print-formatter TARGET=windows-amd64
make installer APP=html-print-formatter TARGET=linux-amd64
```

Wrapper-ele sunt în `apps/html-print-formatter/build/`, pentru Windows, Linux și
macOS. PNG și ICO multi-rezoluție sunt în `resources/app-icons`. Installerul Linux
folosește un cont dedicat `html-print-formatter`, nu root, pentru sandbox-ul Chrome;
are nevoie de `useradd`/`chown`. Contul poate scrie în directorul instalării pentru
config/loguri/update. Serviciile acestui utilitar rulează în foreground sub
managerul de servicii, fără flagul de detașare `-headless`.
Pe Linux, `-installservice` creează același cont și setează proprietarul directorului
aplicației; copiați executabilul într-un director dedicat instalării înainte de
a folosi flagul. Installer-ele native cer uneltele platformei (`makensis`,
`pkgbuild` etc.); compilarea unui executabil nu testează instalarea serviciului.

Testele includ browser real pentru succes jQuery, CSS screen/viewport, păstrarea
base, link/redirect, timeout obligatoriu, printError, anulare, izolarea storage,
concurență, logging, parity HTTP/WS și eliberarea contextelor. Fără Chromium,
testele de integrare sunt skip; `REQUIRE_CHROME_TESTS=1` le face obligatorii.
Fixtures folosesc jQuery 3.7.1 livrat local (licența MIT în fișier), fără dependență
de CDN în timpul testării.

Referințe tehnice: https://pkg.go.dev/github.com/chromedp/chromedp și
https://chromedevtools.github.io/devtools-protocol/tot/Fetch/.
Crearea tabului omite explicit parametrul CDP `newWindow` pentru compatibilitate
cu Chromium headless: https://issues.chromium.org/issues/454825274.
