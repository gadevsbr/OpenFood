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

func TestCustomerPrivacyJourney(t *testing.T) {
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
	var customer, other int64
	must(a.DB.QueryRow(ctx, "INSERT INTO customers(store_id,name,email,notes) VALUES($1,'Privado QA','private-qa@example.org','Texto privado QA') RETURNING id", store).Scan(&customer))
	must(a.DB.QueryRow(ctx, "INSERT INTO customers(store_id,name,email) VALUES($1,'Outra pessoa','other-qa@example.org') RETURNING id", store).Scan(&other))
	code, raw, _ = call("POST", "/api/cash-sessions", map[string]any{"action": "open", "opening_cents": 0}, admin, "Idempotency-Key", "privacy-cash-open")
	if code != 200 && code != 201 {
		t.Fatal(string(raw))
	}
	sale := app.CheckoutRequest{CustomerID: customer, Items: []app.Item{{ProductID: product, Quantity: 1, Notes: "private-qa@example.org"}}, Expected: 2901, Discount: 100, Note: "private-qa@example.org", Service: "counter", Payments: []app.POSPayment{{Method: "card", Amount: 2901}}}
	order, e := a.Checkout(ctx, store, user, "privacy-sale", sale)
	must(e)
	_, e = a.DB.Exec(ctx, "UPDATE manual_receipts SET note='private-qa@example.org' WHERE order_id=$1", order)
	must(e)
	code, raw, _ = call("POST", "/api/consents", map[string]any{"customer_id": customer, "purpose": "contact", "version": "qa-1", "granted": true, "source": "qa", "terms_text": "private-qa@example.org", "evidence": map[string]string{"contact": "private-qa@example.org"}}, admin)
	if code != 200 {
		t.Fatal(string(raw))
	}
	code, raw, _ = call("POST", "/api/dining-tabs", app.DiningRequest{Action: "open", Label: "private-qa@example.org"}, admin, "Idempotency-Key", "privacy-tab-open")
	if code != 200 && code != 201 {
		t.Fatal(string(raw))
	}
	var response map[string]any
	must(json.Unmarshal(raw, &response))
	tab := int64(response["id"].(float64))
	code, raw, _ = call("POST", "/api/dining-tabs", app.DiningRequest{Action: "add", ID: tab, Version: 1, CustomerID: customer, Expected: 3001, Items: []app.Item{{ProductID: product, Quantity: 1, Notes: "private-qa@example.org"}}}, admin, "Idempotency-Key", "privacy-tab-round")
	if code != 200 && code != 201 {
		t.Fatal(string(raw))
	}
	code, raw, _ = call("POST", "/api/dining-tabs", app.DiningRequest{Action: "add", ID: tab, Version: 2, CustomerID: other, Expected: 3001, Items: []app.Item{{ProductID: product, Quantity: 1, Notes: "Texto de outra pessoa"}}}, admin, "Idempotency-Key", "privacy-other-round")
	if code != 200 && code != 201 {
		t.Fatal(string(raw))
	}
	endpoint := fmt.Sprintf("/api/customer-privacy-review?customer_id=%d", customer)
	review := func() app.PrivacyReview {
		code, b, res := call("GET", endpoint, nil, admin)
		if code != 200 || res.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("review %d %s", code, b)
		}
		var c app.PrivacyReview
		must(json.Unmarshal(b, &c))
		return c
	}
	c := review()
	if c.Counts["orders"] != 2 || c.Counts["checkouts"] != 1 || c.Counts["tabs"] != 1 || c.Version == "" {
		t.Fatalf("counts %+v", c)
	}
	export := func() []byte {
		code, b, _ := call("GET", fmt.Sprintf("/api/customer-export?customer_id=%d", customer), nil, admin)
		if code != 200 {
			t.Fatal(string(b))
		}
		return b
	}
	var exported map[string]json.RawMessage
	must(json.Unmarshal(export(), &exported))
	if !bytes.Contains(exported["checkouts"], []byte("private-qa@example.org")) || !bytes.Contains(exported["receipts"], []byte("private-qa@example.org")) || !bytes.Contains(exported["tabs"], []byte("private-qa@example.org")) {
		t.Fatal("export missed linked free text")
	}
	_, e = a.DB.Exec(ctx, "UPDATE customers SET notes='Alterado QA' WHERE id=$1", customer)
	must(e)
	remove := fmt.Sprintf("/api/customer-delete?customer_id=%d", customer)
	code, raw, _ = call("DELETE", remove, nil, admin, "X-Privacy-Review", c.Version)
	if code != 409 {
		t.Fatalf("stale review %d %s", code, raw)
	}
	// A failed cleanup rolls back all earlier updates, including identity and terms.
	_, e = a.DB.Exec(ctx, "CREATE FUNCTION qa_refuse_privacy() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'qa'; END $$; CREATE TRIGGER qa_privacy_failure BEFORE UPDATE ON pos_checkouts FOR EACH ROW EXECUTE FUNCTION qa_refuse_privacy()")
	must(e)
	c = review()
	beforeFailure := export()
	code, raw, _ = call("DELETE", remove, nil, admin, "X-Privacy-Review", c.Version)
	if code != 500 {
		t.Fatalf("failure fixture %d %s", code, raw)
	}
	if !bytes.Equal(export(), beforeFailure) {
		t.Fatal("partial cleanup committed")
	}
	_, e = a.DB.Exec(ctx, "DROP TRIGGER qa_privacy_failure ON pos_checkouts; DROP FUNCTION qa_refuse_privacy()")
	must(e)
	c = review()
	code, raw, _ = call("DELETE", remove, nil, admin, "X-Privacy-Review", c.Version)
	if code != 200 {
		t.Fatalf("remove %d %s", code, raw)
	}
	b := export()
	if bytes.Contains(b, []byte("private-qa@example.org")) || !bytes.Contains(b, []byte("2901")) || !bytes.Contains(b, []byte("3001")) {
		t.Fatalf("cleanup/export %s", b)
	}
	var label, note string
	must(a.DB.QueryRow(ctx, "SELECT label FROM dining_tabs WHERE id=$1", tab).Scan(&label))
	if label == "private-qa@example.org" {
		t.Fatal("tab label retained")
	}
	must(a.DB.QueryRow(ctx, "SELECT i.notes FROM order_items i JOIN orders o ON o.id=i.order_id WHERE o.customer_id=$1", other).Scan(&note))
	if note != "Texto de outra pessoa" {
		t.Fatal("other customer text lost")
	}
	if replay, e := a.Checkout(ctx, store, user, "privacy-sale", sale); e != nil || replay != order {
		t.Fatalf("sale retry %v", e)
	}
	var receipt int64
	must(a.DB.QueryRow(ctx, "SELECT id FROM manual_receipts WHERE order_id=$1", order).Scan(&receipt))
	refund, e := a.RecordReceipt(ctx, store, user, "privacy-refund-after", app.ReceiptRequest{RefundOf: receipt, Note: "private-qa@example.org"})
	must(e)
	must(a.DB.QueryRow(ctx, "SELECT note FROM manual_receipts WHERE id=$1", refund).Scan(&note))
	if note == "private-qa@example.org" || note == "" {
		t.Fatal("refund reintroduced text or lost required marker")
	}
	if retry, e := a.RecordReceipt(ctx, store, user, "privacy-refund-after", app.ReceiptRequest{RefundOf: receipt, Note: "private-qa@example.org"}); e != nil || retry != refund {
		t.Fatalf("refund retry %v", e)
	}
	// Both identifiers and role boundaries remain store-scoped.
	code, raw, _ = call("GET", endpoint, nil, admin, "X-Store-ID", fmt.Sprint(otherStore))
	if code != 404 {
		t.Fatalf("foreign customer %d %s", code, raw)
	}
	for _, role := range []string{"attendant", "finance_viewer"} {
		email := "privacy-" + role + "@example.org"
		_, e = a.DB.Exec(ctx, "INSERT INTO users(org_id,store_id,email,password_hash,role) SELECT org_id,store_id,$2,password_hash,$3 FROM users WHERE id=$1", user, email, role)
		must(e)
		_, _, res = call("POST", "/api/login", map[string]string{"email": email, "password": "QA-pizza-test-password"}, nil)
		code, raw, _ = call("GET", endpoint, nil, res.Cookies()[0])
		if code != 403 {
			t.Fatalf("restricted review %s %d", role, code)
		}
	}
	path, e := r.Backup(ctx)
	must(e)
	dump, e := os.ReadFile(path)
	must(e)
	must(r.Restore(ctx, dump))
	var personal string
	must(a.DB.QueryRow(ctx, "SELECT COALESCE(email,'') FROM customers WHERE id=$1", customer).Scan(&personal))
	if personal != "" {
		t.Fatal("new backup lost removal")
	}
	must(a.DB.QueryRow(ctx, "SELECT discount_cents FROM pos_checkouts WHERE order_id=$1", order).Scan(&user))
	if user != 100 {
		t.Fatal("financial discount changed")
	}
}
