// Preflight and SOAP share the in-memory OCSP session for at most 150 seconds.
export async function connectInvoiceOCSP(api) {
 const session=await api('/api/siui/general/ocsp',{method:'POST',body:'{}'});
 if(!session.authenticated||!session.token)throw new Error('Conectarea OCSP nu a returnat un token valid.');
 return session;
}

export function availableInvoiceItems(items,category,contract,date) {
 return items.filter(i=>i.providerCategory===category&&i.contractType===contract&&(!date||i.validFrom<=date));
}

export async function renderInvoiceDashboard(panel, {api, escapeHtml:esc, downloadSIUIBlob:save, downloadSIUIFile:download, setButtonLoading, alive}) {
 const model=await api('/api/siui/invoice/model');if(!alive())return;
 let preview=null,packed=null,id=crypto.randomUUID(),busy=false,revision=0;
 const labels={header:'Factură și contract',provider:'Furnizor',insurer:'Casa de asigurări'};
 const fields=(section,values={})=>model.fields[section].map(f=>{
  const generated=['totalAmmount','vatAmmount','ammount','isSigned'].includes(f.name),required=f.required&&!generated?'required':'';
  let control;
  if(section==='items'&&f.name==='code')control=`<select name="code" required></select>`;
  else if(f.name==='orgUnitCode'&&model.insurance_houses)control=`<select name="${f.name}" required><option value="">Selectează CAS</option>${model.insurance_houses.map(h=>`<option value="${esc(h.code)}" ${h.code===values[f.name]?'selected':''}>${esc(h.code)} — ${esc(h.name)}</option>`).join('')}</select>`;
  else if(f.name==='providerCategory')control=`<select name="${f.name}">${model.partner_categories.map(c=>`<option ${c===(values[f.name]||'PARA')?'selected':''}>${c}</option>`).join('')}</select>`;
  else if(f.type==='boolean')control=`<select name="${f.name}" ${f.name==='isSigned'?'disabled':''}><option value="">Nespecificat</option><option value="true" ${values[f.name]==='true'?'selected':''}>Da</option><option value="false" ${values[f.name]==='false'?'selected':''}>Nu</option></select>`;
  else control=`<input name="${f.name}" type="${f.type==='date'?'date':'text'}" value="${esc(values[f.name]||'')}" ${required} ${generated?'readonly':''} ${f.maxLength?`maxlength="${Number(f.maxLength)}"`:''} placeholder="${generated?'Calculat automat':f.type==='decimal'?'0.00':''}">`;
  return `<label>${esc(f.label)} ${f.required?'*':''}<small>${esc(f.name)}${f.name==='vat'?' — sumă TVA, nu procent':''}</small>${control}</label>`;
 }).join('');
 panel.innerHTML=`<div class="invoice-heading"><div><span class="invoice-kicker">CNAS · LABORATOR DE TEST</span><h3>Factură electronică</h3><p>XML, semnătură CMS și DataMatrix · PIAS 3.7.32</p></div><span class="invoice-badge">Previzualizare locală</span></div>
 <div class="invoice-flow"><div><b>01 · Raportare</b><p>Feedbackul raportării și decontul: GENERAL → getReportFeedback / getRefund.</p></div><div><b>02 · Factură</b><p>Completează articolele COM_ITEMS. Verifică XML-ul, totalurile și codul tipărit.</p></div><div><b>03 · Înregistrare</b><p>Semnează CMS și transmite sendEInvoice. După timeout consultă getEInvoice înainte de retrimitere.</p></div><div><b>04 · Răspunsuri</b><p>getEInvoiceId: ID SIUI pentru ANAF. getEInvoiceClearingNote: nota de refuz, dacă a fost generată.</p></div></div>
 <details class="invoice-note"><summary>Formularul 13, răspunsuri și codul de bare</summary><p>Documentația furnizată nu identifică un „formular 13” și un al doilea formular obligatoriu universal. Denumirea trebuie confirmată cu CAS. Feedbackul raportării, decontul și nota de refuz sunt documente distincte. Lipsa unei note de refuz nu confirmă plata.</p><p>Factura CNAS cere <b>DataMatrix, nu QR</b>: octeții unui ZIP cu XML conform InvoiceBarcode.xsd. ID-ul pentru e-Factura ANAF nu este conținutul codului. Acest dashboard nu transmite către ANAF.</p></details>
 <button type="button" id="inv-example">Reîncarcă datele de test HIT MED</button><p>Câmpurile * sunt obligatorii. Codurile articolelor se preiau din COM_ITEMS pentru categoria aleasă.</p>
 <form id="invoice-form">${Object.entries(labels).map(([s,label])=>`<details open class="invoice-section"><summary>${label}</summary><div class="invoice-grid" data-section="${s}">${fields(s)}</div></details>`).join('')}
 <details class="invoice-section"><summary>Adrese pentru tipărire / DataMatrix</summary><div class="invoice-grid"><label>Adresa furnizorului<input name="provider_address"></label><label>Adresa CAS<input name="insurer_address"></label></div><p>Adresele sunt incluse în codul de bare, nu în ElectronicInvoice.xsd.</p></details>
 <section class="invoice-section"><h4>Articole facturate</h4><div id="invoice-items"></div><button type="button" id="inv-add">Adaugă articol</button></section><button type="submit">Generează XML și DataMatrix — fără CNAS</button></form>
 <p id="invoice-status" role="status" class="invoice-status"></p><pre id="invoice-ocsp-result" style="white-space:pre-wrap;overflow-wrap:anywhere"></pre><section id="invoice-preview" hidden></section>
 <section class="invoice-section"><h4>Consultă înregistrarea CNAS</h4><p>Aceste operații contactează CNAS. Factura este identificată prin serie, număr și CUI.</p><form id="invoice-lookup"><div class="invoice-grid"><label>Serie<input name="serialCode"></label><label>Număr<input name="serialNumber" required></label><label>CUI furnizor<input name="providerFiscalCode" required></label></div><div class="invoice-actions"><button type="submit" data-operation="getEInvoice">Preia factura înregistrată</button><button type="submit" data-operation="getEInvoiceId">Preia ID SIUI pentru ANAF</button><button type="submit" data-operation="getEInvoiceClearingNote">Preia nota de refuz</button></div></form><pre id="invoice-remote-result"></pre><div id="invoice-download"></div></section>`;
 const form=panel.querySelector('#invoice-form'),status=panel.querySelector('#invoice-status'),area=panel.querySelector('#invoice-preview');
 function showStatus(message,error=false){
  for(const node of [status,area.querySelector('#invoice-send-status')].filter(Boolean)){node.textContent=message;node.classList.toggle('siui-error',error);node.setAttribute('role',error?'alert':'status')}
 }
 const changed=()=>{revision++;preview=null;packed=null;id=crypto.randomUUID();area.hidden=true;status.textContent='Date modificate. Generează din nou previzualizarea.'};
 form.addEventListener('input',changed);form.addEventListener('change',changed);
 function itemOptions(row,code='') {
  const header=form.querySelector('[data-section="header"]'),value=name=>header.querySelector(`[name="${name}"]`).value;
  const options=availableInvoiceItems(model.invoice_items||[],value('providerCategory'),value('contractType'),value('issueDate'));
  const select=row.querySelector('[name="code"]');select.innerHTML='<option value="">Selectează articolul</option>'+options.map(i=>`<option value="${esc(i.code)}">${esc(i.code)} — ${esc(i.description)}</option>`).join('');select.value=code;
  select.onchange=()=>{const item=options.find(i=>i.code===select.value);if(item)row.querySelector('[name="description"]').value=item.description};
 }
 form.addEventListener('change',event=>{if(['providerCategory','contractType','issueDate'].includes(event.target.name))panel.querySelectorAll('.invoice-item').forEach(row=>itemOptions(row,row.querySelector('[name="code"]').value))});
 function addItem(values={}){const row=document.createElement('div');row.className='invoice-item';row.innerHTML=`<div class="invoice-grid">${fields('items',values)}</div><button type="button" class="ghost">Elimină articol</button>`;row.querySelector('button').onclick=()=>{row.remove();changed()};panel.querySelector('#invoice-items').append(row);itemOptions(row,values.code||'')}
 addItem();panel.querySelector('#inv-add').onclick=()=>{addItem();changed()};
 function read(){const data={items:[]};for(const s of Object.keys(labels)){data[s]={};panel.querySelectorAll(`[data-section="${s}"] [name]`).forEach(e=>{if(e.value&&!e.readOnly&&!e.disabled)data[s][e.name]=e.value})}panel.querySelectorAll('.invoice-item').forEach(row=>{const item={};row.querySelectorAll('[name]').forEach(e=>{if(e.value&&!e.readOnly)item[e.name]=e.value});data.items.push(item)});data.provider_address=form.elements.provider_address.value;data.insurer_address=form.elements.insurer_address.value;return data}
 panel.querySelector('#inv-example').onclick=()=>{
  if(busy)return;
  const demo={header:{contractDate:'2026-06-30',contractNumber:'3131',contractType:'PAR',dueDate:'2026-10-30',endDate:'2026-09-30',isFinal:'true',isRegularization:'false',issueDate:'2026-10-13',orgUnitCode:'CAS-DJ',providerCategory:'PARA',providerCode:'7991962',serialCode:'HIT24',serialNumber:'0337',startDate:'2026-09-01',totalAmmount:'27937.65',vatAmmount:'0.00'},provider:{bankAccount:'RO12TREZ2915069XXX010466',bankName:'TREZORERIA MUN. CRAIOVA',capital:'3500',companyName:'HIT MED S.R.L.',fiscalCode:'7991962',registerNo:'J16/1502/1995',signature:'VADUVA ALICE-ROXANA',stencilNo:'227523'},insurer:{bankAccount:'RO61TREZ27A660404200109X',bankName:'TREZORERIA MUN. CRAIOVA',companyName:'CJAS',fiscalCode:'11340652'}};
  for(const s of Object.keys(labels))panel.querySelectorAll(`[data-section="${s}"] [name]`).forEach(e=>e.value=demo[s][e.name]||'');
  form.elements.provider_address.value='Craiova str. Stefan cel Mare nr. 23';form.elements.insurer_address.value='Craiova Str. 1 Decembrie 1918';
  panel.querySelector('#invoice-items').replaceChildren();addItem({ammount:'27937.65',code:'PARACLLAB',description:'Asistenta medicala paraclinica LAB. (cf.contr.nr.3131 din 30 iun. 2026,af.lunii SEPTEMBRIE.2026)',quantity:'1',unitName:'LUNA',unitPrice:'27937.65',vat:'0'});changed()
 };
 panel.querySelector('#inv-example').onclick();
 async function withBusy(action,button){
  if(busy)return;busy=true;
  const controls=[...panel.querySelectorAll('button,input,select')],previous=controls.map(e=>e.disabled);
  setButtonLoading(button,true);controls.forEach(e=>e.disabled=true);
  try{await action()}catch(e){if(alive())showStatus(e.message||String(e),true)}
  finally{setButtonLoading(button,false);controls.forEach((e,i)=>e.disabled=previous[i]);busy=false}
 }
 async function runOperation(name,parameters,operationID=crypto.randomUUID()){
  const output=panel.querySelector('#invoice-remote-result');output.classList.remove('siui-error');output.textContent='Cerere CNAS în curs…';panel.querySelector('#invoice-download').replaceChildren();
  const response=await api('/api/siui/facturare/'+name,{method:'POST',body:JSON.stringify({operation_id:operationID,parameters})});let result;
  while(alive()){const data=await api('/api/siui/calls/'+encodeURIComponent(response.call.id));result=data.call;if(!alive())return;output.textContent=(result.error?result.error+'\n\n':'')+JSON.stringify(result,null,2);output.classList.toggle("siui-error",["failed","unknown"].includes(result.status));if(!['queued','running'].includes(result.status))break;await new Promise(resolve=>setTimeout(resolve,1500))}
  if(!alive())return;
  if(result.result?.file){const button=document.createElement('button');button.textContent='Descarcă documentul CNAS';button.onclick=()=>withBusy(()=>download(result.result.file));panel.querySelector('#invoice-download').append(button)}
  if(result.status==='unknown')showStatus('Rezultat incert. Consultă getEInvoice înainte de o nouă trimitere.',true);
  else if(result.status==='failed')throw new Error(result.error||'Operație nereușită');
  else if(result.status==='completed')showStatus('Operație CNAS finalizată. Verifică răspunsul de mai jos; finalizarea apelului nu confirmă plata.');
  else throw new Error('Stare CNAS neașteptată: '+result.status);
 }
 form.onsubmit=event=>{event.preventDefault();const data=read(),current=revision;return withBusy(async()=>{
  showStatus('Generez și verific XML-urile cu schemele CNAS…');const result=await api('/api/siui/invoice/preview',{method:'POST',body:JSON.stringify(data)});if(!alive()||current!==revision)return;
  preview=result;packed=null;showStatus('Validare XSD reușită. Factura nu este semnată și nu a fost trimisă la CNAS.');
  for(const n of ['totalAmmount','vatAmmount'])form.querySelector(`[name="${n}"]`).value=result.data.header[n];
  panel.querySelectorAll('.invoice-item').forEach((row,i)=>row.querySelector('[name="ammount"]').value=result.data.items[i].ammount);
  const d=result.data;area.hidden=false;area.innerHTML=`<div class="invoice-actions"><button type="button" id="inv-xml">Descarcă XML</button><button type="button" id="inv-code">Descarcă DataMatrix PNG</button><button type="button" id="inv-print">Tipărește previzualizarea</button></div>
   <div id="invoice-paper" class="invoice-paper"><div class="invoice-paper-top"><div><span class="invoice-kicker">PREVIZUALIZARE · NETRIMISĂ</span><h2>Factură ${esc(d.header.serialCode||'')} ${esc(d.header.serialNumber)}</h2><p>Emisă ${esc(d.header.issueDate)} · Perioada ${esc(d.header.startDate)} — ${esc(d.header.endDate)}</p><p>Contract ${esc(d.header.contractNumber)} / ${esc(d.header.contractDate)} · ${esc(d.header.providerCategory)}</p></div>${result.barcode_png_base64?`<figure><img class="invoice-datamatrix" src="data:image/png;base64,${result.barcode_png_base64}" alt="DataMatrix CNAS, XML comprimat ZIP"><figcaption>DataMatrix CNAS</figcaption></figure>`:`<p>${esc(result.barcode_warning)}</p>`}</div>
   <div class="invoice-partners">${['provider','insurer'].map(s=>`<div><h4>${s==='provider'?'Furnizor':'Cumpărător'}</h4><strong>${esc(d[s].companyName)}</strong><p>CUI ${esc(d[s].fiscalCode)} · ${esc(d[s].registerNo||'')}</p><p>${esc(s==='provider'?d.provider_address:d.insurer_address)}</p><p>${esc(d[s].bankName||'')} ${esc(d[s].bankAccount||'')}</p>${s==='provider'?`<p>Capital ${esc(d[s].capital)} · Reprezentant ${esc(d[s].signature)}</p>`:''}</div>`).join('')}</div>
   <table><thead><tr><th>Articol</th><th>UM</th><th>Cantitate</th><th>Preț</th><th>Valoare</th><th>TVA</th></tr></thead><tbody>${d.items.map(i=>`<tr><td>${esc(i.description)}<small>${esc(i.code)}</small></td><td>${esc(i.unitName)}</td><td>${esc(i.quantity)}</td><td>${esc(i.unitPrice)}</td><td>${esc(i.ammount)}</td><td>${esc(i.vat)}</td></tr>`).join('')}</tbody></table><p class="invoice-total">Total fără TVA: ${esc(d.header.totalAmmount)} · TVA: ${esc(d.header.vatAmmount)} · Total de plată: ${esc(result.total_with_vat||'')}</p><p>Document de test. Previzualizarea nu confirmă înregistrarea în CNAS.</p></div>
   <details><summary>XML factură / XML DataMatrix</summary><pre id="inv-xml-text"></pre><pre id="inv-barcode-text"></pre></details>
   <div class="invoice-send"><label><input type="checkbox" id="inv-confirm"> Am verificat datele, codurile COM_ITEMS și endpointul CNAS; solicit transmiterea efectivă.</label><button type="button" id="inv-send">Semnează cu tokenul și transmite la CNAS</button><p id="invoice-send-status" class="invoice-status" role="status" aria-live="polite"></p><p>Verifică datele de test înainte de transmitere. PIN-ul este cerut de driver. Endpointul configurat poate fi producție.</p></div>`;
  area.querySelector('#inv-xml-text').textContent=result.invoice_xml;area.querySelector('#inv-barcode-text').textContent=result.barcode_xml;
  const lookup=panel.querySelector('#invoice-lookup');lookup.elements.serialCode.value=d.header.serialCode||'';lookup.elements.serialNumber.value=d.header.serialNumber;lookup.elements.providerFiscalCode.value=d.provider.fiscalCode;
  area.querySelector('#inv-xml').onclick=()=>save(new Blob([result.invoice_xml],{type:'application/xml'}),'invoice.xml');
  area.querySelector('#inv-code').disabled=!result.barcode_png_base64;area.querySelector('#inv-code').onclick=()=>save(new Blob([Uint8Array.from(atob(result.barcode_png_base64),c=>c.charCodeAt(0))],{type:'image/png'}),'invoice-datamatrix.png');
  area.querySelector('#inv-print').disabled=!result.barcode_png_base64;
  area.querySelector('#inv-print').onclick=()=>{document.querySelector('#siui-print-root')?.remove();const root=document.createElement('section');root.id='siui-print-root';const paper=area.querySelector('#invoice-paper').cloneNode(true);paper.removeAttribute('id');root.append(paper);document.body.append(root);document.body.classList.add('printing-siui-invoice');const cleanup=()=>{root.remove();document.body.classList.remove('printing-siui-invoice')};window.addEventListener('afterprint',cleanup,{once:true});try{window.print()}catch(e){cleanup();throw e}};
  area.querySelector('#inv-send').onclick=()=>{if(!result.barcode_png_base64){showStatus('DataMatrix indisponibil. Redu conținutul facturii înainte de emitere.');return}if(!area.querySelector('#inv-confirm').checked){showStatus('Confirmă datele înainte de transmitere.');return}if(d.provider.fiscalCode==='00000000'||d.items.some(i=>i.code.startsWith('TEST-'))){showStatus('Înlocuiește datele fictive înainte de transmitere.');return}return withBusy(async()=>{showStatus('Reîncărcare certificate și conectare OCSP…');const ocsp=panel.querySelector('#invoice-ocsp-result');ocsp.classList.remove('siui-error');ocsp.textContent='Conectare OCSP…';try{const connection=await api('/api/siui/general/ocsp');if(!alive())return;ocsp.textContent='Parametri OCSP configurați:\n'+JSON.stringify(connection.debug,null,2)+'\nDate factură (providerCategory nu se transmite la OCSP):\n'+JSON.stringify({providerCode:d.header.providerCode,providerCategory:d.header.providerCategory,orgUnitCode:d.header.orgUnitCode},null,2)+'\nConectare OCSP…';const session=await connectInvoiceOCSP(api);if(!alive())return;ocsp.textContent+='\nFolosesc token-ul OCSP '+session.token+' cu vechime de '+(session.age_seconds||0)+' secunde'+(session.reused?' (reutilizat)':' (nou)')}catch(e){if(alive()){ocsp.classList.add('siui-error');ocsp.textContent+='\nOCSP: '+e.message};throw e}showStatus('OCSP reușit. Semnare CMS cu tokenul…');if(!packed)packed=await api('/api/siui/invoice/package',{method:'POST',body:JSON.stringify({invoice_xml:preview.invoice_xml})});if(alive()){showStatus('Semnare reușită. Transmitere și așteptare răspuns CNAS…');await runOperation('sendEInvoice',{invoiceFile:packed.invoiceFile},id)}},area.querySelector('#inv-send'))};
 },event.submitter)};
 panel.querySelector('#invoice-lookup').onsubmit=event=>{event.preventDefault();const name=event.submitter?.dataset.operation||'getEInvoice',data=Object.fromEntries(new FormData(event.currentTarget));return withBusy(()=>runOperation(name,data),event.submitter)};
}
