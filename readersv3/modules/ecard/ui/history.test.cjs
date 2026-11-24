const {JSDOM}=require('jsdom'),{test}=require('node:test'),assert=require('node:assert/strict'),fs=require('fs');
test('history displays calls and deletes previous completed operations',async()=>{
 const calls=[];const dom=new JSDOM(fs.readFileSync(__dirname+'/index.html','utf8'),{url:'http://localhost/ecard/',runScripts:'dangerously',beforeParse(w){w.fetch=async(url,opt={})=>{
 calls.push({url,method:opt.method});let data={};
 if(url.endsWith('settings'))data={sdk_order:['2.0.0.4'],reader_name:'',um_host:'localhost',um_port:443};
 else if(url.endsWith('health'))data={available_sdks:['2.0.0.4']};
 else if(url.includes('/history?'))data={items:[{time:'now',method:'POST',path:'/api/ecard/jobs',status:400,request:{operation:'enroll'},response:{error:'example'}}],next_offset:1};
 else if(url.includes('/jobs?'))data={items:[{id:'old-job',operation:'read',state:'completed',created:1}],next_offset:1};
 else if(url.endsWith('/results?offset=0&limit=20'))data={items:[{ok:true}],next_offset:1};
 else if(opt.method==='DELETE')data={ok:true};
 else data={job:{state:'completed'}};
 return {ok:true,json:async()=>data};};}});
 try{await new Promise(r=>setImmediate(r));const $=id=>dom.window.document.getElementById(id);
 await $('history-refresh').onclick();assert.match($('call-history').textContent,/example/);assert.match($('call-history').textContent,/enroll/);
 await $('jobs-refresh').onclick();let buttons=$('job-history').querySelectorAll('button');await buttons[0].onclick();assert.match($('job-history').textContent,/"ok": true/);
 await buttons[1].onclick();assert(calls.some(c=>c.method==='DELETE'&&c.url.endsWith('/jobs/old-job')));
 }finally{dom.window.close()}
});
