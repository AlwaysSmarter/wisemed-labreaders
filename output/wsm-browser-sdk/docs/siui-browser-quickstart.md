# SIUI din browser — ghid practic pentru objWSMSIUI

Acesta este API-ul recomandat. Nu folosi SIUIBridge.create sau await objWSM.ready.
Include siui-bridge.js în pagina unde aplicația are deja obiectul objWSM:

~~~html
<script src="https://wslocal.wisemed.eu/sdk/siui-bridge.js"></script>
~~~

Scriptul detectează objWSM, inclusiv variabila globală declarată cu let/const,
și publică singletonul objWSMSIUI. Conexiunea este verificată la fiecare apel.
Dacă este căzută, sunt încercate implicit 3 reconectări, de maximum 5 secunde fiecare.
Dacă objWSM lipsește, primești o eroare ca rezultat; wrapperul nu inventează URL-ul
și acreditările WSM. Dacă există mai multe utilitare, alege primul siui-bridge
din lista WSM și păstrează identitatea lui până la reselecție explicită.

**Denumirea callback-ului este callwmapi, nu callwsapi.** El cere SIUI Bridge să
apeleze API-ul WiseMED după rezultat. Transportul browser → bridge rămâne WSM.

## 1. Rezultate și moduri de apelare

~~~js
async function afiseazaStatusBridge() {
  const response = await objWSMSIUI.getStatus();

  if (!response.success) {
    afiseazaEroare(response.error); // text
    return;
  }

  afiseazaStatus(response.data);
}
~~~

Funcțiile afiseazaEroare/afiseazaStatus și cele similare din exemple sunt funcțiile
UI ale aplicației tale. Formatul comun este:

~~~js
// Succes:
({ success: true, data: /* rezultatul endpointului sau call */ {} });

// Eroare:
({
  success: false,
  error: "SIUI Bridge selectat nu este conectat.",
  code: "bridge_offline"
});
~~~

Pot exista și status (HTTP), operationId, callId sau attempts. Nu sunt respinse
Promise-urile pentru erorile de execuție ale singletonului. Erorile de sintaxă din
codul paginii apar înaintea apelului și nu pot fi capturate de wrapper.

Se poate folosi și then:
~~~js
objWSMSIUI.getStatus().then(response => {
  if (response.success) {
    afiseazaStatus(response.data);
  } else {
    afiseazaEroare(response.error);
  }
});
~~~

Pentru operațiile CNAS sunt echivalente:
~~~js
// În interiorul unei funcții async, alege UNA dintre forme:
await objWSMSIUI.general.getInsured(parameters, options);
await objWSMSIUI.getInsured(parameters, options);
await objWSMSIUI.executeOperation("general", "getInsured", parameters, options);
~~~

Nu executa toate trei pentru aceeași intenție. Datele WSDL se transmit ca string.
operationId se generează o singură dată pentru intenția de lucru și se păstrează,
împreună cu parametrii și datele callback-ului, pentru reconciliere/retry.

## 2. Status intermediar și progress bar

~~~html
<span id="siui-status"></span>
<progress id="siui-progress" max="100"></progress>
~~~

~~~js
function afiseazaProgresSIUI(event) {
  const label = document.getElementById("siui-status");
  const progress = document.getElementById("siui-progress");

  label.textContent = event.message;

  if (event.percent === null) {
    progress.removeAttribute("value");
  } else {
    progress.value = event.percent;
  }
}

// Opțional, pentru toate apelurile din această pagină:
objWSMSIUI.setStatusHandler(afiseazaProgresSIUI);

// Pentru eliminarea handlerului global:
// objWSMSIUI.setStatusHandler(null);
~~~

Pentru un singur apel folosește options.onStatus. Dacă înregistrezi ambele
callback-uri, ambele primesc evenimente. Pentru operații simultane, folosește
event.requestId pentru câte un indicator separat.

Etapele sunt starting, connecting, discovering, requesting, waiting, downloading,
processing, completed, error și aborted. percent este procentul etapei, nu al
întregii operații. Așteptarea CNAS are procent nedeterminat. Descărcarea originalului
poate avea procent calculat din octeți; pentru fișierul transformat totalul poate
lipsi. completed confirmă apelul wrapperului, nu livrarea callback-ului WiseMED.

## 3. Verificare asigurat, cu update pacient prin callwmapi

Creează cererea o singură dată; păstrează acest obiect dacă trebuie reconciliat:
~~~js
function pregatesteVerificareAsigurat(cnp, loginToken) {
  const requestDate = new Date().toISOString();

  return {
    parameters: {
      pid: String(cnp),
      requestDate: requestDate
    },
    options: {
      operationId: crypto.randomUUID(),
      callwmapi: {
        lt: loginToken,
        apipath: "/pacient/{cnp_pacient}",
        method: "POST",
        transform_path: [
          { from_field: "cnp", to_field: "cnp_pacient" }
        ],
        body: {
          verificare_la_data: requestDate,
          transform_result: [
            { from_field: "pid", to_field: "cnp_pacient" },
            { from_field: "cid", to_field: "cid_pacient" },
            { from_field: "state", to_field: "stare_asigurare_cas" },
            { from_field: "categories", to_field: "categorii_cas" },
            { from_field: "states", to_field: "stari_cas" }
          ]
        }
      }
    }
  };
}

async function verificaAsigurat(cerere) {
  const response = await objWSMSIUI.general.getInsured(
    cerere.parameters,
    {
      ...cerere.options,
      onStatus: afiseazaProgresSIUI
    }
  );

  if (!response.success) {
    afiseazaEroare(response.error);
    return response;
  }

  const call = response.data;

  if (call.result.parse_error) {
    afiseazaEroare(call.result.parse_error);
    return response;
  }

  afiseazaPacient(call.result.insured);
  return response;
}
~~~

Browserul primește PID/CID și state ca string, plus listele categories/states.
Callback-ul trimite numeric stare_asigurare_cas. Valorile SIUI uzuale sunt
0=neasigurat, 1=asigurat, 3=decedat. Nu interpreta orice număr nenul ca asigurat.

lt este opțional și este tokenul de login WiseMED, nu JWT-ul WSM. Pentru apel fără
context de utilizator, omite proprietatea lt. Bridge-ul trimite tokenul primit în
JWT către WiseMED fără login preliminar. WiseMED decide drepturile; 401/403
marchează livrarea failed. Nu se folosește sesiunea globală a altui utilizator.

## 4. Personalizare ZIP/XML → JSON, cu POST /cnas/personalisation

~~~js
function pregatestePersonalizare(codFiscal, loginToken, start, stop) {
  return {
    parameters: {
      partnerCategory: "CLIN",
      start: start,
      stop: stop,
      uic: String(codFiscal)
    },
    options: {
      operationId: crypto.randomUUID(),

      // Procesarea rezultatului care ajunge în browser:
      file: {
        unzip: true,
        format: "json"
      },

      // Procesare și livrare independentă, executată pe SIUI Bridge:
      callwmapi: {
        lt: loginToken,
        apipath: "/cnas/personalisation",
        method: "POST",
        file: {
          unzip: true
        },
        body: {
          cod_fiscal: String(codFiscal),
          partnerCategory: "CLIN",
          transform_result: [
            { from_field: "file_json", to_field: "personalisation" }
          ]
        }
      }
    }
  };
}

async function descarcaPersonalizare(cerere) {
  const response = await objWSMSIUI.general.getProviderInfo(
    cerere.parameters,
    cerere.options
  );

  if (!response.success) {
    afiseazaEroare(response.error);
    return response;
  }

  const call = response.data;

  if (call.download) {
    afiseazaPersonalizare(call.download.data);
  } else {
    afiseazaEroare("CNAS nu a returnat un fișier de personalizare.");
  }

  return response;
}
~~~

XSD este ales de server și validat implicit. Dacă CNAS a publicat un XML mai nou
decât definițiile bridge-ului, poți dezactiva explicit doar XSD, separat pentru
browser și pentru callback:
~~~js
cerere.options.file.validateXSD = false;
cerere.options.callwmapi.file.validateXSD = false;
~~~

Fă această alegere înainte de prima trimitere: modificarea callwmapi pentru același
operationId produce 409. Dacă arhiva are mai multe fișiere, adaugă entry cu numele
exact al XML-ului în ambele obiecte file. JSON-ul este un arbore generic cu name,
namespace, attributes și content; nu pierde repetițiile și nu convertește codurile
în numere. Body-ul WiseMED conține cod_fiscal, partnerCategory și personalisation
(arborele XML transformat). Endpointul este un contract exemplificativ de implementat
în backendul WiseMED.

## 5. Validare la 72h și update /file/{file_id}/

~~~js
function pregatesteValidare72h(fileId, reportXml, loginToken) {
  return {
    parameters: {
      reportXml: reportXml,
      reportType: "PARA",
      requestType: "RQ_PARA_SRV"
    },
    options: {
      operationId: crypto.randomUUID(),
      callwmapi: {
        lt: loginToken,
        apipath: "/file/" + String(fileId) + "/",
        method: "POST",
        body: {
          file_id: String(fileId),
          requested_at: new Date().toISOString(),
          transform_result: [
            {
              from_field: "values.0",
              to_field: "cnas_validation_response_xml"
            }
          ]
        }
      }
    }
  };
}

async function valideaza72h(cerere) {
  const response = await objWSMSIUI.general.validateReport(
    cerere.parameters,
    cerere.options
  );

  if (!response.success) {
    afiseazaEroare(response.error);
    return response;
  }

  afiseazaRaspunsValidare(response.data.result.values[0]);
  return response;
}
~~~

Exemplul este pentru PARA, cu XML real generat de WiseMED, și păstrează cerința
de autorizare a operației CNAS. fileId trebuie să fie un ID valid al fișei (de
exemplu numeric), nu o cale introdusă arbitrar. Pentru CLIN, aplicația furnizează
reportType/requestType corespunzătoare. Nu confunda completed cu „toate serviciile
validate”: backendul WiseMED parsează răspunsul și corelează AppId cu datele salvate.
Nu se setează automat validat=true.

## 6. Confirmarea update-ului WiseMED

Închiderea paginii nu oprește livrarea persistentă callwmapi. Totuși, terminarea
apelului din browser nu garantează că WiseMED a primit deja update-ul:
~~~js
async function verificaLivrare(callId) {
  const response = await objWSMSIUI.getCall(callId);

  if (!response.success) {
    afiseazaEroare(response.error);
    return response;
  }

  afiseazaStareLivrare(response.data.call.callwmapi);
  return response;
}

async function reincearcaLivrare(callId) {
  // Numai după tratarea cauzei și o decizie explicită de retry.
  const response = await objWSMSIUI.retryCallWMAPI(callId);

  if (!response.success) {
    afiseazaEroare(response.error);
  }

  return response;
}
~~~

Stări: waiting, pending, sending, delivered, failed, skipped. delivered înseamnă
HTTP 2xx; WiseMED trebuie să returneze non-2xx la refuz și să deduplice atomic
Idempotency-Key: siui-<call.id>. Retry reia doar livrarea cu body deja pregătit,
nu reexecută CNAS. Transformările eșuate fără body nu pot fi reluate astfel.
Callback-ul este permis utilizatorilor obișnuiți autentificați; drepturile
operației CNAS și verificările WiseMED rămân aplicabile.

## 7. Submit fără așteptare, reluare și anulare locală

~~~js
async function trimiteFaraAsteptare(cerere) {
  return objWSMSIUI.general.getInsured(cerere.parameters, {
    ...cerere.options,
    wait: false
  });
}

async function reiaAsteptarea(callId) {
  const waited = await objWSMSIUI.waitCall(callId);

  if (!waited.success) {
    return waited;
  }

  // Descarcă fișierul, dacă există; file poate cere procesarea lui.
  return objWSMSIUI.completeCall(waited.data);
}

async function verificaCuAnulare(cerere, signal) {
  return objWSMSIUI.general.getInsured(cerere.parameters, {
    ...cerere.options,
    signal: signal
  });
}
~~~

Creează AbortController în pagina apelantă și apelează controller.abort() din
butonul „Oprește așteptarea”. Se oprește numai așteptarea browserului, nu CNAS sau
callwmapi. După timeout, verifică callId/operationId; nu crea automat o nouă
raportare cu alt ID. Metoda submitOperation întoarce în data răspunsul inițial
{ok,call}; metodele numite cu wait:false întorc în data direct call.

## 8. Ajustări opționale

~~~js
objWSMSIUI.setTimeouts({
  connectTimeout: 5000,
  requestTimeout: 135000,
  waitTimeout: 180000,
  downloadTimeout: 180000,
  totalTimeout: 300000
});

objWSMSIUI.setReconnectOptions({
  connectAttempts: 3
});

objWSMSIUI.setPollingInterval(1000);
objWSMSIUI.selectBridge({ equipmentID: "18" });

// Reselectează automat primul bridge:
objWSMSIUI.selectBridge({});

// Alt obiect WSM, doar în această pagină:
objWSMSIUI.setWSM(altObjWSM);
objWSMSIUI.useAutomaticWSM();

// Consultă / resetează opțiunile:
const currentOptions = objWSMSIUI.getOptions();
objWSMSIUI.resetOptions();
~~~

Nu sunt necesare pentru funcționarea implicită. Toate returnează success/error.
Setările afectează numai pagina curentă și apelurile viitoare, fără persistență.
[Referința completă, inventarul operațiilor și API legacy](siui-browser-wrapper.md).


## Fișier JavaScript de copiat
[siui-singleton-examples.js](../examples/browser/siui-singleton-examples.js) conține
funcțiile de mai sus, expuse sub SIUIPageExamples, fără apeluri CNAS automate.
Publică fișierul din aplicația ta după siui-bridge.js; nu este un endpoint /sdk.
Exemplu: SIUIPageExamples.verificaAsigurat(cerere).
Funcțiile UI afiseaza... trebuie definite de aplicație.
