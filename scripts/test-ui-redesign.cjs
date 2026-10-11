/* Real local PostgreSQL UI journey. Fixtures are synthetic; backend responses are real. */
const { chromium } = require("../tools/qa/node_modules/playwright");
const fs = require("fs"),
  path = require("path"),
  assert = require("assert");
const root = path.resolve(__dirname, ".."),
  data =
    process.env.OPENFOOD_UI_TEST_DATA ||
    path.join(root, "runtime", "ui-redesign-20261010");
const evidence =
  process.env.OPENFOOD_UI_EVIDENCE ||
  path.join(root, "docs", "evidence", "ui-redesign");
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
(async () => {
  for (
    let i = 0;
    i < 80 && !fs.existsSync(path.join(data, "runtime.json"));
    i++
  )
    await sleep(1000);
  const { URL: url } = JSON.parse(
    fs.readFileSync(path.join(data, "runtime.json"), "utf8"),
  );
  const config = JSON.parse(
    fs.readFileSync(path.join(data, "config.json"), "utf8"),
  );
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({
    viewport: { width: 1440, height: 1000 },
    locale: "pt-BR",
    timezoneId: "America/Sao_Paulo",
  });
  page.setDefaultTimeout(12000);
  fs.mkdirSync(evidence, { recursive: true });
  const errors = [],
    failures = [];
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("response", (r) => {
    if (r.status() >= 400 && r.status() !== 401)
      failures.push({ path: new URL(r.url()).pathname, status: r.status() });
  });
  await page.goto(url + "/#setup=" + config.setup_token);
  await page.locator("#setup:not(.hidden)").waitFor();
  await page.screenshot({
    path: path.join(evidence, "setup-desktop.png"),
    fullPage: true,
    animations: "disabled",
  });
  await page
    .locator("#setupForm [name=organization]")
    .fill("Operação de teste QA");
  await page.locator("#setupForm [name=store]").fill("Cozinha de teste · QA");
  await page.locator("#setupForm [name=email]").fill("qa@example.org");
  await page
    .locator("#setupForm [name=password]")
    .fill("QA-test-only-password");
  await page.locator("#setupForm button").click();
  await page.locator("#login:not(.hidden)").waitFor();
  await page.screenshot({
    path: path.join(evidence, "login-desktop.png"),
    fullPage: true,
    animations: "disabled",
  });
  async function login() {
    await page.locator("#loginForm [name=email]").fill("qa@example.org");
    await page
      .locator("#loginForm [name=password]")
      .fill("QA-test-only-password");
    await page.locator("#loginForm button").click();
    await page.locator("#workspace:not(.hidden)").waitFor();
  }
  await login();
  await page.locator("#ordersEmptyState:not(.hidden)").waitFor();
  await page.screenshot({
    path: path.join(evidence, "orders-empty.png"),
    fullPage: true,
    animations: "disabled",
  });
  await page.locator("#tabCatalogo").click();
  await page.locator("#catalogEmptyState:not(.hidden)").waitFor();
  for (const [name, price, stock] of [
    ["Hambúrguer da casa", "29,90", "30"],
    ["Porção de batatas", "18,00", "20"],
    ["Suco natural", "9,50", "25"],
    ["Sobremesa do dia", "14,00", "0"],
  ]) {
    await page.locator("#pageAction").click();
    await page.locator("#productDialog").waitFor({ state: "visible" });
    await page.locator("#advProductForm [name=name]").fill(name);
    await page.locator("#advProductForm [name=price]").fill(price);
    await page.locator("#advProductForm details summary").click();
    await page.locator("#advProductForm [name=stock]").fill(stock);
    await page.locator("#advProductSubmit").click();
    await page.locator("#productDialog").waitFor({ state: "hidden" });
    await page.locator("#advProducts tr").filter({ hasText: name }).waitFor();
  }
  await page.locator("#productSearch").fill("batatas");
  assert.equal(await page.locator("#advProducts tr").count(), 1);
  await page.locator("#productSearch").fill("");
  await page.locator("#productAvailability").selectOption("unavailable");
  assert.equal(await page.locator("#advProducts tr").count(), 1);
  await page.locator("#productAvailability").selectOption("all");
  await page.screenshot({
    path: path.join(evidence, "catalog-desktop.png"),
    fullPage: true,
    animations: "disabled",
  });
  await page.locator("#tabOperacao").click();
  for (let i = 0; i < 4; i++) {
    await page.locator("#pageAction").click();
    await page.locator("#orderDialog").waitFor({ state: "visible" });
    await page
      .locator("#productSelect")
      .selectOption({ label: "Hambúrguer da casa · R$\u00a029,90" });
    await page.locator("#orderForm [name=quantity]").fill(String(i + 1));
    assert(
      (await page.locator("#orderPreviewTotal").textContent()).includes(
        (29.9 * (i + 1)).toFixed(2).replace(".", ","),
      ),
    );
    await page.locator("#orderForm input[type=checkbox]").check();
    await page.locator("#createOrderSubmit").click();
    await page.locator("#orderDialog").waitFor({ state: "hidden" });
    await page.locator("#orders tr").nth(i).waitFor();
  }
  // Prepare one and ready another through visible row actions.
  await page
    .locator("#orders tr")
    .nth(0)
    .getByRole("button", { name: /Em preparo/ })
    .click();
  await page
    .locator("#orders tr")
    .nth(0)
    .getByText("Em preparo", { exact: true })
    .waitFor();
  await page
    .locator("#orders tr")
    .nth(1)
    .getByRole("button", { name: /Em preparo/ })
    .click();
  await page
    .locator("#orders tr")
    .nth(1)
    .getByRole("button", { name: /Pronto/ })
    .click();
  await page
    .locator("#orders tr")
    .nth(1)
    .getByText("Pronto", { exact: true })
    .waitFor();
  await page.locator("[data-order-filter=ready]").click();
  assert.equal(await page.locator("#orders tr").count(), 1);
  await page.locator("[data-order-filter=active]").click();
  await page.locator("#orderSearch").fill("999999");
  assert(await page.locator("#ordersEmptyState").isVisible());
  await page.locator("#orderSearch").fill("");
  for (const [name, width, height] of [
    ["desktop", 1440, 1000],
    ["tablet", 768, 1024],
    ["mobile", 390, 844],
  ]) {
    await page.setViewportSize({ width, height });
    await page.screenshot({
      path: path.join(evidence, "orders-" + name + ".png"),
      fullPage: true,
      animations: "disabled",
    });
    assert(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
      "horizontal page overflow " + name,
    );
    if (name === "mobile") {
      await page
        .locator("#orders tr")
        .first()
        .getByRole("button", { name: /Ver resumo/ })
        .click();
      await page
        .locator("#orderDetailsAmount")
        .getByText("R$ 119,60")
        .waitFor();
      await page.keyboard.press("Escape");
      await page.locator("#menuToggle").click();
      await page.locator("#tabCatalogo").click();
      assert.equal(
        await page.locator("#menuToggle").getAttribute("aria-expanded"),
        "false",
      );
      await page.screenshot({
        path: path.join(evidence, "catalog-mobile.png"),
        fullPage: true,
        animations: "disabled",
      });
      await page.locator("#menuToggle").click();
      await page.locator("#tabOperacao").click();
    }
  }
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.locator("#themeSelect").selectOption("dark");
  await page.screenshot({
    path: path.join(evidence, "orders-dark.png"),
    fullPage: true,
    animations: "disabled",
  });
  await page.reload();
  await page.locator("#workspace:not(.hidden)").waitFor();
  assert.equal(await page.locator("html").getAttribute("data-theme"), "dark");
  await page.locator("#themeSelect").selectOption("light");
  await page.locator("#pageAction").click();
  await page.locator("#productSelect").waitFor({ state: "visible" });
  await page.keyboard.press("Escape");
  assert(!(await page.locator("#orderDialog").isVisible()));
  assert.equal(
    await page.evaluate(() => document.activeElement.id),
    "pageAction",
  );
  await page.locator("#tabPagamentos").click();
  assert(
    await page.getByText("Em desenvolvimento", { exact: true }).isVisible(),
  );
  assert(!(await page.locator("#paymentCreateForm").isVisible()));
  await page.locator("#tabOperacao").click();
  await page.context().setOffline(true);
  await page.locator("#refresh").click();
  await page.locator("#connectionNotice:not(.hidden)").waitFor();
  assert.equal(await page.locator("#orders tr").count(), 4);
  await page.context().setOffline(false);
  await page.locator("#retryConnection").click();
  await page.locator("#connectionNotice").waitFor({ state: "hidden" });
  await page.locator("#tabUsuarios").click();
  await page.locator("#usersTable").getByText("qa@example.org").waitFor();
  await page.screenshot({
    path: path.join(evidence, "team-desktop.png"),
    fullPage: true,
    animations: "disabled",
  });
  await page.locator("#tabManutencao").click();
  await page.screenshot({
    path: path.join(evidence, "system-desktop.png"),
    fullPage: true,
    animations: "disabled",
  });
  const [download] = await Promise.all([
    page.waitForEvent("download"),
    page.locator("#backup").click(),
  ]);
  const dump = path.join(root, "runtime", "ui-redesign-backup.dump");
  await download.saveAs(dump);
  assert.equal(fs.readFileSync(dump).subarray(0, 5).toString(), "PGDMP");
  await page.locator("#restoreFile").setInputFiles(dump);
  await page.locator("#restoreConfirm").fill("RESTAURAR");
  await page.locator("#restore").click();
  await page.locator("#login:not(.hidden)").waitFor({ timeout: 30000 });
  await login();
  assert.equal(await page.locator("#orders tr").count(), 4);
  const results = await page.evaluate(async () => ({
    orders: await (await fetch("/api/orders")).json(),
    products: await (await fetch("/api/products")).json(),
  }));
  assert.equal(
    results.products.find((p) => p.name === "Hambúrguer da casa")
      .stock_quantity,
    20,
  );
  assert.equal(
    results.orders.find((o) => o.total_cents === 11960).total_cents,
    11960,
  );
  assert.equal(errors.length, 0, errors.join("\n"));
  assert.equal(failures.length, 0, JSON.stringify(failures));
  const report = {
    date: "2026-10-10",
    status: "PASS",
    environment:
      "Windows de desenvolvimento, Chromium headless, PostgreSQL 17 real e dados de teste isolados. Sem validação de provedor ou ambiente limpo.",
    passed: [
      "setup",
      "login",
      "empty states",
      "product creation via real backend",
      "catalog search and availability",
      "manual orders and integer cents",
      "stock consumption",
      "prepare and ready transitions",
      "order search and filters",
      "desktop/tablet/mobile page overflow",
      "mobile navigation",
      "dark theme persistence",
      "dialog Escape and focus return",
      "payments visibly unavailable",
      "team list",
      "backup and restore with session revocation and preserved data",
    ],
    javascript_errors: errors,
    http_failures: failures,
  };
  fs.writeFileSync(
    path.join(evidence, "results.json"),
    JSON.stringify(report, null, 2),
  );
  console.log(JSON.stringify(report));
  await browser.close();
})().catch((e) => {
  console.error(e.stack);
  process.exit(1);
});
