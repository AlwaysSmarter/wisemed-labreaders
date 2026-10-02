# WiseMED WSS — readere și utilitare

Transportul comun, configurarea, API-ul și fereastra de control remote sunt descrise
în [documentația WSM](../../wsm-server/docs/remote-control.md).

În fiecare dintre cele 26 aplicații echipament: **Setări → Debug WSS**. Configurați
URL WSS și autentificarea, salvați și reconectați. Serverul trebuie provisionat cu
aceleași chei/tenant/issuer și drepturi. Nu puneți cheia principală a tenantului pe
reader; folosiți cheie legată de echipament sau token obținut de la backend.

Readerul apelează întâi API-ul HTTP de înregistrare a echipamentului. După un ID
valid, se conectează la WSS. Topbarul devine verde numai după hello_ack; la întrerupere
arată secundele până la următoarea încercare (30s). Click reîncearcă imediat.

Debug WSS permite lista clienților, ping, mesaj, reconnect, apelarea API prin WSS și
„Deschide” pentru aceeași interfață într-o fereastră remote. Administrarea WSS locală
cere administrator. Fereastra remote folosește doar JSON WSS pentru date; nu cere
acces direct la IP-ul readerului și nu transferă HTML prin canal.

Testare:

```sh
cd readersv3
go test -race ./...
go vet ./...
node --test modules/localhttp/ui/wss-ui.test.cjs modules/localhttp/ui/wss-remote.test.cjs
cd ../wsm-server/integration
go test -race -v ./...
```

Testul de integrare durează aproximativ 31s pentru a verifica retry-ul real de 30s.
Operațiile asupra analizorului/PAD/imprimantei necesită hardware și drivere locale;
transportul nu le emulează.

Configurarea WSS se salvează în fișierul YAML activ al aplicației, sub
`modules.wisemed-ws`, nu în SQLite. URL/enabled sunt oglindite și în `wisemed_ws`
pentru compatibilitate. Fișierele de cheie/JWT/CA sunt referințe la fișiere externe.
Trace-ul Debug WSS este în memorie și se pierde la restart. Câmpurile absente din
configurații vechi nu se completează automat cu identități inventate; tenant_id,
key_id și issuer trebuie provisionate conform serverului. Din 2026-09-23 există
un singur indicator WSS cu countdown; vechiul WiseMEDWS duplicat a fost eliminat.
Recompilați readerul pentru actualizarea interfeței încorporate în executabil.
