import {afterEach,describe,expect,it,vi} from 'vitest'
import {readFileSync} from 'node:fs'
import vm from 'node:vm'
const html=readFileSync(new URL('../../../../modules/wisemedai/browser_speech.html',import.meta.url),'utf8')
const script=html.match(/<script nonce="__NONCE__">([\s\S]*?)<\/script>/)![1]
async function harness(supported=true){
 vi.useFakeTimers()
 const nodes:Record<string,any>={};for(const id of ['template','line','ring','start','stop','status','error','queue'])nodes[id]={textContent:'',disabled:false,classList:{toggle:vi.fn()}}
 const requests:any[]=[]
 let recognizer:any
 class Recognition {
  lang='';continuous=false;interimResults=false;onend:any;onresult:any;onerror:any
  starts=0;stops=0
  constructor(){recognizer=this}
  start(){this.starts++}
  stop(){this.stops++;this.onend?.()}
 }
 const context={window:supported?{webkitSpeechRecognition:Recognition}:{},location:{hash:'#test-grant',pathname:'/ai/voice'},history:{replaceState:vi.fn()},
  document:{hidden:false,getElementById:(id:string)=>nodes[id],addEventListener:vi.fn()},crypto:{randomUUID:()=> '11111111-1111-4111-8111-111111111111'},
  setTimeout,clearTimeout,setInterval,Date,console,
  fetch:vi.fn(async(url:string,options:any)=>{requests.push({url,...options,parsed:options.body?JSON.parse(options.body):null});return {ok:true,status:200,json:async()=>({events:[],stop_requested:false,template_name:'Ecografie tiroidiană',template_version:'version',locale:'ro-RO'})}})}
 vm.runInNewContext(script,context)
 await vi.advanceTimersByTimeAsync(0)
 return {nodes,requests,get recognizer(){return recognizer}}
}
afterEach(()=>vi.useRealTimers())
describe('Chrome speech page',()=>{
 it('restarts recognition at a browser boundary without stopping the microphone intent',async()=>{
  const h=await harness();h.nodes.start.onclick()
  expect(h.recognizer.lang).toBe('ro-RO');expect(h.recognizer.continuous).toBe(true);expect(h.recognizer.interimResults).toBe(true)
  h.recognizer.onend();await vi.advanceTimersByTimeAsync(300)
  expect(h.recognizer.starts).toBe(2);expect(h.recognizer.stops).toBe(0)
  expect(h.nodes.status.textContent).toContain('Se reconectează')
  h.recognizer.onaudiostart()
  await vi.advanceTimersByTimeAsync(0)
  expect(h.nodes.status.textContent).toContain('Ascult continuu')
  expect(h.requests.some(r=>r.parsed?.action==='state'&&r.parsed.state==='listening')).toBe(true)
 })
 it('shows interim text and sends finalized batches while recognition stays active',async()=>{
  const h=await harness();h.nodes.start.onclick()
  h.recognizer.onresult({resultIndex:0,results:[Object.assign([{transcript:'lob drept'}],{isFinal:false})]})
  expect(h.nodes.line.textContent).toContain('lob drept')
  expect(h.requests.some(r=>r.parsed?.action==='segment')).toBe(false)
  h.recognizer.onresult({resultIndex:0,results:[Object.assign([{transcript:'lob drept omogen'}],{isFinal:true})]})
  await vi.advanceTimersByTimeAsync(4100)
  expect(h.requests.filter(r=>r.parsed?.action==='segment').map(r=>r.parsed.text)).toEqual(['lob drept omogen'])
  expect(h.recognizer.stops).toBe(0)
  expect(h.requests.every(r=>r.headers.Authorization==='Browser test-grant')).toBe(true)
  expect(h.requests.every(r=>!r.url.includes('test-grant'))).toBe(true)
  h.nodes.stop.onclick();await vi.advanceTimersByTimeAsync(1000)
  expect(h.recognizer.stops).toBe(1)
 })
 it('reports unsupported browsers before requesting microphone permission',async()=>{
  const h=await harness(false)
  expect(h.nodes.error.textContent).toContain('nu expune Web Speech')
  expect(h.recognizer).toBeUndefined()
 })
})
