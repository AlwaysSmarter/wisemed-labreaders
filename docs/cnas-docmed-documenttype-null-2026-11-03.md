# DocMED: GenDocumentSeries documentType null — 3 noiembrie 2026

## Simptom observat

Producție: `https://docmed.e-pias.ro/seq/ws`, operația `GenDocumentSeries`.
Cererea din apelul local `8531d05ff858647fdd10c2a5b8757a22` transmite:

```xml
<GenDocumentSeriesRequest xmlns="http://www.cnas.ro/pias/2.0"
    providerCode="123" providerName="EXAMPLE" providerInsuranceHouse="CAS-DJ">
  <genDocumentSeries insuranceHouseCode="CAS-DJ"
      documentCount="2500" documentType="PE" />
</GenDocumentSeriesRequest>
```

Exemplul este anonimizat la nivel de furnizor; structura, namespace-urile,
tipul PE și numărul 2500 sunt cele trimise efectiv. Nu conține credențiale.

Răspunsul upstream este HTTP 500, SOAP 1.1 Fault `SOAP-ENV:Server`:

```text
Cannot invoke "ro.cnas.generated.DocumentType.value()" because the return value of
"ro.cnas.generated.GenDocumentSeriesRequest$GenDocumentSeries.getDocumentType()" is null
```

## Verificări efectuate

1. Cererea exactă păstrată în jurnalul local conține atributul `documentType="PE"`.
2. `xmllint --nonet --schema GenDocumentSeriesRequest.xsd` validează cererea exactă,
   folosind fișierele originale CNAS, fără modificări de schemă.
3. Arhiva XSD descărcată la verificare de pe portalul CNAS este identică, pentru
   GenDocumentSeriesRequest, GetDocumentSeriesRequest și CommonDocumentTypesPias,
   cu documentația locală. Tipul enumerat `documentType` include explicit `PE`.
4. `elementFormDefault="qualified"` califică elementele. `attributeFormDefault`
   nu este setat; atributele rămân necalificate. Adăugarea unui namespace la atributul
   `documentType` ar încălca schema publicată și este respinsă de verificarea XSD.
5. Clase Java JAXB au fost generate din aceleași XSD-uri în pachetul
   `ro.cnas.generated`. Deserializarea cererii efective, în anvelopa SOAP, întoarce:

```text
Java JAXB documentType=PE
Java JAXB documentCount=2500
```

În jurnalul local există și două citiri `GetDocumentSeries` cu PE, finalizate fără
SOAP Fault și fără serii în rezultatul primit. Rezultatele goale nu dovedesc singure
că o generare anterioară nu a alocat nimic; contează filtrele și vizibilitatea.

## Concluzie și limite

Datele disponibile arată că atributul este trimis și că cererea este conformă
contractului publicat. O implementare JAXB generată din acel contract citește PE
corect. Discrepanța este în interpretarea/contractul efectiv al serviciului CNAS
sau în prelucrarea cererii pe traseul lui; cauza exactă din implementarea serverului
nu este accesibilă din client.

CNAS trebuie să verifice versiunea și adnotările JAXB pentru
GenDocumentSeriesRequest.GenDocumentSeries.documentType, enumerația DocumentType
și eventualele transformări înaintea handlerului. Nu există dovadă suficientă
pentru a afirma dacă problema este exclusiv PE, namespace-ul atributelor sau altă
prelucrare. Nu se schimbă automat forma XML și nu se elimină PE din lista publicată.

Scrierile afectate rămân `unknown`; nu au fost retrimise în timpul investigației.
Nu s-au făcut apeluri autentificate noi către CNAS. Descărcările de documentație au
fost publice; WSDL-ul live nu a putut fi citit: conexiunea TLS publică a fost respinsă.

## Reproducere locală

[Verificarea JAXB independentă](../readersv3/apps/siui-bridge/tests/docmed-contract-oracle/README.md)
este disponibilă în repository. Testul Go
`TestDocMedPESeriesMatchesPublishedNamespaceContract` verifică și forma SOAP,
valoarea PE, numărul 2500 și namespace-ul atributului.

SHA-256 ale fișierelor publicate folosite:

- GenDocumentSeriesRequest.xsd: `f34cf9c728163c51ba104b4f83fa589821dce34e7095bd7aa7251265a18ccdc0`
- CommonDocumentTypesPias.xsd: `64e1278ec5fb6f9635e364f7694bf6cb2a733e758c59e8f298599f41777adc3b`

Surse oficiale:

- [Pagina DocMED CNAS](https://portal.cnas.ro/cnas/pias/docmed)
- [Arhiva XSD DocMED](https://portal.cnas.ro/cnas/pias/docs/specificatii-docmed/StructuriDocMed.zip)
- [Arhiva WSDL DocMED](https://portal.cnas.ro/cnas/pias/docs/specificatii-docmed/DefinitiiServiciiDocMed.zip)
