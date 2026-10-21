# Audit: trimiterea automată a rezultatelor în buletin

Data: 16 septembrie 2026. Cod verificat: checkout local, HEAD `67db1b7`.

## Concluzie și domeniu

Au fost verificate toate cele 22 de aplicații reader din `readersv3/apps`, prin entrypoint, protocolul activ, recepția rezultatelor, setări și apelurile către WiseMED. Celelalte aplicații sunt utilitare/servere, nu readere de rezultate. Codul istoric din `old-src` și binarele deja instalate la clienți nu sunt incluse în confirmare.

**10 aplicații nu au fluxul automat complet conectat; 12 au implementat apelul sincronizare → trimitere.** Aceasta este o confirmare la nivelul surselor, nu o certificare end-to-end pe aparate și server WiseMED real. Auditul nu modifică implementările, configurațiile sau binarele; problemele de mai jos rămân de remediat.

## Readere fără flux automat complet

| Reader | Constatare |
|---|---|
| Autobio AutoLumo S900 (`autobio-autolumo-s900-reader`) | Recepția ASTM salvează și transformă rezultatele local, fără apelul de sincronizare și trimitere automată. |
| Gemini (`gemini-reader`) | Aceeași lipsă pe traseul ASTM. Modulul suplimentar IR Biotyper este doar un meniu, fără import implementat. |
| Horiba Yumizen H500 (`horiba-yumizen-h500-reader`) | Aceeași lipsă în protocolul ASTM comun. |
| Lifotronic H8 (`lifotronic-h8-reader`) | Aceeași lipsă în protocolul ASTM comun. |
| Snibe Maglumi X3 (`snibe-maglumi-x3`) | Aceeași lipsă în protocolul ASTM comun. |
| bioMérieux miniVIDAS (`biomerieux-minividas-reader`) | `processPacket` se încheie după salvare și transformări locale; nu declanșează fluxul automat WiseMED. |
| Erba Mannheim LAURA (`erba-mannheim-laura-reader`) | `process` salvează panelul și aplică transformări, fără sincronizare/trimitere automată. |
| Horiba ABX Pentra 400 (`horiba-abx-pentra400-reader`) | Recepția marchează rezultatele ca finalizate local, fără declanșarea trimiterii în buletin. |
| Zonci XL1000i (`zonci-xl1000i-reader`) | `saveResults` păstrează rezultatele local, inclusiv cele neidentificate, fără apel de trimitere automată. |
| Generic Test Reader (`generic-test-reader`) | Protocolul generic expune numai metadate HTTP; nu are import efectiv de rezultate și nici trimitere automată. Transportul de fișiere doar configurează directoarele. |

Dovezi: `modules/protocols/astm/module.go:726–858`, `biomerieuxminividas/module.go:170–231`, `erbamannheimlaura/module.go:278–312`, `horibaabxpentra400/module.go:516–557`, `zoncixl1000i/module.go:328–403`, `genericfile/module.go`, `irbiotyper/module.go`, `modules/transports/file/module.go`.

Activarea opțiunii în interfață nu rezolvă aceste lipsuri. Sincronizarea periodică din `result-sync` identifică cereri/analize, dar nu trimite rezultatele în buletin. Host-query pentru programarea probelor nu înlocuiește trimiterea rezultatelor.

## Readere cu fluxul implementat

| Reader | Traseu |
|---|---|
| Analytik Jena PlasmaQuant MS Elite | `fileimportbase` |
| Anatolia Geneworks | `fileimportbase` |
| BEOSL | `fileimportbase` |
| Biosan Hipo MPP-96 | `fileimportbase` |
| CFX96 | `fileimportbase` |
| GammaVision | `fileimportbase` |
| Seegene | `fileimportbase` |
| Shimadzu Generic (`shimatzu-generic-reader`) | `fileimportbase` |
| TriCarb 5110 TR | `fileimportbase` |
| Cary60 UV-VIS | Apel explicit după import: `cary60uvvis/module.go:243` |
| Shimadzu TOC-L (`shimatzu-tocl-reader`) | Apel explicit după import: `shimatzutocl/module.go:242` |
| Labnovation LD-560 | Apel explicit după recepție: `labnovationld560/module.go:905` |

Fluxul comun (`modules/protocols/fileimportbase/module.go:498–534`) verifică `auto_confirm_wisemed`, sincronizează exact ID-urile de cereri colectate pentru data/runda importului prin `RunOrders`, recitește datele locale și trimite rezultatele identificate prin `SaveFileServiceResults`. Trimiterea necesită identificatorul fișei și `WiseMEDFSMID` pentru analiză, plus o valoare de rezultat. Datele deja identificate complet pot fi omise de la un nou lookup de către `result-sync`.

## Configurare și limitări comune

- Opțiunea există în interfața comună și este salvată în `results.auto_confirm_wisemed` și `modules.results.auto_confirm_wisemed`. Configurația normalizată este citită de helperul comun. În șabloanele `config.install.yaml`, BEOSL are `true`; celelalte 21 au `false`. Acestea sunt valori de instalare, nu starea instalațiilor clienților.
- Inclusiv pentru cele 12 implementări, API-ul trebuie configurat, opțiunea activată și probele/analizele identificabile în WiseMED.
- Nu există coadă persistentă sau retry automat al trimiterii. Dacă API-ul nu este disponibil, cererea încă nu există în WiseMED sau sincronizarea este deja în curs, recepția poate rămâne salvată local fără trimitere ulterioară automată. Sincronizarea periodică nu execută PATCH-ul de rezultate.
- `RunOrders` poate returna un sumar cu erori individuale de lookup, fără eroare Go globală. Helperul nu verifică acel sumar; rezultatele fără identificator de analiză sunt omise la trimitere. Un import reușit nu înseamnă că toate rezultatele au ajuns în buletin.
- Helperul recitește toate analizele cererilor selectate, nu doar valorile nou primite; nu filtrează aici după starea anterioară de trimitere. O nouă recepție poate retransmite rezultate existente ale aceleiași cereri.

## Verificare executată

Comanda executată din `readersv3`:

```sh
go test ./modules/protocols/... ./modules/resultsync/... ./core/config/...
```

Rezultat: succes pentru toate pachetele; o parte dintre pachete nu au teste. Testele existente nu acoperă integral fluxul automat pentru fiecare reader. Nu au fost trimise rezultate reale în WiseMED.

## Remediere necesară

Conectarea recepției din ASTM, miniVIDAS, LAURA, Pentra și Zonci la fluxul comun după salvarea locală, cu data/runda/ID-urile cererilor efectiv primite. Generic Test Reader necesită mai întâi un import real. Separat, este necesar un mecanism persistent de reîncercare și teste de integrare pentru identificare, trimitere, opțiune dezactivată, lipsa mapării și indisponibilitatea API-ului.
