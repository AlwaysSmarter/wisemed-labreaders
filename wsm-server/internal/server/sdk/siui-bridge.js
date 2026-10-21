/* SIUI Bridge wrapper. All transport is delegated to an existing WSMServer. */
(function(root) {
  'use strict';
  const fail = (code,message,details={}) => Object.assign(new Error(message),{code,...details});
  const abortError = () => fail('aborted','Așteptarea a fost anulată local; operația remote poate continua.',{name:'AbortError'});
  function abortable(promise,signal) {
    if(!signal) return promise;
    return new Promise((resolve,reject)=>{
      const abort=()=>{cleanup();reject(abortError());};
      const cleanup=()=>signal.removeEventListener('abort',abort);
      signal.addEventListener('abort',abort,{once:true});
      promise.then(v=>{cleanup();resolve(v);},e=>{cleanup();reject(e);});
      if(signal.aborted) abort();
    });
  }
  function bounded(promise,ms,signal,code='request_timeout') {
    if(!Number.isFinite(ms)||ms<=0) return Promise.reject(fail('invalid_timeout','Timeout invalid.'));
    let timer;
    return abortable(Promise.race([promise,new Promise((_,reject)=>{timer=setTimeout(()=>reject(fail(code,'Timpul de așteptare a expirat.')),ms);})]),signal).finally(()=>clearTimeout(timer));
  }
  const connections=new WeakMap();
  const validWSM=wsm=>wsm&&typeof wsm.callAPI==='function'&&typeof wsm.getClientsByType==='function';
  function statusEvent(options,stage,message,details={}) {
    try { Promise.resolve(options.onStatus?.({stage,message,percent:null,...details})).catch(()=>{}); } catch(_) {}
  }
  const check = signal => {if(signal?.aborted) throw abortError();};
  const segment = value => {
    if(typeof value!=='string'||!value||/[\/\\?#]/.test(value)||value==='.'||value==='..') throw fail('invalid_id','ID invalid.');
    return encodeURIComponent(value);
  };
  // Generated from readersv3/modules/siui/assets/operations.json; parity tested.
  const operations = [
    {"group":"general","name":"getCatalogues","parameters":[{"name":"partnerCategory","type":"string"},{"name":"start","type":"dateTime"}],"mutating":false},
    {"group":"general","name":"getProviderInfo","parameters":[{"name":"partnerCategory","type":"string"},{"name":"start","type":"dateTime"},{"name":"stop","type":"dateTime"},{"name":"uic","type":"string"}],"mutating":false},
    {"group":"general","name":"getProviderInfoForPhysician","parameters":[{"name":"partnerCategory","type":"string"},{"name":"start","type":"dateTime"},{"name":"stop","type":"dateTime"},{"name":"uic","type":"string"},{"name":"stencilNo","type":"string"}],"mutating":false},
    {"group":"general","name":"sendReport","parameters":[{"name":"reportType","type":"string"},{"name":"reportXML","type":"string"}],"mutating":true},
    {"group":"general","name":"getReportFeedback","parameters":[{"name":"fileName","type":"string"}],"mutating":false},
    {"group":"general","name":"getRefund","parameters":[{"name":"partnerCategory","type":"string"},{"name":"start","type":"dateTime"},{"name":"stop","type":"dateTime"},{"name":"uic","type":"string"}],"mutating":false},
    {"group":"general","name":"getRefundForPhysician","parameters":[{"name":"partnerCategory","type":"string"},{"name":"start","type":"dateTime"},{"name":"stop","type":"dateTime"},{"name":"uic","type":"string"},{"name":"stencil","type":"string"}],"mutating":false},
    {"group":"general","name":"getRefundForInvoice","parameters":[{"name":"partnerCategory","type":"string"},{"name":"invoiceNumber","type":"string"},{"name":"invoiceDate","type":"dateTime"},{"name":"uic","type":"string"}],"mutating":false},
    {"group":"general","name":"generatePrescriptionSeries","parameters":[{"name":"categoryCode","type":"string"},{"name":"orgUnitCode","type":"string"},{"name":"uic","type":"string"},{"name":"subUnitCode","type":"string","nullable":true},{"name":"validFrom","type":"dateTime"},{"name":"isOnLine","type":"boolean"},{"name":"quantity","type":"int"}],"mutating":true},
    {"group":"general","name":"getPrescriptionSeriesInfo","parameters":[{"name":"categoryCode","type":"string"},{"name":"orgUnitCode","type":"string"},{"name":"uic","type":"string"},{"name":"subUnitCode","type":"string","nullable":true}],"mutating":false},
    {"group":"general","name":"getPhysicianCatalogues","parameters":[],"mutating":false},
    {"group":"general","name":"getInsured","parameters":[{"name":"pid","type":"string"},{"name":"requestDate","type":"dateTime"}],"mutating":false},
    {"group":"general","name":"getInsuredByCID","parameters":[{"name":"cid","type":"string"},{"name":"requestDate","type":"dateTime"}],"mutating":false},
    {"group":"general","name":"getPersonInfo","parameters":[{"name":"pid","type":"string"},{"name":"fromDate","type":"dateTime"}],"mutating":false},
    {"group":"clinice","name":"getPrescription","parameters":[{"name":"serial","type":"string"},{"name":"number","type":"string"},{"name":"pid","type":"string"},{"name":"stencil","type":"string"}],"mutating":false},
    {"group":"clinice","name":"getClinicReferral","parameters":[{"name":"serial","type":"string"},{"name":"number","type":"string"},{"name":"pid","type":"string"},{"name":"stencil","type":"string"}],"mutating":false},
    {"group":"clinice","name":"getLabReferral","parameters":[{"name":"serial","type":"string"},{"name":"number","type":"string"},{"name":"pid","type":"string"},{"name":"stencil","type":"string"}],"mutating":false},
    {"group":"clinice","name":"validatePrescription","parameters":[{"name":"reportXml","type":"string"},{"name":"reportType","type":"string"}],"mutating":true},
    {"group":"clinice","name":"validateClinicReferral","parameters":[{"name":"reportXml","type":"string"},{"name":"reportType","type":"string"}],"mutating":true},
    {"group":"clinice","name":"validateLabReferral","parameters":[{"name":"reportXml","type":"string"},{"name":"reportType","type":"string"}],"mutating":true},
    {"group":"clinice","name":"validateSickLeave","parameters":[{"name":"reportXml","type":"string"}],"mutating":true},
    {"group":"general","name":"validateReport","parameters":[{"name":"reportXml","type":"string"},{"name":"reportType","type":"string"},{"name":"requestType","type":"string"}],"mutating":true},
    {"group":"clinice","name":"processIssuedPrescription","parameters":[{"name":"reportXml","type":"string"}],"mutating":true},
    {"group":"clinice","name":"cancelReleasedPrescription","parameters":[{"name":"providerCode","type":"string"},{"name":"workplaceCode","type":"string"},{"name":"insuranceHouseCode","type":"string"},{"name":"series","type":"string"},{"name":"no","type":"string"},{"name":"fractionNo","type":"int"},{"name":"cancellationReason","type":"string"}],"mutating":true},
    {"group":"clinice","name":"updateInvoices","parameters":[{"name":"requestXml","type":"string"}],"mutating":true},
    {"group":"clinice","name":"getReleasedPrescription","parameters":[{"name":"providerCode","type":"string"},{"name":"workplaceCode","type":"string"},{"name":"insuranceHouseCode","type":"string"},{"name":"series","type":"string"},{"name":"no","type":"string"},{"name":"fractionNo","type":"int"}],"mutating":false},
    {"group":"clinice","name":"processPrescribedPrescription","parameters":[{"name":"reportXml","type":"string"}],"mutating":true},
    {"group":"clinice","name":"getPrescribedPrescription","parameters":[{"name":"providerCode","type":"string"},{"name":"physicianStencilNo","type":"string"},{"name":"contractNo","type":"string"},{"name":"contractType","type":"string"},{"name":"insuranceHouseCode","type":"string"},{"name":"series","type":"string"},{"name":"no","type":"string"},{"name":"prescriptionDate","type":"dateTime"}],"mutating":false},
    {"group":"clinice","name":"cancelPrescribedPrescription","parameters":[{"name":"providerCode","type":"string"},{"name":"physicianStencilNo","type":"string"},{"name":"contractNo","type":"string"},{"name":"contractType","type":"string"},{"name":"insuranceHouseCode","type":"string"},{"name":"series","type":"string"},{"name":"no","type":"string"},{"name":"prescriptionDate","type":"dateTime"},{"name":"cancellationReason","type":"string"}],"mutating":true},
    {"group":"clinice","name":"downloadNotReportedPrescriptionsReport","parameters":[{"name":"insuranceHouseCode","type":"string"},{"name":"fromDate","type":"dateTime"},{"name":"toDate","type":"dateTime"},{"name":"providerCode","type":"string"},{"name":"stencilNo","type":"string"}],"mutating":false},
    {"group":"clinice","name":"getStatusForPrescriptions","parameters":[{"name":"insuranceHouseCode","type":"string"},{"name":"providerCode","type":"string"},{"name":"contractNo","type":"string"},{"name":"contractType","type":"string"},{"name":"startFrom","type":"dateTime"},{"name":"endTo","type":"dateTime"},{"name":"workPlaceCode","type":"string"}],"mutating":false},
    {"group":"clinice","name":"getPrescribedPrescriptionsForCid","parameters":[{"name":"requestXml","type":"string"}],"mutating":false},
    {"group":"clinice","name":"processHospitalPrescription","parameters":[{"name":"reportXml","type":"string"}],"mutating":true},
    {"group":"clinice","name":"cancelReleasedHospitalPrescription","parameters":[{"name":"providerCode","type":"string"},{"name":"workplaceCode","type":"string"},{"name":"insuranceHouseCode","type":"string"},{"name":"series","type":"string"},{"name":"no","type":"string"},{"name":"fractionNo","type":"int"},{"name":"cancellationReason","type":"string"}],"mutating":true},
    {"group":"clinice","name":"getReleasedHospitalPrescription","parameters":[{"name":"providerCode","type":"string"},{"name":"workplaceCode","type":"string"},{"name":"insuranceHouseCode","type":"string"},{"name":"series","type":"string"},{"name":"no","type":"string"},{"name":"fractionNo","type":"int"}],"mutating":false},
    {"group":"facturare","name":"sendEInvoice","parameters":[{"name":"invoiceFile","type":"string"}],"mutating":true},
    {"group":"facturare","name":"getEInvoice","parameters":[{"name":"serialCode","type":"string"},{"name":"serialNumber","type":"string"},{"name":"providerFiscalCode","type":"string"}],"mutating":false},
    {"group":"facturare","name":"getEInvoiceId","parameters":[{"name":"serialCode","type":"string"},{"name":"serialNumber","type":"string"},{"name":"providerFiscalCode","type":"string"}],"mutating":false},
    {"group":"facturare","name":"getEInvoiceClearingNote","parameters":[{"name":"serialCode","type":"string"},{"name":"serialNumber","type":"string"},{"name":"providerFiscalCode","type":"string"}],"mutating":false},
    {"group":"facturare","name":"cancelEInvoice","parameters":[{"name":"serialCode","type":"string"},{"name":"serialNumber","type":"string"},{"name":"providerFiscalCode","type":"string"},{"name":"cancellationDate","type":"dateTime"},{"name":"cancellationReason","type":"string"}],"mutating":true}
  ];
  // Auxiliary routes are explicit so callers never need direct HTTP to the bridge.
  const endpoints = [
    {"name":"getStatus","method":"GET","path":"status"},
    {"name":"getCatalog","method":"GET","path":"catalog"},
    {"name":"getOpenAPI","method":"GET","path":"openapi.json"},
    {"name":"createAPIToken","method":"POST","path":"auth/token"},
    {"name":"getSettings","method":"GET","path":"settings"},
    {"name":"saveSettings","method":"PUT","path":"settings"},
    {"name":"getCertificates","method":"GET","path":"certificates"},
    {"name":"refreshCertificates","method":"POST","path":"certificates/refresh"},
    {"name":"getTokenPINStatus","method":"GET","path":"token-pin"},
    {"name":"setTokenPIN","method":"PUT","path":"token-pin"},
    {"name":"getOCSPDebug","method":"GET","path":"general/ocsp"},
    {"name":"getOCSPSession","method":"POST","path":"general/ocsp"},
    {"name":"testOCSP","method":"POST","path":"ocsp-test"},
    {"name":"getPersonalization","method":"POST","path":"personalization"},
    {"name":"checkInsured","method":"POST","path":"insured"},
    {"name":"getInvoiceModel","method":"GET","path":"invoice/model"},
    {"name":"previewInvoice","method":"POST","path":"invoice/preview"},
    {"name":"packageInvoice","method":"POST","path":"invoice/package"},
    {"name":"submitValidation","method":"POST","path":"validations"},
    {"name":"listValidations","method":"GET","path":"validations"}
  ];
  class Bridge {
    constructor(wsm,options={}) {
      if(!validWSM(wsm) && typeof options.wsmFactory!=='function') throw fail('invalid_wsm','Este necesar un obiect WSMServer.');
      this.wsm=wsm;this.options={pollInterval:1000,waitTimeout:180000,requestTimeout:135000,connectAttempts:3,connectTimeout:5000,download:true,...options};
      this.client=null;this._selection={equipmentID:options.equipmentID,clientID:options.clientID,connectionID:options.connectionID};
      this.general={};this.clinice={};this.facturare={};
      for(const op of operations) {
        const invoke=(parameters={},options={})=>this.executeOperation(op.group,op.name,parameters,options);
        this[op.group][op.name]=invoke;
        this[op.name]=invoke;
      }
      this.ready=options.deferConnect?Promise.resolve(null):this.refresh();this.ready.catch(()=>{});
    }
    get isConnected() {
      return !!(this.wsm?.isConnected && this.client && this.wsm.clients?.some(c=>(c.connectionID||c.id)===(this.client.connectionID||this.client.id)));
    }
    async ensureConnected(options={}) {
      check(options.signal);
      if(this.wsm?.isConnected) return this.wsm;
      statusEvent(options,'connecting','Conectare la WSM');
      const attempts=options.connectAttempts??this.options.connectAttempts;
      const timeout=options.connectTimeout??this.options.connectTimeout;
      if(!Number.isInteger(attempts)||attempts<1||!Number.isFinite(timeout)||timeout<=0) throw fail('invalid_timeout','Setări de reconectare invalide.');
      let flight=this._connection || (this.wsm && connections.get(this.wsm));
      if(!flight) {
        flight=(async()=>{
          let last;
          for(let attempt=1;attempt<=attempts;attempt++) {
            let active=true;
            statusEvent(options,'connecting','Încercare de conectare la WSM',{attempt,maxAttempts:attempts});
            try {
              await bounded((async()=>{
                if(!validWSM(this.wsm)) {
                  const candidate=await this.options.wsmFactory({attempt});
                  if(!active) return;
                  if(!validWSM(candidate)) throw fail('invalid_wsm','wsmFactory trebuie să returneze WSMServer.');
                  this.wsm=candidate;
                }
                if(this.wsm.isConnected) return;
                const connect=this.wsm.reconnect||this.wsm.connect;
                if(typeof connect!=='function') throw fail('wsm_disconnected','WSM nu oferă conectare; configurează wsmFactory.');
                await connect.call(this.wsm);
                if(!this.wsm.isConnected) throw fail('wsm_disconnected','WSM nu este conectat.');
              })(),timeout,undefined,'connect_timeout');
              return this.wsm;
            } catch(e) {
              last=e;
              if(!this.wsm?.isConnected) this.wsm?.disconnect?.();
            } finally { active=false; }
          }
          throw fail('wsm_disconnected','Conectarea WSM a eșuat după '+attempts+' încercări.',{attempts,cause:last});
        })();
        this._connection=flight;
        const original=this.wsm;
        if(original) connections.set(original,flight);
        flight.finally(()=>{if(this._connection===flight)this._connection=null;if(original&&connections.get(original)===flight)connections.delete(original);}).catch(()=>{});
      }
      return abortable(flight,options.signal);
    }
    async refresh(options={}) {
      check(options.signal);
      await this.ensureConnected(options);
      statusEvent(options,'discovering','Caut SIUI Bridge');
      this.client=null;
      const list=await bounded(this.wsm.getClientsByType('siui-bridge'),options.requestTimeout??this.options.requestTimeout,options.signal);
      check(options.signal);
      const selected=(list||[]).filter(c=>Object.entries(this._selection).every(([key,value])=>value===undefined||String(c[key]??({equipmentID:c.equipment_id,clientID:c.client_id,connectionID:c.id})[key])===String(value)));
      if(!selected.length) return null;
      if(selected.length>1 && !(this.options.selectFirst && !Object.values(this._selection).some(v=>v!==undefined))) throw fail('ambiguous_bridge','Sunt conectate mai multe SIUI Bridge. Specifică equipmentID, clientID sau connectionID.');
      this.client=selected[0];
      // Pin identity to this bridge; never silently switch to another device.
      if(!Object.values(this._selection).some(v=>v!==undefined)) {
        const equipment=this.client.equipmentID||this.client.equipment_id;
        this._selection=equipment?{equipmentID:equipment}:{clientID:this.client.clientID||this.client.client_id};
      }
      return this.client;
    }
    async _request(method,path,body,options={}) {
      check(options.signal);
      const client=await this.refresh(options);
      if(!client) throw fail('bridge_offline','SIUI Bridge selectat nu este conectat.');
      check(options.signal);
      statusEvent(options,'requesting','Comunicare cu SIUI Bridge',{httpMethod:method});
      const response=await bounded(this.wsm.callAPI(client,'/api/siui/'+path,{
        method,...(body!==undefined?{body}:{}),query:options.query,
        timeout:options.requestTimeout??this.options.requestTimeout
      }),options.requestTimeout??this.options.requestTimeout,options.signal);
      if(response.body?.ok===false) throw fail('siui_error',response.body.error||'Operația SIUI a eșuat.',{response});
      return response.body;
    }
    async submitOperation(group,name,parameters,options={}) {
      const op=operations.find(v=>v.group===group&&v.name===name);
      if(!op) throw fail('unknown_operation','Operație SIUI necunoscută.');
      if(!/^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$/.test(options.operationId||'')) throw fail('operation_id_required','Furnizează un operationId unic și stabil pentru această operație.');
      if(!parameters||typeof parameters!=='object'||Array.isArray(parameters)) throw fail('invalid_parameters','Parametrii trebuie să fie un obiect.');
      const keys=op.parameters.map(p=>p.name);
      if(Object.keys(parameters).some(k=>!keys.includes(k))||op.parameters.some(p=>typeof parameters[p.name]!=='string'&&!(p.nullable&&parameters[p.name]===null))) throw fail('invalid_parameters','Trimite exact parametrii WSDL ca șiruri (null numai pentru parametrii nullable): '+keys.join(', '));
      try {return await this._request('POST',group+'/'+name,{operation_id:options.operationId,parameters,...(options.callwmapi?{callwmapi:options.callwmapi}:{})},options);}
      catch(e) {e.operationId=options.operationId;throw e;}
    }
    async executeOperation(group,name,parameters={},options={}) {
      const submitted=await this.submitOperation(group,name,parameters,options);
      if(!submitted?.call?.id) throw fail('invalid_response','Lipsește call.id în răspuns.',{response:submitted});
      if(options.wait===false) return submitted.call;
      try {
        const call=await this.waitCall(submitted.call.id,{...options,initial:submitted.call});
        return await this.completeCall(call,options);
      }
      catch(e) {e.operationId=options.operationId;e.callId=submitted.call.id;throw e;}
    }
    async completeCall(call,options={}) {
      if(call.result?.file && (options.download??this.options.download)!==false) {
        const processing=options.file??this.options.file??{};
        statusEvent(options,'downloading','Descarc rezultatul',{callId:call.id});
        const processed=processing.unzip||processing.format==='json'||processing.format==='xml';
        const bytes=await this.downloadCallFile(call.id,{...options,...processing,totalBytes:processed?undefined:call.result.file.size,onProgress:options.onDownloadProgress});
        statusEvent(options,'processing','Pregătesc rezultatul descărcat',{callId:call.id});
        const format=processing.format||'bytes';
        const data=format==='json'?JSON.parse(new TextDecoder().decode(bytes)):format==='xml'?new TextDecoder().decode(bytes):bytes;
        call={...call,download:{source:call.result.file,format,bytes,data}};
      }
      if(typeof options.processResult==='function') {
        statusEvent(options,'processing','Procesez rezultatul',{callId:call.id});
        const processed=await bounded(Promise.resolve().then(()=>options.processResult(call)),options.processingTimeout??this.options.waitTimeout,options.signal,'processing_timeout');
        return {...call,processed};
      }
      return call;
    }
    async _wait(id,kind,options={}) {
      const timeout=options.timeout??this.options.waitTimeout;
      const interval=options.pollInterval??this.options.pollInterval;
      if(!Number.isFinite(timeout)||timeout<=0||!Number.isFinite(interval)||interval<=0) throw fail('invalid_timeout','Timeout și intervalul trebuie să fie pozitive.');
      const deadline=Date.now()+timeout;
      let item=options.initial;
      for(;;) {
        check(options.signal);
        if(item) {
          statusEvent(options,'waiting','Aștept răspunsul CNAS',{callId:id,status:item.status});
          if(typeof options.onProgress==='function') await options.onProgress(item);
          check(options.signal);
          if(item.status==='completed') return item;
          if(['failed','unknown'].includes(item.status)) throw fail('siui_'+item.status,item.error||'Execuție SIUI '+item.status,{[kind]:item});
          if(!['queued','running'].includes(item.status)) throw fail('invalid_status','Stare SIUI necunoscută.',{[kind]:item});
        }
        const remaining=deadline-Date.now();
        if(remaining<=0) throw fail('wait_timeout','Așteptarea a expirat; operația remote poate continua.',{[kind+'Id']:id});
        if(item) await new Promise((resolve,reject)=>{
          const abort=()=>{clearTimeout(timer);options.signal?.removeEventListener('abort',abort);reject(abortError());};
          const timer=setTimeout(()=>{options.signal?.removeEventListener('abort',abort);resolve();},Math.min(interval,remaining));
          options.signal?.addEventListener('abort',abort,{once:true});if(options.signal?.aborted)abort();
        });
        check(options.signal);
        const left=deadline-Date.now();
        if(left<=0) throw fail('wait_timeout','Așteptarea a expirat; operația remote poate continua.',{[kind+'Id']:id});
        // Bound the entire polling read, including discovery, to the wait deadline.
        const controller=new AbortController();
        const propagate=()=>controller.abort();options.signal?.addEventListener('abort',propagate,{once:true});
        const timer=setTimeout(()=>controller.abort(),left);
        try {
          const data=await (kind==='call'?this.getCall(id,{...options,signal:controller.signal}):this.getValidation(id,{...options,signal:controller.signal}));
          item=data?.[kind];if(!item)throw fail('invalid_response','Lipsește '+kind+' din răspuns.');
        } catch(e) {
          if(controller.signal.aborted&&!options.signal?.aborted)throw fail('wait_timeout','Așteptarea a expirat; operația remote poate continua.',{[kind+'Id']:id});
          throw e;
        } finally {clearTimeout(timer);options.signal?.removeEventListener('abort',propagate);}
      }
    }
    waitCall(id,options={}) {return this._wait(id,'call',options);}
    waitValidation(id,options={}) {return this._wait(id,'job',options);}
    retryCallWMAPI(id,options={}) {return this._request('POST','calls/'+segment(id)+'/callwmapi/retry',{},options);}
    getCall(id,options={}) {return this._request('GET','calls/'+segment(id),undefined,options);}
    getCallFileChunk(id,offset=0,options={}) {
      if(!Number.isSafeInteger(offset)||offset<0) return Promise.reject(fail('invalid_offset','Offset invalid.'));
      return this._request('GET','calls/'+segment(id)+'/file',undefined,{...options,query:{offset,...(options.unzip?{unzip:'true'}:{}),...(options.entry?{entry:options.entry}:{}),...(options.validateXSD===false?{validateXSD:'false'}:{}),...(options.format&&options.format!=='bytes'?{format:options.format}:{})}});
    }
    getValidation(id,options={}) {return this._request('GET','validations/'+segment(id),undefined,options);}
    async downloadCallFile(id,options={}) {
      const controller=new AbortController();
      const propagate=()=>controller.abort();
      options.signal?.addEventListener('abort',propagate,{once:true});
      if(options.signal?.aborted)controller.abort();
      try {
        return await bounded(this._downloadCallFile(id,{...options,signal:controller.signal}),
          options.downloadTimeout??this.options.downloadTimeout??180000,options.signal,'download_timeout');
      } finally {controller.abort();options.signal?.removeEventListener('abort',propagate);}
    }
    async _downloadCallFile(id,options={}) {
      const chunks=[];let offset=0;
      const limit=options.maxBytes??64*1024*1024;
      if(!Number.isSafeInteger(limit)||limit<=0)throw fail('invalid_limit','maxBytes invalid.');
      for(;;) {
        check(options.signal);
        const chunk=await this.getCallFileChunk(id,offset,options);
        const data=Uint8Array.from(atob(chunk.data_base64),c=>c.charCodeAt(0));
        if(chunk.offset!==offset||chunk.next_offset!==offset+data.length||(!chunk.eof&&!data.length)) throw fail('invalid_chunk','Fragment de fișier inconsistent.');
        offset=chunk.next_offset;if(offset>limit)throw fail('file_too_large','Fișierul depășește limita configurată.');chunks.push(data);
        const total=Number.isSafeInteger(options.totalBytes)&&options.totalBytes>0?options.totalBytes:null;
        statusEvent(options,'downloading','Descarc rezultatul',{callId:id,receivedBytes:offset,totalBytes:total,
          percent:chunk.eof?100:total?Math.min(99,Math.floor(offset*100/total)):null});
        if(options.onProgress)await options.onProgress({receivedBytes:offset,eof:chunk.eof});
        check(options.signal);
        if(chunk.eof) {const bytes=new Uint8Array(offset);let cursor=0;for(const data of chunks){bytes.set(data,cursor);cursor+=data.length;}return bytes;}
      }
    }
  }
  for(const endpoint of endpoints) {
    Bridge.prototype[endpoint.name]=function(data,options={}) {
      if(endpoint.method==='GET') {options=data||{};data=undefined;}
      return this._request(endpoint.method,endpoint.path,endpoint.method==='GET'?undefined:(data??{}),options);
    };
  }
  function SIUIBridge(wsm,options) {return new Bridge(wsm,options);}
  SIUIBridge.create=async (wsm,options)=>{const bridge=new Bridge(wsm,options);return await bridge.ready?bridge:null;};
  SIUIBridge.operations=operations;SIUIBridge.endpoints=endpoints;
  // Application-facing singleton: every asynchronous method resolves a result envelope.
  function createSafeClient(globalObject) {
    const defaults={connectAttempts:3,connectTimeout:5000,totalTimeout:300000,requestTimeout:135000,waitTimeout:180000,downloadTimeout:180000,pollInterval:1000};
    let settings={...defaults},bridge=null,sequence=0,statusHandler=null;
    const errorResult=e=>({success:false,
      error:typeof e?.message==='string'?e.message:'Operația SIUI a eșuat.',
      code:typeof e?.code==='string'?e.code:'unexpected_error',
      ...(e?.status!==undefined?{status:e.status}:{}),
      ...(e?.operationId?{operationId:e.operationId}:{}),
      ...(e?.callId?{callId:e.callId}:{}),
      ...(e?.attempts?{attempts:e.attempts}:{})
    });
    const api={general:{},clinice:{},facturare:{}};
    api.configure=(options={})=>{
      try {
        if(!options||typeof options!=='object'||Array.isArray(options))throw fail('invalid_options','Configurare invalidă.');
        for(const key of Object.keys(defaults)) {
          if(options[key]!==undefined&&(!Number.isFinite(options[key])||options[key]<=0||(key==='connectAttempts'&&!Number.isInteger(options[key]))))
            throw fail('invalid_options','Valoare invalidă: '+key);
        }
        const selection=bridge&&!['equipmentID','clientID','connectionID','wsm','getWSM'].some(key=>key in options)?bridge._selection:{};
        settings={...settings,...selection,...options};bridge=null;
        return {success:true};
      }catch(e){return errorResult(e);}
    };
    function patch(options,allowed) {
      try {
        if(!options||typeof options!=='object'||Array.isArray(options)||Object.keys(options).some(key=>!allowed.includes(key)))
          throw fail('invalid_options','Opțiuni necunoscute sau invalide.');
        return api.configure(options);
      }catch(e){return errorResult(e);}
    }
    api.setStatusHandler=handler=>{
      if(handler!==null&&typeof handler!=='function')return errorResult(fail('invalid_options','Callback de status invalid.'));
      statusHandler=handler;return {success:true};
    };
    api.setTimeouts=options=>patch(options,['connectTimeout','requestTimeout','waitTimeout','downloadTimeout','totalTimeout']);
    api.setReconnectOptions=options=>patch(options,['connectAttempts','connectTimeout']);
    api.setPollingInterval=pollInterval=>api.configure({pollInterval});
    api.setWSM=wsm=>{
      if(!validWSM(wsm))return errorResult(fail('invalid_wsm','Obiect WSM invalid.'));
      return api.configure({wsm,getWSM:undefined,equipmentID:undefined,clientID:undefined,connectionID:undefined});
    };
    api.useAutomaticWSM=()=>api.configure({wsm:undefined,getWSM:undefined,wsmFactory:undefined,equipmentID:undefined,clientID:undefined,connectionID:undefined});
    api.selectBridge=(selection={})=>{
      if(!selection||typeof selection!=='object'||Array.isArray(selection))return errorResult(fail('invalid_options','Selecție invalidă.'));
      for(const [key,value] of Object.entries(selection)){
        if(!['equipmentID','clientID','connectionID'].includes(key)||!['string','number'].includes(typeof value)||String(value).trim()==='')
          return errorResult(fail('invalid_options','Selector invalid.'));
      }
      return api.configure({equipmentID:undefined,clientID:undefined,connectionID:undefined,...selection});
    };
    api.resetOptions=()=>{settings={...defaults};bridge=null;return {success:true};};
    api.getOptions=()=>({success:true,data:Object.fromEntries(
      [...Object.keys(defaults),'equipmentID','clientID','connectionID'].filter(key=>settings[key]!==undefined).map(key=>[key,settings[key]])
    )});
    async function invoke(method,args,optionsIndex) {
      let controller,signal,propagate,emit=()=>{};
      const requestId='siui-ui-'+(++sequence);let ended=false;
      const started=Date.now();
      try {
        const callSettings=settings;let callBridge=bridge;
        const options=args[optionsIndex]??{};
        if(typeof options!=='object'||Array.isArray(options))throw fail('invalid_options','Opțiuni invalide.');
        const handlers=[statusHandler,options.onStatus].filter((fn,index,list)=>typeof fn==='function'&&list.indexOf(fn)===index);
        emit=event=>{
          if(ended)return;
          const value={requestId,operation:method==='executeOperation'||method==='submitOperation'?args[1]:method,
            operationId:options.operationId,elapsedMs:Date.now()-started,percent:null,...event};
          for(const fn of handlers){try {Promise.resolve(fn({...value})).catch(()=>{});}catch(_){}}
        };
        emit({stage:'starting',message:'Pornesc operația'});
        const timeout=options.totalTimeout??callSettings.totalTimeout;
        if(!Number.isFinite(timeout)||timeout<=0)throw fail('invalid_timeout','totalTimeout invalid.');
        controller=new AbortController();signal=options.signal;
        propagate=()=>controller.abort();signal?.addEventListener('abort',propagate,{once:true});
        if(signal?.aborted)controller.abort();
        args[optionsIndex]={...options,signal:controller.signal,onStatus:emit};
        const work=Promise.resolve().then(async()=>{
          check(controller.signal);
          const wsm=typeof callSettings.getWSM==='function'?await callSettings.getWSM():callSettings.wsm??globalObject.objWSM??(typeof objWSM!=='undefined'?objWSM:undefined);
          check(controller.signal);
          if(!callBridge||(validWSM(wsm)&&wsm!==callBridge.wsm)) {
            callBridge=new Bridge(wsm,{...callSettings,selectFirst:true,deferConnect:true,
              wsmFactory:callSettings.wsmFactory||(()=>{throw fail('wsm_unavailable','Obiectul objWSM nu este disponibil în pagină. Inițializează conexiunea WSM a aplicației.');})});
          }
          if(settings===callSettings)bridge=callBridge;
          const data=await callBridge[method](...args);
          if(method==='refresh'&&!data)throw fail('bridge_offline','SIUI Bridge selectat nu este conectat.');
          return {success:true,data};
        });
        const result=await bounded(work,timeout,signal,'operation_timeout');
        emit({stage:'completed',message:'Apel încheiat',percent:100});
        return result;
      }catch(e){emit({stage:e?.code==='aborted'?'aborted':'error',message:e?.message||'Operația a eșuat.',code:e?.code});return errorResult(e);}
      finally {ended=true;controller?.abort();try {signal?.removeEventListener('abort',propagate);} catch(_) {}}
    }
    for(const op of operations){
      const fn=(parameters={},options={})=>invoke('executeOperation',[op.group,op.name,parameters,options],3);
      api[op.group][op.name]=fn;api[op.name]=fn;
    }
    for(const endpoint of endpoints){
      api[endpoint.name]=(...args)=>invoke(endpoint.name,args,endpoint.method==='GET'?0:1);
    }
    for(const [name,index] of Object.entries({
      refresh:0,ensureConnected:0,getCall:1,getValidation:1,waitCall:1,waitValidation:1,
      getCallFileChunk:2,downloadCallFile:1,completeCall:1,retryCallWMAPI:1,
      submitOperation:3,executeOperation:3
    })) api[name]=(...args)=>invoke(name,args,index);
    return api;
  }
  SIUIBridge.createSafeClient=()=>createSafeClient(root);
  SIUIBridge.objWSMSIUI=createSafeClient(root);
  root.objWSMSIUI=SIUIBridge.objWSMSIUI;

  if(typeof module!=='undefined'&&module.exports) module.exports=SIUIBridge;
  else root.SIUIBridge=SIUIBridge;
})(typeof globalThis!=='undefined'?globalThis:window);
