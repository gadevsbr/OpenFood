package app_test

import (
	"bytes"
	"context"
	"encoding/json"
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
	}{{"confirmed", "preparing", true}, {"confirmed", "cancelled", true}, {"preparing", "ready", true}, {"ready", "completed", true}, {"completed", "confirmed", false}, {"cancelled", "preparing", false}, {"confirmed", "completed", false}} {
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
	call := func(method, path string, payload any, cookie *http.Cookie) (int, []byte, *http.Response) {
		var raw []byte
		if payload != nil {
			raw, _ = json.Marshal(payload)
		}
		req, _ := http.NewRequest(method, server.URL+path, bytes.NewReader(raw))
		req.Header.Set("X-OpenFood", "1")
		if cookie != nil {
			req.AddCookie(cookie)
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
	code, _, _ := call("POST", "/api/setup", map[string]string{"token": "wrong", "email": "admin@example.org", "password": "secure-password-long", "store": "Loja"}, nil)
	if code != 403 {
		t.Fatalf("bad setup token: %d", code)
	}
	code, _, _ = call("POST", "/api/setup", map[string]string{"token": r.Config.SetupToken, "email": "admin@example.org", "password": "secure-password-long", "store": "Loja"}, nil)
	if code != 201 {
		t.Fatalf("setup: %d", code)
	}
	code, _, _ = call("POST", "/api/setup", map[string]string{"token": r.Config.SetupToken, "email": "x@example.org", "password": "secure-password-long", "store": "Loja"}, nil)
	if code != 409 {
		t.Fatalf("setup reused: %d", code)
	}
	code, _, res := call("POST", "/api/login", map[string]string{"email": "admin@example.org", "password": "secure-password-long"}, nil)
	if code != 200 {
		t.Fatalf("login: %d", code)
	}
	cookie := res.Cookies()[0]
	code, _, _ = call("GET", "/api/products", nil, nil)
	if code != 401 {
		t.Fatal("anonymous catalog")
	}
	code, _, _ = call("POST", "/api/shutdown", map[string]string{}, nil)
	if code != 401 {
		t.Fatal("anonymous shutdown accepted")
	}
	req, _ := http.NewRequest("POST", server.URL+"/api/products", bytes.NewBufferString(`{"name":"x","price_cents":1,"stock":1}`))
	req.AddCookie(cookie)
	req.Header.Set("X-OpenFood", "1")
	req.Header.Set("Origin", "https://malicious.example")
	res, e = http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("cross-origin accepted")
	}
	var store, user int64
	if e = a.DB.QueryRow(ctx, "SELECT id,store_id FROM users LIMIT 1").Scan(&user, &store); e != nil {
		t.Fatal(e)
	}
	var product int64
	if e = a.DB.QueryRow(ctx, "INSERT INTO products(store_id,name,price_cents,stock) VALUES($1,'Último item',1999,1) RETURNING id", store).Scan(&product); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	ids := make(chan int64, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, e := a.CreateOrder(ctx, store, user, app.Token(), app.OrderRequest{Items: []app.Item{{ProductID: product, Quantity: 1}}})
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
		t.Fatalf("concurrent stock: %d successes", success)
	}
	id := <-ids
	var stock int64
	a.DB.QueryRow(ctx, "SELECT stock FROM products WHERE id=$1", product).Scan(&stock)
	if stock != 0 {
		t.Fatalf("stock %d", stock)
	}
	var other int64
	a.DB.QueryRow(ctx, "INSERT INTO stores(name) VALUES('Outra') RETURNING id").Scan(&other)
	if _, e = a.CreateOrder(ctx, other, user, app.Token(), app.OrderRequest{Items: []app.Item{{ProductID: product, Quantity: 1}}}); e == nil {
		t.Fatal("cross store access")
	}
	a.DB.Exec(ctx, "UPDATE products SET stock=10 WHERE id=$1", product)
	key := app.Token()
	input := app.OrderRequest{Items: []app.Item{{ProductID: product, Quantity: 2}}}
	first, e := a.CreateOrder(ctx, store, user, key, input)
	if e != nil {
		t.Fatal(e)
	}
	second, e := a.CreateOrder(ctx, store, user, key, input)
	if e != nil || first != second {
		t.Fatal("deduplication failed")
	}
	input.Items[0].Quantity = 3
	if _, e = a.CreateOrder(ctx, store, user, key, input); e == nil {
		t.Fatal("key reused with changed payload")
	}
	a.DB.Exec(ctx, "UPDATE products SET price_cents=9999 WHERE id=$1", product)
	var snapshot int64
	a.DB.QueryRow(ctx, "SELECT total_cents FROM orders WHERE id=$1", first).Scan(&snapshot)
	if snapshot != 3998 {
		t.Fatalf("snapshot changed: %d", snapshot)
	}
	code, _, _ = call("POST", "/api/order-state", map[string]any{"id": id, "state": "completed"}, cookie)
	if code != 409 {
		t.Fatal("invalid transition accepted")
	}
	for _, state := range []string{"preparing", "ready", "completed"} {
		code, _, _ = call("POST", "/api/order-state", map[string]any{"id": id, "state": state}, cookie)
		if code != 200 {
			t.Fatalf("transition %s: %d", state, code)
		}
	}
	a.DB.Exec(ctx, "UPDATE jobs SET state='running',lease_until=now()-interval '1 second'")
	if e = a.WorkOnce(ctx); e != nil {
		t.Fatal(e)
	}
	var completed int
	a.DB.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE state='done'").Scan(&completed)
	if completed != 1 {
		t.Fatal("abandoned lease not recovered")
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
		t.Fatal("invalid restore accepted")
	}
	a.DB.Exec(ctx, "UPDATE stores SET name='Changed' WHERE id=$1", store)
	if e = r.Restore(ctx, raw); e != nil {
		t.Fatal(e)
	}
	var name string
	if e = a.DB.QueryRow(ctx, "SELECT name FROM stores WHERE id=$1", store).Scan(&name); e != nil || name != "Loja" {
		t.Fatalf("restore: %s %v", name, e)
	}
	code, _, _ = call("GET", "/api/status", nil, cookie)
	if code != 401 {
		t.Fatal("restored session still usable")
	}
	a.DB.Close()
	a, e = app.Open(ctx, r.DSN, r.Config.SetupToken, r)
	if e != nil {
		t.Fatal(e)
	}
	defer a.DB.Close()
	if e = a.DB.QueryRow(ctx, "SELECT total_cents FROM orders WHERE id=$1", first).Scan(&snapshot); e != nil || snapshot != 3998 {
		t.Fatal("restart lost order")
	}
	t.Log("PASS: UTF-8 paths, setup replay, CSRF, auth, store isolation, last-stock concurrency, deduplication, snapshots, transitions, expired lease, real pg_dump/pg_restore, invalid restore, sessions revoked, database reopen")
}
