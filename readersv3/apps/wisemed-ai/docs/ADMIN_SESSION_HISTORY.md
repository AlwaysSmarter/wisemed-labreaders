# Istoricul administrativ al sesiunilor

În „Sesiuni și rezultate”, administratorul poate consulta sesiunile utilizatorilor,
filtrate după utilizator și stare, în pagini de 50. Lista arată proprietarul,
organizația, șablonul, data, starea și existența rezultatului. Detaliile includ
raportul, transcrierea, datele structurate și proveniența.

- Administratorul instanței (`instance_admin`) vede toate organizațiile.
- Administratorul unei organizații (`role=admin`) vede utilizatorii organizației sale.
- Medicul și utilizatorul de tip viewer nu au acces la aceste rute administrative.

Consultarea este separată de API-urile clinice personale. Nu permite modificarea,
aprobarea, ștergerea sau atribuirea sesiunii altui medic. Pentru sesiunea proprie,
administratorul poate reveni în spațiul clinic obișnuit. Accesul la detalii produce
un eveniment de audit `session.admin.read`, cu identitatea administratorului.
Raportul HTML se afișează într-un iframe sandbox fără scripturi.

API-uri:

- `GET /v1/admin/sessions?user_id=<uuid>&status=recording|review|approved&offset=0`
  returnează `items`, `users`, `total`, `offset`, `limit`, `instance_scope`, `read_only`.
  Filtrele sunt opționale. Lista utilizatorilor respectă același domeniu de acces.
- `GET /v1/admin/sessions/<uuid>` returnează `session`, `owner`, `read_only`.
  Decriptarea folosește organizația proprietarului, inclusiv la consultarea între
  organizații de către administratorul instanței.

Rutele normale `/v1/sessions` rămân personale. Indicatorii și sesiunile recente
din „Privire de ansamblu” sunt etichetate explicit ca personale pentru administratori.
Interfața de pe mobil afișează serverul și contul lângă istoricul personal.

Livrarea necesită backendul și bundle-ul web reconstruite; nu necesită migrare SQL.
Testele PostgreSQL acoperă paginarea, filtrele, decriptarea raportului și transcrierii,
auditul, izolarea organizațiilor, refuzul accesului medicilor și refuzul mutațiilor.
Testul Playwright verifică selecția utilizatorului, paginarea, raportul sandbox și
afișarea pe mobil.
