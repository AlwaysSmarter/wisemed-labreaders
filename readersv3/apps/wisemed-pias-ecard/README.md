# wisemed-pias-ecard

Utilitar WiseMED PIAS eCard: serviciu Windows pentru card de sănătate / CEI,
API local autentificat și același API prin WSM. Nu include modulele de laborator
(analize, probe, QC, ordine de lucru).

- Go: lifecycle comun, serviciu, login WiseMED API, WSM, actualizări, API, joburi.
- .NET Framework 4.8 x86: host separat pentru fiecare SDK; fără DLL-uri amestecate
  într-un singur proces. .NET Framework 4.8 și driverul cititorului se instalează
  pe Windows. Hostul EXE este inclus, nu se descarcă/compilează la utilizator.
- SDK-uri incluse în `deployments/sdks`: 2.0.0.4, pachetul 2.0.0.3,
  thinkIT 1.3.1.4, 1.3.0.4, 1.3.0.2, 1.3.0.1, 1.2.0.1.
- Batch cu sesiune păstrată, semnări secvențiale, identitate pacient obligatorie,
  verificare CMS și detectare schimbare card. Nu promite paralelism pe card.
- Migrare TDX exclusiv explicită, în doi pași; înrolare CNAS distinctă.

## Pornire

Din directorul aplicației:

```powershell
.\wisemed-pias-ecard.exe -version
.\wisemed-pias-ecard.exe -config deployments/config.yaml -showlog
.\wisemed-pias-ecard.exe -installservice
.\wisemed-pias-ecard.exe -headless
```

Sunt moștenite `-config`, `-headless`, `-installservice`, `-reconfigure`,
`-showlog`, `-version`. Nu folosiți flagul intern `-headless-child` manual.
Interfața locală: `http://127.0.0.1:19124/`, apoi meniul **PIAS eCard** (`/ecard/`).
Configurați API-ul WiseMED și autentificați-vă în shell-ul comun. Setări → WSS
configurează conexiunea și identitatea echipamentului. WSM este oprit implicit
până există configurarea reală; nu sunt livrate credențiale inventate.
Actualizările comune sunt activate implicit, canal `stable`, app_id exact
`wisemed-pias-ecard`; adresa implicită este cea a utilitarelor existente.

Structura standard a repository-ului folosește numele **deployments/**. Toate
SDK-urile, hostul și ajutorul sunt incluse acolo în installer și în update.
`config.yaml` este creat din `config.install.yaml` și păstrat la actualizare.

## Configurare

`modules.ecard.sdk_order` selectează subsetul și ordinea SDK-urilor. UI și
`PUT /api/ecard/settings` permit schimbarea ordinii, cititorului și UM; necesită
administrator și repornire. Restul limitelor se configurează în YAML.
Citiți [API](docs/API.md), [analiza documentației](docs/SDK-STUDY.md) și
[validarea / verificările pe stație](docs/VALIDATION.md).

Datele joburilor sunt în `deployments/ecard-state`: SQLite și cheia locală
`payload.key`. Cererile, textele și rezultatele sunt criptate AES-GCM, legate
criptografic de job/poziție. PIN-ul nu este stocat în DB sau log. Backup-ul
joburilor necesită DB și cheia. ACL-ul Windows al directorului trebuie să permită
acces doar contului serviciului și administratorilor. Nu includeți acest director
în distribuții. Retenție implicită 24 ore, verificată la pornire și orar; nu se
șterg joburi active. Identificatorii de idempotency sunt păstrați ca tombstone
pentru a împiedica reluarea semnării după expirarea rezultatelor.

## Build / release

```sh
cd readersv3
go test -race ./modules/ecard ./modules/localhttp ./shared/apibridge ./apps/runner
make release APP=wisemed-pias-ecard TARGET=windows-amd64
scripts/prepare-reader-output.sh wisemed-pias-ecard
```

Hostul se reconstruiește pe Windows cu
`apps/wisemed-pias-ecard/deployments/host/build-host.ps1`, sau multiplatformă cu
`scripts/build-host.sh` și variabilele DOTNET, CSC_DLL, NET48_REFS (Roslyn 4.8 și
reference assemblies .NET Framework 4.8). Actualizați și manifestul hostului după
rebuild. Nu publicați un release cu un EXE vechi față de sursă.

Buildul Go nativ macOS permite verificarea API-ului și a stocării, dar operațiile
hardware nu sunt disponibile pe macOS/Linux. SDK-ul CNAS este Windows/.NET și
folosește WinSCard. Suportul fizic CEAS/CEI și acceptarea CNAS necesită probe reale;
un build reușit nu reprezintă certificarea fluxului.
