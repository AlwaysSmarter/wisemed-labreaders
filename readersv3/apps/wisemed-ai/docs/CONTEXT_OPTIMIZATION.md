# Context compact, regăsire și statistici — 1.2.0

Scopul este reducerea contextului repetat și evitarea apelurilor AI inutile, cu statistici verificabile. Nu este necesar un serviciu RAG extern pentru primul nivel de optimizare.

## Memorie persistentă pentru fiecare șablon

În editor: **Context AI al șablonului · memorie persistentă**. Completează regulile de completare (maximum 2000 de caractere), terminologia/abrevierile (1500) și exemple anonimizate acceptate (2000), apoi publică o versiune nouă.

Acesta este context direct asociat șablonului, fără căutare obligatorie. PostgreSQL păstrează payload-ul criptat AES-GCM în `wai_template_contexts`, legat de versiunea imuabilă. Nu se persistă în clar în documentul JSON al șablonului. Accesul respectă aceeași izolare de organizație și proprietar ca șablonul. Biblioteca comună poate avea context comun; contextul șabloanelor private rămâne privat.

La un apel AI, serviciul încarcă exact contextul versiunii folosite de sesiune. O versiune nouă nu schimbă sesiunile începute pe o versiune anterioară. Cache-ul local nu înlocuiește persistența PostgreSQL, iar repornirea utilitarului nu pierde contextul. Nu se creează o memorie de pacient și nu se preiau date din alte sesiuni. Exemplele sunt referințe de structură/stil, niciodată dovezi despre pacientul curent; validarea factuală folosește în continuare citate din transcriere.

Contextul salvat nu înseamnă antrenarea modelului și nu elimină tokenurile: informația relevantă trebuie transmisă la procesare, apare în dimensiunea contextului și în usage-ul raportat de provider. Păstrează reguli concise și exemple strict anonimizate. Câmpul API este `ai_context` pe `POST /v1/templates`; salvarea se face odată cu publicarea versiunii, nu printr-o mutație a versiunilor vechi.

Exemplu de regulă: „Păstrează negațiile. Nu completa valori lipsă. Folosește unitățile câmpurilor fără conversii sau valori inventate.” Terminologia poate explica aliasuri precum TA, fără să transforme exemplele în valori clinice implicite.

## Definiție compilată pentru extragere

Câmpurile unui șablon sunt compilate într-o definiție compactă: cheie, etichetă, tip și, dacă există, unitate, aliasuri, semantică și repetabilitate. Proprietățile de prezentare, atributele goale și regulile validate deja pe server nu sunt retransmise la fiecare apel. Toate câmpurile rămân disponibile; selecția lexicală nu elimină câmpuri clinice din extragere.

Definiția este memorată într-un cache local limitat la 256 de intrări, indexat prin hash-ul întregii definiții. O schimbare de câmp produce automat altă intrare. Cache-ul conține definiții, nu rapoarte, valori sau răspunsuri clinice.

Definițiile sunt serializate înaintea transcrierii variabile. Se păstrează toate textele și identificatorii fragmentelor curente; nu se elimină negații sau context clinic pentru a obține o economie aparentă. Reviziile, timpii și validarea dovezilor rămân pe server.

Prefixele stabile pot ajuta cache-ul providerului, în funcție de model și lungime. Nu se presupune un cache hit sau un discount: se stochează `cached_tokens` numai dacă este raportat. Vezi [documentația oficială OpenAI despre prompt caching](https://developers.openai.com/api/docs/guides/prompt-caching). Costul operațional afișat rămâne estimat după tarifele configurate; nu se inventează un tarif redus de cache.

## Generarea definițiilor de câmp

Pentru rapoarte explicite `Etichetă: valoare`, parserul local propune definiții fără apel OpenAI. Nu transformă valorile clinice în valori implicite ale șablonului. Intrările narative, identificatorii de pacient și cazurile neclare nu folosesc această scurtătură. Propunerile trebuie în continuare acceptate de medic.

Pentru text liber, se regăsesc cel mult 12 definiții relevante prin căutare lexicală normalizată, din biblioteca comună și șabloanele proprii autorizate. AI primește raportul și aceste definiții, nu întreaga bibliotecă. Nu se caută în rapoartele altor medici și nu se trimit exemple clinice private ale altor utilizatori. Aceasta este regăsire locală de definiții pentru generare, nu un motor RAG semantic extern complet; embeddings și un index extern nu sunt activate automat.

Nu se cache-uiesc răspunsuri clinice între pacienți și nu se generează date clinice din exemplele regăsite. Dacă un raport liber nu beneficiază de context mai mic, economia nu este pretinsă.

## Evidență pe generare și șablon

Fiecare propunere reușită are `generation_id` și usage; parserul local înregistrează explicit zero tokenuri facturabile. La publicare, o generație proprie validată se leagă de prima versiune de șablon publicată. Copierea unui receipt de la alt utilizator este respinsă; republicarea nu dublează costul inițial. Procesările clinice se leagă direct de sesiune și de versiunea șablonului.

Ledgerul păstrează tokenuri de intrare, ieșire, intrare din cache, rezervări, model, operație, utilizator și perioadă. Păstrează și numărul de octeți inițial/trimis, dimensiunea cererii, strategia de context și durata unui apel complet. Nu persistă prompturi sau transcrieri în aceste contoare.

Dashboardul afișează totaluri pe versiune de șablon/operație/model și detalii per procesare. `cached_input_tokens` este un subset al intrării, nu se adună încă o dată la total. Rezervările/erorile incerte sunt afișate separat de tokenurile efectiv raportate. Tokenurile de generare nelegate încă la un șablon publicat rămân vizibile ca propuneri.

Testul sintetic de compilare măsoară **562 → 212 octeți**, păstrând textul și toate câmpurile. Este un rezultat pentru acel fixture, nu un procent garantat de reducere a tokenurilor sau a facturii. Niciun apel OpenAI facturabil nu a fost efectuat pentru această verificare.
