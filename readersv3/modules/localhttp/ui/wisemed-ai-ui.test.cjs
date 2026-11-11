const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync(__dirname + '/app.js', 'utf8');
function scope(names, globals) {
  const functions = names.map(name => {
    const start = source.indexOf(`function ${name}(`);
    assert.ok(start >= 0);
    return source.slice(source.slice(start-6,start)==='async '?start-6:start,source.indexOf('\n}',start)+2);
  });
  const ctx=vm.createContext(globals);vm.runInContext(functions.join('\n'),ctx);return ctx;
}
test('AI settings keep server and WSM, hiding laboratory navigation and fields', () => {
  const nodes=new Map();
  const node=id=>{if(!nodes.has(id))nodes.set(id,{hidden:false,closest(){return this;}});return nodes.get(id);};
  const links=['reader','wss','analytes','qc','daily-details','daily-analysis-filters','astm-specimens'].map(name=>({dataset:{settingsSubview:name}}));
  const state={aiMode:true};
  const ctx=scope(['applyWiseMEDAIUI'],{state,window:{},els:{readerSettingsForm:{elements:{namedItem:node}},settingsSubmenu:{},navSublinks:links},barcodeNavButton:node,document:{getElementById:node,querySelectorAll:()=>[node('transport'),node('results')]}});
  ctx.applyWiseMEDAIUI();
  for(const id of ['overview','orders','qc','daily-details','analyzer-pill','result-sync-pill','analyzer_name','analyzer_code','db_name','sqlite_path','transport','results'])assert.equal(node(id).hidden,true,id);
  assert.deepEqual(links.map(link=>link.hidden),[false,false,true,true,true,true,true]);
  assert.equal(node('wisemed-ai-dashboard-nav').hidden,false);
  assert.equal(node('reader-eyebrow').textContent,'UTILITAR');
  assert.equal(state.utilityMode,true);
});
test('AI deep links cannot activate QC, orders or laboratory settings locally or through WSM',()=>{
  for(const remote of [false,true])for(const path of ['/','/qc','/orders','/daily-details','/settings/qc','/settings/analytes','/settings/reader','/settings/wss','/debug/wsm']) {
    const state={aiMode:true};
    const ctx=scope(['currentUIPath','viewFromLocation','settingsSubViewFromLocation','pathForView'],{state,window:{location:{pathname:remote?'/control/':path},WSM_REMOTE:remote?{route:()=>path}:null}});
    assert.equal(ctx.viewFromLocation(),path==='/debug/wsm'?'wsm':'analytes');
    state.settingsSubView=ctx.settingsSubViewFromLocation();
    assert.equal(state.settingsSubView,path==='/settings/wss'?'wss':'reader');
    assert.equal(ctx.pathForView('analytes'),'/settings/'+state.settingsSubView);
  }
});
test('AI mount detects protocol or application ID and loads no laboratory APIs',async()=>{
  for(const reader of [{protocol:'wisemed-ai',comm_type:'utility'},{id:'wisemed-ai',comm_type:'file'}]) {
    const calls=[],state={session:{user_type:0}};
    const globals={state,els:new Proxy({},{get:()=>({})}),isESignatureMode:()=>false,settingsSubViewFromLocation:()=>'reader',viewFromLocation:()=>'analytes',formatReaderPrimaryLabel:()=>'',formatReaderSecondaryLabel:()=>'',applyBarcodeModeUI(){state.utilityMode=true;},applyWiseMEDAIUI(){}};
    for(const name of ['syncAppVersionUI','syncLanguageControls','applyLanguage','toggleLogsAccess','syncDebugNavVisibility','syncOrderControls','activateSettingsSubView','renderRoundSelect','activateView','restartLogPolling','restartStatusPolling'])globals[name]=()=>{};
    for(const name of ['loadWSSStatus','loadStatus','loadLogs','loadReaderSettings','refreshAppUpdateStatus'])globals[name]=async()=>calls.push(name);
    await scope(['mountDashboard'],globals).mountDashboard({reader});
    assert.equal(state.aiMode,true);
    assert.deepEqual(calls,['loadWSSStatus','loadStatus','loadLogs','loadReaderSettings','refreshAppUpdateStatus']);
  }
});
test('AI status polling does not request laboratory dashboard or result synchronization',async()=>{
  const els={analyzerPill:{hidden:false}};
  const ctx=scope(['loadDashboard','loadResultSyncStatus','updateConnectionPills'],{state:{aiMode:true},els});
  await ctx.loadDashboard();await ctx.loadResultSyncStatus();ctx.updateConnectionPills({analyzer_connected:true});
  assert.equal(els.analyzerPill.hidden,true);
});
