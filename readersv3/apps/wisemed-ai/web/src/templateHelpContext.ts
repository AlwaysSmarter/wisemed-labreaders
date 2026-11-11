import type {InjectionKey, Ref} from 'vue'
import type {TemplateHelpTopic} from './templateHelp'

export interface TemplateHelpContext {
  enabled: Readonly<Ref<boolean>>
  open: (topic: TemplateHelpTopic) => void
}
export const templateHelpKey: InjectionKey<TemplateHelpContext> = Symbol('templateHelp')
