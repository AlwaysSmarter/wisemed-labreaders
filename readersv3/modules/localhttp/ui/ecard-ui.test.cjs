const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync(__dirname + '/app.js', 'utf8');

test('embedded eCard bridge accepts operation/report/events routes and rejects foreign paths', async () => {
  const start=source.indexOf('window.wiseMEDECardRequest=async(');
  assert.ok(start>=0);
  const end=source.indexOf('\n};',start)+3;
  const calls=[];
  const ctx=vm.createContext({window:{},fetch:async(path,options)=>{calls.push({path,options});return {ok:true,status:200,json:async()=>({ok:true})};}});
  vm.runInContext(source.slice(start,end),ctx);
  for(const path of ['operations','jobs/job-1/report','jobs/job-1/report?part=0','events?job_id=job-1&after=0','events/subscribe','history?offset=0','jobs','jobs/job-1/results','sign/batch?async=true','health']){
    const response=await ctx.window.wiseMEDECardRequest(path,'GET');
    assert.equal(response.status,200,path);
    assert.equal(calls.at(-1).path,'/api/ecard/'+path);
  }
  const count=calls.length;
  for(const path of ['../settings','jobs/../settings','jobs/%2e%2e/report','/api/ecard/operations','https://example.test/','operations/unknown','jobs/job-1/unknown']){
    await assert.rejects(()=>ctx.window.wiseMEDECardRequest(path,'GET'),/Endpoint eCard invalid/,path);
  }
  assert.equal(calls.length,count);
});
function scope(names, globals) {
  const functions = names.map(name => {
    const start = source.indexOf(`function ${name}(`);
    assert.ok(start >= 0);
    return source.slice(source.slice(start-6, start) === 'async ' ? start-6 : start, source.indexOf('\n}', start)+2);
  });
  const ctx = vm.createContext({isHistoryMode:()=>false,isSIUIMode:()=>false,...globals});
  vm.runInContext(functions.join('\n'), ctx);
  return ctx;
}
test('ecard local and remote routes exclude laboratory subviews', () => {
  for (const remote of [false, true]) for (const path of ['/', '/orders', '/qc', '/daily-details', '/settings/analytes', '/settings/qc', '/settings/reader', '/settings/wss', '/settings/ecard', '/debug/ecard']) {
    const state = {ecardMode:true};
    const ctx = scope(['currentUIPath','viewFromLocation','settingsSubViewFromLocation','pathForView'], {
      state, window:{location:{pathname:remote?'/control/':path}, WSM_REMOTE:remote?{route:()=>path}:null}
    });
    assert.equal(ctx.viewFromLocation(), path.startsWith('/settings')?'analytes':path==='/debug/ecard'?'debug':'overview');
    state.settingsSubView = ctx.settingsSubViewFromLocation();
    assert.equal(state.settingsSubView, path === '/settings/wss'?'wss':path==='/settings/ecard'?'ecard':'reader');
    assert.equal(ctx.pathForView('analytes'), '/settings/'+state.settingsSubView);
  }
});
test('ecard hides laboratory navigation, charts and configuration fields', () => {
  const nodes = new Map();
  const node = id => {if (!nodes.has(id)) nodes.set(id,{hidden:false,closest(){return this;},querySelector(){return null;}});return nodes.get(id);};
  const links = ['reader','wss','analytes','qc','daily-details','astm-specimens','siui','pad'].map(name=>({dataset:{settingsSubview:name}}));
  const sections = [node('transport'),node('results')];
  const ctx = scope(['applyECardUI'], {
    state:{},window:{},els:{readerSettingsForm:{elements:{namedItem:node},querySelector:()=>null},settingsSubmenu:{},navSublinks:links},
    barcodeNavButton:node, document:{getElementById:node,querySelectorAll:()=>sections}
  });
  ctx.applyECardUI();
  for (const id of ['orders','qc','daily-details','analyzer-pill','result-sync-pill','today-donut','qc-summary','line-chart','analyzer_name','analyzer_code','db_name','sqlite_path','transport','results']) assert.equal(node(id).hidden,true,id);
  assert.deepEqual(links.map(link=>link.hidden),[false,false,true,true,true,true,true,true]);
});
test('ecard mount loads only utility data', async () => {
  const calls = [];
  const state = {session:{user_type:1},readerInfo:{}};
  const globals = {state, els:new Proxy({}, {get:()=>({})}), isESignatureMode:()=>false,
    settingsSubViewFromLocation:()=> 'reader', viewFromLocation:()=> 'overview',
    formatReaderPrimaryLabel:()=>'',formatReaderSecondaryLabel:()=>'',
    applyBarcodeModeUI(){state.utilityMode=true;state.barcodeMode=false;}};
  for(const name of ['syncAppVersionUI','syncLanguageControls','applyLanguage','toggleLogsAccess','syncDebugNavVisibility','syncOrderControls','activateSettingsSubView','renderRoundSelect','activateView','restartLogPolling','restartStatusPolling']) globals[name]=()=>{};
  for(const name of ['loadWSSStatus','loadStatus','loadLogs','loadReaderSettings','refreshAppUpdateStatus']) globals[name]=async()=>{calls.push(name);};
  const ctx = scope(['mountDashboard'],globals);
  await ctx.mountDashboard({reader:{protocol:'ecard'}});
  assert.equal(state.ecardMode,true);
  assert.deepEqual(calls,['loadWSSStatus','loadStatus','loadLogs','loadReaderSettings','refreshAppUpdateStatus']);
});

test('eCard dashboard reads only utility status and displays configured SDKs', async () => {
  const node={},hostNode={hidden:true};const calls=[];
  const ctx=scope(['loadECardDashboard'],{state:{readerInfo:{id:'wisemed-pias-ecard',equipment_id:18}},document:{getElementById:id=>id==='ecard-status'?node:hostNode},api:async path=>{
    calls.push(path);return path.endsWith('health')?{ok:true,hardware_supported:false,sdk_order:['2.0.0.4','2.0.0.3'],versions:{app_version:'1.0.30',ecard_host_version:'1.0.29',ecard_host_sha256:'host-hash'}}:{reader_name:'<reader>',um_host:'um.example',um_port:443,max_items:100000};
  }});
  await ctx.loadECardDashboard();
  assert.deepEqual(calls,['/api/ecard/health','/api/ecard/settings']);
  assert.match(node.textContent,/2.0.0.4 → 2.0.0.3/);assert.match(node.textContent,/necesită Windows/);
  assert.equal(node.innerHTML,undefined);
  assert.match(node.textContent,/PIAS eCard: 1.0.30 · ECardHost: 1.0.29/);
  assert.match(node.textContent,/ID echipament: 18/);
  assert.equal(hostNode.textContent,'· ECardHost: 1.0.29');
  assert.equal(hostNode.hidden,false);
  assert.equal(hostNode.title,'ECardHost SHA-256: host-hash');
});

test('eCard settings replaces WSS selection and switching back clears it', () => {
  const {JSDOM} = require('jsdom');
  const dom = new JSDOM(`<button data-settings-subview="wss" class="active"></button><button data-settings-subview="ecard"></button><article id="settings-panel-wss"></article><article id="settings-panel-ecard" hidden></article>`, {url:'https://example.test/settings/wss'});
  const document = dom.window.document;
  const state = {ecardMode:true,settingsSubView:'wss'};
  const ctx = scope(['activateSettingsSubView'], {
    state, document, window:dom.window,
    els:{navSublinks:[...document.querySelectorAll('button')]},
    mountECardPanel:()=>{document.getElementById('settings-panel-ecard').hidden=false;},
    renderTopbarTitle:()=>{},isESignatureMode:()=>false,
    renderWSSPanel:()=>{},loadWSSStatus:async()=>{}
  });
  ctx.activateSettingsSubView('ecard');
  assert.equal(state.settingsSubView,'ecard');
  assert.equal(dom.window.location.pathname,'/settings/ecard');
  assert.equal(document.querySelector('.active').dataset.settingsSubview,'ecard');
  assert.equal(document.getElementById('settings-panel-wss').hidden,true);
  assert.equal(document.getElementById('settings-panel-ecard').hidden,false);
  ctx.activateSettingsSubView('wss');
  assert.equal(state.settingsSubView,'wss');
  assert.equal(document.querySelector('.active').dataset.settingsSubview,'wss');
  assert.equal(document.getElementById('settings-panel-ecard').hidden,true);
});


test('eCard sidebar shows registered equipment ID separately from application ID', () => {
  for(const equipmentID of [18,0,undefined]){
    const els={readerIdentityIcon:{},readerIdentityTitle:{},readerIdentitySubtitle:{},readerSummaryList:{}};
    const ctx=scope(['renderReaderSidebar'],{state:{ecardMode:true,readerInfo:{id:'wisemed-pias-ecard',equipment_id:equipmentID}},els});
    ctx.renderReaderSidebar();
    assert.equal(els.readerIdentitySubtitle.textContent,'PIAS eCard · ID echipament: '+(equipmentID===18?'18':'neînregistrat'));
  }
});
