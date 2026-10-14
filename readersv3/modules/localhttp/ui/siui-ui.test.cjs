const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync(__dirname+'/app.js','utf8');
function fn(name) {
 const start=source.indexOf(`function ${name}(`);
 assert.ok(start>=0);
 return source.slice(source.slice(start-6,start)==='async '?start-6:start,source.indexOf('\n}',start)+2);
}
function scope(names,globals) {
 const ctx=vm.createContext(globals);
 vm.runInContext(names.map(fn).join('\n'),ctx);
 return ctx;
}
test('SIUI configuration deep links work locally and through WSM',()=>{
 for(const remote of [false,true]) for(const subview of ['siui','reader','wss']) {
  const path='/settings/'+subview;
  const ctx=scope(['currentUIPath','viewFromLocation','settingsSubViewFromLocation','pathForView'],{state:{settingsSubView:subview},isSIUIMode:()=>true,isESignatureMode:()=>false,window:{location:{pathname:remote?'/control/':path},WSM_REMOTE:remote?{route:()=>path}:null}});
  assert.equal(ctx.viewFromLocation(),'analytes');
  assert.equal(ctx.settingsSubViewFromLocation(),subview);
  assert.equal(ctx.pathForView('analytes'),path);
 }
});
test('polls pending jobs without resubmitting, stops on terminal result',async()=>{
 const calls=[], timers=[],result={classList:{toggle(){}},setAttribute(){}};
 let status='running';
 const ctx=scope(['showSIUIJob','setSIUIResult'],{siuiPollGeneration:0,state:{settingsSubView:'siui',currentView:'analytes'},api:async(path)=>{calls.push(path);return {job:{status}}},document:{getElementById:()=>result},setTimeout:fn=>timers.push(fn),showToast(){}});
 await ctx.showSIUIJob('job/1');
 assert.deepEqual(calls,['/api/siui/validations/job%2F1']);
 assert.equal(timers.length,1);
 status='completed';
 await timers.shift()();
 assert.equal(timers.length,0);
 assert.equal(JSON.parse(result.textContent).status,'completed');
});
test('late job response cannot replace a newer selection or logged-out view',async()=>{
 const pending=[],result={textContent:'',classList:{toggle(){}},setAttribute(){}};
 const ctx=scope(['showSIUIJob','setSIUIResult'],{siuiPollGeneration:0,state:{},api:()=>new Promise(resolve=>pending.push(resolve)),document:{getElementById:()=>result},setTimeout(){throw Error('unexpected polling')},showToast(){}});
 const first=ctx.showSIUIJob('first'),second=ctx.showSIUIJob('second');
 pending[1]({job:{id:'second',status:'completed'}});await second;
 pending[0]({job:{id:'first',status:'completed'}});await first;
 assert.equal(JSON.parse(result.textContent).id,'second');
 const last=ctx.showSIUIJob('last');ctx.siuiPollGeneration++;
 pending[2]({job:{id:'last',status:'completed'}});await last;
 assert.equal(JSON.parse(result.textContent).id,'second');
});
test('SIUI hides analyzer and results settings while retaining CNAS, server and WSM',()=>{
 const sections=[{hidden:false},{hidden:false}];
 const links=['siui','reader','wss','astm-specimens'].map(name=>({dataset:{settingsSubview:name},hidden:false}));
 const submenu={hidden:true};
 const ctx=scope(['applyBarcodeModeUI'],{state:{language:'ro'},els:{settingsSubmenu:submenu,navSublinks:links},isSIUIMode:()=>true,barcodeNavButton:()=>({}),document:{querySelectorAll:()=>sections,getElementById:()=>({})}});
 ctx.applyBarcodeModeUI();assert.ok(sections.every(s=>s.hidden));assert.equal(submenu.hidden,false);assert.deepEqual(links.map(l=>l.hidden),[false,false,false,true]);
});
test('download reconstruction uses every offset and rejects a stalled response',async()=>{
 const calls=[],saved=[];
 const ctx=scope(['downloadSIUIFile'],{Uint8Array,Blob,atob,api:async path=>{calls.push(path);return path.endsWith('=0')?{data_base64:btoa('abc'),next_offset:3}:{data_base64:btoa('de'),next_offset:5}},downloadSIUIBlob:(blob,name)=>saved.push({blob,name})});
 await ctx.downloadSIUIFile({path:'/api/siui/calls/id/file',size:5,filename:'result.zip',content_type:'application/zip'});
 assert.deepEqual(calls,['/api/siui/calls/id/file?offset=0','/api/siui/calls/id/file?offset=3']);assert.equal(await saved[0].blob.text(),'abcde');
 ctx.api=async()=>({data_base64:'',next_offset:0});await assert.rejects(()=>ctx.downloadSIUIFile({path:'/file',size:1}),/invalid/);
});

test('invoice OCSP preflight exposes token and rejects failed or incomplete authentication',async()=>{
 const text=fs.readFileSync(__dirname+'/siui-invoice.js','utf8');
 const ctx=vm.createContext({});vm.runInContext(text.replaceAll('export ',''),ctx);
 let completed=false;
 const pending=ctx.connectInvoiceOCSP(async(path,options)=>{assert.equal(path,'/api/siui/general/ocsp');assert.equal(options.method,'POST');await Promise.resolve();completed=true;return {authenticated:true,token:'test-token'}});
 assert.equal(completed,false);assert.equal((await pending).token,'test-token');
 await assert.rejects(()=>ctx.connectInvoiceOCSP(async()=>{throw Error('timeout')}),/timeout/);
 await assert.rejects(()=>ctx.connectInvoiceOCSP(async()=>({authenticated:true})),/token valid/);
});
test('invoice catalog preserves contract variants and filters date and category',()=>{
 const text=fs.readFileSync(__dirname+'/siui-invoice.js','utf8');
 const ctx=vm.createContext({});vm.runInContext(text.replaceAll('export ',''),ctx);
 const items=JSON.parse(fs.readFileSync(__dirname+'/../../siui/assets/invoice_items.json'));
 assert.equal(items.length,34);
 assert.equal(items.filter(i=>i.code==='HEMOGAMB').length,2);
 const para=ctx.availableInvoiceItems(items,'PARA','PAR','2026-10-13');
 assert.equal(para.filter(i=>i.code==='HEMOGAMB').length,1);assert.ok(para.some(i=>i.code==='PARACLLAB'));
 assert.ok(!ctx.availableInvoiceItems(items,'SPT','SPT','2025-12-31').some(i=>i.code==='SPZIPNS'));
 assert.ok(ctx.availableInvoiceItems(items,'SPT','SPT','2026-01-01').some(i=>i.code==='SPZIPNS'));
});

test('CNAS errors are announced in red and reset for a new request',()=>{
 const classes=new Set();const element={textContent:'',classList:{toggle(name,on){on?classes.add(name):classes.delete(name)}},setAttribute(name,value){this[name]=value}};
 const ctx=scope(['setSIUIResult'],{});
 ctx.setSIUIResult(element,'CNAS authentication HTTP 471',true);
 assert.ok(classes.has('siui-error'));assert.equal(element.role,'alert');assert.match(element.textContent,/471/);
 ctx.setSIUIResult(element,'Autentificare OCSP…');assert.ok(!classes.has('siui-error'));assert.equal(element.role,'status');
});

test('login refreshes certificates and enumeration errors do not reject login',async()=>{
 const calls=[], notices=[];
 let fail=false;
 const ctx=scope(['refreshSIUICertificatesAfterLogin'],{api:async(path,options)=>{calls.push([path,options.method]);if(fail)throw Error('token unavailable');},showToast:(...args)=>notices.push(args)});
 await ctx.refreshSIUICertificatesAfterLogin();
 assert.deepEqual(calls,[['/api/siui/certificates/refresh','POST']]);
 fail=true;
 await ctx.refreshSIUICertificatesAfterLogin();
 assert.equal(notices[0][1],'error');
 assert.match(notices[0][0],/token unavailable/);
 assert.match(fn('mountDashboard'),/await refreshSIUICertificatesAfterLogin\(\)/);
});
test('token PIN settings mask status, clear submitted secrets and preserve blank input',async()=>{
 const elements=new Map();
 const el=id=>{if(!elements.has(id))elements.set(id,{value:'',textContent:'',disabled:false,handlers:{},classList:{toggle(){}},setAttribute(){},addEventListener(name,fn){this.handlers[name]=fn;}});return elements.get(id);};
 const panel={querySelector:el};
 const form={elements:{certificate_thumbprint:el('certificate'),certificate_store:el('store')}};
 form.elements.certificate_thumbprint.value='ABC';form.elements.certificate_store.value='CurrentUser';
 const sent=[],notices=[];
 const ctx=scope(['setupSIUITokenPIN','setSIUIResult'],{api:async(path,options)=>{if(options){sent.push(JSON.parse(options.body));return {ok:true};}return {configured:true,blocked:false,manual_required:false};},bindAsyncClick:(button,fn)=>button.click=fn,showToast:(...args)=>notices.push(args)});
 await ctx.setupSIUITokenPIN(panel,form,{certificate_thumbprint:'ABC',certificate_store:'CurrentUser'});
 assert.equal(el('#siui-token-pin-state').textContent,'PIN salvat: <hidden>');
 assert.equal(el('#siui-token-pin-value').value,'');
 await el('#siui-token-pin-save').click();assert.equal(sent.length,0);
 el('#siui-token-pin-value').value='test-secret';await el('#siui-token-pin-save').click();
 assert.equal(sent[0].pin,'test-secret');assert.equal(el('#siui-token-pin-value').value,'');
 assert.ok(!el('#siui-token-pin-state').textContent.includes('test-secret'));
 el('#siui-token-pin-value').value='manual-secret';await el('#siui-token-pin-manual').click();assert.equal(sent[1].manual,true);assert.equal(el('#siui-token-pin-value').value,'');
 form.elements.certificate_thumbprint.value='OTHER';form.elements.certificate_thumbprint.handlers.change();assert.equal(el('#siui-token-pin-section').disabled,true);
});
