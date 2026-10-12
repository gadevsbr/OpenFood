Object.assign(menuErrors, {
  addon_configuration_invalid: "Confira nomes, preços, estoque e limites. Grupos obrigatórios precisam de opções disponíveis.",
  addon_product_invalid: "Um dos produtos não está disponível nesta loja. Reabra o cadastro.",
  addon_name_in_use: "Já existe um grupo ou opção com este nome. Confira os nomes repetidos.",
  addon_group_limit: "O limite é de 12 grupos por produto e 50 opções por grupo.",
  addon_group_not_found: "Este grupo não está disponível nesta loja.",
  addon_configuration_failed: "Não foi possível salvar. Confira os nomes repetidos e tente novamente."
});
// Keep legacy form elements for existing catalog refresh code; expose one atomic editor.
for (const section of [...$("subpanelAdicionais").children].slice(1)) section.classList.add("hidden");
$("subpanelAdicionais").firstElementChild.append(menuNode("p", "menu-hint", "Cadastre opções uma vez e use o mesmo grupo em vários produtos. Preço e estoque são compartilhados entre eles."));

async function sharedAddonEditor(id) {
  const priceCents = value => {
    if (!/^[0-9]+([,.][0-9]{1,2})?$/.test(value.trim())) throw Error("addon_configuration_invalid");
    const cents=parsePrice(value.trim());
    if(!Number.isSafeInteger(cents))throw Error("addon_configuration_invalid");
    return cents;
  };
  const store = currentStoreId;
  const [c, products] = await Promise.all([
    id ? api("addon-configuration?group_id=" + id) : Promise.resolve({version:"", group:{id:0,name:"",min_selections:0,max_selections:1,required:false,active:true,options:[]},description:"",display_order:0,products:[]}),
    api("products")
  ]);
  if (store !== currentStoreId || !canManageCatalog()) return;
  let dialog = $("addonEditorDialog");
  if (!dialog) {
    dialog = menuNode("dialog", "dialog menu-editor-dialog");dialog.id="addonEditorDialog";
    dialog.setAttribute("aria-labelledby","addonEditorTitle");
    dialog.innerHTML='<div class="dialog-header"><h2 id="addonEditorTitle"></h2><button type="button" class="icon-button" aria-label="Fechar adicionais">✕</button></div><div class="dialog-body"></div>';
    dialog.querySelector("button").onclick=()=>dialog.close();document.body.append(dialog);
  }
  $("addonEditorTitle").textContent=id ? c.group.name+" · adicionais" : "Novo grupo de adicionais";
  const root=dialog.querySelector(".dialog-body");root.replaceChildren();
  root.append(menuNode("p","menu-hint","As alterações de preço, disponibilidade e estoque valem para todos os produtos vinculados. Pedidos já registrados mantêm os dados da venda."));
  const form=menuNode("form","menu-admin-form");form.id="addonAdminForm";root.append(form);
  const info=menuNode("div","menu-admin-row");form.append(info);
  const name=menuField(info,"Nome do grupo",c.group.name,"group_name"), min=menuField(info,"Mínimo de escolhas",c.group.min_selections,"group_min","number"),max=menuField(info,"Máximo de escolhas",c.group.max_selections,"group_max","number");
  name.required=true;min.max=max.max=50;
  const required=menuCheck(info,"Escolha obrigatória",c.group.required,"group_required"),active=menuCheck(info,"Grupo disponível",c.group.active,"group_active");
  const description=menuField(form,"Descrição",c.description,"group_description");description.maxLength=500;
  const order=menuField(form,"Ordem no cadastro",c.display_order,"group_order","number");
  const optionsRoot=menuNode("div","addon-admin-options"),options=[];
  form.append(menuNode("h3","","Opções e estoque compartilhado"),optionsRoot);
  function addOption(o={id:0,name:"",price_cents:0,stock_quantity:0,active:true}) {
    const row=menuNode("div","menu-admin-row");optionsRoot.append(row);
    const n=menuField(row,"Adicional",o.name,"option_name"),price=menuField(row,"Preço (R$)",(o.price_cents/100).toFixed(2).replace(".",","),"option_price"),stock=menuField(row,"Estoque",o.stock_quantity,"option_stock","number"),a=menuCheck(row,"Disponível",o.active,"option_active");
    n.required=true;price.inputMode="decimal";
    const entry={o,n,price,stock,a};options.push(entry);
    if(!o.id) row.append(posButton("Descartar opção",()=>{options.splice(options.indexOf(entry),1);row.remove()}));
  }
  c.group.options.forEach(addOption);
  form.append(posButton("Adicionar opção",()=>{if(options.length<50)addOption()}));
  form.append(menuNode("h3","","Produtos vinculados"),menuNode("p","menu-hint","Selecione os produtos que usarão este grupo. Desmarcar remove apenas o vínculo; os pedidos anteriores são preservados."));
  const filter=menuField(form,"Buscar produto","","addon_product_search"),links=menuNode("div","addon-product-links");form.append(links);
  const selected=products.map(p=>{
    const checkbox=menuCheck(links,p.name+(p.active?"":" · inativo"),c.products.some(x=>x.id===p.id),"addon_product");checkbox.value=p.id;
    return {p,checkbox};
  });
  filter.oninput=()=>selected.forEach(({p,checkbox})=>checkbox.parentElement.classList.toggle("hidden",!p.name.toLocaleLowerCase().includes(filter.value.toLocaleLowerCase())));
  const impact=menuNode("p","menu-hint");impact.id="addonImpact";form.append(impact);
  const confirm=menuCheck(form,"Aplicar alterações aos produtos listados",false,"addon_confirm");
  const affected=()=>products.filter(p=>c.products.some(x=>x.id===p.id)||selected.some(x=>x.p.id===p.id&&x.checkbox.checked));
  const update=()=>{const list=affected();impact.textContent=list.length ? `Produtos afetados (${list.length}): ${list.map(p=>p.name).join(", ")}` : "Grupo sem produtos vinculados.";confirm.required=list.length>0;confirm.parentElement.classList.toggle("hidden",!list.length);confirm.checked=false};
  selected.forEach(x=>x.checkbox.onchange=update);update();
  const error=menuNode("p","pos-inline-error hidden");error.setAttribute("role","alert");form.append(error);
  const save=menuNode("button","","Salvar grupo e vínculos");save.type="submit";form.append(save);
  form.onsubmit=async e=>{
    e.preventDefault();if(save.disabled)return;save.disabled=true;
    try {
      if(store!==currentStoreId)throw Error("configuration_changed");
      await api("addon-configuration?group_id="+id,{version:c.version,description:description.value,display_order:Number(order.value),group:{id,name:name.value.trim(),min_selections:Number(min.value),max_selections:Number(max.value),required:required.checked,active:active.checked,options:options.map(({o,n,price,stock,a})=>({id:o.id,name:n.value.trim(),price_cents:priceCents(price.value),stock_quantity:Number(stock.value),expected_stock_quantity:o.stock_quantity,active:a.checked}))},products:selected.filter(x=>x.checkbox.checked).map(x=>({id:x.p.id,name:x.p.name}))});
      dialog.close();await loadCatalog();msg("Grupo salvo. Reabra os cadastros e atualize os itens do pedido que usam estes adicionais.");
    }catch(err){error.textContent=menuError(err);error.classList.remove("hidden");save.disabled=false}
  };
  dialog.showModal();name.focus();
}
