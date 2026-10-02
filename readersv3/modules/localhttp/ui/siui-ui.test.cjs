const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync(__dirname+'/app.js','utf8');
function fn(name) {
 const start=source.indexOf(`function ${name}(`);
 assert.ok(start>=0);
 return source.slice(source.slice(start-6,start)==='async '?start-6:start,source.indexOf('\n}',start)+2);
}
function scope(names,globals) {
 const ctx=vm.createContext(globals);
 vm.runInContext(names.map(fn).join('\n'),ctx);
 return ctx;
}
test('SIUI configuration deep links work locally and through WSM',()=>{
 for(const remote of [false,true]) for(const subview of ['siui','reader','wss']) {
  const path='/settings/'+subview;
  const ctx=scope(['currentUIPath','viewFromLocation','settingsSubViewFromLocation','pathForView'],{state:{settingsSubView:subview},isSIUIMode:()=>true,isESignatureMode:()=>false,window:{location:{pathname:remote?'/control/':path},WSM_REMOTE:remote?{route:()=>path}:null}});
  assert.equal(ctx.viewFromLocation(),'analytes');
  assert.equal(ctx.settingsSubViewFromLocation(),subview);
  assert.equal(ctx.pathForView('analytes'),path);
 }
});
test('polls pending jobs without resubmitting, stops on terminal result',async()=>{
 const calls=[], timers=[],result={};
 let status='running';
 const ctx=scope(['showSIUIJob'],{siuiPollGeneration:0,state:{settingsSubView:'siui',currentView:'analytes'},api:async(path)=>{calls.push(path);return {job:{status}}},document:{getElementById:()=>result},setTimeout:fn=>timers.push(fn),showToast(){}});
 await ctx.showSIUIJob('job/1');
 assert.deepEqual(calls,['/api/siui/validations/job%2F1']);
 assert.equal(timers.length,1);
 status='completed';
 await timers.shift()();
 assert.equal(timers.length,0);
 assert.equal(JSON.parse(result.textContent).status,'completed');
});
test('late job response cannot replace a newer selection or logged-out view',async()=>{
 const pending=[],result={textContent:''};
 const ctx=scope(['showSIUIJob'],{siuiPollGeneration:0,state:{},api:()=>new Promise(resolve=>pending.push(resolve)),document:{getElementById:()=>result},setTimeout(){throw Error('unexpected polling')},showToast(){}});
 const first=ctx.showSIUIJob('first'),second=ctx.showSIUIJob('second');
 pending[1]({job:{id:'second',status:'completed'}});await second;
 pending[0]({job:{id:'first',status:'completed'}});await first;
 assert.equal(JSON.parse(result.textContent).id,'second');
 const last=ctx.showSIUIJob('last');ctx.siuiPollGeneration++;
 pending[2]({job:{id:'last',status:'completed'}});await last;
 assert.equal(JSON.parse(result.textContent).id,'second');
});
