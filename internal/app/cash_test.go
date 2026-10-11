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

func TestCashJourney(t *testing.T) {
	bin := os.Getenv("OPENFOOD_TEST_PG_BIN")
	if bin == "" {
		t.Skip("requires real PostgreSQL")
	}
	bin, _ = filepath.Abs(bin)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
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
	order, err := a.CreateOrder(ctx, store, user, app.Token(), app.OrderRequest{Items: []app.Item{{ProductID: product, Quantity: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	mutate := func(path string, input any, key string, want int) int64 {
		code, raw, _ := call("POST", path, input, admin, "Idempotency-Key", key)
		if code != want {

			t.Fatalf("%s want %d got %d: %s", path, want, code, raw)
		}
		var result map[string]int64
		if code == 200 {
			json.Unmarshal(raw, &result)
		}
		return result["id"]
	}
	mutate("/api/manual-receipts", app.ReceiptRequest{OrderID: order, Amount: 100, Method: "cash"}, app.Token(), 409)
	opening := app.CashRequest{Action: "open", Opening: 0, Note: "Saldo QA de abertura"}
	key := app.Token()
	session := mutate("/api/cash-sessions", opening, key, 200)
	if mutate("/api/cash-sessions", opening, key, 200) != session {
		t.Fatal("open retry duplicated")
	}
	mutate("/api/cash-sessions", app.CashRequest{Action: "open", Opening: 100}, key, 409)
	mutate("/api/cash-sessions", opening, app.Token(), 409)
	cookies := map[string]*http.Cookie{}
	for _, role := range []string{"attendant", "finance_viewer", "kitchen"} {
		email := role + "-cash-qa@example.org"
		code, raw, _ = call("POST", "/api/users", map[string]any{"email": email, "password": "QA-cash-test-password", "role": role, "store_id": store}, admin)
		if code != 201 {
			t.Fatalf("user %s", raw)
		}
		code, _, res = call("POST", "/api/login", map[string]string{"email": email, "password": "QA-cash-test-password"}, nil)
		if code != 200 {
			t.Fatal("role login")
		}
		cookies[role] = res.Cookies()[0]
	}
	for _, path := range []string{"/api/cash-sessions", "/api/manual-receipts"} {
		method := "GET"
		var input any
		if path == "/api/manual-receipts" {
			method = "POST"
			input = app.ReceiptRequest{OrderID: order, Amount: 100, Method: "cash"}
		}
		code, _, _ = call(method, path, input, cookies["attendant"], "X-Store-ID", fmt.Sprint(otherStore), "Idempotency-Key", app.Token())
		if code != 403 {
			t.Fatalf("cross store %s: %d", path, code)
		}
	}
	code, _, _ = call("GET", "/api/cash-sessions", nil, cookies["kitchen"])
	if code != 403 {
		t.Fatal("kitchen cash access")
	}
	code, _, _ = call("GET", "/api/cash-sessions", nil, cookies["finance_viewer"])
	if code != 200 {
		t.Fatal("finance read")
	}
	code, _, _ = call("POST", "/api/manual-receipts", app.ReceiptRequest{OrderID: order, Amount: 100, Method: "cash"}, cookies["finance_viewer"], "Idempotency-Key", app.Token())
	if code != 403 {
		t.Fatal("finance mutated")
	}
	code, _, _ = call("POST", "/api/cash-sessions", opening, cookies["attendant"], "Idempotency-Key", app.Token())
	if code != 409 {
		t.Fatal("attendant must not open a second cash session")
	}
	// Two concurrent distinct partial receipts cannot exceed the order balance.
	type result struct {
		code int
		body []byte
		key  string
	}
	out := make(chan result, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := app.Token()
			code, raw, _ := call("POST", "/api/manual-receipts", app.ReceiptRequest{OrderID: order, Amount: 2500, Method: "cash"}, cookies["attendant"], "Idempotency-Key", key)
			out <- result{code, raw, key}
		}()
	}
	wg.Wait()
	close(out)
	var receiptKey string
	var cashReceipt int64
	success := 0
	for result := range out {
		if result.code == 200 {
			success++
			receiptKey = result.key
			var payload map[string]int64
			json.Unmarshal(result.body, &payload)
			cashReceipt = payload["id"]
		} else if result.code != 409 {
			t.Fatalf("concurrent receipt %d %s", result.code, result.body)
		}
	}
	if success != 1 {
		t.Fatal("concurrent overpayment accepted")
	}
	if mutate("/api/manual-receipts", app.ReceiptRequest{OrderID: order, Amount: 2500, Method: "cash"}, receiptKey, 200) != cashReceipt {
		t.Fatal("duplicate receipt")
	}
	mutate("/api/manual-receipts", app.ReceiptRequest{OrderID: order, Amount: 2501, Method: "cash"}, receiptKey, 409)
	pixReceipt := mutate("/api/manual-receipts", app.ReceiptRequest{OrderID: order, Amount: 1498, Method: "pix_manual"}, app.Token(), 200)
	var financial string
	if err = a.DB.QueryRow(ctx, "SELECT financial_state FROM orders WHERE id=$1", order).Scan(&financial); err != nil || financial != "manual_confirmed" {
		t.Fatal("paid state")
	}
	code, _, _ = call("POST", "/api/order-state", map[string]any{"id": order, "state": "cancelled"}, admin)
	if code != 409 {
		t.Fatal("paid order cancelled")
	}
	mutate("/api/cash-sessions", app.CashRequest{Action: "close", ID: session, Counted: 2500, Expected: 0}, app.Token(), 409)
	mutate("/api/cash-sessions", app.CashRequest{Action: "close", ID: session, Counted: 2400, Expected: 2500}, app.Token(), 409)
	closeKey := app.Token()
	closing := app.CashRequest{Action: "close", ID: session, Counted: 2400, Expected: 2500, Note: "Diferença QA identificada"}
	mutate("/api/cash-sessions", closing, closeKey, 200)
	mutate("/api/cash-sessions", closing, closeKey, 200)
	if mutate("/api/manual-receipts", app.ReceiptRequest{OrderID: order, Amount: 2500, Method: "cash"}, receiptKey, 200) != cashReceipt {
		t.Fatal("receipt retry after cash closed")
	}
	mutate("/api/manual-receipts", app.ReceiptRequest{OrderID: order, Amount: 1, Method: "cash"}, app.Token(), 409)
	mutate("/api/cash-sessions", app.CashRequest{Action: "open", Opening: 0}, app.Token(), 200)
	mutate("/api/manual-receipts", app.ReceiptRequest{RefundOf: cashReceipt, Note: "Devolução QA"}, app.Token(), 409)
	// Close empty cash and open funded cash: refund of an older period goes into the new session.
	var empty int64
	if err = a.DB.QueryRow(ctx, "SELECT id FROM cash_sessions WHERE store_id=$1 AND closed_at IS NULL", store).Scan(&empty); err != nil {
		t.Fatal(err)
	}
	mutate("/api/cash-sessions", app.CashRequest{Action: "close", ID: empty}, app.Token(), 200)
	next := mutate("/api/cash-sessions", app.CashRequest{Action: "open", Opening: 5000}, app.Token(), 200)
	code, _, _ = call("POST", "/api/manual-receipts", app.ReceiptRequest{RefundOf: cashReceipt, Note: "Devolução QA"}, cookies["attendant"], "Idempotency-Key", app.Token())
	if code != 403 {
		t.Fatal("attendant refunded")
	}
	refundKey := app.Token()
	refund := app.ReceiptRequest{RefundOf: cashReceipt, Note: "Devolução QA conferida"}
	refundID := mutate("/api/manual-receipts", refund, refundKey, 200)
	if mutate("/api/manual-receipts", refund, refundKey, 200) != refundID {
		t.Fatal("refund retry")
	}
	mutate("/api/manual-receipts", refund, app.Token(), 409)
	mutate("/api/manual-receipts", app.ReceiptRequest{RefundOf: pixReceipt, Note: "Pix devolvido QA"}, app.Token(), 200)
	code, _, _ = call("POST", "/api/order-state", map[string]any{"id": order, "state": "cancelled"}, admin)
	if code != 200 {
		t.Fatal("cancel after refund")
	}
	mutate("/api/manual-receipts", app.ReceiptRequest{OrderID: order, Amount: 1, Method: "card"}, app.Token(), 409)
	code, raw, _ = call("GET", "/api/cash-sessions", nil, admin)
	if code != 200 {
		t.Fatalf("overview %s", raw)
	}
	var overview struct {
		Sessions []app.CashSessionView `json:"sessions"`
		Receipts []map[string]any      `json:"receipts"`
	}
	if json.Unmarshal(raw, &overview) != nil {
		t.Fatal("decode overview")
	}
	if overview.Sessions[2].OpeningNote != "Saldo QA de abertura" {
		t.Fatal("opening note lost during close")
	}
	if len(overview.Sessions) != 3 || overview.Sessions[0].ID != next || overview.Sessions[0].Cash != -2500 || overview.Sessions[0].Pix != -1498 || overview.Sessions[2].Cash != 2500 || *overview.Sessions[2].Difference != -100 {
		t.Fatalf("cash reconciliation %s", raw)
	}
	integratedOrder, e := a.CreateOrder(ctx, store, user, app.Token(), app.OrderRequest{Items: []app.Item{{ProductID: product, Quantity: 1}}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = a.DB.Exec(ctx, "INSERT INTO payments(store_id,order_id,provider,external_id,amount_cents,idempotency_key) VALUES($1,$2,'qa','qa-future-provider',1999,'qa-future-provider')", store, integratedOrder); e != nil {
		t.Fatal(e)
	}
	if _, e = a.DB.Exec(ctx, "UPDATE orders SET financial_state='refund_partial' WHERE id=$1", integratedOrder); e != nil {
		t.Fatal(e)
	}
	mutate("/api/manual-receipts", app.ReceiptRequest{OrderID: integratedOrder, Amount: 100, Method: "cash"}, app.Token(), 409)
	checkout := app.CheckoutRequest{Items: []app.Item{{ProductID: product, Quantity: 2}}, Expected: 3900, Discount: 98, Note: "Desconto QA", Service: "kitchen", Payments: []app.POSPayment{{Method: "cash", Amount: 1000, Tendered: 2000}, {Method: "card", Amount: 2900}}}
	posKey := app.Token()
	posOrder := mutate("/api/pos-checkout", checkout, posKey, 200)
	if mutate("/api/pos-checkout", checkout, posKey, 200) != posOrder {
		t.Fatal("POS duplicated")
	}
	duplicateResults := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() {
			id, err := a.Checkout(ctx, store, user, posKey, checkout)
			if err == nil && id != posOrder {
				err = fmt.Errorf("duplicate returned %d", id)
			}
			duplicateResults <- err
		}()
	}
	for i := 0; i < 4; i++ {
		if err := <-duplicateResults; err != nil {
			t.Fatal(err)
		}
	}
	conflict := checkout
	conflict.Expected--
	mutate("/api/pos-checkout", conflict, posKey, 409)
	var state string
	var total int64
	if e = a.DB.QueryRow(ctx, "SELECT state,financial_state,total_cents FROM orders WHERE id=$1", posOrder).Scan(&state, &financial, &total); e != nil || state != "confirmed" || financial != "manual_confirmed" || total != 3900 {
		t.Fatalf("POS state %s %s %d %v", state, financial, total, e)
	}
	code, raw, _ = call("GET", fmt.Sprintf("/api/pos-receipt?id=%d", posOrder), nil, admin)
	if code != 200 || !bytes.Contains(raw, []byte(`"tendered_cents": 2000`)) && !bytes.Contains(raw, []byte(`"tendered_cents":2000`)) {
		t.Fatalf("receipt %d %s", code, raw)
	}
	var before, after float64
	a.DB.QueryRow(ctx, "SELECT stock_quantity FROM products WHERE id=$1", product).Scan(&before)
	bad := checkout
	bad.Expected = 3800
	mutate("/api/pos-checkout", bad, app.Token(), 409)
	bad = checkout
	bad.Payments = []app.POSPayment{{Method: "cash", Amount: 3900, Tendered: 3899}}
	mutate("/api/pos-checkout", bad, app.Token(), 409)
	bad = checkout
	bad.Items = []app.Item{{ProductID: product, Quantity: 10000}}
	mutate("/api/pos-checkout", bad, app.Token(), 409)
	a.DB.QueryRow(ctx, "SELECT stock_quantity FROM products WHERE id=$1", product).Scan(&after)
	if before != after {
		t.Fatal("failed checkout changed stock")
	}
	code, _, _ = call("POST", "/api/pos-checkout", checkout, cookies["attendant"], "Idempotency-Key", app.Token())
	if code != 403 {
		t.Fatal("attendant discounted")
	}
	code, _, _ = call("POST", "/api/pos-checkout", checkout, cookies["finance_viewer"], "Idempotency-Key", app.Token())
	if code != 403 {
		t.Fatal("finance sold")
	}
	mutate("/api/cash-sessions", app.CashRequest{Action: "close", ID: next, Counted: 3500, Expected: 3500}, app.Token(), 200)
	if mutate("/api/pos-checkout", checkout, posKey, 200) != posOrder {
		t.Fatal("retry after close")
	}
	mutate("/api/pos-checkout", checkout, app.Token(), 409)
	code, raw, _ = call("POST", "/api/cash-sessions", app.CashRequest{Action: "open", Opening: 1200}, cookies["attendant"], "Idempotency-Key", app.Token())
	if code != 200 {
		t.Fatalf("attendant opening %d %s", code, raw)
	}
	var opened struct {
		ID int64 `json:"id"`
	}
	json.Unmarshal(raw, &opened)
	adjust := app.AdjustmentRequest{Session: opened.ID, Amount: -500, Expected: 1200, Note: "Sangria QA"}
	adjustKey := app.Token()
	mutate("/api/cash-adjustments", adjust, adjustKey, 200)
	mutate("/api/cash-adjustments", adjust, adjustKey, 200)
	adjust.Amount = -800
	adjust.Expected = 700
	mutate("/api/cash-adjustments", adjust, app.Token(), 409)
	code, raw, _ = call("POST", "/api/cash-sessions", app.CashRequest{Action: "close", ID: opened.ID, Counted: 700, Expected: 700}, cookies["attendant"], "Idempotency-Key", app.Token())
	if code != 200 {
		t.Fatalf("attendant own close %d %s", code, raw)
	}
	code, _, _ = call("POST", "/api/cash-sessions", app.CashRequest{Action: "close", ID: next}, cookies["attendant"], "Idempotency-Key", app.Token())
	if code != 403 {
		t.Fatal("attendant closed manager cash")
	}
	backup, e := runtime.Backup(ctx)
	if e != nil {
		t.Fatal(e)
	}
	dump, e := os.ReadFile(backup)
	if e != nil {
		t.Fatal(e)
	}
	if e = runtime.Restore(ctx, dump); e != nil {
		t.Fatal(e)
	}
	var count int
	if e = a.DB.QueryRow(ctx, "SELECT count(*) FROM manual_receipts WHERE store_id=$1", store).Scan(&count); e != nil || count != 6 {
		t.Fatalf("ledger restore %d %v", count, e)
	}
	t.Log("PASS: receipts, partial/split payment, idempotency/concurrent ceiling, refunds across periods, cash reconciliation, permissions, cancellation and backup")
}
