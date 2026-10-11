const { chromium } = require("../tools/qa/node_modules/playwright"),
  fs = require("fs"),
  assert = require("assert");
(async () => {
  const { URL: url } = JSON.parse(
    fs.readFileSync(process.env.OPENFOOD_UI_TEST_DATA + "/runtime.json"),
  );
  const b = await chromium.launch({ headless: true }),
    context = await b.newContext({
      viewport: { width: 1440, height: 1000 },
      locale: "pt-BR",
      timezoneId: "America/Sao_Paulo",
    }),
    p = await context.newPage();
  const errors = [];
  p.on("pageerror", (e) => errors.push(e.message));
  async function login(page, email) {
    await page.goto(url);
    await page.locator("#loginForm [name=email]").fill(email);
    await page
      .locator("#loginForm [name=password]")
      .fill("QA-test-only-password");
    await page.locator("#loginForm button").click();
    await page.locator("#modeChooser:not(.hidden)").waitFor();
    await page.locator("#enterAdmin").click();
    await page.locator("#workspace:not(.hidden)").waitFor();
    if (!(await page.locator("#tabOperacao").isVisible()))
      await page.locator("#menuToggle").click();
    await page.locator("#tabOperacao").click();
    if (await page.locator("#menuToggle").isVisible())
      await page.locator("#menuToggle").click();
    await page.locator("#tabPagamentos").click();
    await page.locator("#cashHistory tr").first().waitFor();
  }
  await login(p, "qa@example.org");
  assert.equal(
    await p.locator("#cashHistory tr").last().locator("td").count(),
    9,
  );
  const row = p.locator("#cashHistory tr").last();
  const cells = await row.locator("td").allTextContents();
  assert(
    cells[3].includes("20,00") &&
      cells[4].includes("30,00") &&
      cells[5].includes("19,30"),
  );
  assert(cells[1].includes("Abertura QA"));
  assert(cells[8].includes("0,50 QA conferida"));
  await p.screenshot({
    path:
      (process.env.OPENFOOD_CASH_EVIDENCE || "docs/evidence/cash-stage") +
      "/cash-final-desktop.png",
    fullPage: true,
  });
  await p.locator("#themeSelect").selectOption("dark");
  await p.screenshot({
    path:
      (process.env.OPENFOOD_CASH_EVIDENCE || "docs/evidence/cash-stage") +
      "/cash-dark-desktop.png",
    fullPage: true,
  });
  await p.locator("#themeSelect").selectOption("light");
  await p.setViewportSize({ width: 390, height: 844 });
  assert(
    await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
  );
  await p.screenshot({
    path:
      (process.env.OPENFOOD_CASH_EVIDENCE || "docs/evidence/cash-stage") +
      "/cash-final-mobile.png",
    fullPage: true,
  });
  const store = (await p.evaluate(async () => (await fetch("/api/me")).json()))
    .store_id;
  for (const role of ["attendant", "finance_viewer"]) {
    const email = "cash-final-" + role + "@example.org";
    const status = await p.evaluate(
      async ({ email, role, store }) => {
        const r = await fetch("/api/users", {
          method: "POST",
          headers: { "Content-Type": "application/json", "X-OpenFood": "1" },
          body: JSON.stringify({
            email,
            password: "QA-test-only-password",
            role,
            store_id: store,
          }),
        });
        return r.status;
      },
      { email, role, store },
    );
    assert([201, 409].includes(status));
    const c = await b.newContext({
        viewport: { width: 390, height: 844 },
        locale: "pt-BR",
        timezoneId: "America/Sao_Paulo",
      }),
      q = await c.newPage();
    q.on("pageerror", (e) => errors.push(e.message));
    await login(q, email);
    assert(!(await q.locator("#cashOpen").isVisible()));
    assert(!(await q.locator("#cashClose").isVisible()));
    assert.equal(
      await q.getByRole("button", { name: /Devolver recebimento/ }).count(),
      0,
    );
    const receive = q.getByRole("button", { name: /Receber pedido/ });
    if (role === "attendant") {
      assert(await receive.first().isVisible());
      await receive.first().click();
      await q.locator("#cashDialog").waitFor({ state: "visible" });
      await q.keyboard.press("Escape");
      assert(!(await q.locator("#cashDialog").isVisible()));
    } else assert.equal(await receive.count(), 0);
    await q.evaluate(() => window.scrollTo({ top: 0, behavior: "instant" }));
    await q.screenshot({
      path:
        (process.env.OPENFOOD_CASH_EVIDENCE || "docs/evidence/cash-stage") +
        "/cash-" +
        role +
        "-mobile.png",
      fullPage: true,
    });
    await c.close();
  }
  assert.deepEqual(errors, []);
  fs.writeFileSync(
    (process.env.OPENFOOD_CASH_EVIDENCE || "docs/evidence/cash-stage") +
      "/cash-final-results.json",
    JSON.stringify(
      {
        status: "PASS",
        date: "2026-10-11",
        checks: [
          "closed cash shows cash/card/manual Pix totals",
          "opening and closing notes preserved",
          "latest desktop/dark/mobile assets",
          "mobile pending actions visible",
          "attendant receives without close/refund actions",
          "finance viewer read-only UI",
          "modal Escape",
        ],
        pageerrors: errors,
      },
      null,
      2,
    ),
  );
  await b.close();
  console.log("PASS: final cash UI review");
})().catch((e) => {
  console.error(e);
  process.exit(1);
});
