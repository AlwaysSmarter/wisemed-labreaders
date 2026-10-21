const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
function explorer(){
 class Element{
  constructor(){this.style={};this.children=[];this.value='';this.textContent='';this.hidden=false;this.disabled=false;this.classList={add(){}}}
  append(...elements){for(const e of elements){e.parent=this;this.children.push(e)}}
  replaceChildren(...elements){this.children=[];this.append(...elements)}
  querySelectorAll(){return this.children.filter(e=>e.tag==='input')}
  setAttribute(name,value){this[name]=value}
  remove(){this.parent.children=this.parent.children.filter(e=>e!==this)}
 }
 const elements=new Map();for(const id of ['endpoint','path-label','path-value','query','body','json-label','file-label','file','method','request-url','run','copy','add','status','response'])elements.set(id,new Element());
 const calls=[];
 const document={getElementById:id=>elements.get(id),documentElement:new Element(),createElement:tag=>Object.assign(new Element(),{tag})};
 const context=vm.createContext({document,URL,URLSearchParams,location:{origin:'https://localhost:19114',search:''},navigator:{clipboard:{writeText:async()=>{}}},performance:{now:()=>1},fetch:async(url,options)=>{calls.push({url,options});return {status:200,statusText:'OK',text:async()=>'{"success":true,"totalCount":1,"rows":[{"code":"001"}]}'}}});context.window=context;context.parent=context;
 const html=fs.readFileSync(__dirname+'/debug.html','utf8');vm.runInContext(html.match(/<script>([\s\S]*?)<\/script>/)[1],context);
 return {context,elements,calls,run:code=>vm.runInContext(code,context)}
}
test('Debug constructs multi-value URLs without executing an endpoint',()=>{
 const e=explorer();assert.equal(e.calls.length,0);
 const url=new URL(e.elements.get('request-url').textContent);
 assert.equal(url.pathname,'/api/siui-data/services');assert.deepEqual(url.searchParams.getAll('specialties[]'),['CARD','DERM']);assert.equal(url.searchParams.get('start'),'0');
 e.run(`$('endpoint').value=String(endpoints.findIndex(e=>e[0]==='DELETE'));choose();$('path-value').value='abc/def';preview()`);
 assert.ok(e.elements.get('request-url').textContent.endsWith('/imports/abc%2Fdef'));assert.equal(e.calls.length,0);
 assert.equal(e.elements.get('json-label').hidden,true);assert.equal(e.elements.get('file-label').hidden,true);
});
test('Debug executes the displayed URL and formats the wire JSON',async()=>{
 const e=explorer();await e.elements.get('run').onclick();assert.equal(e.calls.length,1);assert.equal(e.calls[0].options.method,'GET');
 assert.equal(e.calls[0].url,new URL(e.elements.get('request-url').textContent).pathname+new URL(e.elements.get('request-url').textContent).search);
 assert.ok(e.elements.get('response').textContent.includes('\n  "success": true'));assert.ok(e.elements.get('status').textContent.includes('HTTP 200 OK'));
});
test('Debug rejects invalid arrays and exposes JSON bodies only for matching operations',()=>{
 const e=explorer();e.run(`$('query').children[1].valueInput.value='[1]';preview()`);assert.equal(e.elements.get('run').disabled,true);assert.equal(e.calls.length,0);
 e.run(`$('endpoint').value=String(endpoints.findIndex(e=>e[0]==='PUT'));choose()`);assert.equal(e.elements.get('json-label').hidden,false);assert.equal(e.elements.get('file-label').hidden,true);
});
test('Embedded Debug sends API calls through the parent WSM transport',async()=>{
 const e=explorer(),remote=[];
 e.context.parent={location:{origin:'https://wsm.example.invalid'},fetch:async(url,options)=>{remote.push({url,options});return {status:200,statusText:'OK',text:async()=>'{"success":true,"totalCount":0,"rows":[]}'}}};
 e.run('preview()');await e.elements.get('run').onclick();assert.equal(e.calls.length,0);assert.equal(remote.length,1);assert.ok(remote[0].url.startsWith('/api/siui-data/services?'));assert.equal(e.elements.get('status').textContent,'HTTP 200 OK · 0 ms');
});

test('Debug parameter menus and explanations follow the selected endpoint',()=>{const e=explorer();const service=e.run('endpointParameters()');assert.ok(service.includes('specialties'));assert.ok(!service.includes('maxPriceMin'));e.run(`$('endpoint').value=String(endpoints.findIndex(e=>e[1]==='drugs'));choose()`);const fields=e.run('endpointParameters()');assert.ok(fields.includes('maxPriceMin'));assert.ok(!fields.includes('specialties'));e.run(`$('query').children[0].parameterSelect.value='maxPriceMin';$('query').children[0].parameterSelect.onchange()`);assert.ok(e.run(`$('query').children[0].children[0].children[2].textContent`).includes('limita inferioară'));});

test('Flat service explorer uses row pagination without a code cursor',()=>{
 const e=explorer();e.run(`$('endpoint').value=String(endpoints.findIndex(e=>e[1]==='simple-services'));choose()`);
 assert.ok(!e.run('endpointParameters()').includes('after'));
 assert.ok(e.run("parameterDescription('start')").includes('rând plat'));
 e.run(`$('endpoint').value=String(endpoints.findIndex(e=>e[1]==='simple-drugs'));choose()`);
 assert.ok(e.run("parameterDescription('limit')").includes('rânduri plate'));
});

test('Physician explorer exposes physician filters and source provenance',()=>{
 const e=explorer();e.run(`$('endpoint').value=String(endpoints.findIndex(e=>e[1]==='simple-physicians'));choose()`);
 const fields=e.run('endpointParameters()');for(const name of ['source','stencil','specialties','contractType','contractNo','insuranceHouse'])assert.ok(fields.includes(name));
 assert.ok(!fields.includes('after'));assert.ok(e.run("parameterDescription('stencil')").includes('Parafa'));
 assert.ok(new URL(e.elements.get('request-url').textContent).pathname.endsWith('/simple-physicians'));
 e.run(`$('endpoint').value=String(endpoints.findIndex(e=>e[1]==='physicians/{value}'));choose()`);
 assert.deepEqual(Array.from(e.run('endpointParameters()')),['source']);
});

test('Targeted explorer exposes date and narrowing filters without tenant controls',()=>{
 const e=explorer();e.run(`$('endpoint').value=String(endpoints.findIndex(e=>e[1]==='targeted_services/{value}'));choose()`);
 const fields=e.run('endpointParameters()');assert.ok(fields.includes('atDate'));assert.ok(fields.includes('specialties'));assert.ok(!fields.includes('type'));assert.ok(!fields.includes('after'));assert.ok(e.run("parameterDescription('atDate')").includes('nu pot fi lărgite'));
 assert.ok(new URL(e.elements.get('request-url').textContent).pathname.endsWith('/targeted_services/7991962'));
 assert.ok(!e.run('endpoints.map(e=>e[1]).join(" ")').includes('clients/data'));
});

test('Simple medicine explorer defaults to the CLIN source without a special-only filter',()=>{
 const e=explorer();e.run(`$('endpoint').value=String(endpoints.findIndex(e=>e[1]==='simple-drugs'));choose()`);
 const url=new URL(e.elements.get('request-url').textContent);assert.equal(url.searchParams.get('type'),'CLIN');assert.ok(!url.searchParams.has('isSpecial'));
});


test('Medicine explorer exposes AND fragment filters and preserves scalar and array URLs',()=>{
 const e=explorer();
 for(const route of ['drugs','simple-drugs','drug-compensations','drug-relations']){
  e.run(`$('endpoint').value=String(endpoints.findIndex(e=>e[1]==='${route}'));choose()`);
  const fields=e.run('endpointParameters()');
  for(const field of ['presentationMode','activeSubstance','name','concentration','pharmaceuticalForm','company','diseaseName','nationalProgramName','copaymentListName','commissionName','diagnosisName']){
   assert.ok(fields.includes(field+'All'));assert.ok(e.run(`parameterDescription('${field}All')`).includes('(AND)'));assert.ok(e.run(`parameterDescription('${field}')`).includes('(OR)'));
  }
  e.run(`$('query').replaceChildren();addRow('presentationModeAll','["cutie","20"]');addRow('activeSubstance','paracet');preview()`);
  const url=new URL(e.elements.get('request-url').textContent);
  assert.deepEqual(url.searchParams.getAll('presentationModeAll[]'),['cutie','20']);assert.equal(url.searchParams.get('activeSubstance'),'paracet');
 }
});


test('All text search variants follow services, physicians, targeted and raw record endpoints',()=>{
 const e=explorer();
 for(const [route,fields] of [['services',['name','q','specialtyName']],['simple-services',['name','q','specialtyName']],['targeted_services/{value}',['name','q','specialtyName']],['targeted-simple-services/{value}',['name','q','specialtyName']],['physicians',['name','q','specialtyName','insuranceHouseName']],['simple-physicians',['name','q','specialtyName','insuranceHouseName']],['records/{value}',['name','q']],['drug-records/{value}',['name','q']],['physician-records/{value}',['name','q']]]){
  e.run(`$('endpoint').value=String(endpoints.findIndex(e=>e[1]==='${route}'));choose()`);
  const params=e.run('endpointParameters()');for(const field of fields){assert.ok(params.includes(field+'All'),route+' '+field);assert.ok(e.run(`parameterDescription('${field}All')`).includes('(AND)'));}
 }
});
