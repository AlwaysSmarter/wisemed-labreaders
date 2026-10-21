const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const path=require('node:path');
const source=fs.readFileSync(path.join(__dirname,'../../../examples/browser/siui-prescriptions.js'),'utf8');
function fixture(result){
 const calls=[],callbacks=[],toasts=[],spins=[];
 const ctx=vm.createContext({crypto:{randomUUID:()=> 'lookup-id'},objWSMSIUI:{general:{getPrescriptionSeriesInfo:async(...args)=>{calls.push(args);return result;},generatePrescriptionSeries:async(...args)=>{calls.push(args);return result;}}},$:{aceOverWatch:{utilities:{isVoid:(value)=>value==null||value==='',runIt:(fn,...args)=>fn(...args)},toast:{show:(...args)=>toasts.push(args)}}}});
 vm.runInContext(source+'\nthis.example=SIUIPrescriptions;',ctx);
 const target={find:()=>({addClass:name=>spins.push('add:'+name),removeClass:name=>spins.push('remove:'+name)})};
 return {example:ctx.example,calls,callbacks,toasts,spins,target,done:(...args)=>callbacks.push(args)};
}
test('aceOverWatch retrieval forwards every value and always cleans up spinner',async()=>{
 const values=['one','two',''];
 const f=fixture({success:true,data:{result:{values}}});
 const parameters={categoryCode:'MF',orgUnitCode:'TEST',uic:'TEST',subUnitCode:'TEST'};
 await f.example.getPrescriptionSeriesSIUI(f.target,parameters,f.done);
 assert.equal(f.calls[0][0],parameters);assert.equal(f.callbacks[0][0],true);assert.equal(f.callbacks[0][1],values);
 assert.deepEqual(f.spins,['add:fa-spin','remove:fa-spin']);assert.equal(f.toasts.length,0);
});
test('aceOverWatch failures invoke callback and toast without assuming a carnet schema',async()=>{
 for(const result of [{success:false,error:'offline'},{success:true,data:{result:{}}}]){
  const f=fixture(result);await f.example.getPrescriptionSeriesSIUI(f.target,{},f.done);
  assert.equal(f.callbacks[0][0],false);assert.equal(f.toasts[0][0],'error');
  assert.deepEqual(f.spins,['add:fa-spin','remove:fa-spin']);
 }
 const f=fixture({success:false,error:'offline'});await f.example.getPrescriptionSeriesSIUI(null,{},f.done);
 assert.equal(f.spins.length,0);
});
test('aceOverWatch generation requires and reuses the caller operation ID',async()=>{
 const f=fixture({success:true,data:{result:{values:['receipt']}}});
 await f.example.generatePrescriptionSeriesSIUI(f.target,{},'',f.done);
 assert.equal(f.calls.length,0);assert.equal(f.callbacks[0][0],false);
 for(let i=0;i<2;i++)await f.example.generatePrescriptionSeriesSIUI(f.target,{},'stable-generation-id',f.done);
 assert.equal(f.calls.length,2);assert.ok(f.calls.every(c=>c[1].operationId==='stable-generation-id'));
});
