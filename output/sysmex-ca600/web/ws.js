var objLabReaderWS={};
var scrollNowTimer = null;
$.extend(true, objLabReaderWS, $.extend(true,{
    uri : null,
    websocket: null,
    connected: false,

}));


function objLabReaderWSReConnect(uri) {
    if ((objLabReaderWS.websocket!=null) && (objLabReaderWS.websocket.readyState == WebSocket.OPEN)) {
        objLabReaderWS.websocket.close()
    }
    //now connect
    objLabReaderWSConfigWebSocket(uri);
}
//<editor-fold desc="objLabReaderWS Socket Helper Functions">
function objLabReaderWSConfigWebSocket(uri) {
    if (!isVoid(uri)) objLabReaderWS.uri = uri;
    if ((objLabReaderWS.websocket!=null) && (objLabReaderWS.websocket.readyState == WebSocket.OPEN)) {
        //addAnalizerWSLogMessage('warning',"Deja conectat!");
        objLabReaderWS.websocket.close()
        //return;
    }

    try {
        objLabReaderWS.websocket = new WebSocket(objLabReaderWS.uri);
    }
    catch (e) {
        //addAnalizerWSLogMessage("error", e, true);
        return;
    }
    objLabReaderWS.websocket.onopen = function (evt) {
        //addAnalizerWSLogMessage('success',"WMeCARD Manager conectat cu succes");
        $("#wmecardmenu").addClass('wm-ceas-comm-active');
        //objLabReaderSetIcon('ace-fresh-green-color');
    };
    objLabReaderWS.websocket.onclose = function (evt) {
        //addAnalizerWSLogMessage("run", "Connection closed", true);
        //objLabReaderSetIcon('ace-red');
    };
    objLabReaderWS.websocket.onmessage = function (evt) {
        console.log('WS Message');
        console.log(evt);
        setTimeout(function () {
            //objLabReaderSetIcon('ace-fresh-green-color');
        }, 300);

        let respParsed = true;
        let receivedMessages = [];
        try {
            let resp = JSON.parse(evt.data);
            receivedMessages.push(resp);
        } catch (e) {
            console.log('An error occurred on parsing message: ' + evt.data)
            receivedMessagesRaw = evt.data.split("\n");
            $.each(receivedMessagesRaw, function (idx, val) {
                try {
                    console.log('Trying to parse ' + val);
                    var resp = JSON.parse(val);
                    receivedMessages.push(resp);
                } catch (e) {
                    console.log('cannot parse returned message ' + idx);
                    console.log(val);
                    return;
                }
            });
        }
        $.each(receivedMessages, function (idx, resp) {
            if (resp.success) {
                let msgTxt = unicodeLiteral(resp.msg);
                switch (resp.action) {
                    case 'analyzermsg':
                        msgTxt = "(" + resp.msg.charCodeAt(0) + ") " + unicodeLiteral(resp.msg) + " [len:" + resp.msg.length + "]";
                        $('<div class="ace-col-12 ws-analyzer-msg"></div>').appendTo($("#ws-an-communication")).html(msgTxt);
                        break;
                    case 'hostmsg':
                        msgTxt = "(" + resp.msg.charCodeAt(0) + ") " + unicodeLiteral(resp.msg) + " [len:" + resp.msg.length + "]";
                        $('<div class="ace-col-12 ws-host-msg"></div>').appendTo($("#ws-an-communication")).html(msgTxt);
                        break;
                    case 'logaerr':
                        $('<div class="ace-col-12 ws-err-msg"></div>').appendTo($("#ws-an-communication")).html(msgTxt);
                        break;
                    case 'logamsg':
                        $('<div class="ace-col-12 ws-log-msg"></div>').appendTo($("#ws-an-communication")).html(msgTxt);
                        break;
                    case 'newresult':
                        if (!isVoid(resp.msg)) {
                            var msg = JSON.parse(resp.msg);
                            console.log("NEW RESULT FOR:");
                            console.log(msg);
                        }

                        getWMReaderOrders();
                        break;
                }


                if (scrollNowTimer !== null) {
                    clearTimeout(scrollNowTimer)
                }
                scrollNowTimer = setTimeout(function() {
                    $('#ws-an-communication').animate({
                        scrollTop: $('#ws-an-communication').get(0).scrollHeight
                    }, 100);
                }, 500);

            } else {
                var err = resp.error
                if (isVoid(err)) err = _L['wmreaderrundefined'];
                if (isVoid(resp.errcode)) err += ".Code: " + resp.errcode;
                $('<div class="ace-col-12 ws-err-msg"></div>').appendTo($("#ws-an-communication")).html(err);
                //addAnalizerWSLogMessage('error', err);
            }
        });
    };
    objLabReaderWS.websocket.onerror = function (evt) {
        try {
            var resp = JSON.parse(evt.data);
            //addAnalizerWSLogMessage('error','WMeCARD ERROR: ' + resp.error);
        }
        catch (e) {
        }
    };
}

function objLabReaderWSSendActualTextFrame(message) {
    if (objLabReaderWS.websocket.readyState == WebSocket.OPEN) {
        //addAnalizerWSLogMessage("run", "Send message to WMCEASeCard", true);
        //addAnalizerWSLogMessage("run", message, true);
        objLabReaderWS.websocket.send(message);
    }
    else {
        //addAnalizerWSLogMessage('warning',"Reader-ul nu este online. Stare: " + objLabReaderWS.websocket.readyState);
    }
}
function objLabReaderWSSendTextFrame(message) {
    if (objLabReaderWS.websocket.readyState !== WebSocket.OPEN) {
        //addAnalizerWSLogMessage('warning',"Reader-ul nu este online, incerc reconectarea");
        objLabReaderWSReConnect();
        setTimeout(function() {
            objLabReaderWSSendActualTextFrame(message);
        }, 500);
        return;
    }
    objLabReaderWSSendActualTextFrame(message);
}

function objLabReaderWSColoseWebSocket() {
    if (objLabReaderWS.websocket.readyState == WebSocket.OPEN) {
        //addAnalizerWSLogMessage("run", "Closing socket", true);
        objLabReaderWS.websocket.close();
    }
    else {
        //addAnalizerWSLogMessage('warning',"WMeCARD nu este conectat, stare curenta: " + objLabReaderWS.websocket.readyState);
    }
}
//</editor-fold>


function toLowerCase(str) {
    str = str+"";
    return str.toLowerCase();
}

function isVoid(val,veryAsStringToo) {
    if ((veryAsStringToo==='undefined')||(veryAsStringToo===null)) veryAsStringToo=false;
    if (($.type(val) === "undefined") || ($.type(val) === "null") || (veryAsStringToo && val==="")) return true;
    return false;
}
function sleep(delay) {
    var start = new Date().getTime();
    while (new Date().getTime() < start + delay);
}
function sendDataToServer() {
    let lines = $('textarea[name="sim_an_host"]').val().split('\n');
    $.each(lines, function( index, value ) {
        setTimeout(function() {
        //sleep(1000);
        console.log("Sending: "+ value);
        $.post("/communication", {
            'sim_an_host':value,
            'simulate_an':'Send',
            'json':true,
        }, function (data, status, jqXHR) {
            console.log("sent ok");
        });
        }, 100+index*100);
    });
}

/* Creates a uppercase hex number with at least length digits from a given number */
function fixedHex(number, length){
    var str = number.toString(16).toUpperCase();
    while(str.length < length)
        str = "0" + str;
    return str;
}

function returnSpecialCodeChar(chr) {
    return "<a  href='#' title='\\x02' alt='\\x02' style='color:white;background-color:black;'>"+chr+"</a>";
}
/* Creates a unicode literal based on the string */
function unicodeLiteral(str, translateKnownCodes){
    var i;
    var result = "";
    for( i = 0; i < str.length; ++i) {
        /* You should probably replace this by an isASCII test */
        if (str.charCodeAt(i) > 126 || str.charCodeAt(i) < 32) {
            switch (str.charCodeAt(i)) {
                case 2:
                    result += returnSpecialCodeChar("␂");
                    break;
                case 3:
                    result += returnSpecialCodeChar("␃");
                    break;
                case 4:
                    result += returnSpecialCodeChar("␄");
                    break;
                case 5:
                    result += returnSpecialCodeChar("␅");
                    break;
                case 6:
                    result += returnSpecialCodeChar("␆");
                    break;
                case 10:
                    result += returnSpecialCodeChar("␊");
                    break;
                case 13:
                    result += returnSpecialCodeChar("␍");
                    break;
                default:
                    result += "\\x" + fixedHex(str.charCodeAt(i), 2);
                    break;
            }
        }
        else
            result += str[i];
    }

    return result;
}

$(document).ready(function () {
    objLabReaderWSReConnect(ws_type+ws_addr+':'+ws_port+ws_path);
});
