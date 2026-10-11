const { chromium } = require("../tools/qa/node_modules/playwright");
const fs = require("fs"),
  assert = require("assert");
(async () => {
  const url = JSON.parse(
    fs.readFileSync(process.env.OPENFOOD_UI_TEST_DATA + "/runtime.json"),
  ).URL;
  const out = "docs/evidence/pos-stage";
  fs.mkdirSync(out, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1000 },
    locale: "pt-BR",
    timezoneId: "America/Sao_Paulo",
  });
  const p = await context.newPage();
  p.setDefaultTimeout(15000);
  const errors = [];
  p.on("pageerror", (e) => errors.push(e.message));
  const shot = async (name) =>
    p.screenshot({
      path: out + "/" + name + ".png",
      fullPage: true,
      animations: "disabled",
    });
  await p.goto(url);
  await p.locator("#loginForm [name=email]").fill("qa@example.org");
  await p.locator("#loginForm [name=password]").fill("QA-test-only-password");
  await p.locator("#loginForm button").click();
  await p.locator("#modeChooser:not(.hidden)").waitFor();
  await shot("mode-desktop");
  assert.equal(await p.locator("#enterPOS").isEnabled(), true);
  await p.locator("#enterAdmin").click();
  await p.locator("#dashboardStats strong").first().waitFor();
  await shot("admin-dashboard");
  await p.evaluate(async () => {
    const post = async (path, data, method = "POST") => {
      const r = await fetch("/api/" + path, {
        method,
        headers: { "Content-Type": "application/json", "X-OpenFood": "1" },
        body: JSON.stringify(data),
      });
      if (!r.ok) throw Error(await r.text());
      return r.json();
    };
    let categories = await (await fetch("/api/categories")).json();
    for (const name of ["Lanches", "Porções", "Bebidas"])
      if (!categories.some((c) => c.name === name))
        await post("categories", {
          name,
          active: true,
          display_order: categories.length,
        });
    categories = await (await fetch("/api/categories")).json();
    const products = await (await fetch("/api/products")).json();
    for (const product of products) {
      const name = product.name.includes("Hambúrguer")
        ? "Lanches"
        : product.name.includes("batatas")
          ? "Porções"
          : product.name.includes("Suco")
            ? "Bebidas"
            : null;
      if (!name) continue;
      await post(
        "products",
        {
          ...product,
          category_id: categories.find((c) => c.name === name).id,
          expected_stock_quantity: product.stock_quantity,
          available_from: product.available_from || "",
          available_until: product.available_until || "",
        },
        "PATCH",
      );
    }
  });
  await p.locator("#adminToPOS").click();
  await p.waitForFunction(()=>!document.querySelector('#posCashGate').classList.contains('hidden')||document.querySelector('#posGrid button'));
  if(await p.locator('#posCashGate').isVisible()){await p.locator('#posOpenForm [name=opening]').fill('100,00');await p.locator('#posOpenSubmit').click();}
  await p.locator("#posGrid button").first().waitFor();
  assert(await p.locator(".sidebar").isHidden());
  await shot("pos-desktop");
  await p
    .locator("#posCategories button")
    .filter({ hasText: "Bebidas" })
    .click();
  assert.equal(await p.locator("#posGrid button").count(), 1);
  await p.locator("#posCategories button").filter({ hasText: "Todos" }).click();
  await p.keyboard.press("F2");
  assert.equal(
    await p
      .locator("#posSearch")
      .evaluate((el) => el === document.activeElement),
    true,
  );
  await p.locator("#posSearch").fill("Suco");
  await p.locator("#posGrid button").first().click();
  await p.locator("#posSearch").fill("");
  await p.locator("#posHold").click();
  assert.equal(await p.locator("#posCartCount").textContent(), "0");
  await p.locator("#posHeld").click();
  await p.locator("#posDialog .dialog-body button").first().click();
  assert.equal(await p.locator("#posCartCount").textContent(), "1");
  await p.locator("#posGrid button").filter({ hasText: "Hambúrguer" }).click();
  await p.locator("#posDiscountSection").evaluate((el) => (el.open = true));
  await p.locator("#posDiscount").fill("1,00");
  await p.locator("#posDiscountNote").fill("Desconto QA PDV");
  await p.locator("#posService").selectOption("kitchen");
  await shot("pos-cart");
  await p.keyboard.press("F4");
  await p.locator("#posPaymentRows input").first().fill("10,00");
  await p.locator("#posAddPayment").click();
  await p.locator("#posTender").fill("20,00");
  assert((await p.locator("#posChange").textContent()).includes("10,00"));
  await shot("split-payment");
  await p.locator("#posPaymentForm [name=confirmed]").check();
  await p.locator("#posConfirmSale").click();
  await p.locator(".pos-receipt").waitFor();
  assert(
    (await p.locator(".pos-receipt").textContent()).includes(
      "COMPROVANTE NÃO FISCAL",
    ),
  );
  await shot("receipt");
  await p.getByRole("button", { name: "Nova venda", exact: true }).click();
  await p.locator("#posTheme").selectOption("dark");
  await shot("pos-dark");
  assert.equal(
    await p.evaluate(() => document.documentElement.dataset.theme),
    "dark",
  );
  await p.locator("#posTheme").selectOption("light");
  await context.setOffline(true);
  await p.locator("#posRecent").click();
  assert(await p.locator("#posDialog").isVisible());
  await p.locator("#posDialog .dialog-header button").click();
  await context.setOffline(false);
  await p.emulateMedia({ reducedMotion: "reduce" });
  await p.setViewportSize({ width: 390, height: 844 });
  await shot("pos-mobile");
  assert(
    await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
  );
  await p
    .locator("#posGrid button")
    .filter({ hasText: "Suco natural" })
    .click();
  await p.locator("#posMobileCart").click();
  await shot("cart-mobile");
  assert(await p.locator("#posPay").isVisible());
  assert(
    await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
  );
  await p.locator("#posBackProducts").click();
  await p.locator("#posToAdmin").click();
  assert(await p.locator("#workspace").isVisible());
  await p.locator("#menuToggle").click();
  await p.locator("#logout").click();
  await p.locator("#loginForm [name=email]").fill("qa@example.org");
  await p.locator("#loginForm [name=password]").fill("QA-test-only-password");
  await p.locator("#loginForm button").click();
  await p.locator("#modeChooser:not(.hidden)").waitFor();
  await shot("mode-mobile");
  await p.setViewportSize({ width: 1440, height: 1000 });
  await p.locator("#enterPOS").click();
  await p.locator("#posCash.open").waitFor();
  // Simulate a lost response after the server commits. Retrying must reuse the same key.
  const beforeSales = await p.evaluate(
    async () => (await (await fetch("/api/orders")).json()).length,
  );
  await p.locator("#posService").selectOption("counter");
  await p.locator("#posPay").click();
  await p.locator("#posPaymentForm [name=confirmed]").check();
  await p.route("**/api/pos-checkout", async (route) => {
    await route.fetch();
    await route.abort();
  });
  await p.locator("#posConfirmSale").click();
  await p.locator("#posDialog [role=alert]:not(.hidden)").waitFor();
  await p.unroute("**/api/pos-checkout");
  await p.locator("#posConfirmSale").click();
  await p.locator(".pos-receipt").waitFor();
  assert.equal(
    await p.evaluate(
      async () => (await (await fetch("/api/orders")).json()).length,
    ),
    beforeSales + 1,
  );
  await p.getByRole("button", { name: "Nova venda", exact: true }).click();
  await p.locator("#posCash").click();
  await p.getByRole("button", { name: "Fechar caixa", exact: true }).click();
  await p.locator("#cashForm [name=confirmed]").check();
  await p.locator("#cashSubmit").click();
  await p.locator("#cashDialog").waitFor({ state: "hidden" });
  await p.locator("#posCashGate:not(.hidden)").waitFor();
  await shot("cash-gate");
  assert(await p.locator("#posTerminal").isHidden());
  await p.locator("#posOpenForm [name=opening]").fill("50,00");
  await p.locator("#posOpenSubmit").click();
  await p.locator("#posTerminal:not(.hidden)").waitFor();
  await p.locator("#posCash").click();
  await p.getByRole("button", { name: "Sangria", exact: true }).click();
  await p.locator("#cashForm [name=amount]").fill("5,00");
  await p.locator("#cashForm [name=note]").fill("Sangria QA PDV");
  await p.locator("#cashForm [name=confirmed]").check();
  await p.locator("#cashSubmit").click();
  await p.locator("#cashDialog").waitFor({ state: "hidden" });
  await p.locator("#posCash").click();
  await p
    .getByRole("button", { name: "Histórico de sangrias e suprimentos" })
    .click();
  await p
    .locator("#posDialog")
    .getByText(/Sangria QA PDV/)
    .first()
    .waitFor();
  await shot("cash-movements");
  await p.locator("#posDialog .dialog-header button").click();
  await p.locator("#posCash").click();
  await p.getByRole("button", { name: "Fechar caixa", exact: true }).click();
  await p.locator("#cashForm [name=confirmed]").check();
  await p.locator("#cashSubmit").click();
  await p.locator("#cashDialog").waitFor({ state: "hidden" });
  await p.locator("#posCashGate:not(.hidden)").waitFor();
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
    if (role === "finance_viewer") {
      assert(await q.locator("#enterPOS").isDisabled());
      await q.locator("#enterAdmin").click();
      await q.locator("#dashboardStats strong").first().waitFor();
      assert(await q.locator("#adminToPOS").isHidden());
    } else {
      await q.locator("#enterPOS").click();
      await q.locator("#posCashGate:not(.hidden)").waitFor();
      await q.locator("#posOpenForm [name=opening]").fill("20,00");
      await q.locator("#posOpenSubmit").click();
      await q.locator("#posTerminal:not(.hidden)").waitFor();
      assert(await q.locator("#posDiscountSection").isHidden());
      await q.screenshot({
        path: out + "/attendant-mobile.png",
        fullPage: true,
      });
      await q.locator("#posCash").click();
      assert.equal(
        await q
          .getByRole("button", { name: "Devoluções", exact: true })
          .count(),
        0,
      );
      await q
        .getByRole("button", { name: "Fechar caixa", exact: true })
        .click();
      await q.locator("#cashForm [name=confirmed]").check();
      await q.locator("#cashSubmit").click();
      await q.locator("#cashDialog").waitFor({ state: "hidden" });
      await q.locator("#posCashGate:not(.hidden)").waitFor();
    }
    await c.close();
  }
  assert.deepEqual(errors, []);
  fs.writeFileSync(
    out + "/results.json",
    JSON.stringify(
      {
        passed: true,
        date: new Date().toISOString(),
        errors,
        checks: [
          "mode chooser",
          "real dashboard",
          "standalone PDV",
          "search shortcut",
          "hold/resume",
          "manager discount",
          "kitchen checkout",
          "split payment",
          "cash change",
          "receipt",
          "dark theme",
          "local animations",
          "mobile no overflow",
          "reduced motion",
          "switch ADM",
          "cash close gate",
          "cash opening",
          "restaurant category filtering",
          "lost response retry exactly once",
          "cash withdrawal history",
          "attendant own open/close",
          "finance viewer cannot enter PDV",
        ],
      },
      null,
      2,
    ),
  );
  await browser.close();
  console.log("PASS: POS browser journey");
})().catch((e) => {
  console.error(e);
  process.exit(1);
});
