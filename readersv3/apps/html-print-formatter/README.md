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

Pornirea și conversiile nu cer unitate medicală sau echipament ID. Administrarea
folosește loginul la server; WSS rămâne disponibil opțional cu identitate WSM
provisionată separat.

Configurați `modules.html-print-formatter.jwt_keys` și `jwt_signing_key_id` în
`deployments/config.yaml` înainte de conversii. API-ul cere JWT HS256 Bearer; fără
chei accesul este blocat. Contractul JWT și exemplele PHP/Postman sunt în ghid.

Port implicit: `127.0.0.1:19113`. Verificare: `GET /health`.
Descărcați `http://127.0.0.1:19113/help/request.json` și executați:

```sh
curl --fail-with-body -sS http://127.0.0.1:19113/render \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -H 'Content-Type: application/json' \
  --data-binary @request.json -o rendered.html
```

Testele Chromium reale pot fi impuse în CI:

```sh
REQUIRE_CHROME_TESTS=1 go test -race ./modules/htmlprintformatter
go test ./core/runtime ./core/config ./shared/apibridge ./modules/localhttp ./modules/ws ./apps/runner ./tools/releasectl
go build ./apps/html-print-formatter
```

## Ubuntu Linux / Docker

The multi-stage [Dockerfile](docker/ubuntu/Dockerfile) compiles the Linux binary from Go sources
and copies it into an Ubuntu 24.04 image with Chrome and fonts. See
[docker/ubuntu/README.md](docker/ubuntu/README.md) for amd64/arm64 build commands, persistent
configuration, sandbox requirements, and PDF examples.
