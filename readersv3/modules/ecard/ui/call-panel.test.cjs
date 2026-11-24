const {JSDOM}=require('jsdom'),{test}=require('node:test'),assert=require('node:assert/strict'),fs=require('fs');
test('inline call diagnostics filter, limit and clear without deleting server data',async()=>{
 const calls=[];const dom=new JSDOM(fs.readFileSync(__dirname+'/index.html','utf8'),{url:'http://localhost/ecard/',runScripts:'dangerously',beforeParse(w){w.fetch=async(url,opt={})=>{calls.push({url,...opt});let data=url.endsWith('settings')?{sdk_order:['2.0.0.4'],um_host:'localhost',um_port:443}:url.endsWith('health')?{available_sdks:['2.0.0.4']}:url.includes('/events?')?{items:[],cursor:0}:url.endsWith('/sign/verify')?{valid:false,cryptographic_valid:true,reason:'certificate_outside_validity_period'}:{ok:true};return {ok:true,json:async()=>data};};}});
 try{await new Promise(r=>setImmediate(r));const $=id=>dom.window.document.getElementById(id);
 for(let i=0;i<12;i++)await $('verify-signature-button').onclick();assert.match($('verify-result').textContent,/criptografică este corectă/);assert.match($('verify-result').textContent,/valabilitate/);
 $('call-limit').value='10';$('call-limit').onchange();assert.equal($('call-trace').children.length,10);
 $('call-level').value='info';$('call-level').onchange();assert.equal($('call-trace').children.length,0);
 $('call-level').value='error';$('call-level').onchange();assert.equal($('call-trace').children.length,10);
 $('clear-call').click();assert.equal($('call-trace').children.length,0);assert(!calls.some(c=>c.method==='DELETE'));
 }finally{dom.window.close()}
});
