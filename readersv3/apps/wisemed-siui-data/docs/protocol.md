# Protocol și stocare SIUI Data

Contractul HTTP și WSM este descris în [API.md](API.md). Pipeline-ul de import, configurarea și monitorizarea locală sunt descrise în [README](../README.md).

Modelele CLIN/PARA au fost verificate folosind `docs/CNAS/StructuriRaportare/NomenclatoareClinic.xsd`, `NomenclatoareParaclinic.xsd` și arhivele reale `NomenclatoareClinice_20261001.xml.zip`, `NomenclatoareParaclinice_20261001.xml.zip`. Fișierele sursă sunt date CNAS, nu instrucțiuni de runtime. Nu sunt incluse în pachetele aplicației.

Schema PostgreSQL are importuri, pointerul activ per categorie, înregistrări brute versionate și servicii proiectate cu relații. Importul și schimbarea pointerului se confirmă în aceeași tranzacție. Blocările advisory sunt separate pe CLIN/PARA, iar citirile folosesc snapshoturi coerente. Cache distribuit: neimplementat; nu este necesar pentru funcționare.
