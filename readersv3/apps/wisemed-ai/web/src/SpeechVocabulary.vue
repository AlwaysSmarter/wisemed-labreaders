<script setup lang="ts">
import HelpHint from './HelpHint.vue'
import {ref,onMounted} from 'vue'
import {api,type Template} from './api'
const props=defineProps<{template:Template}>()
const emit=defineEmits<{error:[string]}>()
const pack=ref<any>(null),text=ref<Record<string,string>>({}),busy=ref(false)
async function load(){if(!props.template.id)return;try{pack.value=await api('templates/'+props.template.id+'/speech-adaptation?manage=1&version_id='+props.template.version_id)}catch(e){emit('error',String(e))}}
async function save(section:string,value:string,status:string){busy.value=true;try{await api('templates/'+props.template.id+'/sections/'+section+'/speech-terms?version_id='+props.template.version_id,'POST',{text:value,status});text.value[section]='';await load()}catch(e){emit('error',String(e))}finally{busy.value=false}}
onMounted(load)
</script>
<template>
<section class="paper"><h2>Vocabular medical pentru dictare<HelpHint topic="vocabulary"/></h2>
<p class="muted">Termenii sunt legați de identificatorul secțiunii. Redenumirea și reordonarea păstrează asocierea. Publică mai întâi secțiunile noi. Adaugă numai terminologie medicală, fără nume, identificatori sau măsurători individuale.</p>
<template v-if="pack"><p>Versiunea vocabularului {{pack.vocabulary_version}} · maximum 64 de expresii și 2.048 de octeți activi pe secțiune.</p>
<details v-for="section in pack.sections" :key="section.id"><summary>{{section.label}} · {{section.terms.length}} expresii active</summary>
<div class="form-row"><label><span class="help-label">Termen sau expresie medicală<HelpHint topic="medicalTerm"/></span><Textarea v-model="text[section.id]" rows="2" maxlength="160"/></label><Button label="Aprobă termenul" :disabled="busy||!text[section.id]?.trim()" @click="save(section.id,text[section.id],'approved')"/></div>
<div class="table-wrap"><table><thead><tr><th>Termen</th><th>Stare / sursă<HelpHint topic="vocabStatus"/></th><th>Observări / proveniență<HelpHint topic="vocabOrigin"/></th><th>Revizuire<HelpHint topic="vocabStatus"/></th></tr></thead><tbody><tr v-for="entry in section.entries||[]" :key="entry.key"><td>{{entry.text}}</td><td>{{entry.status}} · {{entry.source}}</td><td>{{entry.observations}}<br/><small>{{entry.first_observed}} · {{entry.provenance||'manual'}}</small></td><td><Button v-if="entry.status!=='approved'" label="Aprobă" size="small" :disabled="busy" @click="save(section.id,entry.text,'approved')"/><Button label="Respinge" text size="small" :disabled="busy" @click="save(section.id,entry.text,'rejected')"/></td></tr></tbody></table></div>
<p class="muted small">Gramatică IR versionată · {{section.grammar_hash}}. Dictarea narativă rămâne liberă. Motoarele instalate nu expun un decodor compatibil pentru o gramatică restrictivă.</p>
</details></template><p v-else>Salvează șablonul pentru configurarea vocabularului.</p></section>
</template>
