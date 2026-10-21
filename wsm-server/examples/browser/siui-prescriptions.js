/* Include after siui-bridge.js. Requires jQuery/aceOverWatch.
 * Callback: (responseReceived, rawValues, message, call).
 * responseReceived=true means CNAS responded; inspect rawValues for business errors.
 * rawValues are the CNAS download URL and size, not individual carnets.
 * call.download.data contains the downloaded bytes by default.
 */
const SIUIPrescriptions = {
    getPrescriptionSeriesSIUI: async function(target, parameters, doneCallback) {
        if (!$.aceOverWatch.utilities.isVoid(target)) {
            target.find('i').addClass('fa-spin');
        }

        try {
            const result = await objWSMSIUI.general.getPrescriptionSeriesInfo(parameters, {
                operationId: crypto.randomUUID()
            });
            if (!result.success) {
                const errorMsg = result.error || 'Nu s-au putut prelua carnetele de rețete.';
                $.aceOverWatch.utilities.runIt(doneCallback, false, [], errorMsg, null);
                $.aceOverWatch.toast.show('error', errorMsg);
                return;
            }

            const call = result.data;
            const values = call && call.result && call.result.values;
            if (!Array.isArray(values)) {
                const errorMsg = 'Răspuns SIUI invalid: lipsește lista de valori returnate.';
                $.aceOverWatch.utilities.runIt(doneCallback, false, [], errorMsg, call);
                $.aceOverWatch.toast.show('error', errorMsg);
                return;
            }

            // Interpret these strings according to the actual CNAS response.
            // For these methods values contain URL + size; use call.download.data
            // for the downloaded response file and parse its CNAS business result.
            $.aceOverWatch.utilities.runIt(doneCallback, true, values, '', call);
        } finally {
            if (!$.aceOverWatch.utilities.isVoid(target)) {
                target.find('i').removeClass('fa-spin');
            }
        }
    },

    // Create operationId once per generation intention and retain it with the
    // exact parameters. A timeout/unknown result does not authorize another ID.
    generatePrescriptionSeriesSIUI: async function(target, parameters, operationId, doneCallback) {
        if ($.aceOverWatch.utilities.isVoid(operationId, true)) {
            const errorMsg = 'Lipsește ID-ul stabil al operației de generare.';
            $.aceOverWatch.utilities.runIt(doneCallback, false, [], errorMsg, null);
            $.aceOverWatch.toast.show('error', errorMsg);
            return;
        }
        if (!$.aceOverWatch.utilities.isVoid(target)) {
            target.find('i').addClass('fa-spin');
        }

        try {
            const result = await objWSMSIUI.general.generatePrescriptionSeries(parameters, {
                operationId: operationId
            });
            if (!result.success) {
                const errorMsg = result.error || 'Nu s-au putut genera carnetele de rețete.';
                $.aceOverWatch.utilities.runIt(doneCallback, false, [], errorMsg, null);
                $.aceOverWatch.toast.show('error', errorMsg);
                return;
            }

            const call = result.data;
            const values = call && call.result && call.result.values;
            if (!Array.isArray(values)) {
                const errorMsg = 'Răspuns SIUI invalid: lipsește lista de valori returnate.';
                $.aceOverWatch.utilities.runIt(doneCallback, false, [], errorMsg, call);
                $.aceOverWatch.toast.show('error', errorMsg);
                return;
            }

            // The caller checks the CNAS receipt before showing business success.
            $.aceOverWatch.utilities.runIt(doneCallback, true, values, '', call);
        } finally {
            if (!$.aceOverWatch.utilities.isVoid(target)) {
                target.find('i').removeClass('fa-spin');
            }
        }
    }
};
