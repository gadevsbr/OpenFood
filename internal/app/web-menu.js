function menuPayloadItem(i) {
  return { product_id: i.product_id, quantity: i.quantity, variant_id: i.variant_id || 0, addon_option_ids: i.addon_option_ids || [], notes: i.notes || "", configuration_version: i.configuration_version || 0 };
}
const menuErrors = { configuration_changed: "O cardápio mudou. Atualize e personalize novamente o item.", configuration_stock_changed: "O estoque mudou durante a edição. Reabra o cadastro para conferir.", configuration_shared_group: "Este grupo está vinculado a outros produtos. Edite-o pelo cadastro de adicionais.", configuration_invalid: "Confira os tamanhos, os adicionais e os limites de escolha.", variant_required: "Escolha um tamanho disponível.", addon_unavailable: "Um adicional ficou indisponível. Confira a seleção.", addon_limits_invalid: "Confira as escolhas obrigatórias e os limites de adicionais.", configuration_save_failed: "Não foi possível salvar. Confira nomes repetidos e reabra o cadastro.", photo_invalid: "Use uma foto JPEG ou PNG de até 4096 pixels e 12 megapixels.", photo_too_large: "A foto deve ter até 2 MB. Reduza a imagem e tente novamente." };
function menuError(err) {
  return menuErrors[err.message] || err.message;
}
Object.assign(menuErrors,{stock_insufficient:"A quantidade excede o estoque disponível. Confira os itens do pedido.",product_unavailable:"Este produto ficou indisponível. Atualize o cardápio.",configuration_read_failed:"Não foi possível carregar as opções. Tente novamente.",photo_save_failed:"Não foi possível salvar a foto. Tente novamente."});
window.menuErrors = menuErrors;
function menuNode(tag, cls, text) {
  const el = document.createElement(tag);
  if (cls) el.className = cls;
  if (text != null) el.textContent = text;
  return el;
}
function menuField(root, label, value, name, type = "text") {
  const wrap = menuNode("label", "", label), input = document.createElement("input");
  input.type = type;
  input.name = name;
  input.value = value ?? "";
  if (type === "number") {
    input.min = "0";
    input.max = "1000000";
    input.step = "1";
  } else input.maxLength = 120;
  wrap.append(input);
  root.append(wrap);
  return input;
}
function menuCheck(root, label, value, name) {
  const wrap = menuNode("label", "menu-check"), input = document.createElement("input");
  input.type = "checkbox";
  input.name = name;
  input.checked = value;
  wrap.append(input, menuNode("span", "", label));
  root.append(wrap);
  return input;
}
function menuLineKey(i) {
  return JSON.stringify([i.product_id, i.variant_id || 0, [...i.addon_option_ids || []].sort((a, b) => a - b), i.notes || ""]);
}
let menuOpening = false;
async function menuChoose(product, existing) {
  if (!product) throw Error("product_unavailable");
  if (menuOpening || posAttempt || typeof diningPending !== "undefined" && diningPending) return;
  if (!existing && !product.has_configuration) {
    posAdd(product);
    return;
  }
  menuOpening = true;
  const store = currentStoreId;
  let config;
  try {
    config = await api("product-configuration?product_id=" + product.id);
  } finally {
    menuOpening = false;
  }
  if (currentStoreId !== store || appMode !== "pos") return;
  const variants = config.variants.filter((v) => v.active), groups = config.groups.filter((g) => g.active);
  if (!variants.length && !groups.length && !existing) {
    posAdd({ ...product, configuration_version: config.version });
    return;
  }
  const body = posDialog(product.name);
  body.classList.add("menu-configurator");
  const form = menuNode("form", "menu-choice-form");
  form.id = "menuChoiceForm";
  const content = menuNode("div", "menu-choice-content");
  form.append(content);
  if (product.has_photo) {
    const img = document.createElement("img");
    img.className = "menu-cover";
    img.alt = product.name;
    img.src = `/api/product-photo?product_id=${product.id}&store_id=${store}`;
    content.append(img);
  }
  if (product.description) content.append(menuNode("p", "detail-description", product.description));
  content.append(menuNode("p", "", `Preço base: ${money(product.price_cents)}`));
  let variantSelect;
  if (variants.length) {
    const label2 = menuNode("label", "", "Tamanho ou variante"), select = document.createElement("select");
    select.name = "variant";
    select.required = true;
    select.append(new Option("Selecione um tamanho", ""));
    for (const v of variants) {
      const o = new Option(`${v.name} · ${money(product.price_cents + v.price_adjustment_cents)} · ${v.stock_quantity} disponíveis`, v.id);
      o.disabled = v.stock_quantity < 1 || product.price_cents + v.price_adjustment_cents < 0;
      select.append(o);
    }
    select.value = existing?.variant_id ? String(existing.variant_id) : "";
    label2.append(select);
    content.append(label2);
    variantSelect = select;
  }
  const optionInputs = [];
  for (const g of groups) {
    const field = menuNode("fieldset", "menu-choice-group"), legend = menuNode("legend", "", g.name);
    field.append(legend, menuNode("p", "menu-hint", `${g.required ? "Obrigatório" : "Opcional"} · escolha de ${Math.max(g.min_selections, g.required ? 1 : 0)} a ${g.max_selections}`));
    for (const o of g.options.filter((o2) => o2.active)) {
      const input = menuCheck(field, `${o.name} · + ${money(o.price_cents)}${o.stock_quantity < 1 ? " · indisponível" : ""}`, existing?.addon_option_ids?.includes(o.id), "addon");
      input.value = o.id;
      input.disabled = o.stock_quantity < 1;
      optionInputs.push({ input, option: o, group: g });
    }
    content.append(field);
  }
  const qty = menuField(content, "Quantidade", existing?.quantity || 1, "quantity", "number");
  qty.min = 1;
  const label = menuNode("label", "", "Observação deste item"), notes = document.createElement("textarea");
  notes.name = "notes";
  notes.maxLength = 500;
  notes.rows = 2;
  notes.placeholder = "Ex.: sem cebola, molho à parte";
  notes.value = existing?.notes || "";
  label.append(notes);
  content.append(label);
  const total = menuNode("strong", "menu-choice-total"), error = menuNode("p", "pos-inline-error hidden");
  error.setAttribute("role", "alert");
  const submit = menuNode("button", "", existing ? "Salvar item" : "Adicionar ao pedido");
  submit.type = "submit";
  const actions = menuNode("div", "menu-choice-actions");
  actions.append(total, error, submit);
  form.append(actions);
  body.append(form);
  const calculation = () => product.price_cents + (variants.find((v) => String(v.id) === variantSelect?.value)?.price_adjustment_cents || 0) + optionInputs.filter((o) => o.input.checked).reduce((s, o) => s + o.option.price_cents, 0);
  const update = () => {
    total.textContent = "Total do item: " + money(calculation() * Number(qty.value || 1));
    error.classList.add("hidden");
  };
  form.oninput = update;
  update();
  form.onsubmit = (e) => {
    e.preventDefault();
    try {
      const variant = variants.find((v) => String(v.id) === variantSelect?.value), selected = optionInputs.filter((o) => o.input.checked), quantity = Number(qty.value);
      if (variants.length && !variant) throw Error("variant_required");
      for (const g of groups) {
        const n = selected.filter((o) => o.group.id === g.id).length;
        if (n < Math.max(g.min_selections, g.required ? 1 : 0) || n > g.max_selections) throw Error("addon_limits_invalid");
      }
      const other = posCart.filter((i) => i !== existing), used = other.filter((i) => i.product_id === product.id).reduce((s, i) => s + i.quantity, 0);
      if (!Number.isInteger(quantity) || quantity < 1 || quantity + used > product.stock_quantity || variant && quantity + other.filter((i) => i.variant_id === variant.id).reduce((s, i) => s + i.quantity, 0) > variant.stock_quantity || selected.some((o) => quantity + other.filter((i) => i.addon_option_ids?.includes(o.option.id)).reduce((s, i) => s + i.quantity, 0) > o.option.stock_quantity)) throw Error("stock_insufficient");
      const item = { product_id: product.id, quantity, variant_id: variant?.id || 0, addon_option_ids: selected.map((o) => o.option.id), notes: notes.value.trim(), configuration_version: config.version, variant_stock: variant?.stock_quantity, addon_stocks: Object.fromEntries(selected.map((o) => [o.option.id, o.option.stock_quantity])), price_cents: calculation(), name: product.name + (variant ? " · " + variant.name : "") + selected.map((o) => " + " + o.option.name).join("") };
      const match = other.find((i) => menuLineKey(i) === menuLineKey(item) && i.configuration_version === item.configuration_version && i.price_cents === item.price_cents);
      if (match) match.quantity += quantity;
      else other.push(item);
      posCart = other;
      posSave();
      renderPOSCart();
      $("posDialog").close();
    } catch (err) {
      error.textContent = menuError(err);
      error.classList.remove("hidden");
    }
  };
}
async function menuEditor(product) {
  const store = currentStoreId, config = await api("product-configuration?product_id=" + product.id);
  if (currentStoreId !== store) return;
  let dialog = $("menuEditorDialog");
  if (!dialog) {
    dialog = document.createElement("dialog");
    dialog.id = "menuEditorDialog";
    dialog.className = "dialog menu-editor-dialog";
    dialog.setAttribute("aria-labelledby", "menuEditorTitle");
    dialog.innerHTML = '<div class="dialog-header"><h2 id="menuEditorTitle"></h2><button type="button" class="icon-button" aria-label="Fechar configuração">✕</button></div><div class="dialog-body"></div>';
    dialog.querySelector("button").onclick = () => dialog.close();
    document.body.append(dialog);
  }
  $("menuEditorTitle").textContent = product.name + " · cardápio";
  const root = dialog.querySelector(".dialog-body");
  root.replaceChildren();
  const photo = menuNode("section", "menu-photo-editor");
  photo.append(menuNode("h3", "", "Foto do produto"), menuNode("p", "menu-hint", "JPEG ou PNG, até 2 MB. A foto fica neste computador e nos backups."));
  if (product.has_photo) {
    const img = document.createElement("img");
    img.className = "menu-photo-preview";
    img.alt = product.name;
    img.src = `/api/product-photo?product_id=${product.id}&store_id=${store}`;
    photo.append(img);
  }
  const file = menuField(photo, "Escolher foto", "", "photo", "file");
  file.accept = "image/jpeg,image/png";
  file.removeAttribute("maxlength");
  const photoError = menuNode("p", "pos-inline-error hidden");
  photoError.setAttribute("role", "alert");
  const upload = posButton("Salvar foto", async () => {
    upload.disabled = true;
    try {
      if (!file.files.length) throw Error("Escolha uma foto.");
      if (file.files[0].size > 2 * 1024 * 1024) throw Error("photo_too_large");
      const r = await fetch(`/api/product-photo?product_id=${product.id}`, { method: "POST", headers: { "X-OpenFood": "1", "X-Store-ID": String(store) }, body: file.files[0] });
      if (!r.ok) throw Error((await r.json()).error);
      product.has_photo = true;
      remove.disabled = false;
      let img = photo.querySelector("img");
      if (!img) {
        img = document.createElement("img");
        img.className = "menu-photo-preview";
        img.alt = product.name;
        photo.prepend(img);
      }
      img.src = `/api/product-photo?product_id=${product.id}&store_id=${store}&v=${Date.now()}`;
      file.value = "";
      photoError.classList.add("hidden");
      if (currentStoreId === store) await refresh();
    } catch (e) {
      photoError.textContent = menuError(e);
      photoError.classList.remove("hidden");
    } finally {
      upload.disabled = false;
    }
  });
  const remove = posButton("Remover foto", async () => {
    remove.disabled = true;
    try {
      const r = await fetch(`/api/product-photo?product_id=${product.id}`, { method: "DELETE", headers: { "X-OpenFood": "1", "X-Store-ID": String(store) } });
      if (!r.ok) throw Error((await r.json()).error);
      product.has_photo = false;
      photo.querySelector("img")?.remove();
      photoError.classList.add("hidden");
      if (currentStoreId === store) await refresh();
    } catch (e) {
      photoError.textContent = menuError(e);
      photoError.classList.remove("hidden");
    } finally {
      remove.disabled = !product.has_photo;
    }
  });
  photo.append(upload, remove);
  remove.disabled = !product.has_photo;
  photo.append(photoError);
  root.append(photo);
  const form = menuNode("form", "menu-admin-form");
  form.id = "menuAdminForm";
  root.append(form);
  form.append(menuNode("h3", "", "Tamanhos e variantes"), menuNode("p", "menu-hint", "O preço é o valor base mais o ajuste do tamanho. A venda baixa o estoque do produto e do tamanho."));
  const variantRoot = menuNode("div", "menu-admin-variants");
  form.append(variantRoot);
  const variants = [];
  function addVariant(v = { id: 0, name: "", price_adjustment_cents: 0, stock_quantity: 0, active: true }) {
    const row = menuNode("div", "menu-admin-row");
    row.dataset.variantId = v.id;
    const name = menuField(row, "Nome", v.name, "variant_name"), adjust = menuField(row, "Ajuste de preço (R$)", (v.price_adjustment_cents / 100).toFixed(2).replace(".", ","), "variant_adjustment"), stock = menuField(row, "Estoque", v.stock_quantity, "variant_stock", "number"), active = menuCheck(row, "Disponível", v.active, "variant_active");
    name.required = true;
    adjust.inputMode = "decimal";
    const entry = { v, row, name, adjust, stock, active };
    variants.push(entry);
    variantRoot.append(row);
    if (!v.id) row.append(posButton("Descartar tamanho", () => {
      variants.splice(variants.indexOf(entry), 1);
      row.remove();
    }));
  }
  config.variants.forEach(addVariant);
  form.append(posButton("Adicionar tamanho", () => addVariant()));
  form.append(menuNode("h3", "", "Grupos de adicionais"));
  const groupRoot = menuNode("div", "menu-admin-groups");
  form.append(groupRoot);
  const groups = [];
  function addGroup(g = { id: 0, name: "", min_selections: 0, max_selections: 1, required: false, active: true, options: [] }) {
    const row = menuNode("fieldset", "menu-admin-group"), legend = menuNode("legend", "", "Grupo de adicionais");
    row.append(legend);
    row.dataset.groupId = g.id;
    const fields = menuNode("div", "menu-admin-row");
    row.append(fields);
    const name = menuField(fields, "Nome do grupo", g.name, "group_name"), minimum = menuField(fields, "Mínimo", g.min_selections, "group_min", "number"), maximum = menuField(fields, "Máximo", g.max_selections, "group_max", "number"), required = menuCheck(fields, "Obrigatório", g.required, "group_required"), active = menuCheck(fields, "Disponível", g.active, "group_active");
    name.required = true;
    minimum.max = maximum.max = 50;
    const options = [], optionRoot = menuNode("div", "menu-admin-options");
    row.append(optionRoot);
    function addOption(o = { id: 0, name: "", price_cents: 0, stock_quantity: 0, active: true }) {
      const el = menuNode("div", "menu-admin-row");
      el.dataset.optionId = o.id;
      const name2 = menuField(el, "Adicional", o.name, "option_name"), price = menuField(el, "Preço (R$)", (o.price_cents / 100).toFixed(2).replace(".", ","), "option_price"), stock = menuField(el, "Estoque", o.stock_quantity, "option_stock", "number"), active2 = menuCheck(el, "Disponível", o.active, "option_active");
      name2.required = true;
      price.inputMode = "decimal";
      const entry2 = { o, name: name2, price, stock, active: active2 };
      options.push(entry2);
      optionRoot.append(el);
      if (!o.id) el.append(posButton("Descartar opção", () => {
        options.splice(options.indexOf(entry2), 1);
        el.remove();
      }));
    }
    g.options.forEach(addOption);
    row.append(posButton("Adicionar opção", () => addOption()));
    if (g.shared) {
      row.disabled = true;
      row.append(menuNode("p", "menu-hint", "Grupo compartilhado com outros produtos. A edição conjunta fica para outra etapa."));
    }
    const entry = { g, name, minimum, maximum, required, active, options };
    groups.push(entry);
    groupRoot.append(row);
    if (!g.id) row.append(posButton("Descartar grupo", () => {
      groups.splice(groups.indexOf(entry), 1);
      row.remove();
    }));
  }
  config.groups.forEach(addGroup);
  form.append(posButton("Adicionar grupo", () => addGroup()));
  const error = menuNode("p", "pos-inline-error hidden");
  error.setAttribute("role", "alert");
  const save = menuNode("button", "", "Salvar tamanhos e adicionais");
  save.type = "submit";
  form.append(error, save);
  const signedPrice = (value, negative = true) => {
    const s = value.trim();
    if (!/^-?[0-9]+([,.][0-9]{1,2})?$/.test(s) || !negative && s.startsWith("-")) throw Error("Informe os preços em reais, com até duas casas decimais.");
    const amount = s.startsWith("-") ? -parsePrice(s.slice(1)) : parsePrice(s);
    if (!Number.isSafeInteger(amount)) throw Error("Preço inválido.");
    return amount;
  };
  form.onsubmit = async (e) => {
    e.preventDefault();
    save.disabled = true;
    try {
      const c = { product_id: product.id, version: config.version, variants: variants.map(({ v, name, adjust, stock, active }) => ({ id: v.id, name: name.value.trim(), price_adjustment_cents: signedPrice(adjust.value), stock_quantity: Number(stock.value), expected_stock_quantity: v.stock_quantity, active: active.checked })), groups: groups.map(({ g, name, minimum, maximum, required, active, options }) => ({ id: g.id, name: name.value.trim(), min_selections: Number(minimum.value), max_selections: Number(maximum.value), required: required.checked, active: active.checked, shared: !!g.shared, options: options.map(({ o, name: name2, price, stock, active: active2 }) => ({ id: o.id, name: name2.value.trim(), price_cents: signedPrice(price.value, false), stock_quantity: Number(stock.value), expected_stock_quantity: o.stock_quantity, active: active2.checked })) })) };
      await api("product-configuration?product_id=" + product.id, c, { "X-Store-ID": String(store) });
      dialog.close();
      if (currentStoreId === store) await refresh();
      msg("Cardápio do produto atualizado.");
    } catch (e2) {
      error.textContent = menuError(e2);
      error.classList.remove("hidden");
    } finally {
      save.disabled = false;
    }
  };
  if (!dialog.open) dialog.showModal();
}
