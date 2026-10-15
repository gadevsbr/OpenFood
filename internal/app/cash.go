package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func migrateCash(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, cashSchema); err != nil {
		return fmt.Errorf("migration_v3_to_v4: %w", err)
	}
	_, err := tx.Exec(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES(4,$1)", Hash(schema+cashSchema))
	return err
}
func (u *SessionUser) CanReceiveManual() bool { return u.CanManageCatalog() || u.Role == RoleAttendant }
func (u *SessionUser) CanOpenCash() bool      { return u.CanReceiveManual() }
func (u *SessionUser) CanReadCash() bool      { return u.CanViewFinancials() || u.Role == RoleAttendant }

type CashRequest struct {
	Action   string `json:"action"`
	ID       int64  `json:"id"`
	Opening  int64  `json:"opening_cents"`
	Counted  int64  `json:"counted_cents"`
	Expected int64  `json:"expected_cents"`
	Note     string `json:"note"`
}
type ReceiptRequest struct {
	OrderID  int64  `json:"order_id"`
	Amount   int64  `json:"amount_cents"`
	Method   string `json:"method"`
	Note     string `json:"note"`
	RefundOf int64  `json:"refund_of"`
}

func moneyKey(key string) bool     { return len(key) >= 8 && len(key) <= 128 }
func moneyDigest(input any) string { raw, _ := json.Marshal(input); return Hash(string(raw)) }
func cashLock(ctx context.Context, tx pgx.Tx, store int64) error {
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,1))", fmt.Sprintf("cash:%d", store))
	return err
}
func moneyResponse(w http.ResponseWriter, id int64, err error) {
	if err != nil {
		known := map[string]bool{"cash_already_open": true, "cash_not_open": true, "cash_changed_reload": true, "cash_difference_note_required": true, "idempotency_conflict": true, "receipt_invalid": true, "order_not_found": true, "order_cancelled": true, "overpayment": true, "receipt_not_found": true, "receipt_already_refunded": true, "cash_insufficient": true, "integrated_payment_unsupported": true}
		if known[err.Error()] {
			failure(w, 409, err.Error())
		} else {
			failure(w, 500, "cash_operation_failed")
		}
		return
	}
	jsonResponse(w, 200, map[string]int64{"id": id})
}
func (a *App) cashSessions(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanAccessStore(store) {
		failure(w, 403, "forbidden_cross_store_access")
		return
	}
	if !user.CanReadCash() {
		failure(w, 403, "forbidden_cash_access")
		return
	}
	if r.Method == "GET" {
		a.cashOverview(w, r, store)
		return
	}
	if r.Method != "POST" {
		failure(w, 405, "method_not_allowed")
		return
	}
	var input CashRequest
	if decode(w, r, &input) != nil || !moneyKey(r.Header.Get("Idempotency-Key")) || len(input.Note) > 1000 || input.Opening < 0 || input.Opening > 1000000000000 || input.Counted < 0 || input.Counted > 1000000000000 || (input.Action != "open" && input.Action != "close") {
		failure(w, 400, "cash_invalid")
		return
	}
	if !user.CanOpenCash() {
		failure(w, 403, "forbidden_cash_management")
		return
	}
	if input.Action == "close" && !user.CanManageCatalog() {
		var owner int64
		if err := a.DB.QueryRow(r.Context(), "SELECT opened_by FROM cash_sessions WHERE store_id=$1 AND id=$2", store, input.ID).Scan(&owner); err != nil || owner != user.ID {
			failure(w, 403, "forbidden_cash_management")
			return
		}
	}
	id, err := a.ChangeCash(r.Context(), store, user.ID, r.Header.Get("Idempotency-Key"), input)
	moneyResponse(w, id, err)
}
func (a *App) ChangeCash(ctx context.Context, store, user int64, key string, input CashRequest) (int64, error) {
	if !moneyKey(key) || (input.Action != "open" && input.Action != "close") || input.Opening < 0 || input.Opening > 1000000000000 || input.Counted < 0 || input.Counted > 1000000000000 || len(input.Note) > 1000 {
		return 0, errors.New("cash_invalid")
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if err = cashLock(ctx, tx, store); err != nil {
		return 0, err
	}
	digest := moneyDigest(input)
	var id int64
	var previous string
	query := "SELECT id,request_hash FROM cash_sessions WHERE store_id=$1 AND idempotency_key=$2"
	if input.Action == "close" {
		query = "SELECT id,close_hash FROM cash_sessions WHERE store_id=$1 AND close_key=$2"
	}
	err = tx.QueryRow(ctx, query, store, key).Scan(&id, &previous)
	if err == nil {
		if previous != digest {
			return 0, errors.New("idempotency_conflict")
		}
		return id, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	if input.Action == "open" {
		var exists bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM cash_sessions WHERE store_id=$1 AND closed_at IS NULL)", store).Scan(&exists); err != nil {
			return 0, err
		}
		if exists {
			return 0, errors.New("cash_already_open")
		}
		err = tx.QueryRow(ctx, "INSERT INTO cash_sessions(store_id,opened_by,opening_cents,note,idempotency_key,request_hash) VALUES($1,$2,$3,$4,$5,$6) RETURNING id", store, user, input.Opening, input.Note, key, digest).Scan(&id)
	} else {
		var opening, net int64
		if err = tx.QueryRow(ctx, "SELECT opening_cents FROM cash_sessions WHERE store_id=$1 AND id=$2 AND closed_at IS NULL FOR UPDATE", store, input.ID).Scan(&opening); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return 0, errors.New("cash_not_open")
			}
			return 0, err
		}
		if err = tx.QueryRow(ctx, "SELECT COALESCE(sum(amount_cents),0) FROM manual_receipts WHERE store_id=$1 AND session_id=$2 AND method='cash'", store, input.ID).Scan(&net); err != nil {
			return 0, err
		}
		var adjustments int64
		if err = tx.QueryRow(ctx, "SELECT COALESCE(sum(amount_cents),0) FROM cash_adjustments WHERE store_id=$1 AND session_id=$2", store, input.ID).Scan(&adjustments); err != nil {
			return 0, err
		}
		expected := opening + net + adjustments
		if input.Expected != expected {
			return 0, errors.New("cash_changed_reload")
		}
		if input.Counted != expected && strings.TrimSpace(input.Note) == "" {
			return 0, errors.New("cash_difference_note_required")
		}
		id = input.ID
		_, err = tx.Exec(ctx, "UPDATE cash_sessions SET closed_by=$3,closed_at=now(),counted_cents=$4,expected_cents=$5,difference_cents=$4::bigint-$5::bigint,close_note=$6,close_key=$7,close_hash=$8 WHERE store_id=$1 AND id=$2", store, id, user, input.Counted, expected, input.Note, key, digest)
	}
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit(store_id,user_id,action,entity_id) VALUES($1,$2,$3,$4)", store, user, "cash_"+input.Action, id); err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}
func (a *App) manualReceipts(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanAccessStore(store) {
		failure(w, 403, "forbidden_cross_store_access")
		return
	}
	if r.Method != "POST" {
		failure(w, 405, "method_not_allowed")
		return
	}
	if !user.CanReceiveManual() {
		failure(w, 403, "forbidden_receipt_management")
		return
	}
	var input ReceiptRequest
	if decode(w, r, &input) != nil || !moneyKey(r.Header.Get("Idempotency-Key")) || len(input.Note) > 1000 || input.Amount < 0 || input.Amount > 1000000000000 || input.RefundOf < 0 {
		failure(w, 400, "receipt_invalid")
		return
	}
	if input.RefundOf > 0 && !user.CanManageCatalog() {
		failure(w, 403, "forbidden_refund_management")
		return
	}
	if input.RefundOf > 0 && strings.TrimSpace(input.Note) == "" {
		failure(w, 400, "refund_note_required")
		return
	}
	if input.RefundOf == 0 && (input.OrderID < 1 || input.Amount < 1 || (input.Method != "cash" && input.Method != "card" && input.Method != "pix_manual")) {
		failure(w, 400, "receipt_invalid")
		return
	}
	id, err := a.RecordReceipt(r.Context(), store, user.ID, r.Header.Get("Idempotency-Key"), input)
	moneyResponse(w, id, err)
}
func (a *App) RecordReceipt(ctx context.Context, store, user int64, key string, input ReceiptRequest) (int64, error) {
	if !moneyKey(key) || len(input.Note) > 1000 || input.RefundOf < 0 || input.Amount < 0 || input.Amount > 1000000000000 || (input.RefundOf == 0 && (input.OrderID < 1 || input.Amount < 1 || (input.Method != "cash" && input.Method != "card" && input.Method != "pix_manual"))) {
		return 0, errors.New("receipt_invalid")
	}
	if input.RefundOf > 0 && strings.TrimSpace(input.Note) == "" {
		return 0, errors.New("refund_note_required")
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if err = cashLock(ctx, tx, store); err != nil {
		return 0, err
	}
	id, err := a.recordReceiptTx(ctx, tx, store, user, key, input)
	if err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}

func (a *App) recordReceiptTx(ctx context.Context, tx pgx.Tx, store, user int64, key string, input ReceiptRequest) (int64, error) {
	var err error
	digest := moneyDigest(input)
	var id int64
	var previous string
	err = tx.QueryRow(ctx, "SELECT id,request_hash FROM manual_receipts WHERE store_id=$1 AND idempotency_key=$2", store, key).Scan(&id, &previous)
	if err == nil {
		if previous != digest {
			return 0, errors.New("idempotency_conflict")
		}
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	var session, opening int64
	if err = tx.QueryRow(ctx, "SELECT id,opening_cents FROM cash_sessions WHERE store_id=$1 AND closed_at IS NULL FOR UPDATE", store).Scan(&session, &opening); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, errors.New("cash_not_open")
		}
		return 0, err
	}
	order, amount, method := input.OrderID, input.Amount, input.Method
	if input.RefundOf > 0 {
		var original int64
		if err = tx.QueryRow(ctx, "SELECT order_id,amount_cents,method FROM manual_receipts WHERE store_id=$1 AND id=$2 AND amount_cents>0", store, input.RefundOf).Scan(&order, &original, &method); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return 0, errors.New("receipt_not_found")
			}
			return 0, err
		}
		var refunded bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM manual_receipts WHERE store_id=$1 AND refund_of=$2)", store, input.RefundOf).Scan(&refunded); err != nil {
			return 0, err
		}
		if refunded {
			return 0, errors.New("receipt_already_refunded")
		}
		amount = -original
	}
	var total, net int64
	var state, financial string
	if err = tx.QueryRow(ctx, "SELECT total_cents,state,financial_state FROM orders WHERE store_id=$1 AND id=$2 FOR UPDATE", store, order).Scan(&total, &state, &financial); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, errors.New("order_not_found")
		}
		return 0, err
	}
	recordedNote := input.Note
	var identificationRemoved bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM orders o JOIN customers c ON c.store_id=o.store_id AND c.id=o.customer_id WHERE o.store_id=$1 AND o.id=$2 AND c.anonymized_at IS NOT NULL)", store, order).Scan(&identificationRemoved); err != nil {
		return 0, err
	}
	if identificationRemoved && recordedNote != "" {
		recordedNote = "[texto removido por solicitação]"
	}
	var integrated bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM payments WHERE store_id=$1 AND order_id=$2)", store, order).Scan(&integrated); err != nil {
		return 0, err
	}
	if integrated {
		return 0, errors.New("integrated_payment_unsupported")
	}
	if !strings.HasPrefix(financial, "manual_") && financial != "refunded" && financial != "refund_partial" {
		return 0, errors.New("integrated_payment_unsupported")
	}
	if err = tx.QueryRow(ctx, "SELECT COALESCE(sum(amount_cents),0) FROM manual_receipts WHERE store_id=$1 AND order_id=$2", store, order).Scan(&net); err != nil {
		return 0, err
	}
	if amount > 0 && state == "cancelled" {
		return 0, errors.New("order_cancelled")
	}
	if net+amount > total || net+amount < 0 {
		return 0, errors.New("overpayment")
	}
	if method == "cash" && amount < 0 {
		var cash int64
		if err = tx.QueryRow(ctx, "SELECT COALESCE(sum(amount_cents),0) FROM manual_receipts WHERE store_id=$1 AND session_id=$2 AND method='cash'", store, session).Scan(&cash); err != nil {
			return 0, err
		}
		var adjustments int64
		if err = tx.QueryRow(ctx, "SELECT COALESCE(sum(amount_cents),0) FROM cash_adjustments WHERE store_id=$1 AND session_id=$2", store, session).Scan(&adjustments); err != nil {
			return 0, err
		}
		if opening+cash+adjustments+amount < 0 {
			return 0, errors.New("cash_insufficient")
		}
	}
	var refund any
	if input.RefundOf > 0 {
		refund = input.RefundOf
	}
	err = tx.QueryRow(ctx, "INSERT INTO manual_receipts(store_id,session_id,order_id,method,amount_cents,user_id,note,refund_of,idempotency_key,request_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id", store, session, order, method, amount, user, recordedNote, refund, key, digest).Scan(&id)
	if err != nil {
		return 0, err
	}
	status := "manual_pending"
	if net+amount == total {
		status = "manual_confirmed"
	} else if amount < 0 {
		if net+amount == 0 {
			status = "refunded"
		} else {
			status = "refund_partial"
		}
	}
	if _, err = tx.Exec(ctx, "UPDATE orders SET financial_state=$3 WHERE store_id=$1 AND id=$2", store, order, status); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit(store_id,user_id,action,entity_id) VALUES($1,$2,$3,$4)", store, user, "manual_"+map[bool]string{true: "refund", false: "receipt"}[amount < 0], id); err != nil {
		return 0, err
	}
	return id, nil
}

type CashSessionView struct {
	ID          int64      `json:"id"`
	Opening     int64      `json:"opening_cents"`
	Opened      time.Time  `json:"opened_at"`
	Closed      *time.Time `json:"closed_at"`
	Counted     *int64     `json:"counted_cents"`
	Expected    *int64     `json:"expected_cents"`
	Difference  *int64     `json:"difference_cents"`
	Cash        int64      `json:"cash_cents"`
	Card        int64      `json:"card_cents"`
	Pix         int64      `json:"pix_cents"`
	Note        string     `json:"note"`
	Adjustments int64      `json:"adjustments_cents"`
	OpenedByID  int64      `json:"opened_by_id"`
	OpeningNote string     `json:"opening_note"`
	OpenedBy    string     `json:"opened_by"`
	ClosedBy    string     `json:"closed_by"`
}

func (a *App) cashOverview(w http.ResponseWriter, r *http.Request, store int64) {
	ctx := r.Context()
	tx, err := a.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		failure(w, 500, "cash_read_failed")
		return
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT s.id,s.opening_cents,s.opened_at,s.closed_at,s.counted_cents,s.expected_cents,s.difference_cents,COALESCE(sum(m.amount_cents) FILTER(WHERE m.method='cash'),0),COALESCE(sum(m.amount_cents) FILTER(WHERE m.method='card'),0),COALESCE(sum(m.amount_cents) FILTER(WHERE m.method='pix_manual'),0),CASE WHEN s.closed_at IS NULL THEN s.note ELSE s.close_note END,s.note,u.email,COALESCE(c.email,''),COALESCE((SELECT sum(x.amount_cents) FROM cash_adjustments x WHERE x.store_id=s.store_id AND x.session_id=s.id),0),s.opened_by FROM cash_sessions s JOIN users u ON u.id=s.opened_by LEFT JOIN users c ON c.id=s.closed_by LEFT JOIN manual_receipts m ON m.store_id=s.store_id AND m.session_id=s.id WHERE s.store_id=$1 GROUP BY s.id,u.email,c.email ORDER BY s.id DESC LIMIT 100`, store)
	if err != nil {
		failure(w, 500, "cash_read_failed")
		return
	}
	sessions := []CashSessionView{}
	for rows.Next() {
		var s CashSessionView
		if err = rows.Scan(&s.ID, &s.Opening, &s.Opened, &s.Closed, &s.Counted, &s.Expected, &s.Difference, &s.Cash, &s.Card, &s.Pix, &s.Note, &s.OpeningNote, &s.OpenedBy, &s.ClosedBy, &s.Adjustments, &s.OpenedByID); err != nil {
			rows.Close()
			failure(w, 500, "cash_read_failed")
			return
		}
		sessions = append(sessions, s)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		failure(w, 500, "cash_read_failed")
		return
	}
	rows, err = tx.Query(ctx, `SELECT m.id,m.order_id,m.session_id,m.method,m.amount_cents,m.created_at,m.note,u.email,m.refund_of,EXISTS(SELECT 1 FROM manual_receipts f WHERE f.refund_of=m.id AND f.store_id=m.store_id) FROM manual_receipts m JOIN users u ON u.id=m.user_id WHERE m.store_id=$1 ORDER BY m.id DESC LIMIT 500`, store)
	if err != nil {
		failure(w, 500, "cash_read_failed")
		return
	}
	receipts := []map[string]any{}
	for rows.Next() {
		var id, order, session, amount int64
		var method, note, email string
		var at time.Time
		var refund *int64
		var refunded bool
		if err = rows.Scan(&id, &order, &session, &method, &amount, &at, &note, &email, &refund, &refunded); err != nil {
			rows.Close()
			failure(w, 500, "cash_read_failed")
			return
		}
		receipts = append(receipts, map[string]any{"id": id, "order_id": order, "session_id": session, "amount_cents": amount, "method": method, "created_at": at, "note": note, "operator": email, "refund_of": refund, "refunded": refunded})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		failure(w, 500, "cash_read_failed")
		return
	}
	rows, err = tx.Query(ctx, `SELECT o.id,o.total_cents,COALESCE(sum(m.amount_cents),0) FROM orders o LEFT JOIN manual_receipts m ON m.store_id=o.store_id AND m.order_id=o.id WHERE o.store_id=$1 AND o.state<>'cancelled' AND NOT EXISTS(SELECT 1 FROM payments p WHERE p.store_id=o.store_id AND p.order_id=o.id) AND o.financial_state IN ('manual_pending','manual_confirmed','refunded','refund_partial') GROUP BY o.id HAVING o.total_cents>COALESCE(sum(m.amount_cents),0) ORDER BY o.id DESC LIMIT 200`, store)
	if err != nil {
		failure(w, 500, "cash_read_failed")
		return
	}
	pending := []map[string]int64{}
	for rows.Next() {
		var id, total, received int64
		if err = rows.Scan(&id, &total, &received); err != nil {
			rows.Close()
			failure(w, 500, "cash_read_failed")
			return
		}
		pending = append(pending, map[string]int64{"id": id, "total_cents": total, "received_cents": received, "remaining_cents": total - received})
	}
	err = rows.Err()
	rows.Close()
	if err != nil || tx.Commit(ctx) != nil {
		failure(w, 500, "cash_read_failed")
		return
	}
	jsonResponse(w, 200, map[string]any{"sessions": sessions, "receipts": receipts, "pending": pending, "currency": "BRL"})
}
