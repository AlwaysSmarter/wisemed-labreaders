# HTML Print Formatter — WiseMED

Utilitar WiseMED v3 cu `chromedp`: primește HTML sau un link HTTP(S), execută
scripturile în Chromium headless și returnează DOM-ul final sau un PDF.
`POST /pdf` convertește HTML/link direct, fără să ceară `printReady`;
`POST /render` păstrează randarea WiseMED cu semnal obligatoriu. Folosește runner-ul,
HTTP/HTTPS, configurarea YAML, login-ul administrativ, WSS, logurile, helper-ul,
actualizările și instalarea ca serviciu comune utilitarelor WiseMED.

Instrucțiunile complete, contractul API/WS și limitările sunt în
[docs/API.md](docs/API.md) și în helper-ul livrat la `/help/`.

Pornire locală, din `readersv3` (Chrome/Chromium trebuie instalat):

```sh
scripts/prepare-reader-output.sh html-print-formatter
cd output/html-print-formatter
./html-print-formatter -showlog
```

Port implicit: `127.0.0.1:19113`. Verificare: `GET /health`.
Descărcați `http://127.0.0.1:19113/help/request.json` și executați:

```sh
curl --fail-with-body -sS http://127.0.0.1:19113/render \
  -H 'Content-Type: application/json' \
  --data-binary @request.json -o rendered.html
```

Testele Chromium reale pot fi impuse în CI:

```sh
REQUIRE_CHROME_TESTS=1 go test -race ./modules/htmlprintformatter
go test ./core/runtime ./core/config ./shared/apibridge ./modules/localhttp ./modules/ws ./apps/runner ./tools/releasectl
go build ./apps/html-print-formatter
```
