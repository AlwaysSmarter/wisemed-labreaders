<script setup lang="ts">
import {computed,ref} from 'vue'
import {api,type Template} from './api'
const props=defineProps<{session:any,template?:Template}>()
const emit=defineEmits<{saved:[any],error:[string]}>()
const active=ref<any>(null),value=ref(''),suggestions=ref<string[]>([]),busy=ref(false),issue=ref<any>(null),editRevision=ref(0)
const fields=computed(()=>props.template?.fields.filter(f=>!f.invisible)||[])
const issues=computed(()=>props.session.result?.field_issues||[])
function fieldValue(key:string){const v=props.session.result?.structured_data?.[key];return Array.isArray(v)?v.map(x=>x.value).join('; '):v?.value?.toString()||'—'}
const report=computed(()=>{const doc=new DOMParser().parseFromString(props.session.result?.rendered_html||'','text/html');for(const el of Array.from(doc.querySelectorAll('[data-field],[data-review-field]'))){const key=el.getAttribute('data-review-field')||el.getAttribute('data-field');el.setAttribute('data-review-field',key||'');el.setAttribute('tabindex','0');el.setAttribute('role','button');if(issues.value.some((i:any)=>i.field===key))el.setAttribute('style','background:#fee2e2;color:#991b1b;cursor:pointer');else el.setAttribute('style','cursor:pointer')}return doc.body.innerHTML})
function open(key:string){active.value=fields.value.find(f=>f.key===key);if(!active.value)return;editRevision.value=props.session.revision;value.value=fieldValue(key)==='—'?'':fieldValue(key);issue.value=issues.value.find((i:any)=>i.field===key);suggestions.value=[...(issue.value?.suggestions||[])]}
function click(e:Event){const el=(e.target as HTMLElement).closest('[data-review-field]');if(el)open(el.getAttribute('data-review-field')||'')}
async function options(){busy.value=true;try{const reply=await api('sessions/'+props.session.id+'/field-options','POST',{revision:editRevision.value,field:active.value.key});suggestions.value=reply.suggestions}catch(e){emit('error',String(e))}finally{busy.value=false}}
async function save(){busy.value=true;try{emit('saved',await api('sessions/'+props.session.id+'/field-edit','POST',{revision:editRevision.value,field:active.value.key,value:value.value}));active.value=null}catch(e){emit('error',String(e))}finally{busy.value=false}}
</script>
<template>
<div class="report" v-html="report" @click="click" @keydown.enter="click"/>
<details open class="field-review"><summary>Câmpurile documentului · apasă pentru corectare</summary><div class="field-grid"><button v-for="field in fields" :key="field.key" :class="{invalid:issues.some((i:any)=>i.field===field.key)}" @click="open(field.key)"><strong>{{field.label}}</strong><span>{{fieldValue(field.key)}}</span><small>{{issues.find((i:any)=>i.field===field.key)?.message}}</small></button></div></details>
<div v-if="active" class="field-overlay" role="dialog" aria-modal="true" :aria-label="active.label"><section class="paper field-dialog"><h2>{{active.label}}</h2><p v-if="issue" class="field-error">{{issue.message}}</p><blockquote v-if="issue?.quote">{{issue.quote}}</blockquote><p v-if="!value">— Nu există informație pentru acest câmp.</p><div class="actions"><Button v-for="option in suggestions" :key="option" :label="option" outlined @click="value=option"/><Button label="Propune interpretări din text" text @click="options" :loading="busy" :disabled="session.status==='approved'"/></div><label>Valoare · {{active.unit||'text'}}<Textarea v-model="value" rows="4" :disabled="session.status==='approved'"/></label><p class="muted small">Editarea sau alegerea unei variante este confirmarea ta pentru acest câmp. Celelalte câmpuri se păstrează. Poți redicta numai acest câmp din aplicația mobilă.</p><div class="actions"><Button label="Închide" text @click="active=null"/><Button label="Confirmă valoarea" @click="save" :loading="busy" :disabled="!value.trim()||session.status==='approved'"/></div></section></div>
</template>
<style scoped>
.field-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:10px;margin-top:12px}.field-grid button{text-align:left;border:1px solid #dfe5e2;border-radius:9px;padding:12px;background:#fff;cursor:pointer}.field-grid span,.field-grid small{display:block;margin-top:5px}.field-grid .invalid,.field-error{background:#fee2e2;color:#991b1b}.field-overlay{position:fixed;inset:0;background:#0007;display:grid;place-items:center;z-index:1000;padding:20px}.field-dialog{width:min(600px,100%);max-height:90vh;overflow:auto}.field-dialog textarea{width:100%}
</style>
