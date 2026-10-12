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

func migrateDining(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, diningSchema); err != nil {
		return fmt.Errorf("migration_v5_to_v6: %w", err)
	}
	_, err := tx.Exec(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES(6,$1)", Hash(schema+cashSchema+posSchema+diningSchema))
	return err
}

type DiningRequest struct {
	CustomerID int64        `json:"customer_id,omitempty"`
	Action     string       `json:"action"`
	ID         int64        `json:"id"`
	TableID    int64        `json:"table_id"`
	Label      string       `json:"label"`
	Version    int64        `json:"expected_version"`
	Expected   int64        `json:"expected_remaining_cents"`
	Items      []Item       `json:"items"`
	Payments   []POSPayment `json:"payments"`
	Note       string       `json:"note"`
}
type DiningTab struct {
	ID        int64  `json:"id"`
	TableID   *int64 `json:"table_id"`
	Label     string `json:"label"`
	State     string `json:"state"`
	Version   int64  `json:"version"`
	OpenedAt  string `json:"opened_at"`
	Total     int64  `json:"total_cents"`
	Received  int64  `json:"received_cents"`
	Remaining int64  `json:"remaining_cents"`
	Orders    int    `json:"order_count"`
}

func diningFailure(w http.ResponseWriter, err error) {
	known := map[string]bool{"cash_not_open": true, "tab_not_open": true, "tab_changed_reload": true, "table_unavailable": true, "tab_label_in_use": true, "tab_limit": true, "tab_not_empty": true, "payment_total_invalid": true, "idempotency_conflict": true, "checkout_price_changed": true, "stock_insufficient": true, "product_unavailable": true, "quantity_invalid": true, "total_limit": true, "order_duplicate_product": true, "configuration_invalid": true, "configuration_changed": true, "variant_required": true, "addon_unavailable": true, "addon_limits_invalid": true, "combo_invalid": true, "combo_unavailable": true, "combo_limits_invalid": true, "combo_quantity_invalid": true, "customer_invalid": true, "integrated_payment_unsupported": true}
	if known[err.Error()] {
		failure(w, 409, err.Error())
	} else {
		failure(w, 500, "dining_operation_failed")
	}
}
func diningReplay(ctx context.Context, tx pgx.Tx, store int64, key string, input any) (int64, bool, error) {
	var id int64
	var digest string
	err := tx.QueryRow(ctx, "SELECT result_id,request_hash FROM dining_operations WHERE store_id=$1 AND idempotency_key=$2", store, key).Scan(&id, &digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if digest != moneyDigest(input) {
		return 0, false, errors.New("idempotency_conflict")
	}
	return id, true, nil
}
func diningSave(ctx context.Context, tx pgx.Tx, store, user, id int64, key, action string, input any) error {
	if _, err := tx.Exec(ctx, "INSERT INTO dining_operations(store_id,idempotency_key,request_hash,result_id,action) VALUES($1,$2,$3,$4,$5)", store, key, moneyDigest(input), id, action); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "INSERT INTO audit(store_id,user_id,action,entity_id) VALUES($1,$2,$3,$4)", store, user, "dining_"+action, id)
	return err
}
func (a *App) diningTables(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanAccessStore(store) || !user.CanReadCash() {
		failure(w, 403, "forbidden_dining")
		return
	}
	if r.Method == "GET" {
		rows, err := a.DB.Query(r.Context(), "SELECT id,name,capacity,active FROM dining_tables WHERE store_id=$1 ORDER BY id LIMIT 200", store)
		if err != nil {
			failure(w, 500, "dining_read_failed")
			return
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var id int64
			var name string
			var capacity int
			var active bool
			if rows.Scan(&id, &name, &capacity, &active) != nil {
				failure(w, 500, "dining_read_failed")
				return
			}
			items = append(items, map[string]any{"id": id, "name": name, "capacity": capacity, "active": active})
		}
		if rows.Err() != nil {
			failure(w, 500, "dining_read_failed")
			return
		}
		jsonResponse(w, 200, items)
		return
	}
	if r.Method != "POST" {
		failure(w, 405, "method_not_allowed")
		return
	}
	if !user.CanManageCatalog() {
		failure(w, 403, "forbidden_dining_management")
		return
	}
	var input struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Capacity int    `json:"capacity"`
		Active   bool   `json:"active"`
	}
	if decode(w, r, &input) != nil || !moneyKey(r.Header.Get("Idempotency-Key")) || strings.TrimSpace(input.Name) == "" || len(input.Name) > 80 || input.Capacity < 1 || input.Capacity > 100 || input.ID < 0 {
		failure(w, 400, "table_invalid")
		return
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		failure(w, 500, "dining_operation_failed")
		return
	}
	defer tx.Rollback(r.Context())
	if err = cashLock(r.Context(), tx, store); err != nil {
		diningFailure(w, err)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	id, found, err := diningReplay(r.Context(), tx, store, key, input)
	if err != nil {
		diningFailure(w, err)
		return
	}
	if found {
		jsonResponse(w, 200, map[string]int64{"id": id})
		return
	}
	var busy bool
	var count int
	if err = tx.QueryRow(r.Context(), "SELECT count(*) FROM dining_tables WHERE store_id=$1", store).Scan(&count); err != nil {
		diningFailure(w, err)
		return
	}
	if input.ID == 0 && count >= 200 {
		failure(w, 409, "table_limit")
		return
	}
	if err = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM dining_tabs WHERE store_id=$1 AND table_id=$2 AND state='open')", store, input.ID).Scan(&busy); err != nil {
		diningFailure(w, err)
		return
	}
	if busy {
		failure(w, 409, "table_occupied")
		return
	}
	var duplicate bool
	if err = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM dining_tables WHERE store_id=$1 AND lower(name)=lower($2) AND id<>$3)", store, strings.TrimSpace(input.Name), input.ID).Scan(&duplicate); err != nil {
		diningFailure(w, err)
		return
	}
	if duplicate {
		failure(w, 409, "table_name_in_use")
		return
	}
	if input.ID == 0 {
		err = tx.QueryRow(r.Context(), "INSERT INTO dining_tables(store_id,name,capacity,active) VALUES($1,$2,$3,$4) RETURNING id", store, strings.TrimSpace(input.Name), input.Capacity, input.Active).Scan(&id)
	} else {
		err = tx.QueryRow(r.Context(), "UPDATE dining_tables SET name=$3,capacity=$4,active=$5 WHERE store_id=$1 AND id=$2 RETURNING id", store, input.ID, strings.TrimSpace(input.Name), input.Capacity, input.Active).Scan(&id)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		failure(w, 404, "table_not_found")
		return
	}
	if err == nil {
		err = diningSave(r.Context(), tx, store, user.ID, id, key, "table", input)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		diningFailure(w, err)
		return
	}
	jsonResponse(w, 200, map[string]int64{"id": id})
}
func (a *App) diningTabs(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanAccessStore(store) || !user.CanReadCash() {
		failure(w, 403, "forbidden_dining")
		return
	}
	if r.Method == "GET" {
		a.diningList(w, r, store)
		return
	}
	if r.Method != "POST" {
		failure(w, 405, "method_not_allowed")
		return
	}
	if !user.CanCreateOrders() {
		failure(w, 403, "forbidden_dining_management")
		return
	}
	var input DiningRequest
	if decode(w, r, &input) != nil || !moneyKey(r.Header.Get("Idempotency-Key")) || len(input.Label) > 80 || len(input.Note) > 1000 || input.ID < 0 || input.TableID < 0 || input.Expected < 0 || input.Expected > 1000000000000 || len(input.Items) > 100 || len(input.Payments) > 10 || !map[string]bool{"open": true, "add": true, "close": true, "cancel": true}[input.Action] {
		failure(w, 400, "tab_invalid")
		return
	}
	if (input.Action == "open" && strings.TrimSpace(input.Label) == "") || (input.Action != "open" && (input.ID < 1 || input.Version < 1)) || (input.Action == "add" && len(input.Items) == 0) {
		failure(w, 400, "tab_invalid")
		return
	}
	id, err := a.ChangeDining(r.Context(), store, user.ID, r.Header.Get("Idempotency-Key"), input)
	if err != nil {
		diningFailure(w, err)
		return
	}
	jsonResponse(w, 200, map[string]int64{"id": id})
}
func (a *App) ChangeDining(ctx context.Context, store, user int64, key string, input DiningRequest) (int64, error) {
	if !moneyKey(key) || !map[string]bool{"open": true, "add": true, "close": true, "cancel": true}[input.Action] || (input.Action != "open" && (input.ID < 1 || input.Version < 1)) || len(input.Label) > 80 || len(input.Note) > 1000 || input.Expected < 0 || input.Expected > 1000000000000 || len(input.Items) > 100 || len(input.Payments) > 10 {
		return 0, errors.New("tab_changed_reload")
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if err = cashLock(ctx, tx, store); err != nil {
		return 0, err
	}
	id, found, err := diningReplay(ctx, tx, store, key, input)
	if err != nil {
		return 0, err
	}
	if found {
		return id, tx.Commit(ctx)
	}
	if input.Action != "cancel" {
		var session int64
		if err = tx.QueryRow(ctx, "SELECT id FROM cash_sessions WHERE store_id=$1 AND closed_at IS NULL FOR UPDATE", store).Scan(&session); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return 0, errors.New("cash_not_open")
			}
			return 0, err
		}
	}
	id = input.ID
	if input.Action == "open" {
		if strings.TrimSpace(input.Label) == "" || input.TableID < 0 {
			return 0, errors.New("tab_changed_reload")
		}
		var exists bool
		var count int
		if err = tx.QueryRow(ctx, "SELECT count(*),COALESCE(bool_or(lower(label)=lower($2)),false) FROM dining_tabs WHERE store_id=$1 AND state='open'", store, strings.TrimSpace(input.Label)).Scan(&count, &exists); err != nil {
			return 0, err
		}
		if count >= 200 {
			return 0, errors.New("tab_limit")
		}
		if exists {
			return 0, errors.New("tab_label_in_use")
		}
		var table any
		if input.TableID > 0 {
			var active bool
			if err = tx.QueryRow(ctx, "SELECT active FROM dining_tables WHERE store_id=$1 AND id=$2 FOR UPDATE", store, input.TableID).Scan(&active); err != nil || !active {
				return 0, errors.New("table_unavailable")
			}
			if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM dining_tabs WHERE store_id=$1 AND table_id=$2 AND state='open')", store, input.TableID).Scan(&exists); err != nil {
				return 0, err
			}
			if exists {
				return 0, errors.New("table_unavailable")
			}
			table = input.TableID
		}
		if err = tx.QueryRow(ctx, "INSERT INTO dining_tabs(store_id,table_id,label,opened_by) VALUES($1,$2,$3,$4) RETURNING id", store, table, strings.TrimSpace(input.Label), user).Scan(&id); err != nil {
			return 0, err
		}
	} else {
		var state string
		var version int64
		if err = tx.QueryRow(ctx, "SELECT state,version FROM dining_tabs WHERE store_id=$1 AND id=$2 FOR UPDATE", store, id).Scan(&state, &version); err != nil || state != "open" {
			return 0, errors.New("tab_not_open")
		}
		if version != input.Version {
			return 0, errors.New("tab_changed_reload")
		}
		switch input.Action {
		case "add":
			if len(input.Items) < 1 {
				return 0, errors.New("quantity_invalid")
			}
			order, e := a.createOrderTx(ctx, tx, store, user, "dining:"+Hash(key), OrderRequest{Items: input.Items, CustomerID: input.CustomerID})
			if e != nil {
				return 0, e
			}
			var total int64
			if err = tx.QueryRow(ctx, "SELECT total_cents FROM orders WHERE store_id=$1 AND id=$2", store, order).Scan(&total); err != nil {
				return 0, err
			}
			if total != input.Expected {
				return 0, errors.New("checkout_price_changed")
			}
			var previousTotal int64
			var roundCount int
			if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(o.total_cents) FILTER(WHERE o.state<>'cancelled'),0),count(*) FROM dining_orders d JOIN orders o ON o.store_id=d.store_id AND o.id=d.order_id WHERE d.store_id=$1 AND d.tab_id=$2`, store, id).Scan(&previousTotal, &roundCount); err != nil {
				return 0, err
			}
			if previousTotal+total > 1000000000000 || roundCount >= 200 {
				return 0, errors.New("total_limit")
			}
			if _, err = tx.Exec(ctx, "UPDATE orders SET notes=$3 WHERE store_id=$1 AND id=$2", store, order, input.Note); err != nil {
				return 0, err
			}
			if _, err = tx.Exec(ctx, "INSERT INTO dining_orders(store_id,tab_id,order_id) VALUES($1,$2,$3)", store, id, order); err != nil {
				return 0, err
			}
		case "close":
			type dueOrder struct{ ID, Remaining int64 }
			dues := []dueOrder{}
			var remaining int64
			rows, e := tx.Query(ctx, `SELECT o.id,o.total_cents-COALESCE((SELECT sum(m.amount_cents) FROM manual_receipts m WHERE m.store_id=o.store_id AND m.order_id=o.id),0),EXISTS(SELECT 1 FROM payments p WHERE p.store_id=o.store_id AND p.order_id=o.id) FROM dining_orders d JOIN orders o ON o.store_id=d.store_id AND o.id=d.order_id WHERE d.store_id=$1 AND d.tab_id=$2 AND o.state<>'cancelled' ORDER BY o.id FOR UPDATE OF o`, store, id)
			if e != nil {
				return 0, e
			}
			var queryErr error
			for rows.Next() {
				var due dueOrder
				var integrated bool
				if queryErr = rows.Scan(&due.ID, &due.Remaining, &integrated); queryErr != nil {
					break
				}
				if integrated {
					queryErr = errors.New("integrated_payment_unsupported")
					break
				}
				if due.Remaining < 0 {
					queryErr = errors.New("tab_changed_reload")
					break
				}
				remaining += due.Remaining
				dues = append(dues, due)
			}
			if queryErr == nil {
				queryErr = rows.Err()
			}
			rows.Close()
			if queryErr != nil {
				return 0, queryErr
			}
			if remaining != input.Expected {
				return 0, errors.New("tab_changed_reload")
			}
			if remaining == 0 && len(input.Payments) != 0 {
				return 0, errors.New("payment_total_invalid")
			}
			if remaining > 0 {
				if e = validatePOSPayments(input.Payments, remaining); e != nil {
					return 0, e
				}
			}
			// Allocate receipts to existing orders: no second aggregate sale and no stock mutation.
			orderIndex := 0
			receiptIndex := 0
			for _, payment := range input.Payments {
				amount := payment.Amount
				for amount > 0 {
					for orderIndex < len(dues) && dues[orderIndex].Remaining == 0 {
						orderIndex++
					}
					if orderIndex == len(dues) {
						return 0, errors.New("payment_total_invalid")
					}
					part := amount
					if part > dues[orderIndex].Remaining {
						part = dues[orderIndex].Remaining
					}
					_, e = a.recordReceiptTx(ctx, tx, store, user, "dining:"+Hash(key)+fmt.Sprintf(":%d", receiptIndex), ReceiptRequest{OrderID: dues[orderIndex].ID, Amount: part, Method: payment.Method, Note: fmt.Sprintf("Fechamento da comanda #%d", id)})
					if e != nil {
						return 0, e
					}
					receiptIndex++
					amount -= part
					dues[orderIndex].Remaining -= part
				}
			}
			raw, _ := json.Marshal(input.Payments)
			if _, err = tx.Exec(ctx, "UPDATE dining_tabs SET state='closed',closed_at=now(),closed_by=$3,closing_payments=$4 WHERE store_id=$1 AND id=$2", store, id, user, raw); err != nil {
				return 0, err
			}
		case "cancel":
			var nonempty bool
			if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM dining_orders d JOIN orders o ON o.store_id=d.store_id AND o.id=d.order_id WHERE d.store_id=$1 AND d.tab_id=$2 AND o.state<>'cancelled')", store, id).Scan(&nonempty); err != nil {
				return 0, err
			}
			if nonempty {
				return 0, errors.New("tab_not_empty")
			}
			if _, err = tx.Exec(ctx, "UPDATE dining_tabs SET state='cancelled',closed_at=now(),closed_by=$3 WHERE store_id=$1 AND id=$2", store, id, user); err != nil {
				return 0, err
			}
		default:
			return 0, errors.New("tab_changed_reload")
		}
		if _, err = tx.Exec(ctx, "UPDATE dining_tabs SET version=version+1 WHERE store_id=$1 AND id=$2", store, id); err != nil {
			return 0, err
		}
	}
	if err = diningSave(ctx, tx, store, user, id, key, input.Action, input); err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}
func validatePOSPayments(payments []POSPayment, total int64) error {
	if len(payments) < 1 || len(payments) > 10 {
		return errors.New("payment_total_invalid")
	}
	var sum int64
	for _, p := range payments {
		if p.Amount < 1 || p.Amount > 1000000000000 || p.Tendered < 0 || p.Tendered > 1000000000000 || (p.Method != "cash" && p.Method != "card" && p.Method != "pix_manual") || (p.Method == "cash" && p.Tendered < p.Amount) || (p.Method != "cash" && p.Tendered != 0) {
			return errors.New("payment_total_invalid")
		}
		sum += p.Amount
	}
	if sum != total {
		return errors.New("payment_total_invalid")
	}
	return nil
}

const diningTabSelect = `SELECT t.id,t.table_id,t.label,t.state,t.version,t.opened_at::text,COALESCE(x.total,0),COALESCE(x.received,0),COALESCE(x.n,0) FROM dining_tabs t LEFT JOIN LATERAL(SELECT sum(o.total_cents) total,sum(COALESCE((SELECT sum(m.amount_cents) FROM manual_receipts m WHERE m.store_id=o.store_id AND m.order_id=o.id),0)) received,count(*) n FROM dining_orders d JOIN orders o ON o.store_id=d.store_id AND o.id=d.order_id WHERE d.store_id=t.store_id AND d.tab_id=t.id AND o.state<>'cancelled') x ON true WHERE t.store_id=$1`

func (a *App) diningList(w http.ResponseWriter, r *http.Request, store int64) {
	var id int64
	var err error
	if r.URL.Query().Get("id") != "" {
		id, err = strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err != nil || id < 1 {
			failure(w, 400, "tab_invalid")
			return
		}
	}
	query := diningTabSelect
	args := []any{store}
	if id > 0 {
		query += " AND t.id=$2"
		args = append(args, id)
	} else {
		query += " AND (t.state='open' OR t.id IN (SELECT id FROM dining_tabs WHERE store_id=$1 AND state<>'open' ORDER BY id DESC LIMIT 50)) ORDER BY (t.state='open') DESC,t.id DESC LIMIT 250"
	}
	rows, err := a.DB.Query(r.Context(), query, args...)
	if err != nil {
		failure(w, 500, "dining_read_failed")
		return
	}
	tabs := []DiningTab{}
	for rows.Next() {
		var t DiningTab
		if err = rows.Scan(&t.ID, &t.TableID, &t.Label, &t.State, &t.Version, &t.OpenedAt, &t.Total, &t.Received, &t.Orders); err != nil {
			break
		}
		t.Remaining = t.Total - t.Received
		tabs = append(tabs, t)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		failure(w, 500, "dining_read_failed")
		return
	}
	if id == 0 {
		jsonResponse(w, 200, tabs)
		return
	}
	if len(tabs) == 0 {
		failure(w, 404, "tab_not_found")
		return
	}
	orders, err := a.diningOrders(r.Context(), store, id)
	if err != nil {
		failure(w, 500, "dining_read_failed")
		return
	}
	jsonResponse(w, 200, map[string]any{"tab": tabs[0], "orders": orders})
}
func (a *App) diningOrders(ctx context.Context, store, id int64) ([]map[string]any, error) {
	rows, err := a.DB.Query(ctx, `SELECT o.id,o.total_cents,o.state,COALESCE(o.notes,''),o.created_at::text FROM dining_orders d JOIN orders o ON o.store_id=d.store_id AND o.id=d.order_id WHERE d.store_id=$1 AND d.tab_id=$2 ORDER BY o.id`, store, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var order, total int64
		var state, note, at string
		if err = rows.Scan(&order, &total, &state, &note, &at); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"id": order, "total_cents": total, "state": state, "note": note, "created_at": at})
	}
	return items, rows.Err()
}
func (a *App) diningReceipt(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanAccessStore(store) || !user.CanReadCash() {
		failure(w, 403, "forbidden_dining")
		return
	}
	if r.Method != "GET" {
		failure(w, 405, "method_not_allowed")
		return
	}
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil || id < 1 {
		failure(w, 400, "tab_invalid")
		return
	}
	var label, state, at, operator, storeName string
	var payments json.RawMessage
	err = a.DB.QueryRow(r.Context(), `SELECT t.label,t.state,COALESCE(t.closed_at,t.opened_at)::text,u.email,s.name,t.closing_payments FROM dining_tabs t JOIN stores s ON s.id=t.store_id JOIN users u ON u.id=COALESCE(t.closed_by,t.opened_by) WHERE t.store_id=$1 AND t.id=$2`, store, id).Scan(&label, &state, &at, &operator, &storeName, &payments)
	if errors.Is(err, pgx.ErrNoRows) {
		failure(w, 404, "tab_not_found")
		return
	}
	if err != nil {
		failure(w, 500, "dining_read_failed")
		return
	}
	rows, err := a.DB.Query(r.Context(), `SELECT i.name,i.price_cents,i.quantity,i.unit,COALESCE(i.notes,''),i.selection FROM dining_orders d JOIN orders o ON o.store_id=d.store_id AND o.id=d.order_id JOIN order_items i ON i.store_id=o.store_id AND i.order_id=o.id WHERE d.store_id=$1 AND d.tab_id=$2 AND o.state<>'cancelled' ORDER BY o.id,i.id`, store, id)
	if err != nil {
		failure(w, 500, "dining_read_failed")
		return
	}
	items := []map[string]any{}
	var total int64
	for rows.Next() {
		var name, unit, notes string
		var selection json.RawMessage
		var price int64
		var quantity float64
		if err = rows.Scan(&name, &price, &quantity, &unit, &notes, &selection); err != nil {
			break
		}
		total += price * int64(quantity)
		items = append(items, map[string]any{"name": name, "unit": unit, "price_cents": price, "quantity": quantity, "notes": notes, "selection": selection})
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		failure(w, 500, "dining_read_failed")
		return
	}
	var received int64
	err = a.DB.QueryRow(r.Context(), `SELECT COALESCE(sum(m.amount_cents),0) FROM manual_receipts m JOIN dining_orders d ON d.store_id=m.store_id AND d.order_id=m.order_id WHERE d.store_id=$1 AND d.tab_id=$2`, store, id).Scan(&received)
	if err != nil {
		failure(w, 500, "dining_read_failed")
		return
	}
	jsonResponse(w, 200, map[string]any{"id": id, "label": label, "store": storeName, "operator": operator, "created_at": at, "service": "table", "state": state, "financial_state": map[bool]string{true: "manual_confirmed", false: "manual_pending"}[received == total], "total_cents": total, "received_cents": received, "remaining_cents": total - received, "discount_cents": 0, "payments": payments, "items": items, "fiscal": false})
}
