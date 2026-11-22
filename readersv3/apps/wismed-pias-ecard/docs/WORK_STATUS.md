# Checkpoint — wismed-pias-ecard

Cerere: utilitar exclusiv (service, CLI, WiseMED login/API, WSM, update), SDK-uri
împachetate și selectabile în ordine, fallback la citire, migrare TDX numai
explicită, CEAS/CEI, batch mare, un call API sincron opțional. Semnare protejată
prin CNP/CID: CNP vid sau 0000000000000 cere CID; CNP normal diferit respinge.

Implementat în `modules/ecard` și `apps/wismed-pias-ecard`:
- sesiuni autentificate comune și acces WSM via api.request;
- host .NET Framework 4.8 x86 separat per SDK, IPC prin pipes;
- șapte SDK-uri incluse, thinkIT după pachetul 2.0.0.3; manifest SHA-256;
- cititoare/stare/citire/certificate, semnare, scriere, PIN/activare/înrolare;
- joburi persistente criptate, chunk-uri, progres, rezultate paginate, anulare;
- batch unic sincron/asincron, operation_id idempotent, rezultate parțiale;
- identitate înainte de semnare, generație card și verificare criptografică CMS;
- PIN challenge one-shot, fără stocare; fără fallback după PIN sau la mutații;
- TDX preview+confirm, păstrare payload/chei, backup, fără auto-generare;
- UI locală, setări ordine SDK, help/API/studiu SDK, icon PNG/ICO;
- scaffolding service/install/build, validare payload în releasectl.

Teste race trecute: ecard, localhttp, apibridge, runner, releasectl; go vet ecard.
Teste suplimentare includ API prin apibridge cu izolarea tenantului și un call
sincron care returnează toate rezultatele unui batch. Host C# compilat cu Roslyn
4.8 / reference assemblies net48, fără execuție hardware. DLL 2.0.0.3 declară
intern 2.0.0.2; este byte-identic cu cel din nupkg furnizat. PDF-urile vechi
v4.0/v5.1 sunt identice. TDX 2.0.0.4 păstrează formatul/hashul vechi.

Output nativ macOS v0.1.0 populat, inclusiv deployments/config.yaml. Smoke real
pe 19124 și refuzul accesului neautentificat verificate; proces oprit.
NU s-au făcut apeluri CNAS, login WiseMED real, test hardware Windows sau publish.
NU s-a migrat/scris vreun TDX real. Fișierele HITMED nu sunt în pachete.

## Release Windows finalizat — 2026-11-21

Utilizatorul a eliberat spațiul. Buildul standard Windows amd64 0.1.0,
installerul NSIS și arhiva de update au fost generate și verificate local:
- `readersv3/dist/installers/wismed-pias-ecard-0.1.0-Setup.exe`
- `readersv3/dist/updates/wismed-pias-ecard-0.1.0-windows-amd64.zip`
- metadate și SHA-256: `readersv3/dist/releases/wismed-pias-ecard/windows-amd64/0.1.0.json`.

Ambele pachete au fost extrase; toate fișierele deployments și executabilul
corespund byte-cu-byte sursei/runtime-ului, inclusiv cele șapte SDK-uri și hostul.
Fără config.yaml, TDX, chei locale, DB sau loguri. Installerul include iconul eCard.
Testele releasectl au trecut după includerea iconului.

NSIS local se prăbușea la autotest din cauza locale-ului; compilarea a reușit cu:
`LC_ALL=en_US.UTF-8 LANG=en_US.UTF-8 go run ./tools/releasectl release --app wismed-pias-ecard --target windows-amd64 --version 0.1.0`.
Nu s-a modificat instalarea globală NSIS. Artefactele nu au fost publicate.

## Validare rămasă

1. Instalare și test Windows + cititor real conform VALIDATION.md, în special
   Session 0, CEI, PIN fizic/PC, schimbare card în batch, certificate și erori per SDK.
2. Login WiseMED și conexiune WSM reale.
3. Măsurați batch și validați un serviciu la CNAS; nu declarați acceptare din CMS.

Nu reluați crearea aplicației de la zero. Modificările de documentație arhitectură
și fișierele .DS_Store deja existente în git status nu aparțin acestei lucrări.
