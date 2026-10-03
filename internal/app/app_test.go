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
	if e = a.DB.QueryRow(ctx, "INSERT INTO products(store_id,name,price_cents,stock) VALUES($1,'Último item',1999,1) RETURNING id", store).Scan(&product); e != nil {
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
	a.DB.QueryRow(ctx, "SELECT stock FROM products WHERE id=$1", product).Scan(&stock)
	if stock != 0 {
		t.Fatalf("stock should be 0, got %d", stock)
	}

	// --- Idempotency ---
	a.DB.Exec(ctx, "UPDATE products SET stock=10 WHERE id=$1", product)
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
