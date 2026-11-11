<script setup lang="ts">
import {ref,computed,onMounted,watch} from 'vue'
import {api,clone} from './api'
import SubscriptionPanel from './SubscriptionPanel.vue'
import {type Plan,emptyPlan,priceKeys,parseMoney,money} from './billing'
const props=defineProps<{profile:any,adminMode?:boolean,userId?:string}>()
const emit=defineEmits<{error:[string],notice:[string],selectUser:[string],accountChanged:[any]}>()
const plans=ref<Plan[]>([]),account=ref<any>(null),busy=ref(false),draft=ref<Plan|null>(null),prices=ref<Record<string,string>>({}),users=ref<any[]>([]),usage=ref<any[]>([]),selectedSpecialties=ref<string[]>([]),selectedUser=ref<any>(null),assignedVersion=ref(''),creditOverride=ref(''),assignmentNote=ref(''),confirmed=ref(false),selectedUsage=ref<any[]>([]),selectedStatistics=ref<any[]>([]),roleType=ref(1),disabled=ref(false),filter=ref(''),statistics=ref<any[]>([])
const superAdmin=computed(()=>props.profile.instance_admin===true)
const activePlans=computed(()=>plans.value.filter(p=>p.active))
const filteredUsers=computed(()=>users.value.filter(u=>(u.name+' '+u.email).toLowerCase().includes(filter.value.toLowerCase())))
const labels:Record<string,string>={monthly_price_minor:'Preț lunar',extra_specialty_price_minor:'Specialitate suplimentară / lună',extra_template_price_minor:'Șablon suplimentar / lună',extra_million_tokens_price_minor:'Un milion de tokenuri suplimentare',credit_limit_minor:'Limită de credit implicită'}
async function run(fn:()=>Promise<void>){busy.value=true;try{await fn()}catch(e){emit('error',e instanceof Error?e.message:String(e))}finally{busy.value=false}}
async function load(){plans.value=await api('billing/plans');account.value=await api('billing/account');selectedSpecialties.value=[...account.value.selected_specialties];usage.value=await api('billing/usage');statistics.value=(await api('billing/statistics?period=all')).items;if(superAdmin.value&&props.adminMode)users.value=await api('billing/users')}
function editPlan(plan:Plan){draft.value=clone(plan);prices.value={};for(const key of priceKeys)prices.value[key]=(plan[key]/100).toFixed(2)}
function newPlan(){editPlan(emptyPlan())}
async function savePlan(){await run(async()=>{if(!draft.value)return;const payload=clone(draft.value);for(const key of priceKeys)payload[key]=parseMoney(prices.value[key]);const saved=await api<Plan>('billing/plans','POST',payload);await load();editPlan(saved);emit('notice','Pachetul a fost publicat ca versiune nouă. Alocările existente păstrează versiunea și tarifele lor.')})}
async function saveSpecialties(){await run(async()=>{await api('billing/specialties','PUT',{specialties:selectedSpecialties.value});await load();emit('accountChanged',account.value);emit('notice','Specialitățile active au fost salvate.')})}
const newPassword=ref('')
async function cancelAddon(addon:any){
  if(!window.confirm('Oprești reînnoirea pentru „'+addon.name+'” la sfârșitul lunii UTC?'))return
  await run(async()=>{
    await api('billing/template-addons/'+addon.template_id,'DELETE')
    await load()
    emit('accountChanged',account.value)
    emit('notice','Reînnoirea șablonului a fost oprită.')
  })
}
type PromptMessage = {role:string,content:string}
type ProcessingDetails = {messages:PromptMessage[],output?:{content:string,refusal?:string,finish_reason?:string}|null}
const selectedPrompt=ref<{id:string,details:ProcessingDetails|null}|null>(null)
async function showPrompt(id:string){
  if(selectedPrompt.value?.id===id){selectedPrompt.value=null;return}
  selectedPrompt.value={id,details:null}
  const current=selectedPrompt.value
  try {
    const result=await api<ProcessingDetails>('billing/usage/prompt?id='+encodeURIComponent(id))
    if(selectedPrompt.value===current)current.details=result
  } catch(e) {
    if(selectedPrompt.value!==current)return
    selectedPrompt.value=null
    emit('error',e instanceof Error?e.message:String(e))
  }
}
function selectUser(user:any){selectedPrompt.value=null;newPassword.value='';selectedUsage.value=[];void run(async()=>{const items=await api('billing/usage?user_id='+encodeURIComponent(user.id));if(selectedUser.value?.id===user.id){selectedUsage.value=items;selectedStatistics.value=(await api('billing/statistics?period=all&user_id='+encodeURIComponent(user.id))).items}});selectedUser.value=user;assignedVersion.value=plans.value.find(p=>p.id===user.plan.plan_id&&p.active)?.version_id||'';creditOverride.value=user.credit_limit_minor==null?'':(user.credit_limit_minor/100).toFixed(2);assignmentNote.value='';confirmed.value=false;roleType.value=user.super_admin?-1:user.memberships.some((m:any)=>m.role==='admin')?0:1;disabled.value=user.disabled}
async function assign(){await run(async()=>{await api('billing/assignments','POST',{user_id:selectedUser.value.id,plan_version:assignedVersion.value,credit_limit_minor:creditOverride.value.trim()===''?null:parseMoney(creditOverride.value),note:assignmentNote.value,confirmed:confirmed.value});await load();emit('selectUser','');confirmed.value=false;emit('notice','Pachet alocat administrativ. Nu a fost încasată nicio plată.')})}
async function saveRole(){await run(async()=>{await api('billing/users/role','PUT',{user_id:selectedUser.value.id,user_type:roleType.value,disabled:disabled.value,confirmed:confirmed.value});await load();emit('selectUser','');confirmed.value=false;emit('notice','Rolul și starea utilizatorului au fost actualizate.')})}
async function savePassword(){
  await run(async()=>{
    await api('billing/users/password','PUT',{user_id:selectedUser.value.id,password:newPassword.value})
    newPassword.value=''
    emit('notice','Parola WiseMED AI a fost schimbată. Sesiunile utilizatorului au fost închise.')
  })
}
watch([()=>props.userId,users],([id,list])=>{
  if(!id){
    selectedUser.value=null
    newPassword.value=''
    return
  }
  const target=list.find(u=>u.id===id)
  if(target&&target.id!==selectedUser.value?.id)selectUser(target)
})
onMounted(()=>run(load))
</script>
<template>
<div v-if="busy" class="loading" role="status"><i aria-hidden="true" class="pi pi-spin pi-spinner"/> Se încarcă pachetele…</div>
<template v-if="adminMode&&superAdmin">
<div class="section-heading"><div><h2>Catalog de servicii</h2><p class="muted small">Prețurile și limitele sunt definite de super admin. Pachetele au versiuni; editarea nu schimbă alocările existente.</p></div><Button label="Pachet nou" icon="pi pi-plus" @click="newPlan"/></div>
<div class="three"><article class="paper" v-for="plan in plans" :key="plan.version_id"><span class="eyebrow">{{plan.code}} · V{{plan.version}} · {{plan.active?'ACTIV':'SCHIȚĂ'}}</span><h2>{{plan.name}}</h2><strong class="plan-price">{{money(plan.monthly_price_minor,plan.currency)}}<small> / lună</small></strong><p>{{plan.description}}</p><ul class="plan-limits"><li>{{plan.max_specialties}} specialități active</li><li>{{plan.max_templates}} șabloane proprii</li><li>{{plan.included_ai_tokens.toLocaleString('ro-RO')}} tokenuri AI / lună</li><li>{{plan.max_fields_per_template}} câmpuri / șablon</li></ul><Button label="Editează pachetul" severity="secondary" @click="editPlan(plan)"/></article></div>
<form v-if="draft" class="paper" @submit.prevent="savePlan"><div class="section-heading"><h2>{{draft.id?'Versiune nouă pentru '+draft.name:'Definire pachet'}}</h2><Button label="Închide" text severity="secondary" @click="draft=null"/></div><div class="three"><label>Cod stabil<InputText v-model="draft.code" :readonly="!!draft.id" required pattern="[a-z][a-z0-9_-]{1,39}"/></label><label>Denumire<InputText v-model="draft.name" required minlength="2" maxlength="100"/></label><label>Monedă<Select v-model="draft.currency" :options="['USD','EUR','RON']"/></label></div><label>Descriere<Textarea v-model="draft.description" rows="2" maxlength="2000"/></label><div class="three"><label>Specialități maxime<input type="number" min="1" max="10000" v-model.number="draft.max_specialties" required/></label><label>Șabloane proprii maxime<input type="number" min="0" max="1000000" v-model.number="draft.max_templates" required/></label><label>Tokenuri AI incluse / lună<input type="number" min="0" max="1000000000000" v-model.number="draft.included_ai_tokens" required/></label></div><label>Complexitate: număr maxim de câmpuri / șablon<input type="number" min="1" max="150" v-model.number="draft.max_fields_per_template" required/></label><h3>Tarife și credit · {{draft.currency}}</h3><div class="three"><label v-for="key in priceKeys" :key="key">{{labels[key]}}<InputText v-model="prices[key]" inputmode="decimal" required pattern="[0-9]+([.,][0-9]{0,2})?"/></label></div><label class="check"><input type="checkbox" v-model="draft.extras_enabled"/> Oferă suplimente pentru acest pachet (definirea ofertei)</label><label class="check"><input type="checkbox" v-model="draft.active" :disabled="draft.code==='free'"/> Pachet activ în catalog</label><Message severity="info">Consumul AI suplimentar folosește creditul numai cu abonament, suplimente active și tarif pozitiv pe milionul de tokenuri. Plățile verificate se înregistrează de super admin; EuPlătesc urmează.</Message><Button label="Publică pachetul" type="submit" icon="pi pi-check" :loading="busy"/></form>
<section class="paper"><div class="section-heading"><h2>Utilizatorii din sistem și pachetele alocate</h2><InputText v-model="filter" aria-label="Caută utilizator" placeholder="Nume sau email"/></div><div class="table-wrap"><table><thead><tr><th>Utilizator</th><th>Rol</th><th>Organizații</th><th>Pachet curent</th><th></th></tr></thead><tbody><tr v-for="u in filteredUsers" :key="u.id"><td><b>{{u.name}}</b><br/><small>{{u.email}}</small></td><td>{{u.super_admin?'Super admin (-1)':u.memberships.some((m:any)=>m.role==='admin')?'Admin (0)':'Utilizator'}} {{u.disabled?'· dezactivat':''}}</td><td><small v-for="m in u.memberships" :key="m.tenant_id">{{m.organization}} · {{m.role}}<br/></small></td><td>{{u.plan.name}} · v{{u.plan.version}}<br/><small>{{money(u.plan.monthly_price_minor,u.plan.currency)}} / lună</small></td><td><Button label="Gestionează" size="small" :disabled="busy" @click="emit('selectUser',u.id)"/></td></tr></tbody></table></div></section>
<form v-if="selectedUser" class="paper" @submit.prevent="assign"><h2>{{selectedUser.name}} · {{selectedUser.email}}</h2><p class="muted small">Pachet curent: {{selectedUser.plan.name}}, versiunea {{selectedUser.plan.version}}. Alocarea de mai jos este administrativă și nu reprezintă cumpărare sau plată.</p><div class="two"><section><label>Pachet de alocat<Select v-model="assignedVersion" :options="activePlans" optionLabel="name" optionValue="version_id" placeholder="Selectați un pachet activ"/></label><label>Limită de credit individuală · {{plans.find(p=>p.version_id===assignedVersion)?.currency||''}}<InputText v-model="creditOverride" inputmode="decimal" placeholder="Gol folosește limita pachetului"/></label><label>Motivul alocării<Textarea v-model="assignmentNote" rows="2" maxlength="500"/></label><Button label="Aplică pachetul" type="submit" :disabled="!confirmed||!assignedVersion" :loading="busy"/></section><section><label>Tip utilizator<Select v-model="roleType" :options="[{label:'Super admin (-1)',value:-1},{label:'Admin (0)',value:0},{label:'Utilizator normal',value:1}]" optionLabel="label" optionValue="value"/></label><label class="check"><input type="checkbox" v-model="disabled"/> Cont dezactivat</label><Button label="Salvează rolul și starea" severity="secondary" @click="saveRole" :disabled="!confirmed||selectedUser.id===profile.user_id" :loading="busy"/><p class="muted small">Modificarea rolului global se aplică apartenențelor utilizatorului. Propriul rol de super admin este protejat.</p></section></div><label class="check"><input type="checkbox" v-model="confirmed"/> Confirm explicit modificarea selectată.</label><details><summary>Statistici pe șablon ale utilizatorului</summary><div class="table-wrap"><table><thead><tr><th>Șablon</th><th>Operație</th><th>Procesări</th><th>Intrare</th><th>Ieșire</th><th>Cache</th></tr></thead><tbody><tr v-for="s in selectedStatistics"><td>{{s.template_name}}<br/><small>{{s.template_version}}</small></td><td>{{s.operation}}</td><td>{{s.calls}}</td><td>{{s.input_tokens}}</td><td>{{s.output_tokens}}</td><td>{{s.cached_input_tokens}}</td></tr></tbody></table></div></details><details><summary>Consumul AI al utilizatorului · {{selectedUsage.length}} procesări recente</summary><div class="table-wrap"><table><thead><tr><th>Operație</th><th>Șablon</th><th>Intrare</th><th>Ieșire</th><th>Total</th><th>Stare</th><th>Detalii</th></tr></thead><tbody><template v-for="u in selectedUsage" :key="u.id"><tr><td>{{u.operation}}<br/><small>{{new Date(u.created_at).toLocaleString('ro-RO')}}</small></td><td><code>{{u.template_version||'—'}}</code></td><td>{{u.input_tokens}}</td><td>{{u.output_tokens}}</td><td>{{u.total_tokens}}</td><td>{{u.status}}</td><td><Button type="button" size="small" :label="selectedPrompt?.id===u.id?'Închide':'Vezi detalii'" :aria-expanded="selectedPrompt?.id===u.id" @click="showPrompt(u.id)"/></td></tr>
<tr v-if="selectedPrompt && selectedPrompt.id===u.id"><td colspan="7">
  <p v-if="selectedPrompt.details===null" role="status">Se încarcă detaliile…</p>
  <p v-else-if="!selectedPrompt.details.messages.length">Detalii indisponibile pentru această procesare.</p>
  <div v-for="(message,index) in selectedPrompt.details?.messages || []" :key="index">
    <strong>{{message.role==='system'?'Instrucțiuni de sistem':message.role==='user'?'Context trimis':'Prompt de transcriere'}}</strong>
    <pre style="white-space:pre-wrap;overflow-wrap:anywhere;max-height:24rem;overflow:auto">{{message.content}}</pre>
  </div>
<template v-if="selectedPrompt.details?.messages.length">
    <strong>Răspunsul AI</strong>
    <template v-if="selectedPrompt.details.output">
      <p v-if="selectedPrompt.details.output.finish_reason" class="muted small">Motiv de încheiere: {{selectedPrompt.details.output.finish_reason}}</p>
      <pre style="white-space:pre-wrap;overflow-wrap:anywhere;max-height:24rem;overflow:auto">{{selectedPrompt.details.output.content}}</pre>
      <p v-if="selectedPrompt.details.output.refusal">{{selectedPrompt.details.output.refusal}}</p>
    </template>
    <p v-else>Nu există un răspuns AI înregistrat.</p>
  </template>
</td></tr></template></tbody></table></div></details></form>
<form v-if="selectedUser && selectedUser.id!==profile.user_id" :key="selectedUser.id" class="paper" @submit.prevent="savePassword">
  <h2>Schimbă parola WiseMED AI</h2>
  <p class="muted small">{{selectedUser.name}} · {{selectedUser.email}}. Salvarea închide sesiunile active.</p>
  <label>Parolă nouă<InputText v-model="newPassword" type="password" autocomplete="new-password" minlength="12" maxlength="72" required :disabled="busy"/></label>
  <Button label="Schimbă parola" type="submit" :loading="busy"/>
</form>
<SubscriptionPanel v-if="selectedUser" :user-id="selectedUser.id" :admin="true" :plans="plans" @changed="run(load)" @error="emit('error',$event)"/>
</template>
<template v-else-if="account">
<section class="paper"><span class="eyebrow">PACHETUL MEU</span><div class="section-heading"><div><h2>{{account.plan.name}} · v{{account.plan.version}}</h2><p>{{money(account.plan.monthly_price_minor,account.plan.currency)}} / lună</p></div><span class="badge">{{account.quota_exempt?'Super admin · exceptat de limitele comerciale':'Limite verificate pe server'}}</span></div><div class="stats"><article class="stat"><span>Specialități active</span><strong>{{account.selected_specialties.length}} / {{account.plan.max_specialties}}</strong></article><article class="stat"><span>Șabloane proprii incluse</span><strong>{{account.templates_used}} / {{account.plan.max_templates}}</strong></article><article class="stat"><span>Tokenuri efectiv consumate</span><strong>{{account.total_tokens.toLocaleString('ro-RO')}}</strong><small>din {{account.plan.included_ai_tokens.toLocaleString('ro-RO')}} tokenuri incluse</small></article><article class="stat"><span>Tokenuri rezervate / incerte</span><strong>{{account.reserved_tokens.toLocaleString('ro-RO')}}</strong></article></div><p class="muted small">Complexitate permisă: maximum {{account.plan.max_fields_per_template}} câmpuri / șablon.</p><p class="muted small">Tokenuri incluse = intrare + ieșire raportate de provider. Perioadă: luna calendaristică UTC. Modul mock nu consumă tokenuri AI facturabile. Rezervările protejează limita în timpul procesării sau când răspunsul providerului este incert.</p></section>
<Message v-if="!account.quota_exempt&&account.total_tokens+account.reserved_tokens>=account.plan.included_ai_tokens" severity="warn">Limita de tokenuri este atinsă sau rezervată integral. Solicită administratorului un pachet cu mai multe tokenuri; cu abonament și suplimente active, consumul continuă în limita creditului disponibil.</Message><section class="paper"><h2>Specialitățile mele</h2><p class="muted small">Selectați cel mult {{account.plan.max_specialties}} specialități. Șabloanele din biblioteca comună nu consumă numărul de șabloane proprii; personalizarea creează o copie proprie.</p><div class="specialty-options"><label class="check" v-for="specialty in account.available_specialties" :key="specialty"><input type="checkbox" v-model="selectedSpecialties" :value="specialty" :disabled="profile.role==='viewer'||!account.quota_exempt&&!selectedSpecialties.includes(specialty)&&selectedSpecialties.length>=account.plan.max_specialties"/>{{specialty}}</label></div><Button v-if="profile.role!=='viewer'" label="Salvează specialitățile" @click="saveSpecialties" :loading="busy"/></section>
<section v-if="account.template_addons?.length" class="paper">
  <h2>Șabloane suplimentare</h2>
  <div v-for="addon in account.template_addons" :key="addon.template_id" class="status-line">
    <div><strong>{{addon.name}}</strong> · {{money(addon.monthly_minor,addon.currency)}} / lună
      <p v-if="addon.ends_at" class="muted small">Acces până la {{new Date(addon.ends_at).toLocaleDateString('ro-RO',{timeZone:'UTC'})}} (UTC).</p>
    </div>
    <Button v-if="!addon.ends_at&&profile.role!=='viewer'" label="Oprește reînnoirea" text :loading="busy" @click="cancelAddon(addon)"/>
  </div>
</section>
<SubscriptionPanel :plans="plans" @changed="run(load)" @error="emit('error',$event)"/>
<section class="paper"><h2>Suplimente și credit configurate</h2><div class="three"><article><h3>Specialitate suplimentară</h3><p>{{money(account.plan.extra_specialty_price_minor,account.plan.currency)}} / lună</p></article><article><h3>Șablon suplimentar</h3><p>{{money(account.plan.extra_template_price_minor,account.plan.currency)}} / lună</p></article><article><h3>Un milion de tokenuri</h3><p>{{money(account.plan.extra_million_tokens_price_minor,account.plan.currency)}}</p></article></div><p>Limită de credit configurată: <b>{{money(account.credit_limit_minor,account.plan.currency)}}</b></p><Message severity="info">Creditul AI funcționează pentru abonamentele cu suplimente active. Epuizarea creditului suspendă lucrul până la achitarea soldului necesar. Șabloanele suplimentare se pot activa din editor dacă pachetul oferă un tarif. Plățile verificate sunt înregistrate de super admin.</Message></section>
<div class="three"><article v-for="plan in plans" class="paper"><span class="eyebrow">{{plan.code}}</span><h2>{{plan.name}}</h2><p>{{money(plan.monthly_price_minor,plan.currency)}} / lună</p><p>{{plan.max_specialties}} specialități · {{plan.max_templates}} șabloane proprii<br/>{{plan.included_ai_tokens.toLocaleString('ro-RO')}} tokenuri AI</p><span class="badge">{{plan.id===account.plan.id?'Pachet curent':'Alocare prin administrator'}}</span></article></div>
<section class="paper table-wrap"><h2>Statistici pe șablon și operație</h2><p class="muted small">Istoric complet, maximum 500 de grupuri. Tokenurile cache sunt incluse în tokenurile de intrare. Diferența în octeți măsoară contextul compactat, nu o economie garantată în bani.</p><table><thead><tr><th>Șablon / versiune</th><th>Operație</th><th>Procesări</th><th>Intrare / ieșire</th><th>Cache intrare</th><th>Context inițial / trimis</th></tr></thead><tbody><tr v-for="s in statistics" :key="(s.template_version||'')+s.operation+s.model+s.provider"><td>{{s.template_name}}<br/><small>{{s.template_version||'Încă nepublicat'}}</small></td><td>{{s.operation}}<br/><small>{{s.provider}} · {{s.model}}</small></td><td>{{s.calls}}</td><td>{{s.input_tokens}} / {{s.output_tokens}}</td><td>{{s.cached_input_tokens}}</td><td>{{s.original_context_bytes}} / {{s.sent_context_bytes}} B</td></tr></tbody></table></section><section class="paper table-wrap"><h2>Istoric de procesări AI</h2><table><thead><tr><th>Moment / operație</th><th>Șablon / sesiune</th><th>Intrare</th><th>Ieșire</th><th>Total</th><th>Stare</th></tr></thead><tbody><tr v-for="record in usage" :key="record.id"><td>{{new Date(record.created_at).toLocaleString('ro-RO')}}<br/><small>{{record.operation}} · {{record.model}}</small></td><td><code>{{record.template_version||'—'}}</code><br/><code>{{record.session_id||'—'}}</code></td><td>{{record.input_tokens}}</td><td>{{record.output_tokens}}</td><td>{{record.total_tokens}}</td><td><span class="badge">{{record.status}}</span><small v-if="record.reserved_tokens&&['reserved','uncertain'].includes(record.status)"> · {{record.reserved_tokens}} rezervați</small></td></tr></tbody></table><p v-if="!usage.length" class="muted">Nu există încă procesări AI facturabile.</p></section>
</template>
</template>
