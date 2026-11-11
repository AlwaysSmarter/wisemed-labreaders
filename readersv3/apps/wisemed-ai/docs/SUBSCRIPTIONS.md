# Abonamente și credit — 1.2.0

WiseMED AI rămâne utilitarul standard, cu același runner, instalare ca serviciu și mecanism comun de update. Niciun release nu se publică automat pe update-server.

## Ciclu lunar și cinci zile de grație

Utilizatorul poate activa explicit un pachet plătit activ; super adminul îl poate activa pentru alt utilizator. Acceptarea precizează prețul și termenii. Activarea creează un abonament și primul document intern de plată, fără să declare încasarea banilor. Pachetele alocate administrativ anterior nu sunt transformate automat în abonamente. Prima activare primește alocarea pachetului pentru perioada curentă; utilizările dinaintea acceptării nu sunt facturate retroactiv. Contoarele „Pachetul meu” arată perioada abonamentului, iar istoricul/statisticile păstrează și procesările anterioare.

Perioada este luna calendaristică **UTC**, în acord cu alocarea tokenurilor. Prima lună se facturează integral, fără proratare; următoarea reînnoire este în prima zi a lunii următoare. Prețul și limitele sunt fixate la versiunea acceptată a pachetului. Editarea catalogului nu rescrie termenii unui abonament existent. Alocarea administrativă nu poate ocoli sau schimba termenii unui abonament activ.

Fiecare sumă emisă are o scadență de **cinci zile de la emitere**. Reînnoirea lunară este idempotentă, inclusiv când aplicația repornește ori mai mulți workeri rulează simultan. Workerul verifică periodic; API-ul verifică înainte de acces, astfel că blocarea nu depinde de executarea la secundă a unui job.

În perioada de grație, dashboardul arată suma și scadența. La scadență, dacă există restanțe, funcțiile de lucru sunt suspendate. Autentificarea, profilul, soldul și mecanismul de recuperare rămân accesibile. Suspendarea financiară nu setează utilizatorul ca dezactivat administrativ și nu îi schimbă rolul. Interfața reverifică soldul la 30 de secunde; reîncărcarea paginii îl actualizează imediat. Super adminul păstrează accesul necesar administrării.

## Credit pentru tokenuri suplimentare

Creditul comercial folosește moneda pachetului; exemplul de 10 USD se introduce ca 1000 de cenți. Costul comercial nu se confundă cu costul operațional OpenAI în USD.

Consumul peste tokenurile incluse este permis numai dacă există abonament, oferta de suplimente este activă și tariful pe milion de tokenuri suplimentare este pozitiv. Creditul se aplică **consumului AI**, nu prețului lunar al abonamentului. Specialitățile și șabloanele suplimentare au tarife definite, dar achiziția automată a acestor locuri nu este încă implementată.

Calculul se face cumulativ per perioadă:

```
consum suplimentar = max(0, tokenuri efective intrare + ieșire − tokenuri incluse)
cost în subdiviziuni = ceil(consum suplimentar × tarif pe milion / 1.000.000)
```

Calculul folosește aritmetică întreagă, inclusiv pentru produs, fără erori de virgulă mobilă. Rotunjirea se aplică totalului perioadei, nu fiecărui token sau fiecărui apel.

Sumele neachitate pentru consum AI și consumul încă neemis folosesc același credit. Rezervările conservative ale procesărilor în curs/incerte reduc separat creditul disponibil, inclusiv dacă provin dintr-o lună anterioară. Verificarea și rezervarea sunt serializate per utilizator, în toate organizațiile sale. O cerere care ar depăși creditul disponibil este respinsă înainte de apelul providerului; nu se produce o datorie peste limită intenționat.

Când consumul efectiv ajunge la limită, accesul de lucru se suspendă. Utilizatorul poate pregăti consumul curent pentru plată înainte de sfârșitul lunii; emiterea repetată include numai diferența încă neemisă. La închiderea lunii se emite automat diferența rămasă. Noua alocare lunară de tokenuri nu șterge datoriile sau rezervările vechi.

## Plăți și reactivare

Până la integrarea EuPlătesc, **numai super adminul poate înregistra o plată efectiv primită și verificată**, cu referință unică și confirmare explicită. Referința este cheie de idempotency: repetarea aceleiași plăți nu dublează încasarea; altă sumă/document cu aceeași referință produce conflict. Sunt acceptate plăți parțiale, fără supraplata documentului. Utilizatorul nu își poate confirma singur plata.

După înregistrare, starea se recalculează. Accesul revine când nu mai există sume scadente neachitate și creditul efectiv nu mai este epuizat. Plata abonamentului nu stinge separat o datorie pentru consum AI. Operațiile sunt tranzacționale și auditate.

Aceste documente sunt **evidențe interne ale soldului**, nu facturi fiscale/e-Factura. Nu sunt implementate încasări EuPlătesc, callbackuri inventate, debitarea automată a cardului, rambursări, schimbări/proratări ori anularea abonamentului din interfață. Documentația EuPlătesc și regulile fiscale/comerciale pentru aceste operații urmează să fie furnizate. Nu se declară fluxul comercial complet gata pentru încasări online.

## API

- `POST /v1/billing/subscriptions`: activare cu acceptare explicită; `plan_version`, `confirmed`, opțional `user_id` pentru super admin.
- `GET /v1/billing/status`: stare și sold; alt `user_id` numai pentru super admin.
- `GET /v1/billing/invoices`: ultimele 200 de documente, sume achitate/rest și scadențe.
- `POST /v1/billing/settle`: emiterea diferenței de consum efectiv pentru plată, `confirmed: true`.
- `POST /v1/billing/payments`: plată verificată, numai super admin; `invoice_id`, `amount_minor`, `reference`, `confirmed`.
- `GET /v1/billing/statistics`: statistici proprii pe șablon/operație; `period=all` pentru întregul istoric, alt `user_id` numai pentru super admin.

Erori comerciale `402`: `subscription_payment_overdue`, `credit_limit_exhausted`, `credit_reservation_exceeds_available`, respectiv `ai_token_allowance_exceeded` când suplimentele nu sunt permise.

Migrațiile 008 și 009 sunt aditive; toate obiectele noi păstrează prefixul `wai_`. Nu se modifică parolele, cheile de criptare sau conexiunile existente.
