import {defineConfig} from 'vite'
import vue from '@vitejs/plugin-vue'
export default defineConfig({plugins:[vue()],base:'/ai/',build:{outDir:'../../../modules/wisemedai/web/dist',emptyOutDir:true},server:{proxy:{'/v1':'http://127.0.0.1:19120'}}})
