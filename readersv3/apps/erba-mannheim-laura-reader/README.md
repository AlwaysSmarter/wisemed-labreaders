# Erba Mannheim Laura Reader

Reader V3 standard pentru rapoartele DEKAPHAN LAURA observate în `docs/erba-mannheim-laura`.

- Serial implicit, TCP server/client; configurare din interfața și wizard-ul standard.
- Panou fix de 10 analite: BLD, BIL, UBG, KET, GLU, PRO, PH, NIT, LEU, SG.
- Captura de la 13:09 conține un raport STX/ETX valid; cea de la 14:24 conține octeți corupți. Nu se presupune ASTM, checksum, ACK sau suport pentru cereri de comenzi.
- 9600/8N1 este valoare inițială configurabilă, neconfirmată de capturi.
- Rapoarte complete obligatorii; mesaje incomplete/corupte sunt respinse înainte de salvare.
- Salvare atomică și identificator unic per panou; istoric grupat inclusiv pentru două repetări primite în aceeași secundă. Identitatea comenzii urmează reader-ul standard: dată + rundă + ID.
- Funcțiile standard WiseMED sunt incluse; nu se deduc metadate QC inexistente în protocol.

Din `readersv3`:

```sh
./scripts/prepare-reader-output.sh erba-mannheim-laura-reader
go run ./tools/releasectl build --app erba-mannheim-laura-reader --target windows-amd64
go test ./modules/protocols/erbamannheimlaura ./modules/storage/sqlite ./modules/localhttp ./apps/runner
```

Pornire: `erba-mannheim-laura-reader --config deployments/config.yaml`.
La prima pornire, runner-ul folosește `config.install.yaml` și wizard-ul comun.
