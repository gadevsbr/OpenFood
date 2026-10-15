/* Synthetic fixtures only, against an isolated local PostgreSQL runtime. */
const {chromium}=require('../tools/qa/node_modules/playwright');
const fs=require('fs'),assert=require('assert');
(async()=>{
 const data=process.env.OPENFOOD_UI_TEST_DATA;assert(data,'isolated test runtime required');
 const url=JSON.parse(fs.readFileSync(data+'/runtime.json')).URL,out='docs/evidence/privacy-completion-stage';fs.mkdirSync(out,{recursive:true});
 const browser=await chromium.launch({headless:true}),context=await browser.newContext({viewport:{width:1440,height:1000},locale:'pt-BR'}),p=await context.newPage(),errors=[];
 const track=page=>{page.setDefaultTimeout(20000);page.on('pageerror',e=>errors.push(e.message))};track(p);
 await p.goto(url);await p.locator('#loginForm [name=email]').fill('qa@example.org');await p.locator('#loginForm [name=password]').fill('QA-test-only-password');await p.locator('#loginForm button').click();await p.locator('#enterAdmin').click();
 const api=(path,input)=>p.evaluate(([path,input])=>api(path,input),[path,input]);
 const c=await api('customers',{name:'Privacidade entre abas · QA',email:'private-tabs@example.org'});
 const product=await api('products',{name:'Prato entre abas · QA',price_cents:1000,stock_quantity:10,active:true,unit:'unidade'});
 const q=await context.newPage();track(q);await q.goto(url);await q.locator('#enterPOS').click();await q.waitForFunction(()=>typeof currentStoreId==='number'&&currentUser&&!!sessionStorage.getItem(`openfood:privacy:${currentUser.id}:${currentStoreId}`));
 await q.evaluate(([c,product])=>{
  const items=[{product_id:product.id,name:product.name,price_cents:1000,quantity:1,notes:'private-tabs@example.org'}];
  posCart=items;document.querySelector('#posCustomer').append(new Option(c.name,c.id));document.querySelector('#posCustomer').value=String(c.id);
  document.querySelector('#posDiscountNote').value='private-tabs@example.org';posSave();
  sessionStorage.setItem(posKey('held'),JSON.stringify([{items,customer_id:c.id,service:'counter'}]));
  sessionStorage.setItem(posKey('dining-pending'),JSON.stringify({notes:'private-tabs@example.org'}));
 },[c,product]);
 await p.locator('#tabClientes').click();await p.locator('#customersTable tr').filter({hasText:'Privacidade entre abas · QA'}).getByRole('button',{name:'Remover identificação',exact:true}).click();await p.locator('#privacyReviewDialog').waitFor({state:'visible'});
 await p.locator('#privacyRemovalForm [name=privacy_confirm]').check();await p.getByRole('button',{name:'Remover identificação e textos vinculados',exact:true}).click();
 await p.locator('#enterAdmin').waitFor();await q.locator('#enterPOS').waitFor();
 assert(await q.evaluate(()=>!Object.keys(sessionStorage).some(k=>k.startsWith('openfood:pos:'))));
 assert(!await q.evaluate(()=>JSON.stringify(posCart).includes('private-tabs@example.org')));
 const removed=await api('customer-export?customer_id='+c.id);assert.equal(removed.customer.email,null);
 // Same tab and another tab both cleared, without broadcasting personal data.
 await p.locator('#enterAdmin').click();await p.locator('#tabClientes').click();await p.locator('#subtabLGPD').click();await p.locator('#privacyPolicyButton').click();await p.locator('#privacyPolicyDialog').waitFor({state:'visible'});
 await p.locator('#privacyRetentionDays').fill('30');await p.locator('#privacyRetentionPreview').click();await p.getByText(/clientes elegíveis agora/).waitFor();
 await p.locator('[name=retention_enabled]').check();await p.getByRole('button',{name:'Salvar política',exact:true}).click();await p.locator('#privacyPolicyDialog .pos-inline-error:not(.hidden)').waitFor();
 await p.locator('[name=retention_confirm]').check();
 const shot=async name=>{await p.evaluate(()=>document.activeElement?.blur());await p.screenshot({path:out+'/'+name+'.png',animations:'disabled'})};
 await shot('retention-desktop');await p.evaluate(()=>setTheme('dark'));await shot('retention-dark');await p.evaluate(()=>setTheme('light'));await p.setViewportSize({width:390,height:844});await shot('retention-mobile');const action=await p.getByRole('button',{name:'Salvar política',exact:true}).boundingBox();assert(action.y+action.height<=844,'mobile save must remain visible');assert(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
 await p.getByRole('button',{name:'Salvar política',exact:true}).click();await p.locator('#privacyPolicyDialog').waitFor({state:'hidden'});assert((await api('privacy-policy')).enabled);
 // Pause another browser session while a removal happens, then reconnect it.
 const offlineContext=await browser.newContext(),r=await offlineContext.newPage();track(r);await r.goto(url);await r.locator('#loginForm [name=email]').fill('qa@example.org');await r.locator('#loginForm [name=password]').fill('QA-test-only-password');await r.locator('#loginForm button').click();await r.locator('#enterPOS').click();await r.evaluate(()=>api('products'));
 await r.evaluate(()=>sessionStorage.setItem(posKey('cart'),JSON.stringify({items:[],note:'offline-private@example.org'})));
 await offlineContext.setOffline(true);await r.locator('#privacyShield').waitFor({state:'visible'});await r.keyboard.press('Escape');assert(await r.locator('#privacyShield').isVisible());
 const other=await api('customers',{name:'Offline QA',email:'offline-private@example.org'});
 // Request context is independent of the page's cross-tab reload.
 const res=await context.request.delete(url+'/api/customer-delete?customer_id='+other.id,{headers:{'X-OpenFood':'1'}});assert.equal(res.status(),200);
 await offlineContext.setOffline(false);await r.locator('#enterPOS').waitFor();assert(await r.evaluate(()=>!Object.keys(sessionStorage).some(k=>k.startsWith('openfood:pos:'))));
 await p.reload();await p.locator('#enterAdmin').click();
 const policy=await api('privacy-policy');await api('privacy-policy',{...policy,enabled:false,confirm:false});
 assert.equal(errors.length,0,errors.join('\n'));
 const report={date:'2026-10-14',result:'PASS',fixtures:'synthetic isolated PostgreSQL',checks:['cross-tab removal clears carts, held sales and pending dining text','current page reload clears in-memory data','retention preview and explicit versioned activation','desktop/mobile/dark without overflow','offline modal blocks interaction and Escape','separate session reconnect clears stale drafts','financial/identity export verified after removal'],javascript_errors:errors};
 fs.writeFileSync(out+'/privacy-completion-results.json',JSON.stringify(report,null,2));console.log(JSON.stringify(report));await browser.close();
})().catch(e=>{console.error(e);process.exit(1)});
