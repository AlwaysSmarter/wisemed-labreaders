export const templateHelp = {
 sectionVocabulary:{title:'Dicționarul structurii pentru dictare și redactare',text:'Expresii de terminologie pentru această secțiune, câte una pe rând. Exemplu pentru un lob tiroidian: microplaje cu ecogenități diferite, striuri fibroase, aspect pseudonodular. Sunt publicate cu versiunea șablonului, trimise telefonului în pachetul de adaptare vocală și modelului OpenAI pentru extragere și redactare. Nu sunt constatări și nu se inserează automat în rezultat. Maximum 64 de expresii și 2.048 de octeți pe secțiune, fără nume sau valori individuale. Termenii aprobați separat în vocabular se combină cu acest dicționar; o respingere explicită exclude termenul din dicționarul activ al acelei secțiuni. Efectul asupra recunoașterii vocale depinde de motorul folosit pe telefon.'},
  editing:{title:'Editarea textului șablonului',text:'Documentul este editabil direct: apasă în text, scrie, șterge sau lipește conținut. Selectează o porțiune pentru formatare. Câmpurile verzi sunt variabile, configurabile separat din panoul din dreapta. Bara arată formatarea selecției. Modificările sunt salvate la publicarea unei versiuni. În modul HTML, revino la editorul vizual pentru a folosi controalele WYSIWYG.'},
  tables:{title:'Editarea tabelelor',text:'Inserează tabel creează un tabel cu 3 rânduri și 3 coloane, primul rând fiind antet. Apasă într-o celulă pentru controalele de rânduri, coloane, antet și ștergere. Pentru unire, selectează mai multe celule vecine; pentru separare, selectează o celulă unită. Poți scrie și insera câmpuri în celule. Acțiunile indisponibile sunt dezactivate. Anulează poate reveni asupra modificărilor din editor.'},
  "name": {
    "title": "Nume",
    "text": "Denumirea șablonului în bibliotecă și la alegerea unei sesiuni. Exemplu: „Ecografie abdominală”. Titlul din document se editează separat în zona de redactare."
  },
  "specialties": {
    "title": "Specialități",
    "text": "Asociază șablonul cu una sau mai multe specialități, pentru filtrare și acces. Alege cel puțin una. Pentru conturile obișnuite sunt disponibile specialitățile active din pachet."
  },
  "publish": {
    "title": "Publicarea și copierea",
    "text": "Publică versiune salvează modificările într-o versiune nouă. Sesiunile existente păstrează versiunea inițială. Copiază creează un șablon separat, cu „· copie” în nume. Adaugă la șabloanele mele personalizează un șablon din bibliotecă. Un șablon nou ocupă un loc din pachet; dacă este necesar un supliment plătit, costul trebuie confirmat."
  },
  "fields": {
    "title": "Câmpuri structurate",
    "text": "Sunt informațiile extrase din dictare și folosite în document, de exemplu dimensiunea ficatului. Selectează un câmp pentru configurare. Asteriscul marchează un câmp obligatoriu, iar ochiul tăiat indică un câmp invizibil. Adaugă un câmp îl creează și îl inserează la cursor."
  },
  "key": {
    "title": "Identificator",
    "text": "Cheia unică a câmpului, folosită în variabile și reguli: de exemplu ficat_dimensiune, inserat ca {{ficat_dimensiune}}. Începe cu o literă mică; folosește litere mici, cifre și _, maximum 64 de caractere. La redenumire verifică și referințele scrise manual în formulări sau JSON."
  },
  "label": {
    "title": "Etichetă",
    "text": "Numele lizibil al câmpului, afișat în editor și transmis pentru interpretarea dictării. Exemplu: „Dimensiune ficat”. Identificatorul rămâne cheia tehnică; eticheta poate conține spații și diacritice."
  },
  "type": {
    "title": "Tip",
    "text": "string: text liber, de exemplu „contur regulat”. \n\nnumber: valoare numerică, de exemplu 120, cu unitatea configurată separat. \n\nboolean: valoare logică true/false, de exemplu prezența unui semn documentat. Lipsa unei mențiuni nu înseamnă automat false."
  },
  "unit": {
    "title": "Unitate",
    "text": "Unitatea asociată valorii, de exemplu mm, cm sau ml. Se transmite împreună cu câmpul în datele structurate. La generarea structurii, unitatea este adăugată după câmp. Verifică să corespundă valorilor și limitelor configurate."
  },
  "min": {
    "title": "Min",
    "text": "Limita numerică minimă acceptată pentru câmpurile de tip number. O valoare extrasă sub limită produce o avertizare și o eroare de validare; nu este corectată automat. Lasă gol pentru a nu impune o limită inferioară. Nu reprezintă automat un prag clinic de normalitate."
  },
  "max": {
    "title": "Max",
    "text": "Limita numerică maximă acceptată pentru câmpurile de tip number. O valoare peste limită produce o avertizare și o eroare de validare. Lasă gol pentru a nu impune o limită superioară. Min trebuie să fie mai mic sau egal cu Max."
  },
  "aliases": {
    "title": "Aliasuri",
    "text": "Denumiri alternative sau abrevieri care ajută la recunoașterea aceluiași câmp în dictare. Separă-le prin virgulă. Exemplu: pentru „Sept interventricular”, scrie „SIV, sept interventricular”. Mențiunile pot fi astfel asociate aceleiași chei; nu se creează câmpuri separate și nu se completează valori fără dovezi."
  },
  "semantic": {
    "title": "Semantic tag",
    "text": "O etichetă de sens transmisă în contextul de extragere, pentru a clarifica ce reprezintă câmpul. De exemplu „interventricular_septum_thickness”. Este opțională și nu înlocuiește identificatorul, aliasurile sau o mapare către un sistem extern."
  },
  "fieldHelp": {
    "title": "Explicație pentru completare",
    "text": "Instrucțiune specifică acestui câmp, afișată în editor și inclusă în contextul de extragere. Exemplu: „Extrage grosimea SIV în mm numai dacă este dictată explicit”. Explică sensul și modul de completare fără a introduce date de pacient sau valori presupuse."
  },
  "fieldSection": {
    "title": "Secțiune",
    "text": "Grupul din document căruia îi aparține câmpul. Exemplu: câmpul „Dimensiune ficat” aparține secțiunii „Ficat”. Fără secțiune îl lasă în afara grupurilor. Asocierea este folosită la generarea structurii, la condițiile de afișare și la variantele de redactare."
  },
  "required": {
    "title": "Obligatoriu",
    "text": "Dacă lipsește din datele extrase, câmpul este semnalat ca lipsă pentru revizuire, când secțiunea sa este activă. Bifarea nu inventează o valoare și nu transformă o absență a mențiunii în rezultat normal."
  },
  "invisible": {
    "title": "Invizibil în document",
    "text": "Câmpul rămâne disponibil în datele structurate și poate participa la reguli, dar valoarea lui nu este afișată prin variabila din document. Util pentru un indicator care controlează o secțiune. Textul fix scris în jurul variabilei rămâne în document."
  },
  "repeatable": {
    "title": "Repetabil",
    "text": "Permite păstrarea mai multor înregistrări extrase pentru aceeași cheie, sub formă de listă. De exemplu, mai multe observații pentru același tip de leziune. Nu duplică automat structura secțiunilor din editor; verifică și felul în care lista este afișată în raport."
  },
  "insert": {
    "title": "Inserează în document",
    "text": "Inserează câmpul selectat la poziția cursorului din editor. La generarea raportului, variabila este înlocuită cu valoarea extrasă. Poți insera aceeași cheie în mai multe locuri fără să creezi alte câmpuri."
  },
  "format": {title:'Formatarea documentului',text:'Text normal și Titlu 1–3 aleg tipul paragrafului. Aldin, Cursiv, Subliniat și Tăiat se aplică textului selectat sau textului scris în continuare. Șterge formatarea elimină marcajele și readuce blocul la text simplu.\n\nAlinierea poate fi la stânga, centrată, la dreapta sau justificată. Listele pot avea marcatori sau numere; indentarea se aplică elementelor de listă. Citat marchează un bloc distinct; Linia de separare inserează un separator.\n\nAnulează și Refă navighează în istoricul editărilor curente. Butoanele active indică formatarea de la cursor. Inserează tabel creează un tabel editabil; alte controale apar la selectarea unei celule.'},
  "html": {
    "title": "HTML și variabile",
    "text": "HTML comută între editorul vizual și sursa documentului. Câmpurile folosesc {{identificator}} sau noduri cu data-field; secțiunile folosesc data-section cu identificatorul secțiunii. {{patient_ref}} și {{doctor_name}} sunt variabile de sesiune. Modificarea sursei necesită păstrarea acestor legături."
  },
  "fragments": {
    "title": "Fragmente normal/patologic",
    "text": "Fragmentele sunt bucăți de text pregătite de tine, pe care le poți insera manual la cursor, ca să nu le rescrii. „Normal” și „patologic” sunt două categorii de texte reutilizabile; aplicația nu alege singură între ele.\n\nExemplu de fragment normal: „Ficat cu contur regulat și ecostructură omogenă.”\nExemplu de fragment patologic: „La nivelul ficatului se descrie: {{ficat_descriere}}.” Variabila se completează ulterior din datele dictate, dacă ai definit câmpul ficat_descriere în șablon.\n\nCum le configurezi: deschide „Reguli, secțiuni și fragmente” și scrie în „Fragmente normal/patologic” un obiect JSON, de exemplu:\n{\"normal\":\"<p>Ficat cu contur regulat și ecostructură omogenă.</p>\",\"patologic\":\"<p>La nivelul ficatului se descrie: {{ficat_descriere}}.</p>\"}\n\nCum le folosești: revino la editorul vizual, așază cursorul unde vrei textul și apasă Fragment normal sau Fragment patologic. Textul se adaugă în document și poate fi editat. La fiecare apăsare este inserată o nouă copie. Publică versiunea pentru a salva.\n\nAtenție la textul fix: dacă inserezi descrierea „normală”, ea rămâne în șablon și va apărea în documentele generate până o modifici sau o elimini. Butonul nu verifică dacă afirmația este susținută de dictare. Folosește aceste texte numai când se potrivesc scopului șablonului.\n\nPentru formulări selectate automat pe baza datelor documentate folosește variantele de redactare ale secțiunii, cu condiții de aplicare. Fragmentele sunt scurtături de editare manuală."
},
  "sections": {
    "title": "Secțiuni și variante de redactare",
    "text": "Secțiunile grupează câmpurile și pot avea subsecțiuni, condiții de afișare și formulări alternative. Exemplu: „Examen obiectiv” conține „Abdomen”, care include câmpurile sale. Adaugă secțiune creează un grup configurabil; folosește apoi Secțiune în configurarea câmpurilor pentru asociere."
  },
  "sectionName": {
    "title": "Denumirea secțiunii",
    "text": "Titlul lizibil al grupului, de exemplu „Ficat” sau „Recomandări”. Generarea structurii îl introduce ca titlu în document. Redenumirea nu schimbă identificatorul intern; textul deja existent în document poate necesita actualizare."
  },
  "parent": {
    "title": "În cadrul secțiunii",
    "text": "Alege secțiunea părinte, adică grupul în interiorul căruia se află această subsecțiune. Exemplu: pentru „Ficat”, alegi „Abdomen”. Nivel principal înseamnă că secțiunea nu are părinte. Secțiunea depinde și de activarea părintelui; aplicația exclude alegerile care ar crea o buclă."
  },
  "detail": {
    "title": "Nivel de detaliu implicit",
    "text": "Alege nivelul folosit la selectarea variantelor de redactare. Scurt (brief): permite o formulare condensată. \n\nStandard (standard): o variantă trebuie să acopere câmpurile vizibile documentate. \n\nDetaliat (detailed): selectează formulările marcate Detaliat, cu aceeași cerință de acoperire. Textele sunt cele scrise de tine; nivelul nu generează automat o formulare mai lungă. Fără o variantă potrivită se păstrează documentul de bază. \n\nDin șablon: nu impune un nivel local; se moștenește nivelul secțiunii părinte sau al șablonului. Alegerea din sesiune poate suprascrie aceste valori."
  },
  "sectionCondition": {
    "title": "Arată secțiunea numai când câmpul",
    "text": "Întotdeauna nu impune o condiție pe un câmp. Dacă alegi un câmp, secțiunea se afișează numai când valoarea acestuia corespunde valorii configurate mai jos. Condițiile părintelui și ascunderea la lipsa datelor se aplică în continuare."
  },
  "conditionValue": {
    "title": "Are valoarea exactă",
    "text": "Valoarea necesară pentru activarea secțiunii. Exemplu: alegi câmpul „Examinare efectuată” și valoarea true pentru un câmp boolean. Folosește valoarea așteptată a câmpului; nu este o căutare de cuvinte în întreaga dictare."
  },
  "hideEmpty": {
    "title": "Ascunde secțiunea dacă nu există date documentate",
    "text": "Elimină secțiunea din raport când nu există date pentru câmpurile ei sau ale subsecțiunilor. Debifat, secțiunea poate rămâne cu structura de bază și marcaje pentru date nedocumentate. Nu anulează verificarea câmpurilor obligatorii."
  },
  "variantName": {
    "title": "Denumirea variantei",
    "text": "Nume de identificare pentru o formulare alternativă, de exemplu „Descriere scurtă” sau „Aspect documentat normal”. Numele nu este formularea inserată în raport; aceasta se scrie în Formularea medicului."
  },
  "variantDetail": {
    "title": "Nivel de detaliu al variantei",
    "text": "Scurt: variantă pentru nivelul brief, care poate reda un subset al câmpurilor documentate. \n\nStandard: variantă pentru nivelul standard. \n\nDetaliat: variantă pentru nivelul detailed. Pentru Standard și Detaliat, formularea și câmpul de condiție trebuie să acopere câmpurile vizibile documentate din secțiune. Varianta se poate aplica numai dacă nivelul ei coincide cu nivelul ales pentru secțiune și are toate dovezile necesare."
  },
  "variantField": {
    "title": "Se aplică dacă acest câmp",
    "text": "Câmpul ale cărui date susțin aplicarea variantei. Exemplu: un câmp „Aspect ficat”. Regula de aplicare stabilește dacă este necesară o anumită valoare sau doar documentarea explicită a câmpului."
  },
  "operator": {
    "title": "Regulă de aplicare",
    "text": "Valoare exactă: câmpul trebuie documentat și valoarea sa trebuie să corespundă celei configurate; comparația ignoră majusculele și spațiile de la margini. \n\nCâmp documentat explicit: este suficient ca acel câmp să aibă o valoare documentată, fără o valoare fixă cerută. O variantă potrivită cu valoare exactă are prioritate față de una bazată doar pe documentare."
  },
  "variantValue": {
    "title": "Conține valoarea documentată exactă",
    "text": "Valoarea câmpului care permite aplicarea variantei. Exemplu: „normal”, numai dacă acest rezultat este documentat. Este o comparație a valorii complete, nu o căutare de subșir. Pentru boolean folosește true sau false. Absența datelor nu îndeplinește condiția."
  },
  "wording": {
    "title": "Formularea medicului",
    "text": "Textul care înlocuiește conținutul secțiunii când varianta este eligibilă. Butoanele + de sub text adaugă {{identificator}} pentru câmpurile asociate. Exemplu: „Ficat cu diametrul de {{ficat_dimensiune}} mm.” Fiecare variabilă inserată trebuie susținută de date. Dacă lipsesc dovezi sau se potrivesc mai multe variante, se păstrează conținutul de bază; ambiguitatea este semnalată."
  },
  "variantActions": {
    "title": "Adăugarea și eliminarea variantelor",
    "text": "Adaugă variantă creează o formulare alternativă; este disponibil după ce există cel puțin un câmp. Elimină varianta o scoate din versiunea în curs de editare. Modificările devin active prin publicarea șablonului."
  },
  "removeSection": {
    "title": "Elimină secțiunea",
    "text": "Șterge secțiunea din configurația în curs de editare. Butonul este dezactivat cât timp există câmpuri asociate sau subsecțiuni: mută-le mai întâi. Verifică separat conținutul documentului, care poate păstra structura HTML veche."
  },
  "compose": {
    "title": "Generează structura în document",
    "text": "Înlocuiește întregul conținut al editorului cu titlul șablonului, secțiunile în ierarhia configurată și câmpurile asociate. Conținutul redactat manual este înlocuit. Variantele nu sunt inserate acum; se aleg ulterior în funcție de date și nivelul de detaliu. Publică pentru a salva rezultatul."
  },
  "context": {
    "title": "Context AI al șablonului",
    "text": "Instrucțiuni persistente legate de versiunea publicată, folosite când modelul extrage date pentru acest șablon. Modificările se activează la publicare. Contextul contribuie la tokenurile de intrare. Include doar instrucțiuni și exemple anonimizate, fără date de pacient."
  },
  "instructions": {
    "title": "Reguli de completare",
    "text": "Instrucțiuni generale de extragere pentru șablon, maximum 2.000 de caractere. Exemplu: „Completează măsurătorile numai dacă sunt dictate; păstrează separat observațiile pentru fiecare organ”. Aceste instrucțiuni ajută modelul și nu înlocuiesc regulile declarative de validare."
  },
  "terminology": {
    "title": "Terminologie și abrevieri",
    "text": "Dicționar de sens pentru interpretarea raportului, maximum 1.500 de caractere. Exemplu: „SIV = sept interventricular”. Pentru sinonime ale unui singur câmp folosește și Aliasuri. Acest context nu configurează vocabularul motorului de recunoaștere vocală."
  },
  "examples": {
    "title": "Exemple anonimizate acceptate",
    "text": "Exemple de formulări și interpretări dorite, maximum 2.000 de caractere. Folosește cazuri fictive sau anonimizate. Sunt orientative pentru extragere; valorile din exemple nu trebuie copiate în raport când nu sunt susținute de dictare."
  },
  "proposals": {
    "title": "Propune câmpuri dintr-un raport existent",
    "text": "Trimite un raport de referință providerului configurat pentru a sugera câmpuri. Raportul trebuie anonimizat înainte de trimitere. Citește propunerile și citatele justificative; șablonul se modifică numai după acceptare."
  },
  "report": {
    "title": "Raport de referință",
    "text": "Text din care sunt propuse denumiri, identificatori, tipuri și unități de câmpuri. Minimum 5 și maximum 20.000 de caractere. Elimină numele, identificatorii și celelalte date personale înainte de trimitere."
  },
  "accept": {
    "title": "Acceptă propunerile în editor",
    "text": "Adaugă câmpurile propuse ale căror chei nu există deja și le inserează la cursor în document. Verifică tipurile, unitățile și poziția în raport. Acceptarea modifică doar editorul; publicarea salvează noua versiune."
  },
  "json": {
    "title": "Reguli, secțiuni și fragmente",
    "text": "Configurare avansată în JSON. Păstrează sintaxa validă și cheile existente. Secțiunile au și editor vizual. După modificări manuale în Secțiuni JSON, verifică referințele și publică; acest text este citit la salvare și nu actualizează imediat editorul vizual al secțiunilor."
  },
  "rules": {
    "title": "Reguli JSON",
    "text": "Listă de reguli care fac un alt câmp obligatoriu în funcție de o valoare. Exemplu: [{\"field\":\"leziune\",\"equals\":\"true\",\"required_field\":\"dimensiune\"}]. Dacă leziune este true, lipsa câmpului dimensiune este semnalată. Regulile nu generează valori și nu selectează formulări."
  },
  "sectionsJSON": {
    "title": "Secțiuni JSON",
    "text": "Lista secțiunilor, cu id, label, parent, condiții și eventual variants. id leagă secțiunea de câmpuri și de data-section din HTML; parent definește ierarhia. Folosește editorul vizual pentru modificările obișnuite și păstrează identificatorii când editezi manual."
  },
  "vocabulary": {
    "title": "Vocabular medical pentru dictare",
    "text": "Expresii medicale asociate secțiunilor, folosite pentru adaptarea dictării în funcție de capabilitățile motorului. Publică mai întâi secțiunile noi. Este separat de aliasurile câmpurilor și de contextul pentru extragere. Redenumirea sau reordonarea unei secțiuni îi păstrează asocierea prin identificator."
  },
  "medicalTerm": {
    "title": "Termen sau expresie medicală",
    "text": "O expresie medicală, de exemplu „sept interventricular”, maximum 160 de caractere. Aprobă termenul îl salvează în vocabularul secțiunii. Sunt permise maximum 64 de expresii și 2.048 de octeți activi pe secțiune. Nu introduce date personale sau măsurători individuale."
  },
  "vocabStatus": {
    "title": "Stare / sursă",
    "text": "Starea indică dacă termenul este aprobat, respins sau în așteptarea revizuirii; sursa arată originea sa. Aprobă îl acceptă în vocabularul activ. Respinge îl exclude. Aceste acțiuni salvează vocabularul direct, separat de publicarea conținutului șablonului."
  },
  "vocabOrigin": {
    "title": "Observări / proveniență",
    "text": "Numărul de observări și informațiile despre prima observare și proveniența termenului. Ajută la evaluarea propunerii; frecvența singură nu confirmă corectitudinea medicală. „manual” indică proveniența manuală atunci când nu este specificată alta."
  },
  "search": {
    "title": "Caută după specialitate sau șablon",
    "text": "Filtrează biblioteca după textul din denumirea șablonului sau specialitățile sale. Căutarea ignoră majusculele. Pentru conturile obișnuite biblioteca este limitată la specialitățile active din pachet."
  },
  "library": {
    "title": "Biblioteca de șabloane",
    "text": "Fiecare card arată specialitățile, versiunea publicată, proveniența și numărul de câmpuri, inclusiv cele obligatorii. Inclus în bibliotecă este un șablon comun; Șablon propriu este personalizat; Supliment lunar ocupă un loc suplimentar plătit. Numărul din titlu reprezintă rezultatele filtrate."
  },
  "create": {
    "title": "Creează șablon",
    "text": "Deschide un șablon nou în editor. Configurează numele, specialitățile, documentul, câmpurile și secțiunile, apoi publică. Necesită drept de editare și o specialitate disponibilă. Un șablon nou ocupă un loc din pachet."
  },
  "edit": {
    "title": "Editează / Personalizează o copie",
    "text": "Editează deschide șablonul pentru publicarea unei versiuni noi. Personalizează o copie pornește de la un șablon comun și salvează o copie proprie. Sesiunile deja create păstrează versiunea cu care au fost deschise."
  },
  "synthetic": {
    "title": "Test sintetic",
    "text": "Creează o sesiune cu referința DEMO-SINTETIC și încarcă un exemplu de dictare pentru verificarea șablonului. Necesită acces la specialitatea șablonului. Sesiunea apare în fluxul normal; extragerea ulterioară folosește providerul configurat și poate consuma tokenuri."
  }
} as const

export type TemplateHelpTopic = keyof typeof templateHelp
