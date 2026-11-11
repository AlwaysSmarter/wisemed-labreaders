import {test,expect} from '@playwright/test'

test('home opens WiseMED AI instead of the analyzer console',async({page})=>{
  await page.goto('/')
  await expect(page).toHaveURL(/\/ai\/$/)
  await expect(page.getByRole('heading',{name:'Autentificare',exact:true})).toBeVisible()
  await expect(page.locator('#dashboard-view')).toHaveCount(0)
})

test('shared utility settings exclude laboratory UI and API calls after login',async({page})=>{
  const calls:string[]=[],errors:string[]=[]
  const reader={id:'wisemed-ai',label:'WiseMED AI',protocol:'wisemed-ai',comm_type:'utility',app_version:'1.3.1'}
  page.on('pageerror',error=>errors.push(error.message))
  // Synthetic WiseMED administrative session: no external login or runtime configuration writes.
  await page.route('**/api/**',async route=>{
    const path=new URL(route.request().url()).pathname
    calls.push(path)
    const data:Record<string,unknown>={
      '/api/preferences':{preferences:{language:'ro'}},
      '/api/preferences/language':{preferences:{language:'en'}},
      '/api/session':{authenticated:true,session:{username:'synthetic-admin',user_type:0},reader,permissions:{can_view_logs:true,can_debug_tests:true}},
      '/api/status':{reader,communication:{type:'utility'},connections:{},stats:{}},
      '/api/logs':{logs:[]},
      '/api/reader-settings':{settings:{reader_id:'wisemed-ai',reader_label:'WiseMED AI',analyzer_protocol:'wisemed-ai',analyzer_comm_type:'utility',app_updates_enabled:'true',app_updates_app_id:'wisemed-ai',app_updates_current_version:'1.3.1'}},
      '/api/app-update/status':{enabled:true,app_id:'wisemed-ai',current_version:'1.3.1'},
      '/api/wss/status':{enabled:false,permissions:[]}
    }
    await route.fulfill({status:path in data?200:404,contentType:'application/json',body:JSON.stringify({ok:path in data,...(data[path] as object||{})})})
  })
  await page.goto('/settings/reader')
  await expect(page.locator('#dashboard-view')).toBeVisible()
  await expect(page.locator('#reader-eyebrow')).toHaveText('UTILITAR')
  await expect(page.locator('#reader-identity-title')).toHaveText('Utilitar')
  await expect(page.locator('#wisemed-ai-dashboard-nav')).toBeVisible()
  await expect(page.locator('#dashboard-title')).toContainText('Server și actualizări')
  for(const view of ['overview','orders','qc','daily-details'])await expect(page.locator(`.nav-link[data-view="${view}"]`)).toBeHidden()
  for(const view of ['analytes','qc','daily-details','daily-analysis-filters'])await expect(page.locator(`[data-settings-subview="${view}"]`)).toBeHidden()
  await expect(page.locator('[data-settings-subview="reader"]')).toHaveText('Server și actualizări')
  await expect(page.locator('[name="app_updates_enabled"]')).toBeVisible()
  await expect(page.locator('[name="analyzer_name"]')).toBeHidden()
  await expect(page.locator('#analyzer-pill')).toBeHidden()
  await page.locator('#language-select-dashboard').selectOption('en')
  await expect(page.locator('[data-settings-subview="reader"]')).toHaveText('Server și actualizări')
  await expect(page.locator('.nav-link[data-view="qc"]')).toBeHidden()
  await page.locator('[data-settings-subview="wss"]').click()
  await expect(page.locator('#settings-panel-wss')).toBeVisible()
  await expect(page.locator('#dashboard-title')).toContainText('WSM / WSS')
  await page.screenshot({path:'test-results/wisemed-ai-utility-settings.png',fullPage:true})
  expect(errors).toEqual([])
  expect(calls.filter(path=>/\/(analytes|orders|qc|daily-details|dashboard|result-sync|debug\/tests)/.test(path))).toEqual([])
})
