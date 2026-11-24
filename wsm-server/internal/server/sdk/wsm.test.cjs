const {test} = require('node:test');
const assert = require('node:assert/strict');
const WSMServer = require('./wsm.js');
const tick = () => new Promise(resolve=>setImmediate(resolve));
function jwt(extra={}) {return 'e30.'+Buffer.from(JSON.stringify({role:'browser',client_id:'web-1',exp:Math.floor(Date.now()/1000)+300,...extra})).toString('base64url')+'.signature';}
class Socket {
 static instances=[];
 constructor(url,protocols){this.url=url;this.protocols=protocols;this.sent=[];Socket.instances.push(this);}
 send(data){this.sent.push(JSON.parse(data));}
 receive(message){this.onmessage?.({data:JSON.stringify(message)});}
 answer(request,type,payload,peer){this.receive({type,correlation_id:request.request_id,payload,...(peer?{connection_id:peer}:{})});}
 close(){this.onclose?.({code:1000,reason:'closed',wasClean:true});}
}
global.WebSocket=Socket;
async function connected(options={}){
 const wsm=WSMServer('wss://wslocal.wisemed.eu',jwt(),{autoReconnect:false,...options});
 await tick();const ws=Socket.instances.at(-1);ws.onopen();
 assert.equal(wsm.isConnected,false);
 assert.deepEqual(ws.sent[0],{type:'hello',payload:{client_type:'browser',client_id:'web-1'}});
 ws.receive({type:'hello_ack',payload:{connection_id:'browser-conn'}});await wsm.ready;
 return {wsm,ws};
}
test('factory and constructor normalize URL; authentication is only in subprotocols',async()=>{
 const {wsm,ws}=await connected();try{
  assert.equal(ws.url,'wss://wslocal.wisemed.eu/ws');assert.equal(ws.protocols[0],'wsm.v1');assert.match(ws.protocols[1],/^wsm.jwt./);
  assert.equal(wsm.isConnected,true);assert.equal(wsm.connectionID,'browser-conn');
  await assert.rejects(wsm.send('command',{mode:'server'},{command:'server.status'}),{code:'invalid_target'});
  assert.throws(()=>new WSMServer('http://public.example',jwt()),/wss/);
  assert.throws(()=>WSMServer('wss://example/?token=secret',jwt()),/query/);
 }finally{wsm.disconnect();}
});
test('requests wait for hello and concurrent replies correlate independently',async()=>{
 const wsm=WSMServer('wss://example',jwt(),{autoReconnect:false});
 const ping=wsm.ping();await tick();const ws=Socket.instances.at(-1);ws.onopen();assert.equal(ws.sent.length,1);
 ws.receive({type:'hello_ack',payload:{connection_id:'browser'}});await tick();
 const req=ws.sent.at(-1);ws.answer(req,'pong',{alive:true});assert.equal((await ping).alive,true);
 const a=wsm.serverStatus(),b=wsm.serverStatus();await tick();const [ra,rb]=ws.sent.slice(-2);
 ws.answer(rb,'reply',{n:2});ws.answer(ra,'reply',{n:1});assert.equal((await a).n,1);assert.equal((await b).n,2);wsm.disconnect();
});
test('API waits for actual peer response, ignores forged peer and preserves binary/status',async()=>{
 const {wsm,ws}=await connected();try{
  let completed=false;
  const response=wsm.callAPI({connectionID:'peer'},'/api/status',{method:'GET',query:{x:'a b'}}).then(r=>{completed=true;return r;});
  await tick();const request=ws.sent.at(-1);assert.equal(request.payload.args.path,'/api/status?x=a+b');
  ws.answer(request,'command_ack',{recipients:1});await tick();assert.equal(completed,false);
  ws.answer(request,'reply',{kind:'api.response',status:200,body:'forged'},'other-peer');await tick();assert.equal(completed,false);
  ws.answer(request,'reply',{kind:'api.response',status:200,body_base64:'AAEC'},'peer');assert.deepEqual(Array.from((await response).bytes),[0,1,2]);
  let failed;const bad=wsm.callAPI({id:'peer'},'/api/status',{},e=>{failed=e;});
  const rejection=assert.rejects(bad,e=>e.status===403&&e.response.body.reason==='forbidden');await tick();
  ws.answer(ws.sent.at(-1),'reply',{kind:'api.response',status:403,body:{reason:'forbidden'}},'peer');await rejection;assert.equal(failed.status,403);
 }finally{wsm.disconnect();}
});
test('reply before ACK succeeds; missing recipient and server errors reject',async()=>{
 const {wsm,ws}=await connected();try{
  const call=wsm.command({id:'peer'},'device.ping');await tick();const request=ws.sent.at(-1);
  ws.answer(request,'reply',{ok:true},'peer');ws.answer(request,'command_ack',{recipients:1});assert.equal((await call).ok,true);
  const missing=wsm.command({id:'offline'},'device.ping');const rejected=assert.rejects(missing,{code:'no_recipient'});await tick();ws.answer(ws.sent.at(-1),'command_ack',{recipients:0});await rejected;
  const denied=wsm.serverStatus();const refused=assert.rejects(denied,{code:'forbidden'});await tick();ws.answer(ws.sent.at(-1),'error',{code:'forbidden',message:'denied'});await refused;
 }finally{wsm.disconnect();}
});
test('duplicate client IDs reject ambiguous calls; explicit array performs fan-out',async()=>{
 const {wsm,ws}=await connected();try{
  const ambiguous=wsm.callAPI('duplicate','/api/status');const rejected=assert.rejects(ambiguous,{code:'ambiguous_target'});await tick();
  ws.answer(ws.sent.at(-1),'connections',{connections:[{id:'a',client_id:'duplicate'},{id:'b',client_id:'duplicate'}]});await rejected;
  const all=wsm.callAPI([{id:'a'},{id:'b'}],'/api/status');await tick();
  for(const req of ws.sent.slice(-2))ws.answer(req,'reply',{kind:'api.response',status:200,body:req.target.connection_id},req.target.connection_id);
  assert.deepEqual((await all).map(r=>r.body),['a','b']);
 }finally{wsm.disconnect();}
});
test('timeouts and disconnect reject pending operations without replay',async()=>{
 const {wsm,ws}=await connected({timeout:10});
 await assert.rejects(wsm.command({id:'peer'},'change'),{code:'timeout'});
 const requestCount=ws.sent.length;
 const pending=wsm.ping();const rejected=assert.rejects(pending,{code:'disconnected'});await tick();wsm.disconnect();await rejected;
 assert.equal(ws.sent.length,requestCount+1);assert.equal(wsm._pending.size,0);
});
test('expired token requests renewal; provider and manual reconnect use fresh tokens',async()=>{
 const expired=WSMServer('wss://example',jwt({exp:1}));let renewal=0;expired.onTokenRequired=()=>renewal++;
 await assert.rejects(expired.ready,{code:'token_expired'});assert.equal(renewal,1);expired.disconnect();
 let count=0;const {wsm,ws}=await connected({tokenProvider:async()=>{count++;return jwt();}});
 assert.equal(count,1);const next=wsm.reconnect();await tick();const newer=Socket.instances.at(-1);assert.notEqual(ws,newer);
 newer.onopen();newer.receive({type:'hello_ack',payload:{connection_id:'new'}});await next;assert.equal(count,2);wsm.disconnect();
});
test('subscription failure rejects, incoming command can be replied to and callbacks are isolated',async()=>{
 const {wsm,ws}=await connected();try{
  const sub=wsm.subscribe('topic');const rejected=assert.rejects(sub,{code:'operation_failed'});await tick();ws.answer(ws.sent.at(-1),'subscribe_ack',{ok:false});await rejected;
  let incoming;wsm.onCommand=m=>{incoming=m;};ws.receive({type:'command',request_id:'incoming',connection_id:'peer',payload:{command:'test'}});
  const reply=wsm.reply(incoming,{ok:true});await tick();const sent=ws.sent.at(-1);assert.equal(sent.correlation_id,'incoming');assert.equal(sent.target.connection_id,'peer');ws.answer(sent,'command_ack',{recipients:1});await reply;
 }finally{wsm.disconnect();}
});

test('getClientsByType accepts arbitrary exact types and returns null when absent',async()=>{
 const {wsm,ws}=await connected();try{
  for(const type of ['future-utility','siui-bridge','reader','service','siui-server','']){
   const result=wsm.getClientsByType(type);await tick();
   ws.answer(ws.sent.at(-1),'connections',{connections:[{id:'a',client_id:'one',client_type:'future-utility'}]});
   const matches=await result;
   if(type==='future-utility')assert.equal(matches[0].clientID,'one');else assert.equal(matches,null);
  }
 }finally{wsm.disconnect();}
});

test('lazy initialization calls token provider only when needed and shares connections',async()=>{
 let tokens=0;const before=Socket.instances.length;
 const wsm=WSMServer('wss://example',null,{autoConnect:false,autoReconnect:false,tokenProvider:async()=>{tokens++;return jwt();}});
 await tick();assert.equal(tokens,0);assert.equal(Socket.instances.length,before);
 const first=wsm.ping(),second=wsm.serverStatus();await tick();
 assert.equal(tokens,1);assert.equal(Socket.instances.length,before+1);
 let ws=Socket.instances.at(-1);ws.onopen();ws.receive({type:'hello_ack',payload:{connection_id:'lazy'}});await tick();
 for(const request of ws.sent.slice(1))ws.answer(request,request.type==='ping'?'pong':'reply',{});
 await Promise.all([first,second]);assert.equal(tokens,1);
 wsm.disconnect();const again=wsm.ping();await tick();assert.equal(tokens,2);
 ws=Socket.instances.at(-1);ws.onopen();ws.receive({type:'hello_ack',payload:{connection_id:'renewed'}});await tick();ws.answer(ws.sent.at(-1),'pong',{});await again;wsm.disconnect();
});
test('token provider timeout prevents late socket creation and permits a fresh attempt',async()=>{
 let resolve,requests=0;const before=Socket.instances.length;
 const wsm=WSMServer('wss://example',null,{autoConnect:false,autoReconnect:false,connectTimeout:5,tokenProvider:()=>{requests++;return new Promise(done=>{resolve=done;});}});
 await assert.rejects(wsm.connect(),{code:'connect_timeout'});
 resolve(jwt());await tick();assert.equal(Socket.instances.length,before);
 wsm.options.tokenProvider=async()=>{requests++;return jwt();};wsm.options.connectTimeout=1000;
 const pending=wsm.connect();await tick();const ws=Socket.instances.at(-1);ws.onopen();ws.receive({type:'hello_ack',payload:{connection_id:'retry'}});await pending;
 assert.equal(requests,2);wsm.disconnect();
});

const station = {connectionID:'ecard-a',equipmentID:'18'};
function apiAnswer(ws,request,body,status=200) {
 ws.answer(request,'reply',{kind:'api.response',status,body},request.target.connection_id);
}
test('first eCard API calls share automatic hardware subscription; discovery and other APIs do not subscribe',async()=>{
 const {wsm,ws}=await connected();try{
  const a=wsm.callAPI(station,'/api/ecard/health'),b=wsm.callAPI(station,'/api/ecard/events');
  await tick();const subscriptions=ws.sent.filter(m=>m.payload?.args?.path==='/api/ecard/events/subscribe');
  assert.equal(subscriptions.length,1);assert.deepEqual(subscriptions[0].payload.args.body,{hardware:true});
  assert.equal(subscriptions[0].target.connection_id,'ecard-a');
  apiAnswer(ws,subscriptions[0],{subscription_id:'auto-1'});await tick();
  for(const request of ws.sent.slice(-2))apiAnswer(ws,request,{});
  await Promise.all([a,b]);
  const discovery=wsm.listClients();await tick();ws.answer(ws.sent.at(-1),'connections',{connections:[{id:'ecard-b',equipment_id:'19'}]});await discovery;
  assert.equal(ws.sent.filter(m=>m.payload?.args?.path==='/api/ecard/events/subscribe').length,1);
  const events=[];wsm.onEvent=e=>events.push(e.payload.data.event);
  for(const event of ['card_inserted','card_removed'])ws.receive({type:'event',connection_id:'ecard-a',payload:{event:'ecard.event',data:{event}}});
  assert.deepEqual(events,['card_inserted','card_removed']);
 }finally{wsm.disconnect();}
});
test('station switch deletes old subscription and subscribes only selected station',async()=>{
 const {wsm,ws}=await connected();try{
  let pending=wsm.selectECard(station);await tick();apiAnswer(ws,ws.sent.at(-1),{subscription_id:'first'});await pending;
  pending=wsm.selectECard({connectionID:'ecard-b',equipmentID:'19'});await tick();
  const requests=ws.sent.slice(-2),remove=requests.find(m=>m.payload.args.method==='DELETE'),add=requests.find(m=>m.payload.args.method==='POST');
  assert.equal(remove.target.connection_id,'ecard-a');assert.deepEqual(remove.payload.args.body,{subscription_id:'first'});
  assert.equal(add.target.connection_id,'ecard-b');apiAnswer(ws,remove,{ok:true});apiAnswer(ws,add,{subscription_id:'second'});await pending;
  assert.equal(wsm._ecard.subscription,'second');
 }finally{wsm.disconnect();}
});
test('browser and selected station reconnect restore by equipment identity without replaying operations',async()=>{
 const {wsm,ws}=await connected();try{
  const selection=wsm.selectECard(station);await tick();apiAnswer(ws,ws.sent.at(-1),{subscription_id:'first'});await selection;
  const reconnect=wsm.reconnect();await tick();const next=Socket.instances.at(-1);next.onopen();next.receive({type:'hello_ack',payload:{connection_id:'browser-new'}});await reconnect;await tick();
  assert.equal(next.sent.at(-1).type,'list_connections');
  next.answer(next.sent.at(-1),'connections',{connections:[{id:'wrong',equipment_id:'19'},{id:'ecard-new',equipment_id:'18'}]});await tick();
  const subscription=next.sent.at(-1);assert.equal(subscription.target.connection_id,'ecard-new');assert.deepEqual(subscription.payload.args.body,{hardware:true});
  apiAnswer(next,subscription,{subscription_id:'restored'});await tick();
  next.receive({type:'presence',payload:{event:'disconnected',equipment_id:'18',connection_id:'ecard-new'}});
  next.receive({type:'presence',payload:{event:'connected',equipment_id:'18',connection_id:'ecard-newer'}});await tick();
  next.answer(next.sent.at(-1),'connections',{connections:[{id:'ecard-newer',equipment_id:'18'}]});await tick();
  assert.equal(next.sent.at(-1).target.connection_id,'ecard-newer');apiAnswer(next,next.sent.at(-1),{subscription_id:'again'});await tick();
  assert.ok(next.sent.filter(m=>m.type==='command').every(m=>m.payload.args.path==='/api/ecard/events/subscribe'));
 }finally{wsm.disconnect();}
});
test('renewal uses existing lease and expired lease is recreated automatically',async()=>{
 const {wsm,ws}=await connected();try{
  let pending=wsm.selectECard(station);await tick();apiAnswer(ws,ws.sent.at(-1),{subscription_id:'first'});await pending;
  pending=wsm._subscribeECard(wsm._ecard,true);await tick();assert.equal(ws.sent.at(-1).payload.args.body.subscription_id,'first');
  apiAnswer(ws,ws.sent.at(-1),{error:'subscription not found'},404);await tick();
  assert.deepEqual(ws.sent.at(-1).payload.args.body,{hardware:true});apiAnswer(ws,ws.sent.at(-1),{subscription_id:'renewed'});await pending;
  assert.equal(wsm._ecard.subscription,'renewed');
 }finally{wsm.disconnect();}
});
test('failed subscription blocks hardware operation and reports the failure',async()=>{
 const {wsm,ws}=await connected();try{
  const operation=wsm.callAPI(station,'/api/ecard/operations',{method:'POST',body:{operation:'sign'}});
  const rejected=assert.rejects(operation,{status:403});await tick();apiAnswer(ws,ws.sent.at(-1),{error:'denied'},403);await rejected;
  assert.equal(ws.sent.filter(m=>m.payload?.args?.path==='/api/ecard/operations').length,0);
 }finally{wsm.disconnect();}
});
test('deselection during subscription cleans late lease and does not submit pending operation',async()=>{
 const {wsm,ws}=await connected();try{
  const operation=wsm.callAPI(station,'/api/ecard/operations',{method:'POST',body:{operation:'sign'}});
  const rejected=assert.rejects(operation,{code:'ecard_selection_changed'});await tick();const sub=ws.sent.at(-1);
  await wsm.clearECard();apiAnswer(ws,sub,{subscription_id:'late'});await rejected;await tick();
  assert.equal(ws.sent.at(-1).payload.args.method,'DELETE');apiAnswer(ws,ws.sent.at(-1),{ok:true});
  assert.equal(ws.sent.filter(m=>m.payload?.args?.path==='/api/ecard/operations').length,0);
 }finally{wsm.disconnect();}
});
test('connection-only selection resolves stable equipment identity for later reconnect',async()=>{
 const {wsm,ws}=await connected();try{
  const selection=wsm.selectECard({id:'ecard-a'});await tick();assert.equal(ws.sent.at(-1).type,'list_connections');
  ws.answer(ws.sent.at(-1),'connections',{connections:[{id:'ecard-a',equipment_id:'18'},{id:'ecard-b',equipment_id:'19'}]});await tick();
  assert.equal(ws.sent.at(-1).target.connection_id,'ecard-a');apiAnswer(ws,ws.sent.at(-1),{subscription_id:'selected'});await selection;
  assert.equal(wsm._ecard.equipmentID,'18');
 }finally{wsm.disconnect();}
});
test('authentication renews without socket replacement or disconnect and keeps eCard selection',async()=>{
 let expires=Math.floor(Date.now()/1000)+300;
 const {wsm,ws}=await connected({tokenProvider:async()=>jwt({exp:expires})});
 try {
  let disconnected=0;wsm.onDisconnect=()=>disconnected++;
  wsm._authRefreshSupported=true;
  const selection=wsm.selectECard(station);await tick();apiAnswer(ws,ws.sent.at(-1),{subscription_id:'stable'});await selection;
  expires+=300;const renewal=wsm._refreshAuth();await tick();const request=ws.sent.at(-1);
  assert.equal(request.type,'auth.refresh');assert.ok(request.payload.token);
  ws.answer(request,'auth_refreshed',{expires_at:new Date(expires*1000).toISOString()});await renewal;
  assert.equal(disconnected,0);assert.equal(wsm._ws,ws);assert.equal(wsm._ecard.subscription,'stable');assert.equal(wsm._expires,expires*1000);
 }finally{wsm.disconnect();}
});
test('generic topic subscriptions are restored after reconnect',async()=>{
 const {wsm,ws}=await connected();try{
  const subscription=wsm.subscribe('logs:utility');await tick();ws.answer(ws.sent.at(-1),'subscribe_ack',{ok:true});await subscription;
  const reconnect=wsm.reconnect();await tick();const next=Socket.instances.at(-1);next.onopen();next.receive({type:'hello_ack',payload:{connection_id:'new'}});await reconnect;await tick();
  assert.equal(next.sent.at(-1).type,'subscribe');assert.equal(next.sent.at(-1).payload.topic,'logs:utility');
  next.answer(next.sent.at(-1),'subscribe_ack',{ok:true});await tick();
 }finally{wsm.disconnect();}
});
