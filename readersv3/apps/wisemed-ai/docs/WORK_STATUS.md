# WiseMED AI — checkpoint de reluare

Actualizat: 2026-11-03. Acest fișier descrie cererile acceptate, starea reală și pașii rămași. Citește-l înainte de a continua într-un alt chat. Nu conține credențiale.

## Cererile utilizatorului

1. Utilitar standard numit `wisemed-ai`, construit pe runner-ul WiseMED Labreaders, cu API/WSM opționale, instalare ca serviciu și pachete standard de update. PostgreSQL preluat din configurația locală existentă, numai tabele `wai_`.
2. Șabloane pentru specialități comune, dashboard, versiuni imuabile, dictare/extragere AI, revizuire și aprobare înaintea oricărui export/livrări HIS.
3. Monetizare: super admin -1, admin0, utilizator1, Free implicit, Pro/Premium configurabile, limite de specialități/șabloane/tokenuri, tarife suplimentare și credit. Abonamente lunare UTC, cinci zile de grație, suspendare financiară și reactivare după plata verificată. Plata EuPlătesc va fi integrată după primirea documentației; nu inventa callbackuri/încasări.
4. Context persistent pentru fiecare șablon, criptat și legat de versiune; definiții compacte și evidența exactă a tokenurilor per procesare/generare/șablon. Un skill/context extern nu înseamnă procesare gratuită pentru model.
5. NOUL MODEL GENERAL: secțiuni și subsecțiuni, fără dependență de noțiunea de organ. Variante scurte/standard/detaliate, selectate din date documentate. Nivel general al raportului și excepții pe secțiuni. Exemple: ficat/rinichi, ventricul stâng/drept, simptome/antecedente/examen/diagnostic dictat/recomandări/rețetă dictată. Nu deduce normalitate sau diagnostic din tăcere. Utilizatorul a cerut explicit adaptarea TUTUROR celor52 de exemple.
6. Aplicație Flutter completă, elegantă și simplă pentru dictare mobilă, conectată la acest API. Proiect separat găsit în `/Users/raduichim/work/flutter/wisemed-ai`; specificația este `WisemedAIFlutter.md`. Utilizatorul a cerut explicit agent separat pentru acest proiect; acesta a fost pornit și lucrează în paralel.

## Reguli de păstrat

- Nu publica nimic pe update-server: utilizatorul va actualiza serverul singur. Release-uri numai locale.
- Nu modifica/afișa secretele existente; configurația runtime este ignorată de Git și protejată. Păstrează cheile de criptare și parolele.
- Nu folosi date reale de pacient în teste. Nu face apeluri OpenAI facturabile sau plăți reale pentru validare.
- Nu suprascrie șabloanele personalizate. Istoricul și sesiunile păstrează versiunea inițială.
- Nu accepta termeni/licențe SDK, nu publica în magazine și nu instala servicii fără autorizarea necesară.

## Stare backend

Repo: `/Users/raduichim/work/gowork/wisemed-labreaders/readersv3`.
Aplicația locală rulează **1.3.0** la `http://127.0.0.1:19120/ai/`, din `output/wisemed-ai/wisemed-ai`. A fost repornită la 18:43, cu config/chei existente păstrate. Config și bootstrap se află în directorul local `output/wisemed-ai/deployments`; nu copia conținutul lor în surse/checkpoint.

Implementat și verificat anterior: pachete1.1, abonamente/credit/context1.2. Vezi `MONETIZATION.md`, `SUBSCRIPTIONS.md`, `CONTEXT_OPTIMIZATION.md`, `VALIDATION.md`.

Implementat acum pentru1.3:

- `presentation.go`, `session_presentation.go`: variante declarative pe secțiuni, detaliu moștenit, condiții equals/documented, cerințe de dovezi, fallback la date insuficiente/ambiguitate, păstrarea subsecțiunilor.
- `POST /v1/sessions/{id}/presentation`: refolosește faptele validate, fără apel AI; revizie optimistă, aprobare veche invalidată, documentele aprobate rămân blocate.
- `catalog_model.go`: toate52 de exemple au `model_version:2`, câmpuri repartizate, secțiuni/subsecțiuni și variante pentru nivelurile de detaliu. Extragerea AI primește legătura câmp–secțiune.
- Upgrade idempotent al exemplelor comune nepersonalizate, prin versiuni noi; cele publicate de utilizatori sunt păstrate. Catalogul rămâne52 de șabloane /46 de specialități.
- `SectionDesigner.vue` și `PresentationPanel.vue`: controale de structură/variante și stil, fără JSON obligatoriu. JSON avansat rămâne disponibil.
- `Session.template` returnează structura versiunii fixate de sesiune. POST /sessions acceptă opțional `work_context`: ambulatoriu/internare/ambele, stocat criptat și inclus în verificarea idempotency.
- OpenAPI1.3 actualizat; fixture-ul tuturor șabloanelor regenerat.

Verificări1.3 deja trecute:

- Teste de catalog complet, upgrade fără suprascriere, versiuni păstrate, variante imbricate, dovezi și revizii/aprobare.
- Suita PostgreSQL completă cu race:84,691s, PASS; go vet trecut. Ulterior, modificarea mică work_context a trecut `go test ./...` și `go vet ./...`.
- Vue/TypeScript build și7 teste Vitest trecute.
- Playwright1.3 FINAL: catalogul și scenariul clinic extins au trecut, inclusiv scurt global +ficat detaliat, editor, aprobare și mobil. Au fost corectate etichetele accesibile și sincronizarea testului cu animația selectorului. Recuperarea prin plată verificată a trecut separat pe schema temporară. Au fost curățate trei sesiuni sintetice rămase din rulările UI eșuate.

## Livrabile backend — stare finală 1.3

- Playwright nativ trecut; OpenAPI final validat; work_context/idempotency trecut cu PostgreSQL real.
- Release-uri finale1.3 macOS/Linux/Windows regenerate după ultimele modificări. PKG/DMG, tar.gz Linux și Setup.exe NSIS real există local. Checksum-uri și payload fără secrete verificate.
- Executabilul nativ rulează1.3 și este identic cu update-ul macOS. Toate52 de exemple runtime sunt model2; fixture-ul conține52 de șabloane /46 de specialități /1028 secțiuni /5124 variante demonstrative.
- Modelul este documentat în SECTION_MODEL.md; nu s-a publicat pe update-server.
- Flutter a fost finalizat ca surse și build-uri web; verificările și limitările native sunt consemnate mai jos.

## Proiect Flutter și delegare

Agentul `/root/flutter_app` a încheiat implementarea și verificările. Nu mai are editări active. Are proprietatea întregului director `/Users/raduichim/work/flutter/wisemed-ai`; nu edita aceleași fișiere în paralel. Coordonează prin mesaje. Nu crea alt chat: utilizatorul a cerut agent separat, nu un thread nou.

Părintele a inițializat proiectul Flutter pentru Android/iOS/web/macOS și a creat inițial `lib/core/{models,api,storage,speech,controller,demo}.dart`. Agentul le auditează și implementează UI completă, lifecycle, privacy, teste și build-uri. Are spec-ul, OpenAPI și trei exemple sintetice. Este informat că API1.3 rulează și că poate utiliza work_context. Nu pretinde că Flutter este terminat până agentul livrează și verifici rezultatul.

Dependențe rezolvate: http, flutter_secure_storage10.3.4, cryptography, path_provider, shared_preferences, speech_to_text7.4.0, uuid, flutter_widget_from_html_core, share_plus12.0.2, mobile_scanner, url_launcher, intl0.20.2 +flutter_localizations.

Toolchain: Flutter3.35.6/Dart3.9.2 stable instalat, AndroidSDK35/platform36/JDK17 (unele licențe neacceptate), Xcode16.2 pe macOS14.5, simulator runtimes indisponibile, CocoaPods1.12.1 vechi. macOS și Chrome disponibile. Nu s-au schimbat SDK global, licențe sau identități de semnare. Cere build-uri posibile fără codesign și raportează sincer limitele de hardware/dispozitive.

Ținte Flutter: dictare logic continuă cu restart STT nativ și limitări OS explicite; pauză în background/apel și reluare explicită; doar segmente stabilizate trimise; outbox criptat cu UUID/sequence/revision păstrate la retry; izolare utilizator/tenant/server; fără chei OpenAI pe mobil; review/finalize/aprobare exactă înainte de export; QR temporar fără PHI; stări offline/eroare și mod demo sintetic; UI premium responsive și verificări analyze/test/build.

## Limitări de mediu apărute la build-ul Flutter

Build-ul Android a epuizat spațiul liber al hostului. Agentul a eliminat numai intermediarii proiectului Flutter (build/app, build/.cxx, build/test_cache, android/.gradle); părintele a eliminat numai intermediarii regenerabili de packaging WiseMED AI (macos-package/linux-package). Sursele, executabilele runtime, ZIP-urile, instalatoarele finale și capturile au fost păstrate. După curățare erau aproximativ1,1GiB liberi; nu goli cache-uri sau fișiere ale altor proiecte.

Build-ul iOS fără semnare este blocat de platforma/runtime iOS18.2 lipsă pe Xcode16.2. Build-ul web a reușit, primele capturi Flutter au fost inspectate; analiza a fost curată și agentul execută regresiile finale. Nu declara APK/IPA produs până există artefactul verificat. Cerințele native share_plus și secure_storage trebuie aliniate în proiect (AGP/Gradle/JDK/minSdk), nu mascate de lipsa spațiului. Agentul este informat.

## Reluare

Dacă această conversație este pierdută, pornește de la acest fișier, citește starea reală a proceselor și codul de pe disc, apoi continuă lucrările rămase. Nu presupune că procesele sau agenții au continuat după închiderea aplicației. Codul și documentele salvate rămân pe disc.


## Rezultat final Flutter — 2026-11-03

Implementarea și documentația sunt în `/Users/raduichim/work/flutter/wisemed-ai`. Analyzer fără probleme;18 teste unit/controller/crypto/HTTP-contract/widget trecute și un test live omis intenționat din rularea implicită. Testul live separat contraGo1.3 a trecut: autentificare,52 de șabloane/model2, context de lucru, QR one-time, retransmitere idempotentă, prezentare/revizie, finalizare, aprobare exactă și ștergerea sesiunii sintetice. Smoke Chromium real prin proxy same-origin19131: auth/providers, login, billing/account, templates, sessions și billing/statistics au răspuns200 și dashboardul autentic a fost afișat. Fără bypass CORS/TLS, date reale ori apeluri AI facturabile.

Build-uri demo/producție web reușite. Arhivă: `/Users/raduichim/work/flutter/wisemed-ai/dist/wisemed-ai-web-1.0.0.zip`, SHA256 `c8761107ab047942a51f93ffadcd9d60759765a0b7f185bd015d017ebdc8654b`, verificat de părinte. Capturi în screenshots/dashboard-mobile.png, workspace-mobile.png și live-dashboard-desktop.png; telefonul și workspace-ul au fost inspectate independent de părinte.

Această stare nativă a fost reluată ulterior; rezultatele actuale sunt mai jos. macOS rămâne cu targetul corectat11, fără rebuild complet. Dictarea OS, camera și întreruperile pe telefon real rămân de verificat pe dispozitiv.

## Reluare Android/iOS după CleanMyMac — 2026-11-03

La cererea utilizatorului s-a golit Trash și, după confirmare separată, CleanMyMac a curățat17,8GB de cache Gradle/JetBrains/go-build. Spațiul disponibil a ajuns17GiB; cache-ul necesar compilării Android s-a refăcut ulterior. La final rămân aproximativ7,4GiB.

Android: build debug universal și release ARM64 trecute. APK instalabil de test: `/Users/raduichim/work/flutter/wisemed-ai/dist/android/wisemed-ai-1.0.0-debug.apk`; release nesemnat: `dist/android/wisemed-ai-1.0.0-release-arm64-unsigned.apk`. Semnătura debug, manifestul release, alinierea ZIP16KiB și SHA256SUMS verificate. Android minSdk24, compile/target36; nu s-a folosit o identitate de producție. Analyzer curat,18 teste trecute și1 live omis. Nu există dispozitiv Android conectat pentru QA fizic. Rebuild helper: tools/build_android.sh; detalii docs/ANDROID_BUILD.md.

iOS: Keychain entitlements, minimum13 și configurare CocoaPods Profile corectate. plutil și pod install trecute. Workspace device/simulator blocat de platforma18.2 lipsă. Compilarea directă a targetului trece AOT arm64 dar eșuează la storyboard din aceeași cauză; NU există IPA/app valid. Componenta Apple necesită8,13GiB descărcare plus instalare/staging, peste spațiul disponibil; nu s-a pornit descărcarea. Pași și rezultate în `/Users/raduichim/work/flutter/wisemed-ai/docs/IOS_BUILD.md`. Checkpointul Flutter actual este WORK_STATUS.md din același proiect. Nu au fost acceptate licențe, configurată semnarea de producție sau publicate pachete.

## Corecție interfață utilitar — versiunea1.3.1

Utilizatorul a identificat că `/` afișa consola generică de analizor, deși protocolul și comm_type erau corect wisemed-ai/utility. Interfața AI exista doar la `/ai/`, iar consola comună nu trata explicit AI. Corectat: root și rutele clinice vechi deschid dashboardul AI; setările comune afișează identitate Utilitar/WiseMED AI, doar server/actualizări și WSM/WSS. Nu se mai cer dashboard/analytes/orders/QC/daily-details/result-sync/debug-tests de laborator. Schimbarea limbii și pollingul păstrează modul. Wisemed API LoginOnly include AI, fără bootstrap/înregistrare de analizor, cu identitatea WSM existentă păstrată. Conturile și metodele de autentificare AI nu au fost modificate.

Verificat: Go test/vet localhttp, wisemedapi, runner;61 teste UI comune trecute/12 omise,4 regresii AI noi;2 Playwright pe executabilul nativ pentru root și setări cu sesiune administrativă sintetică. Nu s-au transmis credențiale către server extern și nu s-au modificat setări în browser. Runtime nativ1.3.1 rulează pe127.0.0.1:19120, sesiune exec24937, PID67493 la verificare; readiness200 și root307 spre /ai/. Executabilul `output/wisemed-ai/wisemed-ai` corespunde exact release-ului macOS. Binarul debuggerului `output/wisemed-ai/bin/WiseMED_AI` a fost recompilat cu simboluri debug și versiunea1.3.1.

Instanța IDE deja pornită (PID59738, port19121) a fost lăsată neatinsă; încă are codul vechi în memorie și necesită stop/run în IDE. La pornirea verificării, mecanismul standard a ales temporar19122 fiindcă19121 era ocupat. Serverul de verificare a fost mutat înapoi pe19120 pentru compatibilitatea preview-urilor Flutter, iar adresa configurată existentă19121 a fost restaurată în config.yaml. Nu schimba sau afișa celelalte valori/secrete din config. Nu presupune că procesele mai există la reluare; verifică.

Toate livrabilele1.3.1 macOSarm64/Linuxamd64/Windowsamd64 sunt generate local și verificate: update ZIP-uri/metadate, PKG/DMG, tar.gz și Setup.exe NSIS. Nu s-a publicat pe update-server. Versiunile1.3.0 rămân artefacte istorice; folosește1.3.1 pentru corecția UI. Vezi VALIDATION.md pentru limitele verificării.

## Login administrator / WiseMED fără @ — 1.3.2

Utilizatorul a cerut acces admin și a raportat validarea obligatorie @ la login WiseMED. Contul bootstrap existent admin@wisemed.local este verificat efectiv ca super admin (-1), fără resetarea parolei; parola rămâne exclusiv în fișierul protejat output/wisemed-ai/deployments/ai-bootstrap-password, indicat utilizatorului prin link. Nu o copia în chat/loguri/configuri de distribuție. Sesiunile de verificare API și browser au fost revocate.

Cauza: providerul WiseMED era dezactivat local și opțiunea era ascunsă, astfel formularul rămânea în modul Email. Acum Tip de autentificare este explicit: email AI sau username WiseMED (text, fără cerință @). Providerul dezactivat este explicat și submitul lui este dezactivat. Loginul WiseMED a fost activat în configurația locală la cererea utilizatorului; source config.install.yaml păstrează setarea configurabilă implicit false. Nu se creează automat utilizatori/drepturi; accesul extern cere tenantul AI și asocierea wisemed:<user_id> verificată în Utilizatori. Nu s-au folosit credențiale WiseMED reale sau modificat mappinguri.

Verificări: build Vue/TypeScript și7 teste Vitest trecute;5 Playwright trecute (username fără @ către API simulat, provider dezactivat explicat, login/logout real superadmin local, root AI, setări utilitar fără laborator). Compilat1.3.2, actualizat output/wisemed-ai/wisemed-ai și output/wisemed-ai/bin/WiseMED_AI. Runtime pe19120 (sesiune exec95172, PID73202 la verificare), auth/providers={standalone:true,wisemed:true}. Adresa existentă a IDE19121 a fost păstrată în config după pornirea verificării pe19120; procesul IDE59738 vechi nu a fost oprit și trebuie relansat de utilizator. Verifică starea proceselor înainte de reluare.

Livrabile1.3.2 locale pentru macOSarm64/Linuxamd64/Windowsamd64 regenerate, inclusiv ZIP-uri/metadate și instalatoare. Checksum-urile și payloadul fără configurații runtime/secrete verificate; executabilul local corespunde release-ului macOS. Nu s-a publicat nimic pe update-server.

## Dictare incrementală și corectare pe câmp — 2026-11-05

La cererea utilizatorului, `invalid_number` și `unsupported_evidence` nu mai resping întregul segment. `buildResult` produce `field_issues` cu câmp/cod/explicație/sursă/variante, păstrează valoarea brută cu starea `needs_review` și continuă celelalte câmpuri. Valorile neconfirmate nu intră în canonical_data și împiedică aprobarea până la rezolvare. Lipsurile sunt afișate ca —. Interpretările numerelor vorbite/virgulelor omise sunt propuneri separate, cu sursa literală păstrată.

Endpointul transcript-chunks și aliasul audio-transcript-chunks procesează exclusiv noul segment. Rezultatul se îmbină cu faptele existente; câmpurile omise de o nouă extragere rămân completate. Retry-ul identic după pierderea confirmării este idempotent și nu repetă extragerea. Segmentele vechi rămân criptate pentru proveniență, nu se retrimit la AI. Migrația015 adaugă target_field. Acesta limitează extragerea și contextul de vocabular la câmpul selectat. Corecțiile manuale sunt în contextul criptat field_overrides și prevalează până la redictarea explicită a acelui câmp.

Noi API-uri POST sessions/{id}/field-edit și field-options cer revizia curentă. Editarea/alegerea unei interpretări este acceptarea explicită a medicului, inclusiv în afara limitelor șablonului. Un document aprobat se poate redeschide într-o revizie nouă; rezultatul clinic aprobat anterior rămâne arhivat. OpenAPI și SDK-ul Dart din repository sunt actualizate.

Flutter din /Users/raduichim/work/flutter/wisemed-ai: ecran de dictare minimal, Record rotund pe aproximativ50% din înălțimea ecranului, microfon verde pulsatil și contur animat cu unde, transcriere pe o linie cu autoscroll. Apăsarea Record activ face pauză; Stop roșu oprește captura și păstrează sesiunea reluabilă. Raport în dreapta Stop și Info deschid popup-uri. Istoricul/butonul de trimitere a segmentelor au fost eliminate. Raportul are câmpuri apăsabile, roșu pentru probleme, variante, editare și redictare țintită. Raportul se actualizează la fiecare confirmare de segment chiar când dictarea continuă. Android/iOS mențin ecranul aprins în timpul ascultării, eliberând cererea la pauză/stop. Tăcerile nu consumă bugetul de retry al recunoașterii native. La clarificarea utilizatorului, captura continuă și cu telefonul blocat sau în altă aplicație: Android folosește DictationCaptureService, foregroundServiceType=microphone, notificare fără PHI cu Pauză/Stop și partial wake lock; iOS declară UIBackgroundModes=audio și păstrează sesiunea audio activă. Sincronizarea continuă cât captura în fundal este autorizată și activă. Pornirea trebuie făcută explicit în aplicația vizibilă; fără repornire automată după terminarea procesului. Întreruperile audio opresc captura și eliberează serviciul.

Verificat: Go test cu PostgreSQL real în scheme temporare și go vet; build server; build Vue și7 teste Vitest; Playwright sintetic pentru corectarea valorii02 fără pierderea celorlalte câmpuri și trimitere automată; Flutter analyze fără probleme,44 teste trecute și1 test live omis; build Flutter web demo și APK Android debug. Capturi pe390×844 inspectate pentru ecranul de dictare, Raport și Info. Nu s-au efectuat apeluri AI clinice reale sau verificări de microfon pe telefon fizic. Activarea într-un server existent și instalarea pe dispozitiv sunt distincte de build-urile locale.

Documentație nativă verificată pentru implementarea în fundal: [Android microphone foreground services](https://developer.android.com/develop/background-work/services/fgs/restrictions-bg-start), [Apple audio recording in background](https://developer.apple.com/documentation/avfaudio/avaudiosession/category-swift.struct/record). Testele de controller simulează fundalul/notificarea; blocare reală, schimbare aplicație și apeluri necesită dispozitiv fizic. Sintaxa Swift și Info.plist sunt verificate; nu s-a generat un IPA.
