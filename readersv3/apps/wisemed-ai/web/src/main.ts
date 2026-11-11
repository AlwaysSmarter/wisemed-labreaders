import {createApp} from 'vue'
import PrimeVue from 'primevue/config'
import Aura from '@primeuix/themes/aura'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import Message from 'primevue/message'
import App from './App.vue'
import 'primeicons/primeicons.css'
import './style.css'
const app=createApp(App)
app.use(PrimeVue,{theme:{preset:Aura,options:{darkModeSelector:'.dark'}}})
app.component('Button',Button).component('InputText',InputText).component('Textarea',Textarea).component('Select',Select).component('Message',Message)
app.mount('#app')
