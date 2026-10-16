const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
let JSDOM;try{({JSDOM}=require('jsdom'));}catch{}
const source=fs.readFileSync(__dirname+'/app.js','utf8');
function extract(name){
 const start=source.indexOf('function '+name+'(');
 assert.ok(start>=0);
 return source.slice(source.slice(start-6,start)==='async '?start-6:start,source.indexOf('\n}',start)+2);
}
for(const remote of [false,true])test('WSS settings and WSM diagnostics are separate: remote='+remote,{skip:!JSDOM&&'Install jsdom for DOM tests'},()=>{
 const dom=new JSDOM(fs.readFileSync(__dirname+'/index.html','utf8'));
 const document=dom.window.document,window=dom.window;
 window.WSM_REMOTE=remote;
 const actions=[];
 const scope=vm.createContext({document,window,state:{session:{user_type:0}},
 renderWSSReconnectButtons(){},wssBusy:(button,action)=>action(),wssControl:async action=>actions.push(action),wssDebug(){},
 FormData:window.FormData});
 vm.runInContext(['wssScopeOptions','syncWSSAuthFields','renderWSSPanel'].map(extract).join('\n'),scope);
 scope.renderWSSPanel();
 const settings=document.querySelector('#settings-panel-wss'),debug=document.querySelector('#wsm-debug-panel');
 assert.ok(settings.querySelector('#wss-settings-form'));
 assert.equal(settings.querySelectorAll('[data-wss-action]').length,0);
 assert.equal(settings.querySelector('#wss-local-status'),null);
 assert.equal(settings.querySelector('#wss-trace'),null);
 assert.equal(debug.querySelector('#wss-settings-form'),null);
 for(const id of ['wss-local-status','wss-peers','wss-bridge-form','wss-response','wss-trace']) assert.ok(debug.querySelector('#'+id),id);
 for(const panel of [settings,debug]){
  assert.equal(panel.querySelectorAll('[data-wss-control]').length,2);
  panel.querySelector('[data-wss-control="reconnect"]').click();
  assert.equal(panel.querySelector('[data-wss-control="disconnect"]').hidden,remote);
 }
 assert.deepEqual(actions,['reconnect','reconnect']);
 assert.equal(debug.querySelector('#wss-relay-debug').hidden,remote);
 scope.renderWSSPanel();
 assert.equal(document.querySelectorAll('#wss-settings-form').length,1);
 assert.equal(document.querySelectorAll('#wss-trace').length,1);
 // Label toggles the checkbox; information control does not.
 const checkbox=settings.querySelector('[data-wss-scope]');
 const label=checkbox.closest('label');
 label.click();assert.equal(checkbox.checked,true);
 settings.querySelector('.wss-permission-info summary').click();
 assert.equal(checkbox.checked,true);
});

test('Reconnect loader spans POST, retries and both panels until a new connection or disconnect', {skip:!JSDOM},async()=>{
 const dom=new JSDOM('<button data-wss-control="reconnect">Reconectare</button><button data-wss-control="reconnect">Reconectare</button><button data-wss-control="disconnect">Deconectare</button>');
 const document=dom.window.document;
 const state={wss:{snapshot:{status:{enabled:true,connected:true,connection_id:'old'}}}};
 let finish;let requests=0;
 const scope=vm.createContext({document,window:dom.window,state,
  api:async()=>{requests++;await new Promise(resolve=>{finish=resolve;});},
  loadWSSStatus:async()=>{},wssNotice(){}});
 vm.runInContext(['setButtonLoading','wssReconnectBusy','renderWSSReconnectButtons','updateWSSReconnect','wssControl'].map(extract).join('\n'),scope);
 const buttons=[...document.querySelectorAll('[data-wss-control="reconnect"]')];
 const pending=scope.wssControl('reconnect');
 assert.ok(buttons.every(b=>b.disabled&&b.getAttribute('aria-busy')==='true'));
 assert.equal(document.querySelector('[data-wss-control="disconnect"]').disabled,false);
 await scope.wssControl('reconnect');assert.equal(requests,1);
 finish();await pending;
 scope.updateWSSReconnect(state.wss.snapshot.status); // old connection is not success
 assert.ok(buttons.every(b=>b.disabled));
 state.wss.snapshot.status={enabled:true,connected:false,phase:'disconnected',retry_in_seconds:30};
 scope.updateWSSReconnect(state.wss.snapshot.status);
 assert.ok(buttons.every(b=>b.disabled));
 state.wss.snapshot.status={enabled:true,connected:true,connection_id:'new'};
 scope.updateWSSReconnect(state.wss.snapshot.status);
 assert.ok(buttons.every(b=>!b.disabled&&!b.hasAttribute('aria-busy')));
 const again=scope.wssControl('reconnect');finish();await again;
 const disconnect=scope.wssControl('disconnect');finish();await disconnect;
 assert.ok(buttons.every(b=>!b.disabled&&!b.hasAttribute('aria-busy')));
});
