<script setup lang="ts">
import {computed,ref,onBeforeUnmount} from 'vue'
import type {Editor} from '@tiptap/vue-3'
import HelpHint from './HelpHint.vue'
const props=defineProps<{editor:Editor,disabled:boolean}>()
const revision=ref(0)
const update=()=>{revision.value++}
props.editor.on('transaction',update)
onBeforeUnmount(()=>props.editor.off('transaction',update))
const block=computed(()=>{revision.value;return [1,2,3].find(level=>props.editor.isActive('heading',{level}))?.toString()||'paragraph'})
function setBlock(event:Event){const value=(event.target as HTMLSelectElement).value;const chain=props.editor.chain().focus();if(value==='paragraph')chain.setParagraph().run();else chain.setHeading({level:Number(value) as 1|2|3}).run()}
type ToolButton={label:string,icon:string,glyph?:string,mark?:string,active?:boolean,disabled?:boolean,run:()=>boolean}
const groups=computed<{label:string,buttons:ToolButton[]}[]>(()=>{revision.value;const e=props.editor;return [
  {label:'Istoric',buttons:[
    {label:'Anulează',icon:'pi pi-undo',run:()=>e.chain().focus().undo().run(),disabled:!e.can().undo()},
    {label:'Refă',icon:'pi pi-refresh',run:()=>e.chain().focus().redo().run(),disabled:!e.can().redo()}
  ]},
  {label:'Formatare text',buttons:[
    {label:'Aldin',icon:'',glyph:'B',mark:'bold',active:e.isActive('bold'),run:()=>e.chain().focus().toggleBold().run()},
    {label:'Cursiv',icon:'',glyph:'I',mark:'italic',active:e.isActive('italic'),run:()=>e.chain().focus().toggleItalic().run()},
    {label:'Subliniat',icon:'',glyph:'U',mark:'underline',active:e.isActive('underline'),run:()=>e.chain().focus().toggleUnderline().run()},
    {label:'Tăiat',icon:'',glyph:'S',mark:'strikethrough',active:e.isActive('strike'),run:()=>e.chain().focus().toggleStrike().run()},
    {label:'Șterge formatarea',icon:'pi pi-eraser',run:()=>e.chain().focus().unsetAllMarks().clearNodes().run()}
  ]},
  {label:'Aliniere',buttons:[
    ...([{value:'left',label:'Aliniază la stânga',icon:'pi pi-align-left'},{value:'center',label:'Centrează',icon:'pi pi-align-center'},{value:'right',label:'Aliniază la dreapta',icon:'pi pi-align-right'},{value:'justify',label:'Aliniere justificată',icon:'pi pi-align-justify'}]).map(a=>({...a,active:e.isActive({textAlign:a.value}),run:()=>e.chain().focus().updateAttributes('heading',{textAlign:a.value}).updateAttributes('paragraph',{textAlign:a.value}).run()}))
  ]},
  {label:'Liste și structură',buttons:[
    {label:'Listă cu marcatori',icon:'pi pi-list',active:e.isActive('bulletList'),run:()=>e.chain().focus().toggleBulletList().run()},
    {label:'Listă numerotată',icon:'pi pi-sort-numeric-down',active:e.isActive('orderedList'),run:()=>e.chain().focus().toggleOrderedList().run()},
    {label:'Mărește indentarea listei',icon:'pi pi-angle-double-right',disabled:!e.can().sinkListItem('listItem'),run:()=>e.chain().focus().sinkListItem('listItem').run()},
    {label:'Micșorează indentarea listei',icon:'pi pi-angle-double-left',disabled:!e.can().liftListItem('listItem'),run:()=>e.chain().focus().liftListItem('listItem').run()},
    {label:'Citat',icon:'pi pi-comment',active:e.isActive('blockquote'),run:()=>e.chain().focus().toggleBlockquote().run()},
    {label:'Linie de separare',icon:'pi pi-minus',run:()=>e.chain().focus().setHorizontalRule().run()}
  ]},
  {label:'Tabel',buttons:[{label:'Inserează tabel',icon:'pi pi-table',run:()=>e.chain().focus().insertTable({rows:3,cols:3,withHeaderRow:true}).run()}]}
]})
const tableButtons=computed(()=>{revision.value;const e=props.editor;return [
 {label:'Rând deasupra',run:()=>e.chain().focus().addRowBefore().run(),disabled:!e.can().addRowBefore()},
 {label:'Rând dedesubt',run:()=>e.chain().focus().addRowAfter().run(),disabled:!e.can().addRowAfter()},
 {label:'Coloană la stânga',run:()=>e.chain().focus().addColumnBefore().run(),disabled:!e.can().addColumnBefore()},
 {label:'Coloană la dreapta',run:()=>e.chain().focus().addColumnAfter().run(),disabled:!e.can().addColumnAfter()},
 {label:'Antet de tabel',run:()=>e.chain().focus().toggleHeaderRow().run(),disabled:!e.can().toggleHeaderRow()},
 {label:'Unește celulele',run:()=>e.chain().focus().mergeCells().run(),disabled:!e.can().mergeCells()},
 {label:'Separă celula',run:()=>e.chain().focus().splitCell().run(),disabled:!e.can().splitCell()},
 {label:'Șterge rândul',run:()=>e.chain().focus().deleteRow().run(),disabled:!e.can().deleteRow()},
 {label:'Șterge coloana',run:()=>e.chain().focus().deleteColumn().run(),disabled:!e.can().deleteColumn()},
 {label:'Șterge tabelul',run:()=>e.chain().focus().deleteTable().run(),disabled:!e.can().deleteTable()}
]})
const inTable=computed(()=>{revision.value;return props.editor.isActive('table')})
</script>
<template>
<fieldset class="document-tools" :disabled="disabled" aria-label="Formatarea documentului">
  <div class="document-toolbar" role="toolbar" aria-label="Formatare WYSIWYG">
    <select aria-label="Stil de paragraf" :value="block" @change="setBlock"><option value="paragraph">Text normal</option><option value="1">Titlu 1</option><option value="2">Titlu 2</option><option value="3">Titlu 3</option></select>
    <div v-for="group in groups" :key="group.label" class="document-tool-group" role="group" :aria-label="group.label">
      <Button v-for="button in group.buttons" :key="button.label" :icon="button.icon" :aria-label="button.label" :title="button.label" :aria-pressed="'active' in button?button.active:undefined" :class="{selected:'active' in button&&button.active}" :disabled="button.disabled" text severity="secondary" size="small" @mousedown.prevent @click="button.run"><template v-if="button.glyph" #default><span aria-hidden="true" :class="['document-mark','document-mark-'+button.mark]">{{button.glyph}}</span></template></Button>
    </div>
    <HelpHint topic="format"/>
  </div>
  <div v-if="inTable" class="document-table-tools" role="toolbar" aria-label="Editarea tabelului"><span>Tabel selectat</span><Button v-for="button in tableButtons" :key="button.label" :label="button.label" :disabled="button.disabled" text size="small" severity="secondary" @mousedown.prevent @click="button.run"/><HelpHint topic="tables"/></div>
</fieldset>
</template>
