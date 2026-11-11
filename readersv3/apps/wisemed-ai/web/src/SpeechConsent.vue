<script setup lang="ts">
import {ref,onMounted} from 'vue'
import {api} from './api'
const emit=defineEmits<{error:[string]}>()
const value=ref<any>(null),busy=ref(false)
onMounted(async()=>{try{value.value=await api('profile/speech-consent')}catch(e){emit('error',String(e))}})
async function change(){busy.value=true;try{value.value=await api('profile/speech-consent','PUT',{enabled:!value.value.enabled,notice_version:value.value.notice_version})}catch(e){emit('error',String(e))}finally{busy.value=false}}
</script>
<template><section class="paper"><h2>Share with WiseMED.ai community</h2><p>Permite contribuții cu înregistrări vocale și transcrieri din sesiuni începute după activare, pentru îmbunătățirea recunoașterii medicale. Mostrele sunt protejate și intră în dataset numai după revizuirea audio și a de-identificării. Feedbackul terminologic al șablonului este separat.</p><Button v-if="value" :label="value.enabled?'Retrage acordul':'Activează contribuțiile viitoare'" :severity="value.enabled?'secondary':'primary'" :disabled="busy" @click="change"/><p v-if="value" class="muted small">{{value.notice_version}} · {{value.enabled?'Activ din '+value.activated_at:'Dezactivat'}}. Retragerea blochează contribuțiile și revocă eligibilitatea exporturilor viitoare. Copiile deja exportate necesită gestionare de către operatorul datasetului.</p></section></template>
