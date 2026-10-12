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

func TestSharedAddonJourney(t *testing.T) {
	bin := os.Getenv("OPENFOOD_TEST_PG_BIN")
	if bin == "" {
		t.Skip("requires real PostgreSQL")
	}
	bin, _ = filepath.Abs(bin)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	r, err := local.Load(filepath.Join(t.TempDir(), "Adicionais QA isolado"), bin)
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

	var second, foreign int64
	a.DB.QueryRow(ctx, "INSERT INTO products(store_id,name,price_cents,stock_quantity) VALUES($1,'Sanduiche QA',2000,10) RETURNING id", store).Scan(&second)
	a.DB.QueryRow(ctx, "INSERT INTO products(store_id,name,price_cents,stock_quantity) VALUES($1,'Outra loja',2000,10) RETURNING id", otherStore).Scan(&foreign)
	c := app.AddonConfiguration{Group: app.MenuGroup{Name: "Molhos compartilhados QA", Max: 1, Active: true, Options: []app.MenuOption{{Name: "Molho especial", Price: 251, Stock: 3, Active: true}}}, Products: []app.AddonProduct{{ID: product}, {ID: second}}}
	endpoint := "/api/addon-configuration?group_id=0"
	code, raw, _ = call("POST", endpoint, c, admin)
	if code != 200 {
		t.Fatalf("create %d %s", code, raw)
	}
	json.Unmarshal(raw, &c)
	endpoint = fmt.Sprintf("/api/addon-configuration?group_id=%d", c.Group.ID)
	if !c.Group.Shared || len(c.Products) != 2 || c.Version == "" {
		t.Fatalf("shared metadata %+v", c)
	}
	get := func() app.AddonConfiguration {
		code, b, _ := call("GET", endpoint, nil, admin)
		if code != 200 {
			t.Fatalf("read %d %s", code, b)
		}
		var c app.AddonConfiguration
		json.Unmarshal(b, &c)
		return c
	}
	stock := func() int {
		var n int
		a.DB.QueryRow(ctx, "SELECT stock_quantity FROM addon_options WHERE id=$1", c.Group.Options[0].ID).Scan(&n)
		return n
	}
	item := app.Item{ProductID: product, Quantity: 1, Addons: []int64{c.Group.Options[0].ID}, ConfigurationVersion: 2}
	first, e := a.CreateOrder(ctx, store, user, "shared-first", app.OrderRequest{Items: []app.Item{item}})
	if e != nil {
		t.Fatal(e)
	}
	stale := c
	for i := range stale.Group.Options {
		v := stale.Group.Options[i].Stock
		stale.Group.Options[i].ExpectedStock = &v
	}
	code, raw, _ = call("POST", endpoint, stale, admin)
	if code != 409 || !bytes.Contains(raw, []byte("configuration_stock_changed")) {
		t.Fatalf("stock CAS %d %s", code, raw)
	}
	c = get()
	c.Group.Name = "Molhos da casa"
	c.Group.Options[0].Name = "Molho renomeado"
	c.Group.Options[0].Price = 399
	c.Group.Options[0].ExpectedStock = &c.Group.Options[0].Stock
	code, raw, _ = call("POST", endpoint, c, admin)
	if code != 200 {
		t.Fatalf("edit %d %s", code, raw)
	}
	json.Unmarshal(raw, &c)
	code, raw, _ = call("POST", endpoint, stale, admin)
	if code != 409 || !bytes.Contains(raw, []byte("configuration_changed")) {
		t.Fatalf("version CAS %d %s", code, raw)
	}
	if _, e = a.CreateOrder(ctx, store, user, "shared-stale-cart", app.OrderRequest{Items: []app.Item{item}}); e == nil || e.Error() != "configuration_changed" {
		t.Fatalf("stale cart %v", e)
	}
	if replay, e := a.CreateOrder(ctx, store, user, "shared-first", app.OrderRequest{Items: []app.Item{item}}); e != nil || replay != first {
		t.Fatalf("replay %d %v", replay, e)
	}
	var total int64
	a.DB.QueryRow(ctx, "SELECT total_cents FROM orders WHERE id=$1", first).Scan(&total)
	if total != 3252 {
		t.Fatalf("snapshot total %d", total)
	}
	var snapshot []byte
	a.DB.QueryRow(ctx, "SELECT selection FROM order_items WHERE order_id=$1", first).Scan(&snapshot)
	if !bytes.Contains(snapshot, []byte("Molho especial")) {
		t.Fatalf("snapshot mutated %s", snapshot)
	}
	for _, id := range []int64{product, second} {
		var menu app.MenuConfiguration
		code, b, _ := call("GET", fmt.Sprintf("/api/product-configuration?product_id=%d", id), nil, admin)
		json.Unmarshal(b, &menu)
		if code != 200 || menu.Version != 3 || !menu.Groups[0].Shared {
			t.Fatalf("menu propagation %d %+v", code, menu)
		}
	}
	invalid := get()
	invalid.Group.Options[0].ExpectedStock = &invalid.Group.Options[0].Stock
	invalid.Products = append(invalid.Products, app.AddonProduct{ID: foreign})
	code, raw, _ = call("POST", endpoint, invalid, admin)
	if code != 400 {
		t.Fatalf("foreign product %d %s", code, raw)
	}
	invalid = get()
	invalid.Group.Options[0].ExpectedStock = &invalid.Group.Options[0].Stock
	invalid.Group.Options[0].ID = 999999
	code, raw, _ = call("POST", endpoint, invalid, admin)
	if code != 400 {
		t.Fatalf("foreign option %d %s", code, raw)
	}
	// Multi-product order exhausts the common option pool atomically; failed order changes nothing.
	item.ConfigurationVersion = 3
	item.Quantity = 3
	if _, e = a.CreateOrder(ctx, store, user, "shared-insufficient", app.OrderRequest{Items: []app.Item{item}}); e == nil {
		t.Fatal("oversell")
	}
	if stock() != 2 {
		t.Fatal("failed order changed stock")
	}
	item.Quantity = 1
	orders := make(chan error, 3)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			it := item
			if i%2 == 1 {
				it.ProductID = second
			}
			_, e := a.CreateOrder(ctx, store, user, fmt.Sprintf("shared-last-%d", i), app.OrderRequest{Items: []app.Item{it}})
			orders <- e
		}(i)
	}
	wg.Wait()
	close(orders)
	success := 0
	for e := range orders {
		if e == nil {
			success++
		} else if e.Error() != "stock_insufficient" {
			t.Fatal(e)
		}
	}
	if success != 2 || stock() != 0 {
		t.Fatalf("concurrent pool %d %d", success, stock())
	}
	c = get()
	c.Group.Options[0].ExpectedStock = &c.Group.Options[0].Stock
	c.Group.Options[0].Active = false
	c.Products = []app.AddonProduct{{ID: second}}
	code, raw, _ = call("POST", endpoint, c, admin)
	if code != 200 {
		t.Fatalf("detach %d %s", code, raw)
	}
	json.Unmarshal(raw, &c)
	if c.Group.Shared || len(c.Products) != 1 {
		t.Fatal("detach not saved")
	}
	var links int
	a.DB.QueryRow(ctx, "SELECT count(*) FROM product_addon_groups WHERE product_id=$1", product).Scan(&links)
	if links != 0 {
		t.Fatal("old link remains")
	}
	// Legacy endpoints cannot attach a foreign product or create an option in another store's group.
	code, raw, _ = call("POST", fmt.Sprintf("/api/product-addon-groups?product_id=%d", foreign), map[string]any{"group_id": c.Group.ID}, admin)
	if code != 404 {
		t.Fatalf("legacy foreign product %d %s", code, raw)
	}
	var foreignGroup int64
	a.DB.QueryRow(ctx, "INSERT INTO addon_groups(store_id,name) VALUES($1,'Foreign') RETURNING id", otherStore).Scan(&foreignGroup)
	code, raw, _ = call("POST", fmt.Sprintf("/api/addon-options?group_id=%d", foreignGroup), map[string]any{"name": "Bad", "price_cents": 0, "stock_quantity": 1, "active": true}, admin)
	if code != 404 {
		t.Fatalf("legacy foreign group %d %s", code, raw)
	}
	// Restricted role cannot read or edit the shared editor.
	code, raw, _ = call("POST", "/api/users", map[string]any{"email": "addon-attendant@example.org", "password": "QA-addon-password", "role": "attendant", "store_id": store}, admin)
	if code != 201 {
		t.Fatalf("operator create %d %s", code, raw)
	}
	_, _, res = call("POST", "/api/login", map[string]string{"email": "addon-attendant@example.org", "password": "QA-addon-password"}, nil)
	if len(res.Cookies()) == 0 {
		t.Fatal("operator login")
	}
	operator := res.Cookies()[0]
	code, raw, _ = call("GET", endpoint, nil, operator)
	if code != 403 {
		t.Fatalf("restricted read %d %s", code, raw)
	}
	code, raw, _ = call("POST", endpoint, c, operator)
	if code != 403 {
		t.Fatalf("restricted write %d %s", code, raw)
	}
	// Restore preserves the shared option identity, remaining link, stock and original sale.
	code, raw, _ = call("GET", endpoint, nil, admin, "X-Store-ID", fmt.Sprint(otherStore))
	if code != 404 {
		t.Fatalf("cross store group %d %s", code, raw)
	}
	code, raw, _ = call("POST", "/api/order-state", map[string]any{"id": first, "state": "cancelled"}, admin)
	if code != 200 || stock() != 1 {
		t.Fatalf("inactive option cancellation %d %s stock %d", code, raw, stock())
	}
	backup, err := r.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dump, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Restore(ctx, dump); err != nil {
		t.Fatal(err)
	}
	if stock() != 1 {
		t.Fatal("restored stock changed")
	}
	var restored []byte
	if err = a.DB.QueryRow(ctx, "SELECT selection FROM order_items WHERE order_id=$1", first).Scan(&restored); err != nil || !bytes.Equal(restored, snapshot) {
		t.Fatalf("restored snapshot %s %v", restored, err)
	}
	if err = a.DB.QueryRow(ctx, "SELECT count(*) FROM product_addon_groups WHERE group_id=$1 AND product_id=$2", c.Group.ID, second).Scan(&links); err != nil || links != 1 {
		t.Fatalf("restored links %d %v", links, err)
	}

}
