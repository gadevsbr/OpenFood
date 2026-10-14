const { chromium } = require("../tools/qa/node_modules/playwright");
const fs = require("fs"),
  assert = require("assert");
(async () => {
  const suffix=process.env.OPENFOOD_UI_FIXTURE_SUFFIX||"";
  const fixture=value=>value.includes("@")?value.replace("@",suffix.replace(/[^a-zA-Z0-9_-]/g,"_")+"@"):value+suffix;
  const data =
    process.env.OPENFOOD_UI_TEST_DATA || "runtime/ui-redesign-20261010-f";
  const { URL: url } = JSON.parse(fs.readFileSync(data + "/runtime.json"));
  const b = await chromium.launch({ headless: true });
  const p = await b.newPage({ viewport: { width: 1440, height: 1000 } });
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
  await p.locator("#tabCatalogo").click();
  await p.locator("#subtabCategorias").click();
  await p.locator("#pageAction").click();
  await p.locator("#categoryForm [name=name]").fill(fixture("Categoria QA"));
  await p.locator("#categorySubmit").click();
  await p
    .locator("#categoriesTable")
    .getByText(fixture("Categoria QA"), { exact: true })
    .waitFor();
  await p
    .locator("#categoryForm")
    .evaluate((f) => (f.closest("details").open = false));
  await p
    .locator("#categoriesTable tr")
    .filter({ hasText: fixture("Categoria QA") })
    .last()
    .getByRole("button", { name: "Editar", exact: true })
    .click();
  assert(await p.locator("#categoryForm [name=name]").isVisible());
  await p.locator("#categoryForm [name=name]").fill(fixture("Categoria QA revisada"));
  await p.locator("#categorySubmit").click();
  await p
    .locator("#categoriesTable")
    .getByText(fixture("Categoria QA revisada"), { exact: true })
    .waitFor();
  for (const id of ["subtabVariantes", "subtabAdicionais", "subtabCombos"]) {
    await p.locator("#" + id).click();
    await p.locator("#pageAction").click();
    if (id === "subtabAdicionais") {
      await p.locator("#addonEditorDialog").waitFor({state:"visible"});
      await p.getByRole("button", {name:"Fechar adicionais",exact:true}).click();
    }
    assert.equal(
      await p.locator("#panelCatalogo .sub-tab-panel.active").count(),
      1,
    );
  }
  await p.locator("#tabClientes").click();
  await p.locator("#pageAction").click();
  await p.locator("#customerForm [name=name]").fill(fixture("Cliente de teste QA"));
  await p.locator("#customerForm [name=email]").fill(fixture("customer-qa@example.org"));
  await p.locator("#customerSubmit").click();
  await p
    .locator("#customersTable")
    .getByText(fixture("Cliente de teste QA"), { exact: true })
    .waitFor();
  await p
    .locator("#customerForm")
    .evaluate((f) => (f.closest("details").open = false));
  await p
    .locator("#customersTable tr")
    .filter({hasText:fixture("Cliente de teste QA")})
    .getByRole("button", { name: "Editar", exact: true })
    .click();
  assert(await p.locator("#customerForm [name=name]").isVisible());
  await p.locator("#customerForm [name=name]").fill(fixture("Cliente QA revisado"));
  await p.locator("#customerSubmit").click();
  await p
    .locator("#customersTable")
    .getByText(fixture("Cliente QA revisado"), { exact: true })
    .waitFor();
  await p
    .locator("#customerForm")
    .evaluate((f) => (f.closest("details").open = false));
  await p.screenshot({
    path:
      (process.env.OPENFOOD_UI_EVIDENCE || "docs/evidence/ui-redesign") +
      "/customers-desktop.png",
    fullPage: true,
    animations: "disabled",
  });
  await p.locator("#subtabConsentimentos").click();
  assert(await p.locator("#subpanelConsentimentos").isVisible());
  await p.locator("#subtabLGPD").click();
  assert(await p.locator("#subpanelLGPD").isVisible());
  assert(
    !(await p.locator("body").innerText()).includes("Mantidos por 5 anos"),
  );
  await p.locator("#pageAction").click();
  assert(await p.locator("#customerForm [name=name]").isVisible());
  await p.locator("#tabLojas").click();
  await p
    .locator("#storesTable")
    .getByText("Cozinha de teste · QA", { exact: true })
    .waitFor();
  await p.screenshot({
    path:
      (process.env.OPENFOOD_UI_EVIDENCE || "docs/evidence/ui-redesign") +
      "/stores-desktop.png",
    fullPage: true,
    animations: "disabled",
  });
  assert.equal(errors.length, 0, errors.join("\n"));
  const report = {
    date: new Intl.DateTimeFormat("en-CA",{timeZone:"America/Sao_Paulo"}).format(new Date()),
    status: "PASS",
    passed: [
      "contextual catalog actions",
      "category creation and editing",
      "customer creation and editing",
      "editor disclosures and focus",
      "isolated sub-tab navigation",
      "privacy screen without unverified retention promises",
      "store list",
    ],
    javascript_errors: errors,
  };
  fs.writeFileSync(
    (process.env.OPENFOOD_UI_EVIDENCE || "docs/evidence/ui-redesign") +
      "/admin-results.json",
    JSON.stringify(report, null, 2),
  );
  console.log(JSON.stringify(report));
  await b.close();
})().catch((e) => {
  console.error(e.stack);
  process.exit(1);
});
