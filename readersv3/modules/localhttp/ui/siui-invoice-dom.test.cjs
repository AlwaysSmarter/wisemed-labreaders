const {test}=require('node:test');const assert=require('node:assert/strict');const fs=require('node:fs');const vm=require('node:vm');let JSDOM;try{({JSDOM}=require('jsdom'))}catch{}
const root=require('node:path').resolve(__dirname,'../../..');
const source=fs.readFileSync(root+'/modules/localhttp/ui/siui-invoice.js','utf8');const app=fs.readFileSync(root+'/modules/localhttp/ui/app.js','utf8');
const start=app.indexOf('function setButtonLoading(');const loader=app.slice(start,app.indexOf('\n}',start)+2);
for(const failure of ['ocsp','package','poll','failed','unknown','success'])test('send loader and nearby result: '+failure,{skip:!JSDOM&&'Install jsdom to run DOM regression tests'},async()=>{
 const dom=new JSDOM('<div id="panel"></div>');const document=dom.window.document,panel=document.querySelector('#panel');
 let unblock;const gate=new Promise(r=>unblock=r);const calls=[];
 const api=async(path,opts={})=>{
 calls.push(path);
 if(path.endsWith('/model'))return {fields:JSON.parse(fs.readFileSync(root+'/modules/siui/assets/invoice_fields.json')),partner_categories:['PARA'],insurance_houses:[{code:'CAS-DJ',name:'Dolj'}],invoice_items:JSON.parse(fs.readFileSync(root+'/modules/siui/assets/invoice_items.json'))};
 if(path.endsWith('/preview')){const data=JSON.parse(opts.body);Object.assign(data.header,{totalAmmount:'27937.65',vatAmmount:'0.00'});data.items[0].ammount='27937.65';return {data,invoice_xml:'<invoice/>',barcode_xml:'<F/>',barcode_png_base64:'AA=='}}
 if(path.endsWith('/ocsp')&&!opts.method)return {debug:{url:'https://www.siui.ro/OCSP/validator',port:'443'}};
 if(path.endsWith('/ocsp')){await gate;if(failure==='ocsp')throw Error('Server: https://www.siui.ro/OCSP/validator\nPort: 443\ntimeout');return {authenticated:true,token:'test'}}
 if(path.endsWith('/package')){if(failure==='package')throw Error('Signing failed');return {invoiceFile:'test'}}
 if(path.endsWith('/sendEInvoice'))return {call:{id:'test'}};
 if(path.includes('/calls/')){if(failure==='poll')throw Error('Polling failed');return {call:{status:failure==='failed'?'failed':failure==='unknown'?'unknown':'completed',error:failure==='failed'?'Server: https://www.siui.ro/invoice\nPort: 444\nSOAP failure':undefined}}}
 throw Error(path);
 };
 const ctx=vm.createContext({document,crypto:require('node:crypto').webcrypto,console,Blob,FormData:dom.window.FormData,setTimeout,window:dom.window});vm.runInContext(loader+'\n'+source.replaceAll('export ',''),ctx);
 await ctx.renderInvoiceDashboard(panel,{api,escapeHtml:s=>String(s).replaceAll('&','&amp;').replaceAll('"','&quot;').replaceAll('<','&lt;'),downloadSIUIBlob(){},downloadSIUIFile(){},setButtonLoading:ctx.setButtonLoading,alive:()=>true});
 const form=panel.querySelector('#invoice-form');await form.onsubmit({preventDefault(){},submitter:form.querySelector('button[type="submit"]')});
 panel.querySelector('#inv-confirm').checked=true;const button=panel.querySelector('#inv-send');const pending=button.onclick();
 assert.equal(button.disabled,true);assert.equal(button.getAttribute('aria-busy'),'true');assert.ok(button.classList.contains('is-loading'));assert.match(panel.querySelector('#invoice-send-status').textContent,/OCSP/);
 // Duplicate click is ignored while the current request owns the loader.
 await button.onclick();unblock();await pending;
 assert.equal(button.disabled,false);assert.ok(!button.classList.contains('is-loading'));assert.equal(button.hasAttribute('aria-busy'),false);
 const status=panel.querySelector('#invoice-send-status');assert.ok(status.textContent.length>0);
 if(failure==='success')assert.match(status.textContent,/finalizată/);else {assert.ok(status.classList.contains('siui-error'));assert.equal(status.getAttribute('role'),'alert');}
 assert.equal(calls.filter(x=>x.endsWith('/sendEInvoice')).length,['ocsp','package'].includes(failure)?0:1);
 dom.window.close();
});
