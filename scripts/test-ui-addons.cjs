/* Isolated PostgreSQL UI journey; synthetic fixtures only. */
const {chromium}=require('../tools/qa/node_modules/playwright');
const fs=require('fs'),assert=require('assert');
(async()=>{
 const data=process.env.OPENFOOD_UI_TEST_DATA;assert(data,'isolated test installation required');
 const url=JSON.parse(fs.readFileSync(data+'/runtime.json')).URL,out='docs/evidence/addons-stage';fs.mkdirSync(out,{recursive:true});
 const browser=await chromium.launch({headless:true}),p=await browser.newPage({viewport:{width:1440,height:1000},locale:'pt-BR'}),errors=[];
 p.on('pageerror',e=>errors.push(e.message));p.setDefaultTimeout(15000);
 const read=path=>p.evaluate(path=>api(path),path),write=(path,input)=>p.evaluate(([path,input])=>api(path,input),[path,input]);
 const shot=async name=>{
   if(name.startsWith('shared-addons-') && !name.includes('receipt'))await p.evaluate(()=>{
     document.activeElement?.blur();const title=document.querySelector('#addonEditorTitle');title.tabIndex=-1;title.focus({preventScroll:true});
     document.querySelector('#addonEditorDialog .dialog-body').scrollTop=0;
   });
   return p.screenshot({path:out+'/'+name+'.png',animations:'disabled'});
 };
 await p.goto(url);await p.locator('#loginForm [name=email]').fill('qa@example.org');await p.locator('#loginForm [name=password]').fill('QA-test-only-password');await p.locator('#loginForm button').click();await p.locator('#enterAdmin').click();
 const a=await write('products',{name:'Hambúrguer da casa · QA',price_cents:2000,stock_quantity:20,unit:'unidade',active:true}),b=await write('products',{name:'Porção de batata · QA',price_cents:1500,stock_quantity:20,unit:'unidade',active:true});
 a.name='Hambúrguer da casa · QA';b.name='Porção de batata · QA';
 await p.evaluate(()=>refresh());await p.locator('#tabCatalogo').click();await p.locator('[data-subtab=adicionais]').click();
 await p.evaluate(()=>catalogPrimary());await p.locator('#addonAdminForm [name=group_name]').fill('Molhos da casa · QA #'+a.id);await p.locator('#addonAdminForm [name=group_description]').fill('Um único estoque para hambúrgueres e porções');
 await p.getByRole('button',{name:'Adicionar opção',exact:true}).click();await p.locator('#addonAdminForm [name=option_name]').fill('Molho especial');await p.locator('#addonAdminForm [name=option_price]').fill('2,51');await p.locator('#addonAdminForm [name=option_stock]').fill('10');
 for(const id of [a.id,b.id])await p.locator(`#addonAdminForm [name=addon_product][value="${id}"]`).check();
 assert((await p.locator('#addonImpact').innerText()).includes(a.name));assert((await p.locator('#addonImpact').innerText()).includes(b.name));
 await p.getByRole('button',{name:'Salvar grupo e vínculos',exact:true}).click();assert(await p.locator('#addonEditorDialog').isVisible(),'impact confirmation must be required');
 await p.locator('#addonAdminForm [name=addon_confirm]').check();await shot('shared-addons-desktop');await p.evaluate(()=>setTheme('dark'));await shot('shared-addons-dark');await p.evaluate(()=>setTheme('light'));await p.setViewportSize({width:390,height:844});await shot('shared-addons-mobile');assert(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
 await p.setViewportSize({width:1440,height:1000});await p.getByRole('button',{name:'Salvar grupo e vínculos',exact:true}).click();await p.locator('#addonEditorDialog').waitFor({state:'hidden'});
 let menu=await read('product-configuration?product_id='+a.id),other=await read('product-configuration?product_id='+b.id);const group=menu.groups[0];assert(group.shared);assert.equal(group.options[0].id,other.groups[0].options[0].id);
 await p.locator('[data-subtab=produtos]').click();await p.locator('#advProducts tr').filter({hasText:a.name}).last().getByRole('button',{name:'Tamanhos, adicionais e foto'}).click();
 await p.getByRole('button',{name:'Adicionar tamanho',exact:true}).click();await p.locator('[name=variant_name]').last().fill('Rascunho preservado');
 await p.getByRole('button',{name:'Editar grupo compartilhado: '+group.name,exact:true}).click();await p.getByRole('button',{name:'Fechar adicionais',exact:true}).click();assert.equal(await p.locator('[name=variant_name]').last().inputValue(),'Rascunho preservado');
 // Close parent editor through its known dialog, without saving the temporary variant.
 await p.evaluate(()=>document.querySelector('#menuEditorDialog').close());
 await p.locator('#adminToPOS').click();await p.waitForFunction(()=>!posBusy);
 if(await p.locator('#posCashGate').isVisible()){await p.locator('#posOpenForm [name=opening]').fill('0,00');await p.locator('#posOpenSubmit').click()}
 for(const id of [a.id,b.id]){await p.locator(`#posGrid button[data-product-id="${id}"]`).click();await p.locator('#menuChoiceForm [name=addon]').check();await p.getByRole('button',{name:'Adicionar ao pedido',exact:true}).click()}
 assert.equal(await p.evaluate(()=>posCart.reduce((n,x)=>n+x.price_cents*x.quantity,0)),4002);
 await p.locator('#posPay').click();await p.locator('#posPaymentRows select').selectOption('card');await p.locator('#posPaymentForm [name=confirmed]').check();await p.locator('#posConfirmSale').click();await p.locator('.pos-receipt').waitFor();await shot('shared-addons-receipt');
 assert((await p.locator('.pos-receipt').innerText()).includes('Molho especial'));const order=(await read('orders')).find(x=>x.total_cents===4002);assert(order);
 let c=await read('addon-configuration?group_id='+group.id);assert.equal(c.group.options[0].stock_quantity,8);
 await p.getByRole('button',{name:'Nova venda',exact:true}).click();await p.locator('#posToAdmin').click();
 await p.evaluate(id=>sharedAddonEditor(id),group.id);await p.locator('#addonAdminForm [name=option_name]').fill('Molho premium');await p.locator('#addonAdminForm [name=option_price]').fill('3,99');await p.locator('#addonAdminForm [name=addon_confirm]').check();
 await p.getByRole('button',{name:'Salvar grupo e vínculos',exact:true}).click();await p.locator('#addonEditorDialog').waitFor({state:'hidden'});
 const details=await read('orders?id='+order.id);assert(JSON.stringify(details).includes('Molho especial'));assert.equal((await read('product-configuration?product_id='+b.id)).groups[0].options[0].price_cents,399);
 await p.evaluate(id=>sharedAddonEditor(id),group.id);await p.locator(`#addonAdminForm [name=addon_product][value="${a.id}"]`).uncheck();await p.locator('#addonAdminForm [name=addon_confirm]').check();await p.getByRole('button',{name:'Salvar grupo e vínculos',exact:true}).click();await p.locator('#addonEditorDialog').waitFor({state:'hidden'});
 assert.equal((await read('product-configuration?product_id='+a.id)).groups.length,0);assert.equal((await read('product-configuration?product_id='+b.id)).groups.length,1);
 assert.equal(errors.length,0,errors.join('\n'));const report={result:'PASS',data:'synthetic isolated QA only',checks:['shared editor and impact confirmation','desktop/mobile without horizontal overflow','same option IDs across products','unsaved parent variant preserved','two-product POS checkout and shared stock','price propagation and original receipt snapshot','unlink preserves other product'],javascript_errors:errors};fs.writeFileSync(out+'/results.json',JSON.stringify(report,null,2));console.log(JSON.stringify(report));await browser.close();
})().catch(e=>{console.error(e);process.exit(1)});
