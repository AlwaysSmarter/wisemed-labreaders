import {test,expect} from '@playwright/test'
import {readFileSync} from 'node:fs'

test('WiseMED usernames without @ reach the API with explicit provider and organization',async({page})=>{
  const sent:unknown[]=[]
  await page.route('**/v1/auth/providers',route=>route.fulfill({json:{standalone:true,wisemed:true}}))
  await page.route('**/v1/profile',route=>route.fulfill({status:401,json:{error:'authentication_required'}}))
  await page.route('**/v1/auth/login',async route=>{
    sent.push(route.request().postDataJSON())
    await route.fulfill({status:401,json:{error:'invalid_credentials'}})
  })
  await page.goto('/ai/')
  await expect(page.getByLabel('Email',{exact:true})).toHaveAttribute('type','email')
  await page.getByRole('combobox',{name:'Tip de autentificare',exact:true}).selectOption('true')
  const username=page.getByLabel('Utilizator WiseMED',{exact:true})
  await expect(username).toHaveAttribute('type','text')
  await username.fill('medic.test')
  await page.getByLabel('Parolă',{exact:true}).fill('synthetic-test-only')
  await page.getByLabel('Cod organizație AI',{exact:true}).fill('11111111-1111-4111-8111-111111111111')
  await page.getByRole('button',{name:'Autentificare',exact:true}).click()
  await expect.poll(()=>sent.length).toBe(1)
  expect(sent[0]).toEqual({email:'medic.test',password:'synthetic-test-only',wisemed:true,tenant_id:'11111111-1111-4111-8111-111111111111'})
  await expect(page.getByText('invalid_credentials',{exact:true})).toBeVisible()
})

test('disabled WiseMED provider is explained instead of silently requiring an email',async({page})=>{
  let submissions=0
  await page.route('**/v1/auth/providers',route=>route.fulfill({json:{standalone:true,wisemed:false}}))
  await page.route('**/v1/profile',route=>route.fulfill({status:401,json:{error:'authentication_required'}}))
  await page.route('**/v1/auth/login',async route=>{submissions++;await route.fulfill({status:503,json:{error:'wisemed_login_disabled'}})})
  await page.goto('/ai/')
  await page.getByRole('combobox',{name:'Tip de autentificare',exact:true}).selectOption('true')
  await expect(page.getByLabel('Utilizator WiseMED',{exact:true})).toHaveAttribute('type','text')
  await expect(page.getByText(/Autentificarea WiseMED nu este activată pe acest server/)).toBeVisible()
  await expect(page.getByRole('button',{name:'Autentificare',exact:true})).toBeDisabled()
  expect(submissions).toBe(0)
  await page.getByRole('combobox',{name:'Tip de autentificare',exact:true}).selectOption('false')
  await expect(page.getByLabel('Email',{exact:true})).toHaveAttribute('type','email')
  await expect(page.getByRole('button',{name:'Autentificare',exact:true})).toBeEnabled()
})

test('configured bootstrap administrator can sign in and sign out',async({page})=>{
  test.skip(!process.env.WAI_PASSWORD_FILE,'Requires the protected password file in an authorized local environment')
  const password=readFileSync(process.env.WAI_PASSWORD_FILE!,'utf8').trim()
  await page.goto('/ai/')
  await page.getByLabel('Email',{exact:true}).fill('admin@wisemed.local')
  await page.getByLabel('Parolă',{exact:true}).fill(password)
  await page.getByRole('button',{name:'Autentificare',exact:true}).click()
  try {
    await expect(page.getByRole('heading',{name:'Privire de ansamblu',exact:true})).toBeVisible()
    await expect(page.getByText('Super admin · organizație privată',{exact:true})).toBeVisible()
  } finally {
    const passwordField=page.locator('input[type="password"]')
    if(await passwordField.count()===1)await passwordField.fill('')
    // Revoke the browser session even when a visual assertion fails.
    await page.request.post('/v1/auth/logout',{data:{}})
  }
})
