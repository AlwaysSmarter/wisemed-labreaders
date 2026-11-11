import {defineConfig} from '@playwright/test'
export default defineConfig({testDir:'tests',testMatch:'*.e2e.ts',workers:1,timeout:60000,use:{baseURL:process.env.WAI_BASE_URL||'http://127.0.0.1:19120',browserName:'chromium',headless:true,trace:'off',screenshot:'only-on-failure'}})
