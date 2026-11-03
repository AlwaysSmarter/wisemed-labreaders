# Audit CNAS: PIAS 3.7.34 și DocMED 1.0

Verificat la 2 noiembrie 2026 în documentele locale `docs/CNAS` și pe portalul oficial:

- [Specificații SIUI/SIPE/CEAS](https://portal.cnas.ro/cnas/pias/specificatii), PIAS 3.7.34, 20.10.2026; WSDL 14.10.2026 și structuri de raportare 29.10.2026.
- [Specificații DocMED](https://portal.cnas.ro/cnas/pias/docmed), versiunea inițială 1.0, 20.10.2026; WSDL și XSD 14.10.2026.
- PDF local PIAS principal, pagina 47: origini SIUI/SIPE, CEAS și OCSP.
- PDF local DocMED, pagina 5: cele patru servicii și autentificarea comună SIUI.
- `DefinitiiServiciiWeb`, `DefinitiiServiciiDocMed`, `StructuriPrescriptie`, `StructuriDocMed`.

`docs/CNAS/Docs.txt` este un snapshot vechi (7 aprilie 2026, PIAS 3.7.32); adresele și regula 444 din acel fișier nu reprezintă documentația actualizată.

## Destinații actuale

| Sistem / serviciu | Producție | Test / observații |
| --- | --- | --- |
| SIUI, validări, asigurați, eFactura | `https://siui.e-pias.ro/svapntws/services/<serviciu>` | `https://testsiui.siui.ro` |
| OCSP comun | `https://siui.e-pias.ro/OCSP/validator` | Urmează originea SIUI configurată |
| SIPE | `https://sipe.e-pias.ro/svapntws/services/EPrescriptionWS` | `https://testsipe.siui.ro` |
| CEAS / eCard.SDK | `tcp://umceas.siui.ro:443` | `tcp://testumceas.siui.ro:443` |
| DocMED DocMedService | `https://docmed.e-pias.ro/loader/ws` | Portalul nu publică o origine de test separată |
| DocMED DocumentSeries | `https://docmed.e-pias.ro/seq/ws` | Același certificat și aceleași credențiale SIUI |
| DocMED MedicalDocumentDownloader | `https://docmed.e-pias.ro/download/ws` | OCSP comun SIUI |
| DocMED DocStatusChangerService | `https://docmed.e-pias.ro/status/ws` | OCSP comun SIUI |

Originea HTTPS implică portul 443. Documentația nouă nu mai conține instrucțiunea veche de alegere a portului 444 pentru AlfaSign/CertDigital. Aceasta confirmă schimbarea configurației publicate, nu demonstrează că toate listenerele 444 sunt oprite. Nu se deduce și nu se testează folosind credențiale reale. CEAS își păstrează portul TCP 443 în PDF.

## Este DocMED un serviciu nou? Au fost mutate metodele SIUI?

Da, istoricul PDF DocMED îl numește versiune inițială. Acoperă transmiterea/descărcarea formularelor medicale, gestionarea seriilor și a stării (consumat/neconsumat), cu 22 de operații în patru WSDL-uri. Formularele includ trimiteri clinice și paraclinice, scrisori medicale, concedii medicale, prescripții și recomandări/planuri.

Nu este documentată o mutare generală sau o înlocuire automată a API-ului SIUI. Comparația WSDL-urilor noi cu cele încorporate anterior în bridge arată că toate operațiile existente din SiuiWS, SiuiValidateWS, SiuiInsuredWS, SiuiEInvoiceWS și EPrescriptionWS sunt păstrate. EPrescriptionWS adaugă `getPrescriptionsSeriesNo(requestXml)`. Schemele acestei metode descriu istoricul rețetei după serie/număr, nu alocarea de numere noi. În consecință metodele existente de validare/obținere bilete sau rețete nu sunt redirecționate către DocMED.

DocMED folosește contracte SOAP document/literal în namespace-ul `http://www.cnas.ro/pias/2.0`; integrarea lor necesită modele și tratarea răspunsurilor specifice, distincte de RPC-ul SIUI. Lista operațiilor DocMED și schemele oficiale rămân în documentația locală.

## Modificări în aplicație

- Implicit SIUI/eFactura/OCSP: `siui.e-pias.ro`; SIPE: `sipe.e-pias.ro`; HTTPS/443.
- Acceptă explicit și mediul de test SIPE `testsipe.siui.ro`.
- `modules.siui.docmed_base_url`, implicit `https://docmed.e-pias.ro`, disponibil în configurarea inițială, Setări CNAS, GET/PUT settings și profilurile de certificat. PUT fără acest câmp păstrează valoarea curentă.
- GET settings include `docmed_endpoints`; diagnosticul verifică și cele patru destinații cu HEAD fără credențiale. Cele 22 de operații SOAP DocMED sunt implementate în grupul separat `docmed`, cu contractele document/literal proprii.
- Actualizare WSDL SIUI/SIPE încorporate; 41 operații SIUI/SIPE și 22 DocMED în catalogul SOAP și SDK. Noua metodă `clinice.getPrescriptionsSeriesNo` are validare XSD de cerere și schemă de răspuns pentru procesarea fișierelor.
- Erorile salvării sunt vizibile lângă buton și identifică parametrul respins.

## Configurații existente

Încărcarea configurației păstrează valorile explicite, inclusiv porturi personalizate și mediul de test. Nu migrează automat toate instalările sau toate certificatele. Pentru trecerea unei instalări vechi în producție, actualizează separat SIUI, PE și eFactura din Setări CNAS și salvează profilul certificatului; DocMED are origine separată. Nu modifica opțiunile de verificare TLS sau credențialele pentru a schimba adresele.

Configurația locală `readersv3/output/siui-bridge/deployments/config.yaml` a fost actualizată doar pentru vechile origini de producție fără port explicit sau cu 443, inclusiv în profiluri, cu backup. Orice port explicit diferit de 443, origine personalizată sau mediu de test rămâne intact. Nu s-a repornit procesul din debugger și nu s-au efectuat autentificări sau operații medicale reale.

Implementarea completă DocMED și pașii de verificare a documentelor vechi sunt descrise în [API DocMED](../readersv3/apps/siui-bridge/docs/DocMED.md). DocMED nu oferă o listare generală; citirile documentelor folosesc identificatori cunoscuți.
