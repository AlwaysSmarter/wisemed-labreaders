# Validare

## Executat local

- Compilare host C# x86 pentru .NET Framework 4.8, cu Roslyn 4.8 și reference
  assemblies oficiale; nu folosește SDK de card la compilare (adaptor reflection).
- Teste Go cu race detector: identitate CNP/CID, anonimizare/vid, mismatch,
  ordine fallback, interdicție replay PIN/mutații, întrerupere batch, rezultate
  parțiale, lipsa semnării la identitate greșită, criptare și AAD, restart fără
  replay, parser TDX/hash hostname, compatibilitate read-only fixture local,
  ownership/auth, Origin, idempotency batch, offset/duplicate itemi, PIN one-shot.
- Testele comune local-http, API bridge și runner.
- Metadate assembly și comparația exactă a celor două PDF-uri vechi.
- Icon PNG și ICO multirezoluție; verificare vizuală.

Buildurile și arhivele sunt locale. Niciun serviciu existent nu este repornit,
niciun endpoint CNAS nu este apelat și nicio distribuție nu este publicată automat.

## Obligatoriu pe stația Windows înainte de producție

1. .NET Framework 4.8, driverul cititorului, acces PC/SC din contul serviciului;
   verificați serviciul în Session 0, nu doar procesul interactiv.
2. Login WiseMED real și WSM cu tenant/subiect provisionate; verificați accesul
   local, remote, izolarea proprietarilor și rolul admin.
3. Lista cititoarelor și alegerea explicită când există mai multe.
4. Citire CEAS și CEI cu fiecare SDK compatibil. Verificați statutul online/offline,
   citirea CNP/CID și certificatele returnate. Nu presupuneți că vechile SDK-uri
   acceptă CEI doar pentru că au metode cu aceleași nume.
5. Batch sintetic (1, 10, 100, 1000 itemi), măsurați timpul. Verificați semnăturile
   independent. CNP normal, CNP vid și 0000000000000 cu CID valid; mismatch trebuie
   să oprească înaintea semnării.
6. Scoatere/schimbare card în timpul unui batch: oprire, păstrarea rezultatelor
   confirmate, fără continuare pe cardul nou sau replay pe alt SDK.
7. PIN de la PC și de la tastatura cititorului, anulare, timeout; fără reutilizare
   automată a PIN-ului. Nu testați PIN greșit repetat pe card de producție.
8. Simulați oprirea hostului/serviciului: starea unknown/interrupted, fără replay.
9. Migrare TDX: preview, confirm, backup, fișier destinație găsit de SDK; verificați
   permisiunile ProgramData. Fișier original păstrat intact. Nu distribuiți TDX.
10. Activare, schimbare/resetare PIN și scriere numai cu card/mediu de test și
    autorizațiile CNAS necesare. Nu sunt acoperite de teste fizice locale.
11. Validați serviciul medical rezultat la CNAS; succesul CMS nu substituie acest
    pas. Verificați și cardurile expirate din scenariul utilizatorului.

Limită: hostul și interacțiunea reală PC/SC/UM nu au fost executate pe Windows în
această sesiune. Disponibilitatea/performanța hardware și acceptarea CNAS nu sunt
certificate prin testele Go cu backend simulat.

## Rezultat efectiv al buildurilor din 2026-11-21

- Host Windows .NET x86: **compilat**, inclus în deployments/host, hash verificat.
- Serviciu Go macOS arm64 v0.1.0: **compilat**, în
  `readersv3/output/wismed-pias-ecard/wismed-pias-ecard`.
- Smoke real la 127.0.0.1:19124: `/healthz` OK; `/api/ecard/health` fără sesiune
  returnează 401; `/api/modules` listează numai local-http, events, wisemed-api,
  wisemed-ws, login, help, ecard. Procesul de test a fost oprit.
- Verificarea payload-ului de release (SDK-uri + host + sursă + SHA-256): trecută.
- Serviciu Go Windows amd64 v0.1.0: **compilat**; installer NSIS și update ZIP
  generate în `dist/installers` și `dist/updates`. Icon eCard inclus în installer.
- Installerul și ZIP-ul extrase și comparate byte-cu-byte cu runtime-ul și
  sursele deployments: toate cele șapte SDK-uri și hostul incluse, fără config.yaml,
  TDX, chei locale, baze de date sau loguri. Dimensiunile și SHA-256 corespund
  metadatelor `dist/releases/wismed-pias-ecard/windows-amd64/0.1.0.json`.
- Blocajul de spațiu a fost rezolvat de utilizator. NSIS local necesită locale
  UTF-8 explicit: `LC_ALL=en_US.UTF-8 LANG=en_US.UTF-8` înaintea comenzii release.
- Testele releasectl au trecut după modificarea pentru iconul installerului.
- Installerul nu a fost executat pe Windows; testele fizice de mai sus rămân necesare.
