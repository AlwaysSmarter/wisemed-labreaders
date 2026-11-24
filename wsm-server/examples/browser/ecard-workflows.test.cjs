const {test}=require('node:test');
const assert=require('node:assert/strict');
global.crypto=require('node:crypto').webcrypto;
const {Workflow,matches,legacyServiceText}=require('./ecard-workflows.js');
const card={A1:'TEST-CARD',B1:'TEST',B2:'PATIENT',B4:'1234567890123',C1:'TEST-CID'};
const contract={cif:'TEST',cui:'TEST',number:'TEST',date:'2026-01-01',house:'CAS-B',provider_type:'PAR'};
const device={connectionID:'station-connection',equipmentID:'18',readerID:'wisemed-pias-ecard'};
class WSS {
 constructor(){this.ready=Promise.resolve();this.isConnected=true;this.jobs=new Map();this.calls=[];this.pin=false;this.signState='completed';this.signData=null;this.present=true;this.failPost=false;this.failPIN=false;}
 async listClients(){return [device];}
 async callAPI(target,path,options={}){
  assert.equal(target.connectionID,device.connectionID);
  this.calls.push({path,...structuredClone(options)});
  const respond=body=>({kind:'api.response',status:200,body});
  if(path.endsWith('/health'))return respond({sdk_order:['cnas-wrapper-1.0.0.0']});
  if(path.endsWith('/events/subscribe'))return respond({subscription_id:'sub-1'});
  if(path.endsWith('/operations')){
   if(this.failPost){this.failPost=false;throw Object.assign(new Error('response lost'),{code:'timeout'});}
   const body=options.body,op=body.request.operation;
   const data=op==='status'?{TerminalCurent:'Gemalto 0',StareCardInTerminal:this.present?'CardInserat':'CardRetras'}:
    op==='read'?{...card}:this.signData||body.items.map(item=>({id:item.id,ok:true,signature:Buffer.from('CMS-'+item.text).toString('base64')}));
   const state=op==='sign'?this.signState:'completed';
   const report={operation_id:body.operation_id,state,used_sdk:'cnas-wrapper-1.0.0.0',response:{ok:state==='completed',data,error:state==='completed'?null:{code:'card_changed',message:'card removed'}},events:[],errors:[],other_sdks:[]};
   this.jobs.set(body.operation_id,{report,request:body.request,pin:this.pin&&op!=='status',items:body.items});
   return respond({id:body.operation_id,state:'queued'});
  }
  const id=path.split('/jobs/')[1]?.split(/[/?]/)[0],job=this.jobs.get(id);
  if(!job)throw Object.assign(new Error('not found'),{status:404});
  if(path.endsWith('/pin')){
   if(this.failPIN){this.failPIN=false;throw Object.assign(new Error('PIN reply lost'),{code:'timeout'});}
   assert.equal(options.body.challenge,'challenge-1');assert.equal(options.body.value,'8642');job.pin=false;return respond({ok:true});
  }
  if(path.endsWith('/cancel')){job.pin=false;job.report.state='cancelled';job.report.response.ok=false;return respond({ok:true});}
  if(path.endsWith('/report'))return respond(job.report);
  return respond({job:{state:job.pin?'waiting_pin':job.report.state,completed:job.report.state==='completed'?1:0,total:1},...(job.pin?{pin_request:{challenge:'challenge-1',prompt:'PIN-ul dvs.?'}}:{})});
 }
 event(event,data={reader:'Gemalto 0'},extra={}){this.onEvent?.({type:'event',connection_id:device.connectionID,payload:{event:'ecard.event',data:{id:crypto.randomUUID(),source:'pcsc',job:'',event,data,...extra}}});}
}
function setup(options={}){
 const wsm=new WSS(),saved=[],created=[],states=[];let unsigned=false,prepareResolve;
 const plan={operationId:crypto.randomUUID(),fisaId:'TEST-FISA',version:1,expected:{cnp:card.B4,cid:card.C1},items:[{id:'service-1',text:'EXACT|TEXT\r\n'}]};
 const backend={
  async createFisaFromCard(input){created.push(input);return {id:'TEST-FISA',expected:input.expected};},
  async prepareSigning(){if(!unsigned)return null;return structuredClone(plan);},
  async saveSigningResult(result){saved.push(result);unsigned=false;}
 };
 const flow=new Workflow({wsm,equipmentID:'18',readerName:'Gemalto 0',contract,backend,requestPIN:async()=> '8642',onState:s=>states.push(s),pollMS:0,...options});
 return {wsm,flow,saved,created,states,plan,setUnsigned:value=>{unsigned=value;}};
}
const signingCalls=wsm=>wsm.calls.filter(c=>c.body?.request?.operation==='sign');
test('mode 1: insertion creates one file; saved services sign with exact bytes and stable UUID',async()=>{
 const x=setup();x.wsm.present=false;
 try{
  await x.flow.start();assert.equal(x.created.length,0);
  x.wsm.event('card_inserted');await x.flow._tail;assert.equal(x.created.length,1);assert.equal(signingCalls(x.wsm).length,0);
  x.setUnsigned(true);await x.flow.servicesSaved();
  assert.equal(x.saved.length,1);const call=signingCalls(x.wsm)[0];
  assert.equal(call.body.operation_id,x.plan.operationId);assert.equal(call.body.items[0].text,'EXACT|TEXT\r\n');assert.deepEqual(call.body.request.expected,x.plan.expected);
  x.wsm.event('card_inserted');await x.flow._tail;await x.flow.servicesSaved();
  assert.equal(x.created.length,1);assert.equal(signingCalls(x.wsm).length,1);
 }finally{await x.flow.stop();}
});
test('create mode handles the next inserted card while existing-file mode retains its file',async()=>{
 const x=setup();try{
  await x.flow.start();assert.equal(x.created.length,1);
  x.wsm.event('card_removed');x.wsm.event('card_inserted');await x.flow._tail;assert.equal(x.created.length,2);
  await x.flow.enterFisa({id:'EXISTING',expected:{cnp:card.B4,cid:card.C1}});
  x.wsm.event('card_removed');x.wsm.event('card_inserted');await x.flow._tail;
  assert.equal(x.created.length,2);assert.equal(x.flow.fisa.id,'EXISTING');
 }finally{await x.flow.stop();}
});
test('mode 2: opening existing file with matching inserted card signs automatically',async()=>{
 const x=setup();x.setUnsigned(true);x.flow.mode='existing';x.flow.fisa={id:x.plan.fisaId,expected:x.plan.expected};
 try{await x.flow.start();assert.equal(x.created.length,0);assert.equal(x.saved.length,1);assert.equal(signingCalls(x.wsm).length,1);}finally{await x.flow.stop();}
});
test('different CNP never submits signing even with matching CID',async()=>{
 const x=setup();x.setUnsigned(true);x.flow.mode='existing';x.flow.fisa={id:x.plan.fisaId,expected:{cnp:'9999999999999',cid:card.C1}};
 try{await x.flow.start();assert.equal(signingCalls(x.wsm).length,0);assert.ok(x.states.some(s=>s.state==='identity_mismatch'));}finally{await x.flow.stop();}
});
test('anonymous card requires expected CID; ordinary CNP mismatch is never bypassed',()=>{
 assert.equal(matches({cnp:'123'},{cnp:'0000000000000',cid:'cid'}),false);
 assert.equal(matches({cnp:'0000000000000'},{cnp:'123',cid:'cid'}),false);
 assert.equal(matches({cnp:'123',cid:'cid'},{cnp:'0000000000000',cid:'cid'}),true);
 assert.equal(matches({cnp:'123',cid:'cid'},{cnp:'456',cid:'cid'}),false);
});
test('PIN is answered through challenge endpoint; event/callback never receives the PIN value',async()=>{
 const x=setup();x.wsm.pin=true;
 try{await x.flow.start();const calls=x.wsm.calls.filter(c=>c.path.endsWith('/pin'));assert.equal(calls.length,1);assert.equal(calls[0].body.value,'8642');assert.ok(!JSON.stringify(x.states).includes('8642'));}finally{await x.flow.stop();}
});
test('removal during backend preparation prevents signing; wrong reader/peer events ignored',async()=>{
 const x=setup();try{
  await x.flow.start();x.setUnsigned(true);
  let release;const ready=new Promise(r=>release=r);x.flow.backend.prepareSigning=()=>ready;
  const signing=x.flow.servicesSaved();await new Promise(r=>setImmediate(r));
  x.wsm.event('card_removed');release(x.plan);await signing;assert.equal(signingCalls(x.wsm).length,0);
  x.wsm.event('card_inserted',{reader:'Other 0'});await x.flow._tail;assert.equal(x.flow.present,false);
  x.flow._receive({connection_id:'other-station',payload:{event:'ecard.event',data:{id:99,source:'pcsc',event:'card_inserted',data:{reader:'Gemalto 0'}}}});assert.equal(x.flow.present,false);
 }finally{await x.flow.stop();}
});
test('partial signatures are saved against original file even when terminal state failed',async()=>{
 const x=setup();try{
  await x.flow.start();x.setUnsigned(true);x.wsm.signState='failed';x.wsm.signData=[{id:'service-1',ok:true,signature:'Q01T'},{id:'service-2',ok:false,code:'card_changed'}];
  await assert.rejects(x.flow.servicesSaved(),{code:'card_changed'});
  assert.equal(x.saved.length,1);assert.equal(x.saved[0].plan.fisaId,'TEST-FISA');assert.equal(x.saved[0].report.response.data[0].signature,'Q01T');assert.equal(x.flow.pending,null);
 }finally{await x.flow.stop();}
});
test('uncertain POST is not replayed on reconnect; resume only GETs the same UUID',async()=>{
 const x=setup();try{
  await x.flow.start();x.setUnsigned(true);x.wsm.failPost=true;
  await assert.rejects(x.flow.servicesSaved(),{code:'timeout'});assert.equal(x.flow.pending.id,x.plan.operationId);
  await x.flow.reconnect();assert.equal(signingCalls(x.wsm).length,1);
  await assert.rejects(x.flow.resume(),{status:404});assert.equal(signingCalls(x.wsm).length,1);
 }finally{await x.flow.stop();}
});
test('PIN transport failure allows a fresh manual prompt on resume, never blind retransmission',async()=>{
 const x=setup();x.wsm.present=false;try{
  await x.flow.start();x.wsm.pin=true;x.wsm.failPIN=true;x.wsm.present=true;
  await assert.rejects(x.flow.createOnCard(),{code:'timeout'});
  assert.ok(x.flow.pending);await x.flow.resume();assert.equal(x.flow.pending,null);
  assert.equal(x.wsm.calls.filter(c=>c.path.endsWith('/pin')).length,2);
 }finally{await x.flow.stop();}
});
test('legacy service text preserves exact optional second date and never normalizes input',()=>{
 const base={cid:'CID',cardNumber:'CARD',serviceDate:'24/11/2026',code:'TEST'};
 assert.equal(legacyServiceText(base),'CID|CARD|24/11/2026|TEST');
 assert.equal(legacyServiceText({...base,secondDate:''}),'CID|CARD|24/11/2026||TEST');
 assert.equal(legacyServiceText({...base,secondDate:'25/11/2026'}),'CID|CARD|24/11/2026|25/11/2026|TEST');
});
test('chunked report verifies reconstructed SHA-256 over UTF-8 JSON',async()=>{
 const x=setup();const report={state:'completed',message:'Semnătură'},bytes=Buffer.from(JSON.stringify(report));
 const hash=require('node:crypto').createHash('sha256').update(bytes).digest('hex');
 const pieces=[bytes.subarray(0,15),bytes.subarray(15)];
 x.flow._api=async path=>{if(!path.includes('?part='))throw Object.assign(new Error('large'),{status:413});const part=Number(path.split('?part=')[1]);return {part,parts:2,total_bytes:bytes.length,sha256:hash,encoding:'base64-json-utf8',data_base64:pieces[part].toString('base64')};};
 assert.deepEqual(await x.flow._report('op'),report);
 pieces[1]=Buffer.alloc(pieces[1].length,32);await assert.rejects(x.flow._report('op'),{code:'report'});
});

test('page reload starts with persistent plan and recovers without submitting any operation',async()=>{
 const x=setup();x.flow.mode='existing';x.flow.fisa={id:x.plan.fisaId,expected:x.plan.expected};
 const report={operation_id:x.plan.operationId,state:'completed',response:{ok:true,data:[{id:'service-1',ok:true,signature:'Q01T'}]}};
 x.wsm.jobs.set(x.plan.operationId,{report,pin:false});
 try{
  await x.flow.start({pendingPlan:x.plan});await x.flow.resume();
  assert.equal(x.wsm.calls.filter(c=>c.path.endsWith('/operations')).length,0);assert.equal(x.saved.length,1);
 }finally{await x.flow.stop();}
});

test('real WSM browser SDK carries asynchronous operations, events and PIN on api.request wire envelopes',async()=>{
 const WSMServer=require('../../internal/server/sdk/wsm.js');
 const transport=new WSS(),wire=[];transport.present=false;transport.pin=true;
 const oldSocket=global.WebSocket;
 class Socket {
  constructor(){Socket.current=this;queueMicrotask(()=>this.onopen());}
  send(raw){
   const message=JSON.parse(raw);wire.push(message);
   queueMicrotask(async()=>{try{
    if(message.type==='hello')this.receive({type:'hello_ack',payload:{connection_id:'browser-1'}});
    else if(message.type==='list_connections')this.receive({type:'connections',correlation_id:message.request_id,payload:{connections:[{id:device.connectionID,client_id:'pias',client_type:'wisemed-pias-ecard',equipment_id:'18',reader_id:'wisemed-pias-ecard'}]}});
    else if(message.type==='command'){
     assert.equal(message.payload.command,'api.request');const args=message.payload.args;
     const reply=await transport.callAPI({connectionID:message.target.connection_id},args.path,{method:args.method,body:args.body});
     this.receive({type:'reply',connection_id:device.connectionID,correlation_id:message.request_id,payload:reply});
    }
   }catch(error){this.receive({type:'error',correlation_id:message.request_id,payload:{code:'test_failure',message:error.message}});}});
  }
  receive(message){this.onmessage?.({data:JSON.stringify(message)});}
  close(){this.onclose?.({code:1000,reason:'test end',wasClean:true});}
 }
 global.WebSocket=Socket;
 const token='e30.'+Buffer.from(JSON.stringify({role:'browser',client_id:'example-browser',exp:Math.floor(Date.now()/1000)+300})).toString('base64url')+'.test-signature';
 const client=WSMServer('wss://test.invalid',token,{autoReconnect:false});
 const x=setup({wsm:client});
 try{
  await x.flow.start();assert.equal(x.created.length,0);
  Socket.current.receive({type:'event',connection_id:device.connectionID,payload:{event:'ecard.event',data:{id:1,event:'card_inserted',source:'pcsc',job:'',data:{reader:'Gemalto 0'}}}});
  await x.flow._tail;assert.equal(x.created.length,1);
  x.setUnsigned(true);await x.flow.servicesSaved();assert.equal(x.saved.length,1);
  const submitted=wire.filter(m=>m.payload?.args?.body?.request?.operation==='sign');
  assert.equal(submitted.length,1);assert.equal(submitted[0].target.connection_id,device.connectionID);
  assert.equal(submitted[0].payload.args.path,'/api/ecard/operations');
  assert.equal(submitted[0].payload.args.body.operation_id,x.plan.operationId);
  assert.ok(wire.some(m=>m.payload?.args?.path?.endsWith('/pin')));
 }finally{await x.flow.stop();client.disconnect();global.WebSocket=oldSocket;}
});

test('disconnect invalidates UI identity and stop restores existing WSM callbacks',async()=>{
 const x=setup();let events=0,disconnects=0;
 const beforeEvent=()=>events++,beforeDisconnect=()=>disconnects++;
 x.wsm.onEvent=beforeEvent;x.wsm.onDisconnect=beforeDisconnect;
 try{
  await x.flow.start();assert.ok(x.flow.card);
  x.wsm.event('card_removed',{reader:'Other 0'});assert.equal(events,1);
  x.wsm.onDisconnect({code:1006});assert.equal(disconnects,1);assert.equal(x.flow.card,null);assert.equal(x.flow.present,false);
 }finally{await x.flow.stop();}
 assert.equal(x.wsm.onEvent,beforeEvent);assert.equal(x.wsm.onDisconnect,beforeDisconnect);
});
