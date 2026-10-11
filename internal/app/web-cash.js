let cashData = null,
  cashGeneration = 0;
const cashMethods = {
  cash: "Dinheiro",
  card: "Cartão conferido",
  pix_manual: "Pix conferido manualmente",
};
function canReceiveManual() {
  return (
    currentUser &&
    ["instance_admin", "org_admin", "store_manager", "attendant"].includes(
      currentUser.role,
    )
  );
}
const cashDate = (value) =>
  value ? new Date(value).toLocaleString("pt-BR") : "Em aberto";
async function loadCash() {
  const generation = ++cashGeneration,
    store = currentStoreId;
  const result = await api("cash-sessions");
  if (generation !== cashGeneration || store !== currentStoreId) return;
  cashData = result;
  const open = result.sessions.find((s) => !s.closed_at);
  $("cashSummary").replaceChildren();
  const heading = document.createElement("p");
  heading.textContent = open
    ? "Caixa #" +
      open.id +
      " aberto por " +
      open.opened_by +
      " em " +
      cashDate(open.opened_at)
    : "Nenhum caixa aberto nesta loja.";
  $("cashSummary").append(heading);
  if (open) {
    const totals = document.createElement("div");
    totals.className = "cash-totals";
    for (const [label, value] of [
      ["Saldo inicial", open.opening_cents],
      ["Recebimentos líquidos em dinheiro", open.cash_cents],
      ["Dinheiro esperado", open.opening_cents + open.cash_cents],
      ["Cartão registrado", open.card_cents],
      ["Pix manual registrado", open.pix_cents],
    ]) {
      const item = document.createElement("div"),
        title = document.createElement("span"),
        amount = document.createElement("strong");
      title.textContent = label;
      amount.textContent = money(value);
      item.append(title, amount);
      totals.append(item);
    }
    $("cashSummary").append(totals);
  }
  $("cashOpen").classList.toggle("hidden", !canManageCatalog() || !!open);
  $("cashClose").classList.toggle("hidden", !canManageCatalog() || !open);
  $("cashPending").replaceChildren();
  for (const order of result.pending) {
    const row = document.createElement("tr");
    td(row, "#" + order.id);
    td(row, money(order.total_cents));
    td(row, money(order.received_cents));
    td(row, money(order.remaining_cents));
    const action = td(row, "");
    if (canReceiveManual()) {
      const button = document.createElement("button");
      button.textContent = "Receber";
      button.disabled = !open;
      button.setAttribute("aria-label", "Receber pedido " + order.id);
      button.onclick = () => receiptDialog(order);
      action.append(button);
    } else action.textContent = "Consulta";
    $("cashPending").append(row);
  }
  $("cashPendingEmpty").classList.toggle("hidden", !!result.pending.length);
  $("cashReceipts").replaceChildren();
  for (const entry of result.receipts) {
    const row = document.createElement("tr");
    td(row, cashDate(entry.created_at));
    td(row, "Pedido #" + entry.order_id + " · caixa #" + entry.session_id);
    td(row, cashMethods[entry.method]);
    td(row, money(entry.amount_cents));
    td(row, entry.operator + (entry.note ? " · " + entry.note : ""));
    const action = td(row, "");
    if (entry.amount_cents > 0 && !entry.refunded && canManageCatalog()) {
      const button = document.createElement("button");
      button.textContent = "Devolver";
      button.className = "btn-cancel";
      button.disabled = !open;
      button.setAttribute("aria-label", "Devolver recebimento " + entry.id);
      button.onclick = () => refundDialog(entry);
      action.append(button);
    } else
      action.textContent = entry.refund_of
        ? "Devolução #" + entry.refund_of
        : entry.refunded
          ? "Devolvido"
          : "—";
    $("cashReceipts").append(row);
  }
  $("cashReceiptsEmpty").classList.toggle("hidden", !!result.receipts.length);
  $("cashHistory").replaceChildren();
  for (const s of result.sessions) {
    const row = document.createElement("tr");
    td(row, "#" + s.id);
    td(
      row,
      cashDate(s.opened_at) +
        " · " +
        s.opened_by +
        (s.opening_note ? " · " + s.opening_note : ""),
    );
    td(row, cashDate(s.closed_at) + (s.closed_by ? " · " + s.closed_by : ""));
    td(row, money(s.cash_cents));
    td(row, money(s.card_cents));
    td(row, money(s.pix_cents));
    td(row, money(s.expected_cents ?? s.opening_cents + s.cash_cents));
    td(row, s.counted_cents == null ? "—" : money(s.counted_cents));
    td(
      row,
      s.difference_cents == null
        ? "—"
        : money(s.difference_cents) + (s.note ? " · " + s.note : ""),
    );
    $("cashHistory").append(row);
  }
}
function cashDialog(title, body, submitText, onSubmit) {
  clearMsg();
  let dialog = $("cashDialog");
  if (!dialog) {
    dialog = document.createElement("dialog");
    dialog.id = "cashDialog";
    dialog.className = "dialog";
    dialog.setAttribute("aria-labelledby", "cashDialogTitle");
    document.body.append(dialog);
  }
  dialog.innerHTML =
    '<div class="dialog-header"><h2 id="cashDialogTitle"></h2><button type="button" class="icon-button" aria-label="Fechar operação de caixa">✕</button></div><div class="dialog-body"><form id="cashForm"></form></div>';
  $("cashDialogTitle").textContent = title;
  dialog.querySelector("button").onclick = () => dialog.close();
  const form = $("cashForm");
  form.innerHTML =
    body +
    '<label><input type="checkbox" name="confirmed" style="width:auto" required> Conferi os valores e confirmo esta operação</label><button id="cashSubmit" type="submit"></button><p role="alert" class="dialog-message hidden"></p>';
  $("cashSubmit").textContent = submitText;
  const key = crypto.randomUUID();
  form.onsubmit = (e) => {
    e.preventDefault();
    run(async () => {
      await onSubmit(new FormData(form), key);
      dialog.close();
      await refresh();
      await loadCash();
      msg("Operação registrada no caixa.");
    }, $("cashSubmit"));
  };
  dialog.showModal();
}
const cashAmountField = (label, name, value) =>
  "<label>" +
  label +
  '<input name="' +
  name +
  '" required inputmode="decimal" pattern="[0-9]+([,.][0-9]{1,2})?" value="' +
  (value / 100).toFixed(2).replace(".", ",") +
  '"></label>';
const cashNoteField =
  '<label>Motivo / observação<textarea name="note" maxlength="1000" rows="2"></textarea></label>';
$("cashOpen").onclick = () =>
  cashDialog(
    "Abrir caixa",
    cashAmountField("Saldo inicial em dinheiro (R$)", "amount", 0) +
      cashNoteField,
    "Confirmar abertura",
    (d, key) =>
      api(
        "cash-sessions",
        {
          action: "open",
          opening_cents: parsePrice(d.get("amount")),
          note: d.get("note"),
        },
        { "Idempotency-Key": key },
      ),
  );
$("cashClose").onclick = () => {
  const s = cashData?.sessions.find((s) => !s.closed_at);
  if (!s) return;
  const expected = s.opening_cents + s.cash_cents;
  cashDialog(
    "Fechar caixa #" + s.id,
    "<p>Dinheiro esperado: <strong>" +
      money(expected) +
      "</strong>. Cartão e Pix ficam separados da contagem física. Informe o motivo de qualquer diferença.</p>" +
      cashAmountField("Dinheiro contado (R$)", "amount", expected) +
      cashNoteField,
    "Confirmar fechamento",
    (d, key) =>
      api(
        "cash-sessions",
        {
          action: "close",
          id: s.id,
          counted_cents: parsePrice(d.get("amount")),
          expected_cents: expected,
          note: d.get("note"),
        },
        { "Idempotency-Key": key },
      ),
  );
};
function receiptDialog(order) {
  cashDialog(
    "Receber pedido #" + order.id,
    "<p>Saldo a receber: <strong>" +
      money(order.remaining_cents) +
      '</strong>. O registro não movimenta dinheiro nem consulta o banco.</p><label>Forma de recebimento<select name="method"><option value="cash">Dinheiro</option><option value="card">Cartão conferido</option><option value="pix_manual">Pix conferido manualmente</option></select></label>' +
      cashAmountField("Valor recebido (R$)", "amount", order.remaining_cents) +
      cashNoteField,
    "Registrar recebimento",
    (d, key) =>
      api(
        "manual-receipts",
        {
          order_id: order.id,
          amount_cents: parsePrice(d.get("amount")),
          method: d.get("method"),
          note: d.get("note"),
        },
        { "Idempotency-Key": key },
      ),
  );
}
function refundDialog(entry) {
  cashDialog(
    "Registrar devolução",
    "<p>Devolução integral do recebimento #" +
      entry.id +
      ": <strong>" +
      money(entry.amount_cents) +
      "</strong> · " +
      cashMethods[entry.method] +
      ". Registre somente depois de devolver o valor ao cliente. Não executa reembolso bancário.</p>" +
      cashNoteField.replace('maxlength="1000"', 'required maxlength="1000"'),
    "Confirmar valor devolvido",
    (d, key) =>
      api(
        "manual-receipts",
        { refund_of: entry.id, note: d.get("note") },
        { "Idempotency-Key": key },
      ),
  );
}
$("cashReload").onclick = () => run(loadCash, $("cashReload"));
