const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const SIUIBridge=require('./siui-bridge.js');
const device={connectionID:'conn-18',clientID:'siui',equipmentID:'18',clientType:'siui-bridge'};
function mock(handler=()=>({ok:true})) {
 const wsm={isConnected:true,clients:[device],calls:[],async getClientsByType(type){assert.equal(type,'siui-bridge');return this.clients.length?this.clients:null;},async callAPI(client,path,options){this.calls.push({client,path,options});return {status:200,body:await handler(path,options)};}};
 return wsm;
}
const root=path.resolve(__dirname,'../../../..');
test('catalog covers every real SOAP operation and every explicit SIUI auxiliary route',()=>{
 const actual=JSON.parse(fs.readFileSync(path.join(root,'readersv3/modules/siui/assets/operations.json'))).map(({group,name,parameters,mutating})=>({group,name,parameters,mutating}));
 assert.deepEqual(SIUIBridge.operations,actual);assert.equal(actual.length,34);
 const paths=new Set(SIUIBridge.endpoints.map(e=>e.path));
 for(const file of ['module.go','service_jobs.go','invoice.go']) {
  const source=fs.readFileSync(path.join(root,'readersv3/modules/siui',file),'utf8');
  for(const match of source.matchAll(/route == "([^"]+)"/g))assert.ok(paths.has(match[1]),'missing auxiliary path '+match[1]);
 }
});
test('discovery returns null, rejects ambiguity and pins chosen equipment',async()=>{
 const wsm=mock();wsm.clients=[];assert.equal(await SIUIBridge.create(wsm),null);
 wsm.clients=[device,{...device,connectionID:'other',equipmentID:'19'}];
 await assert.rejects(SIUIBridge.create(wsm),{code:'ambiguous_bridge'});
 const bridge=await SIUIBridge.create(wsm,{equipmentID:'18'});assert.equal(bridge.isConnected,true);
 wsm.clients=[{...device,connectionID:'reconnected'}];await bridge.getStatus();assert.equal(wsm.calls[0].client.connectionID,'reconnected');
 wsm.clients=[{...device,equipmentID:'19'}];await assert.rejects(bridge.getStatus(),{code:'bridge_offline'});
 assert.equal(wsm.calls.length,1);wsm.isConnected=false;assert.equal(bridge.isConnected,false);
 await assert.rejects(bridge.getStatus(),{code:'wsm_disconnected'});
});
test('every auxiliary method uses callAPI only, preserving method/body/query',async()=>{
 const wsm=mock(),bridge=await SIUIBridge.create(wsm);
 for(const endpoint of SIUIBridge.endpoints){
  const body={fixture:'test'};const options={query:{store:'CurrentUser'}};
  if(endpoint.method==='GET')await bridge[endpoint.name](options);else await bridge[endpoint.name](body,options);
  const call=wsm.calls.at(-1);assert.equal(call.path,'/api/siui/'+endpoint.path);assert.equal(call.options.method,endpoint.method);
  assert.deepEqual(call.options.body,endpoint.method==='GET'?undefined:body);assert.deepEqual(call.options.query,options.query);
 }
 await bridge.getCall('call-id');assert.equal(wsm.calls.at(-1).path,'/api/siui/calls/call-id');
 await bridge.getValidation('job-id');assert.equal(wsm.calls.at(-1).path,'/api/siui/validations/job-id');
 await bridge.getCallFileChunk('call-id',256);assert.deepEqual(wsm.calls.at(-1).options.query,{offset:256});
});
test('all named operations submit correct parameters once and wait for completion',async()=>{
 const wsm=mock((url,opts)=>opts.method==='POST'?{call:{id:'call-1',status:'queued'}}:{call:{id:'call-1',status:'completed',result:{values:['result XML']}}});
 const bridge=await SIUIBridge.create(wsm,{pollInterval:1});
 for(const op of SIUIBridge.operations){
  const parameters=Object.fromEntries(op.parameters.map(p=>[p.name,'test']));
  const result=await bridge[op.group][op.name](parameters,{operationId:'stable-'+op.name});
  assert.equal(typeof bridge[op.name],'function');assert.equal(result.status,'completed');
  const request=wsm.calls.at(-2);assert.equal(request.path,'/api/siui/'+op.group+'/'+op.name);assert.deepEqual(request.options.body,{operation_id:'stable-'+op.name,parameters});
 }
 assert.equal(wsm.calls.filter(c=>c.options.method==='POST').length,34);
});
test('invalid operation input never submits and wait:false preserves queued state',async()=>{
 const wsm=mock(()=>({call:{id:'id',status:'queued'}}));const bridge=await SIUIBridge.create(wsm);
 await assert.rejects(bridge.getInsured({pid:'x',requestDate:'x'}),{code:'operation_id_required'});
 await assert.rejects(bridge.getInsured({pid:123,requestDate:'x'},{operationId:'x'}),{code:'invalid_parameters'});
 assert.equal(wsm.calls.length,0);
 const call=await bridge.getInsured({pid:'x',requestDate:'x'},{operationId:'x',wait:false});assert.equal(call.status,'queued');assert.equal(wsm.calls.length,1);
});
test('failed, unknown and deadline reject without retrying a mutation',async()=>{
 for(const status of ['failed','unknown']){
  const wsm=mock((url,opts)=>opts.method==='POST'?{call:{id:'id',status:'queued'}}:{call:{id:'id',status,error:'CNAS unavailable'}});
  const bridge=await SIUIBridge.create(wsm,{pollInterval:1});
  await assert.rejects(bridge.sendReport({reportType:'CLIN',reportXML:'xml'},{operationId:'report'}),e=>e.code==='siui_'+status&&e.callId==='id'&&e.operationId==='report');
  assert.equal(wsm.calls.filter(c=>c.options.method==='POST').length,1);
 }
 const bridge=await SIUIBridge.create(mock(()=>({call:{id:'id',status:'running'}})),{pollInterval:2});
 await assert.rejects(bridge.waitCall('id',{timeout:5}),{code:'wait_timeout'});
});
test('local abort stops waiting and ignores late results without disconnecting WSM',async()=>{
 let finish;const wsm=mock(()=>new Promise(resolve=>{finish=resolve;}));const bridge=await SIUIBridge.create(wsm);
 const controller=new AbortController();const request=bridge.checkInsured({mode:'cnas',cnp:'test',date:'2026-10-16'},{signal:controller.signal});
 const rejected=assert.rejects(request,{name:'AbortError',code:'aborted'});
 await new Promise(resolve=>setImmediate(resolve));controller.abort();await rejected;
 finish({ok:true,result:{insured:true}});assert.equal(wsm.isConnected,true);
 const already=new AbortController();already.abort();await assert.rejects(bridge.getStatus({signal:already.signal}),{code:'aborted'});assert.equal(wsm.calls.length,1);
});
test('polling can be aborted in delay and deadline bounds an unresponsive read',async()=>{
 const wsm=mock(()=>({call:{id:'id',status:'running'}}));const bridge=await SIUIBridge.create(wsm,{pollInterval:10000});
 const controller=new AbortController();let progress=0;
 const waiting=bridge.waitCall('id',{initial:{id:'id',status:'running'},signal:controller.signal,onProgress:()=>progress++});
 const rejected=assert.rejects(waiting,{code:'aborted'});await new Promise(resolve=>setImmediate(resolve));controller.abort();await rejected;
 assert.equal(wsm.calls.length,0);assert.equal(progress,1);
 wsm.callAPI=()=>new Promise(()=>{});await assert.rejects(bridge.waitCall('id',{timeout:5}),{code:'wait_timeout'});
});
test('chunked download preserves bytes and rejects nonprogressing offsets',async()=>{
 const wsm=mock((url,opts)=>opts.query.offset===0?{offset:0,next_offset:2,eof:false,data_base64:'AAE='}:{offset:2,next_offset:3,eof:true,data_base64:'Ag=='});
 const bridge=await SIUIBridge.create(wsm);assert.deepEqual(Array.from(await bridge.downloadCallFile('id')),[0,1,2]);
 await assert.rejects(bridge.downloadCallFile('id',{maxBytes:1}),{code:'file_too_large'});
 wsm.callAPI=async()=>({body:{offset:0,next_offset:0,eof:false,data_base64:''}});
 await assert.rejects(bridge.downloadCallFile('id'),{code:'invalid_chunk'});
});
