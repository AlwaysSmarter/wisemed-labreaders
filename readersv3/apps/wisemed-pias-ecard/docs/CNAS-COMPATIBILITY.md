# Integrarea SDK oficial și wrapper CNAS

Analiza din 2026-11-23 a folosit decompilare țintită cu ILSpy pentru DLL-urile
furnizate în `docs/CNAS/CNAS-PARA`. Codul decompilat nu este compilat în produs.

## Variante distincte

- `2.0.0.4`: DLL-ul SDK din documentația oficială. Acesta și celelalte SDK-uri
  directe păstrează recuperările inspirate din analiza CNAS: token offline cu
  înrolare/profil, refresh o singură dată, recitire CID și rezultate parțiale.
- `cnas-wrapper-1.0.0.0`: DLL-ul original `Cnas.Ceas.SmartCard.Novensys.dll`,
  AssemblyVersion 1.0.0.0, împreună cu SDK 2.0.0.4 și dependențele originale.
  Manifestul include SHA-256 și versiuni. Copiile SDK sunt identice; wrapperul
  este diferența efectivă de comportament.

## Apeluri către implementarea originală

`CnasWrapperAdapter.cs` încarcă assembly-ul original și creează contextul CNAS.
Injectează sesiunea și identitatea contractuală a jobului, dezactivează loggingul
brut și cache-ul pacientului și invocă interceptorul original offline. Nu recreează
algoritmii GetToken, EvaluateOperationResult sau FixKnownCardIssues în surse proprii.

- `GetToken`: inclusiv tokenul offline pentru erorile UM tratate de CNAS.
- `EvaluateOperationResult`: refresh de token și recuperările originale CNAS în
  jurul citirii/scrierii SDK; datele brute și codurile pe câmp rămân disponibile.
- `ComputeHash`: semnarea cu wrapperul; certificatul MAI explicit este incompatibil
  dacă nu există metoda distinctă, fără substituirea certificatului solicitat.
- `Activate`, `ChangePin`, `ResetPin`: operațiile explicite ale wrapperului.

Nu rulăm desktopul CNAS, auditul sau SQL-ul său. ServiceReadData/ServiceWriteData
traduc și prin baza aplicației CNAS; acest nivel nu este folosit. Recitirea privată
CID aflată în ServiceReadData nu este executată de adaptor. Contextul are aceeași
sesiune furnizată de host; nu îi cerem să selecteze interactiv un alt cititor.

## Reparații și izolare

Selectarea variantei wrapper implică semantica originală FixKnownCardIssues,
inclusiv eventuale modificări de activare sau PIN în UM. Varianta directă păstrează recuperările de citire/token/CID; nu adaugă
implicit secvențele de reparație PIN/activare din wrapper. Constructorul wrapperului original include și ApduUtilInterceptor, care
poate modifica în memorie comportamentul SDK în procesul x86. Procesul dedicat
per variantă izolează această intervenție de încercările SDK directe.

Hostul păstrează verificarea identității, generația cardului și verificarea CMS.
Înrolarea lipsă/coruptă nu declanșează automat o nouă înrolare la citire; enroll
rămâne explicit. Semnarea sau modificarea cu rezultat incert nu se reexecută pe
alt SDK. Cititoarele se identifică fizic, iar inițializarea este așteptată înainte
operației.

## Diagnostic și verificări

Raportul /jobs/{id}/report folosește used_sdk ca valoare simplă; request, response,
events și errors aparțin ultimului SDK. other_sdks păstrează încercările anterioare.
Diagnosticul complet este disponibil explicit prin ?diagnostics=full. sdk_identity arată versiunile/hashurile reale, iar evenimentele
cnas_wrapper_loaded și cnas_wrapper_call_started/finished/failed arată apelurile
către DLL-ul încărcat. SDK changed este publicat prin sdk_changed și pe WSM.

Testele hostului verifică separarea căii directe de cea wrapper și delegarea către
un assembly fixture de test. Testele Go verifică raportul complet, retenția,
idempotency, izolarea proprietarilor și publicarea finalului WSM. Buildul net48
compilează hostul Windows x86. Aceste verificări nu confirmă încă rularea
constructorului original CNAS și a operațiilor pe hardware Windows; este necesară
proba fizică cu DLL-urile livrate, cititorul și UM.

## Modelul de fir și diagnosticul bootstrap

Entry pointul original Cnas.Siui.Paraclinic.Starter.Program.Main are STAThread.
Hostul folosește un fir STA dedicat numai pentru --cnas-wrapper; calea SDK directă
rămâne neschimbată. Pașii anteriori constructorului original sunt jurnalizați,
iar Win32Exception păstrează NativeErrorCode, mesajul și cadrele cu numele metodelor.
Testul fixture injectează o excepție în constructor și verifică păstrarea cauzei;
nu confirmă execuția constructorului original pe hardware Windows.

## Dependența SQL din constructor identificată în raportul Windows

Lanțul observat: NovensysSmartCardContext..ctor → HttpsCredentialsProvider.SetupDefaults
→ CommonContext.GetActivationKey → SqlConnection.Open → SqlException → Win32 233.
Constructorul caută întâi ICredentialsProvider în Unity. Adaptorul înregistrează o
implementare independentă construită peste AbstractCredentialsProvider original,
înainte de constructor. Nu se modifică DLL-urile și nu se ocolește autentificarea
cardului/UM. Se evită doar fallback-ul de configurare HTTPS din SQL-ul desktopului.
Metodele HTTPS fără configurare eșuează explicit. Înregistrarea implicită a fost
verificată și cu DLL-urile originale Unity/WebServices, pe lângă fixture-ul care
reproduce constructorul dependent de SQL. Acest test nu este o citire de card reală.
