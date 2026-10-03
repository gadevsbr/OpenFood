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
product = api("products", {"name": "QA Product", "price_cents": 1234, "stock": 3})
payload = {"items": [{"product_id": product["id"], "quantity": 2}]}
headers = {"Idempotency-Key": "linux-compose-test-order"}
order = api("orders", payload, headers)
assert api("orders", payload, headers)["id"] == order["id"]
assert api("orders")[0]["total_cents"] == 2468
assert api("products")[0]["stock"] == 1
for state in ["preparing", "ready", "completed"]:
    api("order-state", {"id": order["id"], "state": state})
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
print("PASS: real Compose setup, authentication, catalog, transactional stock, idempotency, order states, pg_dump/pg_restore, restart")
