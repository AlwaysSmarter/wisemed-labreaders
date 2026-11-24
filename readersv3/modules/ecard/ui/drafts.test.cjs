const {JSDOM}=require('jsdom'),{test}=require('node:test'),assert=require('node:assert/strict'),fs=require('fs');
const html=fs.readFileSync(__dirname+'/index.html','utf8');
async function open(saved={},scope='station-user-a'){
 const dom=new JSDOM(html,{url:'http://localhost/ecard/',runScripts:'dangerously',beforeParse(w){
 for(const [k,v] of Object.entries(saved))w.localStorage.setItem(k,v);
 w.fetch=async url=>({ok:true,json:async()=>url.endsWith('health')?{available_sdks:['2.0.0.4'],draft_scope:scope}:{sdk_order:['2.0.0.4'],reader_name:'',um_host:'umceas.siui.ro',um_port:443}});
 }});await new Promise(r=>setImmediate(r));return dom;
}
const snapshot=w=>Object.fromEntries(Array.from({length:w.localStorage.length},(_,i)=>{const k=w.localStorage.key(i);return [k,w.localStorage.getItem(k)]}));
test('debug values survive reload; confirmations and PIN do not; clear removes saved draft',async()=>{
 let dom=await open();
 try{
  const w=dom.window,$=id=>w.document.getElementById(id);
  for(const [id,value] of Object.entries({c_cif:'123',c_number:'Contract A',cnp:'synthetic-cnp',texts:'line one\nline two',operation:'enroll',sourcehost:'WS2'})){$(id).value=value;$(id).dispatchEvent(new w.Event('change'));}
  $('confirm').checked=true;$('pinvalue').value='1234';
  $('endpoint').value=Array.from($('endpoint').options).find(o=>o.textContent.startsWith('Job: enroll')).value;$('endpoint').dispatchEvent(new w.Event('change'));
  $('endpoint-body').value=JSON.stringify({operation:'enroll',confirm:true,contract:{cif:'123'},password:'secret-test'});$('endpoint-body').dispatchEvent(new w.Event('input'));
  const saved=snapshot(w);assert(!JSON.stringify(saved).includes('secret-test'));assert(!JSON.stringify(saved).includes('1234'));
  dom.window.close();dom=await open(saved);
  const d=dom.window.document;assert.equal(d.getElementById('c_number').value,'Contract A');assert.equal(d.getElementById('texts').value,'line one\nline two');assert.equal(d.getElementById('operation').value,'enroll');assert.equal(d.getElementById('confirm').checked,false);assert.equal(d.getElementById('endpoint-confirm').checked,false);assert.equal(JSON.parse(d.getElementById('endpoint-body').value).confirm,undefined);
  const isolated=await open(saved,'station-user-b');assert.equal(isolated.window.document.getElementById('c_number').value,'');isolated.window.close();
  d.getElementById('clear-draft').click();assert.equal(dom.window.localStorage.length,0);assert.equal(d.getElementById('c_number').value,'');
 }finally{dom.window.close()}
});
test('PIN debug payload and malformed JSON are never persisted',async()=>{
 const dom=await open();try{const w=dom.window,$=id=>w.document.getElementById(id);
 $('endpoint').value=Array.from($('endpoint').options).find(o=>o.textContent.startsWith('Răspuns PIN')).value;$('endpoint').dispatchEvent(new w.Event('change'));
 $('endpoint-body').value='{"value":"secret-pin","challenge":"secret-challenge"}';$('endpoint-body').dispatchEvent(new w.Event('input'));
 assert(!JSON.stringify(snapshot(w)).includes('secret-pin'));assert(!JSON.stringify(snapshot(w)).includes('secret-challenge'));
 $('endpoint').value='0';$('endpoint').dispatchEvent(new w.Event('change'));
 $('endpoint-body').value='{invalid-json';$('endpoint-body').dispatchEvent(new w.Event('input'));
 assert(!JSON.stringify(snapshot(w)).includes('invalid-json'));
 }finally{dom.window.close()}
});
test('migration source contract is independent, remembered, and edits invalidate preview',async()=>{
 let dom=await open();try{
 const w=dom.window,$=id=>w.document.getElementById(id);
 $('c_number').value='P0272/2023';$('copy-source-contract').click();
 assert.equal($('s_number').value,'P0272/2023');
 $('s_number').value='P0272/2014';$('migrate').disabled=false;$('s_number').dispatchEvent(new w.Event('input'));
 assert.equal($('c_number').value,'P0272/2023');assert.equal($('migrate').disabled,true);
 const saved=snapshot(w);dom.window.close();dom=await open(saved);
 assert.equal(dom.window.document.getElementById('s_number').value,'P0272/2014');
 assert.equal(dom.window.document.getElementById('c_number').value,'P0272/2023');
 }finally{dom.window.close()}
});
