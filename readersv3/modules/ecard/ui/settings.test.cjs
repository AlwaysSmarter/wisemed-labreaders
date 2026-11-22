const {JSDOM}=require('jsdom');
const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
test('SDK grid, unique choices, reordering and connected reader selection apply immediately',async()=>{
 const calls=[];let readers=['Gemalto USB Smart Card Reader 0'];
 const html=fs.readFileSync(__dirname+'/index.html','utf8');
 const dom=new JSDOM(html,{url:'http://localhost/ecard/',runScripts:'dangerously',beforeParse(w){w.fetch=async(url,options={})=>{
  calls.push({url,...options});const path=url.replace('/api/ecard/','');
  const data=path==='settings'?{sdk_order:['2.0.0.4','1.3.0.4'],reader_name:'Saved reader',um_host:'umceas.siui.ro',um_port:443}:path==='health'?{available_sdks:['2.0.0.4','2.0.0.3','1.3.0.4']}:path==='jobs'?{id:'j'}:path==='jobs/j'?{job:{state:'completed'}}:path.endsWith('/results')?{items:[{data:readers}]}:{ok:true,restart_required:false};
  return {ok:true,status:200,json:async()=>data};
 }}});
 try{
 await new Promise(r=>setImmediate(r));const d=dom.window.document,$=id=>d.getElementById(id),rows=()=>[...d.querySelectorAll('#sdk-rows tr')];
 assert.equal(rows().length,2);assert.equal($('reader').tagName,'SELECT');
 assert.equal(rows()[0].querySelector('option[value="1.3.0.4"]').disabled,true);
 $('sdk-add').click();assert.equal(rows().length,3);assert.equal($('sdk-add').disabled,true);
 rows()[2].querySelector('button').click();assert.equal(rows()[1].querySelector('select').value,'2.0.0.3');
 rows()[0].querySelectorAll('button')[2].click();assert.equal(rows().length,2);
 const choice=rows()[0].querySelector('select');choice.value='2.0.0.4';choice.dispatchEvent(new dom.window.Event('change'));
 await $('detect-readers').onclick();assert.equal($('reader').value,'Saved reader');assert.match($('reader').selectedOptions[0].textContent,/nedetectat/);
 assert.equal($('reader').options.length,3);$('reader').value=readers[0];
 await $('save').onclick();const saved=JSON.parse(calls.find(c=>c.method==='PUT').body);
 assert.deepEqual(saved.sdk_order,['2.0.0.4','1.3.0.4']);assert.equal(saved.reader_name,readers[0]);assert.match($('message').textContent,/salvată și aplicată/);
 readers=[];await $('detect-readers').onclick();assert.equal($('reader').value,saved.reader_name);assert.match($('reader-notice').textContent,/Niciun cititor/);
 rows()[0].querySelectorAll('button')[2].click();assert.equal(rows().length,1);assert.equal(rows()[0].querySelectorAll('button')[2].disabled,true);
 }finally{dom.window.close()}
});
