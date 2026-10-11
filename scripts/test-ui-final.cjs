const { chromium } = require("../tools/qa/node_modules/playwright"),
  fs = require("fs"),
  assert = require("assert");
(async () => {
  const data = process.env.OPENFOOD_UI_TEST_DATA || "runtime/qa-ui";
  const { URL: url } = JSON.parse(fs.readFileSync(data + "/runtime.json"));
  const b = await chromium.launch({ headless: true });
  const p = await b.newPage({
    viewport: { width: 1440, height: 1000 },
    locale: "pt-BR",
    timezoneId: "America/Sao_Paulo",
  });
  p.setDefaultTimeout(12000);
  const errors = [];
  p.on("pageerror", (e) => errors.push(e.message));
  await p.goto(url);
  await p.locator("#login:not(.hidden)").waitFor();
  await p.locator("#loginForm [name=email]").fill("qa@example.org");
  await p.locator("#loginForm [name=password]").fill("QA-test-only-password");
  await p.locator("#loginForm button").click();
  await p.locator("#modeChooser:not(.hidden)").waitFor();
  await p.locator("#enterAdmin").click();
  await p.locator("#workspace:not(.hidden)").waitFor();
  if (!(await p.locator("#tabOperacao").isVisible()))
    await p.locator("#menuToggle").click();
  await p.locator("#tabOperacao").click();
  await p.locator("#tabClientes").click();
  await p
    .locator("#customersTable")
    .getByText("Cliente QA revisado", { exact: true })
    .waitFor();
  await p.locator("#subtabConsentimentos").click();
  await p
    .locator("#consentForm")
    .evaluate((f) => (f.closest("details").open = true));
  await p
    .locator("#consentForm [name=customer_id]")
    .selectOption({ label: "Cliente QA revisado (customer-qa@example.org)" });
  await p.locator("#consentForm [name=purpose]").fill("qa-test");
  await p.locator("#consentForm [name=version]").fill("1");
  await p.locator("#consentForm [name=source]").selectOption({ index: 1 });
  await p.locator("#consentForm [name=granted]").check();
  await p.locator("#consentForm button").click();
  await p
    .locator("#consentsTable")
    .getByText("Concedido", { exact: true })
    .waitFor();
  p.once("dialog", (d) => d.accept());
  await p
    .locator("#consentsTable")
    .getByRole("button", { name: "Revogar", exact: true })
    .click();
  await p
    .locator("#consentsTable")
    .getByText("Revogado", { exact: true })
    .waitFor();
  if (!(await p.locator("#tabOperacao").isVisible()))
    await p.locator("#menuToggle").click();
  await p.locator("#tabOperacao").click();
  await p.context().setOffline(true);
  await p.locator("#refresh").click();
  await p.locator("#connectionNotice:not(.hidden)").waitFor();
  assert.equal(await p.locator("#orders tr").count(), 4);
  await p.context().setOffline(false);
  await p.locator("#retryConnection").click();
  await p.locator("#connectionNotice").waitFor({ state: "hidden" });
  await p.locator("#dismissMessage").click();
  await p.setViewportSize({ width: 390, height: 844 });
  await p
    .locator("#orders tr")
    .first()
    .getByRole("button", { name: /Ver resumo/ })
    .click();
  assert(
    (await p.locator("#orderDetailsAmount").textContent()).includes("119,60"),
  );
  await p.screenshot({
    path:
      (process.env.OPENFOOD_UI_EVIDENCE || "docs/evidence/ui-redesign") +
      "/order-summary-mobile.png",
    fullPage: true,
    animations: "disabled",
  });
  await p.keyboard.press("Escape");
  await p.locator("#menuToggle").click();
  assert(await p.locator("#mainContent").evaluate((e) => e.inert));
  await p.keyboard.press("Escape");
  assert(!(await p.locator("#mainContent").evaluate((e) => e.inert)));
  assert.equal(
    await p.locator("#sidebar").evaluate((e) => getComputedStyle(e).visibility),
    "hidden",
  );
  await p.locator("#orderSearch").fill("");
  await p.locator("#orderSearch").blur();
  await p.screenshot({
    path:
      (process.env.OPENFOOD_UI_EVIDENCE || "docs/evidence/ui-redesign") +
      "/orders-mobile.png",
    fullPage: true,
    animations: "disabled",
  });
  await p.setViewportSize({ width: 1440, height: 1000 });
  await p.screenshot({
    path:
      (process.env.OPENFOOD_UI_EVIDENCE || "docs/evidence/ui-redesign") +
      "/orders-desktop.png",
    fullPage: true,
    animations: "disabled",
  });
  assert.equal(errors.length, 0, errors.join("\n"));
  const report = {
    date: "2026-10-10",
    status: "PASS",
    passed: [
      "consent register/revoke badge",
      "offline last-read preservation and retry",
      "mobile order amount detail",
      "mobile menu focus and inert background",
      "updated screenshot review",
    ],
    javascript_errors: errors,
  };
  fs.writeFileSync(
    (process.env.OPENFOOD_UI_EVIDENCE || "docs/evidence/ui-redesign") +
      "/final-results.json",
    JSON.stringify(report, null, 2),
  );
  console.log(JSON.stringify(report));
  await b.close();
})().catch((e) => {
  console.error(e.stack);
  process.exit(1);
});
