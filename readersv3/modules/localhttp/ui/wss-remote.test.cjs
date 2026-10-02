const {test} = require('node:test');
const assert = require('node:assert/strict');
const {createTransport} = require('./wss-remote.js');
class Socket {
  static latest;
  constructor(url, protocols) { this.url=url; this.protocols=protocols; this.sent=[]; Socket.latest=this; }
  send(raw) { this.sent.push(JSON.parse(raw)); }
  close() { this.onclose?.(); }
  receive(type,payload, id) { this.onmessage({data:JSON.stringify({type,payload,correlation_id:id})}); }
}
function setup(timeout=1000) {
  const transport=createTransport({WebSocket:Socket,url:'wss://central/ws',equipmentID:'eq-1',timeout});
  transport.connect('ticket_safe'); const ws=Socket.latest; ws.onopen();
  return {transport,ws,ready:()=>ws.receive('hello_ack',{})};
}
const tick=()=>new Promise(resolve=>setImmediate(resolve));
test('queues initial fetch until hello, uses ticket protocol, ignores routing ACK',async()=>{
  const {transport,ws,ready}=setup();
  assert.deepEqual(ws.protocols,['wsm.v1','wsm.ticket.ticket_safe']);
  const result=transport.fetch('https://central/api/session'); await tick();
  assert.equal(ws.sent.length,1); ready(); await tick();
  const command=ws.sent[1]; assert.equal(command.target.equipment_id,'eq-1');
  assert.equal(command.payload.command,'api.request');
  ws.receive('command_ack',{recipients:1},command.request_id);
  let settled=false;result.then(()=>settled=true);await tick();assert.equal(settled,false);
  ws.receive('reply',{kind:'api.response',status:200,body:{ok:true},content_type:'application/json'},command.request_id);
  assert.deepEqual(await (await result).json(),{ok:true});transport.close();
});
test('correlates concurrent calls and preserves body bytes and HTTP status',async()=>{
  const {transport,ws,ready}=setup();ready();
  const a=transport.fetch('https://central/api/a',{method:'POST',body:'{"hello":"ț"}',headers:{'content-type':'application/json'}});
  const b=transport.fetch('https://central/api/b');await tick();
  const ca=ws.sent.find(m=>m.payload?.args?.path==='/api/a'), cb=ws.sent.find(m=>m.payload?.args?.path==='/api/b');assert.equal(Buffer.from(ca.payload.args.body_base64,'base64').toString(),'{"hello":"ț"}');
  ws.receive('reply',{kind:'api.response',status:409,body:{error:'conflict'}},cb.request_id);
  ws.receive('reply',{kind:'api.response',status:204},ca.request_id);
  assert.equal((await a).status,204);assert.equal((await b).status,409);transport.close();
});
test('offline routing and disconnect reject pending requests',async()=>{
  const {transport,ws,ready}=setup();ready();
  const a=transport.fetch('https://central/api/a');const check=assert.rejects(a,/offline/);await tick();
  ws.receive('command_ack',{recipients:0},ws.sent[1].request_id);await check;
  const b=transport.fetch('https://central/api/b');const closed=assert.rejects(b,/closed/);await tick();ws.close();await closed;transport.close();
});
test('AbortSignal stops queued and in-flight requests',async()=>{
  const {transport,ws,ready}=setup();let controller=new AbortController();
  const queued=transport.fetch('https://central/api/a',{signal:controller.signal});const check=assert.rejects(queued,{name:'AbortError'});await tick();controller.abort();await check;
  ready();controller=new AbortController();const inflight=transport.fetch('https://central/api/b',{signal:controller.signal});const second=assert.rejects(inflight,{name:'AbortError'});await tick();controller.abort();await second;transport.close();
});
test('request timeout, 4MiB limit, and HTML response rejection',async()=>{
  const {transport,ws,ready}=setup(30);ready();
  await assert.rejects(transport.fetch('https://central/api/a'),/timed out/);
  await assert.rejects(transport.fetch('https://central/api/a',{method:'POST',body:new Uint8Array(4*1024*1024+1)}),/4 MiB/);
  const result=transport.fetch('https://central/api/a');const check=assert.rejects(result,/HTML/);await tick();
  ws.receive('reply',{kind:'api.response',status:200,content_type:'text/html',body:'bad'},ws.sent.at(-1).request_id);await check;transport.close();
});
test('binary response remains binary',async()=>{
  const {transport,ws,ready}=setup();ready();const result=transport.fetch('https://central/api/order-image');await tick();
  ws.receive('reply',{kind:'api.response',status:200,content_type:'image/png',body_base64:'AAEC/w=='},ws.sent.at(-1).request_id);
  assert.deepEqual([...new Uint8Array(await (await result).arrayBuffer())],[0,1,2,255]);transport.close();
});
test('initial API request can await credential handoff before connecting',async()=>{
  const transport=createTransport({WebSocket:Socket,url:'wss://central/ws',equipmentID:'eq-1',timeout:1000});
  const result=transport.fetch('https://central/api/session');await tick();
  transport.connect('new_ticket');const ws=Socket.latest;ws.onopen();ws.receive('hello_ack',{});await tick();
  ws.receive('reply',{kind:'api.response',status:200,body:{ok:true}},ws.sent.at(-1).request_id);
  assert.equal((await result).status,200);transport.close();
});
test('browser bootstrap validates exact opener/source and keeps API off HTTP',async()=>{
  const vm=require('node:vm'),fs=require('node:fs');const listeners={};let nativeCalls=0;
  const opener={postMessage(){}},location=new URL('https://central/control/?equipment_id=eq-1&opener_origin=https%3A%2F%2Fwisemed');
  const window={location,opener,WebSocket:Socket,history:{pushState(){},replaceState(){}},fetch:()=>{nativeCalls++;return Promise.resolve(new Response('asset'));},addEventListener:(type,callback)=>listeners[type]=callback};
  const document={referrer:'',readyState:'loading',addEventListener(){}};
  const context={window,document,location,URL,URLSearchParams,Request,Response,Headers,Uint8Array,DOMException,btoa,atob,setTimeout,clearTimeout,setInterval,Date};
  vm.runInNewContext(fs.readFileSync(require.resolve('./wss-remote.js'),'utf8'),context);
  const old=Socket.latest;
  listeners.message({source:opener,origin:'https://evil',data:{type:'wsm-control-auth',ticket:'bad'}});assert.equal(Socket.latest,old);
  listeners.message({source:{},origin:'https://wisemed',data:{type:'wsm-control-auth',ticket:'bad'}});assert.equal(Socket.latest,old);
  const result=window.fetch('/api/session');await tick();assert.equal(nativeCalls,0);
  listeners.message({source:opener,origin:'https://wisemed',data:{type:'wsm-control-auth',ticket:'valid'}});
  const ws=Socket.latest;ws.onopen();ws.receive('hello_ack',{});await tick();
  ws.receive('reply',{kind:'api.response',status:200,body:{ok:true}},ws.sent.at(-1).request_id);
  assert.equal((await result).status,200);await window.fetch('/app.js');assert.equal(nativeCalls,1);ws.close();
});
test('delayed initial credential and failed handshake preserve startup without sending twice',async()=>{
  const transport=createTransport({WebSocket:Socket,url:'wss://central/ws',equipmentID:'eq-1',timeout:15});
  const result=transport.fetch('https://central/api/preferences');
  await new Promise(resolve=>setTimeout(resolve,40)); // Longer than request deadline before credentials.
  transport.connect('expired_ticket');const first=Socket.latest;first.onopen();
  await new Promise(resolve=>setTimeout(resolve,40)); // Handshake timeout, no hello_ack.
  assert.equal(first.sent.filter(m=>m.type==='command').length,0);
  transport.connect('fresh_ticket');const second=Socket.latest;second.onopen();second.receive('hello_ack',{});await tick();
  assert.equal(second.sent.filter(m=>m.type==='command').length,1);
  second.receive('reply',{kind:'api.response',status:200,body:{preferences:{}}},second.sent.at(-1).request_id);
  assert.equal((await result).status,200);transport.close();
});
test('aborted startup is not replayed on successful authentication',async()=>{
  const transport=createTransport({WebSocket:Socket,url:'wss://central/ws',equipmentID:'eq-1',timeout:15});
  const controller=new AbortController();const result=transport.fetch('https://central/api/command',{method:'POST',signal:controller.signal});
  const check=assert.rejects(result,{name:'AbortError'});await tick();controller.abort();await check;
  transport.connect('valid');const ws=Socket.latest;ws.onopen();ws.receive('hello_ack',{});await tick();
  assert.equal(ws.sent.filter(m=>m.type==='command').length,0);transport.close();
});
test('empty 200 response is empty; explicit JSON null is preserved',async()=>{
  const {transport,ws,ready}=setup();ready();
  const empty=transport.fetch('https://central/api/empty');await tick();
  ws.receive('reply',{kind:'api.response',status:200},ws.sent.at(-1).request_id);assert.equal(await (await empty).text(),'');
  const jsonNull=transport.fetch('https://central/api/null');await tick();
  ws.receive('reply',{kind:'api.response',status:200,body:null},ws.sent.at(-1).request_id);assert.equal(await (await jsonNull).text(),'null');transport.close();
});
test('disconnection countdown requests fresh tickets every 30 seconds and logout stops retries',async()=>{
  const vm=require('node:vm'),fs=require('node:fs'),listeners={},posted=[],elements=[];let periodic,now=1000;
  const opener={postMessage(message,origin){posted.push({message,origin});}};
  const location=new URL('https://central/control/?equipment_id=eq-1&opener_origin=https%3A%2F%2Fwisemed');
  const window={location,opener,WebSocket:Socket,history:{pushState(){},replaceState(){}},fetch(){throw new Error('Unexpected native fetch');},addEventListener:(type,callback)=>listeners[type]=callback};
  const document={referrer:'',readyState:'complete',body:{prepend(){}},createElement(tag){const element={tag,style:{},setAttribute(){},append(){}};elements.push(element);return element;}};
  const context={window,document,location,URL,URLSearchParams,Request,Response,Headers,Uint8Array,DOMException,btoa,atob,setTimeout,clearTimeout,setInterval:callback=>periodic=callback,Date:{now:()=>now}};
  vm.runInNewContext(fs.readFileSync(require.resolve('./wss-remote.js'),'utf8'),context);
  assert.equal(posted[0].message.type,'wsm-control-ready');assert.equal(posted[0].origin,'https://wisemed');
  listeners.message({source:opener,origin:'https://wisemed',data:{type:'wsm-control-auth',ticket:'valid'}});
  const ws=Socket.latest;ws.onopen();ws.receive('hello_ack',{});ws.close();
  now+=29000;periodic();assert.equal(posted.length,1);assert.match(elements.find(e=>e.tag==='span').textContent,/1s/);
  now+=1000;periodic();assert.equal(posted.at(-1).message.type,'wsm-control-refresh');
  now+=1000;periodic();assert.equal(posted.length,2);
  now+=29000;periodic();assert.equal(posted.length,3);
  window.WSM_REMOTE.close();now+=30000;periodic();assert.equal(posted.length,3);
  listeners.message({source:opener,origin:'https://wisemed',data:{type:'wsm-control-auth',ticket:'late'}});assert.equal(Socket.latest,ws);
});
