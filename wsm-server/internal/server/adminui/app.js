(function(root){
'use strict';
function checkOrigin(value){
 let url;try{url=new URL(value);}catch{throw new Error('Origine HTTPS invalidă.');}
 if(!/^https:\/\/[^/?#\s]+$/.test(value)||!url.hostname||url.username||url.password)throw new Error('Originea trebuie să fie HTTPS, fără cale, credențiale, query sau fragment.');
}
function parseCommand(input){
 const args=String(input).trim().split(/\s+/);const command=args.shift().toLowerCase();
 if(command==='logs'){
  let action='show',date='';
  if(['live','stop','dates'].includes(args[0]))action=args.shift();
  if(args.length){date=args.shift();if(!/^\d{4}-\d{2}-\d{2}$/.test(date)||!Number.isFinite(Date.parse(date+'T00:00:00Z'))||new Date(date+'T00:00:00Z').toISOString().slice(0,10)!==date)throw new Error('Data logului trebuie să fie YYYY-MM-DD.');}
  if(args.length||(['stop','dates'].includes(action)&&date))throw new Error('Format: logs [YYYY-MM-DD], logs live [YYYY-MM-DD], logs stop sau logs dates.');
  return {command,action,date};
 }
 if(command==='tenant'){
  if(args.length>1||(args.length===1&&args[0]!=='edit'))throw new Error('Format: tenant sau tenant edit.');
  return {command,action:args[0]||'show'};
 }
 if(command==='origin'){
  if(args.length!==2||!['add','delete'].includes(args[0]))throw new Error('Format: origin add/delete <https://origine>.');
  checkOrigin(args[1]);return {command,action:args[0],origin:args[1]};
 }
 if(command==='browser'){
  const action=args.shift();
  if(!['add','edit','key','delete','rotate','rights','list'].includes(action))throw new Error('Format: browser add/edit <key-id> <https://origine>, browser key/delete/rotate/rights <key-id>, browser list.');
  if(action==='list'){if(args.length)throw new Error('Format: browser list.');return {command,action};}
  const needsOrigin=action==='add'||action==='edit';
  if(args.length!==(needsOrigin?2:1)||!/^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$/.test(args[0]))throw new Error(needsOrigin?'Format: browser '+action+' <key-id> <https://origine>.':'Format: browser '+action+' <key-id>. Folosește ID-ul exact din browsers.');
  if(needsOrigin)checkOrigin(args[1]);
  return {command,action,key_id:args[0],...(needsOrigin?{origin:args[1]}:{})};
 }
 if(!['browsers','help','history','devices','list','add','key','edit','delete','rights','toggle','info','save','cancel','clear','logout'].includes(command))throw new Error('Comandă necunoscută. Folosește help.');
 if(command==='key'||command==='delete'||command==='rights'){
  if(args.length!==1||!/^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$/.test(args[0]))throw new Error(`Format: ${command} <equipment-id>.`);
  return {command,equipment_id:args[0]};
 }
 if(command==='toggle'||command==='info'){
  if(!args.length||args.some(v=>!/^([1-9][0-9]*)$/.test(v)||!Number.isSafeInteger(Number(v))))throw new Error(`Format: ${command} <număr> [număr ...].`);
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
class CommandHistory {
 constructor(storage,key){
  this.storage=storage;this.key=key;this.entries=[];this.persistent=true;
  try {const saved=JSON.parse(storage.getItem(key)||'[]');if(Array.isArray(saved))this.entries=saved.filter(value=>{if(typeof value!=='string'||value.length>4096)return false;try{parseCommand(value);return true;}catch{return false;}}).slice(-1000);}catch{this.persistent=false;}
  this.reset();
 }
 reset(){this.index=this.entries.length;this.draft='';}
 add(command){
  parseCommand(command);this.entries.push(command.trim());this.entries=this.entries.slice(-1000);this.reset();
  try{this.storage.setItem(this.key,JSON.stringify(this.entries));this.persistent=true;}catch{this.persistent=false;}
 }
 move(direction,current){
  if(!this.entries.length)return current;
  if(this.index===this.entries.length)this.draft=current;
  this.index=Math.max(0,Math.min(this.entries.length,this.index+direction));
  return this.index===this.entries.length?this.draft:this.entries[this.index];
 }
}
function historyKey(session){return 'wsm.admin.commands.v1:'+JSON.stringify([session.tenant_id,session.username]);}
function permissionInfo(catalog,numbers){
 if(numbers.some(n=>n>catalog.length))throw new Error('Număr de drept inexistent.');
 return numbers.map(n=>{const p=catalog[n-1];return `${n}. ${p.id} — ${p.description}\n   Info: ${p.info||'Explicație indisponibilă; actualizează serverul WSM.'}\n   Exemplu: ${p.example||'Exemplu indisponibil.'}`;}).join('\n\n');
}
function appendOutput(doc,host,text){
 const row=doc.createElement('pre');row.textContent=String(text);host.append(row);
 while(host.children.length>100)host.firstElementChild.remove();host.scrollTop=host.scrollHeight;
}
if(typeof module!=='undefined'&&module.exports){module.exports={parseCommand,appendOutput,permissionInfo,CommandHistory,historyKey};return;}
const doc=root.document,$=id=>doc.getElementById(id);let csrf='',session=null,busy=false,rightsDraft=null,commandHistory=null;
const logsState={generation:0,timer:null,controller:null,live:false,cursor:-1,cursorDate:'',date:'',timezone:'UTC'};
function notice(message){$('notice').textContent=message;}
function forgetSecret(){ $('device-secret').value='';$('device-secret').type='password';$('device-fields').textContent='';$('secret-panel').hidden=true;$('toggle-secret').textContent='Arată cheia'; }
function setBusy(value){busy=value;doc.querySelectorAll('button').forEach(button=>{button.disabled=value;});$('busy-label').textContent=value?'Se execută…':'';}
function output(text){appendOutput(doc,$('history'),text);}
async function api(path,method='GET',body,signal){
 const response=await fetch('/admin/api/'+path,{method,signal,credentials:'same-origin',cache:'no-store',headers:{'Content-Type':'application/json',...(method!=='GET'?{'X-CSRF-Token':csrf}:{})},...(body!==undefined?{body:JSON.stringify(body)}:{})});
 const data=await response.json().catch(()=>({}));
 if(!response.ok){if(response.status===401){csrf='';session=null;rightsDraft=null;forgetSecret();renderSession();}throw new Error(data.error||'Solicitarea nu a putut fi executată.');}
 return data;
}
function renderSession(){
 const key=session?historyKey(session):null;
 if(!key||commandHistory?.key!==key){
  let storage;try{storage=root.localStorage;}catch{}
  commandHistory=key?new CommandHistory(storage,key):null;
  $('command-input').value='';
 }

 $('login-panel').hidden=!!session;$('console-panel').hidden=!session;
 $('session-label').textContent=session?`${session.tenant_id} / ${session.username}`:'Consolă echipamente';
 if(!session){stopLogs(true);$('history').replaceChildren();$('tenant-panel').hidden=true;$('tenant-form').elements.api_key.value='';}
}
function stopLogs(hide=false){
 clearTimeout(logsState.timer);logsState.controller?.abort();logsState.controller=null;
 logsState.generation++;logsState.live=false;
 $('log-live').textContent='În timp real';$('log-state').textContent='Oprit';
 if(hide){$('logs-panel').hidden=true;$('log-content').textContent='';}
}
function fillLogDates(dates,selected){
 const values=[...new Set([selected,...dates])].filter(Boolean).sort().reverse();
 const select=$('log-date');select.replaceChildren();
 for(const date of values){const option=doc.createElement('option');option.value=date;option.textContent=date;select.append(option);}
 select.value=selected;
}
async function readLogPage(){
 const generation=logsState.generation;
 const controller=new AbortController();logsState.controller=controller;
 const query=new URLSearchParams({cursor:String(logsState.cursor),cursor_date:logsState.cursorDate});
 if(logsState.date)query.set('date',logsState.date);
 try{
  const page=await api('logs?'+query,'GET',undefined,controller.signal);
  if(generation!==logsState.generation)return;
  if(logsState.cursorDate!==page.date)$('log-content').textContent='';
  logsState.cursor=page.cursor;logsState.cursorDate=page.date;
  if($('log-date').value!==page.date)fillLogDates(Array.from($('log-date').options).map(o=>o.value),page.date);
  const text=page.lines.join('\n');
  if(text){const content=$('log-content');content.textContent=(content.textContent+(content.textContent?'\n':'')+text).slice(-1024*1024);content.scrollTop=content.scrollHeight;}
  $('log-state').textContent=`${logsState.live?'Live · actualizare la 1s':'Instantaneu'} · ${page.date} · ${page.timezone}`;
  if(page.more||logsState.live)logsState.timer=setTimeout(()=>readLogPage(),page.more?0:1000);
 }catch(error){
  if(generation!==logsState.generation||error.name==='AbortError')return;
  stopLogs();$('log-state').textContent=error.message;notice(error.message);
 }
}
async function openLogs(date='',live=false){
 stopLogs();const generation=logsState.generation;
 const data=await api('logs/dates');if(generation!==logsState.generation)return;
 logsState.date=date;logsState.live=live;logsState.cursor=-1;logsState.cursorDate='';
 fillLogDates(data.dates,date||data.today);
 $('logs-panel').hidden=false;$('log-content').textContent='';$('log-live').textContent=live?'Oprește live':'În timp real';
 await readLogPage();
}
async function logout(){stopLogs(true);rightsDraft=null;forgetSecret();await api('logout','POST',{});csrf='';session=null;renderSession();notice('Sesiune închisă.');}
function showRights(){
 output(`Drepturi pentru ${rightsDraft.kind==='browser'?'browser':'echipamentul'} ${rightsDraft.equipment_id} (modificări încă nesalvate):\n`+rightsDraft.catalog.map((p,i)=>`${i+1}. [${rightsDraft.selected.has(p.id)?'x':' '}] ${p.id} — ${p.description}`).join('\n')+'\nFolosește info 1 2 pentru explicații, toggle 1 2 pentru bifare/debifare, save pentru aplicare sau cancel.');
}
async function run(command){
 forgetSecret();notice('');const parsed=parseCommand(command);
 if(commandHistory){commandHistory.add(command);if(!commandHistory.persistent)notice('Istoricul este disponibil doar în sesiunea curentă: stocarea browserului nu este disponibilă.');}
 if(parsed.command==='history'){output(commandHistory?.entries.map((entry,i)=>`${i+1}. ${entry}`).join('\n')||'Istoricul este gol.');return;}
 if(parsed.command==='clear'){$('history').replaceChildren();return;}
 if(parsed.command==='logout'){await logout();return;}
 output('wsm › '+command.trim());
 if(parsed.command==='logs'){
  if(parsed.action==='stop'){stopLogs();output('Urmărirea logurilor a fost oprită.');return;}
  if(parsed.action==='dates'){const data=await api('logs/dates');output('Zile disponibile ('+data.timezone+'):\n'+data.dates.join('\n'));return;}
  await openLogs(parsed.date,parsed.action==='live');return;
 }
 if(parsed.command==='help'){
  output('logs [YYYY-MM-DD]              Loguri pentru ziua selectată (implicit azi UTC)\nlogs live [YYYY-MM-DD]         Urmărire în timp real\nlogs stop / logs dates        Oprește urmărirea / listează zilele\n\ntenant                         Setări tenant și cheie API mascată\ntenant edit                    Formular modificare issuer/API/cheie\norigin add/delete <https://origine>  Adaugă/șterge origine permisă\nbrowsers / browser list         Chei browser/backend (inclusiv YAML)\nbrowser add <key-id> <origine>  Generează cheia și permite originea\nbrowser edit <key-id> <origine> Modifică originea; păstrează cheia\nbrowser key <key-id>            Afișează separat cheia pentru PHP\nbrowser rotate <key-id>         Generează altă cheie și revocă accesul vechi\nbrowser rights <key-id>         Editează drepturile cu toggle/save\nbrowser delete <key-id>         Revocă cheia generată\n\ndevices / list                  Lista echipamentelor și starea online\nkey <equipment-id>              Afișează cheia echipamentului\nadd <equipment-id> [reader-id]   Creează cheia unui echipament (reader + service)\nedit <equipment-id> <reader-id>  Adaugă sau schimbă restricția reader-id\nedit <equipment-id> --remove-reader  Elimină restricția reader-id\ndelete <equipment-id>            Șterge accesul WSS și revocă cheia\nrights <equipment-id>            Afișează și editează drepturile\ntoggle <număr> [număr ...]       Bifează/debifează drepturile din lista rights\ninfo <număr> [număr ...]         Explicații și exemple pentru drepturile din lista rights\nsave / cancel                   Aplică / abandonează selecția drepturilor\nhistory                         Afișează istoricul comenzilor (↑/↓ pentru navigare)\nclear                           Curăță afișajul; păstrează istoricul comenzilor\nlogout                          Închide sesiunea\n\nEquipment ID = ID-ul echipamentului din WiseMED. Reader ID este opțional; completează-l numai dacă știi ID-ul intern exact al aplicației. Nu sunt acceptate comenzi de sistem.');return;
 }
 if(parsed.command==='tenant'){
  const data=await api('tenant');
  if(parsed.action==='edit'){
   const form=$('tenant-form');form.elements.issuer.value=data.issuer||'';form.elements.base_url.value=data.wisemed.base_url||'';form.elements.api_key.value='';$('tenant-panel').hidden=false;
   output('Completează formularul tenantului. Cheia API goală păstrează valoarea curentă. După salvare trebuie să te autentifici din nou.');return;
  }
  output(`Tenant: ${data.tenant_id}\nIssuer: ${data.issuer}\nWiseMED API: ${data.wisemed.base_url}\nCheie API: ${data.wisemed.api_key_masked||'neconfigurată'}\nOrigini permise:\n${(data.allowed_origins||[]).join('\n')||'(niciuna)'}`);return;
 }
 if(parsed.command==='origin'){
  const data=await api('tenant/origins','PATCH',{action:parsed.action,origin:parsed.origin});
  output('Origini salvate și aplicate:\n'+(data.allowed_origins||[]).join('\n')+(parsed.action==='delete'?'\nConexiunile tenantului se reautentifică.':''));return;
 }
 if(parsed.command==='browsers'||parsed.command==='browser'){
  if(parsed.command==='browsers'||parsed.action==='list'){
   const data=await api('browsers');output(data.browsers.length?data.browsers.map(b=>`${b.key_id} | ${b.origin||'origini tenant'} | ${b.managed?'consolă':'YAML (editare în configurație)'} | ${b.disabled?'inactiv':'activ'} | ${b.roles.join(',')} | ${b.secret_masked}`).join('\n'):'Nu există chei browser în această instanță.');return;
  }
  if(parsed.action==='rights'){
   const data=await api('browsers');const b=data.browsers.find(b=>b.key_id===parsed.key_id);
   if(!b||!b.managed)throw new Error('Cheie browser gestionată negăsită; cheile YAML se modifică în configurație.');
   rightsDraft={kind:'browser',equipment_id:b.key_id,key_id:b.key_id,catalog:data.permissions,selected:new Set(b.scopes)};showRights();return;
  }
  if(parsed.action==='delete'){
   await api('browsers/'+encodeURIComponent(parsed.key_id),'DELETE',{});
   if(rightsDraft?.key_id===parsed.key_id)rightsDraft=null;
   output('Cheie browser revocată. Originea rămâne disponibilă altor clienți; folosește origin delete pentru eliminare. Conexiunile tenantului se reautentifică.');return;
  }
  let data;
  if(parsed.action==='add')data=await api('browsers','POST',{key_id:parsed.key_id,origin:parsed.origin});
  else if(parsed.action==='key')data=await api('browsers/key','POST',{key_id:parsed.key_id});
  else data=await api('browsers/'+encodeURIComponent(parsed.key_id),'PATCH',parsed.action==='rotate'?{rotate:true}:{origin:parsed.origin});
  const b=data.browser;
  output(`Browser ${b.key_id}: ${parsed.action==='edit'?'origine actualizată; cheia păstrată':parsed.action==='rotate'?'cheie regenerată; copiaz-o în PHP':parsed.action==='add'?'creat; copiază cheia în PHP':'cheie disponibilă separat'}.`);
  if(data.secret){
   $('secret-title').textContent='Cheie browser / backend PHP';
   $('secret-instructions').textContent='Configurează secretul exclusiv în backendul PHP care emite JWT. Browserul primește numai JWT-ul, niciodată cheia secretă.';
   $('device-fields').textContent=[['Server WSS',`${root.location.protocol==='https:'?'wss:':'ws:'}//${root.location.host}/ws`],['Tenant',b.tenant_id],['ID cheie',b.key_id],['Issuer',b.issuer],['Audience',b.audience],['Origini',b.origin||(b.allowed_origins||[]).join(', ')],['Roluri',b.roles.join(',')],['Permisiuni',b.scopes.join(',')]].map(([k,v])=>`${k}: ${v}`).join('\n');
   $('device-secret').value=data.secret;data.secret='';$('secret-panel').hidden=false;
  }
  return;
 }
 if(parsed.command==='rights'){
  const data=await api('devices');const d=(data.devices||[]).find(v=>v.equipment_id===parsed.equipment_id);
  if(!d)throw new Error('Echipament neînregistrat în această instanță.');
  if(!Array.isArray(data.permissions)||!data.permissions.length)throw new Error('Serverul trebuie actualizat pentru editarea drepturilor.');
  rightsDraft={equipment_id:d.equipment_id,key_id:d.key_id,catalog:data.permissions,selected:new Set(d.scopes||[])};showRights();return;
 }
 if(['toggle','info','save','cancel'].includes(parsed.command)){
  if(!rightsDraft)throw new Error('Mai întâi execută rights <equipment-id>.');
  if(parsed.command==='info'){output(permissionInfo(rightsDraft.catalog,parsed.numbers));return;}
  if(parsed.command==='cancel'){rightsDraft=null;output('Modificările drepturilor au fost abandonate.');return;}
  if(parsed.command==='toggle'){
   if(parsed.numbers.some(n=>n>rightsDraft.catalog.length))throw new Error('Număr de drept inexistent. Selecția nu a fost modificată.');
   for(const n of parsed.numbers){const id=rightsDraft.catalog[n-1].id;if(rightsDraft.selected.has(id))rightsDraft.selected.delete(id);else rightsDraft.selected.add(id);}showRights();return;
  }
  const scopes=rightsDraft.catalog.filter(p=>rightsDraft.selected.has(p.id)).map(p=>p.id);
  const data=await api((rightsDraft.kind==='browser'?'browsers/':'devices/')+encodeURIComponent(rightsDraft.key_id),'PATCH',{scopes});
  if(rightsDraft.kind==='browser'){output('Drepturi browser salvate: '+data.browser.scopes.join(',')+'. Actualizează drepturile cerute în JWT-ul emis de PHP. Conexiunile tenantului se reautentifică.');rightsDraft=null;return;}
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
 $('secret-instructions').textContent='În reader: Settings → Debug WSS, autentificare Cheie echipament (JWT). Lipește cheia în câmpul Cheie WSS și apasă Salvează și aplică.';
 $('secret-title').textContent=reveal?'Cheia echipamentului':'Echipament creat';
 $('device-fields').textContent=[['Server WSS',`${root.location.protocol==='https:'?'wss:':'ws:'}//${root.location.host}/ws`],['Autentificare','device_key'],['Tenant',device.tenant_id],['ID cheie',device.key_id],['Subiect JWT',device.subject],['Emitent JWT',device.issuer],['Audiență JWT',device.audience],['Equipment ID',device.equipment_id],['Reader ID (opțional)',device.reader_id||'neimpus'],['Roluri JWT',Array.isArray(device.roles)?device.roles.join(','):device.roles||''],['Permisiuni',Array.isArray(device.scopes)?device.scopes.join(','):device.scopes||'']].map(([key,value])=>`${key}: ${value??''}`).join('\n');
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
$('command-form').addEventListener('submit',event=>{event.preventDefault();if(busy)return;const input=$('command-input');const command=input.value;input.value='';action(()=>run(command));});
$('command-input').addEventListener('keydown',event=>{
 if(event.isComposing||event.altKey||event.ctrlKey||event.metaKey||!commandHistory||!['ArrowUp','ArrowDown'].includes(event.key))return;
 event.preventDefault();const input=event.currentTarget;
 input.value=commandHistory.move(event.key==='ArrowUp'?-1:1,input.value);
 input.setSelectionRange(input.value.length,input.value.length);
});
$('logout-button').addEventListener('click',()=>action(logout));
$('dismiss-secret').addEventListener('click',forgetSecret);
$('toggle-secret').addEventListener('click',()=>{const shown=$('device-secret').type==='password';$('device-secret').type=shown?'text':'password';$('toggle-secret').textContent=shown?'Ascunde cheia':'Arată cheia';});
$('copy-secret').addEventListener('click',()=>action(async()=>{if(!$('device-secret').value)return;try{await navigator.clipboard.writeText($('device-secret').value);notice('Cheia a fost copiată. Folosește instrucțiunile din panoul cheii.');}catch{notice('Copierea automată nu este disponibilă. Folosește Arată cheia și copiază manual.');}}));
$('log-load').addEventListener('click',()=>action(()=>openLogs($('log-date').value,false)));
$('log-date').addEventListener('change',()=>action(()=>openLogs($('log-date').value,logsState.live)));
$('log-live').addEventListener('click',()=>action(async()=>{if(logsState.live){stopLogs();return;}await openLogs(logsState.date,true);}));
$('log-close').addEventListener('click',()=>stopLogs(true));
$('tenant-form').addEventListener('submit',event=>{event.preventDefault();action(async()=>{
 const form=event.currentTarget;
 const body={issuer:form.elements.issuer.value,base_url:form.elements.base_url.value};
 if(form.elements.api_key.value)body.api_key=form.elements.api_key.value;
 form.elements.api_key.value='';
 try{await api('tenant','PATCH',body);}finally{delete body.api_key;}
 $('tenant-panel').hidden=true;csrf='';session=null;rightsDraft=null;forgetSecret();renderSession();notice('Setări tenant salvate. Autentifică-te din nou. Emitenții JWT trebuie să folosească noul issuer; cheia API trebuie să corespundă celei din WiseMED.');
});});
$('tenant-cancel').addEventListener('click',()=>{$('tenant-form').elements.api_key.value='';$('tenant-panel').hidden=true;});
root.addEventListener('pagehide',()=>{stopLogs(true);forgetSecret();$('tenant-form').elements.api_key.value='';});
action(async()=>{
 const tenants=await api('tenants');const select=$('login-form').elements.tenant_id;select.replaceChildren();
 for(const tenant of tenants.tenants||[]){const option=doc.createElement('option');option.value=tenant.id;option.textContent=tenant.label||tenant.id;select.append(option);}
 if(!select.options.length)notice('Nu există instanțe WiseMED disponibile pentru administrare.');
 const current=await api('session');if(current.authenticated){session=current.session;csrf=current.csrf_token||'';output('Sesiune restaurată. Scrie help pentru comenzi.');}renderSession();
});
})(typeof window!=='undefined'?window:globalThis);
