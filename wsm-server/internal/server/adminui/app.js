(function(root){
'use strict';
function parseCommand(input){
 const args=String(input).trim().split(/\s+/);const command=args.shift().toLowerCase();
 if(!['help','devices','list','add','key','edit','delete','rights','toggle','save','cancel','clear','logout'].includes(command))throw new Error('Comandă necunoscută. Folosește help.');
 if(command==='key'||command==='delete'||command==='rights'){
  if(args.length!==1||!/^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$/.test(args[0]))throw new Error(`Format: ${command} <equipment-id>.`);
  return {command,equipment_id:args[0]};
 }
 if(command==='toggle'){
  if(!args.length||args.some(v=>!/^([1-9][0-9]*)$/.test(v)||!Number.isSafeInteger(Number(v))))throw new Error('Format: toggle <număr> [număr ...].');
  return {command,numbers:[...new Set(args.map(Number))]};
 }
 if(command==='edit'){
  if(args.length!==2||!/^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$/.test(args[0])||(args[1]!=='--remove-reader'&&!/^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$/.test(args[1])))throw new Error('Format: edit <equipment-id> <reader-id> sau edit <equipment-id> --remove-reader.');
  return {command,equipment_id:args[0],reader_id:args[1]==='--remove-reader'?'':args[1]};
 }
 if(command==='add'){
  if(args.length<1||args.length>2||args.some(value=>!/^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$/.test(value)))throw new Error('Format: add <equipment-id> [reader-id]. ID-uri: litere, cifre, punct, _, : sau -; maximum 128 caractere.');
  return {command,equipment_id:args[0],...(args[1]?{reader_id:args[1]}:{})};
 }
 if(args.length)throw new Error('Această comandă nu primește argumente. Folosește help.');
 return {command};
}
function appendOutput(doc,host,text){
 const row=doc.createElement('pre');row.textContent=String(text);host.append(row);
 while(host.children.length>100)host.firstElementChild.remove();host.scrollTop=host.scrollHeight;
}
if(typeof module!=='undefined'&&module.exports){module.exports={parseCommand,appendOutput};return;}
const doc=root.document,$=id=>doc.getElementById(id);let csrf='',session=null,busy=false,rightsDraft=null;
function notice(message){$('notice').textContent=message;}
function forgetSecret(){ $('device-secret').value='';$('device-secret').type='password';$('device-fields').textContent='';$('secret-panel').hidden=true;$('toggle-secret').textContent='Arată cheia'; }
function setBusy(value){busy=value;doc.querySelectorAll('button').forEach(button=>{button.disabled=value;});$('busy-label').textContent=value?'Se execută…':'';}
function output(text){appendOutput(doc,$('history'),text);}
async function api(path,method='GET',body){
 const response=await fetch('/admin/api/'+path,{method,credentials:'same-origin',cache:'no-store',headers:{'Content-Type':'application/json',...(method!=='GET'?{'X-CSRF-Token':csrf}:{})},...(body!==undefined?{body:JSON.stringify(body)}:{})});
 const data=await response.json().catch(()=>({}));
 if(!response.ok){if(response.status===401){csrf='';session=null;rightsDraft=null;forgetSecret();renderSession();}throw new Error(data.error||'Solicitarea nu a putut fi executată.');}
 return data;
}
function renderSession(){
 $('login-panel').hidden=!!session;$('console-panel').hidden=!session;
 $('session-label').textContent=session?`${session.tenant_id} / ${session.username}`:'Consolă echipamente';
 if(!session)$('history').replaceChildren();
}
async function logout(){rightsDraft=null;forgetSecret();await api('logout','POST',{});csrf='';session=null;renderSession();notice('Sesiune închisă.');}
function showRights(){
 output(`Drepturi pentru echipamentul ${rightsDraft.equipment_id} (modificări încă nesalvate):\n`+rightsDraft.catalog.map((p,i)=>`${i+1}. [${rightsDraft.selected.has(p.id)?'x':' '}] ${p.id} — ${p.description}`).join('\n')+'\nFolosește toggle 1 2 pentru bifare/debifare, save pentru aplicare sau cancel.');
}
async function run(command){
 forgetSecret();notice('');const parsed=parseCommand(command);
 if(parsed.command==='clear'){$('history').replaceChildren();return;}
 if(parsed.command==='logout'){await logout();return;}
 output('wsm › '+[parsed.command,parsed.numbers?.join(' '),parsed.equipment_id,parsed.command==='edit'&&!parsed.reader_id?'--remove-reader':parsed.reader_id].filter(Boolean).join(' '));
 if(parsed.command==='help'){
  output('devices / list                  Lista echipamentelor și starea online\nkey <equipment-id>              Afișează cheia echipamentului\nadd <equipment-id> [reader-id]   Creează cheia unui echipament\nedit <equipment-id> <reader-id>  Adaugă sau schimbă restricția reader-id\nedit <equipment-id> --remove-reader  Elimină restricția reader-id\ndelete <equipment-id>            Șterge accesul WSS și revocă cheia\nrights <equipment-id>            Afișează și editează drepturile explicate\ntoggle <număr> [număr ...]       Bifează/debifează drepturile din lista rights\nsave / cancel                   Aplică / abandonează selecția drepturilor\nclear                           Șterge istoricul afișat\nlogout                          Închide sesiunea\n\nEquipment ID = ID-ul echipamentului din WiseMED. Reader ID este opțional; completează-l numai dacă știi ID-ul intern exact al aplicației. Nu sunt acceptate comenzi de sistem.');return;
 }
 if(parsed.command==='rights'){
  const data=await api('devices');const d=(data.devices||[]).find(v=>v.equipment_id===parsed.equipment_id);
  if(!d)throw new Error('Echipament neînregistrat în această instanță.');
  if(!Array.isArray(data.permissions)||!data.permissions.length)throw new Error('Serverul trebuie actualizat pentru editarea drepturilor.');
  rightsDraft={equipment_id:d.equipment_id,key_id:d.key_id,catalog:data.permissions,selected:new Set(d.scopes||[])};showRights();return;
 }
 if(['toggle','save','cancel'].includes(parsed.command)){
  if(!rightsDraft)throw new Error('Mai întâi execută rights <equipment-id>.');
  if(parsed.command==='cancel'){rightsDraft=null;output('Modificările drepturilor au fost abandonate.');return;}
  if(parsed.command==='toggle'){
   if(parsed.numbers.some(n=>n>rightsDraft.catalog.length))throw new Error('Număr de drept inexistent. Selecția nu a fost modificată.');
   for(const n of parsed.numbers){const id=rightsDraft.catalog[n-1].id;if(rightsDraft.selected.has(id))rightsDraft.selected.delete(id);else rightsDraft.selected.add(id);}showRights();return;
  }
  const scopes=rightsDraft.catalog.filter(p=>rightsDraft.selected.has(p.id)).map(p=>p.id);
  const data=await api('devices/'+encodeURIComponent(rightsDraft.key_id),'PATCH',{scopes});
  output(`Drepturi salvate pentru ${rightsDraft.equipment_id}. Cheia rămâne aceeași.\nCopiază în reader → Debug WSS → Permisiuni solicitate, apoi Salvează și aplică:\n${data.device.scopes.join(',')}`);rightsDraft=null;return;
 }
 if(['devices','list'].includes(parsed.command)){
  const data=await api('devices');const devices=data.devices||[];
  output(devices.length?devices.map(item=>`${item.equipment_id}  |  ${item.reader_id||'orice reader'}  |  ${item.key_id}  |  ${item.online?'online':'offline'}`).join('\n'):'Nu există echipamente provisionate în această instanță.');return;
 }
 if(['edit','delete'].includes(parsed.command)){
  const listed=await api('devices');const device=(listed.devices||[]).find(item=>item.equipment_id===parsed.equipment_id);
  if(!device)throw new Error('Echipament neînregistrat în această instanță.');
  await api('devices/'+encodeURIComponent(device.key_id),parsed.command==='edit'?'PATCH':'DELETE',parsed.command==='edit'?{reader_id:parsed.reader_id}:{});
  if(rightsDraft?.key_id===device.key_id)rightsDraft=null;
  output(parsed.command==='delete'?`Acces WSS șters pentru ${parsed.equipment_id}; cheia a fost revocată. Echipamentul din WiseMED nu este șters.`:`Echipament ${parsed.equipment_id} actualizat: reader-id ${parsed.reader_id||'neimpus'}. Cheia rămâne aceeași; conexiunea echipamentului se reautentifică.`);return;
 }
 const reveal=parsed.command==='key';
 const data=await api(reveal?'devices/key':'devices','POST',{equipment_id:parsed.equipment_id,...(parsed.reader_id?{reader_id:parsed.reader_id}:{})});
 if(!data.device||typeof data.secret!=='string'||!data.secret)throw new Error('Răspuns de provisionare incomplet. Verifică lista devices înainte de reîncercare.');
 const device=data.device;
 output(`${reveal?'Cheie solicitată pentru':'Echipament creat'}: ${device.equipment_id}. Cheia este afișată separat.`);
 $('secret-title').textContent=reveal?'Cheia echipamentului':'Echipament creat';
 $('device-fields').textContent=[['Server WSS',`${root.location.protocol==='https:'?'wss:':'ws:'}//${root.location.host}/ws`],['Autentificare','device_key'],['Tenant',device.tenant_id],['ID cheie',device.key_id],['Subiect JWT',device.subject],['Emitent JWT',device.issuer],['Audiență JWT',device.audience],['Equipment ID',device.equipment_id],['Reader ID (opțional)',device.reader_id||'neimpus'],['Permisiuni',Array.isArray(device.scopes)?device.scopes.join(','):device.scopes||'']].map(([key,value])=>`${key}: ${value??''}`).join('\n');
 $('device-secret').value=data.secret;$('secret-panel').hidden=false;data.secret='';
}
async function action(callback){if(busy)return;setBusy(true);try{await callback();}catch(error){notice(error.message);if(session)output("Eroare: "+error.message);}finally{setBusy(false);}}
$('login-form').addEventListener('submit',event=>{event.preventDefault();action(async()=>{
 forgetSecret();notice('');const form=event.currentTarget;
 const credentials={tenant_id:form.elements.tenant_id.value,username:form.elements.username.value,password:form.elements.password.value};form.elements.password.value='';
 let response;try{response=await api('login','POST',credentials);}finally{credentials.password='';}
 if(!response.authenticated||!response.csrf_token)throw new Error('Autentificare incompletă.');
 session=response.session;csrf=response.csrf_token;renderSession();output('Conectat. Scrie help pentru comenzi.');$('command-input').focus();
});});
$('command-form').addEventListener('submit',event=>{event.preventDefault();const input=$('command-input');const command=input.value;input.value='';action(()=>run(command));});
$('logout-button').addEventListener('click',()=>action(logout));
$('dismiss-secret').addEventListener('click',forgetSecret);
$('toggle-secret').addEventListener('click',()=>{const shown=$('device-secret').type==='password';$('device-secret').type=shown?'text':'password';$('toggle-secret').textContent=shown?'Ascunde cheia':'Arată cheia';});
$('copy-secret').addEventListener('click',()=>action(async()=>{if(!$('device-secret').value)return;try{await navigator.clipboard.writeText($('device-secret').value);notice('Cheia a fost copiată. Lipește-o în câmpul Cheie WSS din Debug WSS al readerului/utilitarului.');}catch{notice('Copierea automată nu este disponibilă. Folosește Arată cheia și copiază manual.');}}));
root.addEventListener('pagehide',forgetSecret);
action(async()=>{
 const tenants=await api('tenants');const select=$('login-form').elements.tenant_id;select.replaceChildren();
 for(const tenant of tenants.tenants||[]){const option=doc.createElement('option');option.value=tenant.id;option.textContent=tenant.label||tenant.id;select.append(option);}
 if(!select.options.length)notice('Nu există instanțe WiseMED disponibile pentru administrare.');
 const current=await api('session');if(current.authenticated){session=current.session;csrf=current.csrf_token||'';output('Sesiune restaurată. Scrie help pentru comenzi.');}renderSession();
});
})(typeof window!=='undefined'?window:globalThis);
