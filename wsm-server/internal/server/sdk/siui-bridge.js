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
      if(!wsm || typeof wsm.callAPI!=='function' || typeof wsm.getClientsByType!=='function') throw fail('invalid_wsm','Este necesar un obiect WSMServer.');
      this.wsm=wsm;this.options={pollInterval:1000,waitTimeout:180000,requestTimeout:135000,...options};
      this.client=null;this._selection={equipmentID:options.equipmentID,clientID:options.clientID,connectionID:options.connectionID};
      this.general={};this.clinice={};this.facturare={};
      for(const op of operations) {
        const invoke=(parameters={},options={})=>this.executeOperation(op.group,op.name,parameters,options);
        this[op.group][op.name]=invoke;
        this[op.name]=invoke;
      }
      this.ready=this.refresh();this.ready.catch(()=>{});
    }
    get isConnected() {
      return !!(this.wsm.isConnected && this.client && this.wsm.clients?.some(c=>(c.connectionID||c.id)===(this.client.connectionID||this.client.id)));
    }
    async refresh(options={}) {
      check(options.signal);
      if(!this.wsm.isConnected) {this.client=null;throw fail('wsm_disconnected','WSM nu este conectat. Așteaptă objWSM.ready.');}
      this.client=null;
      const list=await abortable(this.wsm.getClientsByType('siui-bridge'),options.signal);
      check(options.signal);
      const selected=(list||[]).filter(c=>Object.entries(this._selection).every(([key,value])=>value===undefined||String(c[key]??({equipmentID:c.equipment_id,clientID:c.client_id,connectionID:c.id})[key])===String(value)));
      if(!selected.length) return null;
      if(selected.length>1) throw fail('ambiguous_bridge','Sunt conectate mai multe SIUI Bridge. Specifică equipmentID, clientID sau connectionID.');
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
      const response=await abortable(this.wsm.callAPI(client,'/api/siui/'+path,{
        method,...(body!==undefined?{body}:{}),query:options.query,
        timeout:options.requestTimeout??this.options.requestTimeout
      }),options.signal);
      if(response.body?.ok===false) throw fail('siui_error',response.body.error||'Operația SIUI a eșuat.',{response});
      return response.body;
    }
    async submitOperation(group,name,parameters,options={}) {
      const op=operations.find(v=>v.group===group&&v.name===name);
      if(!op) throw fail('unknown_operation','Operație SIUI necunoscută.');
      if(!/^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$/.test(options.operationId||'')) throw fail('operation_id_required','Furnizează un operationId unic și stabil pentru această operație.');
      if(!parameters||typeof parameters!=='object'||Array.isArray(parameters)) throw fail('invalid_parameters','Parametrii trebuie să fie un obiect.');
      const keys=op.parameters.map(p=>p.name);
      if(Object.keys(parameters).some(k=>!keys.includes(k))||keys.some(k=>typeof parameters[k]!=='string')) throw fail('invalid_parameters','Trimite exact parametrii WSDL ca șiruri: '+keys.join(', '));
      try {return await this._request('POST',group+'/'+name,{operation_id:options.operationId,parameters},options);}
      catch(e) {e.operationId=options.operationId;throw e;}
    }
    async executeOperation(group,name,parameters={},options={}) {
      const submitted=await this.submitOperation(group,name,parameters,options);
      if(!submitted?.call?.id) throw fail('invalid_response','Lipsește call.id în răspuns.',{response:submitted});
      if(options.wait===false) return submitted.call;
      try {return await this.waitCall(submitted.call.id,{...options,initial:submitted.call});}
      catch(e) {e.operationId=options.operationId;e.callId=submitted.call.id;throw e;}
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
    getCall(id,options={}) {return this._request('GET','calls/'+segment(id),undefined,options);}
    getCallFileChunk(id,offset=0,options={}) {
      if(!Number.isSafeInteger(offset)||offset<0) return Promise.reject(fail('invalid_offset','Offset invalid.'));
      return this._request('GET','calls/'+segment(id)+'/file',undefined,{...options,query:{offset}});
    }
    getValidation(id,options={}) {return this._request('GET','validations/'+segment(id),undefined,options);}
    async downloadCallFile(id,options={}) {
      const chunks=[];let offset=0;
      const limit=options.maxBytes??64*1024*1024;
      for(;;) {
        check(options.signal);
        const chunk=await this.getCallFileChunk(id,offset,options);
        const data=Uint8Array.from(atob(chunk.data_base64),c=>c.charCodeAt(0));
        if(chunk.offset!==offset||chunk.next_offset!==offset+data.length||(!chunk.eof&&!data.length)) throw fail('invalid_chunk','Fragment de fișier inconsistent.');
        offset=chunk.next_offset;if(offset>limit)throw fail('file_too_large','Fișierul depășește limita configurată.');chunks.push(data);
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
  if(typeof module!=='undefined'&&module.exports) module.exports=SIUIBridge;
  else root.SIUIBridge=SIUIBridge;
})(typeof globalThis!=='undefined'?globalThis:window);
