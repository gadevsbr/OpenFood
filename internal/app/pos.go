package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

func migratePOS(ctx context.Context, tx pgx.Tx) error {
	if _, e := tx.Exec(ctx, posSchema); e != nil {
		return fmt.Errorf("migration_v4_to_v5: %w", e)
	}
	_, e := tx.Exec(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES(5,$1)", Hash(schema+cashSchema+posSchema))
	return e
}

type POSPayment struct {
	Method   string `json:"method"`
	Amount   int64  `json:"amount_cents"`
	Tendered int64  `json:"tendered_cents"`
}
type CheckoutRequest struct {
	Items    []Item       `json:"items"`
	Payments []POSPayment `json:"payments"`
	Expected int64        `json:"expected_total_cents"`
	Discount int64        `json:"discount_cents"`
	Note     string       `json:"discount_note"`
	Service  string       `json:"service"`
}

func (a *App) posCheckout(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanAccessStore(store) || !user.CanCreateOrders() {
		failure(w, 403, "forbidden_pos")
		return
	}
	if r.Method != "POST" {
		failure(w, 405, "method_not_allowed")
		return
	}
	var input CheckoutRequest
	if decode(w, r, &input) != nil || !moneyKey(r.Header.Get("Idempotency-Key")) || len(input.Items) < 1 || len(input.Items) > 100 || len(input.Payments) < 1 || len(input.Payments) > 10 || input.Discount < 0 || input.Discount > 1000000000000 || len(input.Note) > 1000 || input.Expected < 0 || input.Expected > 1000000000000 || (input.Service != "counter" && input.Service != "kitchen") {
		failure(w, 400, "checkout_invalid")
		return
	}
	if input.Discount > 0 && (!user.CanManageCatalog() || strings.TrimSpace(input.Note) == "") {
		failure(w, 403, "discount_manager_required")
		return
	}
	id, e := a.Checkout(r.Context(), store, user.ID, r.Header.Get("Idempotency-Key"), input)
	if e != nil {
		known := map[string]bool{"cash_not_open": true, "checkout_price_changed": true, "payment_total_invalid": true, "checkout_invalid": true, "discount_invalid": true, "stock_insufficient": true, "product_unavailable": true, "quantity_invalid": true, "idempotency_conflict": true, "order_duplicate_product": true, "configuration_invalid": true, "configuration_changed": true, "variant_required": true, "addon_unavailable": true, "addon_limits_invalid": true}
		if known[e.Error()] {
			failure(w, 409, e.Error())
		} else {
			failure(w, 500, "checkout_failed")
		}
		return
	}
	jsonResponse(w, 200, map[string]int64{"id": id})
}
func (a *App) Checkout(ctx context.Context, store, user int64, key string, input CheckoutRequest) (int64, error) {
	if !moneyKey(key) || len(input.Items) < 1 || len(input.Items) > 100 || len(input.Payments) < 1 || len(input.Payments) > 10 || input.Discount < 0 || input.Discount > 1000000000000 || input.Expected < 0 || input.Expected > 1000000000000 || len(input.Note) > 1000 || (input.Discount > 0 && strings.TrimSpace(input.Note) == "") || (input.Service != "counter" && input.Service != "kitchen") {
		return 0, errors.New("checkout_invalid")
	}
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback(ctx)
	if e = cashLock(ctx, tx, store); e != nil {
		return 0, e
	}
	digest := moneyDigest(input)
	var id int64
	var previous string
	e = tx.QueryRow(ctx, "SELECT order_id,request_hash FROM pos_checkouts WHERE store_id=$1 AND idempotency_key=$2", store, key).Scan(&id, &previous)
	if e == nil {
		if previous != digest {
			return 0, errors.New("idempotency_conflict")
		}
		return id, tx.Commit(ctx)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return 0, e
	}
	var session int64
	if e = tx.QueryRow(ctx, "SELECT id FROM cash_sessions WHERE store_id=$1 AND closed_at IS NULL FOR UPDATE", store).Scan(&session); e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return 0, errors.New("cash_not_open")
		}
		return 0, e
	}
	// Order, stock and all receipts share one transaction. No partial sale survives a failure.
	id, e = a.createOrderTx(ctx, tx, store, user, "pos:"+Hash(key), OrderRequest{Items: input.Items})
	if e != nil {
		return 0, e
	}
	var subtotal int64
	if e = tx.QueryRow(ctx, "SELECT total_cents FROM orders WHERE store_id=$1 AND id=$2", store, id).Scan(&subtotal); e != nil {
		return 0, e
	}
	if input.Discount > subtotal {
		return 0, errors.New("discount_invalid")
	}
	total := subtotal - input.Discount
	if total != input.Expected {
		return 0, errors.New("checkout_price_changed")
	}
	// Free sales are not silently treated as payments.
	if total < 1 {
		return 0, errors.New("discount_invalid")
	}
	var paid int64
	for _, p := range input.Payments {
		if p.Amount < 1 || p.Amount > 1000000000000 || (p.Method != "cash" && p.Method != "card" && p.Method != "pix_manual") || p.Tendered < 0 || p.Tendered > 1000000000000 || (p.Method == "cash" && p.Tendered < p.Amount) || (p.Method != "cash" && p.Tendered != 0) {
			return 0, errors.New("payment_total_invalid")
		}
		paid += p.Amount
	}
	if paid != total {
		return 0, errors.New("payment_total_invalid")
	}
	state := "completed"
	if input.Service == "kitchen" {
		state = "confirmed"
	}
	if _, e = tx.Exec(ctx, "UPDATE orders SET total_cents=$3,state=$4 WHERE store_id=$1 AND id=$2", store, id, total, state); e != nil {
		return 0, e
	}
	for i, p := range input.Payments {
		_, e = a.recordReceiptTx(ctx, tx, store, user, "pos:"+Hash(key)+fmt.Sprintf(":%d", i), ReceiptRequest{OrderID: id, Amount: p.Amount, Method: p.Method, Note: "Recebimento no PDV"})
		if e != nil {
			return 0, e
		}
	}
	raw, _ := json.Marshal(input.Payments)
	if _, e = tx.Exec(ctx, "INSERT INTO pos_checkouts(store_id,order_id,user_id,session_id,idempotency_key,request_hash,discount_cents,discount_note,service,payments) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)", store, id, user, session, key, digest, input.Discount, input.Note, input.Service, raw); e != nil {
		return 0, e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO audit(store_id,user_id,action,entity_id) VALUES($1,$2,'pos_checkout',$3)", store, user, id); e != nil {
		return 0, e
	}
	return id, tx.Commit(ctx)
}
func (a *App) posReceipt(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanAccessStore(store) || !user.CanReadCash() {
		failure(w, 403, "forbidden_pos")
		return
	}
	if r.Method != "GET" {
		failure(w, 405, "method_not_allowed")
		return
	}
	id, e := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if e != nil || id < 1 {
		failure(w, 400, "order_invalid")
		return
	}
	var storeName, email, at, service, note, state, financial string
	var total, discount int64
	var payments json.RawMessage
	e = a.DB.QueryRow(r.Context(), `SELECT s.name,u.email,p.created_at::text,p.service,p.discount_note,o.state,o.financial_state,o.total_cents,p.discount_cents,p.payments FROM pos_checkouts p JOIN orders o ON o.id=p.order_id AND o.store_id=p.store_id JOIN stores s ON s.id=p.store_id JOIN users u ON u.id=p.user_id WHERE p.store_id=$1 AND p.order_id=$2`, store, id).Scan(&storeName, &email, &at, &service, &note, &state, &financial, &total, &discount, &payments)
	if e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			failure(w, 404, "order_not_found")
		} else {
			failure(w, 500, "receipt_failed")
		}
		return
	}
	rows, e := a.DB.Query(r.Context(), "SELECT name,price_cents,quantity,unit,COALESCE(notes,''),selection FROM order_items WHERE store_id=$1 AND order_id=$2 ORDER BY id", store, id)
	if e != nil {
		failure(w, 500, "receipt_failed")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var name, unit, notes string
		var selection json.RawMessage
		var price int64
		var quantity float64
		if rows.Scan(&name, &price, &quantity, &unit, &notes, &selection) != nil {
			failure(w, 500, "receipt_failed")
			return
		}
		items = append(items, map[string]any{"name": name, "price_cents": price, "quantity": quantity, "unit": unit, "notes": notes, "selection": selection})
	}
	if rows.Err() != nil {
		failure(w, 500, "receipt_failed")
		return
	}
	jsonResponse(w, 200, map[string]any{"id": id, "store": storeName, "operator": email, "created_at": at, "service": service, "state": state, "financial_state": financial, "total_cents": total, "discount_cents": discount, "discount_note": note, "payments": payments, "items": items, "fiscal": false})
}
func (a *App) posSummary(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanAccessStore(store) {
		failure(w, 403, "forbidden_pos")
		return
	}
	if r.Method != "GET" {
		failure(w, 405, "method_not_allowed")
		return
	}
	ctx := r.Context()
	var sales, receipts, pending int64
	var active, products int
	if e := a.DB.QueryRow(ctx, `SELECT COALESCE(sum(total_cents) FILTER(WHERE state<>'cancelled' AND (created_at AT TIME ZONE 'America/Sao_Paulo')::date=(now() AT TIME ZONE 'America/Sao_Paulo')::date),0),count(*) FILTER(WHERE state IN ('confirmed','preparing','ready')) FROM orders WHERE store_id=$1`, store).Scan(&sales, &active); e != nil {
		failure(w, 500, "summary_failed")
		return
	}
	if e := a.DB.QueryRow(ctx, `SELECT COALESCE(sum(amount_cents),0) FROM manual_receipts WHERE store_id=$1 AND (created_at AT TIME ZONE 'America/Sao_Paulo')::date=(now() AT TIME ZONE 'America/Sao_Paulo')::date`, store).Scan(&receipts); e != nil {
		failure(w, 500, "summary_failed")
		return
	}
	if e := a.DB.QueryRow(ctx, `SELECT COALESCE(sum(GREATEST(o.total_cents-COALESCE((SELECT sum(m.amount_cents) FROM manual_receipts m WHERE m.store_id=o.store_id AND m.order_id=o.id),0),0)),0) FROM orders o WHERE o.store_id=$1 AND o.state<>'cancelled' AND o.financial_state IN ('manual_pending','manual_confirmed','refunded','refund_partial') AND NOT EXISTS(SELECT 1 FROM payments p WHERE p.store_id=o.store_id AND p.order_id=o.id)`, store).Scan(&pending); e != nil {
		failure(w, 500, "summary_failed")
		return
	}
	if e := a.DB.QueryRow(ctx, "SELECT count(*) FROM products WHERE store_id=$1 AND active=true", store).Scan(&products); e != nil {
		failure(w, 500, "summary_failed")
		return
	}
	rows, e := a.DB.Query(ctx, "SELECT id,name FROM categories WHERE store_id=$1 AND active=true ORDER BY display_order,id", store)
	if e != nil {
		failure(w, 500, "summary_failed")
		return
	}
	categories := []map[string]any{}
	for rows.Next() {
		var id int64
		var name string
		if e = rows.Scan(&id, &name); e != nil {
			rows.Close()
			failure(w, 500, "summary_failed")
			return
		}
		categories = append(categories, map[string]any{"id": id, "name": name})
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		failure(w, 500, "summary_failed")
		return
	}
	recent := []map[string]any{}
	rows, e = a.DB.Query(ctx, `SELECT p.order_id,o.total_cents,p.created_at::text,o.state,o.financial_state FROM pos_checkouts p JOIN orders o ON o.store_id=p.store_id AND o.id=p.order_id WHERE p.store_id=$1 ORDER BY p.id DESC LIMIT 50`, store)
	if e != nil {
		failure(w, 500, "summary_failed")
		return
	}
	for rows.Next() {
		var id, total int64
		var at, state, financial string
		if e = rows.Scan(&id, &total, &at, &state, &financial); e != nil {
			rows.Close()
			failure(w, 500, "summary_failed")
			return
		}
		recent = append(recent, map[string]any{"id": id, "total_cents": total, "created_at": at, "state": state, "financial_state": financial})
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		failure(w, 500, "summary_failed")
		return
	}
	result := map[string]any{"categories": categories, "recent": recent, "active_orders": active, "active_products": products}
	if !user.CanReadCash() {
		result["recent"] = []map[string]any{}
	}
	if user.CanReadCash() {
		result["sales_today_cents"] = sales
		result["received_today_cents"] = receipts
		result["pending_cents"] = pending
	}
	jsonResponse(w, 200, result)
}

type AdjustmentRequest struct {
	Session  int64  `json:"session_id"`
	Amount   int64  `json:"amount_cents"`
	Note     string `json:"note"`
	Expected int64  `json:"expected_cents"`
}

func (a *App) cashAdjustments(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanAccessStore(store) || !user.CanReadCash() {
		failure(w, 403, "forbidden_cash_management")
		return
	}
	if r.Method == "GET" {
		rows, err := a.DB.Query(r.Context(), `SELECT c.id,c.session_id,c.amount_cents,c.note,c.created_at::text,u.email FROM cash_adjustments c JOIN users u ON u.id=c.user_id WHERE c.store_id=$1 ORDER BY c.id DESC LIMIT 200`, store)
		if err != nil {
			failure(w, 500, "cash_operation_failed")
			return
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var id, session, amount int64
			var note, at, operator string
			if rows.Scan(&id, &session, &amount, &note, &at, &operator) != nil {
				failure(w, 500, "cash_operation_failed")
				return
			}
			items = append(items, map[string]any{"id": id, "session_id": session, "amount_cents": amount, "note": note, "created_at": at, "operator": operator})
		}
		if rows.Err() != nil {
			failure(w, 500, "cash_operation_failed")
			return
		}
		jsonResponse(w, 200, items)
		return
	}
	if !user.CanReceiveManual() {
		failure(w, 403, "forbidden_cash_management")
		return
	}
	if r.Method != "POST" {
		failure(w, 405, "method_not_allowed")
		return
	}
	var input AdjustmentRequest
	if decode(w, r, &input) != nil || !moneyKey(r.Header.Get("Idempotency-Key")) || input.Amount == 0 || input.Amount < -1000000000000 || input.Amount > 1000000000000 || input.Session < 1 || strings.TrimSpace(input.Note) == "" || len(input.Note) > 1000 {
		failure(w, 400, "adjustment_invalid")
		return
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		failure(w, 500, "cash_operation_failed")
		return
	}
	defer tx.Rollback(r.Context())
	if e = cashLock(r.Context(), tx, store); e != nil {
		failure(w, 500, "cash_operation_failed")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	digest := moneyDigest(input)
	var id int64
	var old string
	e = tx.QueryRow(r.Context(), "SELECT id,request_hash FROM cash_adjustments WHERE store_id=$1 AND idempotency_key=$2", store, key).Scan(&id, &old)
	if e == nil {
		if old != digest {
			failure(w, 409, "idempotency_conflict")
			return
		}
		moneyResponse(w, id, tx.Commit(r.Context()))
		return
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		failure(w, 500, "cash_operation_failed")
		return
	}
	var opening, owner, net int64
	if e = tx.QueryRow(r.Context(), "SELECT opening_cents,opened_by FROM cash_sessions WHERE store_id=$1 AND id=$2 AND closed_at IS NULL FOR UPDATE", store, input.Session).Scan(&opening, &owner); e != nil {
		failure(w, 409, "cash_not_open")
		return
	}
	if !user.CanManageCatalog() && owner != user.ID {
		failure(w, 403, "forbidden_cash_management")
		return
	}
	if e = tx.QueryRow(r.Context(), `SELECT COALESCE((SELECT sum(amount_cents) FROM manual_receipts WHERE store_id=$1 AND session_id=$2 AND method='cash'),0)+COALESCE((SELECT sum(amount_cents) FROM cash_adjustments WHERE store_id=$1 AND session_id=$2),0)`, store, input.Session).Scan(&net); e != nil {
		failure(w, 500, "cash_operation_failed")
		return
	}
	if input.Expected != opening+net {
		failure(w, 409, "cash_changed_reload")
		return
	}
	if opening+net+input.Amount < 0 {
		failure(w, 409, "cash_insufficient")
		return
	}
	e = tx.QueryRow(r.Context(), "INSERT INTO cash_adjustments(store_id,session_id,user_id,amount_cents,note,idempotency_key,request_hash) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id", store, input.Session, user.ID, input.Amount, input.Note, key, digest).Scan(&id)
	if e == nil {
		_, e = tx.Exec(r.Context(), "INSERT INTO audit(store_id,user_id,action,entity_id) VALUES($1,$2,'cash_adjustment',$3)", store, user.ID, id)
	}
	if e == nil {
		e = tx.Commit(r.Context())
	}
	moneyResponse(w, id, e)
}
