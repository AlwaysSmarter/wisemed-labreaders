const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
let JSDOM;try{({JSDOM}=require('jsdom'))}catch{}
const source=fs.readFileSync(__dirname+'/app.js','utf8');
const formSource=fs.readFileSync(__dirname+'/docmed-forms.js','utf8');
const models=JSON.parse(fs.readFileSync(__dirname+'/../../siui/assets/docmed_forms.json'));
const examples=JSON.parse(fs.readFileSync(__dirname+'/../../siui/assets/docmed_examples.json'));
const ops=JSON.parse(fs.readFileSync(__dirname+'/../../siui/assets/docmed_operations.json'));
function fn(name){const start=source.indexOf(`function ${name}(`);assert.ok(start>=0);return source.slice(source.slice(start-6,start)==='async '?start-6:start,source.indexOf('\n}',start)+2)}
function fieldsFor(window,model,xml){
 const root=new window.DOMParser().parseFromString(xml,'application/xml').documentElement;
 const convert=(model,n)=>{if(model.value)return n.textContent;const values={};for(const a of n.attributes)if(a.name!=='xmlns')values[a.localName]=a.value;for(const child of model.children||[]){const found=[...n.children].filter(n=>n.localName===child.name);if(found.length)values[child.name]=child.max===1?convert(child,found[0]):found.map(n=>convert(child,n));}return values;};return convert(model,root);
}
function fill(document,model,fields,path=model.name){
 if(model.value)return;
 for(const spec of model.attributes||[]){if(fields[spec.name]===undefined)continue;const input=document.querySelector('[data-docmed-path='+JSON.stringify(path+'.'+spec.name)+']');assert.ok(input,path+'.'+spec.name);input.value=fields[spec.name];}
 for(const child of model.children||[]){if(fields[child.name]===undefined)continue;const list=child.max===1?[fields[child.name]]:fields[child.name];for(const [i,value]of list.entries())fill(document,child,value,path+'.'+child.name+(child.max!==1?'['+i+']':''));}
}
const domOptions={skip:!JSDOM&&'Install jsdom to run DOM regression tests'};
test('DocMED uses variable forms for all operations and posts fields without XML',domOptions,async()=>{
 const dom=new JSDOM('<div id="view-debug"></div>'),{window}=dom,document=window.document;
 const catalog={operations:[{name:'getPhysicianCatalogues',group:'general',service:'SiuiWS',parameters:[]},...ops.map(op=>({...op,form:models[op.name]}))],insurance_houses:[{code:'CAS-DJ',name:'Dolj'}],docmed_defaults:{providerCode:'7991962',providerInsuranceHouse:'CAS-DJ',insuranceHouseCode:'CAS-DJ'}};
 const submitted=[],downloads=[];
 const ctx=vm.createContext({document,window,HTMLButtonElement:window.HTMLButtonElement,HTMLElement:window.HTMLElement,FormData:window.FormData,Blob,crypto:require('node:crypto').webcrypto,state:{currentView:'debug'},siuiDebugGeneration:0,api:async(path,options)=>{
  if(path.endsWith('/catalog'))return catalog;
  if(options?.method==='POST'){submitted.push({path,body:JSON.parse(options.body)});return {call:{id:'fixture'}};}
  if(path.includes('/calls/'))return {call:{status:'completed',result:{file:{path:'/xml',size:10},docmed:{attachment:{path:'/zip?docmed_attachment=true',size:10}}}}};
  throw Error(path);
 },escapeHtml:s=>String(s).replaceAll('&','&amp;').replaceAll('"','&quot;').replaceAll('<','&lt;').replaceAll('>','&gt;'),downloadSIUIFile:async file=>downloads.push(file.path),downloadSIUIBlob(){},showToast(){},setTimeout,renderInvoiceDashboard(){}});
 vm.runInContext(formSource.replaceAll('export ','')+'\n'+['loadSIUIDebug','bindAsyncClick','bindAsyncSubmit','resolveActionButton','setButtonLoading','setSIUIResult'].map(fn).join('\n'),ctx);
 await ctx.loadSIUIDebug();document.querySelector('#siui-debug-docmed').click();
 const select=document.querySelector('#siui-operation');assert.equal(select.value,'GetMedicalDocumentXml');
 for(const op of ops){select.value=op.name;select.dispatchEvent(new window.Event('change'));const form=document.querySelector('#siui-operation-form form');assert.equal(form.querySelector('[name=requestXml]'),null,op.name);assert.ok(form.querySelector('[data-docmed-path]'),op.name);assert.match(form.parentElement.textContent,new RegExp('/api/siui/docmed/'+op.name));}
 select.value='GetDocumentSeries';select.dispatchEvent(new window.Event('change'));
 assert.equal(document.querySelector('[name=providerCode]').value,'7991962');assert.equal(document.querySelector('[name=providerInsuranceHouse]').value,'CAS-DJ');
 const values={providerCode:'7991962',providerName:'HITMED & Test',providerInsuranceHouse:'CAS-DJ',getDocumentSeries:{insuranceHouseCode:'CAS-DJ',documentType:'ML',validFrom:'2026-11-02',validTo:'2026-11-02'}};
 fill(document,models.GetDocumentSeries,values);
 let form=document.querySelector('#siui-operation-form form');form.dispatchEvent(new window.Event('submit',{cancelable:true}));await new Promise(resolve=>setImmediate(resolve));
 assert.equal(submitted.length,1);assert.equal(submitted[0].path,'/api/siui/docmed/GetDocumentSeries');assert.deepEqual(submitted[0].body.fields,values);assert.equal(submitted[0].body.parameters,undefined);
 select.value='GetMedicalDocument';select.dispatchEvent(new window.Event('change'));assert.equal(document.querySelector('[name=providerName]').value,'HITMED & Test');
 fill(document,models.GetMedicalDocument,fieldsFor(window,models.GetMedicalDocument,examples.GetMedicalDocument));
 form=document.querySelector('#siui-operation-form form');form.dispatchEvent(new window.Event('submit',{cancelable:true}));await new Promise(resolve=>setImmediate(resolve));
 const buttons=[...document.querySelectorAll('#siui-download-area button')];assert.equal(buttons.length,2);for(const button of buttons)button.click();await new Promise(resolve=>setImmediate(resolve));assert.deepEqual(downloads,['/xml','/zip?docmed_attachment=true']);
 // Switching out of DocMED must restore the legacy parameter transport.
 document.querySelector('#siui-debug-general').click();form=document.querySelector('#siui-operation-form form');form.dispatchEvent(new window.Event('submit',{cancelable:true}));await new Promise(resolve=>setImmediate(resolve));assert.deepEqual(submitted.at(-1).body.parameters,{});assert.equal(submitted.at(-1).body.fields,undefined);
 dom.window.close();
});
test('DocMED optional sections, repeated investigations and PDF upload are variable inputs',domOptions,async()=>{
 const dom=new JSDOM('<form><div id="fields"></div></form>'),{window}=dom,document=window.document;
 const ctx=vm.createContext({document});vm.runInContext(formSource.replaceAll('export ',''),ctx);
 const container=document.querySelector('#fields'),model=models.LaboratoryMFReferral;
 const adapter=ctx.renderDocMedForm(container,model);
 const values=fieldsFor(window,model,examples.LaboratoryMFReferral);fill(document,model,values);
 const first=document.querySelector('[data-docmed-path$="investigation[0].code"]');assert.ok(first);
 const group=first.closest('fieldset');group.querySelector(':scope > button').click();
 const second=document.querySelector('[data-docmed-path$="investigation[1].code"]');assert.ok(second);second.value='SECOND';document.querySelector('[data-docmed-path$="investigation[1].no"]').value='2';
 let fields=await adapter.read();assert.equal(fields.laboratoryMFReferral.investigation.length,2);assert.equal(fields.laboratoryMFReferral.investigation[1].code,'SECOND');
 second.closest('.docmed-form-row').querySelector(':scope > button').click();fields=await adapter.read();assert.equal(fields.laboratoryMFReferral.investigation.length,1);
 const attachmentGroup=[...container.querySelectorAll('fieldset')].find(el=>el.firstElementChild.textContent==='Document PDF sau ZIP');assert.ok(attachmentGroup);attachmentGroup.querySelector(':scope > button').click();
 const input=attachmentGroup.querySelector('input[type=file]');assert.ok(input);const pdf='%PDF-1.4 synthetic';Object.defineProperty(input,'files',{value:[new window.File([pdf],'referral.pdf',{type:'application/pdf'})]});
 fields=await adapter.read();assert.equal(fields.laboratoryMFReferral.attachedFile.filename,'referral.pdf');assert.equal(atob(fields.laboratoryMFReferral.attachedFile.data_base64),pdf);
 dom.window.close();
});

test('DocMED document types, CNAS enums and CAS values use labelled dropdowns',domOptions,()=>{
 const dom=new JSDOM('<div id="fields"></div>'),document=dom.window.document;
 const ctx=vm.createContext({document});vm.runInContext(formSource.replaceAll('export ',''),ctx);
 const houses=[{code:'CAS-DJ',name:'Dolj'},{code:'CAS-B',name:'București'}];
 for(const name of ['GetDocumentSeries','GenDocumentSeries','LaboratoryMFReferral','MedicalLetter']){
  ctx.renderDocMedForm(document.querySelector('#fields'),models[name],{providerInsuranceHouse:'CAS-DJ',insuranceHouseCode:'CAS-DJ'},houses);
  const check=(model,path=model.name)=>{
   for(const field of model.attributes||[]){const control=document.querySelector('[data-docmed-path='+JSON.stringify(path+'.'+field.name)+']');if(!control)continue;
    if(field.options?.length){assert.equal(control.tagName,'SELECT',field.name);for(const option of field.options)assert.ok([...control.options].some(o=>o.value===option.value&&o.textContent.includes(option.label)),field.name+': '+option.value);}
    if(/insuranceHouse/i.test(field.name)){assert.equal(control.tagName,'SELECT');assert.ok([...control.options].some(o=>o.value==='CAS-DJ'&&o.textContent.includes('Dolj')));}
   }
   for(const child of model.children||[]){if(child.min>0)check(child,path+'.'+child.name+(child.max!==1?'[0]':''));}
  };check(models[name]);
 }
 ctx.renderDocMedForm(document.querySelector('#fields'),models.GetDocumentSeries,{},houses);
 const type=document.querySelector('[name=documentType]');assert.equal(type.tagName,'SELECT');assert.ok([...type.options].some(o=>o.value==='ML'&&o.textContent.includes('Scrisoare medicală')));
 dom.window.close();
});
