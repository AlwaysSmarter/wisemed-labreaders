const {JSDOM}=require('jsdom');
const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const html=fs.readFileSync(__dirname+'/index.html','utf8');
async function setup(remote=false,state='completed'){
 const calls=[];
 async function request(path,method='GET',body){
  calls.push({path,method,body});
  if(path==='settings')return {sdk_order:['2.0.0.4'],reader_name:'',um_host:'umceas.siui.ro',um_port:443};
  if(path==='health')return {available_sdks:['2.0.0.4']};
  if(path==='jobs'&&method==='POST')return {id:'job-1'};
  if(path==='jobs/job-1')return {job:{state,error:state==='failed'?'sdk_error':''}};
  if(path.startsWith('jobs/job-1/results'))return path.includes('offset=20')?{items:[{id:'21',ok:true,data:'last'}],next_offset:21}:{items:Array.from({length:20},(_,i)=>({id:String(i+1),ok:true,data:i===0?'<img src=x onerror=alert(1)>':'result'})),next_offset:20};
  return {ok:true};
 }
 const dom=new JSDOM(html,{url:'http://localhost/ecard/',runScripts:'dangerously',beforeParse(w){
  if(remote)Object.defineProperty(w,'parent',{value:{wiseMEDECardRequest:async(...args)=>({ok:true,status:200,data:await request(...args)})}});
  w.fetch=async(url,o={})=>({ok:true,status:200,json:async()=>request(url.replace('/api/ecard/',''),o.method,o.body?JSON.parse(o.body):undefined)});
 }});
 await new Promise(r=>setImmediate(r));return {dom,calls,$:id=>dom.window.document.getElementById(id)};
}
for(const remote of [false,true])test(`enrollment confirmation and automatic inline results (${remote?'WSM':'local'})`,async()=>{
 const {dom,calls,$}=await setup(remote);
 try{
 $('operation').value='enroll';$('operation').onchange();
 await $('start').onclick();assert(!calls.some(c=>c.path==='jobs'));assert.match($('result-status').textContent,/Bifați confirmarea/);
 $('confirm').checked=true;await $('start').onclick();
 assert.equal(calls.find(c=>c.path==='jobs').body.confirm,true);
 assert(calls.some(c=>c.path==='jobs/job-1/start'));
 assert.equal($('results').children.length,20);assert.equal($('download'),null);
 assert.match($('results').textContent,/<img/);assert.equal($('results').querySelector('img'),null);
 assert.equal($('results-more').hidden,false);await $('results-more').onclick();assert.equal($('results').children.length,21);assert.equal($('results-more').hidden,true);
 assert.equal($('confirm').checked,false);assert.equal($('start').disabled,false);
 }finally{dom.window.close()}
});
test('debug enrollment checkbox sets explicit operation confirmation',async()=>{
 const {dom,calls,$}=await setup();
 try{
 $('endpoint').value=[...$('endpoint').options].find(o=>o.textContent.startsWith('Job: enroll')).value;
 $('endpoint').onchange();assert.match($('endpoint-confirm-label').textContent,/enroll/);
 await $('endpoint-send').onclick();assert(!calls.some(c=>c.path==='jobs'));
 $('endpoint-confirm').checked=true;await $('endpoint-send').onclick();
 assert.equal(calls.find(c=>c.path==='jobs').body.confirm,true);assert.match($('endpoint-response').textContent,/HTTP 200/);assert.equal($('endpoint-confirm').checked,false);
 }finally{dom.window.close()}
});
test('failed jobs display available partial results',async()=>{
 const {dom,$}=await setup(false,'failed');
 try{await $('start').onclick();assert.match($('result-status').textContent,/failed.*sdk_error/);assert.equal($('results').children.length,20)}finally{dom.window.close()}
});
