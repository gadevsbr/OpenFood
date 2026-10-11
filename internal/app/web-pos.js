let appMode = null,
  posCart = [],
  posSummaryData = null,
  posCategory = "",
  posBusy = false,
  posLoadedStore = null,
  posAttempt = null;
const posKey = (s) => `openfood:pos:${currentUser?.id}:${currentStoreId}:${s}`;
function posRead(s, f) {
  try {
    return JSON.parse(sessionStorage.getItem(posKey(s))) || f;
  } catch {
    return f;
  }
}
function posSave() {
  sessionStorage.setItem(
    posKey("cart"),
    JSON.stringify({
      items: posCart,
      service: $("posService").value,
      discount: $("posDiscount").value,
      note: $("posDiscountNote").value,
    }),
  );
}
function posAnimate(el) {
  if (
    !matchMedia("(prefers-reduced-motion: reduce)").matches &&
    window.OpenFoodMotion
  )
    try {
      OpenFoodMotion.animate(el, { opacity: [0, 1] }, { duration: 0.22 });
    } catch {}
}
function resetModes() {
  appMode = null;
  posCart = [];
  posAttempt = null;
  posLoadedStore = null;
  document.body.classList.remove("pos-cart-open");
}
function chooseMode() {
  window.scrollTo(0, 0);
  appMode = null;
  show("modeChooser");
  $("modeAccount").textContent = currentUser?.email || "";
  $("modeRole").textContent = roleLabels[currentUser?.role] || "";
  $("enterPOS").disabled = !canCreateOrders();
  posAnimate($("modeChooser"));
}
function displayCurrentMode() {
  if (!currentUser) return;
  $("adminToPOS").classList.toggle("hidden", !canCreateOrders());
  if (appMode === "pos") {
    show("posWorkspace");
    refreshPOS().catch(posError);
  } else if (appMode === "adm") {
    show("workspace");
    if ($("tabDashboard").classList.contains("active"))
      loadDashboard().catch(posError);
  } else chooseMode();
}
function enterAdmin() {
  window.scrollTo(0, 0);
  appMode = "adm";
  show("workspace");
  document.body.classList.remove("pos-cart-open");
  selectPage("dashboard");
  posAnimate($("workspace"));
}
async function enterPOS() {
  window.scrollTo(0, 0);
  if (!canCreateOrders()) return;
  appMode = "pos";
  show("posWorkspace");
  posAnimate($("posWorkspace"));
  await refreshPOS();
}
function posError(e) {
  const messages = {
    cash_not_open: "O caixa foi fechado. Abra um caixa para vender.",
    checkout_price_changed:
      "O preço mudou. Atualize os produtos e confira a venda.",
    payment_total_invalid:
      "Confira a soma dos pagamentos e o dinheiro entregue.",
    stock_insufficient:
      "Estoque insuficiente. Atualize os produtos e ajuste a quantidade.",
    product_unavailable: "Um produto ficou indisponível. Revise a venda.",
    cash_changed_reload: "O saldo mudou. Atualize o caixa e confira novamente.",
    cash_insufficient: "A retirada excede o dinheiro disponível.",
  };
  const text = messages[e.message] || e.message;
  const d = document.querySelector("dialog[open]");
  if (d) {
    let p = d.querySelector("[role=alert]");
    if (!p) {
      p = document.createElement("p");
      p.setAttribute("role", "alert");
      d.append(p);
    }
    p.textContent = text;
    p.classList.remove("hidden");
  } else {
    $("posGateError").textContent = text;
    $("posGateError").classList.remove("hidden");
    msg(text, true);
  }
}
function posOpenSession() {
  return cashData?.sessions.find((s) => !s.closed_at);
}
async function refreshPOS() {
  if (posBusy || appMode !== "pos") return;
  posBusy = true;
  const store = currentStoreId;
  try {
    const [products, cash, summary] = await Promise.all([
      api("products"),
      api("cash-sessions"),
      api("pos-summary"),
    ]);
    if (appMode !== "pos" || store !== currentStoreId) return;
    allProducts = products;
    cashData = cash;
    posSummaryData = summary;
    if (posLoadedStore !== store) {
      const d = posRead("cart", { items: [], service: "kitchen" });
      posCart = d.items;
      $("posService").value = d.service;
      $("posDiscount").value = d.discount || "0,00";
      $("posDiscountNote").value = d.note || "";
      posAttempt = posRead("attempt", null);
      posLoadedStore = store;
    }
    setOptions($("posStore"), latestStores, null);
    $("posStore").value = String(store);
    $("posStore").disabled = !canManageStores();
    $("posOperator").textContent = currentUser.email;
    $("posTheme").value = $("themeSelect").value;
    const open = posOpenSession();
    $("posCash").textContent = open
      ? "Caixa #" + open.id + " aberto"
      : "Caixa fechado";
    $("posCash").classList.toggle("open", !!open);
    $("posCashGate").classList.toggle("hidden", !!open);
    $("posTerminal").classList.toggle("hidden", !open);
    $("posMobileCart").classList.toggle("hidden", !open);
    $("posDiscountSection").classList.toggle("hidden", !canManageCatalog());
    renderPOSCategories();
    renderPOSProducts();
    renderPOSCart();
  } finally {
    posBusy = false;
  }
}
function renderPOSCategories() {
  const root = $("posCategories");
  root.replaceChildren();
  for (const c of [
    { id: "", name: "Todos" },
    ...(posSummaryData?.categories || []),
  ]) {
    const b = document.createElement("button");
    b.textContent = c.name;
    b.setAttribute("aria-pressed", String(String(c.id) === posCategory));
    b.onclick = () => {
      posCategory = String(c.id);
      renderPOSCategories();
      renderPOSProducts();
    };
    root.append(b);
  }
}
function renderPOSProducts() {
  const root = $("posGrid");
  root.replaceChildren();
  const q = $("posSearch").value.trim().toLocaleLowerCase("pt-BR");
  const products = allProducts.filter(
    (p) =>
      p.active &&
      (!posCategory || String(p.category_id) === posCategory) &&
      [p.name, p.sku, p.barcode].some((v) =>
        String(v || "")
          .toLocaleLowerCase("pt-BR")
          .includes(q),
      ),
  );
  for (const p of products) {
    const b = document.createElement("button");
    b.className = "pos-product";
    b.dataset.productId = p.id;
    b.disabled =
      p.stock_quantity < 1 ||
      p.is_weight_based ||
      !["unidade", "porcao"].includes(p.unit);
    for (const [cls, value] of [
      [
        "product-initial",
        p.name
          .split(" ")
          .slice(0, 2)
          .map((s) => s[0])
          .join("")
          .toUpperCase(),
      ],
      ["product-name", p.name],
      ["product-price", money(p.price_cents)],
      [
        "product-meta",
        b.disabled
          ? "Indisponível"
          : `${p.unit === "porcao" ? "Porção" : "Unidade"} · ${p.stock_quantity} disponíveis`,
      ],
    ]) {
      const el = document.createElement(
        cls === "product-name" ? "strong" : "span",
      );
      el.className = cls;
      el.textContent = value;
      b.append(el);
    }
    b.onclick = () => posAdd(p);
    root.append(b);
  }
  $("posEmpty").classList.toggle("hidden", products.length > 0);
}
function posAdd(p) {
  if (posAttempt) {
    posError(Error("Retome a confirmação desta venda antes de editar."));
    return;
  }
  let i = posCart.find((i) => i.product_id === p.id);
  if ((i?.quantity || 0) + 1 > p.stock_quantity) {
    posError(Error("stock_insufficient"));
    return;
  }
  if (i) i.quantity++;
  else
    posCart.push({
      product_id: p.id,
      quantity: 1,
      name: p.name,
      price_cents: p.price_cents,
    });
  renderPOSCart();
  posSave();
}
function posSubtotal() {
  return posCart.reduce((s, i) => s + i.price_cents * i.quantity, 0);
}
function posDiscount() {
  try {
    return canManageCatalog() ? parsePrice($("posDiscount").value || "0") : 0;
  } catch {
    return NaN;
  }
}
function renderPOSCart() {
  const root = $("posCartLines");
  root.replaceChildren();
  for (const i of posCart) {
    const row = document.createElement("div");
    row.className = "pos-line";
    const title = document.createElement("div");
    title.className = "pos-line-title";
    const name = document.createElement("strong"),
      amount = document.createElement("span");
    name.textContent = i.name;
    amount.textContent = money(i.quantity * i.price_cents);
    title.append(name, amount);
    const controls = document.createElement("div");
    controls.className = "pos-line-controls";
    const minus = document.createElement("button"),
      qty = document.createElement("input"),
      plus = document.createElement("button"),
      remove = document.createElement("button");
    minus.textContent = "−";
    plus.textContent = "+";
    minus.setAttribute("aria-label", "Diminuir " + i.name);
    plus.setAttribute("aria-label", "Aumentar " + i.name);
    qty.type = "number";
    qty.min = 1;
    qty.step = 1;
    qty.value = i.quantity;
    qty.setAttribute("aria-label", "Quantidade de " + i.name);
    remove.textContent = "Remover";
    remove.className = "pos-remove";
    const change = (n) => {
      if (posAttempt) return;
      const p = allProducts.find((p) => p.id === i.product_id);
      if (!Number.isInteger(n) || n < 1 || n > (p?.stock_quantity || 0)) {
        qty.value = i.quantity;
        return;
      }
      i.quantity = n;
      renderPOSCart();
      posSave();
    };
    minus.onclick = () => change(i.quantity - 1);
    plus.onclick = () => change(i.quantity + 1);
    qty.onchange = () => change(Number(qty.value));
    remove.onclick = () => {
      if (posAttempt) return;
      posCart = posCart.filter((v) => v !== i);
      renderPOSCart();
      posSave();
    };
    [minus, qty, plus, remove].forEach((b) => (b.disabled = !!posAttempt));
    controls.append(minus, qty, plus, remove);
    row.append(title, controls);
    root.append(row);
  }
  const count = posCart.reduce((s, i) => s + i.quantity, 0),
    total = posSubtotal() - posDiscount();
  $("posCartCount").textContent = count;
  $("posCartEmpty").classList.toggle("hidden", !!count);
  $("posSubtotal").textContent = money(posSubtotal());
  $("posTotal").textContent = Number.isFinite(total)
    ? money(total)
    : "Confira desconto";
  $("posPay").disabled = !count || !Number.isFinite(total) || total < 1;
  $("posPay").textContent = posAttempt ? "Retomar confirmação" : "Receber · F4";
  $("posHold").disabled = !count || !!posAttempt;
  for (const id of ["posClear", "posService", "posDiscount", "posDiscountNote"])
    $(id).disabled = !!posAttempt;
  $("posMobileCart").textContent =
    `Ver venda · ${count} itens · ${money(total || 0)}`;
}
function posDialog(title) {
  let d = $("posDialog");
  if (!d) {
    d = document.createElement("dialog");
    d.id = "posDialog";
    d.className = "dialog";
    document.body.append(d);
  }
  if (d.open) d.close();
  d.innerHTML =
    '<div class="dialog-header"><h2 id="posDialogTitle"></h2><button type="button" class="icon-button" aria-label="Fechar">×</button></div><div class="dialog-body"></div><p role="alert" class="dialog-message hidden"></p>';
  d.setAttribute("aria-labelledby", "posDialogTitle");
  $("posDialogTitle").textContent = title;
  d.querySelector("button").onclick = () => d.close();
  d.showModal();
  posAnimate(d);
  return d.querySelector(".dialog-body");
}
function posButton(label, fn) {
  const b = document.createElement("button");
  b.type = "button";
  b.textContent = label;
  b.onclick = fn;
  return b;
}
function posPay() {
  if (!posCart.length) return;
  const total =
    posAttempt?.payload.expected_total_cents ?? posSubtotal() - posDiscount();
  if (!Number.isSafeInteger(total) || total < 1) return;
  if (posDiscount() > 0 && !$("posDiscountNote").value.trim()) {
    posError(Error("Informe o motivo do desconto."));
    return;
  }
  const body = posDialog(posAttempt ? "Retomar confirmação" : "Receber venda");
  body.innerHTML =
    '<div class="pos-payment-summary"><span>Total da venda</span><strong id="posPaymentTotal"></strong></div><form id="posPaymentForm"><div id="posPaymentRows"></div><button type="button" id="posAddPayment" class="btn-secondary">Dividir pagamento</button><label id="posTenderLabel">Dinheiro entregue (R$)<input id="posTender" inputmode="decimal" value="0,00"></label><p id="posPaymentBalance" aria-live="polite"></p><p id="posChange" aria-live="polite"></p><label><input type="checkbox" name="confirmed" required style="width:auto"> Conferi o recebimento, inclusive cartão ou Pix no equipamento/banco.</label><button id="posConfirmSale" type="submit">Confirmar venda</button><p>Comprovante não fiscal. Cartão e Pix são conferidos manualmente.</p></form>';
  $("posPaymentTotal").textContent = money(total);
  const rows = [];
  const amounts = () =>
    rows.map((r) => ({
      method: r.select.value,
      amount_cents: parsePrice(r.input.value),
    }));
  function update() {
    try {
      const payments = amounts(),
        sum = payments.reduce((s, p) => s + p.amount_cents, 0),
        cash = payments
          .filter((p) => p.method === "cash")
          .reduce((s, p) => s + p.amount_cents, 0),
        tender = cash ? parsePrice($("posTender").value) : 0;
      $("posTenderLabel").classList.toggle("hidden", !cash);
      $("posPaymentBalance").textContent =
        sum === total
          ? "Pagamentos conferem com o total"
          : `Falta distribuir: ${money(total - sum)}`;
      $("posChange").textContent = cash
        ? `Troco: ${money(Math.max(0, tender - cash))}`
        : "";
      $("posConfirmSale").disabled =
        sum !== total ||
        payments.some((p) => p.amount_cents < 1) ||
        tender < cash;
    } catch {
      $("posConfirmSale").disabled = true;
    }
  }
  function add(p) {
    const row = document.createElement("div");
    row.className = "pos-payment-row";
    const select = document.createElement("select");
    select.setAttribute("aria-label", "Forma de pagamento");
    for (const [v, l] of Object.entries(cashMethods)) {
      const o = document.createElement("option");
      o.value = v;
      o.textContent = l;
      select.append(o);
    }
    select.value = p.method;
    const input = document.createElement("input");
    input.inputMode = "decimal";
    input.setAttribute("aria-label", "Valor do pagamento");
    input.value = (p.amount_cents / 100).toFixed(2).replace(".", ",");
    const entry = { row, select, input };
    const remove = posButton("×", () => {
      rows.splice(rows.indexOf(entry), 1);
      row.remove();
      update();
    });
    remove.setAttribute("aria-label", "Remover pagamento");
    rows.push(entry);
    row.append(select, input, remove);
    $("posPaymentRows").append(row);
    select.onchange = input.oninput = update;
    if (posAttempt) select.disabled = input.disabled = remove.disabled = true;
  }
  for (const p of posAttempt?.payload.payments || [
    { method: "cash", amount_cents: total },
  ])
    add(p);
  const tender =
    posAttempt?.payload.payments
      .filter((p) => p.method === "cash")
      .reduce((s, p) => s + p.tendered_cents, 0) ?? total;
  $("posTender").value = (tender / 100).toFixed(2).replace(".", ",");
  $("posTender").disabled = !!posAttempt;
  $("posAddPayment").disabled = !!posAttempt;
  $("posAddPayment").onclick = () => {
    try {
      add({
        method: "card",
        amount_cents: Math.max(
          0,
          total - amounts().reduce((s, p) => s + p.amount_cents, 0),
        ),
      });
      update();
    } catch {}
  };
  $("posTender").oninput = update;
  update();
  $("posPaymentForm").onsubmit = async (e) => {
    e.preventDefault();
    const button = $("posConfirmSale");
    button.disabled = true;
    try {
      if (!posAttempt) {
        const payments = amounts();
        let extra =
          parsePrice($("posTender").value) -
          payments
            .filter((p) => p.method === "cash")
            .reduce((s, p) => s + p.amount_cents, 0);
        for (const p of payments) {
          p.tendered_cents =
            p.method === "cash" ? p.amount_cents + Math.max(0, extra) : 0;
          if (p.method === "cash") extra = 0;
        }
        posAttempt = {
          key: crypto.randomUUID(),
          payload: {
            items: posCart.map((i) => ({
              product_id: i.product_id,
              quantity: i.quantity,
            })),
            payments,
            expected_total_cents: total,
            discount_cents: posDiscount(),
            discount_note: $("posDiscountNote").value.trim(),
            service: $("posService").value,
          },
        };
        sessionStorage.setItem(posKey("attempt"), JSON.stringify(posAttempt));
        renderPOSCart();
      }
      for (const r of rows) {
        r.select.disabled = r.input.disabled = true;
      }
      $("posTender").disabled = $("posAddPayment").disabled = true;
      const result = await api("pos-checkout", posAttempt.payload, {
        "Idempotency-Key": posAttempt.key,
      });
      posAttempt = null;
      sessionStorage.removeItem(posKey("attempt"));
      posCart = [];
      $("posDiscount").value = "0";
      $("posDiscountNote").value = "";
      posSave();
      await refreshPOS().catch(posError);
      try {
        await showPOSReceipt(result.id);
      } catch (e) {
        const body = posDialog("Venda #" + result.id + " registrada");
        const p = document.createElement("p");
        p.textContent =
          "A venda foi registrada. Não foi possível carregar o comprovante agora.";
        body.append(
          p,
          posButton("Carregar comprovante", () =>
            showPOSReceipt(result.id).catch(posError),
          ),
          posButton("Nova venda", () => $("posDialog").close()),
        );
      }
    } catch (err) {
      if (
        [
          "cash_not_open",
          "checkout_price_changed",
          "payment_total_invalid",
          "stock_insufficient",
          "product_unavailable",
          "quantity_invalid",
          "discount_invalid",
          "discount_manager_required",
          "checkout_invalid",
          "order_duplicate_product",
        ].includes(err.message)
      ) {
        posAttempt = null;
        sessionStorage.removeItem(posKey("attempt"));
        for (const r of rows) {
          r.select.disabled = r.input.disabled = false;
        }
        $("posTender").disabled = $("posAddPayment").disabled = false;
      }
      posError(err);
      renderPOSCart();
      button.disabled = false;
    }
  };
}
async function showPOSReceipt(id) {
  const r = await api("pos-receipt?id=" + id),
    body = posDialog("Venda #" + id + " registrada"),
    receipt = document.createElement("div");
  receipt.className = "pos-receipt";
  const title = document.createElement("h3");
  title.textContent = r.store;
  receipt.append(title);
  for (const line of [
    "COMPROVANTE NÃO FISCAL",
    "Venda #" + r.id + " · " + cashDate(r.created_at),
    "Operador: " + r.operator,
    r.service === "kitchen" ? "Encaminhada para cozinha" : "Venda de balcão",
    "Situação: " +
      ({
        confirmed: "Aguardando preparo",
        preparing: "Em preparo",
        ready: "Pronto",
        completed: "Concluída",
        cancelled: "Cancelada",
      }[r.state] || r.state) +
      " · " +
      ({
        manual_confirmed: "Recebida",
        manual_pending: "A receber",
        refunded: "Devolvida",
        refund_partial: "Devolução parcial",
      }[r.financial_state] || r.financial_state),
  ]) {
    const p = document.createElement("p");
    p.textContent = line;
    receipt.append(p);
  }
  receipt.append(document.createElement("hr"));
  for (const i of r.items) {
    const row = document.createElement("div");
    row.className = "pos-receipt-row";
    const n = document.createElement("span"),
      v = document.createElement("span");
    n.textContent = `${i.quantity} × ${i.name}`;
    v.textContent = money(i.quantity * i.price_cents);
    row.append(n, v);
    receipt.append(row);
  }
  for (const line of [
    r.discount_cents ? "Desconto: " + money(r.discount_cents) : "",
    "TOTAL: " + money(r.total_cents),
    ...r.payments.map(
      (p) => cashMethods[p.method] + ": " + money(p.amount_cents),
    ),
    "Troco: " +
      money(
        r.payments.reduce(
          (s, p) => s + Math.max(0, p.tendered_cents - p.amount_cents),
          0,
        ),
      ),
  ]) {
    if (!line) continue;
    const p = document.createElement("p");
    p.textContent = line;
    receipt.append(p);
  }
  body.append(
    receipt,
    posButton("Imprimir comprovante", () => window.print()),
    posButton("Nova venda", () => {
      $("posDialog").close();
      document.body.classList.remove("pos-cart-open");
      $("posSearch").focus();
    }),
  );
}
function posList(title, items, label, action) {
  const body = posDialog(title);
  body.classList.add("pos-dialog-list");
  if (!items.length) body.textContent = "Nenhum registro.";
  for (const item of items) {
    const row = document.createElement("div"),
      text = document.createElement("span");
    text.textContent = label(item);
    row.append(
      text,
      posButton("Abrir", () => action(item)),
    );
    body.append(row);
  }
  return body;
}
function heldSales() {
  const held = posRead("held", []);
  posList(
    "Vendas em espera",
    held,
    (i) =>
      `${cashDate(i.at)} · ${i.items.reduce((s, p) => s + p.quantity, 0)} itens`,
    (i) => {
      if (posCart.length || posAttempt) {
        posError(Error("Finalize ou coloque a venda atual em espera."));
        return;
      }
      posCart = i.items;
      $("posService").value = i.service;
      sessionStorage.setItem(
        posKey("held"),
        JSON.stringify(held.filter((v) => v !== i)),
      );
      posSave();
      renderPOSCart();
      $("posDialog").close();
    },
  );
}
function holdSale() {
  if (!posCart.length || posAttempt) return;
  const held = posRead("held", []);
  if (held.length >= 10) {
    posError(
      Error("Retome uma das 10 vendas em espera antes de guardar outra."),
    );
    return;
  }
  held.push({
    items: posCart,
    service: $("posService").value,
    at: new Date().toISOString(),
  });
  sessionStorage.setItem(posKey("held"), JSON.stringify(held));
  posCart = [];
  $("posDiscount").value = "0";
  $("posDiscountNote").value = "";
  renderPOSCart();
  posSave();
}
async function posCashDialog() {
  await loadCash();
  const s = posOpenSession();
  if (!s) {
    await refreshPOS();
    return;
  }
  const expected = s.opening_cents + s.cash_cents + s.adjustments_cents,
    body = posDialog("Caixa #" + s.id);
  body.append(
    posButton("Histórico de sangrias e suprimentos", async () => {
      try {
        const entries = await api("cash-adjustments");
        const list = posDialog("Sangrias e suprimentos");
        for (const e of entries) {
          const p = document.createElement("p");
          p.textContent = `Caixa #${e.session_id} · ${money(e.amount_cents)} · ${e.note} · ${e.operator} · ${cashDate(e.created_at)}`;
          list.append(p);
        }
        if (!entries.length)
          list.textContent = "Nenhuma sangria ou suprimento registrado.";
      } catch (e) {
        posError(e);
      }
    }),
  );
  for (const [l, v] of [
    ["Abertura", s.opening_cents],
    ["Dinheiro esperado", expected],
    ["Cartão conferido", s.card_cents],
    ["Pix manual", s.pix_cents],
    ["Sangrias / suprimentos", s.adjustments_cents],
  ]) {
    const p = document.createElement("p");
    p.textContent = l + ": " + money(v);
    body.append(p);
  }
  if (canManageCatalog() || s.opened_by_id === currentUser.id) {
    for (const [label, sign] of [
      ["Sangria", -1],
      ["Suprimento", 1],
    ])
      body.append(
        posButton(label, () => {
          $("posDialog").close();
          cashDialog(
            label,
            cashAmountField("Valor (R$)", "amount", 0) +
              cashNoteField.replace("maxlength", "required maxlength"),
            "Registrar",
            (d, key) =>
              api(
                "cash-adjustments",
                {
                  session_id: s.id,
                  amount_cents: sign * parsePrice(d.get("amount")),
                  note: d.get("note"),
                  expected_cents: expected,
                },
                { "Idempotency-Key": key },
              ),
          );
        }),
      );
    body.append(
      posButton("Fechar caixa", () => {
        $("posDialog").close();
        $("cashClose").onclick();
      }),
    );
  }
  if (canManageCatalog())
    body.append(
      posButton("Devoluções", () =>
        posList(
          "Recebimentos para devolução",
          cashData.receipts.filter((e) => e.amount_cents > 0 && !e.refunded),
          (e) =>
            "Pedido #" +
            e.order_id +
            " · " +
            money(e.amount_cents) +
            " · " +
            cashMethods[e.method],
          (e) => {
            $("posDialog").close();
            refundDialog(e);
          },
        ),
      ),
    );
}
async function loadDashboard() {
  if (appMode !== "adm") return;
  const store = currentStoreId,
    result = await api("pos-summary");
  if (appMode !== "adm" || store !== currentStoreId) return;
  const root = $("dashboardStats");
  root.replaceChildren();
  for (const [l, v] of [
    [
      result.sales_today_cents == null ? "Produtos ativos" : "Vendas de hoje",
      result.sales_today_cents == null
        ? String(result.active_products)
        : money(result.sales_today_cents),
    ],
    ["Em preparo / entrega", String(result.active_orders)],
    [
      "Recebimentos de hoje",
      result.received_today_cents == null
        ? "Perfil operacional"
        : money(result.received_today_cents),
    ],
    [
      "Saldo a receber",
      result.pending_cents == null
        ? "Perfil operacional"
        : money(result.pending_cents),
    ],
  ]) {
    const div = document.createElement("div"),
      span = document.createElement("span"),
      strong = document.createElement("strong");
    span.textContent = l;
    strong.textContent = v;
    div.append(span, strong);
    root.append(div);
  }
  $("dashboardDate").textContent = new Date().toLocaleDateString("pt-BR", {
    dateStyle: "full",
  });
  $("dashboardCash").textContent =
    "Resumo da loja selecionada. Vendas e recebimentos são apurados separadamente.";
  $("dashboardPOS").classList.toggle("hidden", !canCreateOrders());
  for (const b of document.querySelectorAll("[data-admin-page]"))
    b.classList.toggle(
      "hidden",
      !{
        catalogo: canManageCatalog(),
        pagamentos: canAccessPayments(),
        operacao: true,
      }[b.dataset.adminPage],
    );
}
$("enterPOS").onclick = () => enterPOS().catch(posError);
$("enterAdmin").onclick = enterAdmin;
$("adminToPOS").onclick = $("dashboardPOS").onclick = () =>
  enterPOS().catch(posError);
$("posToAdmin").onclick = enterAdmin;
$("posGateBack").onclick = chooseMode;
$("modeLogout").onclick = $("posLogout").onclick = () => $("logout").onclick();
$("posTheme").onchange = () => setTheme($("posTheme").value);
$("posStore").onchange = async () => {
  if (posAttempt) {
    $("posStore").value = String(currentStoreId);
    posError(
      Error("Retome a confirmação desta venda antes de trocar de loja."),
    );
    return;
  }
  posSave();
  currentStoreId = Number($("posStore").value);
  posCategory = "";
  $("posDiscount").value = "0";
  $("posDiscountNote").value = "";
  await refresh();
  await refreshPOS();
};
let posOpeningKey = crypto.randomUUID();
$("posOpenForm").onsubmit = async (e) => {
  e.preventDefault();
  const b = $("posOpenSubmit");
  b.disabled = true;
  try {
    const d = new FormData(e.target);
    await api(
      "cash-sessions",
      {
        action: "open",
        opening_cents: parsePrice(d.get("opening")),
        note: d.get("note"),
      },
      { "Idempotency-Key": posOpeningKey },
    );
    posOpeningKey = crypto.randomUUID();
    $("posGateError").classList.add("hidden");
    await refreshPOS();
  } catch (err) {
    posError(err);
  } finally {
    b.disabled = false;
  }
};
$("posSearch").oninput = renderPOSProducts;
$("posSearch").onkeydown = (e) => {
  if (e.key !== "Enter") return;
  const code = e.target.value.trim(),
    p = allProducts.find(
      (p) => p.active && (p.barcode === code || p.sku === code),
    );
  if (p && !p.is_weight_based && ["unidade", "porcao"].includes(p.unit)) {
    posAdd(p);
    e.target.value = "";
    renderPOSProducts();
  }
};
$("posReload").onclick = async () => {
  try {
    await refreshPOS();
    if (!posAttempt) {
      for (const i of posCart) {
        const p = allProducts.find((p) => p.id === i.product_id);
        if (p) {
          i.price_cents = p.price_cents;
          i.name = p.name;
        }
      }
      renderPOSCart();
      posSave();
    }
  } catch (e) {
    posError(e);
  }
};
$("posPay").onclick = posPay;
$("posHold").onclick = holdSale;
$("posHeld").onclick = heldSales;
$("posCash").onclick = () => posCashDialog().catch(posError);
$("posRecent").onclick = () =>
  posList(
    "Últimas vendas do PDV",
    posSummaryData?.recent || [],
    (i) =>
      "#" +
      i.id +
      " · " +
      money(i.total_cents) +
      " · " +
      cashDate(i.created_at),
    (i) => showPOSReceipt(i.id).catch(posError),
  );
$("posPending").onclick = () =>
  posList(
    "Pedidos a receber",
    cashData?.pending || [],
    (i) => "#" + i.id + " · " + money(i.remaining_cents),
    (i) => {
      $("posDialog").close();
      receiptDialog(i);
    },
  );
$("posHelp").onclick = () => {
  posDialog("Atalhos do PDV").textContent =
    "F2: buscar produto · F4: receber venda · F8: colocar em espera · Esc: fechar janela. Cartão e Pix precisam de conferência manual. Vendas em espera ficam nesta aba do navegador.";
};
$("posClear").onclick = () => {
  if (posAttempt || !posCart.length) return;
  posDialog("Limpar venda?").append(
    posButton("Confirmar limpeza", () => {
      posCart = [];
      $("posDiscount").value = "0";
      $("posDiscountNote").value = "";
      renderPOSCart();
      posSave();
      $("posDialog").close();
    }),
  );
};
$("posDiscount").oninput = () => {
  renderPOSCart();
  posSave();
};
$("posDiscountNote").oninput = posSave;
$("posService").onchange = posSave;
$("posMobileCart").onclick = () => {
  document.body.classList.add("pos-cart-open");
  window.scrollTo(0, 0);
};
$("posBackProducts").onclick = () =>
  document.body.classList.remove("pos-cart-open");
for (const b of document.querySelectorAll("[data-admin-page]"))
  b.onclick = () => selectPage(b.dataset.adminPage);
document.addEventListener("keydown", (e) => {
  if (appMode !== "pos" || document.querySelector("dialog[open]")) return;
  if (e.key === "F2") {
    e.preventDefault();
    document.body.classList.remove("pos-cart-open");
    $("posSearch").focus();
  }
  if (e.key === "F4" && !$("posPay").disabled) {
    e.preventDefault();
    posPay();
  }
  if (e.key === "F8") {
    e.preventDefault();
    holdSale();
  }
});
