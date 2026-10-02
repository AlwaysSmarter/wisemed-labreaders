# Verificare locală — 22 septembrie 2026

Implementarea a fost verificată în workspace; nu a fost instalată pe serverul
central și nu a fost conectată la echipamente ori la un backend WiseMED real.

| Verificare | Rezultat |
| --- | --- |
| Server: `go test -race ./...`, `go vet ./...` | PASS |
| Readere/utilitare: `go test -race ./...`, `go vet ./...`, `go build ./apps/...` | PASS |
| UI JavaScript: 9 teste WSS + 12 teste transport remote | PASS |
| Integrare server TLS + modul reader, apel API și tichet de control | PASS |
| Reconectare automată după așteptarea efectivă de 30 secunde | PASS |
| Izolare tenant, autentificare, Arevocare, restricții tichet, 50 socketuri WSS | PASS |
| Chrome real, asseturi comune reale, date și transport simulate | PASS |
| `govulncheck -mode=binary` pentru serverul construit cu Go 1.26.8 | Fără vulnerabilități raportate |

În Chrome au fost verificate pornirea și navigarea în modurile reader, barcode,
Docsmart și eSignature, lista WSS, deschiderea ferestrei remote, predarea tichetului,
reînnoirea după reload, tipărirea din JSON și logout. Nu au apărut erori JavaScript
și nici cereri HTTP directe remote către `/api/*` sau `/help/*`. Testul Chrome
folosește transport simulat; testul separat de integrare folosește socketuri TLS reale.

Reproducere: `./verify.sh` din `wsm-server`. Integrarea durează aproximativ 32 secunde
deoarece verifică intervalul real de reconectare, fără a-l accelera artificial.

Pachete locale:

- `wsm-server/discu ct/`: server și utilitar chei pentru macOS ARM64, asseturi și ghiduri.
- `wsm-server/dist/linux-amd64/`: server central Linux x86_64 și aceleași resurse.
- `readersv3/dist/wss-preview/20260921T225216Z/`: 26 aplicații macOS ARM64 și
  eSignature Windows x86 cu DLL-ul existent. Toate executabilele native au trecut
  verificarea `-version`. Sunt preview-uri, nu instalatoare semnate.

Pentru testul real trebuie configurate certificatul și cheia TLS, tenantul și
cheile de autentificare, API-ul WiseMED și URL-ul WSS. Folosiți ghidurile
[de operare](operations.md) și [de control remote](remote-control.md). Configurațiile,
bazele de date și instalațiile existente nu au fost înlocuite de pachetele preview.
Validarea pe Windows, driverele și hardware-ul fizic rămân necesare în mediul real.
