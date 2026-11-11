<script setup lang="ts">
import {computed,onBeforeUnmount,onMounted,ref} from 'vue'
import {api} from './api'
const props=defineProps<{userId:string}>()
const emit=defineEmits<{'open-own':[string]}>()
type Owner={user_id:string,name:string,email:string,organization:string}
type Summary={id:string,template_name:string,status:string,revision:number,created_at:string,has_result:boolean,owner:Owner}
type Page={items:Summary[],users:{id:string,name:string,email:string}[],total:number,limit:number,instance_scope:boolean}
const items=ref<Summary[]>([]),users=ref<Page['users']>([]),total=ref(0),limit=ref(50),offset=ref(0),user=ref(''),status=ref(''),busy=ref(false),error=ref(''),instance=ref(false)
const selected=ref<{session:any,owner:Owner}|null>(null)
let generation=0
const userOptions=computed(()=>[{id:'',label:'Toți utilizatorii'},...users.value.map(u=>({id:u.id,label:(u.name||u.email)+' · '+u.email}))])
const statuses=[{value:'',label:'Toate stările'},{value:'recording',label:'În lucru'},{value:'review',label:'De revizuit'},{value:'approved',label:'Aprobate'}]
const stateLabel:Record<string,string>={recording:'În lucru',review:'De revizuit',approved:'Aprobat'}
const report=computed(()=>`<!doctype html><html lang="ro"><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; form-action 'none'; base-uri 'none'"><style>body{font:16px system-ui;line-height:1.5;color:#172c32;background:white;padding:16px}table{border-collapse:collapse}td,th{padding:8px;border:1px solid #ccd6d5}</style></head><body>${selected.value?.session.result?.rendered_html||''}</body></html>`)
async function load(reset=false){
 if(reset)offset.value=0
 const request=++generation;busy.value=true;error.value='';selected.value=null
 try{
  const q=new URLSearchParams({offset:String(offset.value),user_id:user.value,status:status.value})
  const page=await api<Page>('admin/sessions?'+q)
  if(request!==generation)return
  items.value=page.items;users.value=page.users;total.value=page.total;limit.value=page.limit;instance.value=page.instance_scope
 }catch(e){if(request===generation){items.value=[];total.value=0;error.value=e instanceof Error?e.message:String(e)}}finally{if(request===generation)busy.value=false}
}
async function open(id:string){
 const request=++generation;busy.value=true;error.value='';selected.value=null
 try{const detail=await api<{session:any,owner:Owner}>('admin/sessions/'+id);if(request===generation)selected.value=detail}
 catch(e){if(request===generation)error.value=e instanceof Error?e.message:String(e)}finally{if(request===generation)busy.value=false}
}
function page(direction:number){offset.value=Math.max(0,offset.value+direction*limit.value);load()}
onMounted(()=>load())
onBeforeUnmount(()=>generation++)
</script>
<template>
<section class="paper">
 <h2>{{instance?'Sesiunile tuturor utilizatorilor':'Sesiunile utilizatorilor din organizație'}}</h2>
 <p class="muted">Consultare administrativă. Selectează utilizatorul pentru a vedea sesiunile, transcrierea și rezultatul lui.</p>
 <div class="form-row">
  <label>Utilizator<Select v-model="user" :options="userOptions" optionLabel="label" optionValue="id" placeholder="Toți utilizatorii" filter :disabled="busy" @change="load(true)"/></label>
  <label>Stare<Select v-model="status" :options="statuses" optionLabel="label" optionValue="value" placeholder="Toate stările" :disabled="busy" @change="load(true)"/></label>
  <Button label="Actualizează" icon="pi pi-refresh" :loading="busy" @click="load()"/>
 </div>
 <Message v-if="error" severity="error">{{error}}</Message>
 <p v-if="!busy&&!items.length" class="muted">Nu există sesiuni pentru filtrul selectat.</p>
 <div class="table-wrap" v-if="items.length"><table><thead><tr><th>Utilizator / organizație</th><th>Șablon</th><th>Creată</th><th>Stare</th><th>Rezultat</th><th></th></tr></thead><tbody>
 <tr v-for="item in items" :key="item.id"><td>{{item.owner.name}}<br/><small>{{item.owner.email}} · {{item.owner.organization}}</small></td><td>{{item.template_name}}</td><td>{{new Date(item.created_at).toLocaleString('ro-RO')}}</td><td>{{stateLabel[item.status]||item.status}}</td><td>{{item.has_result?'Disponibil':'Încă fără rezultat'}}</td><td><Button label="Vezi sesiunea" text :disabled="busy" @click="open(item.id)"/></td></tr>
 </tbody></table></div>
 <div class="actions"><Button label="Înapoi" text :disabled="busy||offset===0" @click="page(-1)"/><span>{{total?offset+1:0}}–{{Math.min(offset+limit,total)}} din {{total}}</span><Button label="Înainte" text :disabled="busy||offset+limit>=total" @click="page(1)"/></div>
</section>
<section class="paper" v-if="selected">
 <h2>{{selected.session.template?.name||'Sesiune clinică'}} <span class="badge">Consultare</span></h2>
 <p>{{selected.owner.name}} · {{selected.owner.email}} · {{selected.owner.organization}}</p>
 <p>Stare: {{stateLabel[selected.session.status]||selected.session.status}} · Revizia {{selected.session.revision}}</p>
 <p v-if="selected.session.context?.patient_ref">Referință: {{selected.session.context.patient_ref}}</p>
 <Button v-if="selected.owner.user_id===props.userId" label="Deschide sesiunea mea pentru lucru" text @click="emit('open-own',selected.session.id)"/>
 <h3>Rezultat</h3>
 <iframe v-if="selected.session.result?.rendered_html" :srcdoc="report" sandbox="" title="Raport clinic în consultare" style="width:100%;height:520px;border:1px solid #ccd6d5;border-radius:12px;background:white"/>
 <p v-else class="muted">Sesiunea nu are încă un rezultat generat.</p>
 <details><summary>Transcriere · {{selected.session.chunks?.length||0}} segmente</summary><p v-for="chunk in selected.session.chunks" :key="chunk.id" style="white-space:pre-wrap">{{chunk.sequence}}. {{chunk.text}}</p></details>
 <details v-if="selected.session.result"><summary>Date structurate și proveniență</summary><pre>{{JSON.stringify(selected.session.result.structured_data,null,2)}}</pre><pre>{{JSON.stringify(selected.session.result.evidence,null,2)}}</pre></details>
</section>
</template>
