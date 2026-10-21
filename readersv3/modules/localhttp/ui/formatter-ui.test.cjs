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
test('formatter local and remote routes exclude laboratory subviews', () => {
  for (const remote of [false, true]) for (const path of ['/', '/orders', '/qc', '/daily-details', '/settings/analytes', '/settings/qc', '/settings/reader', '/settings/wss']) {
    const state = {formatterMode:true};
    const ctx = scope(['currentUIPath','viewFromLocation','settingsSubViewFromLocation','pathForView'], {
      state, window:{location:{pathname:remote?'/control/':path}, WSM_REMOTE:remote?{route:()=>path}:null}
    });
    assert.equal(ctx.viewFromLocation(), path.startsWith('/settings')?'analytes':'overview');
    state.settingsSubView = ctx.settingsSubViewFromLocation();
    assert.equal(state.settingsSubView, path === '/settings/wss'?'wss':'reader');
    assert.equal(ctx.pathForView('analytes'), '/settings/'+state.settingsSubView);
  }
});
test('formatter hides laboratory navigation, charts and configuration fields', () => {
  const nodes = new Map();
  const node = id => {if (!nodes.has(id)) nodes.set(id,{hidden:false,closest(){return this;}});return nodes.get(id);};
  const links = ['reader','wss','analytes','qc','daily-details','astm-specimens','siui','pad'].map(name=>({dataset:{settingsSubview:name}}));
  const sections = [node('transport'),node('results')];
  const ctx = scope(['applyHTMLFormatterUI'], {
    els:{readerSettingsForm:{elements:{namedItem:node}},settingsSubmenu:{},navSublinks:links},
    barcodeNavButton:node, document:{getElementById:node,querySelectorAll:()=>sections}
  });
  ctx.applyHTMLFormatterUI();
  for (const id of ['orders','qc','daily-details','analyzer-pill','result-sync-pill','today-donut','qc-summary','line-chart','analyzer_name','analyzer_code','db_name','sqlite_path','transport','results']) assert.equal(node(id).hidden,true,id);
  assert.deepEqual(links.map(link=>link.hidden),[false,false,true,true,true,true,true,true]);
});
test('formatter mount loads only utility data', async () => {
  const calls = [];
  const state = {session:{user_type:1},readerInfo:{}};
  const globals = {state, els:new Proxy({}, {get:()=>({})}), isESignatureMode:()=>false,
    settingsSubViewFromLocation:()=> 'reader', viewFromLocation:()=> 'overview',
    formatReaderPrimaryLabel:()=>'',formatReaderSecondaryLabel:()=>'',
    applyBarcodeModeUI(){state.utilityMode=true;state.barcodeMode=false;}};
  for(const name of ['syncAppVersionUI','syncLanguageControls','applyLanguage','toggleLogsAccess','syncDebugNavVisibility','syncOrderControls','activateSettingsSubView','renderRoundSelect','activateView','restartLogPolling','restartStatusPolling']) globals[name]=()=>{};
  for(const name of ['loadWSSStatus','loadStatus','loadLogs','loadReaderSettings','refreshAppUpdateStatus']) globals[name]=async()=>{calls.push(name);};
  const ctx = scope(['mountDashboard'],globals);
  await ctx.mountDashboard({reader:{protocol:'html-print-formatter'}});
  assert.equal(state.formatterMode,true);
  assert.deepEqual(calls,['loadWSSStatus','loadStatus','loadLogs','loadReaderSettings','refreshAppUpdateStatus']);
});
test('formatter rendering returns HTML as text through the shared API', async () => {
  const html = "<html>\n<p title=\"quoted\">'single' `backticks` \\ text</p>\n</html>";
  const nodes = {'formatter-html':{value:html},'formatter-html-link':{value:''},'formatter-timeout':{value:'120000'},'formatter-render':{},'formatter-pdf-direct':{},'formatter-notice':{},'formatter-output':{},'formatter-pdf-download':{removeAttribute(){}}};
  const ctx = scope(['renderHTMLFormatter','clearHTMLFormatterPDF'], {window:{WSM_REMOTE:null},state:{},URL:{createObjectURL:()=> 'blob:pdf',revokeObjectURL(){}},Blob,Uint8Array,atob,document:{getElementById:id=>nodes[id]},setButtonLoading(){},api:async(path, options)=>{
    if(path.endsWith('/token')) return {token:'signed-test'};
    assert.equal(options.headers.Authorization,'Bearer signed-test');
    assert.equal(path,'/api/html-print-formatter/pdf');assert.equal(options.method,'POST');
    assert.deepEqual(JSON.parse(options.body),{html,timeout_ms:120000,wait_for_print_ready:true});return {html:'<script>test()</script>',pdf_base64:btoa('%PDF-test'),request_id:'test'};
  }});
  await ctx.renderHTMLFormatter();
  assert.equal(nodes['formatter-output'].value,'<script>test()</script>');
  assert.match(nodes['formatter-notice'].textContent,/test/);
  assert.equal(nodes['formatter-pdf-download'].href,'blob:pdf');
  assert.equal(nodes['formatter-pdf-download'].hidden,false);
});
test('status polling keeps the analyzer badge hidden', () => {
  const els = {analyzerPill:{hidden:false}};
  const ctx = scope(['updateConnectionPills'],{state:{formatterMode:true},els});
  ctx.updateConnectionPills({analyzer_connected:false});
  assert.equal(els.analyzerPill.hidden,true);
});
test('page errors retain their readable message in the utility UI', async () => {
  const ctx = scope(['api'],{fetch:async()=>({ok:false,status:422,json:async()=>({ok:false,error:{code:'print_error',message:'Pagination failed'}})})});
  await assert.rejects(()=>ctx.api('/api/html-print-formatter/render'),/Pagination failed/);
});

test('formatter sends URL input and rejects ambiguous sources or invalid timeouts', async () => {
  const nodes = {'formatter-html':{value:''},'formatter-html-link':{value:' https://example.org/report.html '},'formatter-timeout':{value:'45000'},'formatter-render':{},'formatter-pdf-direct':{},'formatter-notice':{},'formatter-output':{},'formatter-pdf-download':{removeAttribute(){}}};
  const calls = [];
  const ctx = scope(['renderHTMLFormatter','clearHTMLFormatterPDF'], {window:{WSM_REMOTE:null},state:{},URL:{createObjectURL:()=> 'blob:pdf',revokeObjectURL(){}},Blob,Uint8Array,atob,document:{getElementById:id=>nodes[id]},setButtonLoading(){},api:async(path,options)=>{if(path.endsWith('/token')) return {token:'signed-test'};calls.push(JSON.parse(options.body));return {html:'done',pdf_base64:btoa('%PDF-test'),request_id:'test'};}});
  await ctx.renderHTMLFormatter();
  assert.deepEqual(calls,[{html_link:'https://example.org/report.html',timeout_ms:45000,wait_for_print_ready:true}]);
  nodes['formatter-html'].value = '<html></html>';
  await ctx.renderHTMLFormatter();
  assert.match(nodes['formatter-notice'].textContent,/un singur/);
  nodes['formatter-html-link'].value = '';
  for (const value of ['', '0', '-1', '1.5', '600001', 'NaN']) {
    nodes['formatter-timeout'].value = value;
    await ctx.renderHTMLFormatter();
    assert.match(nodes['formatter-notice'].textContent,/Timeout-ul/);
  }
  nodes['formatter-html'].value = '  ';
  await ctx.renderHTMLFormatter();
  assert.match(nodes['formatter-notice'].textContent,/un singur/);
  assert.equal(calls.length,1);
  nodes['formatter-html'].value = '<html>Static PDF</html>';
  nodes['formatter-timeout'].value = '120000';
  await ctx.renderHTMLFormatter(false);
  assert.equal(calls[1].wait_for_print_ready,false);
  assert.equal(nodes['formatter-render'].disabled,false);
});
test('formatter login setup only asks for authentication server details', async () => {
  const host={innerHTML:''};const calls=[];
  const ctx=scope(['renderLoginSetupPanel'],{state:{},els:{loginSetupHost:host},escapeHtml:value=>value,document:{getElementById:()=>null},api:async path=>{calls.push(path);return {login_only:true,configured:true,setup_complete:false,settings:{}};}});
  await ctx.renderLoginSetupPanel();
  assert.deepEqual(calls,['/api/wisemed/setup']);
  assert.match(host.innerHTML,/Server de autentificare/);
  assert.match(host.innerHTML,/name="cfg_wisemed_ip"/);
  for(const name of ['unitate_medicala_id','tip_de_echipament_id','cod_echipament','numar_serial_echipament']) assert.ok(!host.innerHTML.includes('name="'+name+'"'));
});

test('formatter WSS relies on authenticated bridge without HTTP token issuance', async () => {
  const nodes = {'formatter-html':{value:'<html>remote</html>'},'formatter-html-link':{value:''},'formatter-timeout':{value:'120000'},'formatter-render':{},'formatter-pdf-direct':{},'formatter-notice':{},'formatter-output':{},'formatter-pdf-download':{removeAttribute(){}}};
  let count = 0;
  const ctx = scope(['renderHTMLFormatter','clearHTMLFormatterPDF'], {window:{WSM_REMOTE:{}},state:{},URL:{createObjectURL:()=> 'blob:pdf',revokeObjectURL(){}},Blob,Uint8Array,atob,document:{getElementById:id=>nodes[id]},setButtonLoading(){},api:async(path,options)=>{
    count++;assert.equal(path,'/api/html-print-formatter/pdf');assert.equal(options.headers.Authorization,undefined);
    return {html:'remote',pdf_base64:btoa('%PDF-test'),request_id:'wss'};
  }});
  await ctx.renderHTMLFormatter(false);
  assert.equal(count,1);assert.equal(nodes['formatter-output'].value,'remote');
});

test('formatter real API wrapper preserves JSON content type with JWT and WSS headers', async () => {
  for (const remote of [false, true]) {
    const nodes = {'formatter-html':{value:'<html>test</html>'},'formatter-html-link':{value:''},'formatter-timeout':{value:'120000'},'formatter-render':{},'formatter-pdf-direct':{},'formatter-notice':{},'formatter-output':{},'formatter-pdf-download':{removeAttribute(){}}};
    const calls = [];
    const ctx = scope(['renderHTMLFormatter','clearHTMLFormatterPDF','api'], {
      window:{WSM_REMOTE:remote?{}:null},state:{},URL:{createObjectURL:()=> 'blob:pdf',revokeObjectURL(){}},Blob,Uint8Array,atob,
      document:{getElementById:id=>nodes[id]},setButtonLoading(){},
      fetch:async(path, options)=>{
        calls.push(path);
        assert.equal(options.headers['Content-Type'],'application/json');
        assert.equal(options.credentials,'same-origin');
        assert.equal(options.method,'POST');
        if(path.endsWith('/token')) return {ok:true,json:async()=>({token:'test-jwt'})};
        assert.equal(options.headers.Authorization,remote?undefined:'Bearer test-jwt');
        assert.equal(JSON.parse(options.body).html,'<html>test</html>');
        return {ok:true,json:async()=>({html:'rendered',pdf_base64:btoa('%PDF-test'),request_id:'test'})};
      }
    });
    for (const wait of [true,false]) {
      await ctx.renderHTMLFormatter(wait);
      assert.equal(nodes['formatter-output'].value,'rendered');
      assert.equal(nodes['formatter-pdf-download'].hidden,false);
    }
    assert.equal(calls.length,remote?2:4);
  }
});
