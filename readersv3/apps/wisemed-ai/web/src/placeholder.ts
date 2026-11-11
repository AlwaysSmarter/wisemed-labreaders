import {Node,mergeAttributes} from '@tiptap/core'
export const Placeholder=Node.create({name:'placeholder',group:'inline',inline:true,atom:true,selectable:true,
 addAttributes(){return {field:{default:'camp'},label:{default:'Câmp'}}},
 parseHTML(){return [{tag:'span[data-field]',getAttrs:(el)=>({field:(el as HTMLElement).getAttribute('data-field'),label:(el as HTMLElement).textContent})}]},
 renderHTML({HTMLAttributes}){return ['span',mergeAttributes({'data-field':HTMLAttributes.field,class:'field-node'}),HTMLAttributes.label||HTMLAttributes.field]}
})
export const ClinicalSection=Node.create({name:'clinicalSection',group:'block',content:'block+',defining:true,
 addAttributes(){return {section:{default:''}}},
 parseHTML(){return [{tag:'section[data-section]',getAttrs:(el)=>({section:(el as HTMLElement).getAttribute('data-section')})}]},
 renderHTML({HTMLAttributes}){return ['section',{'data-section':HTMLAttributes.section},0]}
})
