"""Real Compose integration test; run only against an empty, isolated test installation."""
import http.cookiejar
import json
import os
import subprocess
import tempfile
import time
import urllib.request
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
    assert restored[0]["state"] == "completed" and restored[0]["total_cents"] == 2468
assert len(api("cash-sessions")["receipts"]) == 4
assert api("orders")[0]["financial_state"] == "refunded"
print("PASS: manual receipts, split payments, cash closing/refunds across periods and real Compose setup, authentication, catalog, transactional stock, idempotency, order states, pg_dump/pg_restore, restart")
