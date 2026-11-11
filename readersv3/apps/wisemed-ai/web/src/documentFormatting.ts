import {Extension} from '@tiptap/core'

// Only these alignment values are persisted by the template HTML sanitizer.
export const DocumentAlignment=Extension.create({
  name:'documentAlignment',
  addGlobalAttributes(){return [{types:['paragraph','heading'],attributes:{textAlign:{
    default:null,
    parseHTML:element=>['left','center','right','justify'].includes(element.style.textAlign)?element.style.textAlign:null,
    renderHTML:attributes=>['left','center','right','justify'].includes(attributes.textAlign)?{style:`text-align: ${attributes.textAlign}`}:{},
  }}}]},
})
