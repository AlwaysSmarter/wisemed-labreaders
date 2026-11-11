import {expect,test} from '@playwright/test'

test('admin browses another user sessions, filters and reads a sandboxed result',async({page})=>{
 const admin='11111111-1111-4111-8111-111111111111',doctor='22222222-2222-4222-8222-222222222222',id='33333333-3333-4333-8333-333333333333'
 const owner={user_id:doctor,name:'Medic Sintetic',email:'medic@example.invalid',organization:'Cabinet Sintetic'}
 const summary={id,template_name:'Ecografie sintetică',status:'review',revision:2,created_at:'2026-11-05T12:00:00Z',has_result:true,owner}
 const requests:URL[]=[]
 await page.route('**/v1/**',route=>{
  const url=new URL(route.request().url()),path=url.pathname
  if(path==='/v1/admin/sessions'){
   requests.push(url)
   return route.fulfill({json:{items:[summary],users:[{id:doctor,name:owner.name,email:owner.email}],total:51,limit:50,offset:Number(url.searchParams.get('offset')),instance_scope:true,read_only:true}})
  }
  if(path==='/v1/admin/sessions/'+id)return route.fulfill({json:{owner,read_only:true,session:{id,revision:2,status:'review',template:{name:summary.template_name},context:{patient_ref:'SYNTHETIC'},chunks:[{id:'chunk',sequence:1,text:'dictare sintetică'}],result:{rendered_html:'<p>Rezultat sintetic verificat</p><script>parent.postMessage("unsafe","*")</script>',structured_data:{aspect:'omogen'},evidence:[]}}}})
  const responses:Record<string,unknown>={'/v1/auth/providers':{standalone:true},'/v1/profile':{user_id:admin,name:'Admin Sintetic',role:'admin',instance_admin:true},'/v1/billing/account':{},'/v1/dashboard':{},'/v1/templates':[],'/v1/sessions':[],'/v1/specialty-packs':[]}
  return route.fulfill({json:responses[path]??[]})
 })
 await page.goto('/ai/#/sessions')
 await expect(page.getByRole('heading',{name:'Sesiunile tuturor utilizatorilor'})).toBeVisible()
 await expect(page.getByRole('cell',{name:/Medic Sintetic/})).toBeVisible()
 await page.getByRole('combobox').first().click()
 await page.getByRole('option',{name:/Medic Sintetic/}).click()
 await expect.poll(()=>requests.at(-1)?.searchParams.get('user_id')).toBe(doctor)
 await page.getByRole('button',{name:'Înainte',exact:true}).click()
 await expect.poll(()=>requests.at(-1)?.searchParams.get('offset')).toBe('50')
 await page.getByRole('button',{name:'Vezi sesiunea',exact:true}).click()
 await expect(page.getByRole('heading',{name:/Ecografie sintetică/})).toBeVisible()
 await expect(page.frameLocator('iframe').getByText('Rezultat sintetic verificat')).toBeVisible()
 await expect(page.locator('iframe')).toHaveAttribute('sandbox','')
 await page.getByText('Transcriere · 1 segmente',{exact:true}).click()
 await expect(page.getByText('1. dictare sintetică',{exact:true})).toBeVisible()
 await expect(page.getByRole('button',{name:'Deschide sesiunea mea pentru lucru'})).toHaveCount(0)
 await expect(page.getByRole('button',{name:'Șterge sesiunea'})).toHaveCount(0)
 await page.setViewportSize({width:390,height:844})
 await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 await page.screenshot({path:'test-results/admin-sessions-mobile.png',fullPage:true})
})
