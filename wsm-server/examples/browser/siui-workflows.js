/* Load after wsm.js and siui-bridge.js. Endpoints are illustrative WiseMED contracts.
 * Supply stable operationId from your application; retain it for reconciliation.
 * Do not log lt, patient data, or XML. All transport uses the authenticated WSM.
 */
(function(root){
 'use strict';
 const utcDateTime=()=>new Date().toISOString();
 function safeFileID(id){
  const value=String(id);
  if(!/^[A-Za-z0-9_-]+$/.test(value))throw new Error('fileId invalid');
  return value;
 }
 const workflows={
  async checkInsurance(siui,{cnp,lt,operationId,requestDate=utcDateTime(),signal}){
   return siui.getInsured({pid:String(cnp),requestDate},{
    operationId,signal,
    callwmapi:{
     ...(lt?{lt}:{}),apipath:'/pacient/{cnp_pacient}',method:'POST',
     transform_path:[{from_field:'cnp',to_field:'cnp_pacient'}],
     body:{
      verificare_la_data:requestDate,
      transform_result:[
       {from_field:'pid',to_field:'cnp_pacient'},
       {from_field:'cid',to_field:'cid_pacient'},
       {from_field:'state',to_field:'stare_asigurare_cas'},
       {from_field:'categories',to_field:'categorii_cas'},
       {from_field:'states',to_field:'stari_cas'}
      ]
     }
    }
   });
  },
  async downloadPersonalisation(siui,{partnerCategory,start,stop,uic,entry,validateXSD=true,lt,operationId,signal}){
   const processing={unzip:true,...(entry?{entry}:{}),...(validateXSD===false?{validateXSD:false}:{})};
   return siui.getProviderInfo({partnerCategory,start,stop,uic:String(uic)},{
    operationId,signal,
    file:{...processing,format:'json'}, // For browser result: call.download.data.
    callwmapi:{
     ...(lt?{lt}:{}),apipath:'/cnas/personalisation',method:'POST',
     file:processing, // Executed independently on the bridge, even if browser closes.
     body:{
      partnerCategory,start,stop,uic:String(uic),
      transform_result:[{from_field:'file_json',to_field:'personalisation'}]
     }
    }
   });
  },
  async validate72h(siui,{fileId,reportXml,reportType='PARA',requestType='RQ_PARA_SRV',requestedAt,lt,operationId,signal}){
   const id=safeFileID(fileId);
   if(!requestedAt)throw new Error('requestedAt must be fixed for this operationId');
   return siui.validateReport({reportXml,reportType,requestType},{
    operationId,signal,
    callwmapi:{
     ...(lt?{lt}:{}),apipath:'/file/'+id+'/',method:'POST',
     body:{
      file_id:id,report_type:reportType,requested_at:requestedAt,
      transform_result:[{from_field:'values.0',to_field:'cnas_validation_response_xml'}]
     }
    }
   });
  }
 };
 if(typeof module!=='undefined'&&module.exports)module.exports=workflows;
 else root.SIUIWorkflows=workflows;
})(typeof globalThis!=='undefined'?globalThis:window);
