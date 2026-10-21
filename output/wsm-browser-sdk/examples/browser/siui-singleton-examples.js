/* Exemple de integrare, fără apeluri automate CNAS.
 * Încarcă după siui-bridge.js. UI: afiseazaEroare, afiseazaStatus, afiseazaPacient,
 * afiseazaPersonalizare, afiseazaRaspunsValidare, afiseazaStareLivrare sunt furnizate
 * de pagina aplicației. Callback-ul canonic este callwmapi, nu callwsapi.
 * Endpointurile WiseMED ilustrate trebuie implementate în backend.
 */
(function (root) {
  "use strict";

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

async function afiseazaStatusBridge() {
  const response = await objWSMSIUI.getStatus();

  if (!response.success) {
    afiseazaEroare(response.error); // text
    return;
  }

  afiseazaStatus(response.data);
}

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

  root.SIUIPageExamples = {
    afiseazaProgresSIUI,
    afiseazaStatusBridge,
    pregatesteVerificareAsigurat,
    verificaAsigurat,
    pregatestePersonalizare,
    descarcaPersonalizare,
    pregatesteValidare72h,
    valideaza72h,
    verificaLivrare,
    reincearcaLivrare,
    trimiteFaraAsteptare,
    reiaAsteptarea,
    verificaCuAnulare
  };
})(globalThis);
