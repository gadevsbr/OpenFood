Object.assign(menuErrors, {
  pizza_configuration_invalid: "Confira os sabores, preços e limite de 1 a 4 sabores. Pizza usa venda por unidade.",
  pizza_selection_invalid: "Escolha sabores disponíveis, sem repetir, com até 8 partes no total."
});
function pizzaChoices(root, config, existing) {
  if (!config?.enabled) return { read: () => [], price: base => base, name: () => "" };
  const field = menuNode("fieldset", "menu-choice-group");
  field.append(menuNode("legend", "", "Sabores da pizza"));
  field.append(menuNode("p", "menu-hint", `Escolha até ${config.max_flavors} sabores. Partes iguais: 1 + 1 faz meio a meio; 2 + 1 faz dois terços e um terço. Máximo de 8 partes.`));
  field.append(menuNode("p", "menu-hint", config.pricing_policy === "highest" ? "Preço do sabor mais caro, mais ajuste do tamanho e adicionais." : "Média dos preços proporcional às partes, arredondada para o centavo mais próximo (meio centavo para cima), mais tamanho e adicionais."));
  const inputs = config.flavors.filter(f => f.active).map(f => {
    const row = menuNode("div", "pizza-flavor-row");
    const checked = menuCheck(row, `${f.name} · ${money(f.price_cents)}`, existing?.flavors?.some(p => p.flavor_id === f.id), "pizza_flavor");
    checked.value = f.id;
    const parts = menuField(row, `Partes de ${f.name}`, existing?.flavors?.find(p => p.flavor_id === f.id)?.parts || 1, "pizza_parts", "number");
    parts.min = 1; parts.max = 8; parts.disabled = !checked.checked;
    checked.addEventListener("change", () => { parts.disabled = !checked.checked; });
    field.append(row);
    return { f, checked, parts };
  });
  root.append(field);
  const chosen = () => inputs.filter(i => i.checked.checked);
  const read = () => {
    const selected = chosen(), total = selected.reduce((n,i) => n + Number(i.parts.value),0);
    if (!selected.length || selected.length > config.max_flavors || total > 8 || selected.some(i => !Number.isInteger(Number(i.parts.value)) || Number(i.parts.value)<1 || Number(i.parts.value)>8)) throw Error("pizza_selection_invalid");
    return selected.map(i => ({ flavor_id:i.f.id,parts:Number(i.parts.value) })).sort((a,b) => a.flavor_id.localeCompare(b.flavor_id));
  };
  const price = () => {
    const selected = chosen(), parts = selected.reduce((n,i) => n+Number(i.parts.value),0);
    if (!parts) return 0;
    return config.pricing_policy === "highest" ? Math.max(...selected.map(i => i.f.price_cents)) : Math.floor((selected.reduce((n,i) => n+i.f.price_cents*Number(i.parts.value),0)+Math.floor(parts/2))/parts);
  };
  const name = () => {
    const total = chosen().reduce((n,i) => n+Number(i.parts.value),0);
    return " · " + chosen().map(i => `${i.parts.value}/${total} ${i.f.name}`).join(" + ");
  };
  return { read, price, name };
}

async function pizzaEditor(product) {
  const store = currentStoreId, c = await api("pizza-configuration?product_id=" + product.id);
  if (store !== currentStoreId) return;
  let dialog = $("pizzaEditorDialog");
  if (!dialog) {
    dialog = document.createElement("dialog"); dialog.id="pizzaEditorDialog"; dialog.className="dialog menu-editor-dialog";
    dialog.setAttribute("aria-labelledby","pizzaEditorTitle");
    dialog.innerHTML='<div class="dialog-header"><h2 id="pizzaEditorTitle"></h2><button type="button" class="icon-button" aria-label="Fechar sabores">✕</button></div><div class="dialog-body"></div>';
    dialog.querySelector("button").onclick=()=>dialog.close(); document.body.append(dialog);
  }
  $("pizzaEditorTitle").textContent = product.name + " · sabores de pizza";
  const root = dialog.querySelector(".dialog-body"); root.replaceChildren();
  root.append(menuNode("p","menu-hint","Cada sabor tem o preço de uma pizza inteira no tamanho base. O ajuste dos tamanhos e os adicionais são somados depois. O estoque baixa pizzas inteiras do produto e tamanho, sem controle de ingredientes por fração."));
  const form=menuNode("form","menu-admin-form"); form.id="pizzaAdminForm";root.append(form);
  const enabled=menuCheck(form,"Ativar escolha de sabores",c.pizza.enabled,"pizza_enabled");
  const policyLabel=menuNode("label","","Regra de preço"), policy=document.createElement("select"); policy.name="pizza_policy";
  policy.append(new Option("Sabor mais caro","highest"),new Option("Média proporcional às partes","weighted_average"));policy.value=c.pizza.pricing_policy;policyLabel.append(policy);form.append(policyLabel);
  const max=menuField(form,"Máximo de sabores",c.pizza.max_flavors,"pizza_max","number");max.min=1;max.max=4;
  const rows=menuNode("div","pizza-admin-flavors"), entries=[];form.append(rows);
  const add=(f={id:"",name:"",price_cents:product.price_cents,active:true})=>{
    const row=menuNode("div","menu-admin-row"); row.dataset.flavorId=f.id;
    const name=menuField(row,"Sabor",f.name,"flavor_name"), price=menuField(row,"Preço inteiro (R$)",(f.price_cents/100).toFixed(2).replace(".",","),"flavor_price"), active=menuCheck(row,"Disponível",f.active,"flavor_active");
    name.required=true; price.inputMode="decimal";
    const e={f,row,name,price,active};entries.push(e);rows.append(row);
    if(!f.id) row.append(posButton("Descartar sabor",()=>{entries.splice(entries.indexOf(e),1);row.remove()}));
  };
  c.pizza.flavors.forEach(add);
  form.append(posButton("Adicionar sabor",()=>add()));
  const error=menuNode("p","pos-inline-error hidden");error.setAttribute("role","alert");
  const save=menuNode("button","","Salvar sabores");save.type="submit";form.append(error,save);
  form.onsubmit=async e=>{
    e.preventDefault();save.disabled=true;
    try {
      const flavors=entries.map(e=>{
        if(!/^[0-9]+([,.][0-9]{1,2})?$/.test(e.price.value.trim())) throw Error("pizza_configuration_invalid");
        return {id:e.f.id,name:e.name.value.trim(),price_cents:parsePrice(e.price.value.trim()),active:e.active.checked};
      });
      await api("pizza-configuration?product_id="+product.id,{product_id:product.id,version:c.version,pizza:{enabled:enabled.checked,pricing_policy:policy.value,max_flavors:Number(max.value),flavors}},{"X-Store-ID":String(store)});
      dialog.close();$("menuEditorDialog").close();if(currentStoreId===store)await refresh();msg("Sabores da pizza atualizados.");
    }catch(err){error.textContent=menuError(err);error.classList.remove("hidden")}finally{save.disabled=false}
  };
  if(!dialog.open)dialog.showModal();
}
