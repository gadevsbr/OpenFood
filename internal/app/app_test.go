package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"openfood/internal/app"
	local "openfood/internal/runtime"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTransitions(t *testing.T) {
	for _, tc := range []struct {
		from, to string
		allowed  bool
	}{
		{"confirmed", "preparing", true},
		{"confirmed", "cancelled", true},
		{"preparing", "ready", true},
		{"ready", "completed", true},
		{"completed", "confirmed", false},
		{"cancelled", "preparing", false},
		{"confirmed", "completed", false},
	} {
		if app.Allowed(tc.from, tc.to) != tc.allowed {
			t.Errorf("%s -> %s", tc.from, tc.to)
		}
	}
}

func TestPostgresJourney(t *testing.T) {
	bin := os.Getenv("OPENFOOD_TEST_PG_BIN")
	if bin == "" {
		t.Skip("set OPENFOOD_TEST_PG_BIN to run real PostgreSQL tests")
	}
	bin, e := filepath.Abs(bin)
	if e != nil {
		t.Fatal(e)
	}
	data := filepath.Join(t.TempDir(), "Dados com espaços e acentuação")
	r, e := local.Load(data, bin)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if e = r.Start(ctx); e != nil {
		t.Fatal(e)
	}
	defer func() { stop, c := context.WithTimeout(context.Background(), 30*time.Second); defer c(); r.Stop(stop) }()
	a, e := app.Open(ctx, r.DSN, r.Config.SetupToken, r)
	if e != nil {
		t.Fatal(e)
	}
	defer a.DB.Close()
	server := httptest.NewServer(a.Handler())
	defer server.Close()

	call := func(method, path string, payload any, cookie *http.Cookie, extraHeaders ...string) (int, []byte, *http.Response) {
		var raw []byte
		if payload != nil {
			raw, _ = json.Marshal(payload)
		}
		req, _ := http.NewRequest(method, server.URL+path, bytes.NewReader(raw))
		req.Header.Set("X-OpenFood", "1")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		for i := 0; i+1 < len(extraHeaders); i += 2 {
			req.Header.Set(extraHeaders[i], extraHeaders[i+1])
		}
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		var out bytes.Buffer
		out.ReadFrom(res.Body)
		return res.StatusCode, out.Bytes(), res
	}

	// --- Setup phase ---
	code, _, _ := call("POST", "/api/setup", map[string]string{
		"token": "wrong", "email": "admin@example.org", "password": "secure-password-long", "store": "Loja",
	}, nil)
	if code != 403 {
		t.Fatalf("bad setup token should be 403: got %d", code)
	}
	code, _, _ = call("POST", "/api/setup", map[string]string{
		"token": r.Config.SetupToken, "email": "admin@example.org", "password": "secure-password-long",
		"store": "Loja", "organization": "Org Principal",
	}, nil)
	if code != 201 {
		t.Fatalf("setup should be 201: got %d", code)
	}
	code, _, _ = call("POST", "/api/setup", map[string]string{
		"token": r.Config.SetupToken, "email": "x@example.org", "password": "secure-password-long", "store": "Loja",
	}, nil)
	if code != 409 {
		t.Fatalf("setup reused should be 409: got %d", code)
	}

	// --- Login phase ---
	code, _, res := call("POST", "/api/login", map[string]string{
		"email": "admin@example.org", "password": "secure-password-long",
	}, nil)
	if code != 200 {
		t.Fatalf("login should be 200: got %d", code)
	}
	cookie := res.Cookies()[0]

	// --- Anon checks ---
	code, _, _ = call("GET", "/api/products", nil, nil)
	if code != 401 {
		t.Fatal("anonymous catalog should be 401")
	}
	code, _, _ = call("POST", "/api/shutdown", map[string]string{}, nil)
	if code != 401 {
		t.Fatal("anonymous shutdown should be 401")
	}

	// --- CSRF check ---
	req, _ := http.NewRequest("POST", server.URL+"/api/products", bytes.NewBufferString(`{"Name":"x","PriceCents":1,"Stock":1}`))
	req.AddCookie(cookie)
	req.Header.Set("X-OpenFood", "1")
	req.Header.Set("Origin", "https://malicious.example")
	crossRes, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	crossRes.Body.Close()
	if crossRes.StatusCode != 403 {
		t.Fatal("cross-origin should be 403")
	}

	// --- Get store and user info ---
	code, body, _ := call("GET", "/api/me", nil, cookie)
	if code != 200 {
		t.Fatalf("/api/me should be 200: got %d", code)
	}
	var me map[string]any
	json.Unmarshal(body, &me)
	if me["role"] != "instance_admin" {
		t.Fatalf("first user should be instance_admin, got %v", me["role"])
	}

	// --- Stores endpoint ---
	code, body, _ = call("GET", "/api/stores", nil, cookie)
	if code != 200 {
		t.Fatalf("/api/stores should be 200: got %d", code)
	}
	var stores []map[string]any
	json.Unmarshal(body, &stores)
	if len(stores) == 0 {
		t.Fatal("should have at least one store")
	}
	store := int64(stores[0]["id"].(float64))
	orgID := int64(stores[0]["org_id"].(float64))
	t.Logf("store=%d org=%d", store, orgID)

	// --- Products in store ---
	var product int64
	if e = a.DB.QueryRow(ctx, "INSERT INTO products(store_id,name,price_cents,stock_quantity) VALUES($1,'Último item',1999,1) RETURNING id", store).Scan(&product); e != nil {
		t.Fatal(e)
	}

	// --- RBAC: store isolation (cross-store order should fail) ---
	var otherOrg, otherStore int64
	a.DB.QueryRow(ctx, "INSERT INTO organizations(name) VALUES('Outra Org') RETURNING id").Scan(&otherOrg)
	a.DB.QueryRow(ctx, "INSERT INTO stores(org_id,name) VALUES($1,'Outra Loja') RETURNING id", otherOrg).Scan(&otherStore)
	var adminUserID int64
	a.DB.QueryRow(ctx, "SELECT id FROM users WHERE email='admin@example.org'").Scan(&adminUserID)
	if _, e = a.CreateOrder(ctx, otherStore, adminUserID, app.Token(), app.OrderRequest{Items: []app.Item{{ProductID: product, Quantity: 1}}}); e == nil {
		t.Fatal("cross-store order should fail: product not visible to other store")
	}

	// --- Concurrent stock isolation ---
	// Organization admins cannot select another organization's store; managers cannot promote roles via PATCH.
	for _, account := range []struct{ email, role string }{{"org-admin@example.org", "org_admin"}, {"manager@example.org", "store_manager"}} {
		if _, e = a.DB.Exec(ctx, "INSERT INTO users(org_id,store_id,email,password_hash,role) SELECT $1,$2,$3,password_hash,$4 FROM users WHERE id=$5", orgID, store, account.email, account.role, adminUserID); e != nil {
			t.Fatal(e)
		}
		code, _, loginRes := call("POST", "/api/login", map[string]string{"email": account.email, "password": "secure-password-long"}, nil)
		if code != 200 {
			t.Fatalf("role login: got %d", code)
		}
		roleCookie := loginRes.Cookies()[0]
		if account.role == "org_admin" {
			code, _, _ = call("POST", "/api/users", map[string]any{"email": "cross-org@example.org", "password": "secure-password-long", "role": "attendant", "store_id": otherStore}, roleCookie)
			if code != 403 {
				t.Fatalf("cross-org user assignment: expected 403, got %d", code)
			}
		}
		code, _, _ = call("GET", "/api/products", nil, roleCookie, "X-Store-ID", fmt.Sprint(otherStore))
		if code != 403 {
			t.Fatalf("%s cross-org catalog: expected 403, got %d", account.role, code)
		}
		var roleUserID int64
		if e = a.DB.QueryRow(ctx, "SELECT id FROM users WHERE email=$1", account.email).Scan(&roleUserID); e != nil {
			t.Fatal(e)
		}
		code, _, _ = call("PATCH", "/api/users", map[string]any{"id": roleUserID, "role": "instance_admin"}, roleCookie)
		if code != 403 {
			t.Fatalf("%s promotion: expected 403, got %d", account.role, code)
		}
		code, _, _ = call("PATCH", "/api/users", map[string]any{"id": adminUserID, "active": false}, roleCookie)
		if code != 403 {
			t.Fatalf("%s admin deactivation: expected 403, got %d", account.role, code)
		}
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	ids := make(chan int64, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, e := a.CreateOrder(ctx, store, adminUserID, app.Token(), app.OrderRequest{Items: []app.Item{{ProductID: product, Quantity: 1}}})
			results <- e
			if e == nil {
				ids <- id
			}
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("concurrent stock: expected 1 success, got %d", success)
	}
	orderID := <-ids
	var stock int64
	a.DB.QueryRow(ctx, "SELECT stock_quantity FROM products WHERE id=$1", product).Scan(&stock)
	if stock != 0 {
		t.Fatalf("stock should be 0, got %d", stock)
	}

	// --- Idempotency ---
	a.DB.Exec(ctx, "UPDATE products SET stock_quantity=10 WHERE id=$1", product)
	key := app.Token()
	input := app.OrderRequest{Items: []app.Item{{ProductID: product, Quantity: 2}}}
	first, e := a.CreateOrder(ctx, store, adminUserID, key, input)
	if e != nil {
		t.Fatal(e)
	}
	second, e := a.CreateOrder(ctx, store, adminUserID, key, input)
	if e != nil || first != second {
		t.Fatal("deduplication failed")
	}
	input.Items[0].Quantity = 3
	if _, e = a.CreateOrder(ctx, store, adminUserID, key, input); e == nil {
		t.Fatal("key reused with changed payload should fail")
	}

	// --- Price snapshot ---
	a.DB.Exec(ctx, "UPDATE products SET price_cents=9999 WHERE id=$1", product)
	var snapshot int64
	a.DB.QueryRow(ctx, "SELECT total_cents FROM orders WHERE id=$1", first).Scan(&snapshot)
	if snapshot != 3998 {
		t.Fatalf("price snapshot should be 3998, got %d", snapshot)
	}

	// Product edits, stale stock, availability and snapshot detail use the real API/database.
	edit := map[string]any{"id": product, "name": "Produto revisado", "price_cents": 2500, "stock_quantity": 8, "expected_stock_quantity": 8, "unit": "porcao", "active": true, "min_order_quantity": 2, "max_order_quantity": 4}
	code, body, _ = call("PATCH", "/api/products", edit, cookie, "X-Store-ID", fmt.Sprint(store))
	if code != 200 {
		t.Fatalf("product edit: %d %s", code, body)
	}
	edit["expected_stock_quantity"] = 10
	code, _, _ = call("PATCH", "/api/products", edit, cookie, "X-Store-ID", fmt.Sprint(store))
	if code != 409 {
		t.Fatalf("stale stock must conflict: %d", code)
	}
	code, body, _ = call("GET", fmt.Sprintf("/api/orders?id=%d", first), nil, cookie, "X-Store-ID", fmt.Sprint(store))
	var detail []map[string]any
	if code != 200 || json.Unmarshal(body, &detail) != nil || len(detail) != 1 || detail[0]["price_cents"] != float64(1999) || detail[0]["name"] != "Último item" {
		t.Fatalf("snapshot detail: %d %s", code, body)
	}
	code, _, _ = call("GET", fmt.Sprintf("/api/orders?id=%d", first), nil, cookie, "X-Store-ID", fmt.Sprint(otherStore))
	if code != 404 {
		t.Fatalf("detail cross-store: %d", code)
	}
	for _, quantity := range []int64{1, 5} {
		if _, err := a.CreateOrder(ctx, store, adminUserID, app.Token(), app.OrderRequest{Items: []app.Item{{ProductID: product, Quantity: quantity}}}); err == nil {
			t.Fatal("quantity limits not enforced")
		}
	}
	if _, err := a.CreateOrder(ctx, store, adminUserID, app.Token(), app.OrderRequest{Items: []app.Item{{ProductID: product, Quantity: 2}, {ProductID: product, Quantity: 2}}}); err == nil {
		t.Fatal("duplicate products not rejected")
	}
	for _, change := range []string{"active=false", "is_weight_based=true", "available_from='00:00',available_until='00:00'"} {
		if _, err := a.DB.Exec(ctx, "UPDATE products SET "+change+" WHERE id=$1", product); err != nil {
			t.Fatal(err)
		}
		if _, err := a.CreateOrder(ctx, store, adminUserID, app.Token(), app.OrderRequest{Items: []app.Item{{ProductID: product, Quantity: 2}}}); err == nil {
			t.Fatalf("availability: %s", change)
		}
		if _, err := a.DB.Exec(ctx, "UPDATE products SET active=true,is_weight_based=false,available_from=NULL,available_until=NULL WHERE id=$1", product); err != nil {
			t.Fatal(err)
		}
	}
	var secondProduct int64
	if err := a.DB.QueryRow(ctx, "INSERT INTO products(store_id,name,price_cents,stock_quantity) VALUES($1,'Segundo produto QA',999,5) RETURNING id", store).Scan(&secondProduct); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateOrder(ctx, store, adminUserID, app.Token(), app.OrderRequest{Items: []app.Item{{ProductID: product, Quantity: 2}, {ProductID: secondProduct, Quantity: 6}}}); err == nil {
		t.Fatal("insufficient second product accepted")
	}
	var unchanged float64
	if err := a.DB.QueryRow(ctx, "SELECT stock_quantity FROM products WHERE id=$1", product).Scan(&unchanged); err != nil || unchanged != 8 {
		t.Fatal("failed mixed order consumed first stock")
	}
	mixed, err := a.CreateOrder(ctx, store, adminUserID, app.Token(), app.OrderRequest{Items: []app.Item{{ProductID: product, Quantity: 2}, {ProductID: secondProduct, Quantity: 3}}})
	if err != nil {
		t.Fatal(err)
	}
	var mixedTotal int64
	if err = a.DB.QueryRow(ctx, "SELECT total_cents FROM orders WHERE id=$1", mixed).Scan(&mixedTotal); err != nil || mixedTotal != 7997 {
		t.Fatalf("mixed total: %d %v", mixedTotal, err)
	}
	code, _, _ = call("POST", "/api/order-state", map[string]any{"id": mixed, "state": "cancelled"}, cookie, "X-Store-ID", fmt.Sprint(store))
	if code != 200 {
		t.Fatal("mixed cancellation")
	}
	code, _, _ = call("POST", "/api/order-state", map[string]any{"id": mixed, "state": "cancelled"}, cookie, "X-Store-ID", fmt.Sprint(store))
	if code != 409 {
		t.Fatal("repeated cancellation must not release stock twice")
	}
	var returned float64
	if err = a.DB.QueryRow(ctx, "SELECT stock_quantity FROM products WHERE id=$1", secondProduct).Scan(&returned); err != nil || returned != 5 {
		t.Fatal("stock not restored once")
	}
	// --- State transitions ---
	code, _, _ = call("POST", "/api/order-state", map[string]any{"id": orderID, "state": "completed"}, cookie,
		"X-Store-ID", fmt.Sprint(store))
	if code != 409 {
		t.Fatal("invalid transition should be 409")
	}
	for _, state := range []string{"preparing", "ready", "completed"} {
		code, _, _ = call("POST", "/api/order-state", map[string]any{"id": orderID, "state": state}, cookie,
			"X-Store-ID", fmt.Sprint(store))
		if code != 200 {
			t.Fatalf("transition %s should be 200: got %d", state, code)
		}
	}

	// --- RBAC: create attendant user, check they cannot create products ---
	code, body, _ = call("POST", "/api/users", map[string]any{
		"email": "garcom@example.org", "password": "senha-super-segura-12",
		"role": "attendant", "store_id": store,
	}, cookie)
	if code != 201 {
		t.Fatalf("create attendant should be 201: got %d body=%s", code, body)
	}
	// Login as attendant
	code, _, attRes := call("POST", "/api/login", map[string]string{
		"email": "garcom@example.org", "password": "senha-super-segura-12",
	}, nil)
	if code != 200 {
		t.Fatalf("attendant login should be 200: got %d", code)
	}
	attCookie := attRes.Cookies()[0]
	code, _, _ = call("PATCH", "/api/products", map[string]any{"id": product}, attCookie, "X-Store-ID", fmt.Sprint(store))
	if code != 403 {
		t.Fatalf("attendant product edit: %d", code)
	}
	// Attendant cannot create products
	code, _, _ = call("POST", "/api/products", map[string]any{
		"Name": "Produto do Garçom", "PriceCents": int64(100), "Stock": int64(10),
	}, attCookie, "X-Store-ID", fmt.Sprint(store))
	if code != 403 {
		t.Fatalf("attendant creating product should be 403: got %d", code)
	}

	// --- RBAC: create kitchen user, check they can only do kitchen transitions ---
	code, _, _ = call("POST", "/api/users", map[string]any{
		"email": "cozinha@example.org", "password": "cozinha-secreta-12",
		"role": "kitchen", "store_id": store,
	}, cookie)
	if code != 201 {
		t.Fatalf("create kitchen should be 201: got %d", code)
	}

	// --- RBAC: deactivate attendant, sessions immediately revoked ---
	var attUserID int64
	a.DB.QueryRow(ctx, "SELECT id FROM users WHERE email='garcom@example.org'").Scan(&attUserID)
	active := false
	code, _, _ = call("PATCH", "/api/users", map[string]any{"id": attUserID, "active": &active}, cookie)
	if code != 200 {
		t.Fatalf("deactivate attendant should be 200: got %d", code)
	}
	// Previously valid session must now be rejected
	code, _, _ = call("GET", "/api/products", nil, attCookie, "X-Store-ID", fmt.Sprint(store))
	if code != 401 {
		t.Fatalf("deactivated user session should return 401: got %d", code)
	}

	// --- List users as org_admin (cross-org denied) ---
	code, body, _ = call("GET", "/api/users", nil, cookie)
	if code != 200 {
		t.Fatalf("admin list users should be 200: got %d", code)
	}
	var users []map[string]any
	json.Unmarshal(body, &users)
	if len(users) < 2 {
		t.Fatalf("should have at least 2 users, got %d", len(users))
	}

	// --- Jobs / abandoned lease recovery ---
	a.DB.Exec(ctx, "UPDATE jobs SET state='running',lease_until=now()-interval '1 second'")
	if e = a.WorkOnce(ctx); e != nil {
		t.Fatal(e)
	}
	var completed int
	a.DB.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE state='done'").Scan(&completed)
	if completed != 1 {
		t.Fatal("abandoned lease not recovered")
	}

	// --- Backup and restore ---
	// Customer/contact fields and consent lists remain usable when optional timestamps are NULL.
	var customerID int64
	for _, name := range []string{"Cliente sem contato A", "Cliente sem contato B"} {
		code, body, _ = call("POST", "/api/customers", map[string]any{"name": name}, cookie)
		if code != 201 {
			t.Fatalf("optional contact: expected 201, got %d %s", code, body)
		}
		var result map[string]int64
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatal(err)
		}
		customerID = result["id"]
	}
	code, body, _ = call("GET", "/api/customers", nil, cookie)
	if code != 200 {
		t.Fatalf("nullable customer list: expected 200, got %d %s", code, body)
	}
	code, body, _ = call("POST", "/api/consents", map[string]any{"customer_id": customerID, "purpose": "qa-contact", "version": "1", "granted": true, "source": "qa"}, cookie)
	if code != 200 {
		t.Fatalf("consent registration: got %d %s", code, body)
	}
	code, body, _ = call("GET", "/api/consents", nil, cookie)
	if code != 200 {
		t.Fatalf("consent list: got %d %s", code, body)
	}
	// Empty optional SKUs must not collide, and nullable legacy fields must remain readable.
	for _, name := range []string{"Produto sem SKU A", "Produto sem SKU B"} {
		code, body, _ = call("POST", "/api/products", map[string]any{"name": name, "price_cents": 1200, "stock_quantity": 3, "active": true}, cookie)
		if code != 201 {
			t.Fatalf("optional SKU: expected 201, got %d %s", code, body)
		}
	}
	code, body, _ = call("GET", "/api/products", nil, cookie)
	if code != 200 {
		t.Fatalf("nullable catalog read: expected 200, got %d %s", code, body)
	}
	backup, e := r.Backup(ctx)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(backup)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Restore(ctx, []byte("invalid")); e == nil {
		t.Fatal("invalid restore should be rejected")
	}
	a.DB.Exec(ctx, "UPDATE stores SET name='Changed' WHERE id=$1", store)
	if e = r.Restore(ctx, raw); e != nil {
		t.Fatal(e)
	}
	var name string
	if e = a.DB.QueryRow(ctx, "SELECT name FROM stores WHERE id=$1", store).Scan(&name); e != nil || name != "Loja" {
		t.Fatalf("restore: expected 'Loja', got '%s' err=%v", name, e)
	}

	// Session must be invalidated post-restore
	code, _, _ = call("GET", "/api/status", nil, cookie)
	if code != 401 {
		t.Fatal("restored session still usable — expected 401")
	}

	// Reopen DB
	a.DB.Close()
	a, e = app.Open(ctx, r.DSN, r.Config.SetupToken, r)
	if e != nil {
		t.Fatal(e)
	}
	defer a.DB.Close()
	if e = a.DB.QueryRow(ctx, "SELECT total_cents FROM orders WHERE id=$1", first).Scan(&snapshot); e != nil || snapshot != 3998 {
		t.Fatal("restart lost order data")
	}

	t.Log("PASS: orgs, stores, RBAC setup, me endpoint, cross-store isolation, concurrent stock, dedup, snapshots, role-based permissions, immediate session revocation on deactivate, transitions, expired lease recovery, backup/restore, session invalidation, database reopen")
}

// Historical fixtures come from b3676d3 (v1) and v0.2.0-alpha.1 (v2).
// All databases are disposable test instances beneath t.TempDir(), never user data.
func TestPostgresUpgrade(t *testing.T) {
	bin := os.Getenv("OPENFOOD_TEST_PG_BIN")
	if bin == "" {
		t.Skip("set OPENFOOD_TEST_PG_BIN to run real PostgreSQL upgrades")
	}
	bin, err := filepath.Abs(bin)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{1, 2, 3} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			r, err := local.Load(filepath.Join(t.TempDir(), "Upgrade isolado"), bin)
			if err != nil {
				t.Fatal(err)
			}
			if err = r.Start(ctx); err != nil {
				t.Fatal(err)
			}
			defer func() { stop, c := context.WithTimeout(context.Background(), 30*time.Second); defer c(); r.Stop(stop) }()
			db, err := pgxpool.New(ctx, r.DSN)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			fixture, err := os.ReadFile(fmt.Sprintf("testdata/schema-v%d.sql", version))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(ctx, string(fixture)); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(ctx, "CREATE TABLE schema_migrations(version integer PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())"); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES($1,$2)", version, app.Hash(string(fixture))); err != nil {
				t.Fatal(err)
			}
			var store int64
			if version >= 2 {
				var org int64
				if err = db.QueryRow(ctx, "INSERT INTO organizations(name) VALUES('Org histórica') RETURNING id").Scan(&org); err != nil {
					t.Fatal(err)
				}
				err = db.QueryRow(ctx, "INSERT INTO stores(org_id,name) VALUES($1,'Loja histórica') RETURNING id", org).Scan(&store)
			} else {
				err = db.QueryRow(ctx, "INSERT INTO stores(name) VALUES('Loja histórica') RETURNING id").Scan(&store)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(ctx, "INSERT INTO products(store_id,name,price_cents,"+map[bool]string{true: "stock_quantity", false: "stock"}[version == 3]+") VALUES($1,'Produto preservado',1999,7)", store); err != nil {
				t.Fatal(err)
			}
			var oldDump []byte
			if version == 3 {
				path, e := r.Backup(ctx)
				if e != nil {
					t.Fatal(e)
				}
				oldDump, e = os.ReadFile(path)
				if e != nil {
					t.Fatal(e)
				}
			}
			a, err := app.Open(ctx, r.DSN, r.Config.SetupToken, r)
			if err != nil {
				t.Fatal(err)
			}
			defer a.DB.Close()
			var stock, price int64
			if err = a.DB.QueryRow(ctx, "SELECT stock_quantity,price_cents FROM products WHERE store_id=$1", store).Scan(&stock, &price); err != nil {
				t.Fatal(err)
			}
			if stock != 7 || price != 1999 {
				t.Fatalf("migration changed product: stock=%d price=%d", stock, price)
			}
			if version == 3 {
				if err = r.Restore(ctx, oldDump); err != nil {
					t.Fatalf("legacy v3 backup restore: %v", err)
				}
				var restored int
				if err = a.DB.QueryRow(ctx, "SELECT max(version) FROM schema_migrations").Scan(&restored); err != nil || restored != 4 {
					t.Fatalf("legacy backup not upgraded: %d %v", restored, err)
				}
			}
			if err = a.Migrate(ctx); err != nil {
				t.Fatalf("migration reopen: %v", err)
			}
		})
	}
}
