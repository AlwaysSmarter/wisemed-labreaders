# Core Functionality

Acest director conține funcționalitatea de bază comună pentru readerele `readersv3`.

Nu este un strat de compatibilitate temporar. Este nucleul funcțional actual care trebuie să ofere același comportament ca în `readerslast`, doar reorganizat modular.

Include:

- modelele comune pentru API, UI și storage
- contractele comune pentru serviciile folosite de interfața locală
- baza pentru migrarea serverului web, storage-ului SQLite și a serviciilor de business

Ordinea de migrare:

1. `modules/core/model`
2. `modules/core/contracts`
3. server web local compatibil
4. storage SQLite compatibil
5. servicii comune pentru orders, analytes, QC, stats
6. protocoale specifice
