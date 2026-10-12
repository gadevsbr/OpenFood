package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
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

func TestMenuJourney(t *testing.T) {
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
	endpoint := fmt.Sprintf("/api/product-configuration?product_id=%d", product)
	cfg := app.MenuConfiguration{ProductID: product, Version: 1, Variants: []app.MenuVariant{{Name: "Pequeno", Stock: 10, Active: true}, {Name: "Grande", Adjustment: 500, Stock: 3, Active: true}}, Groups: []app.MenuGroup{{Name: "Adicionais", Min: 1, Max: 2, Required: true, Active: true, Options: []app.MenuOption{{Name: "Bacon", Price: 300, Stock: 3, Active: true}, {Name: "Queijo", Price: 200, Stock: 5, Active: true}}}}}
	code, raw, _ = call("POST", endpoint, cfg, admin)
	if code != 200 {
		t.Fatalf("config %d %s", code, raw)
	}
	if err = json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{fmt.Sprintf("/api/product-variants?product_id=%d", product), "/api/addon-groups", fmt.Sprintf("/api/addon-options?group_id=%d", cfg.Groups[0].ID)} {
		code, body, _ := call("GET", path, nil, admin)
		if code != 200 {
			t.Fatalf("legacy catalog after configuration %s: %d %s", path, code, body)
		}
	}
	variant, option := cfg.Variants[1].ID, cfg.Groups[0].Options[0].ID
	var thirdOption int64
	if err = a.DB.QueryRow(ctx, "INSERT INTO addon_options(store_id,group_id,name,price_cents,stock_quantity,active) VALUES($1,$2,'Terceira escolha',100,10,true) RETURNING id", store, cfg.Groups[0].ID).Scan(&thirdOption); err != nil {
		t.Fatal(err)
	}
	stock := func(table string, id int64) float64 {
		t.Helper()
		var n float64
		if e := a.DB.QueryRow(ctx, "SELECT stock_quantity FROM "+table+" WHERE id=$1", id).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	expected := cfg.Variants[1].Stock
	cfg.Variants[1].ExpectedStock = &expected
	for i := range cfg.Variants {
		n := cfg.Variants[i].Stock
		cfg.Variants[i].ExpectedStock = &n
	}
	for i := range cfg.Groups {
		for j := range cfg.Groups[i].Options {
			n := cfg.Groups[i].Options[j].Stock
			cfg.Groups[i].Options[j].ExpectedStock = &n
		}
	}
	invalid := []app.Item{{ProductID: product, Quantity: 1}, {ProductID: product, Quantity: 1, VariantID: variant}, {ProductID: product, Quantity: 1, VariantID: variant, Addons: []int64{option, option}}, {ProductID: product, Quantity: 1, VariantID: variant, Addons: []int64{999999}}, {ProductID: product, Quantity: 1, VariantID: variant, Addons: []int64{option}, ConfigurationVersion: 1}}
	for _, item := range invalid {
		if _, e := a.CreateOrder(ctx, store, user, app.Token(), app.OrderRequest{Items: []app.Item{item}}); e == nil {
			t.Fatalf("accepted invalid selection %+v", item)
		}
	}
	tooMany := app.Item{ProductID: product, Quantity: 1, VariantID: variant, Addons: []int64{option, cfg.Groups[0].Options[1].ID, thirdOption}}
	if _, err = a.CreateOrder(ctx, store, user, app.Token(), app.OrderRequest{Items: []app.Item{tooMany}}); err == nil || err.Error() != "addon_limits_invalid" {
		t.Fatalf("max choices %v", err)
	}
	if stock("addon_options", thirdOption) != 10 || stock("addon_options", cfg.Groups[0].Options[1].ID) != 5 {
		t.Fatal("max-choice rejection changed option stock")
	}
	if stock("products", product) != 100 || stock("product_variants", variant) != 3 || stock("addon_options", option) != 3 {
		t.Fatal("invalid selection consumed stock")
	}
	item := app.Item{ProductID: product, Quantity: 2, VariantID: variant, Addons: []int64{option}, Notes: "Sem cebola", ConfigurationVersion: cfg.Version}
	key := app.Token()
	order, e := a.CreateOrder(ctx, store, user, key, app.OrderRequest{Items: []app.Item{item}})
	if e != nil {
		t.Fatal(e)
	}
	if stock("products", product) != 98 || stock("product_variants", variant) != 1 || stock("addon_options", option) != 1 {
		t.Fatal("three stocks not consumed")
	}
	var total int64
	if e = a.DB.QueryRow(ctx, "SELECT total_cents FROM orders WHERE id=$1", order).Scan(&total); e != nil || total != 5598 {
		t.Fatalf("total %d %v", total, e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, e := a.CreateOrder(ctx, store, user, key, app.OrderRequest{Items: []app.Item{item}})
			if e != nil || id != order {
				t.Errorf("retry %d %v", id, e)
			}
		}()
	}
	wg.Wait()
	changed := item
	changed.Notes = "Outro preparo"
	if _, e = a.CreateOrder(ctx, store, user, key, app.OrderRequest{Items: []app.Item{changed}}); e == nil {
		t.Fatal("idempotency conflict accepted")
	}
	code, raw, _ = call("POST", endpoint, cfg, admin)
	if code != 409 || !bytes.Contains(raw, []byte("configuration_stock_changed")) {
		t.Fatalf("stale stock editor %d %s", code, raw)
	}
	if _, e = a.DB.Exec(ctx, "UPDATE addon_options SET name='Bacon revisado',price_cents=350 WHERE id=$1", option); e != nil {
		t.Fatal(e)
	}
	code, raw, _ = call("GET", fmt.Sprintf("/api/orders?id=%d", order), nil, admin)
	if code != 200 || !bytes.Contains(raw, []byte("Sem cebola")) || !bytes.Contains(raw, []byte("Bacon")) || bytes.Contains(raw, []byte("Bacon revisado")) {
		t.Fatalf("snapshot %d %s", code, raw)
	}
	item.Quantity = 1
	success := make(chan int64, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, e := a.CreateOrder(ctx, store, user, app.Token(), app.OrderRequest{Items: []app.Item{item}})
			if e == nil {
				success <- id
			}
		}()
	}
	wg.Wait()
	close(success)
	if len(success) != 1 {
		t.Fatal("last variant/addon sold more than once")
	}
	if stock("product_variants", variant) != 0 || stock("addon_options", option) != 0 {
		t.Fatal("remaining stock")
	}
	code, raw, _ = call("POST", "/api/order-state", map[string]any{"id": order, "state": "cancelled"}, admin)
	if code != 200 {
		t.Fatalf("cancel %d %s", code, raw)
	}
	if stock("product_variants", variant) != 2 || stock("addon_options", option) != 2 || stock("products", product) != 99 {
		t.Fatal("cancellation did not restore stocks")
	}
	code, _, _ = call("POST", "/api/order-state", map[string]any{"id": order, "state": "cancelled"}, admin)
	if code != 409 || stock("product_variants", variant) != 2 {
		t.Fatal("double cancellation")
	}
	code, raw, _ = call("POST", "/api/cash-sessions", app.CashRequest{Action: "open"}, admin, "Idempotency-Key", app.Token())
	if code != 200 {
		t.Fatalf("open cash %d %s", code, raw)
	}
	checkout := app.CheckoutRequest{Items: []app.Item{item}, Expected: 2849, Service: "kitchen", Payments: []app.POSPayment{{Method: "card", Amount: 2849}}}
	checkoutKey := app.Token()
	id, e := a.Checkout(ctx, store, user, checkoutKey, checkout)
	if e != nil {
		t.Fatal(e)
	}
	retry, e := a.Checkout(ctx, store, user, checkoutKey, checkout)
	if e != nil || retry != id {
		t.Fatal("checkout retry")
	}
	bad := checkout
	bad.Expected = 1
	before := stock("product_variants", variant)
	if _, e = a.Checkout(ctx, store, user, app.Token(), bad); e == nil || stock("product_variants", variant) != before {
		t.Fatal("manipulated expected total committed")
	}
	code, raw, _ = call("GET", fmt.Sprintf("/api/pos-receipt?id=%d", id), nil, admin)
	if code != 200 || !bytes.Contains(raw, []byte("Sem cebola")) || !bytes.Contains(raw, []byte("Grande")) {
		t.Fatalf("receipt %d %s", code, raw)
	}
	// A configured product can have distinct lines, but identical choices remain invalid.
	_, e = a.CreateOrder(ctx, store, user, app.Token(), app.OrderRequest{Items: []app.Item{{ProductID: product, Quantity: 1, VariantID: cfg.Variants[0].ID, Addons: []int64{cfg.Groups[0].Options[1].ID}, Notes: "Um"}, {ProductID: product, Quantity: 1, VariantID: cfg.Variants[0].ID, Addons: []int64{cfg.Groups[0].Options[1].ID}, Notes: "Dois"}}})
	if e != nil {
		t.Fatalf("distinct lines %v", e)
	}
	// Photo pixels are in PostgreSQL and survive backup/restore; SVG/oversized rejected.
	var photo bytes.Buffer
	png.Encode(&photo, image.NewRGBA(image.Rect(0, 0, 32, 24)))
	upload := func(data []byte, cookie *http.Cookie, headers ...string) (int, []byte) {
		req, _ := http.NewRequest("POST", server.URL+fmt.Sprintf("/api/product-photo?product_id=%d", product), bytes.NewReader(data))
		req.Header.Set("X-OpenFood", "1")
		req.AddCookie(cookie)
		for i := 0; i < len(headers); i += 2 {
			req.Header.Set(headers[i], headers[i+1])
		}
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		var b bytes.Buffer
		b.ReadFrom(res.Body)
		return res.StatusCode, b.Bytes()
	}
	code, raw = upload(photo.Bytes(), admin)
	if code != 200 {
		t.Fatalf("photo %d %s", code, raw)
	}
	code, _ = upload([]byte("<svg onload='alert(1)'/>"), admin)
	if code != 400 {
		t.Fatal("SVG accepted")
	}
	code, _ = upload(bytes.Repeat([]byte{'a'}, (2<<20)+1), admin)
	if code != 413 {
		t.Fatal("oversized photo accepted")
	}
	code, raw, _ = call("GET", fmt.Sprintf("/api/product-photo?product_id=%d&store_id=%d", product, store), nil, admin)
	if code != 200 || !bytes.HasPrefix(raw, []byte{0xff, 0xd8}) {
		t.Fatal("JPEG not served")
	}
	code, _, _ = call("DELETE", fmt.Sprintf("/api/product-photo?product_id=%d", product), nil, admin)
	if code != 200 {
		t.Fatal("photo delete")
	}
	code, _, _ = call("GET", fmt.Sprintf("/api/product-photo?product_id=%d", product), nil, admin)
	if code != 404 {
		t.Fatal("deleted photo still served")
	}
	code, raw = upload(photo.Bytes(), admin)
	if code != 200 {
		t.Fatalf("photo reupload %d %s", code, raw)
	}
	if _, e = a.DB.Exec(ctx, "INSERT INTO users(org_id,store_id,email,password_hash,role,active) SELECT org_id,store_id,'menu-att@example.org',password_hash,'attendant',true FROM users WHERE id=$1", user); e != nil {
		t.Fatal(e)
	}
	_, _, r2 := call("POST", "/api/login", map[string]string{"email": "menu-att@example.org", "password": "QA-cash-test-password"}, nil)
	att := r2.Cookies()[0]
	code, _ = upload(photo.Bytes(), att)
	if code != 403 {
		t.Fatal("attendant uploaded photo")
	}
	code, _, _ = call("POST", endpoint, cfg, att)
	if code != 403 {
		t.Fatal("attendant edited menu")
	}
	code, _, _ = call("GET", endpoint, nil, att, "X-Store-ID", fmt.Sprint(otherStore))
	if code != 403 {
		t.Fatal("cross-store configuration")
	}
	code, _, _ = call("GET", fmt.Sprintf("/api/product-photo?product_id=%d&store_id=%d", product, otherStore), nil, att)
	if code != 403 {
		t.Fatal("cross-store photo")
	}
	var otherVariant int64
	if e = a.DB.QueryRow(ctx, "INSERT INTO products(store_id,name,price_cents,stock_quantity) VALUES($1,'Outro',100,10) RETURNING id", otherStore).Scan(&otherVariant); e != nil {
		t.Fatal(e)
	}
	if e = a.DB.QueryRow(ctx, "INSERT INTO product_variants(store_id,product_id,name,stock_quantity,active) VALUES($1,$2,'Outro tamanho',10,true) RETURNING id", otherStore, otherVariant).Scan(&otherVariant); e != nil {
		t.Fatal(e)
	}
	foreign := item
	foreign.VariantID = otherVariant
	if _, e = a.CreateOrder(ctx, store, user, app.Token(), app.OrderRequest{Items: []app.Item{foreign}}); e == nil {
		t.Fatal("foreign variant accepted")
	}
	dump, e := runtime.Backup(ctx)
	if e != nil {
		t.Fatal(e)
	}
	saved, e := os.ReadFile(dump)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = a.DB.Exec(ctx, "DELETE FROM product_photos"); e != nil {
		t.Fatal(e)
	}
	if e = runtime.Restore(ctx, saved); e != nil {
		t.Fatal(e)
	}
	var restored []byte
	if e = a.DB.QueryRow(ctx, "SELECT data FROM product_photos WHERE store_id=$1 AND product_id=$2", store, product).Scan(&restored); e != nil || !bytes.HasPrefix(restored, []byte{0xff, 0xd8}) {
		t.Fatal("photo restore")
	}
	var note string
	if e = a.DB.QueryRow(ctx, "SELECT notes FROM order_items WHERE order_id=$1", id).Scan(&note); e != nil || !strings.Contains(note, "Sem cebola") {
		t.Fatal("configured order restore")
	}
}
