# Pachete de servicii — etapa 1 (1.1.0)

> Istoricul etapei inițiale. În **1.2.0**, abonamentele, creditul AI și evidența plăților verificate sunt implementate conform [SUBSCRIPTIONS.md](SUBSCRIPTIONS.md); acea specificație înlocuiește limitările de mai jos privind consumul ferm și creditul neactivat. Plata online EuPlătesc rămâne neactivată. Optimizarea și statisticile sunt descrise în [CONTEXT_OPTIMIZATION.md](CONTEXT_OPTIMIZATION.md).

WiseMED AI Dashboard păstrează utilitarul standard, autentificarea existentă, runner-ul și distribuția prin update-server. Această etapă adaugă definirea ofertei comerciale și aplicarea limitelor. Nu activează încă abonamente, facturi, încasări EuPlătesc sau cheltuirea creditului.

## Roluri

- **Super admin (`user_type: -1`)**: administratorul instanței; vede utilizatorii din sistem și pachetele lor, publică versiuni de pachet, alocă administrativ un pachet și poate modifica rolul/starea altor utilizatori. Propriul rol este protejat.
- **Admin (`user_type: 0`)**: gestionează membrii organizației prin administrarea existentă; nu poate modifica tarife globale, vedea utilizatorii altor organizații sau acorda drepturi de super admin.
- **Utilizator normal (`user_type: 1`)**: își vede pachetul, specialitățile și consumul propriu. Rolul clinic readonly existent rămâne fără drepturi de mutație.

Identitățile și privilegiile sunt verificate pe server; un număr trimis de client nu conferă privilegii. Tipurile sunt derivate din apartenența verificată și statutul local de administrator al instanței, inclusiv pentru identități WiseMED/WSM mapate explicit.

## Catalogul inițial

| Pachet | Specialități | Șabloane proprii | Tokenuri AI / lună | Stare inițială |
| --- | ---: | ---: | ---: | --- |
| Free | 1 | 3 | 50.000 | Implicit, activ, gratuit |
| Pro | 2 | 25 | 500.000 | Schiță, tarif necompletat |
| Premium | 10 | 100 | 2.000.000 | Schiță, tarif necompletat |

Acestea sunt valori inițiale editabile, nu o recomandare comercială. Prețurile plătite nu au fost inventate. Pro/Premium se activează în catalog numai după definirea unui preț lunar pozitiv. Super adminul este exceptat de limitele comerciale pentru administrare; costul real al apelurilor sale continuă să fie urmărit și limitat de bugetul operațional existent.

Fiecare pachet conține:

- Cod stabil, denumire, descriere, monedă USD/EUR/RON și preț lunar.
- Număr maxim de specialități active și de șabloane proprii.
- Număr de tokenuri AI incluse pe lună.
- Complexitate configurabilă exprimată concret prin numărul maxim de câmpuri/șablon (până la limita motorului de 150).
- Tarif lunar pentru o specialitate suplimentară.
- Tarif lunar pentru un șablon suplimentar.
- Tarif pentru un milion de tokenuri suplimentare.
- Limită implicită de credit și activarea ofertei de suplimente.

Sumele sunt stocate ca **întregi în subdiviziunile monedei** (cenți/bani), fără calcule comerciale în virgulă mobilă. Moneda tarifului unui pachet este aceeași pentru toate suplimentele și limita sa de credit. Costul operațional OpenAI în USD este separat de oferta comercială.

## Versiuni și alocare

Publicarea nu rescrie termenii unui pachet existent: creează o versiune nouă și o face versiunea curentă pentru alocări noi. Utilizatorii cu un pachet alocat păstrează versiunea, moneda și tarifele alocate până la o nouă acțiune explicită de administrare. Pentru utilizatorii fără alocare, se aplică automat versiunea curentă Free.

Super adminul poate aloca un pachet activ și o limită individuală de credit (sau limita implicită a pachetului). Alocarea este auditată și afișată explicit ca **administrativă, fără încasare**. Nu generează o comandă plătită, o factură sau un abonament fictiv.

## Aplicarea limitelor

Limitele se verifică în backend, nu doar prin butoane ascunse. Accesul la o specialitate se activează din „Pachetul meu” sau, dacă există loc în pachet, la prima utilizare. Se poate înlocui selecția de specialități în limita permisă.

Biblioteca inițială de 52 șabloane nu consumă limita șabloanelor proprii. Personalizarea unui șablon comun de către un utilizator normal creează o copie proprie. Crearea unei noi copii/definiții consumă un loc; publicarea unei versiuni noi a aceluiași șablon nu consumă alt loc. Șabloanele proprii sunt izolate între medici. Limita este comună utilizatorului în toate organizațiile sale.

O alocare mai mică nu șterge date: istoricul și rezultatele clinice existente rămân accesibile proprietarului. Dacă selecția de specialități depășește noua limită, utilizatorul trebuie să o reducă înainte de utilizări noi; nu poate crea șabloane peste limita pachetului. Șabloanele existente nu sunt șterse automat.

## Tokenuri și trasabilitate

Alocarea de tokenuri este pentru **intrare + ieșire**, conform contoarelor raportate de provider, pe luna calendaristică UTC. Sunt păstrate separat:

- Tokenurile efective de intrare și de ieșire.
- Rezervarea conservatoare a unui apel în curs sau cu răspuns incert.
- Utilizatorul, organizația, operația, sesiunea, versiunea șablonului și versiunea pachetului.
- Starea cererii și costul operațional estimat pe baza tarifelor OpenAI configurate.

Corecțiile de transcriere care declanșează un apel nou au înregistrări distincte, cu întregul context efectiv procesat de provider. Duplicate idempotente nu produc un apel nou. Refuzurile sau extragerile ulterior invalide păstrează usage-ul facturabil raportat de provider. Modul mock nu inventează tokenuri AI facturabile.

Rezervarea și verificarea alocării utilizatorului sunt serializate tranzacțional, inclusiv pentru cereri simultane în organizații diferite. Costul și tokenurile nu dispar la rollback-ul unei modificări clinice. La depășire, API-ul răspunde `402` cu un cod explicit. Până la implementarea facturării/top-up, limita este fermă: creditul configurat și tarifele suplimentare nu creează datorii sau consum suplimentar automat.

Înregistrările vechi păstrează contoarele existente; legarea retroactivă la sesiune/șablon nu este inventată. Noile înregistrări au atribuirea completă. Istoricul afișează ultimele 200 de procesări; totalurile lunare sunt calculate din toate înregistrările relevante.

## API

- `GET/POST /v1/billing/plans`: catalog / versiune nouă (publicarea numai super admin).
- `GET /v1/billing/account`: pachetul și consumul utilizatorului curent.
- `PUT /v1/billing/specialties`: selecție de specialități cu limită verificată.
- `GET /v1/billing/users`: utilizatori, organizații și versiunea pachetului alocat (super admin).
- `PUT /v1/billing/users/role`: rol și stare globală pentru alt utilizator (super admin, confirmare explicită).
- `POST /v1/billing/assignments`: alocare administrativă explicită (super admin).
- `GET /v1/billing/usage`: ledger propriu; super adminul poate selecta `user_id`.

Contractele complete sunt în [OpenAPI](openapi.json). Tabelele noi au prefix `wai_`, iar migrația este aditivă și tranzacțională.

## Etapa următoare, conform cererii

Abonamente lunare, facturi, comenzi și plata online prin **EuPlătesc**, împreună cu top-up, suplimente cumpărate, sold, limită de credit utilizabilă, notificări și reconciliere. Integrarea reală va folosi documentația EuPlătesc furnizată ulterior; nu sunt fabricate endpointuri, semnături de callback sau confirmări de plată.
