let diningData = { tables: [], tabs: [] },
  diningActive = null,
  diningPending = null,
  diningContextKey = null;
const diningButton = posButton("Mesas e comandas", () =>
  openDining().catch(posError),
);
diningButton.id = "posDining";
diningButton.className = "btn-secondary";
document.querySelector(".pos-tools").prepend(diningButton);
const diningGateButton = posButton("Consultar mesas e comandas", () =>
  openDining().catch(posError),
);
diningGateButton.className = "btn-secondary";
diningGateButton.id = "diningGateConsult";
$("posGateBack").before(diningGateButton);
const diningAdminButton = posButton("Mesas e comandas", () =>
  openDining().catch(posError),
);
diningAdminButton.id = "adminDining";
diningAdminButton.classList.add("hidden");
document.querySelector(".admin-shortcuts").append(diningAdminButton);
const diningBanner = document.createElement("div");
diningBanner.id = "diningContext";
diningBanner.className = "dining-context hidden";
document.querySelector(".pos-section-title").after(diningBanner);
const diningCartLabel = document.createElement("p");
diningCartLabel.id = "diningCartLabel";
diningCartLabel.className = "dining-cart-label hidden";
document.querySelector(".pos-cart-heading").after(diningCartLabel);
const diningNote = document.createElement("label");
diningNote.className = "hidden";
diningNote.id = "diningNoteLabel";
diningNote.textContent = "Observação desta rodada";
const diningNoteInput = document.createElement("textarea");
diningNoteInput.id = "diningNote";
diningNoteInput.rows = 2;
diningNoteInput.maxLength = 1000;
diningNote.append(diningNoteInput);
$("posPay").before(diningNote);
const diningSend = posButton("Enviar para preparo", () => sendDiningRound());
diningSend.id = "diningSend";
diningSend.className = "hidden";
$("posPay").before(diningSend);
function resetDiningContext() {
  diningActive = null;
  diningPending = null;
  diningContextKey = null;
}
async function refreshDining() {
  const key = posKey("dining-context"),
    store = currentStoreId,
    user = currentUser?.id;
  const [tables, tabs] = await Promise.all([
    api("dining-tables"),
    api("dining-tabs"),
  ]);
  if (store !== currentStoreId || user !== currentUser?.id) return;
  diningData = { tables, tabs };
  if (diningContextKey !== key) {
    diningActive = null;
    const id = posRead("dining-selected", null);
    diningActive = tabs.find((t) => t.id === id) || null;
    diningPending = posRead("dining-pending", null);
    diningContextKey = key;
  } else if (diningActive)
    diningActive = tabs.find((t) => t.id === diningActive.id) || diningActive;
  if (diningActive?.state !== "open" && !posCart.length && !diningPending) {
    diningActive = null;
    sessionStorage.removeItem(posKey("dining-selected"));
  }
  diningAdminButton.classList.toggle("hidden", !canAccessPayments());
  diningRenderContext();
}
function diningRenderContext() {
  const active = !!diningActive;
  diningBanner.classList.toggle("hidden", !active);
  diningCartLabel.classList.toggle("hidden", !active);
  diningNote.classList.toggle("hidden", !active);
  diningSend.classList.toggle("hidden", !active);
  $("posPay").classList.toggle("hidden", active && !posAttempt);
  if (active && !posAttempt) $("posPay").disabled = true;
  $("posService").closest("label").classList.toggle("hidden", active);
  $("posDiscountSection").classList.toggle(
    "hidden",
    active || !canManageCatalog(),
  );
  if (active) {
    diningBanner.replaceChildren();
    const name = document.createElement("strong");
    name.textContent =
      diningActive.label + " · saldo " + money(diningActive.remaining_cents);
    diningBanner.append(
      name,
      posButton("Ver conta", () =>
        showDiningTab(diningActive.id).catch(posError),
      ),
      posButton("Voltar ao balcão", () => {
        if (posAttempt || diningPending) {
          posError(
            Error("Retome a confirmação antes de trocar o atendimento."),
          );
          return;
        }
        diningActive = null;
        sessionStorage.removeItem(posKey("dining-selected"));
        diningRenderContext();
      }),
    );
    diningCartLabel.textContent = "Adicionar à " + diningActive.label;
    diningSend.textContent = diningPending
      ? "Retomar lançamento"
      : "Enviar itens para preparo";
    diningSend.disabled =
      (!posCart.length && !diningPending) ||
      !!posAttempt ||
      diningActive.state !== "open";
    diningNoteInput.disabled = !!diningPending || !!posAttempt;
    if (diningActive.state !== "open")
      name.textContent +=
        " · conta encerrada, volte ao balcão para usar estes itens";
  }
}
async function openDining() {
  cashData = await api("cash-sessions");
  await refreshDining();
  const body = posDialog("Mesas e comandas");
  $("posDialog").classList.add("dining-dialog");
  const intro = document.createElement("p");
  intro.textContent =
    "Contas salvas no aplicativo. Pedidos seguem para preparo quando os itens são lançados.";
  body.append(intro);
  const actions = document.createElement("div");
  actions.className = "pos-dialog-actions";
  if (canCreateOrders())
    actions.append(
      posButton("Abrir comanda avulsa", () => openDiningTabForm()),
    );
  if (canManageCatalog())
    actions.append(
      posButton("Cadastrar / editar mesas", () => diningTableList()),
    );
  actions.append(posButton("Atualizar", () => openDining().catch(posError)));
  body.append(actions);
  const grid = document.createElement("div");
  grid.className = "dining-grid";
  grid.id = "diningTables";
  for (const table of diningData.tables.filter((t) => t.active)) {
    const tab = diningData.tabs.find(
      (t) => t.table_id === table.id && t.state === "open",
    );
    const button = posButton("", () =>
      tab ? showDiningTab(tab.id).catch(posError) : openDiningTabForm(table),
    );
    button.className = "dining-table " + (tab ? "occupied" : "free");
    button.dataset.tableId = table.id;
    const strong = document.createElement("strong"),
      status = document.createElement("span"),
      amount = document.createElement("span");
    strong.textContent = table.name;
    status.textContent = tab
      ? "Ocupada · " + tab.order_count + " pedidos"
      : "Livre · " + table.capacity + " lugares";
    amount.textContent = tab ? money(tab.remaining_cents) : "Abrir atendimento";
    button.append(strong, status, amount);
    button.disabled = !tab && (!canCreateOrders() || !posOpenSession());
    grid.append(button);
  }
  body.append(grid);
  if (!grid.children.length) {
    const p = document.createElement("p");
    p.textContent =
      "Nenhuma mesa ativa cadastrada. Você pode usar uma comanda avulsa.";
    body.append(p);
  }
  const heading = document.createElement("h3");
  heading.textContent = "Comandas avulsas abertas";
  body.append(heading);
  for (const tab of diningData.tabs.filter(
    (t) => !t.table_id && t.state === "open",
  )) {
    const b = posButton(tab.label + " · " + money(tab.remaining_cents), () =>
      showDiningTab(tab.id).catch(posError),
    );
    b.className = "dining-tab-link";
    body.append(b);
  }
  const history = document.createElement("details");
  const summary = document.createElement("summary");
  summary.textContent = "Últimas 50 contas encerradas";
  history.append(summary);
  for (const tab of diningData.tabs.filter((t) => t.state !== "open"))
    history.append(
      posButton(
        tab.label + " · " + (tab.state === "closed" ? "Fechada" : "Cancelada"),
        () => showDiningTab(tab.id).catch(posError),
      ),
    );
  body.append(history);
}
function diningTableList() {
  const body = posDialog("Mesas da loja");
  for (const table of diningData.tables) {
    const row = document.createElement("div");
    row.className = "dining-management-row";
    const text = document.createElement("span");
    text.textContent =
      table.name +
      " · " +
      table.capacity +
      " lugares · " +
      (table.active ? "Ativa" : "Inativa");
    row.append(
      text,
      posButton("Editar", () => diningTableForm(table)),
    );
    body.append(row);
  }
  body.append(posButton("Nova mesa", () => diningTableForm()));
}
function diningTableForm(table = null) {
  const body = posDialog(table ? "Editar mesa" : "Nova mesa");
  body.innerHTML =
    '<form id="diningTableForm"><label>Identificação da mesa<input name="name" required maxlength="80" placeholder="Ex.: Mesa 01"></label><label>Lugares<input name="capacity" type="number" min="1" max="100" value="4" required></label><label><input name="active" type="checkbox" checked style="width:auto"> Mesa ativa</label><button type="submit">Salvar mesa</button></form>';
  const form = $("diningTableForm");
  if (table) {
    form.elements.name.value = table.name;
    form.elements.capacity.value = table.capacity;
    form.elements.active.checked = table.active;
  }
  const key = crypto.randomUUID();
  form.onsubmit = (e) => {
    e.preventDefault();
    run(async () => {
      await api(
        "dining-tables",
        {
          id: table?.id || 0,
          name: form.elements.name.value.trim(),
          capacity: Number(form.elements.capacity.value),
          active: form.elements.active.checked,
        },
        { "Idempotency-Key": key },
      );
      await openDining();
    }, form.querySelector("button"));
  };
}
function openDiningTabForm(table = null) {
  if (posAttempt || diningPending) {
    posError(
      Error("Retome a confirmação pendente antes de abrir outra conta."),
    );
    return;
  }
  if (!posOpenSession()) {
    posError(Error("cash_not_open"));
    return;
  }
  const body = posDialog(
    table ? "Abrir " + table.name : "Abrir comanda avulsa",
  );
  body.innerHTML =
    '<form id="diningOpenForm"><label>Identificação da comanda<input name="label" required maxlength="80" placeholder="Ex.: Comanda 12"></label><button type="submit">Abrir atendimento</button></form>';
  const form = $("diningOpenForm");
  form.elements.label.value = table?.name || "";
  const key = crypto.randomUUID();
  form.onsubmit = (e) => {
    e.preventDefault();
    run(async () => {
      const result = await api(
        "dining-tabs",
        {
          action: "open",
          table_id: table?.id || 0,
          label: form.elements.label.value.trim(),
        },
        { "Idempotency-Key": key },
      );
      await refreshDining();
      await showDiningTab(result.id);
    }, form.querySelector("button"));
  };
}
async function showDiningTab(id) {
  const data = await api("dining-tabs?id=" + id),
    tab = data.tab,
    body = posDialog(tab.label + " · comanda #" + id);
  $("posDialog").classList.add("dining-dialog");
  const summary = document.createElement("div");
  summary.className = "dining-bill-summary";
  for (const [label, value] of [
    ["Consumo", tab.total_cents],
    ["Recebido líquido", tab.received_cents],
    ["Saldo a receber", tab.remaining_cents],
  ]) {
    const p = document.createElement("p");
    p.textContent = label + ": " + money(value);
    summary.append(p);
  }
  body.append(summary);
  const state = document.createElement("p");
  state.textContent =
    (tab.state === "open"
      ? "Conta aberta"
      : "Conta " + (tab.state === "closed" ? "fechada" : "cancelada")) +
    " · " +
    cashDate(tab.opened_at);
  body.append(state);
  const orders = document.createElement("div");
  orders.id = "diningOrders";
  for (const order of data.orders) {
    const row = document.createElement("p");
    row.textContent =
      "Pedido #" +
      order.id +
      " · " +
      money(order.total_cents) +
      " · " +
      (stateLabels[order.state] || order.state) +
      (order.note ? " · " + order.note : "");
    orders.append(row);
  }
  body.append(orders);
  if (tab.state === "open" && canCreateOrders()) {
    body.append(
      posButton("Adicionar produtos", async () => {
        if (appMode !== "pos") await enterPOS();
        if (posAttempt || diningPending) {
          posError(Error("Retome a confirmação pendente."));
          return;
        }
        if (diningActive && diningActive.id !== tab.id && posCart.length) {
          posError(
            Error(
              "Envie ou limpe o carrinho da comanda atual antes de trocar.",
            ),
          );
          return;
        }
        if (!posOpenSession()) {
          posError(Error("cash_not_open"));
          return;
        }
        diningActive = tab;
        $("posDiscount").value = "0,00";
        $("posDiscountNote").value = "";
        posSave();
        renderPOSCart();
        sessionStorage.setItem(
          posKey("dining-selected"),
          JSON.stringify(tab.id),
        );
        $("posDialog").close();
        diningRenderContext();
        document.body.classList.remove("pos-cart-open");
        window.scrollTo(0, 0);
        $("posSearch").focus();
      }),
    );
    body.append(
      posButton("Fechar conta", () => {
        if (posCart.length || diningPending) {
          posError(
            Error("Envie os itens do carrinho antes de fechar a conta."),
          );
          return;
        }
        if (
          posAttempt &&
          (posAttempt.endpoint !== "dining-tabs" ||
            posAttempt.payload.id !== tab.id)
        ) {
          posError(Error("Retome a venda pendente."));
          return;
        }
        if (!posOpenSession()) {
          posError(Error("cash_not_open"));
          return;
        }
        if (tab.remaining_cents === 0) {
          const confirmBody = posDialog("Fechar conta sem saldo");
          const closeKey = crypto.randomUUID();
          confirmBody.append(
            posButton("Confirmar fechamento", async () => {
              try {
                await api(
                  "dining-tabs",
                  {
                    action: "close",
                    id: tab.id,
                    expected_version: tab.version,
                    expected_remaining_cents: 0,
                    payments: [],
                  },
                  { "Idempotency-Key": closeKey },
                );
                await refreshDining();
                await showPOSReceipt(tab.id, "dining-receipt");
              } catch (e) {
                posError(e);
              }
            }),
          );
        } else
          posPay({
            id: tab.id,
            version: tab.version,
            total: tab.remaining_cents,
          });
      }),
    );
    if (!tab.order_count)
      body.append(
        posButton("Cancelar conta vazia", () => {
          const confirmBody = posDialog("Cancelar conta vazia?");
          const key = crypto.randomUUID();
          confirmBody.append(
            posButton("Confirmar cancelamento", async () => {
              try {
                await api(
                  "dining-tabs",
                  {
                    action: "cancel",
                    id: tab.id,
                    expected_version: tab.version,
                  },
                  { "Idempotency-Key": key },
                );
                await openDining();
              } catch (e) {
                posError(e);
              }
            }),
          );
        }),
      );
  }
  body.append(
    posButton(
      tab.state === "open"
        ? "Conferir conta não fiscal"
        : "Comprovante não fiscal",
      () => showPOSReceipt(id, "dining-receipt").catch(posError),
    ),
  );
}
function sendDiningRound() {
  if (!diningActive || (!posCart.length && !diningPending)) return;
  if (posDiscount() > 0) {
    posError(
      Error(
        "Desconto de comanda não está disponível nesta etapa. Zere o desconto antes de lançar.",
      ),
    );
    return;
  }
  const body = posDialog(
    diningPending ? "Retomar lançamento" : "Enviar itens para preparo",
  );
  const p = document.createElement("p");
  p.textContent =
    diningActive.label +
    " · rodada de " +
    money(diningPending?.payload.expected_remaining_cents ?? posSubtotal()) +
    ". Os produtos serão reservados no estoque e enviados à fila de preparo.";
  body.append(p);
  const button = posButton("Confirmar envio", async () => {
    button.disabled = true;
    try {
      if (!diningPending) {
        diningPending = {
          key: crypto.randomUUID(),
          payload: {
            action: "add",
            id: diningActive.id,
            expected_version: diningActive.version,
            expected_remaining_cents: posSubtotal(),
            items: posCart.map((i) => ({
              product_id: i.product_id,
              quantity: i.quantity,
            })),
            note: diningNoteInput.value.trim(),
          },
        };
        sessionStorage.setItem(
          posKey("dining-pending"),
          JSON.stringify(diningPending),
        );
        renderPOSCart();
      }
      const result = await api("dining-tabs", diningPending.payload, {
        "Idempotency-Key": diningPending.key,
      });
      diningPending = null;
      sessionStorage.removeItem(posKey("dining-pending"));
      posCart = [];
      diningNoteInput.value = "";
      posSave();
      const sentBody = posDialog("Itens enviados para preparo");
      sentBody.append(
        posButton("Ver comanda", () =>
          showDiningTab(result.id).catch(posError),
        ),
      );
      await refreshPOS().catch(posError);
      await refreshDining().catch(posError);
      await showDiningTab(result.id).catch(posError);
    } catch (err) {
      if (
        [
          "tab_changed_reload",
          "tab_not_open",
          "cash_not_open",
          "checkout_price_changed",
          "stock_insufficient",
          "product_unavailable",
          "quantity_invalid",
          "tab_invalid",
          "total_limit",
          "order_duplicate_product",
        ].includes(err.message)
      ) {
        diningPending = null;
        sessionStorage.removeItem(posKey("dining-pending"));
      }
      posError(err);
      button.disabled = false;
      renderPOSCart();
    }
  });
  body.append(button);
}
