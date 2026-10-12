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
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPizzaJourney(t *testing.T) {
	bin := os.Getenv("OPENFOOD_TEST_PG_BIN")
	if bin == "" {
		t.Skip("requires real PostgreSQL")
	}
	bin, _ = filepath.Abs(bin)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	r, err := local.Load(filepath.Join(t.TempDir(), "Pizza QA isolado"), bin)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		stop, end := context.WithTimeout(context.Background(), 30*time.Second)
		defer end()
		r.Stop(stop)
	}()
	a, err := app.Open(ctx, r.DSN, r.Config.SetupToken, r)
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
	code, raw, _ := call("POST", "/api/setup", map[string]string{"token": r.Config.SetupToken, "email": "pizza-qa@example.org", "password": "QA-pizza-test-password", "store": "Pizzaria QA"}, nil)
	if code != 201 {
		t.Fatalf("setup %d %s", code, raw)
	}
	code, _, res := call("POST", "/api/login", map[string]string{"email": "pizza-qa@example.org", "password": "QA-pizza-test-password"}, nil)
	if code != 200 {
		t.Fatal("login")
	}
	admin := res.Cookies()[0]
	var store, user, org, otherStore, product int64
	if err = a.DB.QueryRow(ctx, "SELECT store_id,id,org_id FROM users WHERE email='pizza-qa@example.org'").Scan(&store, &user, &org); err != nil {
		t.Fatal(err)
	}
	if err = a.DB.QueryRow(ctx, "INSERT INTO products(store_id,name,price_cents,stock_quantity) VALUES($1,'Pizza QA',3001,10) RETURNING id", store).Scan(&product); err != nil {
		t.Fatal(err)
	}
	if err = a.DB.QueryRow(ctx, "INSERT INTO stores(org_id,name) VALUES($1,'Outra loja QA') RETURNING id", org).Scan(&otherStore); err != nil {
		t.Fatal(err)
	}
	menuEndpoint := fmt.Sprintf("/api/product-configuration?product_id=%d", product)
	menu := app.MenuConfiguration{ProductID: product, Version: 1, Variants: []app.MenuVariant{{Name: "Grande", Adjustment: 500, Stock: 10, Active: true}}, Groups: []app.MenuGroup{{Name: "Bordas QA", Max: 1, Active: true, Options: []app.MenuOption{{Name: "Catupiry", Price: 300, Stock: 10, Active: true}}}}}
	code, raw, _ = call("POST", menuEndpoint, menu, admin)
	if code != 200 {
		t.Fatalf("menu %d %s", code, raw)
	}
	json.Unmarshal(raw, &menu)
	endpoint := fmt.Sprintf("/api/pizza-configuration?product_id=%d", product)
	c := app.PizzaConfiguration{Enabled: true, Policy: "weighted_average", MaxFlavors: 2, Flavors: []app.PizzaFlavor{{Name: "Calabresa", Price: 3001, Active: true}, {Name: "Queijo", Price: 4002, Active: true}, {Name: "Desativado", Price: 8000, Active: false}}}
	save := func(c app.PizzaConfiguration, version int64) (int, []byte) {
		status, b, _ := call("POST", endpoint, map[string]any{"product_id": product, "version": version, "pizza": c}, admin)
		return status, b
	}
	code, raw = save(c, menu.Version)
	if code != 200 {
		t.Fatalf("pizza config %d %s", code, raw)
	}
	code, raw, _ = call("GET", menuEndpoint, nil, admin)
	if code != 200 {
		t.Fatal("read menu")
	}
	json.Unmarshal(raw, &menu)
	c = menu.Pizza
	code, raw = save(c, menu.Version-1)
	if code != 409 || !bytes.Contains(raw, []byte("configuration_changed")) {
		t.Fatalf("stale config %d %s", code, raw)
	}
	code, raw, _ = call("GET", endpoint, nil, admin, "X-Store-ID", fmt.Sprint(otherStore))
	if code != 404 {
		t.Fatalf("cross store read %d %s", code, raw)
	}
	invalid := c
	invalid.Flavors = append([]app.PizzaFlavor(nil), c.Flavors...)
	invalid.Flavors[0].ID = "unknown-other-product"
	code, raw = save(invalid, menu.Version)
	if code != 400 {
		t.Fatalf("foreign flavor update %d %s", code, raw)
	}
	// A restricted operator cannot alter configuration or read another store.
	code, raw, _ = call("POST", "/api/users", map[string]any{"email": "pizza-attendant@example.org", "password": "QA-pizza-test-password", "role": "attendant", "store_id": store}, admin)
	if code != 201 {
		t.Fatalf("user %d %s", code, raw)
	}
	_, _, res = call("POST", "/api/login", map[string]string{"email": "pizza-attendant@example.org", "password": "QA-pizza-test-password"}, nil)
	attendant := res.Cookies()[0]
	code, raw, _ = call("POST", endpoint, map[string]any{"product_id": product, "version": menu.Version, "pizza": c}, attendant)
	if code != 403 {
		t.Fatalf("operator editing %d %s", code, raw)
	}
	code, raw, _ = call("GET", endpoint, nil, attendant, "X-Store-ID", fmt.Sprint(otherStore))
	if code != 403 {
		t.Fatalf("cross store permission %d %s", code, raw)
	}
	item := app.Item{ProductID: product, Quantity: 2, VariantID: menu.Variants[0].ID, Addons: []int64{menu.Groups[0].Options[0].ID}, ConfigurationVersion: menu.Version, Flavors: []app.PizzaPart{{ID: c.Flavors[0].ID, Parts: 1}, {ID: c.Flavors[1].ID, Parts: 1}}, Notes: "Sem cebola QA"}
	order, err := a.CreateOrder(ctx, store, user, "pizza-order-first", app.OrderRequest{Items: []app.Item{item}})
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	var name string
	var snapshot []byte
	if err = a.DB.QueryRow(ctx, "SELECT total_cents FROM orders WHERE id=$1", order).Scan(&total); err != nil || total != 8604 {
		t.Fatalf("price %d %v", total, err)
	}
	if err = a.DB.QueryRow(ctx, "SELECT name,selection FROM order_items WHERE order_id=$1", order).Scan(&name, &snapshot); err != nil || !strings.Contains(name, "1/2 Calabresa") || !strings.Contains(string(snapshot), "weighted_average") {
		t.Fatalf("snapshot %s %s %v", name, snapshot, err)
	}
	stock := func() (int64, int64, int64) {
		var p, v, o int64
		if e := a.DB.QueryRow(ctx, "SELECT p.stock_quantity,v.stock_quantity,o.stock_quantity FROM products p JOIN product_variants v ON v.product_id=p.id JOIN addon_options o ON o.id=$2 WHERE p.id=$1", product, item.Addons[0]).Scan(&p, &v, &o); e != nil {
			t.Fatal(e)
		}
		return p, v, o
	}
	if p, v, o := stock(); p != 8 || v != 8 || o != 8 {
		t.Fatalf("fraction changed inventory %d/%d/%d", p, v, o)
	}
	// Same key must preserve exactly one sale and one stock deduction.
	for i := 0; i < 2; i++ {
		replay, e := a.CreateOrder(ctx, store, user, "pizza-order-first", app.OrderRequest{Items: []app.Item{item}})
		if e != nil || replay != order {
			t.Fatalf("replay %d %v", replay, e)
		}
	}
	bad := item
	bad.Flavors = []app.PizzaPart{{ID: "other-store", Parts: 1}}
	if _, e := a.CreateOrder(ctx, store, user, "pizza-invalid-flavor", app.OrderRequest{Items: []app.Item{bad}}); e == nil {
		t.Fatal("unknown flavor accepted")
	}
	bad = item
	bad.Flavors = []app.PizzaPart{{ID: c.Flavors[2].ID, Parts: 1}}
	if _, e := a.CreateOrder(ctx, store, user, "pizza-inactive-flavor", app.OrderRequest{Items: []app.Item{bad}}); e == nil {
		t.Fatal("inactive flavor accepted")
	}
	// Checkout recalculates on the server and rolls back stock on manipulated totals.
	code, raw, _ = call("POST", "/api/cash-sessions", map[string]any{"action": "open", "opening_cents": 0}, admin, "Idempotency-Key", "pizza-cash-open")
	if code != 201 && code != 200 {
		t.Fatalf("cash %d %s", code, raw)
	}
	checkout := app.CheckoutRequest{Items: []app.Item{item}, Expected: 1, Service: "kitchen", Payments: []app.POSPayment{{Method: "cash", Amount: 1, Tendered: 1}}}
	code, raw, _ = call("POST", "/api/pos-checkout", checkout, admin, "Idempotency-Key", "pizza-forged-total")
	if code != 409 {
		t.Fatalf("forged total %d %s", code, raw)
	}
	if p, v, o := stock(); p != 8 || v != 8 || o != 8 {
		t.Fatalf("rollback %d/%d/%d", p, v, o)
	}
	// Configuration changes do not rewrite sold snapshots; stale carts are rejected.
	c.Policy = "highest"
	c.Flavors[0].Name = "Calabresa nova"
	c.Flavors[1].Price = 5000
	code, raw = save(c, menu.Version)
	if code != 200 {
		t.Fatalf("update %d %s", code, raw)
	}
	if _, e := a.CreateOrder(ctx, store, user, "pizza-stale-cart", app.OrderRequest{Items: []app.Item{item}}); e == nil || e.Error() != "configuration_changed" {
		t.Fatalf("stale cart %v", e)
	}
	var savedName string
	a.DB.QueryRow(ctx, "SELECT name FROM order_items WHERE order_id=$1", order).Scan(&savedName)
	if savedName != name {
		t.Fatal("sold name changed")
	}
	code, raw, _ = call("POST", "/api/order-state", map[string]any{"id": order, "state": "cancelled"}, admin)
	if code != 200 {
		t.Fatalf("cancel %d %s", code, raw)
	}
	if p, v, o := stock(); p != 10 || v != 10 || o != 10 {
		t.Fatalf("cancel %d/%d/%d", p, v, o)
	}
	// Weighted thirds, highest pricing and tab rounds use the same server path.
	call("GET", menuEndpoint, nil, admin)
	code, raw, _ = call("GET", menuEndpoint, nil, admin)
	json.Unmarshal(raw, &menu)
	item.Quantity = 1
	item.ConfigurationVersion = menu.Version
	item.Flavors[0].Parts = 2
	id, e := a.CreateOrder(ctx, store, user, "pizza-highest-order", app.OrderRequest{Items: []app.Item{item}})
	if e != nil {
		t.Fatal(e)
	}
	a.DB.QueryRow(ctx, "SELECT total_cents FROM orders WHERE id=$1", id).Scan(&total)
	if total != 5800 {
		t.Fatalf("highest total %d", total)
	}
	code, raw, _ = call("POST", "/api/dining-tabs", app.DiningRequest{Action: "open", Label: "Pizza QA"}, admin, "Idempotency-Key", "pizza-tab-open")
	if code != 201 && code != 200 {
		t.Fatalf("tab %d %s", code, raw)
	}
	var response map[string]any
	json.Unmarshal(raw, &response)
	tab := int64(response["id"].(float64))
	code, raw, _ = call("POST", "/api/dining-tabs", app.DiningRequest{Action: "add", ID: tab, Version: 1, Expected: 5800, Items: []app.Item{item}}, admin, "Idempotency-Key", "pizza-tab-round")
	if code != 201 && code != 200 {
		t.Fatalf("round %d %s", code, raw)
	}
	// Backup/restore retains configuration and historical snapshots.
	var combo int64
	if e = a.DB.QueryRow(ctx, "INSERT INTO combos(store_id,name,price_cents,pricing_policy) VALUES($1,'Pizza combo QA',0,'sum') RETURNING id", store).Scan(&combo); e != nil {
		t.Fatal(e)
	}
	if _, e = a.DB.Exec(ctx, "INSERT INTO combo_items(store_id,combo_id,product_id,quantity) VALUES($1,$2,$3,1)", store, combo, product); e != nil {
		t.Fatal(e)
	}
	comboOrder, e := a.CreateOrder(ctx, store, user, "pizza-combo-order", app.OrderRequest{Items: []app.Item{{ComboID: combo, Quantity: 1, Components: []app.Item{item}}}})
	if e != nil {
		t.Fatal(e)
	}
	if e = a.DB.QueryRow(ctx, "SELECT total_cents FROM orders WHERE id=$1", comboOrder).Scan(&total); e != nil || total != 5800 {
		t.Fatalf("pizza combo total %d %v", total, e)
	}
	var comboParts int64
	if e = a.DB.QueryRow(ctx, "SELECT (selection#>>'{combo,components,0,selection,pizza,total_parts}')::bigint FROM order_items WHERE order_id=$1", comboOrder).Scan(&comboParts); e != nil || comboParts != 3 {
		t.Fatalf("pizza combo snapshot %d %v", comboParts, e)
	}
	code, raw, _ = call("POST", "/api/order-state", map[string]any{"id": comboOrder, "state": "cancelled"}, admin)
	if code != 200 {
		t.Fatalf("combo cancel %d %s", code, raw)
	}
	if _, e = a.DB.Exec(ctx, "UPDATE products SET unit='porcao' WHERE id=$1", product); e != nil {
		t.Fatal(e)
	}
	if _, e = a.CreateOrder(ctx, store, user, "pizza-unit-changed", app.OrderRequest{Items: []app.Item{item}}); e == nil || e.Error() != "product_unavailable" {
		t.Fatalf("pizza non-unit %v", e)
	}
	if _, e = a.DB.Exec(ctx, "UPDATE products SET unit='unidade' WHERE id=$1", product); e != nil {
		t.Fatal(e)
	}
	path, e := r.Backup(ctx)
	if e != nil {
		t.Fatal(e)
	}
	dump, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Restore(ctx, dump); e != nil {
		t.Fatal(e)
	}
	if e = a.DB.QueryRow(ctx, "SELECT name FROM order_items WHERE order_id=$1", order).Scan(&savedName); e != nil || savedName != name {
		t.Fatalf("restored snapshot %v", e)
	}
	// Only one of two simultaneous buyers can consume the last whole pizza.
	if _, e = a.DB.Exec(ctx, "UPDATE products SET stock_quantity=1 WHERE id=$1", product); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := a.CreateOrder(ctx, store, user, fmt.Sprintf("pizza-last-%d", i), app.OrderRequest{Items: []app.Item{item}})
			results <- e
		}(i)
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		} else if e.Error() != "stock_insufficient" {
			t.Fatal(e)
		}
	}
	if success != 1 {
		t.Fatalf("last pizza winners %d", success)
	}
}
