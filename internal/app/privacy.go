package app

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"net/http"
)

// Keys/counts are exposed to the operator; raw fields are used only to detect changes before removal.
type PrivacyReview struct {
	CustomerID int64          `json:"customer_id"`
	Name       string         `json:"name"`
	Removed    bool           `json:"identification_removed"`
	Version    string         `json:"version"`
	Counts     map[string]int `json:"counts"`
}

var privacyReviewQueries = map[string]string{
	"orders":    `SELECT COALESCE(jsonb_agg(to_jsonb(x) ORDER BY id),'[]') FROM (SELECT id,customer_name,customer_phone,customer_email,notes,state,financial_state,total_cents FROM orders WHERE store_id=$1 AND customer_id=$2) x`,
	"items":     `SELECT COALESCE(jsonb_agg(to_jsonb(x) ORDER BY id),'[]') FROM (SELECT i.id,i.notes,i.selection FROM order_items i JOIN orders o ON o.store_id=i.store_id AND o.id=i.order_id WHERE o.store_id=$1 AND o.customer_id=$2) x`,
	"consents":  `SELECT COALESCE(jsonb_agg(to_jsonb(x) ORDER BY id),'[]') FROM (SELECT * FROM consents WHERE store_id=$1 AND customer_id=$2) x`,
	"receipts":  `SELECT COALESCE(jsonb_agg(to_jsonb(x) ORDER BY id),'[]') FROM (SELECT m.id,m.note,m.amount_cents FROM manual_receipts m JOIN orders o ON o.store_id=m.store_id AND o.id=m.order_id WHERE o.store_id=$1 AND o.customer_id=$2) x`,
	"checkouts": `SELECT COALESCE(jsonb_agg(to_jsonb(x) ORDER BY id),'[]') FROM (SELECT p.id,p.discount_note,p.discount_cents FROM pos_checkouts p JOIN orders o ON o.store_id=p.store_id AND o.id=p.order_id WHERE o.store_id=$1 AND o.customer_id=$2) x`,
	"tabs":      `SELECT COALESCE(jsonb_agg(to_jsonb(x) ORDER BY id),'[]') FROM (SELECT t.id,t.label,t.version FROM dining_tabs t WHERE t.store_id=$1 AND t.id IN(SELECT d.tab_id FROM dining_orders d JOIN orders o ON o.store_id=d.store_id AND o.id=d.order_id WHERE o.store_id=$1 AND o.customer_id=$2)) x`,
}

func readPrivacyReview(ctx context.Context, q menuQuery, store, id int64) (PrivacyReview, error) {
	p := PrivacyReview{CustomerID: id, Counts: map[string]int{}}
	raw := map[string]json.RawMessage{}
	var c []byte
	if e := q.QueryRow(ctx, "SELECT to_jsonb(c),name,anonymized_at IS NOT NULL FROM customers c WHERE store_id=$1 AND id=$2", store, id).Scan(&c, &p.Name, &p.Removed); e != nil {
		return p, e
	}
	raw["customer"] = c
	for name, query := range privacyReviewQueries {
		var b []byte
		if e := q.QueryRow(ctx, query, store, id).Scan(&b); e != nil {
			return p, e
		}
		raw[name] = b
		var entries []json.RawMessage
		if e := json.Unmarshal(b, &entries); e != nil {
			return p, e
		}
		p.Counts[name] = len(entries)
	}
	b, e := json.Marshal(raw)
	if e != nil {
		return p, e
	}
	p.Version = Hash(string(b))
	return p, nil
}

func (a *App) privacyReview(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanAccessStore(store) || !user.CanManageUsers() {
		failure(w, 403, "forbidden_deletion_access")
		return
	}
	if r.Method != "GET" {
		failure(w, 405, "method_not_allowed")
		return
	}
	id := ParseInt(r.URL.Query().Get("customer_id"))
	if id < 1 {
		failure(w, 400, "customer_id_required")
		return
	}
	tx, e := a.DB.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		failure(w, 500, "privacy_review_failed")
		return
	}
	defer tx.Rollback(r.Context())
	p, e := readPrivacyReview(r.Context(), tx, store, id)
	if e != nil {
		if e == pgx.ErrNoRows {
			failure(w, 404, "customer_not_found")
		} else {
			failure(w, 500, "privacy_review_failed")
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	jsonResponse(w, 200, p)
}
