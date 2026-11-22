# Studiu SDK și decizii de integrare — 2026-11-21

## Surse studiate

- `docs/CNAS/Specificatie Interfatare PIAS - Anexa 102 - Specificatii_eCard_SDK2.0.0.3-v.pdf`:
  48 pagini; introducere CEAS/CEI, funcții §2, instalare/config §3, metode și
  evenimente §4, coduri SDK. API-urile de activare/scriere/resetare sunt online;
  citirea poate fi offline; semnarea are API scalar ComputeHash, nu batch.
- Cele două PDF-uri `SDK1.1.54-v5.1` și `SDK1.1.54-v4.0` sunt identice byte cu byte
  în checkout. Conținutul indică versiunea 3.7 / 15.07.2014, 29 pagini, cu fluxul
  CEAS și aceleași operații fundamentale. Numele fișierelor nu sunt versiuni
  independente ale contractului în această copie.
- Sursele `old-src/WiseMEDeCARDManager`, inclusiv SDK thinkIT 1.3.1.4,
  `WMCEASCard.cs`, `SesiuneCard`, `TerminalManager`, `TerminalDataStorage`,
  `TerminalDataIdentifier`, `UMClient`, `ASN1Helper`, `ISesiuneCard`.
- Inspectarea metadatelor tuturor DLL-urilor livrate și decompilarea țintită a
  claselor de mai sus din 2.0.0.4 (pentru contractele folosite de adaptor).
  Nu s-au efectuat apeluri CNAS și nu s-au încărcat bibliotecile pentru operații
  hardware în mediul macOS.

## Diferențe relevante

2.0.0.3 documentează CEI, certificatul G8 și câmpurile I1/I4/I5/I6/I7/I8/I10,
în timp ce vechile documente descriu CEAS. SDK 2.0.0.4 referențiază .NET 4.8,
BouncyCastle.Cryptography, Ceas.eCard.Cid, System.Text.Json și altele; dependențele
sunt înglobate ca resurse Costura în DLL. Bibliotecile vechi au BouncyCastle.Crypto
și Newtonsoft.Json externe, împachetate în directorul versiunii. Namespace-ul
public `Novensys.eCard.SDK` este păstrat. thinkIT adaugă proprietăți publice de
certificate care NU există în toate SDK-urile oficiale; adaptorul nu se bazează
pe acele proprietăți pentru certificatul de semnare.

DLL-ul din folderul `eCard.SDK.2.0.0.3` declară AssemblyVersion 2.0.0.2;
2.0.0.4 declară 2.0.0.4. Identificatorii din config sunt etichete ale pachetelor
furnizate, cu hashuri exacte în `deployments/sdks/manifest.json`.

`ComputeHash` produce CMS SignedData, nu doar hash. `ASN1Helper.GetSignedAttributes`
include signingTime UTC, messageDigest, contentType și signingCertificate; astfel
aceleași texte pot produce semnături diferite în momente diferite. Semnarea se
execută pe card. Cheile private de semnare nu sunt în TDX. Validarea rezultatului
în host verifică octeții CMS și certificatul capturat de la pacient.

## TDX

În 2.0.0.4 și în thinkIT, numele este SHA-1 uppercase al șirului contractual cu
CRLF, data yyyyMMdd, CUI, CIF, hostname, IP:port UM și versiunea identificatorului.
Formatul local DES-CBC și serializarea .NET sunt compatibile în clasele inspectate.
Payload: TerminalId, Serie, CheieCriptarePIN, indicator mediu, profile și chei.

Migrarea păstrează exact payload-ul criptat, generează numai numele așteptat pe
noua stație și nu pretinde o înrolare CNAS nouă. Fișierele HITMED au fost folosite
numai pentru verificare locală read-only; nu sunt în aplicație, teste sintetice,
release sau documentație. Nu se imprimă chei sau date medicale.

SDK 2.0.0.4 conține și un override de dezvoltare CEAS_UM_LOCAL și un caz special
CEI offline fără înrolare UM. Hostul folosește exclusiv UM configurat și cere
înrolarea existentă pentru operațiile normale, conform cererii de operare
explicită. Configurați un UM de test prin setările utilitarului, nu prin override
ascuns de mediu.

## Fallback și batch

- Un proces per versiune elimină conflictele dintre numele de assembly identice,
  statici, singletons PC/SC și dependențe BouncyCastle incompatibile.
- Fallback este o fază de pregătire/citire. Nu trece peste o încercare PIN și nu
  repetă operații de semnare/scriere/activare/resetare. Erori necunoscute se opresc.
- Lipsa sau coruperea TDX nu declanșează auto-înrolare. Operația `enroll` este
  separată, cu confirmare explicită și administrator.
- Batch-ul păstrează un singur host/sesiune. SDK-ul/cardul poate totuși executa
  verificări interne per ComputeHash. Nu există promisiune de viteză măsurată
  înaintea benchmarkului pe hardware real.
- CNP/CID este verificat înainte de orice semnătură. CNP vid/anonimizat cere CID.
- Eliminarea cardului schimbă generația; batch-ul se oprește. CMS este verificat
  după semnare pentru a nu publica un rezultat al altui certificat.
- API sincron, încărcare unică asincronă și încărcare în chunk-uri folosesc
  același worker/stocare. Limitele WSM nu sunt ascunse prin răspunsuri trunchiate.

## Ce nu demonstrează sursele

Nu demonstrează acceptarea în producție CNAS a fiecărui model de CEI, card expirat
sau identitate TDX migrată, nici disponibilitatea curentă a UM. Acestea se verifică
pe stația țintă, păstrând diferența dintre semnătură validă și serviciu validat CNAS.
