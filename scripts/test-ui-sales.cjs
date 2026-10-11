const { chromium } = require("../tools/qa/node_modules/playwright"),
  fs = require("fs"),
  assert = require("assert");
(async () => {
  const data = process.env.OPENFOOD_UI_TEST_DATA,
    { URL: url } = JSON.parse(fs.readFileSync(data + "/runtime.json"));
  const b = await chromium.launch({ headless: true }),
    p = await b.newPage({
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
  await p.locator("#modeChooser:not(.hidden)").waitFor();
  await p.locator("#enterAdmin").click();
  await p.locator("#workspace:not(.hidden)").waitFor();
  if (!(await p.locator("#tabOperacao").isVisible()))
    await p.locator("#menuToggle").click();
  await p.locator("#tabOperacao").click();
  const products = await p.evaluate(async () =>
      (await fetch("/api/products")).json(),
    ),
    burger = products.find((x) => x.name === "Hambúrguer da casa"),
    juice = products.find((x) => x.name === "Suco natural");
  await p.locator("#tabCatalogo").click();
  const row = p
    .locator("#advProducts tr")
    .filter({ hasText: "Hambúrguer da casa" });
  await row.getByRole("button", { name: "Editar", exact: true }).click();
  await p.locator("#advProductForm [name=price]").fill("31,50");
  await p.locator("#advProductForm [name=name]").fill("Hambúrguer revisado QA");
  await p.locator("#advProductSubmit").click();
  await p.locator("#productDialog").waitFor({ state: "hidden" });
  await p
    .locator("#advProducts tr")
    .filter({ hasText: "Hambúrguer revisado QA" })
    .waitFor();
  await p
    .locator("#advProducts tr")
    .filter({ hasText: "Hambúrguer revisado QA" })
    .getByRole("button", { name: "Editar", exact: true })
    .click();
  await p.screenshot({
    path:
      (process.env.OPENFOOD_SALES_EVIDENCE || "docs/evidence/sales-stage") +
      "/product-edit-desktop.png",
  });
  await p.keyboard.press("Escape");
  if (!(await p.locator("#tabOperacao").isVisible()))
    await p.locator("#menuToggle").click();
  await p.locator("#tabOperacao").click();
  await p.locator("#pageAction").click();
  await p.locator("#productSelect").selectOption(String(burger.id));
  await p.locator("#orderForm [name=quantity]").fill("2");
  await p.locator("#addOrderItem").click();
  await p.locator("#productSelect").selectOption(String(juice.id));
  await p.locator("#orderForm [name=quantity]").fill("3");
  await p.locator("#addOrderItem").click();
  assert.equal(await p.locator("#orderCart .cart-row").count(), 2);
  await p.locator("#orderForm input[type=checkbox]").check();
  assert.match(await p.locator("#orderPreviewTotal").innerText(), /91,50/);
  await p.screenshot({
    path:
      (process.env.OPENFOOD_SALES_EVIDENCE || "docs/evidence/sales-stage") +
      "/cart-desktop.png",
  });
  await p.locator("#createOrderSubmit").click();
  await p.locator("#orderDialog").waitFor({ state: "hidden" });
  const orders = await p.evaluate(async () =>
      (await fetch("/api/orders")).json(),
    ),
    order = orders[0];
  assert.equal(order.total_cents, 9150);
  const items = await p.evaluate(
    async (id) => (await fetch("/api/orders?id=" + id)).json(),
    order.id,
  );
  assert.equal(items.length, 2);
  assert.equal(items.find((x) => x.product_id === burger.id).price_cents, 3150);
  await p
    .locator("#orders tr")
    .first()
    .getByRole("button", { name: /Ver resumo/ })
    .click();
  await p
    .locator("#orderDetailItems")
    .getByText(/Hambúrguer revisado QA/)
    .waitFor();
  await p.screenshot({
    path:
      (process.env.OPENFOOD_SALES_EVIDENCE || "docs/evidence/sales-stage") +
      "/order-items-desktop.png",
  });
  await p.setViewportSize({ width: 390, height: 844 });
  await p.screenshot({
    path:
      (process.env.OPENFOOD_SALES_EVIDENCE || "docs/evidence/sales-stage") +
      "/order-items-mobile.png",
  });
  assert(
    await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
  );
  await p.keyboard.press("Escape");
  await p.setViewportSize({ width: 1440, height: 1000 });
  p.once("dialog", (d) => d.accept());
  await p
    .locator("#orders tr")
    .first()
    .getByRole("button", { name: /Cancelar pedido/ })
    .click();
  await p
    .locator("#orders tr")
    .filter({ hasText: "#" + String(order.id).padStart(4, "0") })
    .waitFor({ state: "hidden" });
  const current = await p.evaluate(async () =>
    (await fetch("/api/products")).json(),
  );
  assert.equal(
    current.find((x) => x.id === burger.id).stock_quantity,
    burger.stock_quantity,
  );
  assert.equal(
    current.find((x) => x.id === juice.id).stock_quantity,
    juice.stock_quantity,
  );
  // A real order changes stock while the edit form remains open; saving must conflict.
  await p.locator("#tabCatalogo").click();
  await p
    .locator("#advProducts tr")
    .filter({ hasText: "Hambúrguer revisado QA" })
    .getByRole("button", { name: "Editar", exact: true })
    .click();
  const conflictOrder = await p.evaluate(async (id) => {
    const r = await fetch("/api/orders", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-OpenFood": "1",
        "Idempotency-Key": crypto.randomUUID(),
      },
      body: JSON.stringify({ items: [{ product_id: id, quantity: 1 }] }),
    });
    if (!r.ok) throw Error(await r.text());
    return r.json();
  }, burger.id);
  await p.locator("#advProductSubmit").click();
  await p
    .locator("#productDialog [role=alert]")
    .getByText(/estoque mudou/)
    .waitFor();
  assert(await p.locator("#productDialog").isVisible());
  await p.keyboard.press("Escape");
  await p.evaluate(async (id) => {
    const r = await fetch("/api/order-state", {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-OpenFood": "1" },
      body: JSON.stringify({ id, state: "cancelled" }),
    });
    if (!r.ok) throw Error(await r.text());
  }, conflictOrder.id);
  assert.deepEqual(errors, []);
  fs.writeFileSync(
    (process.env.OPENFOOD_SALES_EVIDENCE || "docs/evidence/sales-stage") +
      "/sales-results.json",
    JSON.stringify(
      {
        result: "PASS",
        checks: [
          "product edit",
          "mixed cart and integer cents total",
          "snapshot details",
          "cancellation restores both stocks",
          "stale editor rejects intervening sale",
          "desktop/mobile detail",
        ],
        pageerrors: errors,
      },
      null,
      2,
    ),
  );
  await b.close();
  console.log("PASS: sales browser journey");
})().catch((e) => {
  console.error(e);
  process.exit(1);
});
