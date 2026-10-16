# Migrare din WSM Server demonstrativ

Această versiune introduce un contract de securitate nou. Nu pornește cu vechiul
`security.accepted_keys` și `wisemed` global; schema strictă semnalează migrarea.
Fișierul existent `deployments/config.yaml` a fost păstrat, fără rescrierea secretelor
sau a parametrilor utilizați anterior. Folosiți un fișier nou pornind de la
`config.production.yaml`.

## Configurație și clienți

| Înainte | Acum |
| --- | --- |
| Un map subject → secret | `security.keys.<kid>` → tenant, secret extern, roluri, scopes |
| Un upstream WiseMED global | `tenants.<id>.wisemed` separat, opțional |
| HTTP/ws public | HTTPS/wss direct sau Nginx TLS; HTTP backend numai prin opt-in explicit și acces privat |
| Token cu sub/role | JWT obligatoriu cu kid, tenant, issuer, audience, identitate, scopes, iat, exp |
| Token de două ore | Implicit maximum 15 minute, socket închis la expirare |
| Token emis de `/api/test-token` | Backendul WiseMED emite token după autentificare; CLI doar offline |
| `/test/*` servit de server | Nu mai este servit; fișierele web vechi sunt referință istorică |
| Diagnostic public | Bearer + connections:read, filtrat per tenant |
| `broadcast=true` / target lipsă | `target.mode=all` explicit + route:broadcast |
| Mai multe conexiuni aceluiași reader | O singură conexiune per tenant/reader_id |
| Loguri în deployments | stderr/journald, fără payload/token |

Pașii backendului WiseMED:

1. Configurați propriul tenant/kid/issuer și primiți cheia numai pe backend.
2. Adăugați emiterea de tokenuri după autentificarea și autorizarea utilizatorilor
   și readerelor. Backendul decide sub/client_id/reader_id/scopes; nu semnează
   nevalidat aceste valori preluate din cererea browserului.
3. Pentru browser, folosiți subprotocolul JWT și Origin explicit; pentru reader,
   Bearer Authorization. URI-ul public este WSS cu certificat valid.
4. Trimiteți hello care corespunde identității semnate; așteptați hello_ack.
5. Implementați refresh token/reconectare/backoff și refacerea abonamentelor.
6. Mutați broadcasturile la target explicit și gestionați ACK/reply separat.
7. Verificați doi tenanți cu aceleași reader_id/topicuri și confirmați izolarea.

## Integrarea readersv3 existentă

Modulul `readersv3/modules/ws/module.go` a fost migrat: inițializează echipamentul
prin API HTTP înainte de WSS, folosește JWT nou și Bearer, suportă cheie dedicată,
token endpoint sau token file, afișează starea după hello_ack și reîncearcă la 30s.
Toate cele 26 de aplicații echipament includ modulul. Nu se mai utilizează vechiul
fallback de semnare cu cheia globală API.

Sursa nouă trebuie recompilată și configurația WSS provisionată; vechile executabile
nu pot folosi contractul nou. Pentru schema exactă și fereastra de control reutilizată,
consultați [Control remote](remote-control.md).

`readersv3/modules/wisemedapi/module.go` apelează cele două comenzi proxy. Scopul
`wisemed:proxy` trebuie acordat explicit, iar upstreamul/API secret configurate per
tenant. Payload-ul `args.reader` este obiectul administrativ al echipamentului
(inclusiv `cod_echipament` etc.), nu identitatea hello. Serverul nu presupune că
acest obiect are un câmp `id` egal cu reader_id.

## Validări înainte de activare

Certificat real și DNS public; secrete independente pe instanță; JWT-uri noi;
conectare browser + reader; 1:1 și 1:N; topicuri; reconectare la expirare/reload;
verificare cu WiseMED real a celor două rute proxy și a formelor de răspuns.
Serverul a fost testat automat cu upstreamuri TLS simulate, nu cu date medicale sau
infrastructură live. Înlocuiți vechiul deployment numai după migrarea clienților.
