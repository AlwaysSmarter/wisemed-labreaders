import {expect,test} from '@playwright/test'

test('browser history restores sections and an existing user editor',async({page})=>{
  const user={id:'11111111-1111-4111-8111-111111111111',name:'Medic Test',email:'medic@example.invalid',role:'doctor'}
  await page.route('**/v1/**',route=>{
    const path=new URL(route.request().url()).pathname
    const responses:Record<string,unknown>={
      '/v1/auth/providers':{standalone:true},
      '/v1/profile':{name:'Admin Test',role:'admin',instance_admin:true},
      '/v1/billing/account':{},
      '/v1/dashboard':{},
      '/v1/templates':[],
      '/v1/sessions':[],
      '/v1/specialty-packs':[],
      '/v1/organizations':[{name:'Clinica Test'}],
      '/v1/users':[user]
    }
    return route.fulfill({json:responses[path]??[]})
  })
  await page.goto('/ai/')
  await expect(page).toHaveURL(/#\/dashboard$/)
  await page.getByRole('button',{name:'Echipă și acces',exact:true}).click()
  await expect(page).toHaveURL(/#\/users$/)
  await page.getByRole('button',{name:'Editează',exact:true}).click()
  await expect(page).toHaveURL(new RegExp('#/users/'+user.id+'$'))
  await expect(page.getByLabel('Email',{exact:true})).toHaveValue(user.email)

  await page.goBack()
  await expect(page.getByRole('heading',{name:'Adaugă un coleg'})).toBeVisible()
  await page.goBack()
  await expect(page).toHaveURL(/#\/dashboard$/)
  await page.goForward()
  await expect(page.getByRole('heading',{name:'Membrii organizației'})).toBeVisible()
  await page.goForward()
  await expect(page.getByLabel('Email',{exact:true})).toHaveValue(user.email)
  await page.reload()
  await expect(page.getByLabel('Email',{exact:true})).toHaveValue(user.email)
})

test('doctor selects a specialty and confirms a paid copy of a shared table template',async({page})=>{
  const plan={name:'Free',code:'free',version:1,currency:'USD',monthly_price_minor:0,max_specialties:1,max_templates:3,max_fields_per_template:150,included_ai_tokens:50000,extra_specialty_price_minor:0,extra_template_price_minor:0,extra_million_tokens_price_minor:0}
  const template={id:'11111111-1111-4111-8111-111111111111',version_id:'22222222-2222-4222-8222-222222222222',version:1,name:'Consultație',specialty:'Cardiologie',specialties:['Cardiologie','Neurologie'],html:'<table><tbody><tr><td>{{siv}}</td></tr></tbody></table>',fields:[{key:'siv',label:'Sept interventricular',type:'number',unit:'mm',help:'SIV și sept interventricular desemnează același câmp.',aliases:['SIV','sept interventricular']}],sections:[],rules:[],fragments:{},semantic_mapping:{}}
  let selected:string[]=[]
  let extras=false
  const publications:any[]=[]
  await page.route('**/v1/**',route=>{
    const path=new URL(route.request().url()).pathname
    if(path==='/v1/templates'&&route.request().method()==='POST'){
      publications.push(route.request().postDataJSON())
      return route.fulfill({json:{...template,id:'33333333-3333-4333-8333-333333333333',owner_id:'doctor',addon_monthly_minor:750,addon_currency:'USD'}})
    }
    if(path==='/v1/billing/specialties'){
      selected=route.request().postDataJSON().specialties
      return route.fulfill({json:{ok:true}})
    }
    const responses:Record<string,unknown>={
      '/v1/auth/providers':{standalone:true},
      '/v1/profile':{name:'Medic Test',role:'doctor',instance_admin:false},
      '/v1/billing/account':{plan:{...plan,version_id:'44444444-4444-4444-8444-444444444444',extras_enabled:extras,extra_template_price_minor:extras?750:0},selected_specialties:selected,available_specialties:['Cardiologie','Neurologie'],templates_used:3,total_tokens:0,reserved_tokens:0,credit_limit_minor:0},
      '/v1/templates':[template,{...template,id:'55555555-5555-4555-8555-555555555555',name:'Șablon din altă specialitate',specialty:'Radiologie',specialties:['Radiologie']}],
      '/v1/billing/status':{subscription:null},
      '/v1/billing/statistics':{items:[]},
      '/v1/dashboard':{}
    }
    return route.fulfill({json:responses[path]??[]})
  })
  await page.goto('/ai/')
  await expect(page.getByRole('heading',{name:'Free',exact:true})).toBeVisible()
  await expect(page.getByRole('button',{name:'Specialty Packs',exact:true})).toHaveCount(0)
  await page.getByRole('button',{name:'Pachet și specialități'}).click()
  const cardiology=page.getByRole('checkbox',{name:'Cardiologie'})
  const neurology=page.getByRole('checkbox',{name:'Neurologie'})
  await cardiology.check()
  await expect(neurology).toBeDisabled()
  await page.getByRole('button',{name:'Salvează specialitățile'}).click()
  await expect.poll(()=>selected).toEqual(['Cardiologie'])
  await cardiology.uncheck()
  await neurology.check()
  await page.getByRole('button',{name:'Salvează specialitățile'}).click()
  await expect.poll(()=>selected).toEqual(['Neurologie'])
  await page.getByRole('button',{name:'Șabloane',exact:true}).click()
  await expect(page.getByText(/Ai folosit toate cele 3 locuri/)).toBeVisible()
  await expect(page.getByRole('heading',{name:'Șablon din altă specialitate'})).toHaveCount(0)
  await page.getByRole('button',{name:'Personalizează o copie'}).click()
  await expect(page.getByText(/Ai atins limita de șabloane proprii/)).toBeVisible()
  const publish=page.getByRole('button',{name:'Adaugă la șabloanele mele'})
  await expect(publish).toBeDisabled()
  await expect(page.locator('.tiptap table')).toHaveCount(1)
  await page.getByRole('button',{name:'Sept interventricular',exact:false}).click()
  await expect(page.getByText('SIV și sept interventricular desemnează același câmp.')).toBeVisible()

  extras=true
  await page.reload()
  await expect(publish).toBeEnabled()
  page.once('dialog',dialog=>dialog.dismiss())
  await publish.click()
  expect(publications).toHaveLength(0)
  page.once('dialog',dialog=>dialog.accept())
  await publish.click()
  await expect.poll(()=>publications.length).toBe(1)
  expect(publications[0].specialties).toEqual(['Neurologie'])
  expect(publications[0].addon_confirmation.monthly_minor).toBe(750)
  expect(publications[0].html).toContain('<table')

})
