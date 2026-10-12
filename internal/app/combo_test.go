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

func TestComboCustomerJourney(t *testing.T) {
	bin := os.Getenv("OPENFOOD_TEST_PG_BIN")
	if bin == "" {
		t.Skip("requires real PostgreSQL")
	}
	bin, _ = filepath.Abs(bin)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	runtime, err := local.Load(filepath.Join(t.TempDir(), "Caixa QA isolado"), bin)
	if err != nil {
		t.Fatal(err)
	}
	if err = runtime.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		c, end := context.WithTimeout(context.Background(), 30*time.Second)
		defer end()
		runtime.Stop(c)
	}()
	a, err := app.Open(ctx, runtime.DSN, runtime.Config.SetupToken, runtime)
	if err != nil {
		t.Fatal(err)
	}
	defer a.DB.Close()
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	call := func(method, path string, input any, cookie *http.Cookie, headers ...string) (int, []byte, *http.Response) {
		raw, _ := json.Marshal(input)
		req, _ := http.NewRequest(method, server.URL+path, bytes.NewReader(raw))
		req.Header.Set("X-OpenFood", "1")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		for i := 0; i+1 < len(headers); i += 2 {
			req.Header.Set(headers[i], headers[i+1])
		}
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		var body bytes.Buffer
		body.ReadFrom(res.Body)
		return res.StatusCode, body.Bytes(), res
	}
	code, raw, _ := call("POST", "/api/setup", map[string]any{"token": runtime.Config.SetupToken, "email": "cash-qa@example.org", "password": "QA-cash-test-password", "store": "Caixa QA"}, nil)
	if code != 201 {
		t.Fatalf("setup %d %s", code, raw)
	}
	code, _, res := call("POST", "/api/login", map[string]string{"email": "cash-qa@example.org", "password": "QA-cash-test-password"}, nil)
	if code != 200 {
		t.Fatal("login")
	}
	admin := res.Cookies()[0]
	var store, user, org, product, otherStore int64
	if err = a.DB.QueryRow(ctx, "SELECT store_id,id,org_id FROM users WHERE email='cash-qa@example.org'").Scan(&store, &user, &org); err != nil {
		t.Fatal(err)
	}
	if err = a.DB.QueryRow(ctx, "INSERT INTO products(store_id,name,price_cents,stock_quantity) VALUES($1,'Produto caixa QA',1999,100) RETURNING id", store).Scan(&product); err != nil {
		t.Fatal(err)
	}
	if err = a.DB.QueryRow(ctx, "INSERT INTO stores(org_id,name) VALUES($1,'Outra loja caixa QA') RETURNING id", org).Scan(&otherStore); err != nil {
		t.Fatal(err)
	}

	var drink, combo, customer int64
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(a.DB.QueryRow(ctx, "INSERT INTO products(store_id,name,price_cents,stock_quantity) VALUES($1,'Bebida QA',500,100) RETURNING id", store).Scan(&drink))
	must(a.DB.QueryRow(ctx, "INSERT INTO combos(store_id,name,price_cents,pricing_policy) VALUES($1,'Combo QA',2501,'fixed') RETURNING id", store).Scan(&combo))
	_, err = a.DB.Exec(ctx, "INSERT INTO combo_items(combo_id,store_id,product_id,quantity) VALUES($1,$2,$3,2),($1,$2,$4,1)", combo, store, product, drink)
	must(err)
	must(a.DB.QueryRow(ctx, "INSERT INTO customers(store_id,name,phone,email,notes) VALUES($1,'Cliente privado','5511999990000','private@example.org','Dado privado') RETURNING id", store).Scan(&customer))
	stock := func(id int64) int64 {
		var n int64
		must(a.DB.QueryRow(ctx, "SELECT stock_quantity FROM products WHERE id=$1", id).Scan(&n))
		return n
	}
	item := app.Item{ComboID: combo, Quantity: 2, Notes: "Mesa", Components: []app.Item{{ProductID: product, Quantity: 1, Notes: "Sem cebola"}, {ProductID: drink, Quantity: 1}}}
	req := app.OrderRequest{Items: []app.Item{item}, CustomerID: customer}
	key := app.Token()
	order, e := a.CreateOrder(ctx, store, user, key, req)
	must(e)
	var total int64
	must(a.DB.QueryRow(ctx, "SELECT total_cents FROM orders WHERE id=$1", order).Scan(&total))
	if total != 5002 || stock(product) != 96 || stock(drink) != 98 {
		t.Fatalf("price/stock %d %d %d", total, stock(product), stock(drink))
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, e := a.CreateOrder(ctx, store, user, key, req)
			if e != nil || id != order {
				t.Errorf("retry %d %v", id, e)
			}
		}()
	}
	wg.Wait()
	if stock(product) != 96 {
		t.Fatal("retry stock")
	}
	code, raw, _ = call("GET", fmt.Sprintf("/api/customer-export?customer_id=%d", customer), nil, admin)
	if code != 200 || !bytes.Contains(raw, []byte("Sem cebola")) || !bytes.Contains(raw, []byte("5002")) {
		t.Fatalf("export %d %s", code, raw)
	}
	code, raw, _ = call("PATCH", fmt.Sprintf("/api/combos?id=%d", combo), map[string]any{"name": "Combo atualizado", "description": "Alterado", "price_cents": 3000, "pricing_policy": "fixed", "active": true}, admin)
	if code != 201 {
		t.Fatalf("edit %d %s", code, raw)
	}
	code, raw, _ = call("GET", fmt.Sprintf("/api/orders?id=%d", order), nil, admin)
	if code != 200 || !bytes.Contains(raw, []byte("Combo QA")) || bytes.Contains(raw, []byte("Combo atualizado")) {
		t.Fatalf("snapshot %d %s", code, raw)
	}
	code, raw, _ = call("POST", "/api/order-state", map[string]any{"id": order, "state": "cancelled"}, admin)
	if code != 200 || stock(product) != 100 || stock(drink) != 100 {
		t.Fatalf("cancel %d %s", code, raw)
	}
	code, _, _ = call("POST", "/api/order-state", map[string]any{"id": order, "state": "cancelled"}, admin)
	if code != 409 || stock(product) != 100 {
		t.Fatal("double cancel")
	}
	cfg := app.MenuConfiguration{ProductID: product, Version: 1, Variants: []app.MenuVariant{{Name: "Grande", Adjustment: 500, Stock: 100, Active: true}}, Groups: []app.MenuGroup{{Name: "Extra", Min: 1, Max: 1, Required: true, Active: true, Options: []app.MenuOption{{Name: "Queijo", Price: 200, Stock: 100, Active: true}}}}}
	code, raw, _ = call("POST", fmt.Sprintf("/api/product-configuration?product_id=%d", product), cfg, admin)
	if code != 200 {
		t.Fatal(string(raw))
	}
	must(json.Unmarshal(raw, &cfg))
	req.Items[0].Components[0].VariantID = cfg.Variants[0].ID
	req.Items[0].Components[0].Addons = []int64{cfg.Groups[0].Options[0].ID}
	req.Items[0].Components[0].ConfigurationVersion = cfg.Version
	configured := req
	configured.Items = append([]app.Item(nil), req.Items...)
	configured.Items[0].Quantity = 1
	configuredID, e := a.CreateOrder(ctx, store, user, app.Token(), configured)
	must(e)
	must(a.DB.QueryRow(ctx, "SELECT total_cents FROM orders WHERE id=$1", configuredID).Scan(&total))
	if total != 4400 {
		t.Fatalf("fixed plus adjustment %d", total)
	}
	for _, table := range []string{"product_variants", "addon_options"} {
		var qty int64
		must(a.DB.QueryRow(ctx, "SELECT stock_quantity FROM "+table+" WHERE store_id=$1", store).Scan(&qty))
		if qty != 98 {
			t.Fatalf("component option stock %s %d", table, qty)
		}
	}
	code, raw, _ = call("POST", "/api/order-state", map[string]any{"id": configuredID, "state": "cancelled"}, admin)
	if code != 200 {
		t.Fatal(string(raw))
	}
	for policy, want := range map[string]int64{"sum": 5898, "highest": 5398} {
		_, err = a.DB.Exec(ctx, "UPDATE combos SET pricing_policy=$2 WHERE id=$1", combo, policy)
		must(err)
		id, e := a.CreateOrder(ctx, store, user, app.Token(), configured)
		must(e)
		must(a.DB.QueryRow(ctx, "SELECT total_cents FROM orders WHERE id=$1", id).Scan(&total))
		if total != want {
			t.Fatalf("%s price %d", policy, total)
		}
		code, raw, _ = call("POST", "/api/order-state", map[string]any{"id": id, "state": "cancelled"}, admin)
		if code != 200 {
			t.Fatal(string(raw))
		}
	}
	_, err = a.DB.Exec(ctx, "UPDATE combos SET pricing_policy='fixed' WHERE id=$1", combo)
	must(err)
	stale := configured
	stale.Items = append([]app.Item(nil), configured.Items...)
	stale.Items[0].ComboVersion = "old"
	if _, e = a.CreateOrder(ctx, store, user, app.Token(), stale); e == nil || e.Error() != "configuration_changed" {
		t.Fatal("changed combo accepted")
	}
	bad := req
	bad.CustomerID = 999999
	if _, e = a.CreateOrder(ctx, store, user, app.Token(), bad); e == nil || stock(product) != 100 {
		t.Fatal("foreign customer rollback")
	}
	bad = req
	bad.Items = []app.Item{{ComboID: combo, Quantity: 1, ProductID: product}}
	if _, e = a.CreateOrder(ctx, store, user, app.Token(), bad); e == nil {
		t.Fatal("combo product injected")
	}
	code, raw, _ = call("POST", fmt.Sprintf("/api/combo-items?combo_id=%d", combo), map[string]any{"product_id": drink, "quantity": 1, "is_optional": true, "group_name": "Bebida", "min_selections": 1, "max_selections": 1}, admin)
	if code != 200 {
		t.Fatalf("optional config %d %s", code, raw)
	}
	bad = req
	bad.Items = []app.Item{{ComboID: combo, Quantity: 1}}
	if _, e = a.CreateOrder(ctx, store, user, app.Token(), bad); e == nil || stock(product) != 100 {
		t.Fatal("optional minimum rollback")
	}
	_, err = a.DB.Exec(ctx, "UPDATE products SET stock_quantity=2 WHERE id=$1", product)
	must(err)
	req.Items[0].Quantity = 1
	successes := 0
	var mutex sync.Mutex
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := a.CreateOrder(ctx, store, user, app.Token(), req)
			if e == nil {
				mutex.Lock()
				successes++
				mutex.Unlock()
			}
		}()
	}
	wg.Wait()
	if successes != 1 || stock(product) != 0 || stock(drink) != 99 {
		t.Fatal("last component concurrency")
	}
	code, raw, _ = call("POST", "/api/consents", map[string]any{"customer_id": customer, "terms_text": "Termos QA", "purpose": "marketing", "version": "1", "granted": true, "source": "qa", "evidence": map[string]string{"text": "Consentimento QA"}}, admin)
	if code != 200 {
		t.Fatal(string(raw))
	}
	code, raw, _ = call("POST", "/api/consents", map[string]any{"customer_id": customer, "purpose": "marketing", "version": "1", "granted": true, "terms_text": "Alterado", "source": "qa"}, admin)
	if code != 409 {
		t.Fatal("consent overwritten")
	}
	for _, role := range []string{"attendant", "finance_viewer"} {
		email := "combo-" + role + "@example.org"
		_, err = a.DB.Exec(ctx, "INSERT INTO users(org_id,store_id,email,password_hash,role,active) SELECT org_id,store_id,$2,password_hash,$3,true FROM users WHERE id=$1", user, email, role)
		must(err)
		_, _, res = call("POST", "/api/login", map[string]string{"email": email, "password": "QA-cash-test-password"}, nil)
		cookie := res.Cookies()[0]
		code, _, _ = call("POST", "/api/customers", map[string]any{"name": "Forbidden"}, cookie)
		if code != 403 {
			t.Fatalf("customer mutation allowed %s", role)
		}
		code, _, _ = call("POST", "/api/consents", map[string]any{}, cookie)
		if code != 403 {
			t.Fatalf("consent mutation allowed %s", role)
		}
		code, _, _ = call("DELETE", fmt.Sprintf("/api/customer-delete?customer_id=%d", customer), nil, cookie)
		if code != 403 {
			t.Fatalf("removal allowed %s", role)
		}
		if role == "attendant" {
			for _, path := range []string{"/api/combos", fmt.Sprintf("/api/combo-items?combo_id=%d", combo), "/api/customer-lookup"} {
				code, raw, _ = call("GET", path, nil, cookie)
				if code != 200 {
					t.Fatalf("att read %s %d %s", path, code, raw)
				}
				code, _, _ = call("GET", path, nil, cookie, "X-Store-ID", fmt.Sprint(otherStore))
				if code != 403 {
					t.Fatalf("cross-store %s", path)
				}
			}
		}
	}
	code, raw, _ = call("DELETE", fmt.Sprintf("/api/customer-delete?customer_id=%d", customer), nil, admin)
	if code != 200 {
		t.Fatalf("anonymize %d %s", code, raw)
	}
	code, raw, _ = call("GET", fmt.Sprintf("/api/customer-export?customer_id=%d", customer), nil, admin)
	if code != 200 || bytes.Contains(raw, []byte("private@example.org")) || bytes.Contains(raw, []byte("5511999990000")) || bytes.Contains(raw, []byte("Sem cebola")) || !bytes.Contains(raw, []byte("5002")) {
		t.Fatalf("anonymization export %d %s", code, raw)
	}
	_, err = a.DB.Exec(ctx, "UPDATE products SET stock_quantity=100 WHERE store_id=$1", store)
	must(err)
	if _, e = a.CreateOrder(ctx, store, user, app.Token(), req); e == nil || e.Error() != "customer_invalid" {
		t.Fatal("anonymized reused")
	}
	dump, e := runtime.Backup(ctx)
	must(e)
	saved, e := os.ReadFile(dump)
	must(e)
	_, err = a.DB.Exec(ctx, "DELETE FROM combos WHERE store_id=$1", store)
	must(err)
	must(runtime.Restore(ctx, saved))
	var anon bool
	must(a.DB.QueryRow(ctx, "SELECT anonymized_at IS NOT NULL FROM customers WHERE id=$1", customer).Scan(&anon))
	if !anon {
		t.Fatal("anonymized flag lost after backup restore")
	}
	must(a.DB.QueryRow(ctx, "SELECT total_cents FROM orders WHERE id=$1", order).Scan(&total))
	if total != 5002 {
		t.Fatal("combo history restore")
	}
	t.Log("PASS: combo price, component stock, snapshots, retries, optional groups, last-unit concurrency, customer isolation, export, anonymization and financial history")
}
