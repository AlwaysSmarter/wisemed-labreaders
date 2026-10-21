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
test('catalog matches all registered SOAP operations',()=>{
 const actual=JSON.parse(fs.readFileSync(path.join(root,'readersv3/modules/siui/assets/operations.json'))).map(({group,name,parameters,mutating})=>({group,name,parameters,mutating}));
 assert.deepEqual(SIUIBridge.operations,actual);assert.equal(actual.length,40);
});
test('catalog covers every explicit SIUI auxiliary route',()=>{
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
 assert.equal(wsm.calls.filter(c=>c.options.method==='POST').length,40);
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
test('bounded reconnect retries stuck ready, deduplicates concurrent callers and recovers',async()=>{
 const wsm=mock();wsm.isConnected=false;wsm.ready=new Promise(()=>{});
 let attempts=0;
 wsm.disconnect=()=>{wsm.isConnected=false;};
 wsm.reconnect=()=>{attempts++;if(attempts<3)return new Promise(()=>{});wsm.isConnected=true;return Promise.resolve(wsm);};
 const bridge=SIUIBridge(wsm,{connectAttempts:3,connectTimeout:5});
 await Promise.all([bridge.ready,bridge.getStatus(),bridge.getStatus()]);
 assert.equal(attempts,3);assert.equal(wsm.calls.length,2);
 wsm.isConnected=false;wsm.reconnect=()=>{attempts++;return new Promise(()=>{});};
 await assert.rejects(bridge.getStatus(),e=>e.code==='wsm_disconnected'&&e.attempts===3&&e.cause.code==='connect_timeout');
 assert.equal(attempts,6);
});
test('factory initializes absent WSM and hanging factory is bounded',async()=>{
 const bridge=await SIUIBridge.create(null,{wsmFactory:async()=>mock(),connectTimeout:5});
 assert.ok(await bridge.getStatus());
 await assert.rejects(SIUIBridge.create(null,{wsmFactory:()=>new Promise(()=>{}),connectTimeout:5,connectAttempts:2}),{code:'wsm_disconnected',attempts:2});
});
test('high-level pipeline forwards callback, processing options and returns final bytes/JSON',async()=>{
 const payload=JSON.stringify({name:'insured',attributes:[{name:'cid',value:'40172426789613933080'}]});
 const wsm=mock((url,opts)=>{
  if(opts.method==='POST'){assert.equal(opts.body.callwmapi.lt,'test-token');return {call:{id:'id',status:'queued'}};}
  if(url.endsWith('/file')){
   assert.deepEqual(opts.query,{offset:0,unzip:'true',entry:'report.xml',format:'json'});
   return {offset:0,next_offset:Buffer.byteLength(payload),eof:true,data_base64:Buffer.from(payload).toString('base64')};
  }
  return {call:{id:'id',status:'completed',result:{file:{filename:'id.zip'}}}};
 });
 const bridge=await SIUIBridge.create(wsm,{pollInterval:1});
 const call=await bridge.getInsured({pid:'test',requestDate:'test'},{
  operationId:'op',callwmapi:{lt:'test-token'},file:{unzip:true,entry:'report.xml',format:'json'},
  processResult:async call=>call.download.data.name
 });
 assert.equal(call.download.data.attributes[0].value,'40172426789613933080');
 assert.equal(call.processed,'insured');assert.equal(wsm.calls.filter(c=>c.options.method==='POST').length,1);
});
test('download timeout and processing errors keep operation identity, no resubmission',async()=>{
 const wsm=mock((url,opts)=>url.endsWith('/file')?new Promise(()=>{}):{call:{id:'id',status:'completed',result:{file:{}}}});
 const bridge=await SIUIBridge.create(wsm);
 await assert.rejects(bridge.getPhysicianCatalogues({}, {operationId:'op',downloadTimeout:5}),e=>e.code==='download_timeout'&&e.callId==='id'&&e.operationId==='op');
 assert.equal(wsm.calls.filter(c=>c.options.method==='POST').length,1);
});
test('published WiseMED workflows send matching callback contracts and stable validation payload',async()=>{
 const workflows=require('../../../examples/browser/siui-workflows.js');
 const calls=[];
 const siui={getInsured:async(...args)=>{calls.push(args);return {};},getProviderInfo:async(...args)=>{calls.push(args);return {};},validateReport:async(...args)=>{calls.push(args);return {};}};
 await workflows.checkInsurance(siui,{cnp:'1000000000000',lt:'token',operationId:'insured',requestDate:'2026-10-18'});
 assert.equal(calls.at(-1)[1].callwmapi.lt,'token');
 await workflows.downloadPersonalisation(siui,{partnerCategory:'CLIN',start:'x',stop:'y',uic:'1',operationId:'personalisation'});
 assert.equal(calls.at(-1)[1].callwmapi.file.unzip,true);
 assert.equal(calls.at(-1)[1].callwmapi.body.transform_result[0].from_field,'file_json');
 const args={fileId:'123',reportXml:'<xml/>',operationId:'validate',requestedAt:'2026-10-18'};
 await workflows.validate72h(siui,args);
 const first=calls.at(-1);
 await workflows.validate72h(siui,args);
 assert.deepEqual(calls.at(-1),first);
 assert.equal(first[1].callwmapi.apipath,'/file/123/');
 assert.equal(first[0].fileId,undefined);
 await assert.rejects(workflows.validate72h(siui,{...args,fileId:'../invalid'}));
});
test('safe singleton resolves success, offline, validation and timeout errors without rejection',async()=>{
 const safe=SIUIBridge.createSafeClient();
 const wsm=mock(()=>({call:{id:'id',status:'completed',result:{values:['ok']}}}));
 assert.equal(safe.configure({getWSM:()=>wsm,equipmentID:'18',totalTimeout:100}).success,true);
 const good=await safe.general.getInsured({pid:'test',requestDate:'test'},{operationId:'stable'});
 assert.equal(good.success,true);assert.equal(good.data.status,'completed');
 const invalid=await safe.general.getInsured({pid:'test'});
 assert.equal(invalid.success,false);assert.equal(invalid.code,'operation_id_required');
 wsm.clients=[];
 const offline=await safe.getStatus();
 assert.equal(offline.success,false);assert.equal(offline.code,'bridge_offline');
 wsm.clients=[device];wsm.callAPI=()=>new Promise(()=>{});
 const timed=await safe.getStatus({totalTimeout:5});
 assert.equal(timed.success,false);assert.equal(timed.code,'operation_timeout');
 assert.equal((await safe.getStatus({totalTimeout:0})).success,false);
});
test('safe singleton initializes lazily, recovers after missing WSM, catches user callback throws',async()=>{
 const safe=SIUIBridge.createSafeClient();let current;
 safe.configure({getWSM:()=>current,connectAttempts:1,connectTimeout:5});
 assert.equal((await safe.getStatus()).success,false);
 current=mock(()=>({call:{id:'id',status:'completed'}}));
 const result=await safe.getPhysicianCatalogues({}, {operationId:'stable',onProgress:()=>{throw new Error('callback failure');}});
 assert.equal(result.success,false);assert.equal(result.error,'callback failure');
 assert.equal((await safe.getStatus()).success,true);
});
test('browser singleton works with global lexical objWSM without configuration, including late initialization',async()=>{
 const vm=require('node:vm');
 const context=vm.createContext({setTimeout,clearTimeout,AbortController,TextDecoder,atob});
 vm.runInContext('let objWSM;',context);
 vm.runInContext(fs.readFileSync(path.join(__dirname,'siui-bridge.js'),'utf8'),context);
 const missing=await vm.runInContext('objWSMSIUI.getStatus()',context);
 assert.equal(missing.success,false);
 context.peer=mock(()=>({configured:true}));
 vm.runInContext('objWSM = peer;',context);
 const result=await vm.runInContext('objWSMSIUI.getStatus()',context);
 assert.equal(result.success,true);assert.equal(result.data.configured,true);
 context.peer.isConnected=false;
 context.peer.reconnect=async function(){this.isConnected=true;};
 const reconnected=await vm.runInContext('objWSMSIUI.getStatus()',context);
 assert.equal(reconnected.success,true);
});
test('optional page tuning validates atomically, replaces WSM and resets defaults',async()=>{
 const safe=SIUIBridge.createSafeClient(), first=mock(),second=mock(()=>({source:'second'}));
 assert.equal(safe.setWSM(first).success,true);
 assert.equal(safe.setTimeouts({connectTimeout:2500,totalTimeout:9000}).success,true);
 assert.equal(safe.setReconnectOptions({connectAttempts:2}).success,true);
 assert.equal(safe.setPollingInterval(250).success,true);
 assert.equal(safe.selectBridge({equipmentID:'18'}).success,true);
 const before=safe.getOptions().data;
 assert.equal(safe.setTimeouts({totalTimeout:0}).success,false);
 assert.equal(safe.setReconnectOptions({connectAttempts:1.5}).success,false);
 assert.equal(safe.setTimeouts({wrongOption:1}).success,false);
 assert.deepEqual(safe.getOptions().data,before);
 assert.equal((await safe.getStatus()).success,true);
 assert.equal(safe.setWSM(second).success,true);
 assert.equal((await safe.getStatus()).data.source,'second');
 assert.equal(safe.getOptions().data.equipmentID,undefined);
 assert.equal(safe.resetOptions().success,true);
 assert.equal(safe.getOptions().data.totalTimeout,300000);
 assert.equal(first.isConnected,true);assert.equal(second.isConnected,true);
});
test('status events correlate calls and isolate failing UI handlers',async()=>{
 const safe=SIUIBridge.createSafeClient(),events=[],local=[];
 safe.setWSM(mock(()=>({call:{id:'id',status:'completed',result:{values:['ok']}}})));
 safe.setStatusHandler(event=>{events.push(event);throw new Error('UI failed');});
 const result=await safe.getPhysicianCatalogues({}, {operationId:'stable',onStatus:event=>{local.push(event);return Promise.reject(new Error('UI async failed'));}});
 assert.equal(result.success,true);
 assert.equal(events[0].stage,'starting');assert.equal(events.at(-1).stage,'completed');
 assert.ok(events.some(e=>e.stage==='waiting'));
 assert.equal(new Set(events.map(e=>e.requestId)).size,1);
 assert.equal(events.length,local.length);
 assert.equal(events.at(-1).percent,100);
 assert.equal(safe.setStatusHandler(null).success,true);
});
test('status callback emits one terminal timeout and suppresses late progress',async()=>{
 const safe=SIUIBridge.createSafeClient(),events=[];
 let finish;
 safe.setWSM(mock(()=>new Promise(resolve=>{finish=resolve;})));
 safe.setStatusHandler(e=>events.push(e));
 const result=await safe.getStatus({totalTimeout:5});
 assert.equal(result.success,false);assert.equal(events.at(-1).stage,'error');
 const count=events.length;finish({ok:true});
 await new Promise(resolve=>setImmediate(resolve));
 assert.equal(events.length,count);
});
test('singleton defaults to first SIUI bridge and pins it until explicit reselection',async()=>{
 const safe=SIUIBridge.createSafeClient(),wsm=mock();
 const other={...device,equipmentID:'19',clientID:'other',connectionID:'conn-19'};
 wsm.clients=[device,other];safe.setWSM(wsm);
 assert.equal((await safe.getStatus()).success,true);
 assert.equal(wsm.calls.at(-1).client.equipmentID,'18');
 wsm.clients=[other,device];
 assert.equal((await safe.getStatus()).success,true);
 assert.equal(wsm.calls.at(-1).client.equipmentID,'18');
 wsm.clients=[other];
 assert.equal((await safe.getStatus()).code,'bridge_offline');
 safe.selectBridge({});
 assert.equal((await safe.getStatus()).success,true);
 assert.equal(wsm.calls.at(-1).client.equipmentID,'19');
});

test('prescription series and dispensing methods use the safe singleton and preserve receipts',async()=>{
 const values=['series-one','series-two',''];
 const wsm=mock((url,opts)=>opts.method==='POST'?{call:{id:'prescription-call',status:'queued'}}:{call:{id:'prescription-call',status:'completed',result:{values}}});
 const safe=SIUIBridge.createSafeClient();
 assert.equal(safe.configure({getWSM:()=>wsm,pollInterval:1}).success,true);
 for(const name of ['generatePrescriptionSeries','getPrescriptionSeriesInfo','processIssuedPrescription','getReleasedPrescription','cancelReleasedPrescription','updateInvoices']){
  const op=SIUIBridge.operations.find(o=>o.name===name);
  const parameters=Object.fromEntries(op.parameters.map(p=>[p.name,p.type==='boolean'?'false':p.type==='int'?'1':'test']));
  const result=await safe[op.group][name](parameters,{operationId:'prescription-'+name});
  assert.equal(result.success,true);assert.deepEqual(result.data.result.values,values);
  assert.equal(wsm.calls.at(-2).path,'/api/siui/'+op.group+'/'+name);
  assert.deepEqual(wsm.calls.at(-2).options.body.parameters,parameters);
 }
 const rejected=await safe.general.generatePrescriptionSeries({categoryCode:'MF',orgUnitCode:'CAS-TEST',uic:'TEST',subUnitCode:'TEST',validFrom:'2026-10-22T00:00:00+03:00',isOnLine:false,quantity:'1'},{operationId:'invalid-native-boolean'});
 assert.equal(rejected.success,false);assert.equal(rejected.code,'invalid_parameters');
});

test('series null subUnitCode reaches WSM and downloaded file reaches caller',async()=>{
 const payload='<series/>';
 const wsm=mock((url,opts)=>{
  if(url.includes('/file'))return {offset:0,data_base64:Buffer.from(payload).toString('base64'),next_offset:payload.length,eof:true};
  if(opts.method==='POST')return {call:{id:'nullable',status:'completed',result:{values:['/files/series.xml',String(payload.length)],file:{size:payload.length}}}};
  throw Error('unexpected request');
 });
 const safe=SIUIBridge.createSafeClient();safe.configure({getWSM:()=>wsm});
 const parameters={categoryCode:'CLIN',orgUnitCode:'CAS-DJ',uic:'TEST',subUnitCode:null};
 for(const name of ['getPrescriptionSeriesInfo','generatePrescriptionSeries']){
  const args=name==='generatePrescriptionSeries'?{...parameters,validFrom:'2026-10-22T00:00:00+03:00',isOnLine:'false',quantity:'1'}:parameters;
  const result=await safe.general[name](args,{operationId:'null-'+name});
  assert.equal(result.success,true,JSON.stringify(result));assert.equal(result.data.download.bytes.length,payload.length);
  assert.equal(new TextDecoder().decode(result.data.download.data),payload);
  assert.equal(wsm.calls.at(-2).options.body.parameters.subUnitCode,null);
 }
 for(const args of [{...parameters,uic:null},{categoryCode:'CLIN',orgUnitCode:'CAS-DJ',uic:'TEST'},{...parameters,subUnitCode:12}]){
  const before=wsm.calls.length;
  const result=await safe.general.getPrescriptionSeriesInfo(args,{operationId:'bad-null'});
  assert.equal(result.code,'invalid_parameters');assert.equal(wsm.calls.length,before);
 }
});
