# Context compact pentru sesiunile următoare

- **Scop:** orientare rapidă pentru un agent care trebuie să găsească locul corect al unei
  schimbări și impactul ei. Nu înlocuiește documentele detaliate; trimite la ele.
- **Data:** 2026-11-14. Starea descrisă este cea a commit-ului `d53088e` plus modificările
  locale ale utilizatorului.
- **Regulă:** memoria (`aimemory/`) și README-urile sunt orientare. Verifică în cod.

## 1. Harta repository-ului

| Zonă | Ce este |
|---|---|
| `readersv3/` | modul Go `wisemed-labreaders/readersv3` (go 1.24): framework + 32 de aplicații în `apps/` |
| `wsm-server/` | modul Go separat: hub WSS multi-tenant (`cmd/wsm-server`, `cmd/wsmctl`) |
| `wsm-server/integration/` | modul Go separat: testul `roundtrip_test.go` |
| `go.mod` (rădăcină) | modul orfan, fără cod |
| `apis/` | stratul REST PHP al backendului WiseMED (`apiv2`, `mp-api`), fără logica de business |
| `analyzers-comm-protocols/`, `docs/` | specificații și materiale de la producători / CNAS |
| `old-src/` | surse C# istorice (ignorate de git), referință pentru SIUI și semnătură |
| `DocSMART/` | rămășiță; implementarea e `readersv3/apps/anaf-docsmart` |
| `output/` (rădăcină) | pachetul de distribuție WSM browser SDK |
| `aimemory/` | memorie de proiect; prima jumătate din `wisemed-labreaders.md` e depășită |

Rulează comenzile Go din directorul modulului (`readersv3/`, `wsm-server/`,
`wsm-server/integration/`).

## 2. Unde modifici ce

| Vrei să schimbi | Te uiți în |
|---|---|
| lista de module a unei aplicații | `readersv3/apps/<app>/main.go` (hardcodată; nu vine din YAML) |
| înregistrarea unui ID de modul | `readersv3/modules/builtin/register.go:54` |
| pornire, bootstrap, headless, serviciu, update la pornire | `readersv3/apps/runner/` |
| lifecycle, mux, registry de servicii | `readersv3/core/runtime/runtime.go`, `core/module/` |
| config, merge `config.install.yaml`, migrări | `readersv3/core/config/` |
| UI local, sesiuni, orders, QC, sincronizare, trimitere | `readersv3/modules/localhttp/` (cea mai mare parte a logicii) |
| schema SQLite a readerelor | `readersv3/modules/storage/sqlite/` |
| un protocol de analizor (inclusiv I/O serial/TCP) | `readersv3/modules/protocols/<nume>/`; baza pentru fișiere: `protocols/fileimportbase` |
| client WiseMED API | `readersv3/modules/wisemedapi/` |
| client WSM, comenzi de dispatcher | `readersv3/modules/ws/` |
| `api.request` (WSS → handler local) | `readersv3/shared/apibridge/bridge.go` |
| hub, autentificare, rutare WSM | `wsm-server/internal/server/` (`auth.go`, `hub.go`, `server.go`) |
| wrapper HTTP de comenzi (PHP) | `wsm-server/internal/server/http_commands.go` |
| SDK browser | `wsm-server/internal/server/sdk/` (`wsm.js`, `siui-bridge.js`) |
| operații SIUI/SIPE/DocMED | `readersv3/modules/siui/` (`operations.go`, `assets/operations.json`, `assets/docmed_operations.json`) |
| arhivare SIUI | `readersv3/modules/siuihistory/`; outbox în `modules/siui` |
| nomenclatoare SIUI | `readersv3/modules/siuidata/` |
| WiseMED AI | `readersv3/modules/wisemedai/`, UI în `readersv3/apps/wisemed-ai/web` |
| actualizări | `readersv3/shared/appupdates`, `apps/runner/updates.go`, `modules/appupdateserver` |
| build, installer, release | `readersv3/tools/releasectl`, `readersv3/Makefile`, `apps/<app>/build` |

## 3. Fapte care surprind (toate confirmate din cod)

- `InitModules` rulează de două ori pe darwin/linux (`apps/runner/runner.go:143`, `:264`).
- Ordinea modulelor e strict cea din slice; `Start` rulează în paralel, iar primul `Start`
  care se întoarce (chiar cu `nil`) oprește tot runtime-ul.
- `transport-serial` și `transport-tcpip` sunt no-op; `login`, `dashboard`, `stats`,
  `analytes`, `events` sunt meniu + `/meta`.
- `update-server` nu folosește `runner.Run`.
- WSM e exclusiv în memorie: fără coadă persistentă, replay, deduplicare sau livrare
  garantată. `command_ack` înseamnă „pus în coada de trimitere”.
- `api.request` apelează `ServeHTTP` pe același mux; nu e proxy. `api:invoke` ≡ sesiune
  locală non-admin, `api:admin` ≡ admin local. Niciunul nu dă drepturi clinice în WiseMED AI
  și nici login în consola admin WSM.
- HTTP 200 de la `POST /api/connections/{id}/commands` înseamnă că a sosit reply-ul; succesul
  remote e în `response.status` / `response.ok`.
- HTTP 202 SIUI înseamnă job rezervat și pus în coadă. `completed` nu înseamnă acceptare CNAS.
  `unknown` cere reconciliere manuală; nu există cod de reconciliere.
- Retry-ul callbackului `callwmapi` și al arhivării nu reapelează CNAS.
- Nu există anulare în protocolul WSM: timeoutul local nu oprește execuția remote. Nicio
  reconectare nu retrimite comenzi.
- Trimiterea rezultatelor în buletin nu are coadă, retry sau cheie de idempotency.
- Update-ul nu are semnătură sau rollback; checksumul e opțional.

## 4. Identificatori

| ID | Rol |
|---|---|
| `tenant_id` | granița de izolare WSM; vine din cheia JWT |
| `sub` | principalul autentificat |
| `client_id` | eticheta sesiunii/aplicației; neunic |
| `equipment_id` | ID-ul WiseMED al echipamentului; țintă de rutare; unic per tenant |
| `reader_id` | identitatea locală a aplicației (`reader.id` din config); unic per tenant |
| `connection_id` | socketul curent; generat de WSM; se schimbă la reconectare |
| `request_id` / `correlation_id` | corelare comandă ↔ reply în plicul WSS; nu deduplică |
| `operation_id` | cheia de idempotency a apelantului pentru un job SIUI, per `owner` |
| ID job SIUI | identificatorul intern al jobului persistent |

## 5. Verificare

- Verificările rulate și rezultatele lor: [verification-matrix.md](verification-matrix.md).
- Stare la 2026-11-14: build, `vet`, teste și `-race` trec în `readersv3` și `wsm-server`;
  integrarea WSM trece. Eșecuri preexistente: `siui-bridge.test.cjs` (ruta `report/package`
  lipsă din SDK) și `readersv3/docker/test-update.py` pe bash 3.2.
- Pentru smoke tests: director temporar, date sintetice, port loopback. Nu atinge
  `readersv3/output/`, `wsm-server/output/`, `deployments/config.yaml`, baze de date,
  certificate.

## 6. Documente

- [../../ARCHITECTURE.md](../../ARCHITECTURE.md) — index.
- [application-catalog.md](application-catalog.md) — aplicații, module, persistență, porturi.
- [runtime-and-modules.md](runtime-and-modules.md) — lifecycle, servicii, configurare.
- [communication-map.md](communication-map.md) — matricea comunicațiilor.
- [api-catalog.md](api-catalog.md) — rute, comenzi, paritate.
- [security-and-identity.md](security-and-identity.md) — credențiale, autorizare, constatări.
- [data-and-state-machines.md](data-and-state-machines.md) — date, tranzacții, stări.
- [end-to-end-flows.md](end-to-end-flows.md) — fluxuri cu diagrame.
- [build-deploy-update.md](build-deploy-update.md) — build, servicii, Docker, update.
- [gaps-and-contradictions.md](gaps-and-contradictions.md) — discrepanțe și necunoscute.
- `_reports/01`–`06` — rapoartele de investigație, cu dovezile `fișier:linie` complete.
