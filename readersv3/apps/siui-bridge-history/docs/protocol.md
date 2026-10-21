# Protocol SIUI History

Protocol utilitar: `siui-history`; identitate WSM `siui-bridge-history`, rol `service`.

API și protocol de arhivare: [API.md](API.md).
Configurare, limite operaționale și modelul de stocare: [README](../README.md).

Persistența este eventuală: recepția rezultatului CNAS și confirmarea arhivării sunt două
momente distincte. Coada locală este obligatorie înainte de orice transmitere asincronă.
Checksumul dovedește integritatea fișierului arhivat față de manifest, nu constituie o
semnătură digitală CNAS sau o certificare independentă a timpului serverului CNAS.
