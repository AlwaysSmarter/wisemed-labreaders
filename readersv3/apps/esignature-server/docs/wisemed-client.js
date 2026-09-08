if ($.aceOverWatch.utilities.isVoid(wmESIG)) { var wmESIG={};}
let eSigCookies = getESIGCookieSettings();
$.extend(true, wmESIG, $.extend(true,{
    uri : eSigCookies.esiguri,
    bcpuri : eSigCookies.bcpuri,
    websocket: null,
    sigRequestSender: null,
    reconnectTries :0,
    messageOnConnect : null,

    saveSignatureToDatabase : function (sig_response, s_image_b64, s_sig_txt) {

        if(
                wmESIG.sigRequestSender.sig_type != sig_response.sig_type
            ||  wmESIG.sigRequestSender.pacient_id != sig_response.pacient_id
        ){
            console.log('WMeESIG: Signature request mismatch.');
            onsole.log('Expected type / entity id: '+wmESIG.sigRequestSender.sig_type+' / '+wmESIG.sigRequestSender.pacient_id);
            console.log('Received type / entity id: '+sig_response.sig_type+' / '+sig_response.pacient_id);
            return;
        }

        $.aceOverWatch.toast.show('success','Semnatura se salveaza in baza de date!');

        if( !this.saveHelper ){

            this.saveHelper = $('<div></div>').ace('create',{
                type:'hidden',
                net : {
                    remote : true,
                    autoload : false,
                    fid  : 3361,
                }
            });
        }

        $.aceOverWatch.net.save(this.saveHelper,{
            _s_tip              : sig_response.sig_type,
            _s_entity_id        : sig_response.pacient_id,
            _s_imagine_b64      : s_image_b64,
            _s_sir_verificare   : s_sig_txt,
        },{
            onsuccess : function(field, data){
                wmESIG.onSaveSignatureSuccess(data.data);
            },
        },null,
        {
            type : 'POST',
        });
    },
    onSaveSignatureSuccess : function(data){

        wmESIGShowPatientSignature(wmESIG.sigRequestSenderObj, data._s_tip, $.aceOverWatch.record.create(data));

        $.aceOverWatch.toast.show('success','Semnatura a fost salvata cu success!');
    }

}));

function getWMESIGColumns() {
    if ((wmESIG.websocket!==null)&&(wmESIG.websocket.readyState === WebSocket.OPEN)) {
        return [
                {
                    label: 'Deconectare',
                    iconcls: 'fad fa-wifi',
                    action: 'wmESIGColoseWebSocket',
                },
                {
                    label: 'Log comunicatie',
                    iconcls: 'fad fa-file-alt',
                    action: 'showCreateWMESIGLogForm',
                },
                {
                    label: 'Download',
                    iconcls: 'fad fa-download',
                    action: 'wmESIGDownloadManager',
                },
                {
                    label: 'URI WMeESIG',
                    iconcls: 'fad fa-ethernet',
                    action: 'wmesigURI',
                }
            ];
    }
    else {
        return [
                {
                    label: 'Conectare',
                    iconcls: 'fad fa-wifi',
                    action: 'wmESIGConfigWebSocket',
                },
                {
                    label: 'Log comunicatie',
                    iconcls: 'fad fa-file-alt',
                    action: 'showCreateWMESIGLogForm',
                },
                {
                    label: 'Download',
                    iconcls: 'fad fa-download',
                    action: 'wmESIGDownloadManager',
                },
                {
                    label: 'URI WMeESIG',
                    iconcls: 'fad fa-ethernet',
                    action: 'wmesigURI',
                }
            ];
    }
}

function wmESIGConfigWebSocket() {
    if( window['disableWebSocketsInitiation'] === true ){ console.log('web socket functionality disabled for this page'); return; }
    if ((wmESIG.websocket!=null) && (wmESIG.websocket.readyState == WebSocket.OPEN)) {
        addESIGLogMessage('warning',"Deja conectat!");
        return;
    }

    try {
        wmESIG.websocket = new WebSocket(wmESIG.uri);
        wmESIG.websocket.onopen = function (evt) {
            wmESIG.reconnectTries = 0;
            addESIGLogMessage('success', "WMeESIG Manager conectat cu succes");
            $("#wmesigmenu").addClass('wm-esig-active-nocom');
            var initObj = {
                cmd: "init"
            }
            wmESIGSendTextFrame(JSON.stringify(initObj));
            $("#wmesigmenu").ace('modify', {
                items: getWMESIGColumns()
            });

            if (!$.aceOverWatch.utilities.isVoid(wmESIG.messageOnConnect)) {
                var msg = wmESIG.messageOnConnect;
                wmESIG.messageOnConnect = null;
                wmESIGSendTextFrame(msg);
            }
        };
        wmESIG.websocket.onclose = function (evt) {
            $("#wmesigmenu").removeClass('wm-esig-active-nocom');
            $("#wmesigmenu").addClass('wm-esig-active-com');
            setTimeout(function () {
                $("#wmesigmenu").removeClass('wm-esig-active-com');
                $("#wmesigmenu").removeClass('wm-esig-active-nocom');
                wmESIG.umState = 'umunknown';
                $("#wmesigmenu").ace('modify', {
                    items: getWMESIGColumns()
                });
                addESIGLogMessage("run", "Connection closed", true);
            }, 300);
        };
        wmESIG.websocket.onmessage = function (evt) {
            $("#wmesigmenu").removeClass('wm-esig-active-nocom');
            $("#wmesigmenu").addClass('wm-esig-active-com');
            try {
                var resp = JSON.parse(evt.data);
                addESIGLogMessage("run", "New message: ", true);
                addESIGLogMessage("run", evt.data, true);
                if (resp.success) {
                    switch (resp.forevent.cmd) {
                        case "signpatient":
                            wmESIG.saveSignatureToDatabase(resp.forevent, resp.data.sigbase64, resp.data.sigenc);
                            break;
                    }
                } else {
                    addESIGLogMessage('error', resp.error);
                }
            } catch (e) {
                addESIGLogMessage("error", e, true);
            }
            setTimeout(function () {
                $("#wmesigmenu").removeClass('wm-esig-active-com');
                $("#wmesigmenu").addClass('wm-esig-active-nocom');
            }, 300);
        };
        wmESIG.websocket.onerror = function (evt) {
            addESIGLogMessage('error', 'WMeESIG ERROR: ' + evt.data);
        };
    }
    catch (e) {
        console.log('wmESIG error: ');
        console.log(e);
    }
}

function wmESIGSendTextFrame(message) {
    if (wmESIG.websocket.readyState == WebSocket.OPEN) {
        addESIGLogMessage("run", "Send message to WMESIGeCard", true);
        addESIGLogMessage("run", message, true);
        wmESIG.websocket.send(message);
        return true;
    }
    else {
        if (wmESIG.reconnectTries == 1) {
            addESIGLogMessage('warning', "Serverul nu este conectat.<br>Puteti sa il <a href='" + app_base + '/downloads/esig/respWMeESIGManager.exe' + "' target='_blank' title='Download WMeSGNATURE Manager'>download-ati aici.</a>");
        }
        else {
            wmESIG.reconnectTries++;
            wmESIG.messageOnConnect = message;
            wmESIGConfigWebSocket();
        }

    }
    return false;
}

function wmESIGColoseWebSocket() {
    if (wmESIG.websocket.readyState == WebSocket.OPEN) {
        addESIGLogMessage("run", "Closing socket", true);
        wmESIG.websocket.close();
    }
    else {
        addESIGLogMessage('warning',"WMeESIG nu este conectat, stare curenta: " + wmESIG.websocket.readyState);
    }
}

function wmESIGDownloadManager() {
    window.open(app_base+'/downloads/esig/' +
        'respWMeESIGManager.exe', '_blank');
}

$(document).ready(function () {
    $("#wmesigmenu").ace('create', {
        type:'menubutton',
        items: getWMESIGColumns()
    });
    wmESIGConfigWebSocket();
});

function returnWMESIGLLogForm() {
    if (!wmESIG.createESIGLogFormCreated) {

        $("<div></div>", {
            id : 'esig-log-window'
        }).addClass('ace-hide').appendTo($('body'));
        wmESIG.createESIGLogForm = $("#esig-log-window");

        wmESIG.createESIGLogForm.ace('create', {
            type: 'form',

            ftype: 'popup',
            displaycancelbtn: true,

            template: 'esig-log-window-tpl',
            renderto: 'esig-log-window',
            displaysavebtn: false,
            autoloadfieldsonshow: false,
            checkdirtyoncancel: true,
            hideonescape: true,

            onshow: function (form) {
                $('html').addClass('ace-no-scrolling');
                $('body').addClass('ace-no-scrolling');
                $(form).removeClass($.aceOverWatch.classes.hide);
            },
            customhide: function (form) {
                $('body').removeClass('ace-no-scrolling');
                $('html').removeClass('ace-no-scrolling');
                $(form).addClass($.aceOverWatch.classes.hide);
                $(form).removeClass($.aceOverWatch.classes.formShow);
            },
        });

        wmESIG.createESIGLogFormCreated = true;

    }
    return wmESIG.createESIGLogForm;
}

function showCreateWMESIGLogForm() {
    returnWMESIGLLogForm();
    wmESIG.createESIGLogForm.ace('show');
}

function clearESIGLogMessage(messageType, message) {
    wmESIG.createESIGLogForm.find('.log-sheet ').html('');
}

function addESIGLogMessage(messageType, message, surpress) {
    returnWMESIGLLogForm();
    var d = new Date();
    var msg = '<span class="ace-black">' + d.getDate() + '/' + d.getMonth() + '/' + d.getFullYear() + ' ' + d.getHours() + ':' + d.getMinutes() + ':' + d.getSeconds() + '</span><br>';
    msg += '<span class="';
    switch (messageType) {
        case "warning" : msg += 'ace-orange';
            break;
        case "error" : msg += 'ace-red';
            break;
        default : msg += 'ace-green';
            break;
    }
    msg += '">' + message + '</span>';

    var msgDiv = $("<div></div>", {
        html: msg
    }).appendTo(wmESIG.createESIGLogForm.find('.log-sheet '));

    if (!surpress) $.aceOverWatch.toast.show(messageType,message);
}

function wmESIGSign(sender, sig_type, entity_id, entity_name, patient_cnp) {
    var initObj = {
        cmd : "signpatient",
        sig_type : sig_type,
        pacient_id: entity_id,
        nume_pacient: entity_name,
        cnp_pacient: patient_cnp,
    }
    wmESIG.sigRequestSender = initObj;
    wmESIG.sigRequestSenderObj = sender;
    if( wmESIGSendTextFrame(JSON.stringify(initObj)) ){
        $.aceOverWatch.toast.show('success','Acum se poate semna!');
    }

}

function wmESIGShowPatientSignature(sender, sig_type, patientRec) {
    if ((!$.aceOverWatch.utilities.isVoid(patientRec)) && (patientRec.val('_s_imagine_b64') != "")) {
        sender.find('.s-imagine-b64-src-'+sig_type).css('background-image','url("data:image/png;base64,'+patientRec.val('_s_imagine_b64')+'")');
        sender.find('.s-imagine-b64-src-'+sig_type).removeClass('ace-hide');
        sender.find('.gdpr-'+sig_type).removeClass('gdpr-gray');
    }
    else {
        sender.find('.s-imagine-b64-src-'+sig_type).css('background-image','');
        sender.find('.s-imagine-b64-src-'+sig_type).addClass('ace-hide');
        sender.find('.gdpr-'+sig_type).addClass('gdpr-gray');

    }
}



//<editor-fold desc="wmESIG URI helper functions">
function returnWMESIGURIForm() {
    if (!wmESIG.createESIGURIForm) {

        $("<div></div>", {
            id : 'esig-uri-window'
        }).addClass('ace-hide').appendTo($('body'));
        wmESIG.createESIGURIForm = $("#esig-uri-window");

        wmESIG.createESIGURIForm.ace('create', {
            type: 'form',

            ftype: 'popup',
            displaycancelbtn: true,

            template: 'esig-uri-window-tpl',
            renderto: 'esig-uri-window',
            displaysavebtn: false,
            autoloadfieldsonshow: false,
            checkdirtyoncancel: true,
            hideonescape: true,

            onshow: function (form) {
                $('html').addClass('ace-no-scrolling');
                $('body').addClass('ace-no-scrolling');
                $(form).removeClass($.aceOverWatch.classes.hide);

                var cs = getESIGCookieSettings();
                wmESIG.createESIGURIForm.find('[fieldname="wmesigURI"]').ace('value', cs.esiguri);
                wmESIG.createESIGURIForm.find('[fieldname="wmbcprintURI"]').ace('value', cs.bcpuri);
            },
            customhide: function (form) {
                $('body').removeClass('ace-no-scrolling');
                $('html').removeClass('ace-no-scrolling ');
                $(form).addClass($.aceOverWatch.classes.hide);
                $(form).removeClass($.aceOverWatch.classes.formShow);
            },
        });
    }
    return wmESIG.createESIGURIForm;
}

function wmesigURI() {
    returnWMESIGURIForm().ace('show');
}

function wmSetESIGURI() {
    wmESIGColoseWebSocket();
    wmESIG.uri =  wmESIG.createESIGURIForm.find('[fieldname="wmesigURI"]').ace('value');
    wmESIG.bcpuri =  wmESIG.createESIGURIForm.find('[fieldname="wmbcprintURI"]').ace('value');

    var cs = getESIGCookieSettings();
    if ($.aceOverWatch.utilities.isVoid(cs)) cs = {};
    cs.esiguri = wmESIG.uri;
    cs.bcpuri = wmESIG.bcpuri;
    setESIGCookieSettings(cs);

    returnWMESIGURIForm().ace('hide');

    wmESIGConfigWebSocket();
}

//</editor-fold>

//<editor-fold desc="wmESIG COOKIES helper functions">
function setESIGCookieSettings(ds) {
    setCookie('wm-esig-'+app_user_id, JSON.stringify(ds));
}
function getESIGCookieSettings() {
    if (app_clear_cookies === '1') return {};
    var settings = getCookie('wm-esig-'+app_user_id);

    var shouldSave = false;
    if (($.aceOverWatch.utilities.isVoid(settings)) || (settings === "")) {
        settings = {};
    }else{
        try {
            settings = JSON.parse(settings);
        } catch (err) {
            settings = {};
        }

    }

    if ($.aceOverWatch.utilities.isVoid(settings.esiguri, true)) {
        settings['esiguri'] = app_ws_esignature_url;
        shouldSave = true;
    }
    if ($.aceOverWatch.utilities.isVoid(settings.bcpuri, true)) {
        settings['bcpuri'] = app_ws_bcprinter_url;
        shouldSave = true;
    }

    if (shouldSave) {
        setESIGCookieSettings(settings);
    }
    return settings;
}
//</editor-fold>