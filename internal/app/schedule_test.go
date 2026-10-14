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
	"testing"
	"time"
)

func TestSaleScheduleJourney(t *testing.T) {
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

	must := func(e error) {
		if e != nil {
			t.Fatal(e)
		}
	}
	open := func(at string) bool {
		var v bool
		must(a.DB.QueryRow(ctx, "SELECT openfood_product_open($1,$2,$3::timestamptz)", store, product, at).Scan(&v))
		return v
	}
	read := func(category int64) app.ScheduleConfiguration {
		code, b, _ := call("GET", fmt.Sprintf("/api/sale-schedule?category_id=%d", category), nil, admin)
		if code != 200 {
			t.Fatalf("read %d %s", code, b)
		}
		var c app.ScheduleConfiguration
		must(json.Unmarshal(b, &c))
		return c
	}
	save := func(c app.ScheduleConfiguration) (int, []byte) {
		code, b, _ := call("POST", fmt.Sprintf("/api/sale-schedule?category_id=%d", c.CategoryID), c, admin)
		return code, b
	}
	c := read(0)
	if c.Timezone != "America/Sao_Paulo" || c.Schedule.Enabled {
		t.Fatalf("migration defaults %+v", c)
	}
	c.Schedule = app.SaleSchedule{Enabled: true, Windows: []app.SaleWindow{{Day: 1, Start: 660, End: 840}, {Day: 1, Start: 1080, End: 1320}, {Day: 6, Start: 1320, End: 120}}}
	code, raw = save(c)
	if code != 200 {
		t.Fatalf("save %d %s", code, raw)
	}
	for _, tc := range []struct {
		at   string
		want bool
	}{{"2026-10-12T13:59:59Z", false}, {"2026-10-12T14:00:00Z", true}, {"2026-10-12T16:59:59Z", true}, {"2026-10-12T17:00:00Z", false}, {"2026-10-12T21:00:00Z", true}, {"2026-10-18T01:00:00Z", true}, {"2026-10-18T04:59:59Z", true}, {"2026-10-18T05:00:00Z", false}} {
		if got := open(tc.at); got != tc.want {
			t.Fatalf("boundary %s got %v", tc.at, got)
		}
	}
	code, raw = save(c)
	if code != 409 {
		t.Fatalf("stale %d %s", code, raw)
	}
	c = read(0)
	c.Schedule.Windows = append(c.Schedule.Windows, app.SaleWindow{Day: 0, Start: 60, End: 180})
	code, raw = save(c)
	if code != 400 {
		t.Fatalf("overnight overlap %d %s", code, raw)
	}
	c = read(0)
	c.Timezone = "America/Manaus"
	code, raw = save(c)
	if code != 200 {
		t.Fatalf("timezone %d %s", code, raw)
	}
	if open("2026-10-12T14:00:00Z") || !open("2026-10-12T15:00:00Z") {
		t.Fatal("zone ignored")
	}
	c = read(0)
	c.Timezone = "Not/AZone"
	code, raw = save(c)
	if code != 400 {
		t.Fatalf("invalid zone %d %s", code, raw)
	}
	c = read(0)
	c.Timezone = "America/New_York"
	c.Schedule.Windows = []app.SaleWindow{{Day: 0, Start: 60, End: 180}}
	code, raw = save(c)
	if code != 200 {
		t.Fatal(string(raw))
	}
	// Both repeated 01:30 instances are inside the Sunday window at DST fall-back.
	if !open("2026-11-01T05:30:00Z") || !open("2026-11-01T06:30:00Z") || open("2026-11-01T08:00:00Z") {
		t.Fatal("DST local windows")
	}
	c = read(0)
	c.Timezone = "America/Sao_Paulo"
	c.Schedule.Enabled = false
	c.Schedule.Windows = []app.SaleWindow{}
	code, raw = save(c)
	if code != 200 {
		t.Fatal(string(raw))
	}
	var parent, child int64
	must(a.DB.QueryRow(ctx, "INSERT INTO categories(store_id,name) VALUES($1,'Almoco') RETURNING id", store).Scan(&parent))
	must(a.DB.QueryRow(ctx, "INSERT INTO categories(store_id,parent_id,name) VALUES($1,$2,'Pratos') RETURNING id", store, parent).Scan(&child))
	_, e := a.DB.Exec(ctx, "UPDATE products SET category_id=$2 WHERE id=$1", product, child)
	must(e)
	code, raw, _ = call("PATCH", "/api/categories", map[string]any{"id": parent, "parent_id": child}, admin)
	if code != 400 || !bytes.Contains(raw, []byte("category_cycle_invalid")) {
		t.Fatalf("cycle API %d %s", code, raw)
	}
	category := read(parent)
	category.Schedule = app.SaleSchedule{Enabled: true, Windows: []app.SaleWindow{{Day: 1, Start: 660, End: 840}}}
	code, raw = save(category)
	if code != 200 {
		t.Fatal(string(raw))
	}
	if !open("2026-10-12T14:00:00Z") || open("2026-10-12T18:00:00Z") {
		t.Fatal("parent not inherited")
	}
	_, e = a.DB.Exec(ctx, "UPDATE categories SET active=false WHERE id=$1", parent)
	must(e)
	if open("2026-10-12T14:00:00Z") {
		t.Fatal("inactive ancestor")
	}
	_, e = a.DB.Exec(ctx, "UPDATE categories SET active=true,parent_id=$2 WHERE id=$1", parent, child)
	must(e)
	if open("2026-10-12T14:00:00Z") {
		t.Fatal("cyclic hierarchy did not fail closed")
	}
	_, e = a.DB.Exec(ctx, "UPDATE categories SET parent_id=NULL WHERE id=$1", parent)
	must(e)
	category = read(parent)
	category.Schedule.Enabled = false
	code, raw = save(category)
	if code != 200 {
		t.Fatal(string(raw))
	}
	// Preserve the legacy daily product window, now interpreted in the store timezone.
	_, e = a.DB.Exec(ctx, "UPDATE products SET available_from='23:00',available_until='02:00' WHERE id=$1", product)
	must(e)
	if !open("2026-10-13T02:00:00Z") || !open("2026-10-13T04:00:00Z") || open("2026-10-13T15:00:00Z") {
		t.Fatal("legacy overnight rule")
	}
	_, e = a.DB.Exec(ctx, "UPDATE products SET available_from=NULL,available_until=NULL WHERE id=$1", product)
	must(e)
	version := func() int64 {
		var v int64
		must(a.DB.QueryRow(ctx, "SELECT configuration_version FROM products WHERE id=$1", product).Scan(&v))
		return v
	}
	item := app.Item{ProductID: product, Quantity: 1, ConfigurationVersion: version()}
	order, e := a.CreateOrder(ctx, store, user, "schedule-first", app.OrderRequest{Items: []app.Item{item}})
	must(e)
	c = read(0)
	c.Schedule.Enabled = true
	c.Schedule.Windows = []app.SaleWindow{}
	code, raw = save(c)
	if code != 200 {
		t.Fatal(string(raw))
	}
	if replay, e := a.CreateOrder(ctx, store, user, "schedule-first", app.OrderRequest{Items: []app.Item{item}}); e != nil || replay != order {
		t.Fatalf("replay after closure %v", e)
	}
	item.ConfigurationVersion = version()
	if _, e = a.CreateOrder(ctx, store, user, "schedule-blocked", app.OrderRequest{Items: []app.Item{item}}); e == nil || e.Error() != "product_unavailable" {
		t.Fatalf("closed sale %v", e)
	}
	var stock int64
	must(a.DB.QueryRow(ctx, "SELECT stock_quantity FROM products WHERE id=$1", product).Scan(&stock))
	if stock != 9 {
		t.Fatal("blocked sale touched stock")
	}
	code, raw, _ = call("GET", "/api/products", nil, admin)
	if code != 200 || !bytes.Contains(raw, []byte(`"available_now":false`)) {
		t.Fatalf("UI availability %d %s", code, raw)
	}
	// Foreign categories and restricted operators cannot mutate schedules.
	code, raw, _ = call("POST", "/api/cash-sessions", map[string]any{"action": "open", "opening_cents": 0}, admin, "Idempotency-Key", "schedule-cash-open")
	if code != 200 && code != 201 {
		t.Fatalf("cash fixture %d %s", code, raw)
	}
	code, raw, _ = call("POST", "/api/pos-checkout", app.CheckoutRequest{Items: []app.Item{item}, Expected: 3001, Service: "counter", Payments: []app.POSPayment{{Method: "card", Amount: 3001}}}, admin, "Idempotency-Key", "schedule-pos-blocked")
	if code != 409 || !bytes.Contains(raw, []byte("product_unavailable")) {
		t.Fatalf("closed checkout %d %s", code, raw)
	}
	var combo int64
	must(a.DB.QueryRow(ctx, "INSERT INTO combos(store_id,name,price_cents,pricing_policy,active) VALUES($1,'Combo fechado',3001,'fixed',true) RETURNING id", store).Scan(&combo))
	_, e = a.DB.Exec(ctx, "INSERT INTO combo_items(store_id,combo_id,product_id,quantity) VALUES($1,$2,$3,1)", store, combo, product)
	must(e)
	if _, e = a.CreateOrder(ctx, store, user, "schedule-combo-blocked", app.OrderRequest{Items: []app.Item{{ComboID: combo, Quantity: 1}}}); e == nil || e.Error() != "product_unavailable" {
		t.Fatalf("closed combo %v", e)
	}
	code, raw, _ = call("POST", "/api/dining-tabs", app.DiningRequest{Action: "open", Label: "Horarios QA"}, admin, "Idempotency-Key", "schedule-tab-open")
	if code != 200 && code != 201 {
		t.Fatal(string(raw))
	}
	var tab map[string]any
	must(json.Unmarshal(raw, &tab))
	code, raw, _ = call("POST", "/api/dining-tabs", app.DiningRequest{Action: "add", ID: int64(tab["id"].(float64)), Version: 1, Expected: 3001, Items: []app.Item{item}}, admin, "Idempotency-Key", "schedule-tab-blocked")
	if code != 409 {
		t.Fatalf("closed tab round %d %s", code, raw)
	}
	// A concurrent closing and sale must serialize: the sale is either committed first or rejected.
	c = read(0)
	c.Schedule.Enabled = false
	code, raw = save(c)
	if code != 200 {
		t.Fatal(string(raw))
	}
	var concurrentProduct int64
	must(a.DB.QueryRow(ctx, "INSERT INTO products(store_id,name,price_cents,stock_quantity) VALUES($1,'Concorrente',1000,4) RETURNING id", store).Scan(&concurrentProduct))
	c = read(0)
	c.Schedule.Enabled = true
	c.Schedule.Windows = []app.SaleWindow{}
	done := make(chan error, 1)
	go func() {
		_, e := a.CreateOrder(ctx, store, user, "schedule-race", app.OrderRequest{Items: []app.Item{{ProductID: concurrentProduct, Quantity: 1, ConfigurationVersion: 1}}})
		done <- e
	}()
	code, raw = save(c)
	if code != 200 {
		t.Fatal(string(raw))
	}
	race := <-done
	if race != nil && race.Error() != "product_unavailable" {
		t.Fatalf("closing race %v", race)
	}
	var concurrentStock int64
	must(a.DB.QueryRow(ctx, "SELECT stock_quantity FROM products WHERE id=$1", concurrentProduct).Scan(&concurrentStock))
	wantStock := int64(4)
	if race == nil {
		wantStock = 3
	}
	if concurrentStock != wantStock {
		t.Fatal("closing race changed stock incorrectly")
	}
	code, raw, _ = call("POST", "/api/order-state", map[string]any{"id": order, "state": "cancelled"}, admin)
	if code != 200 {
		t.Fatal(string(raw))
	}
	var foreign int64
	must(a.DB.QueryRow(ctx, "INSERT INTO categories(store_id,name) VALUES($1,'Outra') RETURNING id", otherStore).Scan(&foreign))
	code, raw, _ = call("GET", fmt.Sprintf("/api/sale-schedule?category_id=%d", foreign), nil, admin)
	if code != 404 {
		t.Fatalf("foreign category %d %s", code, raw)
	}
	code, raw, _ = call("POST", "/api/users", map[string]any{"email": "schedule-attendant@example.org", "password": "QA-schedule-password", "role": "attendant", "store_id": store}, admin)
	if code != 201 {
		t.Fatal(string(raw))
	}
	_, _, res = call("POST", "/api/login", map[string]string{"email": "schedule-attendant@example.org", "password": "QA-schedule-password"}, nil)
	code, raw, _ = call("POST", "/api/sale-schedule", c, res.Cookies()[0])
	if code != 403 {
		t.Fatalf("restricted %d %s", code, raw)
	}
	path, e := r.Backup(ctx)
	must(e)
	dump, e := os.ReadFile(path)
	must(e)
	must(r.Restore(ctx, dump))
	var enabled bool
	must(a.DB.QueryRow(ctx, "SELECT (sale_schedule->>'enabled')::boolean FROM stores WHERE id=$1", store).Scan(&enabled))
	if !enabled {
		t.Fatal("restore lost schedule")
	}
	must(a.DB.QueryRow(ctx, "SELECT stock_quantity FROM products WHERE id=$1", product).Scan(&stock))
	if stock != 10 {
		t.Fatal("restore changed stock")
	}
}
