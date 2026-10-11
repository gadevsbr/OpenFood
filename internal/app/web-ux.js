let editingProductStock = null;
let cartItems = [];
/* OpenFood operations workspace. No external assets or synthetic metrics. */
const icons = {
  orders: "M7 3h10v18l-3-2-2 2-2-2-3 2V3Z M10 7h4 M10 11h4 M10 15h2",
  catalog: "M4 7h16v14H4V7Z M3 3h18v4H3V3Z M9 11h6",
  clock: "M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20 M12 6v6l4 2",
};
const svgIcon = (name) =>
  `<svg class="icon" aria-hidden="true" viewBox="0 0 24 24"><path d="${icons[name] || icons.catalog}"></path></svg>`;
const escapeHTML = (value) =>
  String(value ?? "").replace(
    /[&<>"']/g,
    (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        c
      ],
  );
const stateLabels = {
  confirmed: "Na fila",
  preparing: "Em preparo",
  ready: "Pronto",
  completed: "Concluído",
  cancelled: "Cancelado",
  draft: "Rascunho",
  awaiting_confirmation: "Aguardando confirmação",
  awaiting_payment: "Aguardando pagamento",
};
const financialLabels = {
  manual_pending: "Pagamento pendente",
  manual_confirmed: "Pagamento registrado",
  payment_pending: "Aguardando Pix",
  payment_confirmed: "Pago",
  payment_expired: "Pagamento expirado",
  refund_pending: "Reembolso pendente",
  refund_partial: "Reembolso parcial",
  refunded: "Reembolsado",
};
let latestOrders = [],
  latestProducts = [],
  latestStores = [],
  orderFilter = "active",
  lastRefresh = null,
  refreshPromise = null,
  refreshGeneration = 0;
let messageTimer = null;
const activeStates = [
  "confirmed",
  "preparing",
  "ready",
  "awaiting_confirmation",
  "awaiting_payment",
  "draft",
];
const errors = {
  login_required: "Entre com sua conta para continuar.",
  session_invalid: "Sua sessão terminou. Entre novamente.",
  credentials_invalid: "E-mail ou senha incorretos.",
  login_invalid: "Confira o e-mail e a senha.",
  stock_insufficient:
    "O estoque mudou. Confira a quantidade e tente novamente.",
  product_unavailable: "Este produto não está disponível para pedido.",
  order_invalid: "Confira os itens do pedido.",
  cash_not_open: "Abra um caixa antes de registrar recebimentos ou devoluções.",
  cash_already_open: "Esta loja já tem um caixa aberto.",
  cash_changed_reload:
    "O saldo mudou durante a contagem. Atualize o caixa e confira os valores antes de fechar.",
  cash_difference_note_required: "Informe o motivo da diferença no fechamento.",
  cash_insufficient:
    "O caixa atual não tem dinheiro suficiente para esta devolução.",
  refund_before_cancel:
    "Registre a devolução do valor recebido antes de cancelar o pedido.",
  overpayment: "O valor excede o saldo do pedido. Confira os recebimentos.",
  receipt_already_refunded: "Este recebimento já foi devolvido.",
  refund_note_required: "Informe o motivo da devolução.",
  forbidden_refund_management:
    "Somente gerentes e administradores podem registrar devoluções.",
  forbidden_cash_management:
    "Somente gerentes e administradores podem abrir ou fechar caixa.",
  integrated_payment_unsupported:
    "Este pedido possui pagamento integrado e não aceita registro manual nesta etapa.",
  cash_operation_failed:
    "Não foi possível registrar a operação. Tente novamente.",
  product_invalid: "Confira os dados do produto.",
  product_changed_reload:
    "O estoque mudou. Feche e reabra a edição para conferir os valores atuais.",
  quantity_invalid: "Confira a quantidade mínima e máxima de cada produto.",
  order_duplicate_product: "Agrupe as quantidades do mesmo produto em um item.",
  availability_invalid: "Informe os dois horários de disponibilidade válidos.",
  category_invalid: "Escolha uma categoria desta loja.",
  sku_already_exists: "Este SKU já está cadastrado nesta loja.",
  email_already_registered: "Já existe uma conta com este e-mail.",
  forbidden_catalog_management_required:
    "Sua conta não pode alterar o catálogo.",
  forbidden_order_creation_required: "Sua conta não pode criar pedidos.",
  forbidden_role_transition: "Sua conta não pode realizar esta etapa.",
  transition_invalid: "O pedido mudou de etapa. Atualize a lista.",
  setup_token_invalid:
    "O código de configuração é inválido. Abra a instalação pelo aplicativo.",
  setup_complete: "A instalação já foi configurada. Entre com sua conta.",
  user_input_invalid:
    "Confira os dados. A senha deve ter entre 12 e 72 caracteres.",
  password_invalid: "A senha deve ter entre 12 e 72 caracteres.",
  restore_confirmation_required: "Selecione o arquivo e digite RESTAURAR.",
  method_not_allowed: "Esta ação ainda não está disponível nesta versão.",
};
function readableError(error) {
  const raw = error?.message || String(error);
  return (
    errors[raw] ||
    (raw === "Failed to fetch"
      ? "Não foi possível conectar ao aplicativo. Confira se ele está aberto."
      : raw.replaceAll("_", " "))
  );
}
function msg(text, type = "success") {
  clearTimeout(messageTimer);
  if (type !== "error") messageTimer = setTimeout(clearMsg, 6500);
  $("messageText").textContent = text;
  $("message").classList.remove("hidden");
  $("message").classList.toggle("error", type === "error");
  const dialog = document.querySelector("dialog[open]");
  if (dialog && type === "error") {
    let inline = dialog.querySelector(".dialog-message");
    if (!inline) {
      inline = document.createElement("p");
      inline.className = "dialog-message";
      inline.setAttribute("role", "alert");
      dialog.querySelector(".dialog-body").append(inline);
    }
    inline.textContent = text;
    inline.classList.remove("hidden");
  }
}
function clearMsg() {
  $("message").classList.add("hidden");
  $$(".dialog-message").forEach((m) => m.classList.add("hidden"));
}
$("dismissMessage").onclick = clearMsg;
function show(id) {
  for (const x of [
    "setup",
    "login",
    "workspace",
    "modeChooser",
    "posWorkspace",
  ])
    $(x).classList.toggle("hidden", x !== id);
  document.body.classList.toggle("auth-page", ["setup", "login"].includes(id));
  document.body.classList.toggle("mode-page", id === "modeChooser");
  document.body.classList.toggle("pos-page", id === "posWorkspace");
  if (typeof resetModes === "function" && ["setup", "login"].includes(id))
    resetModes();
  if (["setup", "login"].includes(id)) {
    document.querySelectorAll("dialog[open]").forEach((d) => d.close());
    closeNavigation();
    currentStoreId = null;
  }
}
function td(row, text) {
  const cell = document.createElement("td");
  const badge =
    typeof text === "string" &&
    text.match(/^<span class="badge (badge-[a-z_-]+)">([^<>]*)<\/span>$/);
  if (badge) {
    const span = document.createElement("span");
    span.className = "badge " + badge[1];
    span.textContent = badge[2];
    cell.append(span);
  } else cell.textContent = text ?? "—";
  row.append(cell);
  return cell;
}
function setOptions(select, items, placeholder, format = (x) => x.name) {
  const old = select.value;
  select.replaceChildren();
  if (placeholder !== null) select.append(new Option(placeholder, ""));
  for (const item of items)
    select.append(new Option(format(item), String(item.id)));
  if ([...select.options].some((o) => o.value === old)) select.value = old;
}
function setTheme(mode) {
  const value = ["light", "dark", "system"].includes(mode) ? mode : "system";
  document.documentElement.dataset.theme =
    value === "system"
      ? matchMedia("(prefers-color-scheme: dark)").matches
        ? "dark"
        : "light"
      : value;
  $("themeSelect").value = value;
  try {
    localStorage.setItem("openfood-theme", value);
  } catch {}
}
let savedTheme = "system";
try {
  savedTheme = localStorage.getItem("openfood-theme") || "system";
} catch {}
setTheme(savedTheme);
$("themeSelect").onchange = (e) => setTheme(e.target.value);
matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => {
  if ($("themeSelect").value === "system") setTheme("system");
});
function closeNavigation() {
  document.body.classList.remove("menu-open");
  $("menuToggle").setAttribute("aria-expanded", "false");
  $("mainContent").inert = false;
  $(".app-header").inert = false;
}
$("menuToggle").onclick = () => {
  if (document.body.classList.contains("menu-open")) {
    closeNavigation();
    return;
  }
  document.body.classList.add("menu-open");
  $("menuToggle").setAttribute("aria-expanded", "true");
  $("mainContent").inert = true;
  $(".app-header").inert = true;
  $("#sidebar .tab.active")?.focus();
};
$("closeMenu").onclick = () => {
  closeNavigation();
  $("menuToggle").focus();
};
window.addEventListener("resize", () => {
  if (innerWidth > 800) closeNavigation();
});
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape" && document.body.classList.contains("menu-open")) {
    closeNavigation();
    $("menuToggle").focus();
  }
});
const pageInfo = {
  dashboard: [
    "Gestão da loja",
    "Dashboard administrativo",
    "Visão das vendas, recebimentos e operação.",
    null,
  ],
  operacao: [
    "Visão da operação",
    "Pedidos",
    "Acompanhe o preparo. Organize cada entrega.",
    "Novo pedido",
  ],
  catalogo: [
    "Produtos e estoque",
    "Catálogo",
    "Tudo o que sua loja vende, em um só lugar.",
    "Novo produto",
  ],
  clientes: [
    "Relacionamento e privacidade",
    "Clientes",
    "Organize contatos e acompanhe consentimentos.",
    "Novo cliente",
  ],
  usuarios: [
    "Pessoas e permissões",
    "Equipe",
    "Cada pessoa com o acesso certo à operação.",
    "Adicionar pessoa",
  ],
  lojas: [
    "Sua organização",
    "Lojas",
    "Gerencie as lojas desta instalação.",
    "Nova loja",
  ],
  pagamentos: [
    "Financeiro",
    "Pagamentos",
    "Receba pedidos e confira o caixa da loja.",
    null,
  ],
  manutencao: [
    "Configurações da instalação",
    "Sistema",
    "Cuide dos dados e da continuidade da operação.",
    null,
  ],
};
function selectPage(name, focus = false) {
  const info = pageInfo[name];
  if (!info) return;
  $$(".tab").forEach((b) => {
    const selected = b.dataset.tab === name;
    b.classList.toggle("active", selected);
    b.setAttribute("aria-current", selected ? "page" : "false");
  });
  $$(".tab-panel").forEach((p) => p.classList.remove("active"));
  $("panel" + name[0].toUpperCase() + name.slice(1)).classList.add("active");
  ["pageEyebrow", "pageTitle", "pageDescription"].forEach(
    (id, i) => ($(id).textContent = info[i]),
  );
  const allowed =
    name === "operacao"
      ? canCreateOrders()
      : name === "catalogo"
        ? canManageCatalog()
        : name === "clientes" || name === "usuarios"
          ? canManageUsers()
          : name === "lojas"
            ? canManageStores()
            : false;
  $("pageAction").classList.toggle("hidden", !info[3] || !allowed);
  $("pageAction").querySelector("span").textContent = info[3] || "";
  $("pageAction").onclick = () => {
    if (name === "operacao") openOrderDialog();
    else if (name === "catalogo") catalogPrimary();
    else {
      if (name === "clientes") $("subtabClientes").click();
      const form = $(
        name === "clientes"
          ? "customerForm"
          : name === "usuarios"
            ? "addUserForm"
            : "addStoreForm",
      );
      const disclosure = form.closest("details");
      if (disclosure) disclosure.open = true;
      form.scrollIntoView({ block: "center", behavior: "smooth" });
      form.querySelector("input:not([type=hidden])")?.focus();
    }
  };
  closeNavigation();
  if (focus) {
    $("pageTitle").tabIndex = -1;
    $("pageTitle").focus({ preventScroll: true });
  }
  if (name === "catalogo") {
    loadCatalog();
    updateCatalogPrimary();
  }
  if (name === "clientes") loadCustomers();
  if (name === "usuarios") loadUsers();
  if (name === "lojas") loadStores();
  if (name === "pagamentos") run(loadCash);
  if (name === "dashboard" && typeof loadDashboard === "function")
    run(loadDashboard);
  document.title = info[1] + " · OpenFood";
}
$$(".tab").forEach((b) => (b.onclick = () => selectPage(b.dataset.tab, true)));
$$(".sub-tab").forEach((b) => {
  b.setAttribute("aria-pressed", String(b.classList.contains("active")));
  b.onclick = () => {
    const panel = b.closest(".tab-panel");
    panel.querySelectorAll(".sub-tab").forEach((t) => {
      t.classList.toggle("active", t === b);
      t.setAttribute("aria-pressed", String(t === b));
    });
    panel
      .querySelectorAll(".sub-tab-panel")
      .forEach((p) => p.classList.remove("active"));
    $(
      b.dataset.subtab === "lgpd"
        ? "subpanelLGPD"
        : "subpanel" +
            b.dataset.subtab[0].toUpperCase() +
            b.dataset.subtab.slice(1),
    ).classList.add("active");
    if (panel.id === "panelCatalogo") updateCatalogPrimary();
  };
});
const catalogActions = {
  produtos: ["Novo produto", null],
  categorias: ["Nova categoria", "categoryForm"],
  variantes: ["Nova variante", "variantForm"],
  adicionais: ["Novo grupo", "addonGroupForm"],
  combos: ["Novo combo", "comboForm"],
};
function updateCatalogPrimary() {
  const sub = $("#panelCatalogo .sub-tab.active")?.dataset.subtab || "produtos";
  $("pageAction").querySelector("span").textContent = catalogActions[sub][0];
}
function catalogPrimary() {
  const sub = $("#panelCatalogo .sub-tab.active")?.dataset.subtab || "produtos";
  const id = catalogActions[sub][1];
  if (!id) {
    openProductDialog();
    return;
  }
  const form = $(id);
  const disclosure = form.closest("details");
  if (disclosure) disclosure.open = true;
  form.scrollIntoView({ block: "center", behavior: "smooth" });
  form.querySelector("input:not([type=hidden])")?.focus();
}
for (const [name, formId] of [
  ["editCategory", "categoryForm"],
  ["editCustomer", "customerForm"],
]) {
  const original = window[name];
  window[name] = function (value) {
    const form = $(formId);
    const disclosure = form.closest("details");
    if (disclosure) disclosure.open = true;
    original(value);
    form.querySelector("input:not([type=hidden])")?.focus();
  };
}
function statusBadge(state, label) {
  const span = document.createElement("span");
  span.className = "badge badge-" + state;
  span.textContent = label || stateLabels[state] || state;
  return span;
}
function canTransition(state) {
  if (!currentUser) return false;
  const role = currentUser.role;
  if (["instance_admin", "org_admin", "store_manager"].includes(role))
    return true;
  return role === "attendant"
    ? ["preparing", "cancelled"].includes(state)
    : role === "kitchen"
      ? ["preparing", "ready"].includes(state)
      : role === "dispatch"
        ? state === "completed"
        : false;
}
function renderOrders() {
  const search = $("orderSearch").value.trim().replace(/^#/, "");
  const list = latestOrders.filter(
    (o) =>
      (orderFilter === "all" ||
        (orderFilter === "active"
          ? activeStates.includes(o.state)
          : o.state === orderFilter)) &&
      String(o.id).includes(search),
  );
  $("orders").replaceChildren();
  for (const o of list) {
    const row = document.createElement("tr");
    let cell = td(row, "");
    const number = document.createElement("button");
    number.className = "order-number order-reference";
    number.textContent = "#" + String(o.id).padStart(4, "0");
    number.setAttribute("aria-label", "Ver resumo do pedido " + o.id);
    number.onclick = () => openOrderDetails(o);
    const time = document.createElement("div");
    time.className = "order-time";
    time.textContent = new Date(o.created_at).toLocaleString("pt-BR", {
      day: "2-digit",
      month: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
    });
    cell.append(number, time);
    td(row, money(o.total_cents)).className = "order-amount";
    cell = td(row, "");
    cell.append(statusBadge(o.state));
    cell = td(row, "");
    cell.classList.toggle("hidden", !canViewFinancials());
    if (canViewFinancials())
      cell.append(
        statusBadge(
          "financial",
          financialLabels[o.financial_state] || "Pagamento pendente",
        ),
      );
    const actions = td(row, "");
    const next = {
      confirmed: "preparing",
      preparing: "ready",
      ready: "completed",
    }[o.state];
    if (next && canTransition(next)) {
      const btn = document.createElement("button");
      btn.textContent = stateLabels[next];
      btn.setAttribute("aria-label", stateLabels[next] + " — pedido " + o.id);
      btn.onclick = () =>
        run(async () => {
          await api("order-state", { id: o.id, state: next });
          await refresh();
          msg("Pedido #" + o.id + ": " + stateLabels[next].toLowerCase() + ".");
        }, btn);
      actions.append(btn);
    }
    if (o.state === "confirmed" && canTransition("cancelled")) {
      const btn = document.createElement("button");
      btn.className = "btn-cancel";
      btn.textContent = "Cancelar";
      btn.setAttribute("aria-label", "Cancelar pedido " + o.id);
      btn.onclick = () => {
        if (
          confirm("Cancelar o pedido #" + o.id + "? O estoque será devolvido.")
        )
          run(async () => {
            await api("order-state", { id: o.id, state: "cancelled" });
            await refresh();
            msg("Pedido #" + o.id + " cancelado.");
          }, btn);
      };
      actions.append(btn);
    }
    if (!actions.children.length)
      actions.textContent = activeStates.includes(o.state)
        ? "Sem ação para seu papel"
        : "—";
    $("orders").append(row);
  }
  $("ordersEmptyState").classList.toggle("hidden", list.length > 0);
  $("ordersTable").classList.toggle("hidden", !list.length);
  const noData = !latestOrders.length,
    filtered = !!search || orderFilter !== "active";
  $("emptyOrdersTitle").textContent = noData
    ? "Tudo pronto para o primeiro pedido"
    : filtered
      ? "Nenhum pedido nesta seleção"
      : "Sua fila está em dia";
  $("emptyOrders").textContent = noData
    ? latestProducts.length
      ? "Crie um pedido manual para começar a acompanhar o preparo."
      : "Cadastre um produto no catálogo e comece sua operação."
    : filtered
      ? "Tente outro número ou veja todos os pedidos."
      : "Os pedidos finalizados continuam disponíveis no filtro Todos.";
  const action = $("emptyOrdersAction");
  action.classList.toggle(
    "hidden",
    noData && !canCreateOrders() && !canManageCatalog(),
  );
  action.textContent = noData
    ? latestProducts.length
      ? "Criar primeiro pedido"
      : "Ir para o catálogo"
    : "Ver todos os pedidos";
  action.onclick = () => {
    if (noData) {
      if (latestProducts.length) openOrderDialog();
      else selectPage("catalogo");
    } else {
      $("orderSearch").value = "";
      setOrderFilter("all");
    }
  };
  $("orderResultCount").textContent =
    list.length + " pedido" + (list.length !== 1 ? "s" : "") + " nesta seleção";
  ["confirmed", "preparing", "ready"].forEach(
    (state) =>
      ($("metric" + state[0].toUpperCase() + state.slice(1)).textContent =
        latestOrders.filter((o) => o.state === state).length),
  );
  const count = latestOrders.filter((o) =>
    activeStates.includes(o.state),
  ).length;
  $("countActive").textContent = count;
  $(".nav-count").textContent = count;
  $("metricProducts").textContent = latestProducts.filter(
    (p) => p.active,
  ).length;
  const emptyStock = latestProducts.filter(
    (p) => p.active && p.stock_quantity <= 0,
  ).length;
  $("metricProductsNote").textContent = emptyStock
    ? emptyStock + " sem estoque"
    : "No catálogo desta loja";
}
function setOrderFilter(value) {
  orderFilter = value;
  $$("[data-order-filter]").forEach((b) => {
    const active = b.dataset.orderFilter === value;
    b.classList.toggle("active", active);
    b.setAttribute("aria-pressed", String(active));
  });
  renderOrders();
}
async function openOrderDetails(order) {
  let dialog = $("orderDetailsDialog");
  if (!dialog) {
    dialog = document.createElement("dialog");
    dialog.id = "orderDetailsDialog";
    dialog.className = "dialog";
    dialog.setAttribute("aria-labelledby", "orderDetailsTitle");
    dialog.innerHTML =
      '<div class="dialog-header"><h2 id="orderDetailsTitle"></h2><button class="icon-button" aria-label="Fechar resumo">✕</button></div><div class="dialog-body"><div class="order-preview"><div><span>Total do pedido</span><strong id="orderDetailsAmount"></strong></div></div><div id="orderDetailsState"></div><p id="orderDetailsFinancial" class="muted" style="margin-top:14px"></p><small id="orderDetailsDate"></small></div>';
    dialog.querySelector("button").onclick = () => dialog.close();
    document.body.append(dialog);
  }
  $("orderDetailsTitle").textContent =
    "Pedido #" + String(order.id).padStart(4, "0");
  $("orderDetailsAmount").textContent = money(order.total_cents);
  $("orderDetailsState").replaceChildren(statusBadge(order.state));
  $("orderDetailsFinancial").textContent = canViewFinancials()
    ? financialLabels[order.financial_state] || "Pagamento pendente"
    : "";
  $("orderDetailsDate").textContent =
    "Criado em " + new Date(order.created_at).toLocaleString("pt-BR");
  let list = $("orderDetailItems");
  if (!list) {
    list = document.createElement("div");
    list.id = "orderDetailItems";
    $("orderDetailsDate").before(list);
  }
  list.textContent = "Carregando itens…";
  try {
    const items = await api("orders?id=" + order.id);
    list.replaceChildren();
    for (const item of items) {
      const row = document.createElement("p");
      row.textContent =
        item.quantity +
        " × " +
        item.name +
        " · " +
        money(item.price_cents) +
        " = " +
        money(item.price_cents * item.quantity);
      list.append(row);
    }
    if (order.discount_cents) {
      const row = document.createElement("p");
      row.textContent = "Desconto aplicado: −" + money(order.discount_cents);
      list.append(row);
    }
  } catch (e) {
    list.textContent = readableError(e);
  }

  dialog.showModal();
}
$$("[data-order-filter]").forEach(
  (b) => (b.onclick = () => setOrderFilter(b.dataset.orderFilter)),
);
$("orderSearch").oninput = renderOrders;
function refresh() {
  if (refreshPromise) return refreshPromise;
  const generation = refreshGeneration,
    store = currentStoreId;
  document.body.classList.add("busy");
  refreshPromise = (async () => {
    const [me, status, products, orders] = await Promise.all([
      api("me"),
      api("status"),
      api("products"),
      api("orders"),
    ]);
    if (generation !== refreshGeneration) return;
    currentUser = me;
    currentStoreId = status.store_id;
    latestOrders = orders;
    latestProducts = products;
    allProducts = products;
    $("storeTitle").textContent = status.store;
    $("health").textContent = "Aplicativo conectado";
    $("accountEmail").textContent = me.email;
    $("accountEmail").title = me.email;
    $("queue").textContent =
      `${status.pending_jobs} tarefas pendentes · ${status.failed_jobs} falhas · Versão ${status.version}`;
    $("roleIndicator").className = "badge badge-role-" + me.role;
    $("roleIndicator").textContent = roleLabels[me.role] || me.role;
    const stores = await api("stores");
    if (generation !== refreshGeneration) return;
    latestStores = stores;
    setOptions($("storeSelector"), stores, null);
    $("storeSelector").value = String(currentStoreId);
    $("storeSelector").classList.toggle("hidden", !canManageStores());
    $(".store-context").classList.toggle(
      "has-store-selector",
      canManageStores(),
    );
    setOptions($("newUserStore"), stores, "Sem loja específica");
    const roles = $("#addUserForm select[name=role]");
    [...roles.options].forEach(
      (o) =>
        (o.disabled =
          me.role === "store_manager"
            ? ["instance_admin", "org_admin", "store_manager"].includes(o.value)
            : me.role === "org_admin"
              ? o.value === "instance_admin"
              : false),
    );
    roles.onchange = () => {
      $("newUserStore").required = !["instance_admin", "org_admin"].includes(
        roles.value,
      );
    };
    roles.onchange();
    const eligible = products.filter(
      (p) =>
        p.active &&
        p.stock_quantity >= 1 &&
        !p.is_weight_based &&
        ["unidade", "porcao"].includes(p.unit),
    );
    setOptions(
      $("productSelect"),
      eligible,
      eligible.length ? "Selecione um produto" : "Nenhum produto com estoque",
      (p) => p.name + " · " + money(p.price_cents),
    );
    $("createOrderSubmit").disabled = !canCreateOrders() || !eligible.length;
    updateOrderPreview();
    [
      ["tabUsuarios", canManageUsers()],
      ["tabLojas", canManageStores()],
      ["tabCatalogo", canManageCatalog()],
      ["tabClientes", canAccessCustomers()],
      ["tabPagamentos", canAccessPayments()],
      ["tabManutencao", me.role === "instance_admin"],
    ].forEach(([id, allow]) => $(id).classList.toggle("hidden", !allow));
    $("finCol").classList.toggle("hidden", !canViewFinancials());
    if (
      $$(".tab")
        .find((b) => b.classList.contains("active"))
        ?.classList.contains("hidden")
    )
      selectPage("operacao");
    const currentPage =
      $$(".tab").find((b) => b.classList.contains("active"))?.dataset.tab ||
      "operacao";
    const primaryAllowed =
      currentPage === "operacao"
        ? canCreateOrders()
        : currentPage === "catalogo"
          ? canManageCatalog()
          : ["clientes", "usuarios"].includes(currentPage)
            ? canManageUsers()
            : currentPage === "lojas"
              ? canManageStores()
              : false;
    $("pageAction").classList.toggle(
      "hidden",
      !primaryAllowed || !pageInfo[currentPage][3],
    );
    lastRefresh = new Date();
    $("lastUpdated").textContent =
      "Atualizado às " +
      lastRefresh.toLocaleTimeString("pt-BR", {
        hour: "2-digit",
        minute: "2-digit",
      }) +
      " · até 200 pedidos";
    renderOrders();
    if (currentPage === "pagamentos") await loadCash();
    $("connectionNotice").classList.add("hidden");
    if (typeof displayCurrentMode === "function") displayCurrentMode();
    else show("workspace");
  })()
    .catch((e) => {
      if (!document.body.classList.contains("auth-page")) {
        $("connectionNotice").classList.remove("hidden");
        $("health").textContent = "Sem conexão";
      }
      throw e;
    })
    .finally(() => {
      refreshPromise = null;
      document.body.classList.remove("busy");
    });
  return refreshPromise;
}
$("storeSelector").onchange = async () => {
  $("storeSelector").disabled = true;
  if (refreshPromise)
    try {
      await refreshPromise;
    } catch {}
  currentStoreId = Number($("storeSelector").value);
  refreshGeneration++;
  latestOrders = [];
  latestProducts = [];
  clearMsg();
  document.querySelectorAll("dialog[open]").forEach((d) => d.close());
  resetAdvProductForm();
  $("orderForm").reset();
  $("orderSearch").value = "";
  renderOrders();
  try {
    await refresh();
    const tab = $$(".tab.active")[0]?.dataset.tab;
    selectPage(tab || "operacao");
  } catch (e) {
    msg(readableError(e), "error");
  } finally {
    $("storeSelector").disabled = false;
  }
};
function openOrderDialog() {
  clearMsg();
  if (!canCreateOrders()) return;
  cartItems = [];
  orderKey = crypto.randomUUID();
  renderCart();
  $("orderDialog").showModal();
  updateOrderPreview();
}
function openProductDialog() {
  if (!canManageCatalog()) return;
  clearMsg();
  resetAdvProductForm();
  editingProductStock = null;
  $("productDialogTitle").textContent = "Novo produto";
  $("productDialog").showModal();
}
$$("[data-close-dialog]").forEach(
  (b) => (b.onclick = () => b.closest("dialog").close()),
);
function updateOrderPreview() {
  const product = latestProducts.find(
    (p) => String(p.id) === $("productSelect").value,
  );
  const qty = Number($("#orderForm input[name=quantity]").value) || 0;
  $("orderPreviewTotal").textContent = money(
    cartItems.reduce((sum, item) => sum + item.price_cents * item.quantity, 0) +
      (product?.price_cents || 0) * qty,
  );
}
$("orderForm").oninput = updateOrderPreview;
$("orderForm").onchange = updateOrderPreview;
$("orderForm").onsubmit = (e) => {
  e.preventDefault();
  if (!canCreateOrders()) return;
  const data = new FormData(e.target);
  const product = Number(data.get("product")),
    quantity = Number(data.get("quantity"));
  if (
    (!product && !cartItems.length) ||
    (product && (!Number.isInteger(quantity) || quantity < 1))
  ) {
    msg("Escolha um produto e uma quantidade inteira válida.", "error");
    return;
  }
  run(async () => {
    const result = await api(
      "orders",
      { items: orderItems(product, quantity) },
      { "Idempotency-Key": orderKey },
    );
    orderKey = crypto.randomUUID();
    $("orderDialog").close();
    e.target.reset();
    cartItems = [];
    renderCart();
    await refresh();
    setOrderFilter("active");
    msg("Pedido #" + result.id + " criado. Pagamento pendente.");
  }, $("createOrderSubmit"));
};
$("advProductForm").onsubmit = (e) => {
  e.preventDefault();
  if (!canManageCatalog()) return;
  const d = new FormData(e.target);
  const payload = {
    name: d.get("name").trim(),
    description: d.get("description") || "",
    price_cents: parsePrice(d.get("price")),
    stock_quantity: Number(d.get("stock")),
    unit: d.get("unit"),
    sku: d.get("sku") || "",
    barcode: d.get("barcode") || "",
    min_order_quantity: Number(d.get("min_qty")) || 1,
    max_order_quantity: Number(d.get("max_qty")) || 0,
    preparation_time_minutes: Number(d.get("prep_time")) || 0,
    display_order: Number(d.get("display_order")) || 0,
    active: d.get("active") === "on",
    is_weight_based: d.get("weight_based") === "on",
    substitution_allowed: d.get("substitution_allowed") === "on",
    available_from: d.get("available_from") || "",
    available_until: d.get("available_until") || "",
    category_id: d.get("category_id") ? Number(d.get("category_id")) : null,
  };
  run(async () => {
    const id = Number(d.get("id"));
    if (id)
      await apiPatch("products", {
        ...payload,
        id,
        expected_stock_quantity: editingProductStock,
      });
    else await api("products", payload);
    $("productDialog").close();
    resetAdvProductForm();
    await refresh();
    await loadCatalog();
    msg("Produto cadastrado no catálogo da loja.");
  }, $("advProductSubmit"));
};
async function renderAdvancedProducts() {
  const query = $("productSearch").value.trim().toLocaleLowerCase("pt-BR"),
    availability = $("productAvailability").value;
  const products = allProducts.filter(
    (p) =>
      (!query ||
        [p.name, p.sku, p.barcode].some((v) =>
          String(v || "")
            .toLocaleLowerCase("pt-BR")
            .includes(query),
        )) &&
      (availability === "all" ||
        (availability === "available"
          ? p.active && p.stock_quantity > 0
          : !p.active || p.stock_quantity <= 0)),
  );
  const tbody = $("advProducts");
  tbody.replaceChildren();
  for (const p of products) {
    const row = document.createElement("tr");
    td(row, "#" + p.id);
    const name = td(row, "");
    const symbol = document.createElement("span");
    symbol.className = "product-symbol";
    symbol.innerHTML = svgIcon("catalog");
    const title = document.createElement("div");
    const titleText = document.createElement("span");
    titleText.textContent = p.name;
    title.append(titleText);
    title.className = "product-name";
    const sku = document.createElement("small");
    sku.textContent = p.sku || "Sem SKU";
    title.append(sku);
    name.append(symbol, title);
    td(
      row,
      allCategories.find((c) => c.id === p.category_id)?.name ||
        "Sem categoria",
    );
    td(row, money(p.price_cents));
    td(row, formatQty(p.stock_quantity, p.unit));
    td(row, p.unit);
    td(row, p.sku || "—");
    td(row, p.barcode || "—");
    td(row, p.is_weight_based ? "Sim" : "Não");
    td(row, p.substitution_allowed ? "Sim" : "Não");
    const status = td(row, "");
    status.append(
      statusBadge(
        p.active && p.stock_quantity > 0 ? "active" : "inactive",
        p.active
          ? p.stock_quantity > 0
            ? "Disponível"
            : "Sem estoque"
          : "Inativo",
      ),
    );
    const action = td(row, "");
    const button = document.createElement("button");
    button.textContent = "Detalhes";
    button.className = "btn-secondary";
    button.onclick = () => openProductDetails(p);
    action.append(button);
    if (canManageCatalog()) {
      const edit = document.createElement("button");
      edit.textContent = "Editar";
      edit.className = "btn-secondary";
      edit.onclick = () => editProduct(p);
      action.append(edit);
    }
    tbody.append(row);
  }
  $("catalogCount").textContent =
    products.length + " de " + allProducts.length + " produtos";
  $("catalogEmptyState").classList.toggle("hidden", !!products.length);
  $("advProducts")
    .closest(".scroll")
    .classList.toggle("hidden", !products.length);
  $("catalogEmptyAction").textContent = allProducts.length
    ? "Limpar filtros"
    : "Cadastrar produto";
  $("catalogEmptyAction").onclick = () => {
    if (allProducts.length) {
      $("productSearch").value = "";
      $("productAvailability").value = "all";
      renderAdvancedProducts();
    } else openProductDialog();
  };
}
$("productSearch").oninput = renderAdvancedProducts;
$("productAvailability").onchange = renderAdvancedProducts;
function openProductDetails(product) {
  let dialog = $("productDetailsDialog");
  if (!dialog) {
    dialog = document.createElement("dialog");
    dialog.id = "productDetailsDialog";
    dialog.className = "dialog";
    dialog.setAttribute("aria-labelledby", "productDetailsTitle");
    dialog.innerHTML =
      '<div class="dialog-header"><h2 id="productDetailsTitle"></h2><button type="button" class="icon-button" aria-label="Fechar detalhes">✕</button></div><div class="dialog-body"><p id="productDetailDescription" class="detail-description"></p><dl id="productDefinition" class="definition-list"></dl></div>';
    dialog.querySelector("button").onclick = () => dialog.close();
    document.body.append(dialog);
  }
  $("productDetailsTitle").textContent = product.name;
  $("productDetailDescription").textContent =
    product.description || "Sem descrição";
  const definition = $("productDefinition");
  definition.replaceChildren();
  for (const [label, value] of [
    ["Preço", money(product.price_cents)],
    ["Estoque", formatQty(product.stock_quantity, product.unit)],
    ["SKU", product.sku || "Sem SKU"],
    ["Código de barras", product.barcode || "Não informado"],
    ["Preparo", product.preparation_time_minutes + " min"],
    ["Venda mínima", product.min_order_quantity + " " + product.unit],
    ["Disponibilidade", product.active ? "Ativo" : "Inativo"],
    ["Venda por peso", product.is_weight_based ? "Sim" : "Não"],
  ]) {
    const item = document.createElement("div"),
      dt = document.createElement("dt"),
      dd = document.createElement("dd");
    dt.textContent = label;
    dd.textContent = value;
    item.append(dt, dd);
    definition.append(item);
  }
  dialog.showModal();
}
$("paymentsBack").onclick = () => selectPage("operacao");
async function run(fn, trigger) {
  if (trigger?.disabled) return;
  const form =
    trigger?.closest("form") || document.activeElement?.closest("form");
  const controls = form
    ? [...form.querySelectorAll("button")]
    : trigger
      ? [trigger]
      : [];
  const previous = controls.map((b) => b.disabled);
  controls.forEach((b) => (b.disabled = true));
  if (form) form.setAttribute("aria-busy", "true");
  try {
    await fn();
  } catch (e) {
    msg("Não foi possível concluir: " + readableError(e), "error");
  } finally {
    controls.forEach((b, i) => (b.disabled = previous[i]));
    if (form) form.removeAttribute("aria-busy");
  }
}
$("refresh").onclick = () => run(refresh, $("refresh"));
$("retryConnection").onclick = () => run(refresh, $("retryConnection"));
$("logout").onclick = () =>
  run(async () => {
    await api("logout", {});
    currentUser = null;
    latestOrders = [];
    latestProducts = [];
    clearMsg();
    show("login");
  }, $("logout"));
$("setupForm").onsubmit = (e) => {
  e.preventDefault();
  run(async () => {
    await api("setup", Object.fromEntries(new FormData(e.target)));
    setupToken = "";
    e.target.reset();
    show("login");
    msg("Sua operação foi configurada. Entre com a conta criada.");
  }, e.target.querySelector("button"));
};
$("loginForm").onsubmit = (e) => {
  e.preventDefault();
  run(async () => {
    await api("login", Object.fromEntries(new FormData(e.target)));
    e.target.reset();
    await refresh();
    if (typeof chooseMode === "function") chooseMode();
    else selectPage("operacao");
    clearMsg();
  }, e.target.querySelector("button"));
};
$("addUserForm").onsubmit = (e) => {
  e.preventDefault();
  const d = new FormData(e.target),
    store = Number(d.get("store_id"));
  const payload = {
    email: d.get("email"),
    password: d.get("password"),
    role: d.get("role"),
  };
  if (store) {
    payload.store_id = store;
    if (currentUser.role === "instance_admin")
      payload.org_id = latestStores.find((s) => s.id === store)?.org_id;
  }
  run(async () => {
    await api("users", payload);
    e.target.reset();
    $("#addUserForm select[name=role]").onchange();
    await loadUsers();
    msg("Pessoa adicionada à equipe.");
  }, e.target.querySelector("button"));
};
// Empty states remain meaningful for every administrative table.
const tableObserver = new MutationObserver(() => {
  for (const tbody of $$(
    "tbody:not(#orders):not(#advProducts):not(#products)",
  )) {
    if (!tbody.children.length) {
      const row = document.createElement("tr");
      const cell = document.createElement("td");
      cell.colSpan = tbody.closest("table").querySelectorAll("th").length;
      cell.className = "muted";
      cell.textContent =
        "Nenhum registro nesta loja. Use o cadastro abaixo para começar.";
      row.dataset.empty = "true";
      row.append(cell);
      tbody.append(row);
    }
  }
});
$$("tbody:not(#orders):not(#advProducts):not(#products)").forEach((t) =>
  tableObserver.observe(t, { childList: true }),
);
selectPage("operacao");
(async () => {
  try {
    const state = await api("setup");
    if (state.required) {
      show("setup");
      $("setupForm").elements.token.value = setupToken;
      $("tokenLabel").classList.toggle("hidden", !!setupToken);
      $("health").textContent = "Configuração necessária";
    } else await refresh();
  } catch (e) {
    if (["login_required", "session_invalid"].includes(e.message))
      show("login");
    else {
      show("login");
      msg(readableError(e), "error");
    }
  }
})();
setInterval(() => {
  if (
    !document.hidden &&
    !document.body.classList.contains("auth-page") &&
    !document.querySelector("dialog[open]")
  )
    refresh().catch((e) => {
      if (!["login_required", "session_invalid"].includes(e.message))
        msg(readableError(e), "error");
    });
}, 15000);

function editProduct(product) {
  clearMsg();
  resetAdvProductForm();
  editingProductStock = product.stock_quantity;
  const f = $("advProductForm");
  const values = {
    id: product.id,
    name: product.name,
    description: product.description,
    price: (product.price_cents / 100).toFixed(2).replace(".", ","),
    stock: product.stock_quantity,
    unit: product.unit,
    sku: product.sku,
    barcode: product.barcode,
    min_qty: product.min_order_quantity || 1,
    max_qty: product.max_order_quantity || "",
    prep_time: product.preparation_time_minutes,
    display_order: product.display_order,
    category_id: product.category_id || "",
    available_from: (product.available_from || "").slice(0, 5),
    available_until: (product.available_until || "").slice(0, 5),
  };
  for (const [key, value] of Object.entries(values))
    f.elements.namedItem(key).value = value;
  for (const [key, value] of Object.entries({
    active: product.active,
    weight_based: product.is_weight_based,
    substitution_allowed: product.substitution_allowed,
  }))
    f.elements.namedItem(key).checked = value;
  $("productDialogTitle").textContent = "Editar produto";
  $("advProductSubmit").textContent = "Salvar alterações";
  $("productDialog").showModal();
}
function orderItems(product, quantity) {
  const items = cartItems.map((i) => ({
    product_id: i.product_id,
    quantity: i.quantity,
  }));
  if (product) {
    const existing = items.find((i) => i.product_id === product);
    if (existing) existing.quantity += quantity;
    else items.push({ product_id: product, quantity });
  }
  return items;
}
function renderCart() {
  const list = $("orderCart");
  list.replaceChildren();
  for (const item of cartItems) {
    const row = document.createElement("div");
    row.className = "cart-row";
    const text = document.createElement("span");
    text.textContent =
      item.quantity +
      " × " +
      item.name +
      " · " +
      money(item.price_cents * item.quantity);
    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "btn-secondary";
    remove.textContent = "Remover";
    remove.setAttribute("aria-label", "Remover " + item.name);
    remove.onclick = () => {
      cartItems = cartItems.filter((i) => i !== item);
      renderCart();
      updateOrderPreview();
    };
    row.append(text, remove);
    list.append(row);
  }
  $("productSelect").required = !cartItems.length;
}
$("addOrderItem").onclick = () => {
  const product = latestProducts.find(
      (p) => String(p.id) === $("productSelect").value,
    ),
    quantity = Number($("#orderForm input[name=quantity]").value);
  if (!product || !Number.isInteger(quantity) || quantity < 1) {
    msg("Escolha um produto e uma quantidade inteira válida.", "error");
    return;
  }
  const existing = cartItems.find((i) => i.product_id === product.id);
  if (existing) existing.quantity += quantity;
  else
    cartItems.push({
      product_id: product.id,
      name: product.name,
      price_cents: product.price_cents,
      quantity,
    });
  $("productSelect").value = "";
  $("#orderForm input[name=quantity]").value = "1";
  renderCart();
  updateOrderPreview();
};
