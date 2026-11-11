# Dictare continuă și Chrome Web Speech

## Motor local

Pornirea microfonului stabilește intenția de ascultare până la Stop/Pauză explicită.
O pauză de vorbire finalizează un segment, nu oprește înregistrarea. Pragul vechi
care oprea captura după trei segmente în așteptare a fost eliminat.

Captura PCM, detectarea vorbirii și inferența sunt etape separate. Silero VAD este
inclus în aplicație, rulează local și se inițializează din nou pentru fiecare
captură. Modelul public sherpa/Silero are licență MIT inclusă în assets/models.
SHA-256: 9e2449e1087496d8d4caba907f23e0bd3f78d91fa552479bb9c23ac09cbb1fd6.
Nu se descarcă nimic pentru detectarea vorbirii la pornirea microfonului.

Energia audio este folosită numai pentru indicatorul de nivel. Decizia de vorbire
vine din modelul VAD; zgomotul slab nu este tratat automat ca dictare. Segmentele
fără vorbire nu ajung la STT. Marginile fără vorbire sunt scurtate, păstrând tampon
pentru începutul și sfârșitul cuvintelor și intervalele originale ale capturii.
Nu există o listă de expresii cenzurate precum mulțumim: dacă expresia este rostită,
poate fi transcrisă legitim. VAD nu identifică vorbitorul și nu reprezintă o garanție
împotriva oricărei erori STT sau a vorbirii reale din fundal.

O previzualizare locală a segmentului curent este calculată după primele aproximativ
1,2 secunde disponibile. Segmentele finale au prioritate față de previzualizări.
Finalizarea se face la pauze sau la maximum opt secunde de vorbire continuă.
Decodorul rămâne încărcat între segmente. Când este ocupat, audio așteaptă în fișiere
AES-GCM temporare, în offline-speech/live-queue. RAM păstrează referințe și metadate,
nu întreaga dictare. Cheia temporară există numai pentru captura curentă; fișierele
se șterg după procesare/închidere, iar orfanii mai vechi de o zi sunt eliminați.
Această coadă protejează capturarea față de încetiniri; nu este arhivă recuperabilă
a unei dictări după terminarea procesului. Recunoașterile finalizate folosesc separat
drafturile criptate și sincronizarea persistentă existente.

Eșecurile temporare de decodare se reîncearcă fără oprirea microfonului. Lipsa
spațiului de stocare, pierderea permisiunii sau întreruperea microfonului de sistem
sunt erori reale, distincte de tăcere sau acumularea segmentelor. Alegerea unui model
mai lent decât timpul real poate acumula audio; coada nu poate accelera hardware-ul.

Linia de transcriere afișează ipotezele intermediare și textul finalizat în timpul
capturii. Numai textul finalizat este grupat pentru server: maximum cinci secunde
de așteptare de la primul fragment sau aproximativ 300 caractere. La Stop se
finalizează ultimul segment și se golește lotul de text. Uploadul și procesarea
clinică folosesc mecanismul existent, fără să blocheze microfonul. Segmentele audio
și proveniența lor rămân distincte chiar dacă folosesc același chunk de text.

Butonul de microfon are cerc și contur contrastante, verde luminos în ascultare,
cu pulsație vizibilă. Starea lui nu depinde de existența vorbirii într-o anumită clipă.

## Chrome Android, alternativă online

Setări → Motor de dictare → Chrome · Web Speech · Română (online). Chrome este
verificat prin PackageManager. Dacă lipsește, cardul de setări oferă instalarea
prin magazinul oficial, fără instalare ascunsă de APK-uri. Această integrare Chrome
este disponibilă pe Android; nu presupune suport echivalent în Chrome pentru iOS.

Recunoașterea folosește SpeechRecognition sau webkitSpeechRecognition, cu
lang=ro-RO, continuous=true și interimResults=true. Rezultatele intermediare apar
în pagina vocală și pot fi reflectate în aplicație. La încheierile normale/no-speech,
recunoașterea se reia dacă intenția de ascultare este încă activă. Refuzul permisiunii,
serviciul indisponibil sau limba nesuportată sunt afișate explicit.

Aplicația deschide o pagină dedicată în Chrome, pe serverul HTTPS configurat.
La primul acces, medicul acordă permisiunea și pornește microfonul în pagină.
Dacă permisiunea de microfon este deja acordată, pagina încearcă pornirea după
lansarea explicită din aplicație. Pagina trebuie păstrată în prim-plan. Stop sau
părăsirea ei finalizează dictarea; limitările de fundal ale Chrome nu sunt ascunse.
Intermedierea este online și poate folosi serviciul Google. Nu se pretinde că este
un model local sau că șablonul poate impune un dicționar medical decodorului Chrome.

Șablonul/secțiunea selectate fixează sesiunea de destinație. Pagina afișează numele
șablonului. Textul finalizat se trimite în loturi de aproximativ patru secunde sau
300 caractere prin aceeași procesare clinică incrementală. În timp ce uploadul ori
AI-ul procesează un lot, browserul continuă să recunoască următorul text. Rezultatele
ajung în sesiunea aplicației fără a fi trimise încă o dată ca alte chunkuri.

Test dictare cere și aici un șablon. Testul folosește un grant fără sesiune clinică:
nu creează rapoarte de pacient și nu rulează extracția AI. Motorul Web Speech nu
expune PCM către această aplicație, deci nu contribuie automat cu audio comunitar.

## Backend și contract

Sunt necesare migrările 016_browser_speech.sql și 017_browser_speech_state.sql,
după migrarea 015 existentă.

Pagina Chrome raportează separat starea microfonului: waiting, listening,
restarting, stopping. Numai evenimentul audiostart confirmă captura. Aplicația
nu mai deduce „înregistrează” din simpla deschidere a paginii. La încheiere sau
întrerupere, indicatorul de microfon revine în așteptare. Starea listening are
heartbeat de două secunde și este considerată expirată după opt secunde fără
confirmare, inclusiv dacă browserul este închis forțat.

- POST /v1/browser-speech, autentificat normal: session_id (opțional numai pentru
  teste), template_version, section_id și target_field. Verifică vizibilitatea
  șablonului, apartenența secțiunii/câmpului și sesiunea deschisă. Returnează token,
  path=/ai/voice#<token>, expires_in=7200, locale=ro-RO și numele șablonului.
- GET /ai/voice: pagina dedicată. Tokenul este numai în fragment, se elimină din URL
  după citire și nu este inclus în requesturile paginii ori în referrer.
- GET /v1/browser-speech/bridge?after=N: Authorization: Browser <token>. Returnează
  evenimentele finale noi, textul intermediar, șablonul, stop_requested și finished.
- POST /v1/browser-speech/bridge: același grant, action=partial|segment|stop|finished.
  Acceptă și action=state, state=waiting|listening|restarting|stopping.
  Un segment are event_id UUID, ordinal, text și timestamp. ID-urile, ordinea și
  reviziile se păstrează la retry. Replay identic este acceptat, conținutul diferit
  pe același eveniment/ordinal primește 409.

Grantul are numai scop vocal, expiră în două ore și depinde de loginul inițial.
Logout, dezactivarea utilizatorului sau a apartenenței opresc accesul. Bearerul
aplicației nu este pus în URL și nu este transmis paginii/Google. Textul intermediar
și evenimentele sunt criptate în PostgreSQL; granturile expirate se elimină la
retenție. Rezultatul clinic are retenția normală a sesiunii. Pagina are CSP cu nonce,
no-store și no-referrer. Pentru extracția clinică se aplică regulile normale de
acces, credite și limite ale sesiunii.

Pentru activare trebuie livrat backendul actualizat, iar reverse proxy-ul trebuie
să servească /ai/voice și /v1/browser-speech*. Modelul local funcționează independent
de aceste rute. Un server vechi nu poate oferi integrarea Chrome doar prin update APK.

## Verificări

- Simulare de 20 minute de audio cu decodor blocat temporar: nicio oprire automată;
  toate segmentele sunt procesate în ordine după deblocare și Stop este explicit.
- Previzualizare înaintea segmentului final, zero apeluri STT pentru tăcere și
  criptarea/ștergerea cozii temporare.
- Loturi finale trimise către server în timp ce microfonul rămâne activ.
- Silero nativ pe macOS: zero cadre detectate în zece secunde de liniște, zgomot
  constant simulat și ton; vorbirea sintetică a fost detectată.
- Simulator iOS 18.2 cu motorul ONNX real: două dictări separate prin liniște,
  opt rezultate intermediare, două finale, zero opriri automate, un Stop manual,
  niciun segment final provenit numai din liniște. Precizia terminologiei rămâne
  imperfectă; această probă nu demonstrează eliminarea tuturor halucinațiilor STT.
- Backend PostgreSQL: grant, acces limitat, criptare, replay/conflict și revocare
  la logout. JavaScript: reluare la onend, rezultate intermediare, loturi în timpul
  ascultării și browser nesuportat. Flutter: detectare/instalare Chrome și transfer
  numai cu grantul limitat.

Nu a fost validată aici o sesiune Google reală de 20 minute pe telefon. Verificarea
runtime a Chrome cere backendul public actualizat și permisiunea de microfon.

## WebView Web Speech integrat în Flutter

Motor separat: Setări → Motor de dictare → WebView · Web Speech · Română (online).
Folosește webview_flutter cu o pagină locală fixă, fără URL extern, token de login,
context de pacient sau scripturi descărcate. WebView-ul rămâne atașat în aplicație,
în spatele interfeței Flutter, iar pornirea și oprirea vin din aceleași butoane.
Sunt conectate evenimentele start, audiostart/audioend, soundstart/soundend,
speechstart/speechend, result, nomatch, error și end. Rezultatele finale sunt
grupate și sincronizate prin fluxul clinic obișnuit; ipotezele intermediare sunt
afișate fără a fi trimise ca rezultate finale.

Pentru WebView, trimiterea așteaptă patru secunde de pauză. Evenimentele de început
de vorbire anulează termenul, iar ipotezele intermediare și finale noi îl amână;
nu există trimitere la 300 de caractere sau la cinci secunde de la primul fragment.
Stop finalizează imediat textul rămas. Textul finalizat care așteaptă pauza se
salvează în draftul criptat (held_speech), separat de loturile deja pregătite pentru
server, și se recuperează după repornire. La trimitere, un lot foarte lung este
împărțit în cereri de maximum 12.000 de octeți UTF-8, păstrând ordinea.

API-ul real SpeechRecognition/webkitSpeechRecognition se verifică în WebView-ul
instalat. Nu este înlocuit pe ascuns cu SpeechRecognizer nativ. Dacă API-ul lipsește
sau serviciul refuză pornirea, eroarea este explicită și indicatorul nu pretinde
înregistrare. Serviciul vocal poate fi online; nici vocabularul și nici modul
offline nu sunt garantate de această opțiune. Actualizarea Chrome nu echivalează
cu activarea funcției în WebView. Migrarea 017 nu este necesară pentru motorul
WebView; acesta folosește direct API-urile clinice existente.

Verificare pe Samsung SM-S901E / Android 16, WebView 149.0.7827.91: ambele nume ale
API-ului sunt funcții, iar contextul local este securizat. Proba a verificat numai
capabilitatea, fără a deschide microfonul și fără a întrerupe dictarea activă.
Prezența API-ului nu dovedește pornirea serviciului sau precizia românei; acestea
necesită proba de dictare pe telefon. Testele Flutter și JavaScript acoperă lipsa
API-ului, confirmarea prin audiostart, reluarea la end/no-speech, deduplicarea
finalelor, ultimele cuvinte la Stop, erorile și ignorarea evenimentelor vechi.

## Bara de navigație Android

MainActivity ascunde numai navigationBars prin WindowInsetsControllerCompat,
la revenirea în aplicație și la recâștigarea focusului. Comportamentul transient
permite reapariția temporară prin gest de la margine și ascunderea automată.
Controalele de sistem rămân accesibile prin gestul Android. Nu schimbă setarea
de navigație a telefonului pentru alte aplicații.
