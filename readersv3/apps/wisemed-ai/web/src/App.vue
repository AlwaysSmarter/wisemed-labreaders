<script setup lang="ts">
import FieldReview from "./FieldReview.vue"
import SpeechModels from './SpeechModels.vue'
import SpeechCommunity from './SpeechCommunity.vue'
import SpeechConsent from './SpeechConsent.vue'
import {ref,computed,onMounted,onBeforeUnmount,provide,watch} from 'vue'
import HelpHint from './HelpHint.vue'
import TemplateHelpDialog from './TemplateHelpDialog.vue'
import {templateHelpKey} from './templateHelpContext'
import type {TemplateHelpTopic} from './templateHelp'
import {defineAsyncComponent} from 'vue'
const TemplateEditor=defineAsyncComponent(()=>import('./TemplateEditor.vue'))
const AdminSessions=defineAsyncComponent(()=>import('./AdminSessions.vue'))
const PresentationPanel=defineAsyncComponent(()=>import('./PresentationPanel.vue'))
const BillingPanel=defineAsyncComponent(()=>import('./BillingPanel.vue'))
const RuntimeSettings=defineAsyncComponent(()=>import('./RuntimeSettings.vue'))
import {clone,syntheticTranscript,api,type Template,newField} from './api'
import {money} from './billing'
const profile=ref<any>(null),busy=ref(false),error=ref(''),notice=ref(''),email=ref('admin@wisemed.local'),password=ref(''),authProviders=ref<any>({}),useWiseMED=ref(false),loginTenant=ref(''),memoryQuery=ref(''),deliveries=ref<any[]>([]),view=ref('dashboard'),dark=ref(localStorage.getItem('wai_theme')==='dark'),mobile=ref(false)
const helpEnabled=ref(false),helpTopic=ref<TemplateHelpTopic|null>(null)
provide(templateHelpKey,{enabled:computed(()=>helpEnabled.value&&view.value==='templates'),open:topic=>{helpTopic.value=topic}})
watch(view,()=>{helpTopic.value=null})
function toggleHelp(){helpEnabled.value=!helpEnabled.value;if(!helpEnabled.value)helpTopic.value=null}
const billingState=ref<any>(null),routeDetail=ref('')
let navigation=0
let pending=0
const dashboard=ref<any>({}),templates=ref<Template[]>([]),sessionList=ref<any[]>([]),packs=ref<any[]>([]),selectedTemplate=ref(''),patientRef=ref(''),session=ref<any>(null),transcript=ref(''),chunkID=ref(''),chunkRevision=ref(1),editorTemplate=ref<Template|null>(null),rows=ref<any[]>([]),settings=ref<any>({}),usage=ref<any>({}),resourceName=ref(''),resourceText=ref('{}'),confirmed=ref(false),resourceTab=ref('memory/preferences'),user=ref<any>({name:'',email:'',password:'',role:'doctor',external_subject:'',disabled:false}),account=ref<any>({name:'',current_password:'',password:''}),pairing=ref(''),pairingQR=ref(''),connector=ref(''),catalogFilter=ref(''),organizationName=ref(''),eventSource: {value:EventSource|null}={value:null}
const nav=[['dashboard','pi-th-large','Privire de ansamblu'],['workspace','pi-microphone','Sesiune clinică'],['templates','pi-file-edit','Șabloane'],['packs','pi-box','Specialty Packs'],['sessions','pi-history','Sesiuni și rezultate'],['memory','pi-book','Memorie și feedback'],['subscription','pi-wallet','Pachetul meu'],['billing','pi-credit-card','Pachete și monetizare'],['users','pi-users','Echipă și acces'],['integrations','pi-link','Conectori'],['usage','pi-chart-bar','Consum și costuri'],['audit','pi-shield','Audit'],['settings','pi-cog','Configurare'],['speech-models','pi-download','Modele vocale locale'],['speech','pi-microphone','Contribuții vocale'],['account','pi-user','Contul meu']]
const title=computed(()=>nav.find(n=>n[0]===view.value)?.[2]||'WiseMED AI')
const admin=computed(()=>profile.value?.role==='admin')
const specialtyTemplates=computed(()=>profile.value?.instance_admin?templates.value:templates.value.filter(t=>(t.specialties?.length?t.specialties:[t.specialty]).some(s=>activeSpecialties.value.includes(s))))
const filteredTemplates=computed(()=>specialtyTemplates.value.filter(t=>(t.name+' '+(t.specialties?.length?t.specialties.join(' '):t.specialty)).toLocaleLowerCase('ro').includes(catalogFilter.value.toLocaleLowerCase('ro'))))
const filteredPacks=computed(()=>packs.value.filter(p=>p.name.toLocaleLowerCase('ro').includes(catalogFilter.value.toLocaleLowerCase('ro'))))
const activeSpecialties=computed<string[]>(()=>billingState.value?.selected_specialties||[])
const templateLimitReached=computed(()=>!!billingState.value?.plan&&!billingState.value.quota_exempt&&billingState.value.templates_used>=billingState.value.plan.max_templates)
const templateAddonQuote=computed(()=>{
  const plan=billingState.value?.plan
  return plan?.extras_enabled&&plan.extra_template_price_minor>0?{
    plan_version:plan.version_id,monthly_minor:plan.extra_template_price_minor,currency:plan.currency
  }:undefined
})
const editorSpecialties=computed<string[]>(()=>profile.value?.instance_admin?[...new Set<string>([...(billingState.value?.available_specialties||[]),...(editorTemplate.value?.specialties??[editorTemplate.value?.specialty||'']).filter(Boolean)])]:activeSpecialties.value)
const workspaceTemplates=computed(()=>specialtyTemplates.value.filter(t=>!t.addon_ends_at||Date.parse(t.addon_ends_at)>Date.now()||session.value?.template_version===t.version_id))
const needsSpecialty=computed(()=>!profile.value?.instance_admin&&!activeSpecialties.value.length)

const selected=computed(()=>templates.value.find(t=>t.version_id===selectedTemplate.value))
const stateLabel:Record<string,string>={recording:'În lucru',review:'De revizuit',approved:'Aprobat'}
const visibleNav=computed(()=>nav.filter(n=>(!billingState.value?.subscription?.suspended||profile.value?.instance_admin||['subscription','account','billing'].includes(n[0]))&&(admin.value||!['packs','users','audit','settings','integrations'].includes(n[0]))&&(!['settings','billing','speech-models'].includes(n[0])||profile.value?.instance_admin)))
function updateBillingAccount(account:any){
  billingState.value=account
  if(!session.value&&!workspaceTemplates.value.some(t=>t.version_id===selectedTemplate.value)){
    selectedTemplate.value=workspaceTemplates.value[0]?.version_id||''
  }
}
function theme(){document.documentElement.classList.toggle('dark',dark.value);localStorage.setItem('wai_theme',dark.value?'dark':'light')}
async function run(fn:()=>Promise<void>){const current=navigation;pending++;busy.value=true;error.value='';notice.value='';try{await fn()}catch(e){if(current===navigation)error.value=e instanceof Error?e.message:String(e)}finally{busy.value=--pending>0}}
async function login(){await run(async()=>{const d=await api('auth/login','POST',{email:email.value,password:password.value,wisemed:useWiseMED.value,tenant_id:loginTenant.value});profile.value=d.profile;password.value='';account.value.name=profile.value.name;await refresh()});if(profile.value)await restoreRoute()}
async function logout(){await run(async()=>{await api('auth/logout','POST',{});profile.value=null;navigation++;clearSession()})}
async function refresh(){updateBillingAccount(await api('billing/account'));if(billingState.value.subscription?.suspended&&!profile.value?.instance_admin){eventSource.value?.close();return}const data=await Promise.all([api('dashboard'),api<Template[]>('templates'),api<any[]>('sessions'),api<any[]>('specialty-packs')]);dashboard.value=data[0];templates.value=data[1];sessionList.value=data[2];packs.value=data[3];if(!session.value&&!workspaceTemplates.value.some(t=>t.version_id===selectedTemplate.value))selectedTemplate.value=workspaceTemplates.value[0]?.version_id||''}
const resourceTabs:Record<string,string[]>={memory:['memory/preferences','memory/examples','memory/feedback'],integrations:['integrations','connector-mappings']}
const detailViews=['workspace','templates','users','billing','memory','integrations']
const uuid=/^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i
function writeRoute(v:string,detail:string,replace:boolean){
  const hash='#/'+v+(detail?'/'+encodeURIComponent(detail):'')
  if(location.hash!==hash)history[replace?'replaceState':'pushState'](null,'',hash)
}
async function restoreRoute(){
  if(!profile.value)return
  const [v,detail='']=location.hash.slice(2).split('/')
  let decoded=''
  try{decoded=decodeURIComponent(detail)}catch{}
  await go(location.hash.startsWith('#/')?v:'dashboard',decoded,true)
}
function clearSession(){
  eventSource.value?.close()
  session.value=null
  transcript.value=''
  chunkID.value=''
  chunkRevision.value=1
  pairing.value=''
  pairingQR.value=''
  confirmed.value=false
}
async function go(v:string,detail='',replace=false){
  if(!visibleNav.value.some(n=>n[0]===v)){
    v=billingState.value?.subscription?.suspended&&!profile.value?.instance_admin?'subscription':'dashboard'
    detail=''
    replace=true
  }
  if(!detailViews.includes(v))detail=''
  else if(resourceTabs[v]){
    if(!resourceTabs[v].includes(detail)||detail===resourceTabs[v][0])detail=''
  }else if(!uuid.test(detail)&&!(v==='templates'&&detail==='new'))detail=''
  const current=++navigation
  const changed=view.value!==v
  view.value=v
  routeDetail.value=detail
  mobile.value=false
  writeRoute(v,detail,replace)
  editorTemplate.value=null
  if(changed||resourceTabs[v])rows.value=[]
  if(v==='users')user.value={name:'',email:'',password:'',role:'doctor',external_subject:'',disabled:false}
  if(v==='workspace'&&(session.value?.id||'')!==detail)clearSession()
  await run(async()=>{
    if(['dashboard','sessions','templates','packs'].includes(v)||(v==='workspace'&&!detail))await refresh()
    if(current!==navigation)return
    if(v==='templates'){
      if(detail==='new'&&!needsSpecialty.value)newTemplateDraft()
      else if(detail&&detail!=='new'){
        const template=templates.value.find(t=>t.version_id===detail)
        if(template)editorTemplate.value=clone(template)
        else throw new Error('Șablonul nu mai este disponibil.')
      }
    }
    if(v==='workspace'&&detail&&session.value?.id!==detail){
      const loaded=await api('sessions/'+detail)
      if(current!==navigation)return
      session.value=loaded
      selectedTemplate.value=loaded.template_version
      patientRef.value=loaded.context?.patient_ref||''
      watchEvents()
    }
    if(v==='users'){
      const [members,orgs]=await Promise.all([api('users'),api('organizations')])
      if(current!==navigation)return
      rows.value=members
      organizationName.value=orgs[0]?.name||''
      const member=members.find((u:any)=>u.id===detail)
      if(member)user.value={...member,password:''}
      if(detail&&!member)throw new Error('Utilizatorul nu mai este disponibil.')
    }
    if(v==='audit'){
      const entries=await api('audit')
      if(current===navigation)rows.value=entries
    }
    if(v==='settings')settings.value={...await api('settings'),openai_key:''}
    if(v==='usage')usage.value=await api('usage')
    if(v==='integrations')deliveries.value=await api('outbox')
    if(current!==navigation)return
    if(resourceTabs[v]){
      resourceTab.value=detail||resourceTabs[v][0]
      await loadResource()
    }
  })
}
async function createSession(){
  await run(async()=>{
    const current=navigation
    const created=await api('sessions','POST',{template_version:selectedTemplate.value,patient_ref:patientRef.value})
    if(current!==navigation)return
    clearSession()
    session.value=created
    await go('workspace',created.id)
    watchEvents()
  })
}
function watchEvents(){
  eventSource.value?.close()
  const id=session.value?.id
  if(!id)return
  const source=new EventSource('/v1/sessions/'+id+'/events')
  eventSource.value=source
  source.addEventListener('progress',async e=>{
    const d=JSON.parse((e as MessageEvent).data)
    if(session.value?.id===id&&(d.revision>session.value.revision||d.status!==session.value.status)){
      try{
        const updated=await api('sessions/'+id)
        if(eventSource.value===source&&session.value?.id===id)session.value=updated
      }catch{}
    }
  })
}
function openSession(s:any){return go('workspace',s.id)}
function sample(){if(selected.value&&!(['Ecografie abdominală','Consultație cardiologie','Vizită zilnică'].includes(selected.value.name))){transcript.value=syntheticTranscript(selected.value);return}transcript.value=selected.value?.specialty==='Cardiologie'?'anamneza: palpitații menționate\nta: 120/80\npuls: 72\nekg: ritm sinusal documentat':selected.value?.specialty==='Medicină internă'?'evolutie: fără acuze noi menționate\nexamen: neexaminat':'ficat: ecostructură omogenă\nficat_mm: 140\nsplina: dimensiuni normale descrise\nrinichi_drept: fără dilatații descrise\nrinichi_stang: fără dilatații descrise\nformatiuni: neexaminat'}
let transcriptTimer:ReturnType<typeof setTimeout>|undefined
watch(transcript,()=>{clearTimeout(transcriptTimer);if(transcript.value.trim()&&session.value?.status==='recording')transcriptTimer=setTimeout(autoSendTranscript,1600)})
function autoSendTranscript(){if(!transcript.value.trim()||session.value?.status!=='recording')return;if(busy.value){transcriptTimer=setTimeout(autoSendTranscript,500);return}void sendChunk()}
onBeforeUnmount(()=>clearTimeout(transcriptTimer))
async function sendChunk(){await run(async()=>{if(!session.value)throw new Error('Deschideți întâi o sesiune.');const sentText=transcript.value;const sessionId=session.value.id;const id=chunkID.value||crypto.randomUUID();const old=session.value.chunks?.find((c:any)=>c.id===id);await api('sessions/'+session.value.id+'/transcript-chunks','POST',{id,sequence:old?.sequence||Math.max(0,...(session.value.chunks||[]).map((c:any)=>c.sequence))+1,revision:old?old.revision+1:1,is_final:true,timestamp:new Date().toISOString(),text:sentText});if(session.value?.id!==sessionId)return;session.value=await api('sessions/'+sessionId);if(transcript.value===sentText)transcript.value='';chunkID.value='';notice.value='Transcriere procesată. Verificați datele și sursele.'})}
async function action(name:string){
  await run(async()=>{
    const id=session.value.id
    const revision=session.value.revision
    const body=name==='approve'?{revision,confirmed:confirmed.value}:name==='reopen'?{revision}:{}
    await api('sessions/'+id+'/'+name,'POST',body)
    session.value=await api('sessions/'+id)
    confirmed.value=false
    notice.value=name==='approve'?'Rezultat aprobat. Nicio trimitere externă automată.':name==='reopen'?'Sesiunea este din nou în lucru. Poți continua dictarea.':'Sesiune pregătită pentru revizuire.'
  })
}
async function pair(){await run(async()=>{const d=await api('sessions/'+session.value.id+'/pairing','POST',{});pairing.value=d.token;pairingQR.value=d.qr_png;notice.value='Token one-time, valabil 2 minute. Necesită autentificarea aceluiași medic.'})}
function editChunk(c:any){chunkID.value=c.id;chunkRevision.value=c.revision+1;transcript.value=c.text}
async function briefing(){await run(async()=>{const d=await api('sessions/'+session.value.id+'/briefing','POST',{});notice.value=d.message})}
function edit(t:Template){return go('templates',t.version_id)}
function newTemplate(){return go('templates','new')}
function newTemplateDraft(){editorTemplate.value={id:'',name:'Șablon nou',specialty:activeSpecialties.value[0]||workspaceTemplates.value[0]?.specialty||'General',version_id:'',version:0,html:'<h2>Raport clinic</h2><p>Documentați numai date observate.</p>',fields:[newField()],rules:[],sections:[],fragments:{},semantic_mapping:{}}}
async function saved(t:Template){editorTemplate.value=t;routeDetail.value=t.version_id;writeRoute('templates',t.version_id,true);await run(refresh);notice.value='Versiunea '+t.version+' a fost publicată.'}
async function testTemplate(t:Template){
  if(!workspaceTemplates.value.some(option=>option.version_id===t.version_id)){
    await go('subscription')
    notice.value='Activează una dintre specialitățile șablonului pentru a-l folosi.'
    return
  }
  selectedTemplate.value=t.version_id;patientRef.value='DEMO-SINTETIC';await createSession();sample()}
async function loadResource(){const current=navigation;const entries=await api(resourceTab.value);if(current!==navigation)return;rows.value=entries;resourceName.value='';resourceText.value=resourceTab.value==='integrations'?'{\n  "url": "https://his.example/api/import",\n  "token": ""\n}':'{\n  "text": ""\n}';confirmed.value=false}
async function searchMemory(){await run(async()=>{const result=await api('memory/search','POST',{query:memoryQuery.value});rows.value=result.items})}
async function saveResource(){await run(async()=>{await api(resourceTab.value,'POST',{name:resourceName.value,payload:JSON.parse(resourceText.value),confirmed:confirmed.value});await loadResource();notice.value='Înregistrare salvată.'})}
async function deleteResource(id:string){await run(async()=>{await api(resourceTab.value,'DELETE',{id});await loadResource()})}
async function saveOrganization(){await run(async()=>{await api('organizations','PUT',{name:organizationName.value});notice.value='Organizație actualizată.'})}
async function saveUser(){
  await run(async()=>{
    const {password: newPassword, ...fields}=user.value
    const resetOwnPassword=!!newPassword && fields.id===profile.value?.user_id
    await api('users',fields.id?'PUT':'POST',newPassword?{...fields,password:newPassword}:fields)
    user.value={name:'',email:'',password:'',role:'doctor',external_subject:'',disabled:false}
    if(resetOwnPassword){
      profile.value=null
      session.value=null
      eventSource.value?.close()
      return
    }
    await go('users','',true)
    notice.value='Utilizator salvat.'
  })
}
async function saveSettings(){await run(async()=>{await api('settings','PUT',{provider:settings.value.provider,model:settings.value.model,monthly_budget_usd:settings.value.monthly_budget_usd,input_usd_per_million:settings.value.input_usd_per_million,output_usd_per_million:settings.value.output_usd_per_million,retention_days:settings.value.retention_days,openai_key:settings.value.openai_key});settings.value.openai_key='';notice.value='Configurația a fost salvată. Reporniți utilitarul pentru aplicare.'})}
async function saveAccount(){await run(async()=>{profile.value=await api('profile','PUT',account.value);notice.value='Cont actualizat. După schimbarea parolei autentificați-vă din nou.';account.value.current_password='';account.value.password=''})}
function exportJSON(){const b=new Blob([JSON.stringify(session.value.result,null,2)],{type:'application/json'}),a=document.createElement('a');a.href=URL.createObjectURL(b);a.download='wisemed-ai-result.json';a.click();URL.revokeObjectURL(a.href)}
async function deleteSession(){
  const id=session.value.id
  if(!window.confirm('Ștergi definitiv această sesiune? Transcrierea și documentul vor fi șterse.'))return
  await run(async()=>{await api('sessions/'+id,'DELETE');clearSession();await go('workspace','',true);await refresh();notice.value='Sesiunea și datele clinice asociate au fost șterse.'})
}
async function delivery(){await run(async()=>{await api('sessions/'+session.value.id+'/deliver','POST',{connector_id:connector.value,revision:session.value.revision,confirmed:confirmed.value});notice.value='Rezultatul aprobat a fost pus în coada de livrare.'})}
const billingPoll=setInterval(async()=>{if(!profile.value)return;try{updateBillingAccount(await api('billing/account'));if(billingState.value.subscription?.suspended&&!profile.value?.instance_admin){if(view.value!=='subscription'&&view.value!=='account')await go('subscription','',true);eventSource.value?.close()}}catch{}},30000)
onBeforeUnmount(()=>{clearInterval(billingPoll);window.removeEventListener('popstate',restoreRoute)})
onMounted(async()=>{window.addEventListener('popstate',restoreRoute);theme();await run(async()=>{authProviders.value=await api('auth/providers');try{profile.value=await api('profile')}catch{return};account.value.name=profile.value.name;await refresh()});if(profile.value)await restoreRoute()})
onBeforeUnmount(()=>eventSource.value?.close())
</script>
<template>
<div v-if="!profile" class="login-page"><div class="login-brand"><div class="brand-mark">w<span>ai</span></div><span class="eyebrow">WISEMED · CLINICAL INTELLIGENCE</span><h1>Mai multă atenție<br/>pentru pacient.</h1><p>Din transcriere în documentație clinică structurată.<br/>Cu surse verificabile și decizia medicului la fiecare pas.</p><div class="trust"><i aria-hidden="true" class="pi pi-lock"/> Date criptate <i aria-hidden="true" class="pi pi-check-circle"/> Aprobare explicită</div></div><form class="login-card paper" @submit.prevent="login"><span class="eyebrow">BUN VENIT ÎN WISEMED AI</span><h2>Autentificare</h2><p class="muted">Folosiți contul organizației dumneavoastră.</p><Message v-if="error" severity="error">{{error}}</Message><label>Tip de autentificare<select v-model="useWiseMED"><option :value="false">Cont WiseMED AI · email</option><option :value="true">Cont WiseMED · nume de utilizator</option></select></label><Message v-if="useWiseMED&&!authProviders.wisemed" severity="warn">Autentificarea WiseMED nu este activată pe acest server. Administratorul o poate activa în Configurare, la WiseMED API.</Message><label>{{useWiseMED?'Utilizator WiseMED':'Email'}}<InputText v-model="email" :type="useWiseMED?'text':'email'" autocomplete="username" required/></label><label>Parolă<InputText v-model="password" type="password" autocomplete="current-password" required/></label><template v-if="useWiseMED"><label>Cod organizație AI<InputText v-model="loginTenant" required placeholder="Furnizat de administratorul organizației"/></label><p class="small muted">Folosiți numele de utilizator WiseMED, fără obligația unei adrese de email. Administratorul trebuie să asocieze identitatea WiseMED cu un utilizator din organizația AI.</p></template><Button type="submit" label="Autentificare" icon="pi pi-arrow-right" iconPos="right" :loading="busy" :disabled="useWiseMED&&!authProviders.wisemed"/><p class="small muted">Prima instalare: parola inițială este generată în fișierul protejat ai-bootstrap-password din deployments.</p></form></div>
<div v-else class="shell"><aside class="sidebar" :class="{open:mobile}"><a class="brand" href="/ai/"><div class="brand-mark">w<span>ai</span></div><div><b>WiseMED AI</b><small>DASHBOARD</small></div></a><span class="nav-label">SPAȚIUL DE LUCRU</span><nav><button v-for="n in visibleNav" :key="n[0]" :aria-label="n[2]" @click="go(n[0])" :class="{active:view===n[0]}"><i aria-hidden="true" :class="'pi '+n[1]"/><span>{{n[2]}}</span><span v-if="n[0]==='sessions'&&dashboard.review" class="count">{{dashboard.review}}</span></button></nav><div class="sidebar-foot"><div class="avatar">{{profile.name?.slice(0,1)}}</div><div><b>{{profile.name}}</b><small>{{profile.user_type===-1?'Super admin':profile.user_type===0?'Admin':'Utilizator'}} · organizație privată</small></div><Button icon="pi pi-sign-out" text aria-label="Deconectare" @click="logout"/></div></aside>
<main><header><div class="breadcrumb"><Button icon="pi pi-bars" text class="mobile-menu" aria-label="Meniu" @click="mobile=!mobile"/><span>WiseMED AI</span><i aria-hidden="true" class="pi pi-angle-right"/><b>{{title}}</b></div><div class="actions"><span class="safe"><i aria-hidden="true" class="pi pi-shield"/> Spațiu protejat</span><Button icon="pi pi-question-circle" text :class="{'help-toggle-active':helpEnabled}" :aria-pressed="helpEnabled" aria-label="Ajutor contextual" title="Activează sau dezactivează explicațiile pentru șabloane" @click="toggleHelp"/><Button :icon="dark?'pi pi-sun':'pi pi-moon'" text aria-label="Schimbă tema" @click="dark=!dark;theme()"/></div></header><div class="content"><div class="page-heading"><div><span class="eyebrow">{{new Date().toLocaleDateString('ro-RO',{day:'numeric',month:'long',year:'numeric'})}}</span><h1>{{title}}</h1><p>{{view==='billing'?'Definește pachetele, tarifele și limitele pe utilizator.':view==='subscription'?'Urmărește pachetul, specialitățile și consumul de tokenuri AI.':view==='dashboard'?'Documentație clară. Context păstrat. Decizii în mâinile medicului.':'Documentați numai informații susținute de observații și surse.'}}</p></div><Button v-if="view==='dashboard'||view==='sessions'" label="Sesiune nouă" icon="pi pi-plus" @click="go('workspace')"/></div>
<Message v-if="billingState?.subscription?.outstanding_minor>0||billingState?.subscription?.suspended" :severity="billingState.subscription.suspended?'error':'warn'">{{billingState.subscription.suspended?'Acces suspendat.':'Ai sume de plată.'}} Rest emis: {{(billingState.subscription.outstanding_minor/100).toFixed(2)}} {{billingState.plan.currency}}. Consultă „Pachetul meu” pentru scadențe.</Message><Message v-if="error" severity="error" role="alert" closable @close="error=''">{{error}}</Message><Message v-if="notice" severity="success" role="status" closable @close="notice=''">{{notice}}</Message><div v-if="busy" class="loading" role="status"><i aria-hidden="true" class="pi pi-spin pi-spinner"/> Se procesează…</div>
<template v-if="view==='dashboard'">
<section v-if="billingState?.plan" class="paper">
  <div class="section-heading">
    <div><span class="eyebrow">PACHETUL MEU</span><h2>{{billingState.plan.name}}</h2><p>{{money(billingState.plan.monthly_price_minor,billingState.plan.currency)}} / lună · {{activeSpecialties.length}} / {{billingState.plan.max_specialties}} specialități active</p></div>
    <Button label="Pachet și specialități" @click="go('subscription')"/>
  </div>
</section><div class="stats"><article v-for="(v,i) in [dashboard.sessions,dashboard.review,dashboard.approved,dashboard.templates]" class="paper stat"><span>{{(admin||profile?.instance_admin?['Sesiunile mele','Ale mele · de revizuit','Rezultatele mele aprobate','Șabloane disponibile']:['Sesiuni documentate','În așteptarea revizuirii','Rezultate aprobate','Șabloane disponibile'])[i]}}</span><strong>{{v||0}}</strong><small>{{['Istoricul cabinetului','Necesită confirmarea medicului','Pregătite pentru export','Versiuni controlate'][i]}}</small></article></div><section class="welcome"><div><span class="eyebrow">ASISTENTUL DUMNEAVOASTRĂ DE DOCUMENTARE</span><h2>Consultația începe cu ascultarea.</h2><p>Alegeți un șablon și transformați transcrierea într-un raport<br/>cu date structurate, câmpuri validate și dovezi verificabile.</p><Button label="Deschide cabinetul" icon="pi pi-arrow-right" iconPos="right" @click="go('workspace')"/></div><div class="orb"><i aria-hidden="true" class="pi pi-microphone"/><div class="wave">▂ ▅ ▃ ▇ ▅ ▂ ▆ ▃ ▅ ▂</div><small>TRANSCRIERE → STRUCTURĂ → REVIZUIRE</small></div></section><div class="section-heading"><h2>Șabloane clinice</h2><Button label="Vezi biblioteca" text @click="go('templates')"/></div><div class="three"><article class="paper pack" v-for="t in workspaceTemplates.slice(0,3)"><div class="pack-icon"><i aria-hidden="true" :class="t.specialty==='Cardiologie'?'pi pi-heart':'pi pi-file-check'"/></div><span class="eyebrow">{{t.specialty}}</span><h3>{{t.name}}</h3><p>{{t.fields.length}} câmpuri · versiunea {{t.version}}</p><Button label="Folosește șablonul" text icon="pi pi-arrow-right" iconPos="right" @click="selectedTemplate=t.version_id;go('workspace')"/></article></div><div class="section-heading"><h2>{{admin||profile?.instance_admin?'Sesiunile mele recente':'Sesiuni recente'}}</h2><Button :label="admin||profile?.instance_admin?'Sesiunile utilizatorilor':'Istoric complet'" text @click="go('sessions')"/></div><div class="paper"><div v-if="!sessionList.length" class="empty"><i aria-hidden="true" class="pi pi-file"/><h3>Prima sesiune vă așteaptă</h3><p>Începeți cu un transcript sintetic sau cu documentarea unei consultații.</p></div><div v-for="s in sessionList.slice(0,5)" class="session-row" @click="openSession(s)" tabindex="0" @keydown.enter="openSession(s)"><i aria-hidden="true" class="pi pi-file-edit"/><div><b>{{templates.find(t=>t.version_id===s.template_version)?.name||'Sesiune clinică'}}</b><small>{{new Date(s.created_at).toLocaleString('ro-RO')}}</small></div><span class="badge" :class="s.status">{{stateLabel[s.status]}}</span><i aria-hidden="true" class="pi pi-arrow-up-right"/></div></div></template>
<template v-if="view==='workspace'"><Message v-if="needsSpecialty&&!session" severity="info">Alege o specialitate din <Button label="Pachetul meu" text @click="go('subscription')"/> pentru a vedea șabloanele disponibile.</Message><div class="paper form-row"><label>Șablon clinic<Select v-model="selectedTemplate" :options="workspaceTemplates" optionLabel="name" optionValue="version_id" placeholder="Selectați un șablon" :disabled="!!session"/></label><label>Referință pacient<InputText v-model="patientRef" placeholder="Referință minimă; fără CNP" :disabled="!!session"/></label><Button label="Deschide sesiune" @click="createSession" :disabled="!selectedTemplate||!!session||profile.role==='viewer'" :loading="busy"/><Button v-if="session" label="Altă sesiune" severity="secondary" @click="go('workspace')"/></div><div v-if="!session" class="paper empty"><i aria-hidden="true" class="pi pi-microphone"/><h2>Pregătiți spațiul de consultație</h2><p>Alegeți un șablon și deschideți o sesiune. Transcrierea vocală se face pe telefon; aici primiți și revizuiți textul.</p></div><template v-else><div class="session-banner"><span class="badge" :class="session.status">{{stateLabel[session.status]}}</span><code>{{session.id}}</code><span>rev. {{session.revision}}</span><Button v-if="session.status==='review'||session.status==='approved'" :label="session.status==='approved'?'Editează o versiune nouă':'Continuă dictarea'" icon="pi pi-microphone" outlined @click="action('reopen')" :disabled="busy||profile.role==='viewer'"/><Button label="Informare" icon="pi pi-info-circle" text @click="briefing"/><Button label="Asociază telefon" icon="pi pi-qrcode" text @click="pair"/></div><div v-if="pairing" class="paper"><b>Token de asociere</b><div><img v-if="pairingQR" :src="pairingQR" alt="QR de asociere telefon, valabil două minute" width="192" height="192"/></div><p class="token">{{pairing}}</p><small>One-time · 120 secunde · contul curent</small></div><div class="workspace-grid"><section class="paper"><div class="section-heading"><h2>Transcriere</h2><span class="muted small">{{dashboard.provider==='mock'?'MOCK · sintaxă câmp: valoare':'OPENAI · extragere cu dovezi'}}</span></div><p class="muted small">{{chunkID?'Corectați segmentul selectat. Se actualizează câmpurile la care se referă textul.':'Adăugați textul transcris. Lipsa unei informații nu înseamnă rezultat normal.'}}</p><label class="sr-only" for="transcript">Text transcris</label><Textarea id="transcript" v-model="transcript" rows="10" placeholder="ficat: ecostructură omogenă&#10;ficat_mm: 140&#10;splina: dimensiuni normale descrise" :disabled="session.status!=='recording'"/><div class="actions"><Button label="Exemplu sintetic" text @click="sample" :disabled="session.status!=='recording'"/><span class="muted small">Textul se procesează automat după o pauză.</span></div></section><section class="paper"><div class="section-heading"><h2>Document clinic</h2><span class="badge">{{session.result?.evidence?.length||0}} dovezi</span></div><details v-if="selected?.fields.some(f=>f.help)" class="paper">
  <summary>Ghid de completare a șablonului</summary>
  <ul><li v-for="field in selected.fields.filter(f=>f.help)" :key="field.key"><strong>{{field.label}}</strong> <span class="field-help">{{field.help}}</span></li></ul>
</details><FieldReview v-if="session.result" :session="session" :template="session.template||selected" @saved="session=$event;confirmed=false" @error="error=$event"/><div v-else class="empty"><i aria-hidden="true" class="pi pi-file-check"/><p>Raportul va apărea după procesarea primului segment.</p></div><template v-if="session.result"><Message v-if="session.result.validation_errors?.length" severity="error">Validări: {{session.result.validation_errors.join(', ')}}</Message><Message v-if="session.result.missing_fields.length" severity="warn">Câmpuri obligatorii lipsă: {{session.result.missing_fields.join(', ')}}</Message><p class="small muted" v-for="warning in session.result.warnings">{{warning}}</p><details><summary>Date structurate și proveniență</summary><div v-for="f in session.result.evidence" class="evidence"><b>{{f.field}} · {{f.state}}</b><p>„{{f.quote}}”</p><code>{{f.chunk_id}}</code></div><pre>{{JSON.stringify(session.result.structured_data,null,2)}}</pre></details></template></section></div><PresentationPanel :template="session.template||selected" :session="session" @saved="session=$event;confirmed=false" @error="error=$event"/><div class="paper approval"><div><h3>Revizuire și aprobare</h3><p class="muted">Verificați raportul, măsurătorile și dovezile. Exportul în HIS necesită o acțiune separată.</p><label class="check" v-if="session.status==='review'||session.status==='approved'"><input type="checkbox" v-model="confirmed"/> Confirm revizuirea datelor clinice și versiunea {{session.revision}}.</label></div><div class="actions"><Button v-if="session.status==='recording'" label="Finalizează pentru revizuire" @click="action('finalize')" :disabled="!session.result||busy"/><Button v-if="session.status==='review'" label="Aprobă rezultatul" icon="pi pi-check" @click="action('approve')" :disabled="!confirmed||busy||session.result?.missing_fields.length>0||session.result?.validation_errors?.length>0"/><Button v-if="session.status==='approved'" label="Export JSON" icon="pi pi-download" @click="exportJSON"/><Button label="Șterge sesiunea" severity="danger" text @click="deleteSession" :disabled="busy"/></div></div><details v-if="session.status==='approved'" class="paper"><summary>Livrare către un conector autorizat</summary><label>ID conector<InputText v-model="connector"/></label><Button label="Trimite rezultatul aprobat" @click="delivery" :disabled="!confirmed||!connector||busy"/></details></template></template>
<template v-if="view==='templates'"><Message v-if="helpEnabled" severity="info" class="template-help-banner">Ajutor activ · Apasă pe pictogramele <i class="pi pi-info-circle" aria-hidden="true"/> de lângă etichete și acțiuni pentru explicații și exemple. Explicațiile rămân deschise până le închizi. <Button label="Oprește ajutorul" text @click="toggleHelp"/></Message><TemplateHelpDialog :topic="helpTopic" @close="helpTopic=null"/><TemplateEditor v-if="editorTemplate" :template="editorTemplate" :specialties="editorSpecialties" :templateLimitReached="templateLimitReached" :addonQuote="templateAddonQuote" :copyFromLibrary="!profile.instance_admin&&!editorTemplate.owner_id&&!!editorTemplate.id" @saved="saved" @error="error=$event"/><template v-else><Message v-if="needsSpecialty" severity="info">Alege o specialitate din <Button label="Pachetul meu" text @click="go('subscription')"/> pentru a accesa biblioteca de șabloane.</Message><label><span class="help-label">Caută după specialitate sau șablon<HelpHint topic="search"/></span><InputText v-model="catalogFilter" placeholder="Neurologie, cardiologie, medicină internă…"/></label><div class="section-heading"><h2>Biblioteca de șabloane · {{filteredTemplates.length}}<HelpHint topic="library"/></h2><Button label="Creează șablon" icon="pi pi-plus" @click="newTemplate" :disabled="profile.role==='viewer'||needsSpecialty"/><HelpHint topic="create"/></div><Message v-if="templateLimitReached" severity="warn">
  Ai folosit toate cele {{billingState.plan.max_templates}} locuri pentru șabloane proprii. Poți folosi în continuare șabloanele incluse din specialitățile active.
</Message><div class="three"><article class="paper pack" v-for="t in filteredTemplates"><span class="eyebrow">{{(t.specialties?.length?t.specialties:[t.specialty]).join(' · ')}} · V{{t.version}} · {{t.addon_monthly_minor?'Supliment lunar':t.owner_id?'Șablon propriu':'Inclus în bibliotecă'}}</span><h2>{{t.name}}</h2><p>{{t.fields.length}} câmpuri · {{t.fields.filter(f=>f.required).length}} obligatorii</p><div class="actions"><Button :label="t.owner_id||profile.instance_admin?'Editează':'Personalizează o copie'" icon="pi pi-pencil" @click="edit(t)" :disabled="profile.role==='viewer'"/><HelpHint topic="edit"/><Button label="Test sintetic" text @click="testTemplate(t)"/><HelpHint topic="synthetic"/></div></article></div></template></template>
<template v-if="view==='packs'"><label>Caută o specialitate<InputText v-model="catalogFilter" placeholder="Neurologie, cardiologie…"/></label><div class="three"><article class="paper pack" v-for="p in filteredPacks"><div class="pack-icon"><i aria-hidden="true" class="pi pi-box"/></div><span class="eyebrow">SPECIALTY PACK · BIBLIOTECĂ INIȚIALĂ</span><h2>{{p.name}}</h2><p>Șabloane cu versiuni înghețate, câmpuri locale și validare deterministă.</p><div class="actions"><Button label="Deschide șabloanele" @click="go('templates')"/></div></article></div></template>
<template v-if="view==='sessions'"><AdminSessions v-if="admin||profile?.instance_admin" :user-id="profile.user_id" @open-own="openSession({id:$event})"/><div v-else class="paper"><div v-if="!sessionList.length" class="empty">Nu există încă sesiuni.</div><div class="session-row" v-for="s in sessionList" tabindex="0" @click="openSession(s)" @keydown.enter="openSession(s)"><i aria-hidden="true" class="pi pi-file-edit"/><div><b>{{templates.find(t=>t.version_id===s.template_version)?.name||'Sesiune clinică'}}</b><small>{{new Date(s.created_at).toLocaleString('ro-RO')}} · {{s.id}}</small></div><span class="badge" :class="s.status">{{stateLabel[s.status]}}</span><i aria-hidden="true" class="pi pi-angle-right"/></div></div></template>
<template v-if="view==='users'"><form class="paper form-row" @submit.prevent="saveOrganization"><label>Organizație<InputText v-model="organizationName" required/></label><Button type="submit" label="Salvează organizația" :loading="busy"/></form><div class="two"><form class="paper" @submit.prevent="saveUser"><h2>{{user.id?'Modifică accesul':'Adaugă un coleg'}}</h2><label>Nume<InputText v-model="user.name" required/></label><label>Email<InputText v-model="user.email" type="email" required :disabled="!!user.id"/></label><label :key="user.id || 'new'">{{user.id?'Parolă nouă':'Parolă inițială'}}<InputText v-model="user.password" type="password" autocomplete="new-password" minlength="12" :required="!user.id" :placeholder="user.id?'Lasă gol pentru a păstra parola actuală':undefined"/></label><label>Rol<Select v-model="user.role" :options="['doctor','admin','viewer']"/></label><label>Identitate externă verificată<InputText v-model="user.external_subject" placeholder="WSM subject sau wisemed:user_id"/></label><label class="check"><input type="checkbox" v-model="user.disabled"/> Cont dezactivat</label><Button type="submit" label="Salvează utilizatorul" :loading="busy"/></form><div class="paper"><h2>Membrii organizației</h2><article v-for="u in rows" :key="u.id" class="session-row"><div><b>{{u.name}}</b><small>{{u.email}}</small></div><span class="badge">{{u.role}} {{u.disabled?'· dezactivat':''}}</span><div class="actions"><Button label="Editează" size="small" @click="go('users',u.id)"/><Button v-if="profile.instance_admin" label="Alocă pachet" size="small" severity="secondary" @click="go('billing',u.id)"/></div></article></div></div></template>
<template v-if="view==='memory'||view==='integrations'"><div class="paper"><h2>{{view==='memory'?'Preferințe aprobate și feedback':'Conectori și mapări tehnice'}}</h2><p class="muted">{{view==='memory'?'Memoria este separată de istoricul clinic. Nu introduceți copii de dosare. Preferințele salvate nu modifică dovezile clinice.':'Livrarea cere HTTPS, gazdă permisă în deployments și rezultat aprobat. Secretele sunt criptate și nu sunt returnate în clar.'}}</p><Select v-model="resourceTab" :options="view==='memory'?['memory/preferences','memory/examples','memory/feedback']:['integrations','connector-mappings']" @change="go(view,$event.value)"/></div><div class="paper form-row" v-if="resourceTab==='memory/examples'"><label>Caută în exemplele aprobate<InputText v-model="memoryQuery" placeholder="Cuvânt sau expresie"/></label><Button label="Caută" @click="searchMemory" :loading="busy"/></div><div class="two"><form class="paper" @submit.prevent="saveResource"><label>Denumire<InputText v-model="resourceName" required/></label><label>{{resourceTab==='integrations'?'Configurație conector (url, token)':'Conținut JSON'}}<Textarea v-model="resourceText" rows="10" class="code" required/></label><label class="check"><input type="checkbox" v-model="confirmed"/> Confirm această înregistrare.</label><Button type="submit" label="Salvează" :disabled="!confirmed" :loading="busy"/></form><div class="paper"><div v-if="!rows.length" class="empty">Nicio înregistrare încă.</div><article v-for="row in rows" class="record"><h3>{{row.name}}</h3><pre>{{JSON.stringify(row.payload,null,2)}}</pre><Button label="Șterge" severity="danger" text @click="deleteResource(row.id)"/></article></div></div></template>
<div v-if="view==='integrations'" class="paper table-wrap"><h2>Livrări și erori de integrare</h2><table><thead><tr><th>Livrare</th><th>Stare</th><th>Încercări</th><th>Eroare</th></tr></thead><tbody><tr v-for="d in deliveries"><td><code>{{d.id}}</code></td><td>{{d.status}}</td><td>{{d.attempts}} / 5</td><td>{{d.error||'—'}}</td></tr></tbody></table><p class="muted small" v-if="!deliveries.length">Nicio livrare solicitată.</p></div>
<template v-if="view==='usage'"><div class="stats"><article class="paper stat"><span>Cost estimat luna curentă</span><strong>${{Number(usage.cost_usd||0).toFixed(4)}}</strong></article><article class="paper stat"><span>Buget lunar</span><strong>${{usage.budget_usd}}</strong></article><article class="paper stat"><span>Tokeni de intrare</span><strong>{{usage.input_tokens||0}}</strong></article><article class="paper stat"><span>Tokeni de ieșire</span><strong>{{usage.output_tokens||0}}</strong></article></div><div class="paper"><h2>Costuri controlate</h2><Message v-if="usage.uncertain_requests" severity="warn">{{usage.uncertain_requests}} apeluri au cost rezervat sau incert: ${{Number(usage.reserved_usd).toFixed(4)}}. Rezervarea rămâne în buget până la reconcilierea facturării.</Message><p>Estimările folosesc tarifele configurate de operator. Verificarea bugetului este serializată per organizație. Modul mock nu generează costuri OpenAI.</p><progress :value="usage.cost_usd" :max="usage.budget_usd"/></div></template>
<template v-if="view==='audit'"><div class="paper table-wrap"><table><thead><tr><th>Moment</th><th>Acțiune</th><th>Entitate</th><th>Utilizator</th></tr></thead><tbody><tr v-for="row in rows"><td>{{new Date(row.created_at).toLocaleString('ro-RO')}}</td><td><span class="badge">{{row.action}}</span></td><td><code>{{row.entity_id}}</code></td><td><code>{{row.user_id}}</code></td></tr></tbody></table><div v-if="!rows.length" class="empty">Nu există evenimente de audit.</div></div></template>
<template v-if="view==='settings'"><div class="two"><form class="paper" @submit.prevent="saveSettings"><h2>Provider AI și buget</h2><label>Provider<Select v-model="settings.provider" :options="['mock','openai']"/></label><label>Model<InputText v-model="settings.model" required/></label><label>Cheie OpenAI <span class="muted">{{settings.openai_key_configured?'· configurată':'· neconfigurată'}}</span><InputText v-model="settings.openai_key" type="password" autocomplete="new-password" placeholder="Lăsați gol pentru a păstra cheia"/></label><div class="form-row"><label>Buget lunar USD<input type="number" min="0.01" step="0.01" v-model.number="settings.monthly_budget_usd"/></label><label>Retenție (zile)<input type="number" min="1" v-model.number="settings.retention_days"/></label></div><div class="form-row"><label>USD / milion tokeni intrare<input type="number" min="0" step="0.01" v-model.number="settings.input_usd_per_million"/></label><label>USD / milion tokeni ieșire<input type="number" min="0" step="0.01" v-model.number="settings.output_usd_per_million"/></label></div><Button label="Salvează configurația" type="submit" :loading="busy"/><p class="small muted">Modificările providerului, bugetului și retenției se aplică după repornire.</p></form><section class="paper"><h2>Infrastructură și integrări</h2><div class="status-line"><span>PostgreSQL · tabele wai_</span><span class="badge approved">{{settings.database_configured?'Configurat':'Neconfigurat'}}</span></div><div class="status-line"><span>Criptare date clinice</span><b>{{settings.encryption}}</b></div><div class="status-line"><span>Autentificare WiseMED</span><b>{{settings.wisemed_login_enabled?'Activă':'Opțională · inactivă'}}</b></div><div class="status-line"><span>Mapping identități WSM</span><b>{{settings.wsm_identity_enabled?'Activ':'Opțional · inactiv'}}</b></div><hr/><p class="muted">Configurațiile comune ale ecosistemului sunt disponibile în consola utilitarului, cu autentificarea sa de administrator.</p><a class="setting-link" href="/settings/wss" target="_blank" rel="noopener"><i aria-hidden="true" class="pi pi-link"/> WSM / WSS · autentificare și permisiuni <i aria-hidden="true" class="pi pi-external-link"/></a><a class="setting-link" href="/settings/reader" target="_blank" rel="noopener"><i aria-hidden="true" class="pi pi-server"/> Configurație utilitar și WiseMED API <i aria-hidden="true" class="pi pi-external-link"/></a><a class="setting-link" href="/settings/reader" target="_blank" rel="noopener"><i aria-hidden="true" class="pi pi-refresh"/> Actualizări · canal stable <i aria-hidden="true" class="pi pi-external-link"/></a></section></div><RuntimeSettings @error="error=$event" @notice="notice=$event"/></template>
<BillingPanel v-if="view==='billing'||view==='subscription'" :key="view" :profile="profile" :adminMode="view==='billing'" :userId="routeDetail" @select-user="go('billing',$event)" @account-changed="updateBillingAccount" @error="error=$event" @notice="notice=$event"/>
<SpeechModels v-if="view==='speech-models'&&profile.instance_admin" @error="error=$event"/>
<SpeechCommunity v-if="view==='speech'" :admin="admin||profile.instance_admin" @error="error=$event"/>
<template v-if="view==='account'"><SpeechConsent @error="error=$event"/><form class="paper account" @submit.prevent="saveAccount"><h2>Profil și securitate</h2><p class="muted">{{profile.email}} · {{profile.role}}</p><label>Nume<InputText v-model="account.name" required/></label><label>Parola curentă<InputText v-model="account.current_password" type="password" autocomplete="current-password"/></label><label>Parolă nouă<InputText v-model="account.password" type="password" autocomplete="new-password" minlength="12" placeholder="Minimum 12 caractere"/></label><Button label="Salvează profilul" type="submit" :loading="busy"/></form></template>
<footer><span>WiseMED AI · Asistent de documentare clinică</span><span>Decizia și aprobarea aparțin medicului.</span></footer>
</div></main></div>
</template>
