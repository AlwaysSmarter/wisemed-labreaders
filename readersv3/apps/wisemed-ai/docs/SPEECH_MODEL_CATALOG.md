# Catalogul modelelor vocale locale

Migrarea 014_speech_models.sql creează catalogul, evidența fișierelor verificate și
coada persistentă de copiere/verificare. Fișierele sunt în directorul dedicat
speech-models/blobs/<sha256>, sub directorul de date al instanței. Include acest
director în backup împreună cu PostgreSQL. Fișierele nu conțin dictări de pacient.

## Administrare

În dashboard, administratorul instanței are pagina Modele vocale locale. Catalogul
este comun organizațiilor instanței; adminul unei singure organizații nu poate
publica modele pentru celelalte organizații.

Catalogul inițial conține Whisper Turbo/Small ONNX, Parakeet v3 ONNX cu BPE și cele
trei variante Core ML deja acceptate pe iPhone. La instalarea backendului sunt
adăugate doar manifestele, fără descărcări automate de câțiva GB.

1. Selectează modelul și Copiază / repară fișierele pe server. Workerul persistă
   progresul; repornirea/reîncercarea reutilizează fișierele verificate.
2. Alternativ, încarcă directorul ori fișierele ONNX din calculator. Uploadul are
   părți de maximum 8 MiB, offset confirmat și verificare SHA-256 la final.
3. Adminul instanței poate vedea drafturile în catalogul telefonului pentru teste.
   Verifică transcrierea română și compatibilitatea pe platformele declarate.
4. Confirmă compatibilitatea și dreptul de redistribuire, apoi publică versiunea.
   Publicarea este atomică și cere toate fișierele verificate. Versiunea veche și
   fișierele ei rămân disponibile; se poate republica pentru rollback.
5. Pe telefon, Setări → Actualizează catalogul WiseMED AI. Modelele noi apar în
   selectorul motorului și în laborator. Instalarea unei versiuni noi nu șterge
   versiunea instalată și nu schimbă automat alegerea utilizatorului.

Pentru modele noi, pornește de la manifestul unui model existent, schimbă ID-ul,
versiunea, sursele și hash-urile și salvează draftul. O versiune existentă este
imutabilă: același ID/versiune cu alt conținut primește 409. Publicarea nu convertește
PyTorch/GGUF și nu garantează prin numele familiei compatibilitatea unui graf ONNX.

## Formate acceptate

- sherpa-onnx-2.0.1, family=whisper: fișiere cu rol encoder, decoder, tokens.
- sherpa-onnx-2.0.1, family=parakeet-tdt-v3: encoder, decoder, joiner, bpe, tokens.
  Vocabularul BPE trebuie să aibă cele 8.192 de ID-uri compatibile cu Parakeet v3.
- whisperkit-9f99b5c, family=whisper, numai iOS: model_folder cu
  MelSpectrogram.mlmodelc, AudioEncoder.mlmodelc, TextDecoder.mlmodelc și directorul
  tokenizer cu tokenizer.json, tokenizer_config.json, config.json și, dacă există,
  special_tokens_map.json. Compilatele trebuie să fie compatibile cu iOS/runtime.
- fluidaudio-503b4bd, family=parakeet-tdt-v3, numai iOS: director
  parakeet-tdt-0.6b-v3-coreml cu Preprocessor.mlmodelc, Encoder_v2.mlmodelc,
  Decoder.mlmodelc, JointDecisionv3.mlmodelc și parakeet_vocab.json.

Limba acceptată acum este explicit ro. Adăugarea unei alte limbi necesită
validarea modelului, tokenizerului și a configurării decodorului; eticheta nu
activează automat suportul. Capabilitățile de adaptare sunt cele ale runtime-ului:
ONNX Parakeet folosește BPE/context graph; FluidAudio Core ML nu expune aceeași
funcție; WhisperKit folosește filtrul logit existent.

Fiecare fișier din manifest are path relativ sigur, role dacă este ONNX, size,
sha256 și url HTTPS fixat la o revizie. Limite: 1.000 fișiere, 4 GiB/fișier,
8 GiB/model. Nu sunt acceptate traversări de directoare sau formate executabile
arbitrare. Copierea de la sursă refuză rețele private și redirecturi către HTTP.

## Verificarea actualizărilor

Manifestul poate avea sources: repo Hugging Face, revision de 40 caractere,
prefix, target și track. Pentru versiunile publicate cu track=true, workerul
verifică zilnic revizia main, folosind metadatele API oficial și hash-urile LFS.
Există și butonul Verifică revizia nouă la sursă.

O revizie nouă cu aceeași structură produce un draft imutabil up-<hash> și pune
copierea lui în coadă. Nu înlocuiește versiunea publicată înaintea verificării.
Dacă apar/dispar membri ai pachetelor, operația raportează model_update_layout_changed:
adminul trebuie să revizuiască manifestul. Pentru o sursă fără urmărire automată,
adminul poate crea o versiune nouă manual.

Workerul verifică o operație la fiecare tick de 10 secunde, cu maximum un worker
local și revendicare atomică PostgreSQL. Operațiile întrerupte sunt recuperate
după 35 minute; o operație are limită de 30 minute. Fișierele complete sunt
reutilizate la retry. Copiile vechi nu sunt șterse automat. Uploadurile parțiale
rămân pentru reluare și trebuie incluse în administrarea spațiului disponibil.

## Telefon și fallback

Descărcarea încearcă URL-ul original, apoi copia WiseMED AI cu același SHA-256.
Eșecul HTTP, întreruperea sau un hash greșit declanșează fallback. Bearerul WiseMED
este trimis numai către endpointul aceleiași origini; redirecturile copiei sunt
refuzate. Un fișier parțial nu înlocuiește fișierul valid.

Catalogul și descriptorii versiunilor vechi sunt păstrați local pe serverul
configurat. Cache-ul invalid nu înlocuiește catalogul valid. Inferența nu descarcă
nimic. Modelele instalate rămân disponibile offline. Pentru variantele Core ML
incluse în aplicație, se preferă omologul publicat și verificat din catalog când acesta există;
în lipsa lui se păstrează downloaderul SDK și se persistă asocierea cu selecția existentă.

## Contract API

Toate rutele folosesc autentificarea existentă, inclusiv bearer mobil.

- GET /v1/speech-models: versiuni publicate. include_drafts=1 adaugă drafturile
  numai pentru adminul instanței. Răspunsul are schema wisemed.speech-model-catalog.v1
  și models; fiecare intrare are manifest, canonical_json, sha256, published,
  previously_published, mirrored_files. Hash-ul acoperă exact UTF-8 canonical_json.
- GET /v1/speech-models/manage: manifestele tuturor versiunilor și ultimele joburi;
  numai adminul instanței.
- POST /v1/speech-models: manifest wisemed.speech-model.v1; creare draft idempotentă.
- POST /v1/speech-models/{id}/{version}/mirror: job de copiere/reparare, 202.
- POST /v1/speech-models/{id}/{version}/check-update: job de verificare, 202.
- POST /v1/speech-models/{id}/{version}/publish: compatibility_tested=true și
  redistribution_allowed=true. 409 dacă lipsesc fișiere, 422 fără confirmări.
- PUT /v1/speech-models/blobs/{sha256}?offset=N: upload binar admin, maximum 8 MiB.
  Returnează uploaded. Un offset reluat primește 409 cu offsetul curent; clientul
  continuă de acolo. 422 la hash incorect resetează uploadul parțial.
- GET /v1/speech-models/blobs/{sha256}: descărcare autentificată, ETag și Range.
  Utilizatorii au acces la fișiere publicate cel puțin o dată; adminul poate testa
  și drafturi. Timeoutul de scriere este extins pentru descărcările mari.

## Dictare pe segmente și test medical

Motorul local preîncarcă decodorul înainte de captură și îl reutilizează între
segmente, cu eliberare după 45 secunde de inactivitate. PCM16 mono 16 kHz este
segmentat la pauze de aproximativ 460 ms, după minimum 800 ms, ori la maximum
6 secunde de vorbire continuă. Ultimul segment se finalizează la oprire. Nu se
fabrică ipoteze intermediare; UI-ul afișează rezultatele pe măsura finalizării.
Capturile fără energie vocală nu sunt trimise decodorului.

Test dictare cere alegerea unui șablon când există șabloane disponibile și permite
alegerea secțiunii. General combină reproductibil vocabularul tuturor secțiunilor;
modul pe secțiune folosește termenii ei. Titlurile, inclusiv forma scurtă istm,
fac parte din context. Limita rămâne 64 termeni / 2.048 octeți. Indicatorul de nivel
și numărul segmentelor arată captura și procesarea efective.

Testele acoperă manifestele, formatele incompatibile, fallback la hash greșit,
izolarea bearerului, refuzul redirecturilor, publicarea incompletă, versiuni
imutabile, reluare upload, Range, împărțirea audio fără pierdere de eșantioane și
vocabularul ales după șablon/secțiune. Benchmarkurile sintetice nu înlocuiesc
validarea pe dictările medicului și pe dispozitivele țintă.

## Probe sintetice pentru terminologie

Pe macOS, cu vocea sintetică Ioana, comparația pe aceleași patru fișiere a arătat
că scorul BPE 0,6 nu îmbunătățea expresia mici striuri fibroase. Cu scorul 1,5 și
8 căi, expresia a fost recunoscută integral, fără inserarea termenilor de control
nerostiți în proba separată. Numerele 0,23 și 11 cm și negația au fost păstrate.
Rămân erori: lob drept poate deveni loc drept, iar istm poate deveni este în
vorbire cursivă. Mărirea agresivă a bonusului nu a rezolvat toate aceste cazuri.
Nu există o garanție de dicționar românesc exhaustiv și nu se aplică înlocuiri
post-STT automate pentru constatări clinice. Decodarea cu motorul încărcat a durat
aproximativ 0,9–1,0 s pentru înregistrări de 5–6 s pe Mac, nu pe telefon.

Reproducere: tool/benchmark_local_speech.dart din proiectul Flutter rulează
aceleași fișiere PCM16/mono/16 kHz, fără/cu vocabular și pe chunkuri. Copiază
titles.pcm, terms.pcm, numbers.pcm, control.pcm în ApplicationSupport/speech-benchmark
al aplicației de test. Rezultatul este păstrat în același director, result.json.
Pornește prin flutter run -t tool/benchmark_local_speech.dart, alegând BENCH_MODEL
prin dart-define. Nu instala acest entrypoint diagnostic peste o dictare activă.

Verificare nativă suplimentară: pe SM-S901E/S22 conectat, preîncărcarea motorului
ONNX a durat aproximativ 2,7 s, iar cele patru probe sintetice au fost decodate
cu motorul încărcat în 0,53–0,67 s fiecare. Pe simulatorul iOS 18.2, aceeași
integrare ONNX/CPU a funcționat cu 0,31–0,54 s/probă. Aceste valori exclud durata
înregistrării și nu reprezintă rezultate pe un iPhone fizic sau pe S23. Au fost
verificate ambele configurații de vocabular și reutilizarea decodorului între
probe; termenii de control nerostiți nu au fost inserați.
