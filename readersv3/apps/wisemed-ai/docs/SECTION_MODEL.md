# Model general de șablon — 1.3.0

Un șablon descrie o structură de document, nu o listă fixă de organe. Secțiunile se pot numi Anamneză, Antecedente, Examen, Ficat, Ventricul stâng, Concluzii sau Recomandări. Fiecare poate avea subsecțiuni, câmpuri și formulări proprii.

## Configurare simplă

În editor, „Secțiuni și variante de redactare” permite adăugarea secțiunilor, alegerea părintelui și configurarea formulărilor. Câmpurile se asociază prin lista „Secțiune”. „Generează structura în document” înlocuiește explicit documentul editorului cu ierarhia și câmpurile asociate; textul se poate personaliza apoi în editorul vizual. JSON rămâne o opțiune avansată.

Fiecare secțiune poate avea variante **Scurt / Standard / Detaliat**. O variantă conține formularea medicului și o condiție:

- `equals`: câmpul trebuie să aibă valoarea documentată exactă. Potrivirea ignoră numai diferențele de litere mari/mici și spațiile de la extremități; nu este un clasificator de diagnostic.
- `documented`: câmpul trebuie să aibă o valoare explicit extrasă, cu stare `present` și proveniență validată.

Condițiile exacte au prioritate față de varianta descriptivă generală. Toate câmpurile inserate în formulare și cerințele explicite trebuie documentate. Câmpurile invizibile nu pot fi introduse în textul unei variante. La insuficiența datelor se păstrează documentul de bază; variantele ambiguue de aceeași prioritate nu sunt alese arbitrar și produc avertisment.

În Standard/Detaliat, o variantă nu poate ascunde alte câmpuri documentate ale aceleiași secțiuni. În Scurt, medicul poate alege o formulare mai compactă; datele structurate și dovezile complete rămân în rezultat. Formulările detaliate nu inventează măsurători sau constatări lipsă. O formulare normală necesită datele explicite care satisfac regula configurată.

O secțiune poate fi ascunsă dacă nu are date. Ascunderea vizuală nu elimină verificarea câmpurilor obligatorii. Condițiile secțiunilor și părinților continuă să controleze când un câmp devine obligatoriu.

## La folosirea șablonului

Medicul dictează; AI interpretează transcrierea în câmpurile selectate și întoarce citate verificabile. Modelul primește definițiile compacte și ierarhia secțiunilor, fără întregul HTML de prezentare. Varianta de redactare se compune din acele fapte validate.

„Stilul raportului” permite un nivel general și excepții pe secțiuni. Subsecțiunile moștenesc opțiunea părintelui, dacă nu au o excepție mai specifică. Schimbarea stilului nu reinterpretează dictarea, nu solicită alt apel AI și nu schimbă valorile clinice. Creează o revizie nouă, cu aceleași dovezi; aprobarea trebuie făcută pe acea revizie. Un document deja aprobat este blocat.

API: `POST /v1/sessions/{id}/presentation`, cu `revision`, `detail` și `sections: {section_id: detail}`. Session.template expune structura exactei versiuni folosite, inclusiv dacă între timp a fost publicată o versiune nouă.

## Adaptarea celor 52 de exemple

Toate cele 52 de exemple au `model_version: 2`, ierarhie, câmpuri asociate și variante de detaliu. Sunt păstrate tipurile/unitățile și cheile existente. Consultațiile includ subsecțiuni opționale de antecedente personale/familiale, tratament anterior, boli cronice, diagnostic, recomandări și rețetă **dictate de medic**. Exemplele cardiologice au subsecțiuni de ventricul stâng/drept în ecocardiografie. Ecografia abdominală grupează descrierea și măsurătoarea ficatului și păstrează structuri independente pentru celelalte constatări.

Exemplele sunt puncte de pornire pentru documentare; nu reprezintă o bibliotecă validată de criterii patologice sau protocoale de diagnostic. Variante specifice altor constatări pot fi definite de medic în același model, fără a schimba motorul aplicației. Nu se generează tratamente sau prescripții din deducții.

La upgrade, aplicația publică versiuni noi numai pentru exemplele comune nepersonalizate. Versiunile vechi și sesiunile existente rămân intacte. Un șablon publicat/personalizat de utilizator nu este rescris. Repetarea pornirii nu creează alte versiuni ale catalogului deja adaptat.

## Dicționarul fiecărei structuri

`speech_terms` este o listă de expresii medicale publicată cu secțiunea șablonului. Editorul permite modificarea ei, câte o expresie pe rând. Limitele sunt 64 de expresii și 2.048 de octeți UTF-8 (inclusiv separatorii) pe secțiune; nu se acceptă numere, date individuale sau expresii duplicate. Se aplică aceleași reguli de terminologie ca vocabularului vocal existent.

Pachetul `speech-adaptation` combină dicționarul versiunii cerute cu termenii aprobați separat. O stare salvată explicit, inclusiv `rejected` sau `candidate`, are prioritate pentru aceeași expresie și secțiune. Sursa `template` indică o expresie publicată cu șablonul. Bugetul activ este comun telefonului și extragerii OpenAI. Contextul compact trimite termenii în `sections[].speech_terms`; aceștia ajută și `display_value`, dar nu sunt dovezi clinice. Transcrierea și citatele justificative rămân sursa constatărilor. Pachetul serverului oferă expresiile; aplicarea lor în decodare depinde de capabilitățile motorului de pe telefon și nu implică instalarea sau antrenarea unui model nou.

Ecografia tiroidiană, model 4, este configurată din cele opt variante ale formularului HitMed „Ecografie de tiroida.docx”. Structurile sunt capsula/aspectul general, lobul drept, istmul și joncțiunile istmico-lobare, lobul stâng, vascularizația, ganglionii și sinteza. Dicționarul are 72 de expresii distincte, distribuite și în subsecțiunile relevante. Cheile câmpurilor și identificatorii subsecțiunilor `camp_*` sunt păstrați; nodulii și ganglionii rămân repetabili. Nu se copiază măsurătorile cazurilor, nu se deduc diagnostice sau normalitate și nu se transformă valoarea ambiguă „0.3.2 cm” într-o măsurătoare certă. Contextul de interpretare/redactare al catalogului este salvat criptat, ca la publicarea din editor.
