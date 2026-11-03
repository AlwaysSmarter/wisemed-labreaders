# Independent DocMED JAXB contract check

Requires Java 17+ and Maven. It generates Java classes in package `ro.cnas.generated`
from the published GenDocumentSeriesRequest and CommonDocumentTypesPias schemas,
then deserializes a document/literal SOAP payload using JAXB 4.0.5. This is independent
of the Go serializer. It does not call CNAS or use certificates, PINs or licences.
Maven may download the pinned public JAXB dependencies from its configured registry.

Run from the repository root:

```sh
python3 readersv3/apps/siui-bridge/tests/docmed-contract-oracle/verify.py
```

Expected output:

```text
Java JAXB documentType=PE
Java JAXB documentCount=2500
```

An optional first argument is a local XML request/SOAP file. The XML stays local;
only the decoded document type and count are printed. The check expects type PE.
Generated sources and class files are removed when it exits. The fixture contains
synthetic provider data; no runtime configuration is loaded.

This proves the mapping defined by the published contract. It does not establish
which generated classes, JAXB version, filters or request mutations the deployed
CNAS service actually uses.
