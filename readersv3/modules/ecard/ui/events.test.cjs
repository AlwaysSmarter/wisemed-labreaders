const {JSDOM}=require('jsdom'),{test}=require('node:test'),assert=require('node:assert/strict'),fs=require('fs');
test('event stream advances cursor and filters without losing events',async()=>{
 const urls=[];const dom=new JSDOM(fs.readFileSync(__dirname+'/index.html','utf8'),{url:'http://localhost/ecard/',runScripts:'dangerously',beforeParse(w){w.fetch=async url=>{urls.push(url);const data=url.endsWith('settings')?{sdk_order:['2.0.0.4'],reader_name:'',um_host:'localhost',um_port:443}:url.endsWith('health')?{available_sdks:['2.0.0.4']}:url.endsWith('after=0')?{cursor:1,items:[{id:1,time:'now',source:'pcsc',event:'card_inserted',data:{reader:'Gemalto'}}]}:{cursor:1,items:[]};return {ok:true,json:async()=>data};};}});
 try{await new Promise(r=>setImmediate(r));const $=id=>dom.window.document.getElementById(id);await $('events-refresh').onclick();assert.match($('event-log').textContent,/card_inserted/);await $('events-refresh').onclick();assert(urls.some(u=>u.endsWith('events?after=1')));assert.equal($('event-log').children.length,1);$('events-filter').value='UM';$('events-filter').oninput();assert.equal($('event-log').children.length,0);$('events-filter').value='';$('events-filter').oninput();assert.equal($('event-log').children.length,1);}finally{dom.window.close()}
});

test('automatic read only handles fresh physical insertions for the selected reader while idle',async()=>{
 const dom=new JSDOM(fs.readFileSync(__dirname+'/index.html','utf8'),{url:'http://localhost/ecard/',runScripts:'dangerously',beforeParse(w){w.fetch=async url=>({ok:true,json:async()=>url.endsWith('settings')?{sdk_order:['2.0.0.4'],um_host:'localhost',um_port:443}:url.endsWith('health')?{available_sdks:[]}:{items:[],cursor:0}});}});
 try{
  await new Promise(r=>setImmediate(r));
  const w=dom.window,$=id=>w.document.getElementById(id),calls=[];
  w.startOperation=async item=>calls.push(item.id);
  for(const k of ['cif','cui','number','date','house','provider_type'])$('c_'+k).value=k==='date'?'2026-11-24':'test';
  const event={id:42,source:'pcsc',event:'card_inserted',time:new Date(Date.now()+100).toISOString(),data:{reader:'Reader'}};
  await w.autoReadInserted([event]);assert.deepEqual(calls,[42]);
  await w.autoReadInserted([{...event,job:'correlated'}]);
  await w.autoReadInserted([{...event,time:'2020-01-01T00:00:00Z'}]);
  await w.autoReadInserted([event,{...event,event:'card_removed'}]);
  $('start').disabled=true;await w.autoReadInserted([event]);$('start').disabled=false;
  $('c_cif').value='';await w.autoReadInserted([event]);
  assert.deepEqual(calls,[42]);
 }finally{dom.window.close();}
});
