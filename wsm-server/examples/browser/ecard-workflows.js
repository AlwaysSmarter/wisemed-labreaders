/* Load after wsm.js. Business callbacks belong to the authenticated WiseMED backend.
 * No direct connection to the reader IP, no PIN/token storage, no automatic replay.
 */
(function(root){
 'use strict';
 const terminal=new Set(['completed','failed','cancelled','unknown','interrupted']);
 const fail=(code,message,details={})=>Object.assign(new Error(message),{code,...details});
 const copy=value=>JSON.parse(JSON.stringify(value));
 const normalCNP=value=>String(value||'').trim();
 const identity=card=>({cnp:normalCNP(card.B4),cid:String(card.C1||'').trim()});
 function legacyServiceText({cid,cardNumber,serviceDate,secondDate,code}){
  if([cid,cardNumber,serviceDate,code].some(v=>typeof v!=='string'||!v))throw fail('service_text','CID, număr card, data și codul trebuie să fie stringuri explicite.');
  return cid+'|'+cardNumber+'|'+serviceDate+(secondDate==null?'':'|'+secondDate)+'|'+code;
 }
 function matches(expected,actual){
  const e={cnp:normalCNP(expected.cnp),cid:String(expected.cid||'').trim()};
  if((!e.cnp||e.cnp==='0000000000000')&&!e.cid)return false;
  if(!actual.cnp||actual.cnp==='0000000000000')return !!e.cid&&e.cid===actual.cid;
  return (!e.cnp||e.cnp==='0000000000000'||e.cnp===actual.cnp)&&(!e.cid||e.cid===actual.cid);
 }
 class Workflow {
  constructor({wsm,equipmentID,readerName,contract,backend,requestPIN,onState=()=>{},onEvent=()=>{},onError=()=>{},pollMS=700}){
   if(!wsm||!equipmentID||!readerName||!contract||!backend||typeof requestPIN!=='function')throw fail('configuration','wsm, equipmentID, readerName, backend și requestPIN sunt obligatorii.');
   Object.assign(this,{wsm,equipmentID:String(equipmentID),readerName,contract:copy(contract),backend,requestPIN,onState,onEvent,onError,pollMS});
   this.mode='create';this.fisa=null;this.card=null;this.present=false;this.epoch=0;this.route=0;
   this.pending=null;this.device=null;this.subscription=null;this._tail=Promise.resolve();this._seen=new Set();this.started=false;
  }
  _state(state,details={}){this.onState({state,...details});}
  _enqueue(fn){const task=this._tail.then(fn);this._tail=task.catch(()=>{});return task;}
  async _api(path,method='GET',body){
   if(!this.device)throw fail('disconnected','Stația WSS nu este conectată.');
   const response=await this.wsm.callAPI(this.device,'/api/ecard/'+path,{method,...(body===undefined?{}:{body:copy(body)}),timeout:35000});
   return typeof response.body==='string'?JSON.parse(response.body):response.body;
  }
  async start({pendingPlan}={}){
   if(this.started)return;
   if(pendingPlan){
    if(!pendingPlan.operationId||!pendingPlan.fisaId)throw fail('plan','Planul persistent necesită operationId și fisaId.');
    if(this.fisa&&String(this.fisa.id)!==String(pendingPlan.fisaId))throw fail('plan_identity','Operația de recuperat aparține altei fișe.');
    this.pending={id:pendingPlan.operationId,request:{operation:'sign'},plan:copy(pendingPlan),handled:new Set()};
   }
   this.started=true;
   const previous=this.wsm.onEvent;
   this._eventHandler=message=>{
    try{previous?.(message);}finally{this._receive(message);}
   };
   this._previousEvent=previous;this.wsm.onEvent=this._eventHandler;
   this._previousDisconnect=this.wsm.onDisconnect;
   this._disconnectHandler=event=>{
    try{this._previousDisconnect?.(event);}finally{
     clearInterval(this._renew);this.device=null;this.present=false;this.card=null;this.epoch++;this._state('disconnected');
    }
   };
   this.wsm.onDisconnect=this._disconnectHandler;
   try{await this.reconnect();}catch(error){await this.stop();throw error;}
  }
  // Call from the application's WSM onConnect after reconnect. Never resubmits jobs.
  async reconnect(){
   await this.wsm.ready;
   const candidates=(await this.wsm.listClients()).filter(c=>String(c.equipmentID??c.equipment_id)===this.equipmentID);
   if(candidates.length!==1)throw fail('station_selection','Alege un echipament eCard conectat cu o singură conexiune.');
   this.device=candidates[0];this._seen.clear();
   const health=await this._api('health');
   if(!Array.isArray(health?.sdk_order))throw fail('station_selection','Echipamentul selectat nu expune API-ul PIAS eCard.');
   clearInterval(this._renew);this.subscription=null;
   if(typeof this.wsm.selectECard==='function') {
    this.subscription=await this.wsm.selectECard(this.device);
   } else {
    await this._subscribe();
    this._renew=setInterval(()=>this._subscribe(true).catch(this.onError),20*60*1000);
   }
   if(!this.pending)await this._enqueue(()=>this._refresh());
   else this._state('pending_reconciliation',{operationId:this.pending.id});
  }
  async _subscribe(renew=false){
   const result=await this._api('events/subscribe','POST',{hardware:true,...(renew&&this.subscription?{subscription_id:this.subscription}:{})});
   this.subscription=result.subscription_id;
  }
  async stop(){
   this.started=false;clearInterval(this._renew);this.route++;
   if(this.wsm.onEvent===this._eventHandler)this.wsm.onEvent=this._previousEvent;
   if(this.wsm.onDisconnect===this._disconnectHandler)this.wsm.onDisconnect=this._previousDisconnect;
   if(typeof this.wsm.clearECard==='function')await this.wsm.clearECard();
   else if(this.subscription){try{await this._api('events/subscribe','DELETE',{subscription_id:this.subscription});}catch{/* Connection expiry removes the subscription. */}}
   this.subscription=null;
   // An accepted signing operation continues on the reader; use resume(), not a new job.
  }
  _receive(message){
   const sender=message.connection_id||message.payload?.sender_connection_id;
   if(sender!==(this.device?.connectionID||this.device?.id)||message.payload?.event!=='ecard.event')return;
   const event=message.payload.data;
   if(!event||this._seen.has(event.id))return;
   this._seen.add(event.id);if(this._seen.size>2000)this._seen.delete(this._seen.values().next().value);
   this.onEvent(event);
   if(event.source!=='pcsc'||event.job)return; // Only physical station events, not correlated job copies.
   if(event.event==='monitor_unavailable'){
    this.epoch++;this.present=false;this.card=null;this._state('card_unavailable');return;
   }
   if(event.data?.reader!==this.readerName)return;
   if(['card_removed','reader_disconnected'].includes(event.event)){
    this.epoch++;this.present=false;this.card=null;if(this.mode==='create'&&event.event==='card_removed')this.fisa=null;this._state('card_removed');
   }else if(event.event==='card_inserted'&&!this.present){
    this.present=true;this.epoch++;
    this._enqueue(()=>this._cardAvailable()).catch(this.onError);
   }
  }
  async createOnCard(){
   this.mode='create';this.fisa=null;this.card=null;this.route++;
   return this._enqueue(()=>this._refresh());
  }
  async enterFisa(fisa){
   if(!fisa?.id||!fisa.expected)throw fail('fisa','Fișa trebuie să aibă id și expected:{cnp,cid}.');
   this.mode='existing';this.fisa=copy(fisa);this.route++;
   return this._enqueue(()=>this._refresh());
  }
  async servicesSaved(){
   return this._enqueue(()=>this._signIfReady());
  }
  async _refresh(){
   if(this.pending)throw fail('pending_reconciliation','Recuperează operația existentă înainte de una nouă.',{operationId:this.pending.id});
   const result=await this._operation({operation:'status'});
   const state=result.response?.data;
   this.present=state?.StareCardInTerminal==='CardInserat'&&state?.TerminalCurent===this.readerName;
   this.card=null;this.epoch++;
   if(this.present)return this._cardAvailable();
   this._state('waiting_card');
  }
  async _cardAvailable(){
   if(this.pending||!this.present||!this.started)return;
   if(this.card?.epoch===this.epoch)return this._signIfReady();
   const epoch=this.epoch,route=this.route;
   const report=await this._operation({operation:'read',contract:this.contract,fields:['A1','B1','B2','B4','C1']});
   if(epoch!==this.epoch||route!==this.route||!this.present)return;
   const fields=report.response.data,actual=identity(fields);
   if((!actual.cnp||actual.cnp==='0000000000000')&&!actual.cid)throw fail('identity_unavailable','Cardul nu oferă un CNP sau CID utilizabil.');
   this.card={fields,identity:actual,epoch};
   if(this.mode==='create'&&!this.fisa){
    const fisa=await this.backend.createFisaFromCard({card:copy(fields),expected:actual,readOperationId:report.operation_id});
    if(!fisa?.id||!fisa.expected)throw fail('fisa','Backendul trebuie să întoarcă {id,expected:{cnp,cid}}.');
    if(route!==this.route)return;
    this.fisa=copy(fisa);this._state('fisa_created',{fisa:copy(fisa)});
   }
   if(epoch===this.epoch&&route===this.route)await this._signIfReady();
  }
  async _signIfReady(){
   if(this.pending)throw fail('pending_reconciliation','Recuperează operația existentă înainte de semnare.',{operationId:this.pending.id});
   if(!this.fisa||!this.present||!this.card||this.card.epoch!==this.epoch){this._state('waiting_card');return;}
   const fisa=copy(this.fisa),epoch=this.epoch,route=this.route;
   if(!matches(fisa.expected,this.card.identity)){this._state('identity_mismatch');return;}
   // Backend persists this immutable plan and UUID before the first WSS POST.
   // Return null when no new/changed unsigned services exist.
   const plan=await this.backend.prepareSigning({fisa:copy(fisa),card:copy(this.card.fields)});
   if(!plan)return this._state('waiting_services');
   if(epoch!==this.epoch||route!==this.route||!this.present)return;
   if(String(plan.fisaId)!==String(fisa.id)||!matches(plan.expected,this.card.identity))throw fail('plan_identity','Planul de semnare nu aparține fișei și cardului curent.');
   if(!plan.operationId||!Array.isArray(plan.items)||!plan.items.length)throw fail('plan','Planul necesită operationId stabil și items:[{id,text}].');
   await this._operation({operation:'sign',contract:plan.contract||this.contract,expected:plan.expected,encoding:plan.encoding||'ascii',certificate:plan.certificate||'insured'},plan.items,copy(plan));
  }
  async _operation(request,items,plan){
   const id=plan?.operationId||crypto.randomUUID();
   this.pending={id,request:copy(request),plan,handled:new Set()};
   this._state('operation_started',{operationId:id,operation:request.operation});
   try{
    await this._api('operations','POST',{operation_id:id,request,...(items?{items}: {})});
    return await this._complete();
   }catch(error){
    if(this.pending){error.operationId=id;this._state('pending_reconciliation',{operationId:id});}
    throw error;
   }
  }
  async resume(plan){
   if(this.pending&&plan&&plan.operationId!==this.pending.id)throw fail('plan_identity','Există deja o altă operație de recuperat.');
   if(!this.pending){
    if(!plan?.operationId)throw fail('plan','Furnizează planul persistent din backend după reîncărcarea paginii.');
    this.pending={id:plan.operationId,request:{operation:'sign'},plan:copy(plan),handled:new Set()};
   }
   return this._enqueue(()=>this._complete()); // GET only. Never POST a replacement signing operation.
  }
  async _complete(){
   const pending=this.pending;if(!pending)throw fail('operation','Nu există operație de recuperat.');
   let status;
   for(;;){
    status=await this._api('jobs/'+encodeURIComponent(pending.id));
    this._state('progress',{operationId:pending.id,job:status.job});
    const prompt=status.pin_request;
    if(prompt&&!pending.handled.has(prompt.challenge)){
     pending.handled.add(prompt.challenge);
     let value=await this.requestPIN({prompt:prompt.prompt,operationId:pending.id});
     try{
      if(value===null)await this._api('jobs/'+pending.id+'/cancel','POST',{});
      else await this._api('jobs/'+pending.id+'/pin','POST',{challenge:prompt.challenge,value});
     }catch(error){pending.handled.delete(prompt.challenge);throw error;}
     finally{value=null;}
    }
    if(terminal.has(status.job.state))break;
    await new Promise(resolve=>setTimeout(resolve,this.pollMS));
   }
   const report=await this._report(pending.id);
   if(pending.plan){
    // Includes successful partial signatures when a later service failed/card was removed.
    // The backend must bind each id + exact original text to the stored immutable plan.
    await this.backend.saveSigningResult({plan:copy(pending.plan),report});
   }
   this.pending=null;
   this._state('operation_finished',{operationId:pending.id,report});
   if(report.response?.ok!==true)throw fail(report.response?.error?.code||'operation_failed',report.response?.error?.message||'Operația eCard a eșuat.',{report});
   return report;
  }
  async _report(id){
   const path='jobs/'+encodeURIComponent(id)+'/report';
   try{return await this._api(path);}catch(error){if(error.status!==413)throw error;}
   const first=await this._api(path+'?part=0');
   if(first.encoding!=='base64-json-utf8'||!Number.isInteger(first.parts)||first.parts<1||first.parts>128||!Number.isInteger(first.total_bytes)||first.total_bytes<1||first.total_bytes>64*1024*1024||!/^[a-f0-9]{64}$/.test(first.sha256))throw fail('report','Raport fragmentat invalid sau prea mare pentru exemplul browser.');
   const bytes=new Uint8Array(first.total_bytes);let offset=0;
   for(let part=0;part<first.parts;part++){
    const piece=part?await this._api(path+'?part='+part):first;
    if(piece.part!==part||piece.parts!==first.parts||piece.sha256!==first.sha256||piece.total_bytes!==first.total_bytes)throw fail('report','Metadatele părților raportului diferă.');
    const chunk=Uint8Array.from(atob(piece.data_base64),c=>c.charCodeAt(0));bytes.set(chunk,offset);offset+=chunk.length;
   }
   const hash=Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256',bytes)),b=>b.toString(16).padStart(2,'0')).join('');
   if(offset!==bytes.length||hash!==first.sha256)throw fail('report','SHA-256 al raportului nu corespunde.');
   return JSON.parse(new TextDecoder('utf-8',{fatal:true}).decode(bytes));
  }
 }
 if(typeof module!=='undefined'&&module.exports)module.exports={Workflow,matches,identity,legacyServiceText};
 else root.WiseMEDECard={Workflow,matches,identity,legacyServiceText};
})(typeof globalThis!=='undefined'?globalThis:window);
