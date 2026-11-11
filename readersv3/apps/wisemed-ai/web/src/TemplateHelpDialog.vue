<script setup lang="ts">
import Dialog from 'primevue/dialog'
import {templateHelp, type TemplateHelpTopic} from './templateHelp'
defineProps<{topic: TemplateHelpTopic|null}>()
const emit=defineEmits<{close: []}>()
</script>
<template>
<Dialog :visible="topic!==null" modal :draggable="false" :header="topic?templateHelp[topic].title:''" class="template-help-dialog" :style="{width:'36rem',maxWidth:'calc(100vw - 2rem)'}" @update:visible="!$event&&emit('close')">
  <p v-if="topic" class="template-help-text">{{templateHelp[topic].text}}</p>
  <template #footer><Button label="Am înțeles" @click="emit('close')"/></template>
</Dialog>
</template>
