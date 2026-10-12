const { chromium } = require("../tools/qa/node_modules/playwright");
const fs = require("fs"), assert = require("assert");
(async () => {
  const url = JSON.parse(fs.readFileSync(process.env.OPENFOOD_UI_TEST_DATA + "/runtime.json")).URL;
  const out = process.env.OPENFOOD_RESTAURANT_EVIDENCE || "docs/evidence/restaurant-ui";
  fs.mkdirSync(out, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  const p = await browser.newPage({ viewport: { width: 1440, height: 1000 }, locale: "pt-BR" });
  const errors = [], external = [], failed = [];
  p.on("pageerror", e => errors.push(e.message));
  p.on("request", r => { if (!r.url().startsWith(url)) external.push(r.url()); });
  p.on("response", r => { if (r.url().includes("/assets/") && !r.ok()) failed.push(r.url()); });
  const shot = name => p.screenshot({ path: `${out}/${name}.png`, fullPage: true, animations: "disabled" });
  await p.goto(url);
  await shot("login");
  await p.locator("#loginForm [name=email]").fill("qa@example.org");
  await p.locator("#loginForm [name=password]").fill("QA-test-only-password");
  await p.locator("#loginForm button").click();
  await p.locator("#modeChooser:not(.hidden)").waitFor();
  await shot("modes");
  await p.locator("#enterPOS").click();
  await p.waitForFunction(() => !posBusy);
  if (await p.locator("#posCashGate").isVisible()) {
    await p.locator("#posOpenForm [name=opening]").fill("0,00");
    await p.locator("#posOpenSubmit").click();
  }
  await p.locator("#posGrid button").first().waitFor();
  await p.locator("#posTheme").selectOption("light");
  await p.evaluate(async () => { await Promise.all([...document.querySelectorAll(".category-visual img")].map(i => i.decode())); });
  assert(await p.locator(".category-visual img").count() >= 3);
  assert.equal(await p.locator(".product-initial").count(), 0);
  const contrast = await p.evaluate(() => {
    const css = getComputedStyle(document.documentElement);
    const luminance = hex => {
      const n = hex.trim().replace("#", "");
      const rgb = n.length === 3 ? [...n].map(c => parseInt(c+c,16)) : [0,2,4].map(i => parseInt(n.slice(i,i+2),16));
      const v = rgb.map(c => c/255 <= .04045 ? c/255/12.92 : ((c/255+.055)/1.055)**2.4);
      return v[0]*.2126+v[1]*.7152+v[2]*.0722;
    };
    const ratio = (a,b) => { const x=luminance(css.getPropertyValue(a)), y=luminance(css.getPropertyValue(b)); return (Math.max(x,y)+.05)/(Math.min(x,y)+.05); };
    return { body:ratio("--ink","--surface"), secondary:ratio("--muted","--surface"), action:ratio("--brand","--surface"), selection:ratio("--brand","--lime") };
  });
  for (const [name, ratio] of Object.entries(contrast)) assert(ratio >= 4.5, `${name}: ${ratio}`);
  await shot("pos-desktop");
  await p.locator("#posGrid button:not(:disabled)").first().click();
  await shot("pos-order");
  for (const width of [768,390,320]) {
    await p.setViewportSize({ width, height: 844 });
    assert(await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth+1), `overflow ${width}`);
    await shot(`pos-${width}`);
    if (width <= 390) {
      const bottom = await p.locator("#posGrid button").first().evaluate(el => el.getBoundingClientRect().bottom);
      assert(bottom < 754, `first product hidden by bottom action at ${width}: ${bottom}`);
    }
  }
  await p.locator("#posMobileCart").click();
  await p.locator("#posTotal").waitFor();
  await shot("order-mobile");
  await p.locator("#posBackProducts").click();
  await p.setViewportSize({ width: 1440, height: 1000 });
  await p.locator("#posTheme").selectOption("dark");
  await shot("pos-dark");
  await p.locator("#posTheme").selectOption("light");
  await p.locator("#posToAdmin").click();
  await p.locator("#dashboardStats strong").first().waitFor();
  await shot("admin");
  assert.deepEqual(errors, []); assert.deepEqual(external, []); assert.deepEqual(failed, []);
  fs.writeFileSync(`${out}/visual-results.json`, JSON.stringify({ date:"2026-10-11", status:"PASS", viewports:[1440,768,390,320], contrast, javascript_errors:errors, external_requests:external, failed_assets:failed, checked:["local category images","real menu data","first mobile product visible","page overflow","order mobile","light/dark","PDV/ADM"] }, null, 2));
  await browser.close();
  console.log("PASS: restaurant visual review");
})().catch(e => { console.error(e); process.exit(1); });
