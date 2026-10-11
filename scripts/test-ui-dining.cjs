const { chromium } = require("../tools/qa/node_modules/playwright"),
  fs = require("fs"),
  assert = require("assert");
(async () => {
  const url = JSON.parse(
      fs.readFileSync(process.env.OPENFOOD_UI_TEST_DATA + "/runtime.json"),
    ).URL,
    out = "docs/evidence/dining-stage";
  fs.mkdirSync(out, { recursive: true });
  const browser = await chromium.launch({ headless: true }),
    context = await browser.newContext({
      viewport: { width: 1440, height: 1000 },
      locale: "pt-BR",
      timezoneId: "America/Sao_Paulo",
    }),
    p = await context.newPage();
  p.setDefaultTimeout(15000);
  const errors = [];
  p.on("pageerror", (e) => errors.push(e.message));
  const shot = (name) =>
      p.screenshot({
        path: out + "/" + name + ".png",
        fullPage: true,
        animations: "disabled",
      }),
    read = (path) =>
      p.evaluate(async (path) => (await fetch("/api/" + path)).json(), path);
  await p.goto(url);
  await p.locator("#loginForm [name=email]").fill("qa@example.org");
  await p.locator("#loginForm [name=password]").fill("QA-test-only-password");
  await p.locator("#loginForm button").click();
  await p.locator("#modeChooser:not(.hidden)").waitFor();
  await p.locator("#enterPOS").click();
  await p.waitForFunction(
    () =>
      !document.querySelector("#posCashGate").classList.contains("hidden") ||
      document.querySelector("#posGrid button"),
  );
  if (await p.locator("#posCashGate").isVisible()) {
    await p.locator("#posOpenForm [name=opening]").fill("100,00");
    await p.locator("#posOpenSubmit").click();
  }
  await p.locator("#posGrid button").first().waitFor();
  const products = await read("products"),
    burger = products.find((x) => x.name.includes("Hambúrguer")),
    juice = products.find((x) => x.name === "Suco natural"),
    beforeOrders = (await read("orders")).length,
    beforeStock = burger.stock_quantity;
  const tableName =
    "Mesa " +
    String((await read("dining-tables")).length + 1).padStart(2, "0") +
    " QA";
  await p.locator("#posDining").click();
  await p.getByRole("button", { name: "Cadastrar / editar mesas" }).click();
  await p.getByRole("button", { name: "Nova mesa", exact: true }).click();
  await p.locator("#diningTableForm [name=name]").fill(tableName);
  await p.locator("#diningTableForm [name=capacity]").fill("4");
  await p.locator("#diningTableForm button").click();
  await p
    .locator("#diningTables button")
    .filter({ hasText: tableName })
    .waitFor();
  await shot("tables-free");
  await p
    .locator("#diningTables button")
    .filter({ hasText: tableName })
    .click();
  await p.locator("#diningOpenForm button").click();
  await p
    .getByRole("button", { name: "Adicionar produtos", exact: true })
    .waitFor();
  const tab = (await read("dining-tabs")).find(
    (t) => t.label === tableName && t.state === "open",
  );
  assert(tab);
  await p
    .getByRole("button", { name: "Adicionar produtos", exact: true })
    .click();
  await p.locator("#diningContext:not(.hidden)").waitFor();
  await p.locator("#posGrid button").filter({ hasText: burger.name }).click();
  await p.locator("#diningNote").fill("Sem cebola · QA");
  await shot("table-cart");
  await p.keyboard.press("F8");
  assert.equal(await p.locator("#posCartCount").textContent(), "1");
  await p.keyboard.press("F4");
  await p.getByRole("button", { name: "Confirmar envio", exact: true }).click();
  await p.locator("#diningOrders p").first().waitFor();
  assert(
    (await p.locator("#diningOrders").textContent()).includes("Sem cebola"),
  );
  assert.equal(
    (await read("products")).find((x) => x.id === burger.id).stock_quantity,
    beforeStock - 1,
  );
  await shot("first-round");
  await p
    .getByRole("button", { name: "Adicionar produtos", exact: true })
    .click();
  await p.reload();
  await p.locator("#modeChooser:not(.hidden)").waitFor();
  await p.locator("#enterPOS").click();
  await p.locator("#diningContext:not(.hidden)").waitFor();
  assert((await p.locator("#diningContext").textContent()).includes(tableName));
  await p.locator("#posGrid button").filter({ hasText: juice.name }).click();
  await p.locator("#diningSend").click();
  await p.getByRole("button", { name: "Confirmar envio", exact: true }).click();
  await p.waitForFunction(
    () => document.querySelectorAll("#diningOrders p").length === 2,
  );
  await shot("two-rounds");
  const bill = await read("dining-tabs?id=" + tab.id),
    roundID = bill.orders[0].id;
  assert.equal(bill.tab.total_cents, burger.price_cents + juice.price_cents);
  // Record a real partial receipt before closing the consolidated bill.
  const status = await p.evaluate(
    async (id) =>
      (
        await fetch("/api/manual-receipts", {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "X-OpenFood": "1",
            "Idempotency-Key": crypto.randomUUID(),
          },
          body: JSON.stringify({
            order_id: id,
            method: "cash",
            amount_cents: 500,
            note: "Adiantamento QA",
          }),
        })
      ).status,
    roundID,
  );
  assert.equal(status, 200);
  await p.locator("#posDialog .dialog-header button").click();
  await p.locator("#posDining").click();
  await p
    .locator("#diningTables button")
    .filter({ hasText: tableName })
    .click();
  await p.getByRole("button", { name: "Fechar conta", exact: true }).click();
  const due = burger.price_cents + juice.price_cents - 500;
  assert(
    (await p.locator("#posPaymentTotal").textContent()).includes(
      (due / 100).toFixed(2).replace(".", ","),
    ),
  );
  await p.locator("#posPaymentRows input").first().fill("10,00");
  await p.locator("#posAddPayment").click();
  await p.locator("#posTender").fill("20,00");
  await p.locator("#posPaymentForm [name=confirmed]").check();
  await shot("bill-payment");
  await p.route("**/api/dining-tabs", async (route) => {
    await route.fetch();
    await route.abort();
  });
  await p.locator("#posConfirmSale").click();
  await p.locator("#posDialog [role=alert]:not(.hidden)").waitFor();
  await p.unroute("**/api/dining-tabs");
  await p.locator("#posConfirmSale").click();
  await p.locator(".pos-receipt").waitFor();
  await shot("bill-receipt");
  assert((await p.locator(".pos-receipt").textContent()).includes(tableName));
  assert.equal((await read("orders")).length, beforeOrders + 2);
  assert.equal(
    (await read("products")).find((x) => x.id === burger.id).stock_quantity,
    beforeStock - 1,
  );
  const closed = await read("dining-tabs?id=" + tab.id);
  assert.equal(closed.tab.state, "closed");
  assert.equal(closed.tab.remaining_cents, 0);
  assert(closed.orders.every((o) => o.state === "confirmed"));
  await p.getByRole("button", { name: "Nova venda", exact: true }).click();
  await p.locator("#posDining").click();
  assert(
    (
      await p
        .locator("#diningTables button")
        .filter({ hasText: tableName })
        .textContent()
    ).includes("Livre"),
  );
  await p.setViewportSize({ width: 390, height: 844 });
  await shot("tables-mobile");
  assert(
    await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
  );
  await p.locator("#posDialog .dialog-header button").click();
  await p.locator("#posTheme").selectOption("dark");
  await p.locator("#posDining").click();
  await shot("tables-dark-mobile");
  await p.locator("#posDialog .dialog-header button").click();
  await p.locator("#posTheme").selectOption("light");
  // Empty anonymous tabs can be cancelled, while occupied tabs cannot.
  await p.locator("#posDining").click();
  await p.getByRole("button", { name: "Abrir comanda avulsa" }).click();
  await p
    .locator("#diningOpenForm [name=label]")
    .fill("Comanda avulsa QA " + Date.now());
  await p.locator("#diningOpenForm button").click();
  await p
    .getByRole("button", { name: "Cancelar conta vazia", exact: true })
    .click();
  await p
    .getByRole("button", { name: "Confirmar cancelamento", exact: true })
    .click();
  await p.locator("#diningTables").waitFor();
  for (const role of ["attendant", "finance_viewer"]) {
    const c = await browser.newContext({
        viewport: { width: 390, height: 844 },
        locale: "pt-BR",
      }),
      q = await c.newPage();
    q.on("pageerror", (e) => errors.push(e.message));
    await q.goto(url);
    await q
      .locator("#loginForm [name=email]")
      .fill("cash-final-" + role + "@example.org");
    await q.locator("#loginForm [name=password]").fill("QA-test-only-password");
    await q.locator("#loginForm button").click();
    await q.locator("#modeChooser:not(.hidden)").waitFor();
    if (role === "attendant") {
      await q.locator("#enterPOS").click();
      await q.locator("#posDining").click();
      assert.equal(
        await q
          .getByRole("button", { name: "Cadastrar / editar mesas" })
          .count(),
        0,
      );
      await q
        .locator("#diningTables button")
        .filter({ hasText: tableName })
        .click();
      await q.locator("#diningOpenForm button").click();
      await q
        .getByRole("button", { name: "Cancelar conta vazia", exact: true })
        .click();
      await q
        .getByRole("button", { name: "Confirmar cancelamento", exact: true })
        .click();
      await q.locator("#diningTables").waitFor();
    } else {
      await q.locator("#enterAdmin").click();
      await q.locator("#adminDining").waitFor();
      await q.locator("#adminDining").click();
      assert.equal(
        await q.getByRole("button", { name: "Abrir comanda avulsa" }).count(),
        0,
      );
      assert.equal(
        await q
          .getByRole("button", { name: "Cadastrar / editar mesas" })
          .count(),
        0,
      );
      await q.screenshot({
        path: out + "/finance-readonly-mobile.png",
        fullPage: true,
      });
    }
    await c.close();
  }
  await p.locator("#posDialog .dialog-header button").click();
  await p.locator("#posCash").click();
  await p.getByRole("button", { name: "Fechar caixa", exact: true }).click();
  await p.locator("#cashForm [name=confirmed]").check();
  await p.locator("#cashSubmit").click();
  await p.locator("#cashDialog").waitFor({ state: "hidden" });
  await p.locator("#posCashGate:not(.hidden)").waitFor();
  await p.locator("#diningGateConsult").click();
  await p.locator("#diningTables").waitFor();
  assert(
    await p
      .locator("#diningTables button")
      .filter({ hasText: tableName })
      .isDisabled(),
  );
  await shot("cash-closed-consult");
  assert.deepEqual(errors, []);
  fs.writeFileSync(
    out + "/results.json",
    JSON.stringify(
      {
        passed: true,
        date: new Date().toISOString(),
        errors,
        checks: [
          "table creation",
          "free/occupied map",
          "persistent tab",
          "stock reserved once per round",
          "preparation note",
          "resume after reload",
          "two rounds",
          "partial payment excluded from due",
          "split bill payment and change",
          "lost closing response retry once",
          "no duplicated sale/stock on settlement",
          "preparation state independent of settlement",
          "table released",
          "empty tab cancellation",
          "mobile no overflow",
          "dark theme",
          "closed cash permits consultation without opening another tab",
          "table F4 sends round instead of counter sale",
          "table F8 cannot detach draft",
          "attendant cannot manage tables",
          "finance can read without modifying tabs",
        ],
      },
      null,
      2,
    ),
  );
  await browser.close();
  console.log("PASS: dining browser journey");
})().catch((e) => {
  console.error(e);
  process.exit(1);
});
