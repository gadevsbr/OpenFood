const { chromium } = require("../tools/qa/node_modules/playwright"),
  fs = require("fs"),
  assert = require("assert");
(async () => {
  const { URL: url } = JSON.parse(
    fs.readFileSync(process.env.OPENFOOD_UI_TEST_DATA + "/runtime.json"),
  );
  const browser = await chromium.launch({ headless: true }),
    p = await browser.newPage({
      viewport: { width: 1440, height: 1000 },
      locale: "pt-BR",
      timezoneId: "America/Sao_Paulo",
    });
  p.setDefaultTimeout(15000);
  const errors = [];
  p.on("pageerror", (e) => errors.push(e.message));
  await p.goto(url);
  await p.locator("#loginForm [name=email]").fill("qa@example.org");
  await p.locator("#loginForm [name=password]").fill("QA-test-only-password");
  await p.locator("#loginForm button").click();
  await p.locator("#workspace:not(.hidden)").waitFor();
  const read = (path) =>
    p.evaluate(async (path) => (await fetch("/api/" + path)).json(), path);
  const post = (path, payload, key) =>
    p.evaluate(
      async ({ path, payload, key }) => {
        const r = await fetch("/api/" + path, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "X-OpenFood": "1",
            "Idempotency-Key": key || crypto.randomUUID(),
          },
          body: JSON.stringify(payload),
        });
        return { status: r.status, body: await r.json() };
      },
      { path, payload, key },
    );
  const products = await read("products"),
    burger = products.find((x) => x.name.includes("Hambúrguer")),
    juice = products.find((x) => x.name === "Suco natural");
  await p.locator("#pageAction").click();
  await p.locator("#productSelect").selectOption(String(burger.id));
  await p.locator("#orderForm [name=quantity]").fill("2");
  await p.locator("#addOrderItem").click();
  await p.locator("#productSelect").selectOption(String(juice.id));
  await p.locator("#addOrderItem").click();
  await p.locator("#orderForm input[type=checkbox]").check();
  await p.locator("#createOrderSubmit").click();
  await p.locator("#orderDialog").waitFor({ state: "hidden" });
  const order = (await read("orders"))[0];
  assert.equal(order.total_cents, 6930);
  await p.locator("#tabPagamentos").click();
  await p.locator("#cashOpen").waitFor({ state: "visible" });
  await p.locator("#cashOpen").click();
  await p.locator("#cashForm [name=amount]").fill("100,00");
  await p.locator("#cashForm [name=note]").fill("Abertura QA");
  await p.locator("#cashForm [name=confirmed]").check();
  await p.locator("#cashSubmit").click();
  await p.locator("#cashDialog").waitFor({ state: "hidden" });
  await p.locator("#cashClose").waitFor({ state: "visible" });
  const receipt = (method, amount) =>
    p
      .locator("#cashPending tr")
      .filter({ hasText: "#" + order.id })
      .getByRole("button", { name: "Receber pedido " + order.id })
      .click()
      .then(async () => {
        await p.locator("#cashForm [name=method]").selectOption(method);
        await p.locator("#cashForm [name=amount]").fill(amount);
        await p.locator("#cashForm [name=confirmed]").check();
        await p.locator("#cashSubmit").click();
        await p.locator("#cashDialog").waitFor({ state: "hidden" });
      });
  await p.setViewportSize({ width: 390, height: 844 });
  await receipt("cash", "20,00");
  await p.setViewportSize({ width: 1440, height: 1000 });
  await receipt("card", "30,00");
  await receipt("pix_manual", "19,30");
  await p
    .locator("#cashPending tr")
    .filter({ hasText: "#" + order.id })
    .waitFor({ state: "hidden" });
  let data = await read("cash-sessions");
  assert.equal(data.sessions[0].cash_cents, 2000);
  assert.equal(data.sessions[0].card_cents, 3000);
  assert.equal(data.sessions[0].pix_cents, 1930);
  assert.equal(
    (await read("orders")).find((x) => x.id === order.id).financial_state,
    "manual_confirmed",
  );
  await p.screenshot({
    path: "docs/evidence/cash-stage/cash-open-desktop.png",
    fullPage: true,
  });
  assert.equal(
    (await post("order-state", { id: order.id, state: "cancelled" })).status,
    409,
  );
  await p.locator("#cashClose").click();
  await p.locator("#cashForm [name=amount]").fill("119,50");
  await p.locator("#cashForm [name=confirmed]").check();
  await p.locator("#cashSubmit").click();
  await p
    .locator("#cashDialog [role=alert]")
    .getByText(/motivo da diferença/)
    .waitFor();
  await p
    .locator("#cashForm [name=note]")
    .fill("Diferença de R$ 0,50 QA conferida");
  await p.screenshot({
    path: "docs/evidence/cash-stage/cash-close-desktop.png",
  });
  await p.locator("#cashSubmit").click();
  await p.locator("#cashDialog").waitFor({ state: "hidden" });
  await p.locator("#cashOpen").waitFor({ state: "visible" });
  data = await read("cash-sessions");
  const closed = data.sessions[0];
  assert.equal(closed.expected_cents, 12000);
  assert.equal(closed.counted_cents, 11950);
  assert.equal(closed.difference_cents, -50);
  await p.setViewportSize({ width: 390, height: 844 });
  await p.screenshot({
    path: "docs/evidence/cash-stage/cash-closed-mobile.png",
    fullPage: true,
  });
  assert(
    await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
  );
  await p.setViewportSize({ width: 1440, height: 1000 });
  // Refund all three receipts in a new session; immutable closed period totals remain unchanged.
  await p.locator("#cashOpen").click();
  await p.locator("#cashForm [name=amount]").fill("100,00");
  await p.locator("#cashForm [name=confirmed]").check();
  await p.locator("#cashSubmit").click();
  await p.locator("#cashDialog").waitFor({ state: "hidden" });
  for (const entry of data.receipts.filter(
    (x) => x.order_id === order.id && x.amount_cents > 0,
  )) {
    await p
      .getByRole("button", {
        name: "Devolver recebimento " + entry.id,
        exact: true,
      })
      .click();
    await p
      .locator("#cashForm [name=note]")
      .fill("Devolução QA confirmada ao cliente");
    await p.locator("#cashForm [name=confirmed]").check();
    await p.locator("#cashSubmit").click();
    await p.locator("#cashDialog").waitFor({ state: "hidden" });
  }
  data = await read("cash-sessions");
  assert.equal(data.sessions.find((x) => x.id === closed.id).cash_cents, 2000);
  assert.equal(data.sessions[0].cash_cents, -2000);
  assert.equal(data.sessions[0].card_cents, -3000);
  assert.equal(data.sessions[0].pix_cents, -1930);
  assert.equal(
    (await post("order-state", { id: order.id, state: "cancelled" })).status,
    200,
  );
  const ledgerCount = data.receipts.length;
  const other = (await read("orders")).find((x) => x.state !== "cancelled");
  const payload = {
      order_id: other.id,
      amount_cents: 100,
      method: "cash",
      note: "Idempotência QA",
    },
    key = "qa-ui-receipt-" + Date.now();
  const first = await post("manual-receipts", payload, key),
    again = await post("manual-receipts", payload, key);
  assert.equal(first.status, 200);
  assert.deepEqual(first, again);
  assert.equal((await read("cash-sessions")).receipts.length, ledgerCount + 1);
  await p.locator("#cashReload").click();
  await p.screenshot({
    path: "docs/evidence/cash-stage/refunds-desktop.png",
    fullPage: true,
  });
  await p.locator("#themeSelect").selectOption("dark");
  await p.screenshot({
    path: "docs/evidence/cash-stage/cash-dark-desktop.png",
    fullPage: true,
  });
  await p.locator("#themeSelect").selectOption("light");
  assert.deepEqual(errors, []);
  fs.writeFileSync(
    "docs/evidence/cash-stage/cash-results.json",
    JSON.stringify(
      {
        status: "PASS",
        date: "2026-10-11",
        environment:
          "Chromium headless + PostgreSQL real, isolated synthetic QA data",
        checks: [
          "cash opening",
          "multi-item sale",
          "cash/card/manual Pix split receipts",
          "paid state",
          "paid cancellation rejected",
          "closing difference requires reason",
          "expected/counted/difference reconciliation",
          "mobile page overflow",
          "refunds into new session preserve closed period",
          "cancellation after refunds",
          "duplicate receipt exactly once",
        ],
        pageerrors: errors,
      },
      null,
      2,
    ),
  );
  await browser.close();
  console.log("PASS: cash browser journey");
})().catch((e) => {
  console.error(e);
  process.exit(1);
});
