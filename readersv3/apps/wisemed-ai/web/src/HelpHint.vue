<script setup lang="ts">
import {inject} from 'vue'
import {templateHelp, type TemplateHelpTopic} from './templateHelp'
import {templateHelpKey} from './templateHelpContext'
defineProps<{topic: TemplateHelpTopic}>()
const help=inject(templateHelpKey)
</script>
<template>
<span v-if="help?.enabled.value" role="button" tabindex="0" class="help-hint" :aria-label="'Ajutor: '+templateHelp[topic].title" aria-haspopup="dialog" @click.stop.prevent="help.open(topic)" @keydown.enter.stop.prevent="help.open(topic)" @keydown.space.stop.prevent="help.open(topic)"><i class="pi pi-info-circle" aria-hidden="true"/></span>
</template>
