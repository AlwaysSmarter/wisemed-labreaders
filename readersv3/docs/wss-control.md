# WiseMED WSS — readere și utilitare

Transportul comun, configurarea, API-ul și fereastra de control remote sunt descrise
în [documentația WSM](../../wsm-server/docs/remote-control.md).

În toate readerele și utilitarele care folosesc WSM: **Setări → WSS**. Configurați
URL WSS și autentificarea, salvați și reconectați. Serverul trebuie provisionat cu
aceleași chei/tenant/issuer și drepturi. Nu puneți cheia principală a tenantului pe
reader; folosiți cheie legată de echipament sau token obținut de la backend.

Readerul apelează întâi API-ul HTTP de înregistrare a echipamentului. După un ID
valid, se conectează la WSS. Topbarul devine verde numai după hello_ack; la întrerupere
arată secundele până la următoarea încercare (30s). Click reîncearcă imediat.

**Debug → WSM** (`/debug/wsm`) permite lista clienților, ping, mesaj, reconnect, apelarea API prin WSS și
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


## Formular comun de configurare

Toate cele 28 de aplicații care înregistrează modulul `wisemed-ws` folosesc
aceeași interfață încorporată, inclusiv în consola remote. Adresa este
`/settings/wss`, în submeniul Setări → WSS.

- Cheie echipament: tenant, ID cheie, emitent, audiență, subiect, cheie/fișier
  cheie și permisiunile solicitate.
- Fișier JWT: calea fișierului JWT; permisiunile vin din token.
- Endpoint JWT: calea endpointului WiseMED; permisiunile vin din token.
- Activarea, adresa WSS, ID-ul clientului și CA suplimentar rămân comune.
  Schimbarea metodei ascunde câmpurile neaplicabile și păstrează valorile salvate.

Permisiunile apar individual ca checkbox-uri numerotate, în aceeași ordine ca în consola admin WSM:

1. `route:reply`
2. `route:event`
3. `route:command`
4. `devices:debug`
5. `connections:read`
6. `server:status`
7. `api:invoke`
8. `api:admin`
9. `route:broadcast`
10. `topics:subscribe`
11. `wisemed:proxy`

Selecția existentă se păstrează; afișarea tuturor opțiunilor nu le activează automat.
Cheia serverului trebuie să permită permisiunile selectate.

Fiecare permisiune are o pictogramă „i” accesibilă prin click, atingere sau tastatură. Deschide explicația dreptului, dependențele sale și un exemplu concret, fără a modifica selecția checkbox-ului.

Settings → WSS conține numai configurarea și Deconectare / Reconectare acum. Debug → WSM conține starea conexiunii, lista clienților, ping, mesaje, test API, jurnalul WSS și aceleași acțiuni de conectare. Restricțiile consolei remote rămân aplicate.

Reconectare acum afișează un loader și este blocat simultan în Settings → WSS și Debug → WSM cât timp conectarea sau reîncercările automate sunt în curs. Butoanele se reactivează după confirmarea unei conexiuni noi sau după Deconectare. Deconectarea manuală rămâne disponibilă local; restricția de deconectare din consola remote se păstrează.
