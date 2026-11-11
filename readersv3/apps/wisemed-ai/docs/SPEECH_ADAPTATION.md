# Adaptarea recunoașterii vocale

Migrarea 013_speech_adaptation.sql se aplică la pornirea utilitarului. API-ul
folosește autentificarea cookie/bearer existentă și prefixul /v1. Șabloanele,
secțiunile și accesul sunt izolate pe organizație.

## Capabilități implementate

| Motor | Local | Adaptare la decodare | Captură PCM comunitară | Intermediar | Timestamp-uri |
|---|---|---|---|---|---|
| OS, speech_to_text 7.4.0, iOS/Android | Depinde de OS și limbă | Wrapperul nu expune hotwords | Indisponibilă | Da | Neexportate |
| Whisper Turbo/Small, iOS, WhisperKit Core ML | Da | Filtru logit pentru continuări de expresii la limita cuvântului | Da | Finale pe clip | Estimări de cuvinte + interval de captură |
| Parakeet v3, iOS, FluidAudio Core ML | Da | AsrManager nu expune bias la decodare | Da în motorul batch | Finale pe clip | Estimări de tokenuri + interval de captură |
| Parakeet v3 BPE, Android și opțiune distinctă iOS, sherpa-onnx 2.0.1 | Da, CPU | modified_beam_search, 8 căi, context graph, scor 1,5 | Da | Finale pe clip | Debuturi estimate de tokenuri + interval de captură |
| Whisper ONNX, Android, sherpa-onnx 2.0.1 | Da, CPU | Wrapperul nu expune bias Whisper | Da | Finale pe clip | Debuturi estimate de tokenuri + interval de captură |
| Fluxul Parakeet iOS experimental existent | Da | Neintegrată | Audio neexportat către coadă | Da | Neexportate |

WhisperKit este fixat la 9f99b5cb6a907730543823ad271b82532782a899, FluidAudio
la 503b4bd1bbf7220882de39fe8ae6716aae4132da. FluidAudio oferă separat
VocabularyBoostingSession cu CTC spotting și rescoring. Acesta nu este raportat
ca favorizare la decodare în AsrManager.

Filtrul Whisper nu este un prompt. Bonusul este 0,6 pentru tokenul următor al unei
expresii al cărei prefix de minimum trei caractere este deja decodat la limita
cuvântului. Candidatul trebuie să fie la maximum trei unități logit de cel mai bun
token. Începutul expresiei nu primește bonus global. Termenii cu un singur token
și prefixele scurte nu primesc bonus. Efectul trebuie măsurat pe audio.

Niciuna dintre integrările curente nu expune o gramatică restrictivă compatibilă.
Backendul generează și persistă atomic la publicarea șablonului un IR versionat
pentru câmpuri boolean/numerice, limite, unități și alternative de negație.
Dictarea narativă rămâne liberă. runtime_artifacts este gol și
runtime_grammar_supported este false. Pentru șabloanele vechi se generează IR
determinist la cerere. Nu se aplică GBNF la Parakeet și nu se reutilizează
artefacte din altă versiune. Un compilator de gramatică necesită un motor care
acceptă efectiv acel format și validare dedicată.

### Instalarea obligatorie BPE

Modelul ONNX Parakeet v3 INT8 este fixat la revizia
2bda32ec70b097a55adaa07d9a7173915b43cc78 a
csukuangfj/sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8.

tokenizer.vocab provine din mlx-community/parakeet-tdt-0.6b-v3, revizia
ed2b7e8c15f9aaa0b5772e2efb986255eaef7e15. Are 101.024 octeți și SHA-256
41130ff456706304a1adec782ccc9e003c4d417e8e324353d281be958cac4e17.
Toate cele 8.192 de intrări trebuie să corespundă exact, în ordine, cu tokens.txt.
Blank este separat, ID 8.192. Instalarea verifică această asociere.

Un model ONNX fără BPE este considerat incomplet. Completarea instalării păstrează
fișierele ONNX existente cu dimensiunile așteptate și descarcă artefactele lipsă.
Pe iPhone, Parakeet BPE ONNX/CPU este o alegere explicită distinctă de FluidAudio
Core ML. BPE nu activează automat un decodor diferit în FluidAudio.

## Vocabular și feedback

Cheia vocabularului include tenant_id, template_id și Section.ID. Redenumirea și
reordonarea unei secțiuni păstrează datele. Schimbarea ID-ului creează o asociere
nouă. NFC și spațiile consolidate servesc deduplicării; cheia ignoră capitalizarea,
dar păstrează diacriticele și flexiunile. Termenii nu sunt șterși prin absență.

Se păstrează sursa, prima observare, numărul observărilor, starea, autorul și
evenimentele de proveniență. Lista activă are maximum 64 de expresii și 2.048 de
octeți, ordonate reproductibil după observări și cheia termenului. Vocabularul
persistent este separat de această limită.

Diff-ul determinist este conservator. Măsurătorile, negațiile, lateralitatea,
contextul identificabil, inserările și reformulările ample nu generează termeni
activi. Corecțiile medicale similare pot produce candidați. Termenii noi necesită
aprobare sau transcriere audio verificată explicit. Nu există reguli globale de
înlocuire. Filtrarea automată nu este un detector universal de date personale;
candidații necesită revizuire înainte de activare.

Feedbackul se trimite după aprobarea explicită a reviziei. Se păstrează raw STT,
corecțiile succesive, modelul/runtime-ul și versiunile, vocabularul/gramatica,
chunk-ul și intervalul capturii. Textul secțiunii acceptate este extras pe server
din raportul aprobat. Textul final nu devine automat transcriere fidelă sau țintă
de antrenare. Inserările manuale nu primesc proveniență audio.

Coada locală este criptată și independentă de aprobarea raportului. Un receipt
persistent permite recuperarea după întreruperea aplicației. Retry-urile păstrează
evenimentele. Cheia unică pe organizație/sesiune/revizie/secțiune previne
învățarea dublă. Erorile permanente de feedback sunt păstrate separat pentru
verificare și nu blochează documentele clinice.

## Contractele API

Toate endpointurile sunt autentificate și izolate pe organizație. Parametrul
version_id fixează versiunea imutabilă a șablonului.

- GET /v1/templates/{template_id}/speech-adaptation?version_id={version_id}
  returnează schema, tenant_id, template_id, template_version, vocabulary_version,
  sections, canonical_json și sha256. Hash-ul acoperă exact UTF-8 canonical_json.
  ETag folosește același hash. If-None-Match valid primește 304. manage=1 include
  candidați/statistici și necesită drepturi de editor.
- POST /v1/templates/{template_id}/sections/{section_id}/speech-terms?version_id=...
  primește text și status. Stările sunt candidate, approved, rejected.
- POST /v1/templates/{template_id}/sections/{section_id}/speech-feedback?version_id=...
  primește event_id, template_version, session_id, accepted_revision, section_id,
  accepted_section_text și segments. Un segment are id, chunk_id, raw_text,
  corrections, verified_transcript, transcript_verified, model, model_version,
  runtime, runtime_version, mode, vocabulary_version, grammar_version,
  start_ms, end_ms, capture_started și consent_generation dacă există. Include
  și terms, capabilities, grammar_ir_hash și timings disponibile. Timestamp-urile
  estimate ale decodorului nu stabilesc automat o aliniere verificată.
  Sunt necesare aprobarea aceleiași revizii de către medic și un chunk valid.
  Retry identic primește evenimentul existent. Conflictul primește 409, fără
  statistici noi. Audio nu este trimis în feedback.
- GET/PUT /v1/profile/speech-consent citește/salvează enabled și notice_version.
  Informarea este speech-community-2026-11-05-v1. Se returnează generation,
  activated_at și withdrawn_at. Reactivarea creează un grant nou. Colectarea
  necesită începutul capturii după activare și grantul încă valabil la upload.
- POST /v1/community/speech-contributions primește id, source_session,
  template_id, template_version, section_id, accepted_revision, segment,
  accepted_section_text, notice_version, byte_length, sha256, duration_ms,
  codec=pcm_s16le_wav, sample_rate=16000, channels=1 și language=ro.
  Segmentul trebuie să existe în feedbackul acceptat. Răspunsul conține id și
  uploaded. Limita este 100 de contribuții noi/zi/utilizator.
- PUT /v1/community/speech-contributions/{id}/audio?offset={octeți} primește corp
  binar application/octet-stream, maximum 256 KiB/parte. Se acceptă următorul
  offset sau retry cu același offset și hash. Părțile conflictuale primesc 409.
  Răspunsul indică uploaded, folosit la reluare.
- POST /v1/community/speech-contributions/{id}/finalize validează toate părțile,
  hash-ul și WAV PCM16 mono 16 kHz, maximum 30 secunde. Starea devine
  pending_deidentification. Retry-ul nu creează o mostră nouă.
- GET /v1/community/speech-contributions listează mostrele proprii sau ale
  organizației pentru admin. GET /{id} și GET /{id}/audio sunt autorizate,
  auditate și no-store. Query sanitized=1 livrează varianta mascată.
- POST /v1/community/speech-contributions/{id}/review este rezervat adminului.
  Primește decision, verified_transcript, transcript_verified,
  audio_reviewed_in_full, deidentification_verified, alignment și redactions.
  Aprobarea cere speaker_key pseudonim consecvent (8–64 caractere ASCII, cifre,
  cratimă sau underscore) și single_speaker_verified. Identificatorul se referă
  la vocea reală, nu implicit la utilizatorul care încarcă fișierul.
  Alinierile au start_ms, end_ms și text, relative la fișierul contribuit.
  Redacțiile au start_ms și end_ms. Alinierile trebuie să reproducă exact
  transcrierea verificată. Audio nealiniat este mascat, inclusiv omisiunile STT.
  Redacțiile sunt eliminate și din ținta textuală. Numai verificarea umană
  explicită permite approved_for_dataset. Alte stări sunt needs_review și rejected.
- POST /v1/community/speech-contributions/export, numai admin, primește dataset_id
  UUID. Manifestul conține numai audio mascat și ținte verificate, SHA-256,
  speaker_id pseudonim și split_key. Împărțirea train/eval se face pe vorbitor.
  Exportul este strict pe organizație. Agregarea între organizații sau publicarea
  necesită un flux ulterior explicit.
- DELETE /v1/community/speech-contributions/{id} șterge contribuția autorizată și
  marchează apartenențele exportate drept revocate.

## Protecție și retenție

Fișierele audio sunt dedicate, AES-GCM, sub community-speech, cu director 0700 și
fișiere 0600. Referințele sunt în PostgreSQL. Metadatele, feedbackul, vocabularul
și revizuirile sensibile sunt criptate cu cheia existentă. Nu se loghează
conținut medical.

Uploadurile și mostrele neaprobate expiră în 7 zile. Audio brut și textele brute
ale mostrelor aprobate se elimină după 7 zile. Audio mascat și ținta verificată
pot fi păstrate 365 zile. Feedbackul expiră cu sesiunea sau în maximum 30 zile.
Fișierele orfane au o grație de o oră înaintea ștergerii.

Pe telefon, directorul offline-speech este exclus din backup iOS. Datele sunt
criptate și izolate după server/utilizator/organizație. Audio și feedbackul local
netrimis au retenție de 7 zile, verificată la redeschidere și periodic. Acordul
este implicit dezactivat. Dezactivarea locală se salvează înaintea rețelei,
oprește colectarea și șterge audio netrimis. Activarea necesită serverul.

Nu se declară automat nicio mostră anonimă. Vocea poate rămâne identificabilă.
Sunt necesare revizuirea întregului audio și a alinierii, inclusiv datele personale
omise sau greșite în STT. Retragerea acordului revocă eligibilitatea exporturilor
viitoare; copiile deja exportate necesită gestionare de către operator.

## Verificare reproductibilă

1. Actualizează backendul și aplică migrarea într-o bază PostgreSQL de test.
2. Publică șablonul și configurează vocabularul în editor. Păstrează ID-urile
   secțiunilor la redenumire și reordonare.
3. Instalează modelele și completează BPE din Setări. Alege motorul și secțiunea
   dictată. Testează schimbarea secțiunii și modul avion cu pachetul valid în cache.
4. Corectează un chunk, finalizează și aprobă raportul. Întrerupe rețeaua la
   feedback/upload. Reconectează și repetă retry-ul. Statisticile trebuie să rămână
   fără duplicate.
5. Verifică activarea și retragerea offline a acordului. O captură începută
   înainte de activare sau sub un grant anterior nu trebuie încărcată.
6. Revizuiește o mostră în Contribuții vocale. Introdu date identificabile omise
   din STT și verifică mascarea audio și ținta manifestului. Mostrele neverificate
   sau cu aliniere invalidă trebuie excluse.
7. În Laborator selectează pachetul și secțiunea. Rulează aceeași înregistrare cu
   adaptarea oprită/pornită. Rezultatele se păstrează separat.
8. Folosește minimum 10 dictări sintetice cu terminologie, numere, unități,
   stânga/dreapta, negații și termeni așteptați nerostiți. Compară separat erorile.
9. Pe S23 și modelul concret de iPhone măsoară încărcarea, latența finală și
   intermediară disponibilă, memoria, temperatura și minimum 10 minute de dictare.
   Motoarele batch captează segmente la pauze sau la maximum 6 s. Dacă trei segmente
   așteaptă procesarea, captura se oprește cu mesaj și se finalizează coada.
   Nu se simulează rezultate intermediare.

Testele sunt speech_adaptation_test.go și test/speech_adaptation_test.dart în
Flutter. Integrarea PostgreSQL folosește WAI_TEST_DATABASE_URL și scheme izolate.
Antrenarea și distribuirea automată a modelelor sunt un flux ulterior separat.

Contractele structurate sunt în openapi.json. Verificarea nativă BPE pe macOS a
folosit aceeași înregistrare sintetică cu adaptarea oprită/pornită: aproximativ
2,3 s / 2,0 s, fără inserarea termenilor de control nerostiți. Această probă
nu demonstrează un câștig general de precizie. Evaluarea pe S23 și iPhone fizic
rămâne de executat conform protocolului de mai sus.

Pe telefonul USB disponibil SM-S901E (S22), testul nativ Android debug pe aceeași
înregistrare sintetică a dat 3.535 ms fără adaptare și 3.162 ms cu adaptare BPE,
inclusiv crearea motorului. Ambele au păstrat 11 cm, lateralitatea dreaptă și
negația, nu au introdus hipoecogen / vascularizație Doppler nerostite, dar au
transcris greșit rinichiul ca rinitiul. Au fost returnate debuturile tokenurilor.
Fișierele instalate pe telefon au fost reverificate SHA-256. Acesta nu este un
benchmark pe S23 și nu acoperă dictarea prelungită sau memoria maximă.

Actualizare: catalogul administrabil și fallback-ul copiilor sunt descrise în
SPEECH_MODEL_CATALOG.md. Motorul local reutilizează decodorul între segmente,
preîncarcă modelul înainte de captură și afișează progresul capturii/procesării.
Testul live cere alegerea explicită a șablonului și apoi permite o secțiune sau
vocabularul general al șablonului, incluzând titlurile.
