const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync(__dirname + '/app.js', 'utf8');
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
  const node={};const calls=[];
  const ctx=scope(['loadECardDashboard'],{document:{getElementById:()=>node},api:async path=>{
    calls.push(path);return path.endsWith('health')?{ok:true,hardware_supported:false,sdk_order:['2.0.0.4','2.0.0.3']}:{reader_name:'<reader>',um_host:'um.example',um_port:443,max_items:100000};
  }});
  await ctx.loadECardDashboard();
  assert.deepEqual(calls,['/api/ecard/health','/api/ecard/settings']);
  assert.match(node.textContent,/2.0.0.4 → 2.0.0.3/);assert.match(node.textContent,/necesită Windows/);
  assert.equal(node.innerHTML,undefined);
});
