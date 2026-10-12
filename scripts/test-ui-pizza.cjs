/* PostgreSQL-backed journey. Only synthetic fixtures in an isolated test installation. */
const { chromium } = require("../tools/qa/node_modules/playwright");
const fs=require("fs"),assert=require("assert");
(async()=>{
  const data=process.env.OPENFOOD_UI_TEST_DATA;
  assert(data,"OPENFOOD_UI_TEST_DATA must identify an isolated QA installation");
  const url=JSON.parse(fs.readFileSync(data+"/runtime.json")).URL,out="docs/evidence/pizza-stage";
  fs.mkdirSync(out,{recursive:true});
  const browser=await chromium.launch({headless:true}),p=await browser.newPage({viewport:{width:1440,height:1000},locale:"pt-BR"});
  p.setDefaultTimeout(15000);const errors=[];p.on("pageerror",e=>errors.push(e.message));
  const read=path=>p.evaluate(path=>api(path),path),write=(path,input)=>p.evaluate(([path,input])=>api(path,input),[path,input]);
  const shot=async name=>{
    if(name.startsWith("pizza-choices"))await p.evaluate(()=>{document.activeElement?.blur();document.querySelector('.menu-choice-content').scrollTop=0});
    return p.screenshot({path:`${out}/${name}.png`,animations:"disabled"});
  };
  await p.goto(url);await p.locator('#loginForm [name=email]').fill("qa@example.org");await p.locator('#loginForm [name=password]').fill("QA-test-only-password");await p.locator('#loginForm button').click();await p.locator('#enterAdmin').click();
  const created=await write("products",{name:"Pizza da casa · QA",price_cents:3001,stock_quantity:20,unit:"unidade",active:true});
  const id=created.id;
  await write("product-configuration?product_id="+id,{product_id:id,version:1,variants:[{id:0,name:"Grande",price_adjustment_cents:500,stock_quantity:20,active:true}],groups:[{id:0,name:"Borda da pizza · QA #"+id,min_selections:0,max_selections:1,required:false,active:true,options:[{id:0,name:"Borda de queijo",price_cents:300,stock_quantity:20,active:true}]}]});
  await p.evaluate(()=>refresh());await p.locator('#tabCatalogo').click();
  await p.locator('#advProducts tr').filter({hasText:"Pizza da casa · QA"}).last().getByRole("button",{name:"Tamanhos, adicionais e foto"}).click();
  await p.getByRole("button",{name:"Configurar sabores de pizza",exact:true}).click();
  await p.locator('#pizzaAdminForm [name=pizza_enabled]').check();await p.locator('#pizzaAdminForm [name=pizza_policy]').selectOption("weighted_average");
  for(const [name,price] of [["Calabresa","30,01"],["Quatro queijos","40,02"],["Sabores sazonais","50,00"]]){
    await p.getByRole("button",{name:"Adicionar sabor",exact:true}).click();const row=p.locator('.pizza-admin-flavors .menu-admin-row').last();
    await row.locator('[name=flavor_name]').fill(name);await row.locator('[name=flavor_price]').fill(price);
  }
  await p.locator('#pizzaAdminForm [name=flavor_active]').last().uncheck();
  await shot("pizza-admin-desktop");await p.setViewportSize({width:390,height:844});await shot("pizza-admin-mobile");
  assert(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));await p.setViewportSize({width:1440,height:1000});
  await p.getByRole("button",{name:"Salvar sabores",exact:true}).click();await p.locator('#pizzaEditorDialog').waitFor({state:"hidden"});
  const config=await read("product-configuration?product_id="+id);assert.equal(config.pizza.flavors.length,3);assert(config.pizza.flavors[0].id);assert.equal(config.version,3);
  await p.locator('#adminToPOS').click();await p.waitForFunction(()=>!posBusy);
  if(await p.locator('#posCashGate').isVisible()){await p.locator('#posOpenForm [name=opening]').fill("0,00");await p.locator('#posOpenSubmit').click()}
  await p.locator(`#posGrid button[data-product-id="${id}"]`).click();await p.locator('#menuChoiceForm [name=variant]').selectOption(String(config.variants[0].id));
  assert.equal(await p.locator('#menuChoiceForm [name=pizza_flavor]').count(),2);
  for(const flavor of await p.locator('#menuChoiceForm [name=pizza_flavor]').all())await flavor.check();
  await p.locator('#menuChoiceForm [name=addon]').check();await p.locator('#menuChoiceForm [name=notes]').fill("Bem assada, sem cebola · QA");
  assert((await p.locator('.menu-choice-total').innerText()).includes("43,02"));await shot("pizza-choices-desktop");
  await p.locator('#posTheme').evaluate(el=>{el.value="dark";el.dispatchEvent(new Event("change",{bubbles:true}))});await shot("pizza-choices-dark");
  await p.locator('#posTheme').evaluate(el=>{el.value="light";el.dispatchEvent(new Event("change",{bubbles:true}))});await p.setViewportSize({width:390,height:844});await shot("pizza-choices-mobile");
  assert(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
  await p.locator('#menuChoiceForm [name=pizza_parts]').first().fill("2");assert((await p.locator('.menu-choice-total').innerText()).includes("41,35"));
  await p.getByRole("button",{name:"Adicionar ao pedido",exact:true}).click();await p.reload();await p.locator('#enterPOS').click();await p.waitForFunction(()=>!posBusy&&posCart.length===1);
  assert.equal(await p.evaluate(()=>posCart[0].flavors[0].parts+posCart[0].flavors[1].parts),3);
  await p.locator('#posMobileCart').click();await shot("pizza-cart-mobile");
  await p.getByRole("button",{name:"Personalizar",exact:true}).click();assert.equal(await p.locator('#menuChoiceForm [name=pizza_parts]').first().inputValue(),"2");
  await p.getByRole("button",{name:"Salvar item",exact:true}).click();
  await p.locator('#posPay').click();await p.locator('#posPaymentRows select').selectOption("card");await p.locator('#posPaymentForm [name=confirmed]').check();await p.locator('#posConfirmSale').click();await p.locator('.pos-receipt').waitFor();
  const receipt=await p.locator('.pos-receipt').innerText();assert(receipt.includes("2/3 Calabresa"));assert(receipt.includes("1/3 Quatro queijos"));assert(receipt.includes("41,35"));assert(receipt.includes("Bem assada"));await shot("pizza-receipt-mobile");
  const after=(await read("products")).find(x=>x.id===id);assert.equal(after.stock_quantity,19);
  const orders=await read("orders"),order=orders.find(o=>o.total_cents===4135);assert(order);
  const details=await read("orders?id="+order.id);assert.equal(details[0].selection.pizza.pricing_policy,"weighted_average");assert.equal(details[0].selection.pizza.total_parts,3);
  await p.getByRole("button",{name:"Nova venda",exact:true}).click();await p.setViewportSize({width:1440,height:1000});
  const combo=await write("combos",{name:"Combo pizza · QA #"+id,description:"Pizza configurável no combo",price_cents:0,pricing_policy:"sum",active:true});
  await write("combo-items?combo_id="+combo.id,{product_id:id,quantity:1,is_optional:false,group_name:"",min_selections:0,max_selections:1,display_order:0});
  await p.evaluate(()=>refreshPOS());await p.locator(`#posGrid button[data-combo-id="${combo.id}"]`).click();
  await p.locator('#comboChoiceForm select').first().selectOption(String(config.variants[0].id));
  for(const flavor of await p.locator('#comboChoiceForm [name=pizza_flavor]').all())await flavor.check();
  await p.locator('#comboChoiceForm [name=addon]').check();assert((await p.locator('.menu-choice-total').innerText()).includes("43,02"));
  await shot("pizza-combo-desktop");await p.getByRole("button",{name:"Adicionar combo ao pedido",exact:true}).click();
  assert.equal(await p.evaluate(()=>posCart[0].components[0].flavors.length),2);
  assert.equal((await read("status")).version,"0.12.0-alpha.1");
  assert.equal(errors.length,0,errors.join("\n"));
  const report={result:"PASS",data:"synthetic QA only",version:"0.12.0-alpha.1",checks:["configuration via administration UI","active flavors and limits","half-cent rounding and weighted thirds","variants and add-ons","desktop/mobile/dark","cart edit and reload","manual checkout with server pricing","receipt snapshot and stock","pizza choices inside a combo"],javascript_errors:errors};
  fs.writeFileSync(out+"/results.json",JSON.stringify(report,null,2));console.log(JSON.stringify(report));await browser.close();
})().catch(e=>{console.error(e);process.exit(1)});
