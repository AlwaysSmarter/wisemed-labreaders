# anaf-docsmart

`anaf-docsmart` este acum o aplicație `readersv3` de tip utilitar, pe același model cu utilitarele existente.

Scopul actual:

- se autentifică cu login WiseMED;
- citește XML-urile din directoarele configurate prin `transport-file`;
- în interfața locală afișează fișierele parse în pagina `/orders`;
- ascunde fluxurile clasice de reader pentru analize, QC și trimitere rezultate.

## Structură

- `main.go`: entrypoint-ul utilitarului.
- `deployments/config.install.yaml`: configurația standard pentru bootstrap/install.
- `deployments/help/index.html`: help local.
- `assets/`: fișiere de exemplu.

## Rulare locală

Din rădăcina `readersv3`:

```bash
go run ./tools/releasectl build --app anaf-docsmart --target darwin-amd64
```

Sau direct:

```bash
go run ./apps/anaf-docsmart -config apps/anaf-docsmart/deployments/config.install.yaml
```

## Comportament

- `transport-file.import_dir`: fișiere XML noi;
- `transport-file.processed_dir`: fișiere procesate;
- `transport-file.failed_dir`: fișiere cu erori;
- `protocol-anaf-docsmart.templates_dir`: șabloane PDF Smart;
- `protocol-anaf-docsmart.work_dir`: lucru temporar pentru Acrobat;
- UI-ul local afișează metadatele parse din XML, inclusiv instituție, dată, totaluri și primele conturi.

## Flux PDF

- XML-ul este citit din `inbox`.
- Primul PDF care se potrivește în `templates_dir` este folosit ca șablon.
- Utilitarul generează PDF-ul completat în `outbox`.
- XML-ul este mutat în `processed` sau `failed`.
