const $ = (selector) =>
  document.getElementById(selector) ||
  document.querySelector(
    selector.startsWith("#") || /[ .[>:]/.test(selector)
      ? selector
      : "#" + selector,
  );
let setupToken = new URLSearchParams(location.hash.slice(1)).get("setup") || "";
history.replaceState(null, "", location.pathname);
let orderKey = crypto.randomUUID();
let currentUser = null;
let currentStoreId = null;
let allCategories = [];
let allProducts = [];
let allAddonGroups = [];
let allCombos = [];

async function api(path, data, extra = {}) {
  let headers = {
    "Content-Type": "application/json",
    "X-OpenFood": "1",
    ...extra,
  };
  if (currentStoreId) headers["X-Store-ID"] = String(currentStoreId);
  let res = await fetch("/api/" + path, {
    method: data === undefined ? "GET" : "POST",
    headers,
    body: data === undefined ? undefined : JSON.stringify(data),
  });
  let result = await res.json();
  if (!res.ok) {
    if (res.status === 401) show("login");
    throw Error(result.error || "Falha na operação");
  }
  return result;
}

async function apiPatch(path, data) {
  let headers = { "Content-Type": "application/json", "X-OpenFood": "1" };
  if (currentStoreId) headers["X-Store-ID"] = String(currentStoreId);
  let res = await fetch("/api/" + path, {
    method: "PATCH",
    headers,
    body: JSON.stringify(data),
  });
  let result = await res.json();
  if (!res.ok) {
    if (res.status === 401) show("login");
    throw Error(result.error || "Falha na operação");
  }
  return result;
}

async function apiDelete(path) {
  let headers = { "Content-Type": "application/json", "X-OpenFood": "1" };
  if (currentStoreId) headers["X-Store-ID"] = String(currentStoreId);
  let res = await fetch("/api/" + path, { method: "DELETE", headers });
  let result = await res.json();
  if (!res.ok) {
    if (res.status === 401) show("login");
    throw Error(result.error || "Falha na operação");
  }
  return result;
}

function money(cents) {
  return (cents / 100).toLocaleString("pt-BR", {
    style: "currency",
    currency: "BRL",
  });
}

// Role labels
const roleLabels = {
  instance_admin: "Admin Instalação",
  org_admin: "Admin Org",
  store_manager: "Gerente",
  attendant: "Atendimento",
  kitchen: "Cozinha",
  dispatch: "Expedição",
  finance_viewer: "Financeiro",
};

function canManageCatalog() {
  return (
    currentUser &&
    ["instance_admin", "org_admin", "store_manager"].includes(currentUser.role)
  );
}
function canManageUsers() {
  return (
    currentUser &&
    ["instance_admin", "org_admin", "store_manager"].includes(currentUser.role)
  );
}
function canManageStores() {
  return (
    currentUser && ["instance_admin", "org_admin"].includes(currentUser.role)
  );
}
function canViewFinancials() {
  return (
    currentUser &&
    ["instance_admin", "org_admin", "store_manager", "finance_viewer"].includes(
      currentUser.role,
    )
  );
}
function canCreateOrders() {
  return (
    currentUser &&
    ["instance_admin", "org_admin", "store_manager", "attendant"].includes(
      currentUser.role,
    )
  );
}
function canAccessCustomers() {
  return (
    currentUser &&
    [
      "instance_admin",
      "org_admin",
      "store_manager",
      "finance_viewer",
      "attendant",
    ].includes(currentUser.role)
  );
}
function canAccessPayments() {
  return (
    currentUser &&
    ["instance_admin", "org_admin", "store_manager", "finance_viewer"].includes(
      currentUser.role,
    )
  );
}

async function loadCatalog() {
  try {
    [allCategories, allProducts, allAddonGroups, allCombos] = await Promise.all(
      [api("categories"), api("products"), api("addon-groups"), api("combos")],
    );

    // Populate category selects
    const catOptions = allCategories
      .map(
        (c) =>
          `<option value="${c.id}">${escapeHTML(c.name)}${c.parent_id ? " (sub)" : ""}</option>`,
      )
      .join("");
    $$('select[name="category_id"]').forEach((sel) => {
      sel.innerHTML =
        '<option value="">— Sem categoria —</option>' + catOptions;
    });
    $("#categoryParent").innerHTML =
      '<option value="">— Nenhuma (raiz) —</option>' + catOptions;

    // Populate product selects
    const prodOptions = allProducts
      .map(
        (p) =>
          `<option value="${p.id}">${escapeHTML(p.name)} — ${money(p.price_cents)}</option>`,
      )
      .join("");
    $("#variantProductSelect").innerHTML =
      '<option value="">— Selecione —</option>' + prodOptions;
    $("#productAddonProductSelect").innerHTML =
      '<option value="">— Selecione —</option>' + prodOptions;
    $("#comboItemProductSelect").innerHTML =
      '<option value="">— Selecione —</option>' + prodOptions;
    $("#comboItemComboSelect").innerHTML =
      '<option value="">— Selecione —</option>';

    // Populate addon group selects
    const addonGroupOptions = allAddonGroups
      .map((g) => `<option value="${g.id}">${escapeHTML(g.name)}</option>`)
      .join("");
    $("#addonOptionGroupSelect").innerHTML =
      '<option value="">— Selecione —</option>' + addonGroupOptions;

    // Populate combo selects
    const comboOptions = allCombos
      .map((c) => `<option value="${c.id}">${escapeHTML(c.name)}</option>`)
      .join("");
    $("#comboItemComboSelect").innerHTML =
      '<option value="">— Selecione —</option>' + comboOptions;

    // Render all tables
    await renderAdvancedProducts();
    await renderCategories();
    renderVariants();
    renderAddonGroups();
    renderAddonOptions();
    renderProductAddonGroups();
    renderCombos();
    renderComboItems();
  } catch (e) {
    msg("Erro ao carregar catálogo: " + e.message, "error");
  }
}

function $$(sel) {
  return [...document.querySelectorAll(sel)];
}

function parsePrice(str) {
  let parts = str.replace(",", ".").split(".");
  return Number(parts[0]) * 100 + Number((parts[1] || "").padEnd(2, "0"));
}

function formatQty(qty, unit) {
  if (unit === "kg" || unit === "g" || unit === "l" || unit === "ml") {
    return (
      Number(qty)
        .toFixed(3)
        .replace(/\.?0+$/, "") +
      " " +
      unit
    );
  }
  return Number(qty).toLocaleString("pt-BR") + " " + (unit || "unidade");
}

function resetAdvProductForm() {
  $("#advProductForm").reset();
  $("#advProductId").value = "";
  $("#advProductSubmit").textContent = "Salvar produto";
  $("#advProductCancel").classList.add("hidden");
}
$("#advProductCancel").onclick = resetAdvProductForm;

// --- Categories ---
async function renderCategories() {
  try {
    let cats = await api("categories");
    allCategories = cats;
    let tbody = $("categoriesTable");
    tbody.replaceChildren();
    for (let c of cats) {
      let r = document.createElement("tr");
      let parent = cats.find((x) => x.id === c.parent_id);
      td(r, "#" + c.id);
      td(r, c.name);
      td(r, parent ? parent.name : "—");
      td(r, c.display_order);
      td(
        r,
        c.active
          ? '<span class="badge badge-active">Ativa</span>'
          : '<span class="badge badge-inactive">Inativa</span>',
      );
      let actCell = document.createElement("td");
      let editBtn = document.createElement("button");
      editBtn.textContent = "Editar";
      editBtn.style.fontSize = "12px";
      editBtn.style.padding = "6px 10px";
      editBtn.onclick = () => editCategory(c);
      actCell.append(editBtn);
      let delBtn = document.createElement("button");
      delBtn.className = "btn-cancel";
      delBtn.textContent = "Excluir";
      delBtn.style.fontSize = "12px";
      delBtn.style.padding = "6px 10px";
      delBtn.onclick = () =>
        run(async () => {
          if (confirm('Excluir categoria "' + c.name + '"?')) {
            await apiDelete("categories?id=" + c.id);
            await loadCatalog();
          }
        });
      actCell.append(delBtn);
      r.append(actCell);
      tbody.append(r);
    }
  } catch (e) {
    msg("Erro ao carregar categorias: " + e.message, "error");
  }
}

function editCategory(c) {
  $("#categoryId").value = c.id;
  $('#categoryForm [name="name"]').value = c.name;
  $('#categoryForm [name="description"]').value = c.description || "";
  $('#categoryForm [name="parent_id"]').value = c.parent_id || "";
  $('#categoryForm [name="display_order"]').value = c.display_order || 0;
  $('#categoryForm [name="active"]').checked = c.active;
  $("#categorySubmit").textContent = "Atualizar categoria";
  $("#categoryCancel").classList.remove("hidden");
  $("categoryForm").scrollIntoView({ behavior: "smooth" });
}

$("#categoryForm").onsubmit = (e) => {
  e.preventDefault();
  if (!canManageCatalog()) {
    msg("Sem permissão.");
    return;
  }
  let id = $("#categoryId").value;
  let d = new FormData(e.target);
  let payload = {
    name: d.get("name"),
    description: d.get("description"),
    parent_id: d.get("parent_id") ? Number(d.get("parent_id")) : null,
    display_order: Number(d.get("display_order")) || 0,
    active: d.get("active") === "on",
  };
  if (id) payload.id = Number(id);
  let url = id ? "categories?id=" + id : "categories";
  run(async () => {
    if (id) {
      await apiPatch(url, payload);
    } else {
      await api(url, payload);
    }
    resetCategoryForm();
    await loadCatalog();
    msg(id ? "Categoria atualizada." : "Categoria criada.");
  });
};

function resetCategoryForm() {
  $("#categoryForm").reset();
  $("#categoryId").value = "";
  $("#categorySubmit").textContent = "Salvar categoria";
  $("#categoryCancel").classList.add("hidden");
}

$("#categoryCancel").onclick = resetCategoryForm;

// --- Variants ---
async function renderVariants() {
  try {
    if (!$("#variantProductSelect").value) return;
    let variants = await api(
      "product-variants?product_id=" + $("#variantProductSelect").value,
    );
    let tbody = $("variantsTable");
    tbody.replaceChildren();
    for (let v of variants) {
      let r = document.createElement("tr");
      let prod = allProducts.find((p) => p.id === v.product_id);
      td(r, "#" + v.id);
      td(r, prod ? prod.name : "#" + v.product_id);
      td(r, v.name);
      td(r, money(v.price_adjustment_cents));
      td(r, formatQty(v.stock_quantity, "unidade"));
      td(r, v.sku || "—");
      td(
        r,
        v.active
          ? '<span class="badge badge-active">Ativa</span>'
          : '<span class="badge badge-inactive">Inativa</span>',
      );
      let actCell = document.createElement("td");

      r.append(actCell);
      tbody.append(r);
    }
  } catch (e) {
    msg("Erro ao carregar variantes: " + e.message, "error");
  }
}

$("#variantProductSelect").onchange = renderVariants;

function editVariant(v) {
  $("#variantId").value = v.id;
  $("#variantProductId").value = v.product_id;
  $("#variantProductSelect").value = v.product_id;
  $('#variantForm [name="name"]').value = v.name;
  $('#variantForm [name="price_adjustment"]').value = (
    v.price_adjustment_cents / 100
  )
    .toFixed(2)
    .replace(".", ",");
  $('#variantForm [name="stock"]').value = v.stock_quantity;
  $('#variantForm [name="sku"]').value = v.sku || "";
  $('#variantForm [name="display_order"]').value = v.display_order || 0;
  $('#variantForm [name="active"]').checked = v.active;
  $("#variantSubmit").textContent = "Atualizar variante";
  $("#variantCancel").classList.remove("hidden");
  $("variantForm").scrollIntoView({ behavior: "smooth" });
}

$("#variantForm").onsubmit = (e) => {
  e.preventDefault();
  if (!canManageCatalog()) {
    msg("Sem permissão.");
    return;
  }
  let id = $("#variantId").value;
  let productId =
    $("#variantProductId").value || $("#variantProductSelect").value;
  let d = new FormData(e.target);
  let payload = {
    name: d.get("name"),
    price_adjustment_cents: parsePrice(d.get("price_adjustment")),
    stock_quantity: Number(d.get("stock")),
    sku: d.get("sku") || null,
    display_order: Number(d.get("display_order")) || 0,
    active: d.get("active") === "on",
  };
  let url = id
    ? "product-variants?id=" + id
    : "product-variants?product_id=" + productId;
  run(async () => {
    if (id) {
      await apiPatch(url, payload);
    } else {
      await api(url, payload);
    }
    resetVariantForm();
    await renderVariants();
    msg(id ? "Variante atualizada." : "Variante criada.");
  });
};

function resetVariantForm() {
  $("#variantForm").reset();
  $("#variantId").value = "";
  $("#variantProductId").value = "";
  $("#variantSubmit").textContent = "Salvar variante";
  $("#variantCancel").classList.add("hidden");
}

$("#variantCancel").onclick = resetVariantForm;

// --- Addon Groups ---
async function renderAddonGroups() {
  try {
    let groups = await api("addon-groups");
    allAddonGroups = groups;
    let tbody = $("addonGroupsTable");
    tbody.replaceChildren();
    for (let g of groups) {
      let r = document.createElement("tr");
      td(r, "#" + g.id);
      td(r, g.name);
      td(r, g.min_selections);
      td(r, g.max_selections);
      td(r, g.required ? '<span class="badge badge-active">Sim</span>' : "Não");
      td(r, g.display_order);
      td(
        r,
        g.active
          ? '<span class="badge badge-active">Ativo</span>'
          : '<span class="badge badge-inactive">Inativo</span>',
      );
      let actCell = document.createElement("td");

      r.append(actCell);
      tbody.append(r);
    }
  } catch (e) {
    msg("Erro ao carregar grupos de adicionais: " + e.message, "error");
  }
}

function editAddonGroup(g) {
  $("#addonGroupId").value = g.id;
  $('#addonGroupForm [name="name"]').value = g.name;
  $('#addonGroupForm [name="description"]').value = g.description || "";
  $('#addonGroupForm [name="min_sel"]').value = g.min_selections;
  $('#addonGroupForm [name="max_sel"]').value = g.max_selections;
  $('#addonGroupForm [name="display_order"]').value = g.display_order || 0;
  $('#addonGroupForm [name="required"]').checked = g.required;
  $('#addonGroupForm [name="active"]').checked = g.active;
  $("#addonGroupSubmit").textContent = "Atualizar grupo";
  $("#addonGroupCancel").classList.remove("hidden");
  $("addonGroupForm").scrollIntoView({ behavior: "smooth" });
}

$("#addonGroupForm").onsubmit = (e) => {
  e.preventDefault();
  if (!canManageCatalog()) {
    msg("Sem permissão.");
    return;
  }
  let id = $("#addonGroupId").value;
  let d = new FormData(e.target);
  let payload = {
    name: d.get("name"),
    description: d.get("description"),
    min_selections: Number(d.get("min_sel")),
    max_selections: Number(d.get("max_sel")),
    required: d.get("required") === "on",
    display_order: Number(d.get("display_order")) || 0,
    active: d.get("active") === "on",
  };
  let url = id ? "addon-groups?id=" + id : "addon-groups";
  run(async () => {
    if (id) {
      await apiPatch(url, payload);
    } else {
      await api(url, payload);
    }
    resetAddonGroupForm();
    await loadCatalog();
    msg(id ? "Grupo atualizado." : "Grupo criado.");
  });
};

function resetAddonGroupForm() {
  $("#addonGroupForm").reset();
  $("#addonGroupId").value = "";
  $("#addonGroupSubmit").textContent = "Salvar grupo";
  $("#addonGroupCancel").classList.add("hidden");
}

$("#addonGroupCancel").onclick = resetAddonGroupForm;

// --- Addon Options ---
async function renderAddonOptions() {
  try {
    if (!$("#addonOptionGroupSelect").value) return;
    let options = await api(
      "addon-options?group_id=" + $("#addonOptionGroupSelect").value,
    );
    let tbody = $("addonOptionsTable");
    tbody.replaceChildren();
    for (let o of options) {
      let r = document.createElement("tr");
      let group = allAddonGroups.find((g) => g.id === o.group_id);
      td(r, "#" + o.id);
      td(r, group ? group.name : "#" + o.group_id);
      td(r, o.name);
      td(r, money(o.price_cents));
      td(r, formatQty(o.stock_quantity, "unidade"));
      td(
        r,
        o.is_default ? '<span class="badge badge-active">Sim</span>' : "Não",
      );
      td(
        r,
        o.active
          ? '<span class="badge badge-active">Ativa</span>'
          : '<span class="badge badge-inactive">Inativa</span>',
      );
      let actCell = document.createElement("td");

      r.append(actCell);
      tbody.append(r);
    }
  } catch (e) {
    msg("Erro ao carregar opções: " + e.message, "error");
  }
}

$("#addonOptionGroupSelect").onchange = renderAddonOptions;

function editAddonOption(o) {
  $("#addonOptionId").value = o.id;
  $("#addonOptionGroupId").value = o.group_id;
  $("#addonOptionGroupSelect").value = o.group_id;
  $('#addonOptionForm [name="name"]').value = o.name;
  $('#addonOptionForm [name="description"]').value = o.description || "";
  $('#addonOptionForm [name="price"]').value = (o.price_cents / 100)
    .toFixed(2)
    .replace(".", ",");
  $('#addonOptionForm [name="stock"]').value = o.stock_quantity;
  $('#addonOptionForm [name="display_order"]').value = o.display_order || 0;
  $('#addonOptionForm [name="is_default"]').checked = o.is_default;
  $('#addonOptionForm [name="active"]').checked = o.active;
  $("#addonOptionSubmit").textContent = "Atualizar opção";
  $("#addonOptionCancel").classList.remove("hidden");
  $("addonOptionForm").scrollIntoView({ behavior: "smooth" });
}

$("#addonOptionForm").onsubmit = (e) => {
  e.preventDefault();
  if (!canManageCatalog()) {
    msg("Sem permissão.");
    return;
  }
  let id = $("#addonOptionId").value;
  let groupId =
    $("#addonOptionGroupId").value || $("#addonOptionGroupSelect").value;
  let d = new FormData(e.target);
  let payload = {
    name: d.get("name"),
    description: d.get("description"),
    price_cents: parsePrice(d.get("price")),
    stock_quantity: Number(d.get("stock")),
    is_default: d.get("is_default") === "on",
    display_order: Number(d.get("display_order")) || 0,
    active: d.get("active") === "on",
  };
  let url = id ? "addon-options?id=" + id : "addon-options?group_id=" + groupId;
  run(async () => {
    if (id) {
      await apiPatch(url, payload);
    } else {
      await api(url, payload);
    }
    resetAddonOptionForm();
    await renderAddonOptions();
    msg(id ? "Opção atualizada." : "Opção criada.");
  });
};

function resetAddonOptionForm() {
  $("#addonOptionForm").reset();
  $("#addonOptionId").value = "";
  $("#addonOptionGroupId").value = "";
  $("#addonOptionSubmit").textContent = "Salvar opção";
  $("#addonOptionCancel").classList.add("hidden");
}

$("#addonOptionCancel").onclick = resetAddonOptionForm;

// --- Product Addon Groups ---
async function renderProductAddonGroups() {
  try {
    let pid = $("#productAddonProductSelect").value;
    if (!pid) {
      $("#productAddonGroupsList").innerHTML =
        '<p class="notice">Selecione um produto para gerenciar seus grupos de adicionais.</p>';
      return;
    }
    let linked = await api("product-addon-groups?product_id=" + pid);
    let linkedIds = new Set(linked.map((l) => l.group_id));
    let html = allAddonGroups
      .map(
        (g) => `
      <label style="display:flex;align-items:center;gap:8px;padding:10px;border:1px solid var(--border-subtle);border-radius:var(--radius-sm);background:var(--bg-surface);cursor:pointer;">
        <input type="checkbox" ${linkedIds.has(g.id) ? "checked" : ""} onchange="toggleProductAddonGroup(${pid},${g.id},this.checked)">
        <span>${escapeHTML(g.name)}</span>
        <small style="color:var(--text-muted);margin-left:auto;">${g.min_selections}–${g.max_selections} seleções</small>
      </label>
    `,
      )
      .join("");
    $("#productAddonGroupsList").innerHTML =
      html || '<p class="notice">Nenhum grupo de adicionais cadastrado.</p>';
  } catch (e) {
    msg("Erro ao carregar vínculos: " + e.message, "error");
  }
}

$("#productAddonProductSelect").onchange = renderProductAddonGroups;

async function toggleProductAddonGroup(productId, groupId, checked) {
  try {
    if (checked) {
      await api("product-addon-groups", {
        product_id: productId,
        group_id: groupId,
      });
    } else {
      await apiDelete(
        "product-addon-groups?product_id=" + productId + "&group_id=" + groupId,
      );
    }
    await renderProductAddonGroups();
  } catch (e) {
    msg("Erro ao atualizar vínculo: " + e.message, "error");
    renderProductAddonGroups();
  }
}

// --- Combos ---
async function renderCombos() {
  try {
    let combos = await api("combos");
    allCombos = combos;
    let tbody = $("combosTable");
    tbody.replaceChildren();
    for (let c of combos) {
      let r = document.createElement("tr");
      td(r, "#" + c.id);
      td(r, c.name);
      td(r, money(c.price_cents));
      td(
        r,
        c.pricing_policy === "fixed"
          ? "Fixo"
          : c.pricing_policy === "highest"
            ? "Maior item"
            : "Soma",
      );
      td(
        r,
        c.active
          ? '<span class="badge badge-active">Ativo</span>'
          : '<span class="badge badge-inactive">Inativo</span>',
      );
      let actCell = document.createElement("td");

      let itemsBtn = document.createElement("button");
      itemsBtn.className = "btn-secondary";
      itemsBtn.textContent = "Itens";
      itemsBtn.style.fontSize = "12px";
      itemsBtn.style.padding = "6px 10px";
      itemsBtn.onclick = () => {
        $("#comboItemComboSelect").value = c.id;
        $("#subtabCombos").click();
        $("#subtabCombos").click();
        renderComboItems();
      };
      actCell.append(itemsBtn);
      r.append(actCell);
      tbody.append(r);
    }
    // Update combo item combo select
    $("#comboItemComboSelect").innerHTML =
      '<option value="">— Selecione —</option>' +
      combos
        .map((c) => `<option value="${c.id}">${escapeHTML(c.name)}</option>`)
        .join("");
  } catch (e) {
    msg("Erro ao carregar combos: " + e.message, "error");
  }
}

function editCombo(c) {
  $("#comboId").value = c.id;
  $('#comboForm [name="name"]').value = c.name;
  $('#comboForm [name="description"]').value = c.description || "";
  $('#comboForm [name="price"]').value = (c.price_cents / 100)
    .toFixed(2)
    .replace(".", ",");
  $('#comboForm [name="pricing_policy"]').value = c.pricing_policy;
  $('#comboForm [name="display_order"]').value = c.display_order || 0;
  $('#comboForm [name="active"]').checked = c.active;
  $("#comboSubmit").textContent = "Atualizar combo";
  $("#comboCancel").classList.remove("hidden");
  $("comboForm").scrollIntoView({ behavior: "smooth" });
}

$("#comboForm").onsubmit = (e) => {
  e.preventDefault();
  if (!canManageCatalog()) {
    msg("Sem permissão.");
    return;
  }
  let id = $("#comboId").value;
  let d = new FormData(e.target);
  let payload = {
    name: d.get("name"),
    description: d.get("description"),
    price_cents: parsePrice(d.get("price")),
    pricing_policy: d.get("pricing_policy"),
    display_order: Number(d.get("display_order")) || 0,
    active: d.get("active") === "on",
  };
  let url = id ? "combos?id=" + id : "combos";
  run(async () => {
    if (id) {
      await apiPatch(url, payload);
    } else {
      await api(url, payload);
    }
    resetComboForm();
    await loadCatalog();
    msg(id ? "Combo atualizado." : "Combo criado.");
  });
};

function resetComboForm() {
  $("#comboForm").reset();
  $("#comboId").value = "";
  $("#comboSubmit").textContent = "Salvar combo";
  $("#comboCancel").classList.add("hidden");
}

$("#comboCancel").onclick = resetComboForm;

// --- Combo Items ---
async function renderComboItems() {
  try {
    let cid = $("#comboItemComboSelect").value;
    if (!cid) {
      $("#comboItemsTable").innerHTML = "";
      return;
    }
    let items = await api("combo-items?combo_id=" + cid);
    let tbody = $("comboItemsTable");
    tbody.replaceChildren();
    for (let i of items) {
      let r = document.createElement("tr");
      let combo = allCombos.find((c) => c.id === i.combo_id);
      let prod = allProducts.find((p) => p.id === i.product_id);
      td(r, combo ? combo.name : "#" + i.combo_id);
      td(r, prod ? prod.name : "#" + i.product_id);
      td(r, formatQty(i.quantity, prod ? prod.unit : "unidade"));
      td(r, i.group_name || "—");
      td(
        r,
        i.is_optional ? '<span class="badge badge-active">Sim</span>' : "Não",
      );
      td(r, i.min_selections);
      td(r, i.max_selections);
      td(r, i.display_order);
      let actCell = document.createElement("td");
      let delBtn = document.createElement("button");
      delBtn.className = "btn-cancel";
      delBtn.textContent = "Remover";
      delBtn.style.fontSize = "12px";
      delBtn.style.padding = "6px 10px";
      delBtn.onclick = () =>
        run(async () => {
          if (confirm("Remover item do combo?")) {
            await apiDelete(
              "combo-items?combo_id=" + cid + "&product_id=" + i.product_id,
            );
            await renderComboItems();
          }
        });
      actCell.append(delBtn);
      r.append(actCell);
      tbody.append(r);
    }
  } catch (e) {
    msg("Erro ao carregar itens do combo: " + e.message, "error");
  }
}

$("#comboItemComboSelect").onchange = renderComboItems;

$("#comboItemForm").onsubmit = (e) => {
  e.preventDefault();
  if (!canManageCatalog()) {
    msg("Sem permissão.");
    return;
  }
  let comboId =
    $("#comboItemComboId").value || $("#comboItemComboSelect").value;
  let d = new FormData(e.target);
  let payload = {
    product_id: Number(d.get("product_id")),
    quantity: Number(d.get("quantity")),
    is_optional: d.get("is_optional") === "on",
    group_name: d.get("group_name") || null,
    min_selections: Number(d.get("min_sel")),
    max_selections: Number(d.get("max_sel")),
    display_order: Number(d.get("display_order")) || 0,
  };
  run(async () => {
    await api("combo-items?combo_id=" + comboId, payload);
    resetComboItemForm();
    await renderComboItems();
    msg("Item adicionado ao combo.");
  });
};

function resetComboItemForm() {
  $("#comboItemForm").reset();
  $("#comboItemComboId").value = "";
  $("#comboItemProductId").value = "";
  $("#comboItemSubmit").textContent = "Adicionar item";
  $("#comboItemCancel").classList.add("hidden");
}

$("#comboItemCancel").onclick = resetComboItemForm;

// --- Users ---
async function loadUsers() {
  try {
    let users = await api("users");
    let tbody = $("usersTable");
    tbody.replaceChildren();
    let stores = [];
    try {
      stores = await api("stores");
    } catch (e) {}
    let storeMap = {};
    for (let s of stores) storeMap[s.id] = s.name;
    $("newUserStore").innerHTML =
      '<option value="">— Sem loja específica —</option>';
    for (let s of stores) {
      let o = document.createElement("option");
      o.value = s.id;
      o.textContent = s.name;
      $("newUserStore").append(o);
    }
    for (let u of users) {
      let r = document.createElement("tr");
      td(r, u.email);
      let roleCell = document.createElement("td");
      let rb = document.createElement("span");
      rb.className = "badge badge-role-" + u.role;
      rb.textContent = roleLabels[u.role] || u.role;
      roleCell.append(rb);
      r.append(roleCell);
      td(r, u.store_id ? storeMap[u.store_id] || "#" + u.store_id : "—");
      let statusCell = document.createElement("td");
      let sb = document.createElement("span");
      sb.className = "badge " + (u.active ? "badge-active" : "badge-inactive");
      sb.textContent = u.active ? "Ativo" : "Inativo";
      statusCell.append(sb);
      r.append(statusCell);
      let actCell = document.createElement("td");
      if (
        u.id !== currentUser?.id &&
        (currentUser.role === "instance_admin" ||
          (currentUser.role === "org_admin" && u.role !== "instance_admin") ||
          (currentUser.role === "store_manager" &&
            !["instance_admin", "org_admin", "store_manager"].includes(u.role)))
      ) {
        let toggleBtn = document.createElement("button");
        toggleBtn.className = u.active ? "btn-cancel btn-secondary" : "";
        toggleBtn.style.fontSize = "12px";
        toggleBtn.style.padding = "6px 10px";
        toggleBtn.textContent = u.active ? "Desativar" : "Reativar";
        toggleBtn.onclick = () =>
          run(async () => {
            await apiPatch("users", { id: u.id, active: !u.active });
            await loadUsers();
          });
        actCell.append(toggleBtn);
      }
      r.append(actCell);
      tbody.append(r);
    }
    $("addUserPanel").classList.toggle("hidden", !canManageUsers());
  } catch (e) {
    msg("Erro ao carregar usuários: " + e.message, "error");
  }
}

$("#addUserForm").onsubmit = (e) => {
  e.preventDefault();
  let d = Object.fromEntries(new FormData(e.target));
  let payload = { email: d.email, password: d.password, role: d.role };
  if (d.store_id) payload.store_id = Number(d.store_id);
  run(async () => {
    await api("users", payload);
    e.target.reset();
    await loadUsers();
    msg("Usuário criado com sucesso.");
  });
};

// --- Stores ---
async function loadStores() {
  try {
    let stores = await api("stores");
    let tbody = $("storesTable");
    tbody.replaceChildren();
    for (let s of stores) {
      let r = document.createElement("tr");
      td(r, "#" + s.id);
      td(r, s.name);
      td(r, "#" + s.org_id);
      td(r, new Date(s.created_at).toLocaleString("pt-BR"));
      tbody.append(r);
    }
    $("addStorePanel").classList.toggle("hidden", !canManageStores());
  } catch (e) {
    msg("Erro ao carregar lojas: " + e.message, "error");
  }
}

$("#addStoreForm").onsubmit = (e) => {
  e.preventDefault();
  let d = Object.fromEntries(new FormData(e.target));
  run(async () => {
    await api("stores", { name: d.name });
    e.target.reset();
    await loadStores();
    msg("Loja criada com sucesso.");
  });
};

// --- Customers & LGPD ---
async function loadCustomers() {
  try {
    let [customers, consents] = await Promise.all([
      api("customers"),
      api("consents"),
    ]);
    let tbody = $("customersTable");
    tbody.replaceChildren();
    for (let c of customers) {
      let r = document.createElement("tr");
      td(r, "#" + c.id);
      td(r, c.name);
      td(r, c.phone || "—");
      td(r, c.email || "—");
      td(r, c.document || "—");
      td(
        r,
        c.marketing_consent
          ? '<span class="badge badge-active">Sim</span>'
          : "Não",
      );
      td(r, new Date(c.created_at).toLocaleString("pt-BR"));
      let actCell = document.createElement("td");
      let editBtn = document.createElement("button");
      editBtn.textContent = "Editar";
      editBtn.style.fontSize = "12px";
      editBtn.style.padding = "6px 10px";
      editBtn.onclick = () => editCustomer(c);
      actCell.append(editBtn);
      let delBtn = document.createElement("button");
      delBtn.className = "btn-cancel";
      delBtn.textContent = "Excluir";
      delBtn.style.fontSize = "12px";
      delBtn.style.padding = "6px 10px";
      delBtn.onclick = () =>
        run(async () => {
          if (confirm('Excluir cliente "' + c.name + '"?')) {
            await apiDelete("customers?id=" + c.id);
            await loadCustomers();
          }
        });
      actCell.append(delBtn);
      r.append(actCell);
      tbody.append(r);
    }

    // Populate consent customer select
    $("#consentCustomerSelect").innerHTML =
      '<option value="">— Selecione —</option>' +
      customers
        .map(
          (c) =>
            `<option value="${c.id}">${escapeHTML(c.name)} (${escapeHTML(c.phone || c.email || "sem contato")})</option>`,
        )
        .join("");
    $("#lgpdExportCustomerSelect").innerHTML =
      '<option value="">— Selecione —</option>' +
      customers
        .map((c) => `<option value="${c.id}">${escapeHTML(c.name)}</option>`)
        .join("");
    $("#lgpdDeleteCustomerSelect").innerHTML =
      '<option value="">— Selecione —</option>' +
      customers
        .map((c) => `<option value="${c.id}">${escapeHTML(c.name)}</option>`)
        .join("");

    // Render consents
    renderConsents(consents);
  } catch (e) {
    msg("Erro ao carregar clientes: " + e.message, "error");
  }
}

function editCustomer(c) {
  $("#customerId").value = c.id;
  $('#customerForm [name="name"]').value = c.name;
  $('#customerForm [name="phone"]').value = c.phone || "";
  $('#customerForm [name="email"]').value = c.email || "";
  $('#customerForm [name="document"]').value = c.document || "";
  $('#customerForm [name="notes"]').value = c.notes || "";
  $('#customerForm [name="marketing_consent"]').checked = c.marketing_consent;
  $("#customerSubmit").textContent = "Atualizar cliente";
  $("#customerCancel").classList.remove("hidden");
  $("customerForm").scrollIntoView({ behavior: "smooth" });
}

$("#customerForm").onsubmit = (e) => {
  e.preventDefault();
  if (!canManageUsers()) {
    msg("Sem permissão.");
    return;
  }
  let id = $("#customerId").value;
  let d = new FormData(e.target);
  let payload = {
    name: d.get("name"),
    phone: d.get("phone") || null,
    email: d.get("email") || null,
    document: d.get("document") || null,
    notes: d.get("notes") || null,
    marketing_consent: d.get("marketing_consent") === "on",
  };
  if (id) payload.id = Number(id);
  let url = id ? "customers?id=" + id : "customers";
  run(async () => {
    if (id) {
      await apiPatch(url, payload);
    } else {
      await api(url, payload);
    }
    resetCustomerForm();
    await loadCustomers();
    msg(id ? "Cliente atualizado." : "Cliente criado.");
  });
};

function resetCustomerForm() {
  $("#customerForm").reset();
  $("#customerId").value = "";
  $("#customerSubmit").textContent = "Salvar cliente";
  $("#customerCancel").classList.add("hidden");
}

$("#customerCancel").onclick = resetCustomerForm;

async function renderConsents(consents) {
  let tbody = $("consentsTable");
  tbody.replaceChildren();
  let customers = await api("customers");
  let custMap = {};
  customers.forEach((c) => (custMap[c.id] = c.name));
  for (let c of consents) {
    let r = document.createElement("tr");
    td(r, "#" + c.id);
    td(r, custMap[c.customer_id] || "#" + c.customer_id);
    td(r, c.purpose);
    td(r, c.version);
    td(
      r,
      c.granted && !c.revoked_at
        ? '<span class="badge badge-active">Concedido</span>'
        : '<span class="badge badge-inactive">Revogado</span>',
    );
    td(r, c.granted_at ? new Date(c.granted_at).toLocaleString("pt-BR") : "—");
    td(r, c.revoked_at ? new Date(c.revoked_at).toLocaleString("pt-BR") : "—");
    td(r, c.source);
    let actCell = document.createElement("td");
    if (c.granted && !c.revoked_at) {
      let revokeBtn = document.createElement("button");
      revokeBtn.className = "btn-cancel";
      revokeBtn.textContent = "Revogar";
      revokeBtn.style.fontSize = "12px";
      revokeBtn.style.padding = "6px 10px";
      revokeBtn.onclick = () =>
        run(async () => {
          if (confirm("Revogar este consentimento?")) {
            await apiPatch("consents", { id: c.id });
            await loadCustomers();
          }
        });
      actCell.append(revokeBtn);
    }
    r.append(actCell);
    tbody.append(r);
  }
}

$("#consentForm").onsubmit = (e) => {
  e.preventDefault();
  if (!canManageUsers()) {
    msg("Sem permissão.");
    return;
  }
  let d = new FormData(e.target);
  let evidence = null;
  try {
    evidence = JSON.parse(d.get("evidence") || "null");
  } catch (e) {
    evidence = d.get("evidence");
  }
  let payload = {
    customer_id: Number(d.get("customer_id")),
    purpose: d.get("purpose"),
    version: d.get("version"),
    granted: d.get("granted") === "on",
    source: d.get("source"),
    evidence: evidence,
  };
  run(async () => {
    await api("consents", payload);
    e.target.reset();
    await loadCustomers();
    msg("Consentimento registrado.");
  });
};

async function lgpdExport(customerId) {
  try {
    let res = await fetch("/api/customer-export?customer_id=" + customerId, {
      headers: { "X-OpenFood": "1", "X-Store-ID": String(currentStoreId) },
    });
    if (!res.ok) {
      if (res.status === 401) show("login");
      throw Error((await res.json()).error || "Falha na exportação");
    }
    let blob = await res.blob();
    download(
      blob,
      "customer-export-" +
        customerId +
        "-" +
        new Date().toISOString().split("T")[0] +
        ".json",
    );
    msg("Exportação concluída. O arquivo contém todos os dados do titular.");
  } catch (e) {
    msg("Erro na exportação: " + e.message, "error");
  }
}

async function lgpdDelete(customerId) {
  if (
    !confirm(
      "ATENÇÃO: Esta ação é irreversível. Se o cliente tiver pedidos ativos, os dados serão anonimizados (mantidos para conformidade fiscal). Continuar?",
    )
  )
    return;
  try {
    let res = await fetch("/api/customer-delete?customer_id=" + customerId, {
      method: "DELETE",
      headers: { "X-OpenFood": "1", "X-Store-ID": String(currentStoreId) },
    });
    if (!res.ok) {
      if (res.status === 401) show("login");
      throw Error((await res.json()).error || "Falha na exclusão");
    }
    let result = await res.json();
    if (result.anonymized) {
      msg("Cliente anonimizado. Pedidos mantidos para conformidade fiscal.");
    } else {
      msg("Cliente excluído permanentemente.");
    }
    await loadCustomers();
  } catch (e) {
    msg("Erro na exclusão/anonimização: " + e.message, "error");
  }
}

$("#lgpdExportBtn").onclick = () => {
  let cid = $("#lgpdExportCustomerSelect").value;
  if (!cid) {
    msg("Selecione um cliente.");
    return;
  }
  lgpdExport(cid);
};
$("#lgpdDeleteBtn").onclick = () => {
  let cid = $("#lgpdDeleteCustomerSelect").value;
  if (!cid) {
    msg("Selecione um cliente.");
    return;
  }
  if (!$("#lgpdDeleteConfirm").checked) {
    msg("Marque a confirmação.");
    return;
  }
  lgpdDelete(cid);
};

// --- Maintenance ---
function download(blob, name) {
  let link = document.createElement("a");
  link.href = URL.createObjectURL(blob);
  link.download = name;
  link.click();
  setTimeout(() => URL.revokeObjectURL(link.href), 1000);
}
$("#backup").onclick = () =>
  run(async () => {
    let r = await fetch("/api/backup", {
      method: "POST",
      headers: { "X-OpenFood": "1" },
    });
    if (!r.ok) throw Error("backup_failed");
    download(
      await r.blob(),
      "openfood-" + new Date().toISOString().replaceAll(":", "-") + ".dump",
    );
    msg(
      "Backup criado. O arquivo contém dados pessoais e deve ser guardado em local protegido.",
    );
  });
$("#diagnostics").onclick = () =>
  run(async () =>
    download(
      new Blob([JSON.stringify(await api("diagnostics"), null, 2)], {
        type: "application/json",
      }),
      "openfood-diagnostics.json",
    ),
  );
$("#restore").onclick = () =>
  run(async () => {
    if ($("restoreConfirm").value !== "RESTAURAR" || !$("restoreFile").files[0])
      throw Error("Selecione o backup e confirme RESTAURAR");
    let r = await fetch("/api/restore", {
      method: "POST",
      headers: { "X-OpenFood": "1", "X-Confirm-Restore": "RESTAURAR" },
      body: $("restoreFile").files[0],
    });
    if (!r.ok) throw Error((await r.json()).error);
    show("login");
    msg("Banco restaurado. Entre novamente.");
  });
