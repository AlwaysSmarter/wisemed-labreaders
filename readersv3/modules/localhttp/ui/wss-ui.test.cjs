const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync(__dirname + '/app.js', 'utf8');
function functionSource(name) {
  const start = source.indexOf(`function ${name}(`);
  assert.ok(start >= 0, name);
  const end = source.indexOf('\n}', start);
  return source.slice(source.slice(start-6,start) === "async " ? start-6 : start, end + 2);
}
function context(names, globals = {}) {
  const scope = vm.createContext({Date, siuiPollGeneration:0, isSIUIMode:()=>false, ...globals});
  vm.runInContext(names.map(functionSource).join('\n'), scope);
  return scope;
}
test('retry countdown uses server deadline, rounds up, clamps at zero', () => {
  const scope = context(['wssRetrySeconds']);
  const now = Date.parse('2026-09-22T00:00:00Z');
  assert.equal(scope.wssRetrySeconds({next_retry_at:'2026-09-22T00:00:30Z'}, now, now),30);
  assert.equal(scope.wssRetrySeconds({next_retry_at:'2026-09-22T00:00:30Z'}, now, now+29500),1);
  assert.equal(scope.wssRetrySeconds({next_retry_at:'2026-09-22T00:00:30Z'}, now, now+31000),0);
  assert.equal(scope.wssRetrySeconds({retry_in_seconds:30},now,now+11000),19);
});
test('all application modes resolve WSS deep link and preserve remote logical route', () => {
  for (const barcode of [false,true]) for (const signature of [false,true]) {
    const scope = context(['currentUIPath','viewFromLocation','settingsSubViewFromLocation','pathForView'], {
      state:{barcodeMode:barcode,settingsSubView:'wss'}, isESignatureMode:()=>signature,
      window:{location:{pathname:'/control/'},WSM_REMOTE:{route:()=>'/settings/wss'}}
    });
    assert.equal(scope.viewFromLocation(),'analytes');
    assert.equal(scope.settingsSubViewFromLocation(),'wss');
    assert.equal(scope.pathForView('analytes'),'/settings/wss');
  }
});
test('debug selection refuses empty destination', () => {
  const select = {value:''};
  const scope = context(['wssSelectedTarget'],{document:{getElementById:()=>select}});
  assert.throws(()=>scope.wssSelectedTarget(),/Selectează/);
  select.value='connection-42';
  assert.equal(scope.wssSelectedTarget(),'connection-42');
});
test('top badge shows reconnect countdown, manual pause and live connection', () => {
  const nodes = Object.fromEntries(['wss-status-label','wss-pill','wss-dot','wss-local-status'].map(id=>[id,{style:{},classList:{toggle(){}}}]));
  const state = {wss:{snapshot:{status:{enabled:true,retry_in_seconds:30}},receivedAt:Date.now()}};
  const scope = context(['wssRetrySeconds','renderWSSBadge'],{state,document:{getElementById:id=>nodes[id]}});
  scope.renderWSSBadge(); assert.match(nodes['wss-status-label'].textContent,/30s/);
  state.wss.snapshot.status.paused=true;
  scope.renderWSSBadge(); assert.match(nodes['wss-status-label'].textContent,/manual/);
  state.wss.snapshot.status.connected=true;
  scope.renderWSSBadge(); assert.equal(nodes['wss-status-label'].textContent,'WSS: conectat');
});
test('remote console handoff requires trusted window and exact origin and refreshes on reload', async () => {
  let listener;
  const child={closed:false};
  const messages=[];
  child.postMessage=(message,origin)=>messages.push({message,origin});
  const state={};
  const scope=context(['initWSSUI'], {state, URL, api:async()=>({url:'https://hub.example/control/',ticket:'fresh-on-reload'}), document:{getElementById:()=>({addEventListener(){}})},window:{addEventListener:(type,fn)=>{listener=fn}},setInterval(){}});
  scope.initWSSUI();
  state.wss.windows.set(child,{origin:'https://hub.example',ticket:'single-use',equipmentID:'42'});
  listener({source:child,origin:'https://attacker.example',data:{type:'wsm-control-ready'}});
  listener({source:{},origin:'https://hub.example',data:{type:'wsm-control-ready'}});
  assert.equal(messages.length,0);
  listener({source:child,origin:'https://hub.example',data:{type:'wsm-control-ready'}});
  assert.equal(messages.length,1);
  assert.equal(messages[0].origin,'https://hub.example');
  assert.equal(messages[0].message.ticket,'single-use');
  await listener({source:child,origin:'https://hub.example',data:{type:'wsm-control-ready'}});
  assert.equal(messages.length,2);
  assert.equal(messages[1].message.ticket,'fresh-on-reload');
});

test('remote logout closes browser transport without target logout API', async () => {
  let closed=0, windowClosed=0, called=0;
  const scope=context(['onLogout'],{window:{WSM_REMOTE:{close(){closed++}},close(){windowClosed++}},state:{},els:{dashboardView:{}},closeESignatureDemo(){},api(){called++},document:{getElementById:()=>null,body:{replaceChildren(){},append(){}},createElement:()=>({})}});
  await scope.onLogout();
  assert.equal(closed,1); assert.equal(windowClosed,1); assert.equal(called,0);
});
test('remote worksheets and worklists route through JSON preview, never direct HTTP popup', () => {
  const calls=[];
  const scope=context(['onPrintDailyWorksheetClick','onWorklistOrdersClick'], {URLSearchParams,window:{WSM_REMOTE:{},open(){throw Error('HTTP popup forbidden')}},state:{dailyDetailsDate:'2026-09-22',dailyDetailsScopeTab:'day_round_analyte',dailyDetailsRoundNo:2,dailyDetailsAnalyteTag:'TEST',selectedOrderDate:'2026-09-22',selectedRoundNo:3},openRemotePrint:(path,params)=>calls.push([path,Object.fromEntries(params)])});
  scope.onPrintDailyWorksheetClick(); scope.onWorklistOrdersClick();
  assert.equal(calls[0][0],'/api/daily-details/worksheet'); assert.equal(calls[0][1].round_no,'2'); assert.equal(calls[0][1].analyte_tag,'TEST');
  assert.equal(calls[1][0],'/api/orders/worklist'); assert.equal(calls[1][1].round_no,'3');
});
test('remote target disconnect and transitive commands are refused before API calls',async()=>{
  let calls=0;
  const scope=context(['wssControl','wssDebug'],{window:{WSM_REMOTE:{}},api(){calls++}});
  await assert.rejects(scope.wssControl('disconnect'),/permanentă/);
  await assert.rejects(scope.wssDebug('list'),/consola locală/);
  assert.equal(calls,0);
});
test('print renderer displays computed values as plain text without interpreting markup',()=>{
  const nodes=[];
  function node(tag){const result={tag,children:[],append(...items){this.children.push(...items)},replaceChildren(){this.children=[]},addEventListener(){},createTHead(){return node('thead')},createTBody(){return node('tbody')},insertRow(){return node('tr')},insertCell(){return node('td')}}; nodes.push(result); return result;}
  const doc={documentElement:{},head:node('head'),body:node('body'),createElement:node};
  const scope=context(['renderRemotePrintDocument']);
  scope.renderRemotePrintDocument(doc,{title:'<script>title</script>',columns:['Name'],rows:[['<img src=x onerror=alert(1)>']],details:[{label:'Sample',value:'<b>actual data</b>'}]},()=>{});
  assert.equal(doc.title,'<script>title</script>');
  assert.ok(nodes.some(item=>item.tag==='td'&&item.textContent==='<img src=x onerror=alert(1)>'));
  assert.ok(nodes.every(item=>!Object.hasOwn(item,'innerHTML')));
});

test('WSS secret is a write-only masked field and status never populates it', async () => {
  assert.match(source, /name="device_secret" type="password" autocomplete="new-password"/);
  const secretField = {type:'password',value:'typed-but-never-returned'};
  const form = {dataset:{},elements:{namedItem:key=>key==='device_secret'?secretField:null}};
  const state = {wss:{loading:false}};
  const scope = context(['loadWSSStatus'],{state,renderWSSBadge(){},api:async()=>({settings:{device_secret:'must-not-display',device_secret_configured:true},trace:[]}),document:{getElementById:id=>id==='wss-settings-form'?form:null}});
  await scope.loadWSSStatus();
  assert.equal(secretField.value,'');
  assert.match(secretField.placeholder,/Cheie salvată/);
  assert.match(source,/await api\("\/api\/wss\/settings"[^\n]+\n\s+form.elements.namedItem\("device_secret"\).value = "";/);
});
