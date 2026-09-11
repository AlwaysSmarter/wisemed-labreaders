// Run from readersv3: PLAYWRIGHT_MODULE=/path/to/playwright node tests/ui/order-id-click.cjs
// Uses the full HTML, JavaScript, event bindings and detail renderer with fixture data.
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const fs = require('fs');
(async()=>{
const browser=await chromium.launch({channel:'chrome',headless:true});
const page=await browser.newPage({viewport:{width:1450,height:1050}});const errors=[];page.on('pageerror',e=>errors.push(e.message));
// Exercise browsers without the native dialog API, which previously failed silently.
await page.evaluate(() => { HTMLDialogElement.prototype.showModal = undefined; });
const base=process.cwd()+'/modules/localhttp/ui/';
await page.setContent(fs.readFileSync(base+'index.html','utf8').replace(/<script[^>]*src="\/app.js"[^>]*><\/script>/,''));
await page.addStyleTag({content:fs.readFileSync(base+'styles.css','utf8')});
const code=fs.readFileSync(base+'app.js','utf8').replace('\ninit();','\nwindow.testUI={state,renderOrderDetails,renderOrdersLayout,bindEvents};');
await page.addScriptTag({content:code});
await page.evaluate(()=>{
const ui=window.testUI;ui.bindEvents();
ui.state.orders=[{order:{id:1,sample_id:'POPESCU ION',file_id:'1234',round_no:1,order_date:'2026-09-11',status:'received',meta:{}},analyses:[{analysis:{id:2,analyte_tag:'GLU',result_value:'100',flags:{}},results:[]}]}];
ui.state.selectedOrderId=1;ui.renderOrderDetails();ui.renderOrdersLayout();
document.getElementById('login-view').hidden=true;document.getElementById('dashboard-view').hidden=false;document.getElementById('view-overview').hidden=true;document.getElementById('view-orders').hidden=false;
});
if(await page.locator('[data-edit-order-id]').count()!==3)throw Error('Expected header, explicit action and file ID');
for(let i=0;i<3;i++){
 await page.locator('[data-edit-order-id]').nth(i).click();
 await page.locator('#order-id-change-dialog').waitFor({state:'visible'});
 await page.locator('[data-id-cancel]').click();
}
await page.getByRole('button',{name:'Modifică ID',exact:true}).click();
await page.locator('#order-id-change-dialog').waitFor({state:'visible'});
if (errors.length) throw new Error(errors.join('\n'));
await page.locator('[data-id-cancel]').click();
await page.evaluate(() => window.testUI.renderOrderDetails());
await page.getByRole('button', {name:'Modifică ID', exact:true}).click();
await page.locator('#order-id-change-dialog').waitFor({state:'visible'});
console.log('Full UI: header, explicit action and file ID all open the dialog. Errors:',JSON.stringify(errors));
await browser.close();
})().catch(e=>{console.error(e);process.exit(1)});
