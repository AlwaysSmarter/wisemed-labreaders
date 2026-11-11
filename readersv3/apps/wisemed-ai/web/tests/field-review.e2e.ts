import {test,expect} from '@playwright/test'

test('field review preserves other values and transmits new text automatically',async({page})=>{
 const id='11111111-1111-4111-8111-111111111111'
 const template={id:'22222222-2222-4222-8222-222222222222',version_id:'33333333-3333-4333-8333-333333333333',version:1,name:'Raport sintetic',specialty:'Test',fields:[{key:'aspect',label:'Aspect',type:'string'},{key:'masura',label:'Măsură',type:'number',unit:'cm'},{key:'observatii',label:'Observații',type:'string'}],sections:[],rules:[],fragments:{},semantic_mapping:{}}
 const result:any={revision:1,structured_data:{aspect:{value:'omogen',state:'present'},masura:{value:'02',state:'needs_review'}},rendered_html:'<p>Aspect: <span data-review-field="aspect">omogen</span></p><p>Măsură: <span data-review-field="masura">02</span></p>',field_issues:[{field:'masura',code:'ambiguous_number',message:'Confirmați virgula.',raw_value:'02',quote:'Măsură 02 cm',suggestions:['0.2','2']}],evidence:[],missing_fields:[],validation_errors:['ambiguous_number:masura'],warnings:[]}
 const session:any={id,template_version:template.version_id,template,status:'recording',revision:1,chunks:[],context:{},result}
 const edits:any[]=[],sent:any[]=[]
 await page.route('**/v1/**',async route=>{
  const path=new URL(route.request().url()).pathname
  if(path.endsWith('/events'))return route.fulfill({contentType:'text/event-stream',body:''})
  if(path.endsWith('/field-edit')){
   const body=route.request().postDataJSON();edits.push(body);session.revision++;result.revision=session.revision;result.structured_data.masura={value:body.value,state:'present',user_edited:true};result.field_issues=[];result.validation_errors=[]
   return route.fulfill({json:session})
  }
  if(path.endsWith('/transcript-chunks')){sent.push(route.request().postDataJSON());return route.fulfill({json:{accepted:true,result}})}
  const responses:Record<string,unknown>={'/v1/auth/providers':{standalone:true},'/v1/profile':{name:'Medic sintetic',role:'admin',instance_admin:true},'/v1/billing/account':{},'/v1/dashboard':{},'/v1/templates':[template],'/v1/sessions':[session],'/v1/specialty-packs':[],['/v1/sessions/'+id]:session}
  return route.fulfill({json:responses[path]??[]})
 })
 await page.goto('/ai/#/workspace/'+id)
 await expect(page.locator('.report')).toContainText('omogen')
 await expect(page.getByRole('button',{name:'Procesează segmentul',exact:true})).toHaveCount(0)
 await expect(page.getByText('Segmente primite',{exact:true})).toHaveCount(0)
 const field=page.locator('.field-grid button').filter({hasText:'Măsură'})
 await expect(field).toHaveClass('invalid')
 await expect(page.locator('.field-grid button').filter({hasText:'Observații'})).toContainText('—')
 await field.click()
 const dialog=page.getByRole('dialog')
 await expect(dialog).toContainText('Măsură 02 cm')
 await dialog.getByRole('button',{name:'0.2',exact:true}).click()
 await dialog.getByRole('button',{name:'Confirmă valoarea',exact:true}).click()
 await expect(dialog).toHaveCount(0)
 expect(edits).toEqual([{revision:1,field:'masura',value:'0.2'}])
 await expect(field).not.toHaveClass('invalid')
 await expect(page.locator('.field-grid button').filter({hasText:'Aspect'})).toContainText('omogen')
 await page.getByLabel('Text transcris',{exact:true}).fill('Observații dictate ulterior')
 await expect.poll(()=>sent.length).toBe(1)
 expect(sent[0].text).toBe('Observații dictate ulterior')
 await expect(page.getByLabel('Text transcris',{exact:true})).toHaveValue('')
 await page.setViewportSize({width:390,height:844})
 await page.screenshot({path:'test-results/field-review-mobile.png',fullPage:true})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
})
