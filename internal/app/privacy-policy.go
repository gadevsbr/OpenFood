package app

import (
	"context"
	_ "embed"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"time"
)

//go:embed privacy-policy.sql
var privacySchema string

func migratePrivacy(ctx context.Context, tx pgx.Tx) error {
	if _, e := tx.Exec(ctx, privacySchema); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES(11,$1)", Hash(schema+cashSchema+posSchema+diningSchema+menuSchema+comboSchema+pizzaSchema+scheduleSchema+privacySchema))
	return e
}

// The ledger contains identities of records, never contacts or free text. No FK:
// a request must remain available even when the restored backup predates its customer.
type PrivacyRemoval struct {
	StoreID         int64
	StoreCreated    time.Time
	CustomerID      int64
	CustomerCreated time.Time
	RemovedAt       time.Time
}

func PrivacyRemovals(ctx context.Context, db *pgxpool.Pool) ([]PrivacyRemoval, error) {
	rows, e := db.Query(ctx, "SELECT store_id,store_created_at,customer_id,customer_created_at,removed_at FROM privacy_removals ORDER BY store_id,customer_id")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	result := []PrivacyRemoval{}
	for rows.Next() {
		var v PrivacyRemoval
		if e = rows.Scan(&v.StoreID, &v.StoreCreated, &v.CustomerID, &v.CustomerCreated, &v.RemovedAt); e != nil {
			return nil, e
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func removeCustomerData(ctx context.Context, tx pgx.Tx, store, id int64) error {
	if _, e := tx.Exec(ctx, `INSERT INTO privacy_removals(store_id,store_created_at,customer_id,customer_created_at)
 SELECT c.store_id,s.created_at,c.id,c.created_at FROM customers c JOIN stores s ON s.id=c.store_id WHERE c.store_id=$1 AND c.id=$2
 ON CONFLICT DO NOTHING`, store, id); e != nil {
		return e
	}
	for _, query := range []string{
		`UPDATE customers SET name='Cliente sem identificação',phone=NULL,email=NULL,document=NULL,notes=NULL,marketing_consent=false,marketing_consent_at=NULL,anonymized_at=COALESCE(anonymized_at,now()),updated_at=now() WHERE store_id=$1 AND id=$2`,
		`UPDATE consents SET granted=false,revoked_at=COALESCE(revoked_at,now()),source='anonymized',evidence=NULL,terms_text='' WHERE store_id=$1 AND customer_id=$2`,
		`UPDATE order_items SET notes=NULL,selection=jsonb_set(selection,'{combo,components}',COALESCE((SELECT jsonb_agg(x - 'notes') FROM jsonb_array_elements(selection#>'{combo,components}') x),'[]')) WHERE store_id=$1 AND order_id IN (SELECT id FROM orders WHERE store_id=$1 AND customer_id=$2) AND selection ? 'combo'`,
		`UPDATE order_items SET notes=NULL WHERE store_id=$1 AND order_id IN (SELECT id FROM orders WHERE store_id=$1 AND customer_id=$2)`,
		`UPDATE manual_receipts SET note=CASE WHEN note='' THEN '' ELSE '[texto removido por solicitação]' END WHERE store_id=$1 AND order_id IN(SELECT id FROM orders WHERE store_id=$1 AND customer_id=$2)`,
		`UPDATE pos_checkouts SET discount_note=CASE WHEN discount_note='' THEN '' ELSE '[texto removido por solicitação]' END WHERE store_id=$1 AND order_id IN(SELECT id FROM orders WHERE store_id=$1 AND customer_id=$2)`,
		`UPDATE dining_tabs SET label='Comanda #'||id||' · identificação removida',version=version+1 WHERE store_id=$1 AND id IN(SELECT d.tab_id FROM dining_orders d JOIN orders o ON o.store_id=d.store_id AND o.id=d.order_id WHERE o.store_id=$1 AND o.customer_id=$2) AND label<>'Comanda #'||id||' · identificação removida'`,
		`UPDATE orders SET customer_name='Cliente sem identificação',customer_phone=NULL,customer_email=NULL,notes=NULL WHERE store_id=$1 AND customer_id=$2`,
	} {
		if _, e := tx.Exec(ctx, query, store, id); e != nil {
			return e
		}
	}
	_, e := tx.Exec(ctx, `INSERT INTO privacy_policies(store_id) VALUES($1) ON CONFLICT(store_id) DO UPDATE SET revision=md5(random()::text||clock_timestamp()::text)`, store)
	return e
}

// Run only against an offline staging database before its atomic promotion.
func ReapplyPrivacyRemovals(ctx context.Context, db *pgxpool.Pool, ledger []PrivacyRemoval) error {
	tx, e := db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "INSERT INTO privacy_policies(store_id) SELECT id FROM stores ON CONFLICT DO NOTHING"); e != nil {
		return e
	}
	for _, v := range ledger {
		if _, e = tx.Exec(ctx, `INSERT INTO privacy_removals VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, v.StoreID, v.StoreCreated, v.CustomerID, v.CustomerCreated, v.RemovedAt); e != nil {
			return e
		}
	}
	rows, e := tx.Query(ctx, `SELECT c.store_id,c.id FROM customers c JOIN stores s ON s.id=c.store_id JOIN privacy_removals p ON p.store_id=c.store_id AND p.store_created_at=s.created_at AND p.customer_id=c.id AND p.customer_created_at=c.created_at ORDER BY c.store_id,c.id`)
	if e != nil {
		return e
	}
	var ids [][2]int64
	for rows.Next() {
		var pair [2]int64
		if e = rows.Scan(&pair[0], &pair[1]); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, pair)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, v := range ids {
		if e = removeCustomerData(ctx, tx, v[0], v[1]); e != nil {
			return e
		}
	}
	// A restored policy must never silently re-enable an older destructive schedule.
	if _, e = tx.Exec(ctx, `UPDATE privacy_policies SET enabled=false,version=version+1,revision=md5(random()::text||clock_timestamp()::text); DELETE FROM sessions; UPDATE jobs SET state='failed',last_error_code='restored_requires_review',lease_until=NULL WHERE state IN ('running','pending')`); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

type PrivacyPolicy struct {
	Version    int64 `json:"version"`
	Enabled    bool  `json:"enabled"`
	Days       int   `json:"inactive_days"`
	Candidates int   `json:"candidates"`
	Confirm    bool  `json:"confirm"`
}

// Activity includes customer edits, consents, and orders. Unfinished or unpaid
// operations are excluded, even if all their dates are old.
const retentionCandidates = `SELECT c.id FROM customers c WHERE c.store_id=$1 AND c.anonymized_at IS NULL
 AND GREATEST(c.updated_at,c.created_at,COALESCE((SELECT max(GREATEST(o.created_at,o.confirmed_at,o.privacy_activity_at)) FROM orders o WHERE o.store_id=c.store_id AND o.customer_id=c.id),c.created_at),COALESCE((SELECT max(COALESCE(k.revoked_at,k.granted_at)) FROM consents k WHERE k.store_id=c.store_id AND k.customer_id=c.id),c.created_at),COALESCE((SELECT max(m.created_at) FROM manual_receipts m JOIN orders o ON o.store_id=m.store_id AND o.id=m.order_id WHERE o.store_id=c.store_id AND o.customer_id=c.id),c.created_at),COALESCE((SELECT max(t.closed_at) FROM dining_orders d JOIN dining_tabs t ON t.store_id=d.store_id AND t.id=d.tab_id JOIN orders o ON o.store_id=d.store_id AND o.id=d.order_id WHERE o.store_id=c.store_id AND o.customer_id=c.id),c.created_at),COALESCE((SELECT max(p.updated_at) FROM payments p JOIN orders o ON o.store_id=p.store_id AND o.id=p.order_id WHERE o.store_id=c.store_id AND o.customer_id=c.id),c.created_at)) < now()-make_interval(days=>$2)
 AND NOT EXISTS(SELECT 1 FROM orders o WHERE o.store_id=c.store_id AND o.customer_id=c.id AND (o.state NOT IN ('completed','cancelled') OR (o.state<>'cancelled' AND o.financial_state NOT IN ('manual_confirmed','payment_confirmed','refunded'))))
 AND NOT EXISTS(SELECT 1 FROM dining_orders d JOIN dining_tabs t ON t.store_id=d.store_id AND t.id=d.tab_id JOIN orders o ON o.store_id=d.store_id AND o.id=d.order_id WHERE o.store_id=c.store_id AND o.customer_id=c.id AND t.closed_at IS NULL)`

func (a *App) privacyState(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if r.Method != "GET" {
		failure(w, 405, "method_not_allowed")
		return
	}
	if !user.CanAccessStore(store) {
		failure(w, 403, "forbidden")
		return
	}
	var revision string
	e := a.DB.QueryRow(r.Context(), `SELECT COALESCE((SELECT revision FROM privacy_policies WHERE store_id=$1),'initial')`, store).Scan(&revision)
	if e != nil {
		failure(w, 500, "privacy_state_failed")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	jsonResponse(w, 200, map[string]string{"revision": revision})
}

func (a *App) privacyPolicy(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanAccessStore(store) || !user.CanManageUsers() {
		failure(w, 403, "forbidden_deletion_access")
		return
	}
	if r.Method != "GET" && r.Method != "POST" {
		failure(w, 405, "method_not_allowed")
		return
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		failure(w, 500, "privacy_policy_failed")
		return
	}
	defer tx.Rollback(r.Context())
	if _, e = tx.Exec(r.Context(), "SELECT id FROM stores WHERE id=$1 FOR UPDATE", store); e != nil {
		failure(w, 500, "privacy_policy_failed")
		return
	}
	if _, e = tx.Exec(r.Context(), "INSERT INTO privacy_policies(store_id,revision) VALUES($1,'initial') ON CONFLICT DO NOTHING", store); e != nil {
		failure(w, 500, "privacy_policy_failed")
		return
	}
	var p PrivacyPolicy
	if r.Method == "POST" {
		if decode(w, r, &p) != nil || p.Days < 0 || p.Days > 36500 || (p.Enabled && (p.Days == 0 || !p.Confirm)) {
			failure(w, 400, "privacy_policy_confirmation_required")
			return
		}
		tag, err := tx.Exec(r.Context(), `UPDATE privacy_policies SET enabled=$2,inactive_days=$3,version=version+1,updated_at=now() WHERE store_id=$1 AND version=$4`, store, p.Enabled, p.Days, p.Version)
		if err != nil {
			failure(w, 500, "privacy_policy_failed")
			return
		}
		if tag.RowsAffected() != 1 {
			failure(w, 409, "privacy_policy_changed")
			return
		}
		if _, e = tx.Exec(r.Context(), `INSERT INTO audit(store_id,user_id,action,entity_id,details) VALUES($1,$2,'privacy_policy_changed',$1,jsonb_build_object('enabled',$3::boolean,'inactive_days',$4::integer,'version',$5::bigint,'confirmed',$6::boolean))`, store, user.ID, p.Enabled, p.Days, p.Version+1, p.Confirm); e != nil {
			failure(w, 500, "privacy_policy_failed")
			return
		}
	}
	if e = tx.QueryRow(r.Context(), "SELECT version,enabled,inactive_days FROM privacy_policies WHERE store_id=$1", store).Scan(&p.Version, &p.Enabled, &p.Days); e != nil {
		failure(w, 500, "privacy_policy_failed")
		return
	}
	days := p.Days
	if r.Method == "GET" && r.URL.Query().Has("days") {
		days = int(ParseInt(r.URL.Query().Get("days")))
		if days < 1 || days > 36500 {
			failure(w, 400, "invalid_retention_days")
			return
		}
	}
	if days > 0 {
		if e = tx.QueryRow(r.Context(), "SELECT count(*) FROM ("+retentionCandidates+") x", store, days).Scan(&p.Candidates); e != nil {
			failure(w, 500, "privacy_policy_failed")
			return
		}
	}
	if e = tx.Commit(r.Context()); e != nil {
		failure(w, 500, "privacy_policy_failed")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	jsonResponse(w, 200, p)
}

// Bounded batches; retry on the next minute after rollback/error. Same locks as sales/removal.
func (a *App) RunRetention(ctx context.Context) error {
	a.gate.RLock()
	defer a.gate.RUnlock()
	rows, e := a.DB.Query(ctx, "SELECT store_id FROM privacy_policies WHERE enabled ORDER BY store_id")
	if e != nil {
		return e
	}
	var stores []int64
	for rows.Next() {
		var id int64
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		stores = append(stores, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, store := range stores {
		if e = a.retainStore(ctx, store); e != nil {
			return e
		}
	}
	return nil
}
func (a *App) retainStore(ctx context.Context, store int64) error {
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT id FROM stores WHERE id=$1 FOR UPDATE", store); e != nil {
		return e
	}
	if e = cashLock(ctx, tx, store); e != nil {
		return e
	}
	var days int
	e = tx.QueryRow(ctx, "SELECT inactive_days FROM privacy_policies WHERE store_id=$1 AND enabled", store).Scan(&days)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	rows, e := tx.Query(ctx, retentionCandidates+" ORDER BY c.id LIMIT 50 FOR UPDATE OF c", store, days)
	if e != nil {
		return e
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, id := range ids {
		if e = removeCustomerData(ctx, tx, store, id); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "INSERT INTO audit(store_id,action,entity_id) VALUES($1,'customer_retention_removed',$2)", store, id); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
