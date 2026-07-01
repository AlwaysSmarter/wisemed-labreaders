# signotec / Evolis SDK Notes

Surse folosite:
- `developer-guide_signopad-api-java-documentation_ENG_20260318_C.pdf`
- `developer-guide_signopad-api_linux-documentation_ENG_20260318_E.pdf`
- `developer-guide_signaturepad-api-documentation_ENG_20260318_E.pdf`

## Concluzii de integrare

- Pentru Java exista doua cai:
  - `SigPadPureFacade`
    - pentru dispozitive Evolis expuse prin `TCP/IP`
    - fara librarii native
    - cross-platform
    - functionalitate limitata fata de facade-ul complet
  - `SigPadFacade`
    - necesita `stpad-lib.jar`
    - incarca librarii native prin `JNA`
    - pe Linux mai cere `OpenSSL libssl 3.0`
    - pentru `HID` / `WinUSB` mai cere `libusb >= 1.0.16`

- Pentru Java 8+ este necesar `crypto.policy=unlimited`; documentatia spune ca SDK-ul incearca sa seteze asta automat cat mai devreme.

- Pentru `EVOLIS Sig200` memoria grafica volatila este:
  - foreground `640 x 960`
  - background `640 x 960`
  - overlay `640 x 480`

- Pentru `EVOLIS Sig200` documentatia recomanda:
  - compunerea imaginilor in `background`
  - apoi mutarea in `foreground`
  - afisarea directa poate fi vizibil lenta

## Flux low-level confirmat de documentatie

Fluxul de baza, confirmat de documentatia Windows/Linux:

1. `DeviceSetComPort(...)`
2. `DeviceGetCount()`
3. `DeviceOpen(index, eraseDisplay)`
4. optional `DisplaySetFont(...)`, `DisplaySetFontColor(...)`
5. optional `SensorSetSignRect(left, top, width, height)`
6. `SignatureStart()`
7. la final:
   - `SignatureConfirm()` pentru accept
   - `SignatureRetry()` pentru rescriere
   - `SignatureCancel()` pentru anulare
8. export imagine:
   - `SignatureSaveAsFileEx(...)`
   - sau stream / SignData, in functie de SDK-ul concret

## Evenimente utile

Documentatia low-level expune explicit:
- `DeviceDisconnected`
- `SignatureDataReceived`
- `SensorHotSpotPressed`
- `SensorTimeoutOccured`
- `DisplayScrollPosChanged`

Acestea sunt utile pentru:
- clipire LED / stare activa
- timeout de captură
- butoane Accept / Retry / Cancel desenate pe ecranul device-ului
- telemetrie si loguri detaliate

## Ce lipseste pentru bridge-ul Java real

PDF-ul Java este introductiv. Pentru implementarea concreta in helper-ul Java mai trebuie:
- pachetul SDK complet
  - `stpad-lib.jar`
  - dependintele jar
  - librariile native per OS, daca folosim `SigPadFacade`
- `doc/javadoc/index.html` din kit
- ideal un sample vendor pentru:
  - open device
  - afisare text
  - start capture
  - accept / retry / cancel
  - export PNG sau bytes

## Directie recomandata in WiseMED

Etapa 1:
- `sdk_mode = pure-tcpip`
- helper Java cu `SigPadPureFacade`
- semnatura returnata in `base64 PNG`

Etapa 2:
- mod optional `sdk_mode = native`
- helper Java cu `SigPadFacade`
- suport USB / HID / WinUSB

## Observatii de produs

- Pentru `Evolis Sig200` prin TCP/IP, `SigPadPureFacade` este cea mai buna varianta initiala pentru cross-platform.
- Daca avem nevoie de toate functiile hardware avansate, trebuie trecut pe `SigPadFacade` si bundle cu native libs.
