const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const source=fs.readFileSync(__dirname+'/app.js','utf8');
function scope(names,globals={}) {
 const functions=names.map(name=>{const start=source.indexOf('function '+name+'(');assert.ok(start>=0);return source.slice(start,source.indexOf('\n}',start)+2)}).join('\n');
 const ctx=vm.createContext({escapeHtml:value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])),formatDate:value=>value||'—',...globals});
 vm.runInContext(functions,ctx);return ctx;
}
function fixture(){return {totals:{calls:0,archived:0,pending:0,files:0,payload_bytes:0,manifest_bytes:0,pending_bytes:0},counts:{completed:0,failed:0,unknown:0},hourly:[],from:'2026-10-21T00:00:00Z',to:'2026-10-22T00:00:00Z',traffic:{errors:0,requests:0,received_bytes:0,sample_limit:1000},bridges:[],queue:[],recent_errors:[],sending_bridges:0,authorized_bridges:0,observed_bridges:0,storage:'local'};}
test('archive dashboard handles empty data without invented percentages',()=>{
 const ctx=scope(['archiveBytes','archiveWait','archiveDashboardHTML','siuiTag','siuiChartSVG']);
 const html=ctx.archiveDashboardHTML(fixture());
 assert.doesNotMatch(html,/NaN|Infinity|undefined/);assert.match(html,/Niciun Bridge/);assert.match(html,/Nu sunt transferuri/);assert.match(html,/0 B/);
});
test('archive state is distinct from SIUI results and percentages preserve pending work',()=>{
 const ctx=scope(['archiveBytes','archiveWait','archiveDashboardHTML','siuiTag','siuiChartSVG']);
 const data=fixture();data.totals={...data.totals,calls:1000,archived:999,pending:1,payload_bytes:1024,manifest_bytes:512};data.counts.failed=1;
 data.bridges=[{id:'<img src=x onerror=alert(1)>',authorized:false,calls:1,completed:0,failed:1,unknown:0,pending:1,archived_bytes:1536}];
 const html=ctx.archiveDashboardHTML(data);
 assert.match(html,/99,9%/);assert.match(html,/1,5 KiB/);assert.match(html,/&lt;img/);assert.doesNotMatch(html,/<img/);assert.match(html,/Un apel SIUI eșuat poate fi arhivat complet/);
});
test('History home and settings keep their own local and remote routes',()=>{
 for(const remote of [false,true]) for(const [path,view] of [['/','overview'],['/history','history'],['/debug','debug'],['/settings/history','analytes']]) {
  const ctx=scope(['currentUIPath','viewFromLocation'],{state:{},isHistoryMode:()=>true,window:{location:{pathname:remote?'/control/':path},WSM_REMOTE:remote?{route:()=>path}:null}});
  assert.equal(ctx.viewFromLocation(),view);
 }
});
test('operation status summary counts only the displayed page',()=>{
 const ctx=scope(['siuiOperationSummary','siuiTag']);
 const html=ctx.siuiOperationSummary([{status:'failed'},{status:'failed'},{status:'running'}]);
 assert.match(html,/În pagina curentă/);assert.match(html,/2 failed/);assert.match(html,/1 running/);
});
test('Operations explicit interval crosses days and end without hour includes that entire day',()=>{
 class Fields{constructor(form){this.form=form}get(key){return this.form[key]}}
 const ctx=scope(['siuiOperationsQuery'],{FormData:Fields,URLSearchParams});
 const form={day:'2026-10-19',from:'08:30',end_day:'2026-10-21',to:'',operation:' getCatalogues ',status:'failed'};
 const q=ctx.siuiOperationsQuery(form,100);
 assert.equal(q.get('from'),new Date('2026-10-19T08:30').toISOString());assert.equal(q.get('to'),new Date('2026-10-22T00:00').toISOString());assert.equal(q.get('offset'),'100');assert.equal(q.get('operation'),'getCatalogues');
 assert.throws(()=>ctx.siuiOperationsQuery({...form,end_day:'2026-10-18'}),/Sfârșitul/);
});
