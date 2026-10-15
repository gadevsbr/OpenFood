"""Real Compose integration test; run only against an empty, isolated test installation."""
import http.cookiejar
import json
import os
import subprocess
import tempfile
import time
import urllib.request
import urllib.error
from pathlib import Path

base = "http://127.0.0.1:18880"
jar = http.cookiejar.CookieJar()
client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
env = dict(line.split("=", 1) for line in Path(".env").read_text().splitlines() if line and not line.startswith("#"))
password = os.urandom(24).hex()

def api(name, body=None, headers=None):
    req = urllib.request.Request(base + "/api/" + name,
        data=None if body is None else json.dumps(body).encode(),
        headers={"X-OpenFood": "1", "Content-Type": "application/json", **(headers or {})})
    with client.open(req, timeout=30) as res:
        return json.load(res)

def ready():
    for _ in range(90):
        try:
            with urllib.request.urlopen(base + "/readyz", timeout=2) as res:
                if res.status == 200:
                    return
        except Exception:
            pass
        time.sleep(1)
    raise RuntimeError("Compose readiness timeout")

def command(*args, **kwargs):
    return subprocess.run(["docker", "compose", *args], check=True, **kwargs)

ready()
assert api("setup")["required"]
api("setup", {"token": env["SETUP_TOKEN"], "store": "QA Compose", "email": "linux-qa@example.org", "password": password})
api("login", {"email": "linux-qa@example.org", "password": password})
product = api("products", {"name": "QA Product", "price_cents": 1234, "stock_quantity": 3, "active": True})
payload = {"items": [{"product_id": product["id"], "quantity": 2}]}
headers = {"Idempotency-Key": "linux-compose-test-order"}
order = api("orders", payload, headers)
assert api("orders", payload, headers)["id"] == order["id"]
assert api("orders")[0]["total_cents"] == 2468
assert api("products")[0]["stock_quantity"] == 1
for state in ["preparing", "ready", "completed"]:
    api("order-state", {"id": order["id"], "state": state})
# Exercise real manual money operations and preserve closed periods across refunds.
cash = api("cash-sessions", {"action": "open", "opening_cents": 500}, {"Idempotency-Key": "linux-cash-open"})
receipt_payload = {"order_id": order["id"], "amount_cents": 1000, "method": "cash"}
receipt = api("manual-receipts", receipt_payload, {"Idempotency-Key": "linux-cash-receipt"})
assert api("manual-receipts", receipt_payload, {"Idempotency-Key": "linux-cash-receipt"})["id"] == receipt["id"]
card = api("manual-receipts", {"order_id": order["id"], "amount_cents": 1468, "method": "card"}, {"Idempotency-Key": "linux-card-receipt"})
assert api("orders")[0]["financial_state"] == "manual_confirmed"
api("cash-sessions", {"action": "close", "id": cash["id"], "expected_cents": 1500, "counted_cents": 1490, "note": "QA difference 10 cents"}, {"Idempotency-Key": "linux-cash-close"})
second_cash = api("cash-sessions", {"action": "open", "opening_cents": 2000}, {"Idempotency-Key": "linux-cash-open-next"})
for entry in [receipt, card]:
    api("manual-receipts", {"refund_of": entry["id"], "note": "QA manual refund confirmed"}, {"Idempotency-Key": "linux-refund-" + str(entry["id"])})
api("cash-sessions", {"action": "close", "id": second_cash["id"], "expected_cents": 1000, "counted_cents": 1000}, {"Idempotency-Key": "linux-cash-close-next"})
summary = api("cash-sessions")
assert len(summary["receipts"]) == 4
assert summary["sessions"][0]["cash_cents"] == -1000
assert summary["sessions"][1]["cash_cents"] == 1000
assert summary["sessions"][1]["difference_cents"] == -10

# Pizza must use the same integer pricing/snapshot path in Linux Compose.
pizza = api("products", {"name": "QA Pizza", "price_cents": 3001, "stock_quantity": 3, "active": True, "unit": "unidade"})
configured = api("pizza-configuration?product_id=" + str(pizza["id"]), {
    "product_id": pizza["id"], "version": 1,
    "pizza": {"enabled": True, "pricing_policy": "weighted_average", "max_flavors": 2,
              "flavors": [{"id": "", "name": "Calabresa QA", "price_cents": 3001, "active": True},
                          {"id": "", "name": "Queijo QA", "price_cents": 4002, "active": True}]}})
pizza_payload = {"items": [{"product_id": pizza["id"], "quantity": 1, "configuration_version": configured["version"],
                            "flavors": [{"flavor_id": configured["pizza"]["flavors"][0]["id"], "parts": 2},
                                        {"flavor_id": configured["pizza"]["flavors"][1]["id"], "parts": 1}]}]}
pizza_headers = {"Idempotency-Key": "linux-compose-pizza-order"}
pizza_order = api("orders", pizza_payload, pizza_headers)
assert api("orders", pizza_payload, pizza_headers)["id"] == pizza_order["id"]
assert next(x for x in api("orders") if x["id"] == pizza_order["id"])["total_cents"] == 3335
assert next(x for x in api("products") if x["id"] == pizza["id"])["stock_quantity"] == 2
assert api("orders?id=" + str(pizza_order["id"]))[0]["selection"]["pizza"]["total_parts"] == 3
api("order-state", {"id": pizza_order["id"], "state": "cancelled"})
assert next(x for x in api("products") if x["id"] == pizza["id"])["stock_quantity"] == 3

# Shared add-on stock and price propagation across products use the same tables in Linux.
addon_products = [api("products", {"name": name, "price_cents": 2000, "stock_quantity": 3, "active": True, "unit": "unidade"}) for name in ["QA Burger", "QA Fries"]]
shared = api("addon-configuration?group_id=0", {"version": "", "group": {"id": 0, "name": "Shared sauces QA", "min_selections": 0, "max_selections": 1, "required": False, "active": True, "options": [{"id": 0, "name": "Sauce QA", "price_cents": 251, "stock_quantity": 4, "active": True}]}, "products": [{"id": p["id"]} for p in addon_products]})
addon_endpoint = "addon-configuration?group_id=" + str(shared["group"]["id"])
assert shared["group"]["shared"]
for p in addon_products:
    menu = api("product-configuration?product_id=" + str(p["id"]))
    assert menu["groups"][0]["options"][0]["id"] == shared["group"]["options"][0]["id"]
    extra_order = api("orders", {"items": [{"product_id": p["id"], "quantity": 1, "configuration_version": menu["version"], "addon_option_ids": [shared["group"]["options"][0]["id"]]}]}, {"Idempotency-Key": "linux-shared-" + str(p["id"])})
    assert next(x for x in api("orders") if x["id"] == extra_order["id"])["total_cents"] == 2251
shared = api(addon_endpoint)
assert shared["group"]["options"][0]["stock_quantity"] == 2
shared["group"]["options"][0]["expected_stock_quantity"] = 2
shared["group"]["options"][0]["price_cents"] = 399
shared = api(addon_endpoint, shared)
assert api("product-configuration?product_id=" + str(addon_products[1]["id"]))["groups"][0]["options"][0]["price_cents"] == 399

# Linked-text removal preserves financial values and survives a new backup/restore.
privacy_customer = api("customers", {"name": "Privacy QA", "email": "privacy-linux@example.org"})
privacy_product = api("products", {"name": "Privacy item", "price_cents": 1000, "stock_quantity": 2, "active": True, "unit": "unidade"})
privacy_order = api("orders", {"customer_id": privacy_customer["id"], "items": [{"product_id": privacy_product["id"], "quantity": 1, "notes": "privacy-linux@example.org"}]}, {"Idempotency-Key": "linux-privacy-order"})
api("consents", {"customer_id": privacy_customer["id"], "purpose": "contact", "version": "qa", "granted": True, "terms_text": "privacy-linux@example.org", "source": "qa"})
privacy_review = api("customer-privacy-review?customer_id=" + str(privacy_customer["id"]))
assert privacy_review["counts"]["orders"] == 1
req = urllib.request.Request(base + "/api/customer-delete?customer_id=" + str(privacy_customer["id"]), method="DELETE", headers={"X-OpenFood": "1", "X-Privacy-Review": privacy_review["version"]})
with client.open(req, timeout=30) as res:
    assert res.status == 200
privacy_export = api("customer-export?customer_id=" + str(privacy_customer["id"]))
assert "privacy-linux@example.org" not in json.dumps(privacy_export)
assert privacy_export["orders"][0]["total_cents"] == 1000

# Weekly schedule applies to new product sales; old order retries remain unchanged after closing.
schedule = api("sale-schedule")
assert schedule["timezone"] == "America/Sao_Paulo" and not schedule["schedule"]["enabled"]
schedule["timezone"] = "UTC"
schedule["schedule"] = {"enabled": True, "windows": []}
schedule = api("sale-schedule", schedule)
assert not next(x for x in api("products") if x["id"] == product["id"])["available_now"]
assert api("orders", payload, headers)["id"] == order["id"]
try:
    api("orders", {"items": [{"product_id": product["id"], "quantity": 1}]}, {"Idempotency-Key": "linux-closed-new-order"})
    raise AssertionError("Closed store accepted a new sale")
except urllib.error.HTTPError as error:
    assert error.code == 409
    assert json.load(error)["error"] == "product_unavailable"


with tempfile.TemporaryDirectory() as tmp:
    dump = Path(tmp) / "openfood.dump"
    with dump.open("wb") as file:
        command("exec", "-T", "db", "pg_dump", "-U", "openfood", "-d", "openfood", "-Fc", stdout=file)
    assert dump.read_bytes().startswith(b"PGDMP")
    command("stop", "app")
    with dump.open("rb") as file:
        command("exec", "-T", "db", "pg_restore", "-U", "openfood", "-d", "openfood", "--clean", "--if-exists", "--single-transaction", "--no-owner", "--no-acl", stdin=file)
    command("exec", "-T", "db", "psql", "-U", "openfood", "-d", "openfood", "-c", "DELETE FROM sessions; UPDATE jobs SET state='failed',last_error_code='restored_requires_review' WHERE state IN ('pending','running');")
    command("start", "app")
    ready()
    jar.clear()
    api("login", {"email": "linux-qa@example.org", "password": password})
    restored = api("orders")
    original = next(x for x in restored if x["id"] == order["id"])
    assert original["state"] == "completed" and original["total_cents"] == 2468
    assert api("pizza-configuration?product_id=" + str(pizza["id"]))["pizza"]["enabled"]
    assert api("orders?id=" + str(pizza_order["id"]))[0]["selection"]["pizza"]["price_cents"] == 3335
assert len(api("cash-sessions")["receipts"]) == 4
assert next(x for x in api("orders") if x["id"] == order["id"])["financial_state"] == "refunded"
assert api(addon_endpoint)["group"]["options"][0]["stock_quantity"] == 2
assert len(api(addon_endpoint)["products"]) == 2
assert api("sale-schedule")["timezone"] == "UTC"
assert api("sale-schedule")["schedule"]["enabled"]
assert api("customer-export?customer_id=" + str(privacy_customer["id"]))["customer"]["email"] is None
print("PASS: manual receipts, split payments, cash closing/refunds across periods and real Compose setup, authentication, catalog, transactional stock, idempotency, order states, pg_dump/pg_restore, restart")
