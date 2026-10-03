package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

//go:embed schema.sql
var schema string

//go:embed web.html
var web []byte

const Version = "0.1.0-alpha.1"

type Maintenance interface {
	Backup(context.Context) (string, error)
	Restore(context.Context, []byte) error
	Diagnostics() any
}
type App struct {
	DB           *pgxpool.Pool
	SetupToken   string
	Ops          Maintenance
	OnShutdown   func()
	gate         sync.RWMutex
	authMu       sync.Mutex
	authAttempts map[string]attempt
}
type attempt struct {
	count int
	until time.Time
}

func Token() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func Hash(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }
func Open(ctx context.Context, dsn string, setup string, ops Maintenance) (*App, error) {
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		return nil, errors.New("database_configuration_invalid")
	}
	cfg.MaxConns = 8
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		return nil, e
	}
	a := &App{DB: db, SetupToken: setup, Ops: ops, authAttempts: map[string]attempt{}}
	if e = db.Ping(ctx); e != nil {
		db.Close()
		return nil, errors.New("database_unavailable")
	}
	if e = a.Migrate(ctx); e != nil {
		db.Close()
		return nil, e
	}
	return a, nil
}
func (a *App) Migrate(ctx context.Context) error {
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(717331)"); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations(version integer PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())"); e != nil {
		return e
	}
	var n int
	var checksum string
	e = tx.QueryRow(ctx, "SELECT version,checksum FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&n, &checksum)
	if errors.Is(e, pgx.ErrNoRows) {
		if _, e = tx.Exec(ctx, schema); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES(1,$1)", Hash(schema))
	} else if e == nil && (n != 1 || checksum != Hash(schema)) {
		return errors.New("migration_incompatible")
	}
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func ValidateSchema(version int, checksum string) error {
	if version != 1 || checksum != Hash(schema) {
		return errors.New("migration_incompatible")
	}
	return nil
}
func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func failure(w http.ResponseWriter, status int, code string) {
	jsonResponse(w, status, map[string]string{"error": code})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return e
	}
	var extra any
	if e := d.Decode(&extra); e == io.EOF {
		return nil
	}
	return errors.New("extra_json")
}
func (a *App) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("X-Request-ID", Token()[:16])
		host := strings.Split(r.Host, ":")[0]
		if host != "127.0.0.1" && host != "localhost" {
			failure(w, 403, "host_not_allowed")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			origin := r.Header.Get("Origin")
			u, e := url.Parse(origin)
			if origin != "" && (e != nil || u.Host != r.Host) {
				failure(w, 403, "origin_not_allowed")
				return
			}
			if r.Header.Get("X-OpenFood") != "1" {
				failure(w, 403, "csrf_header_required")
				return
			}
		}
		if r.URL.Path == "/api/restore" {
			a.gate.Lock()
			defer a.gate.Unlock()
		} else {
			a.gate.RLock()
			defer a.gate.RUnlock()
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		switch r.URL.Path {
		case "/":
			if r.Method != "GET" {
				failure(w, 405, "method_not_allowed")
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(web)
			return
		case "/healthz":
			jsonResponse(w, 200, map[string]string{"status": "alive"})
			return
		case "/readyz":
			if a.DB.Ping(ctx) != nil {
				failure(w, 503, "database_unavailable")
				return
			}
			jsonResponse(w, 200, map[string]string{"status": "ready", "version": Version})
			return
		case "/api/setup":
			a.setup(w, r)
			return
		case "/api/login":
			a.login(w, r)
			return
		}
		cookie, e := r.Cookie("openfood_session")
		if e != nil {
			failure(w, 401, "login_required")
			return
		}
		var user, store int64
		if e = a.DB.QueryRow(ctx, "SELECT u.id,u.store_id FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now()", Hash(cookie.Value)).Scan(&user, &store); e != nil {
			failure(w, 401, "session_invalid")
			return
		}
		switch r.URL.Path {
		case "/api/shutdown":
			if r.Method != "POST" {
				failure(w, 405, "method_not_allowed")
				return
			}
			if a.OnShutdown == nil {
				failure(w, 501, "shutdown_not_configured")
				return
			}
			jsonResponse(w, 200, map[string]bool{"ok": true})
			go func() { time.Sleep(200 * time.Millisecond); a.OnShutdown() }()
		case "/api/logout":
			if r.Method != "POST" {
				failure(w, 405, "method_not_allowed")
				return
			}
			a.DB.Exec(ctx, "DELETE FROM sessions WHERE token_hash=$1", Hash(cookie.Value))
			http.SetCookie(w, &http.Cookie{Name: "openfood_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
			jsonResponse(w, 200, map[string]bool{"ok": true})
		case "/api/status":
			var name string
			var pending, failed int
			a.DB.QueryRow(ctx, "SELECT name FROM stores WHERE id=$1", store).Scan(&name)
			a.DB.QueryRow(ctx, "SELECT count(*) FILTER(WHERE state IN ('pending','running')),count(*) FILTER(WHERE state='failed') FROM jobs WHERE store_id=$1", store).Scan(&pending, &failed)
			jsonResponse(w, 200, map[string]any{"store": name, "version": Version, "pending_jobs": pending, "failed_jobs": failed, "integrations": "disabled", "public_webhooks": "not_configured", "local_only": true})
		case "/api/products":
			a.products(w, r, store, user)
		case "/api/orders":
			a.orders(w, r, store, user)
		case "/api/order-state":
			a.orderState(w, r, store, user)
		case "/api/backup":
			if r.Method != "POST" {
				failure(w, 405, "method_not_allowed")
				return
			}
			if a.Ops == nil {
				failure(w, 501, "backup_not_configured")
				return
			}
			path, e := a.Ops.Backup(ctx)
			if e != nil {
				failure(w, 500, "backup_failed")
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", "attachment; filename=openfood.dump")
			http.ServeFile(w, r, path)
		case "/api/restore":
			a.restore(w, r)
		case "/api/diagnostics":
			if a.Ops == nil {
				failure(w, 501, "diagnostics_not_configured")
				return
			}
			w.Header().Set("Content-Disposition", "attachment; filename=openfood-diagnostics.json")
			jsonResponse(w, 200, a.Ops.Diagnostics())
		default:
			failure(w, 404, "not_found")
		}
	})
}
func (a *App) setup(w http.ResponseWriter, r *http.Request) {
	var count int
	if e := a.DB.QueryRow(r.Context(), "SELECT count(*) FROM users").Scan(&count); e != nil {
		failure(w, 503, "database_unavailable")
		return
	}
	if r.Method == "GET" {
		jsonResponse(w, 200, map[string]bool{"required": count == 0})
		return
	}
	if r.Method != "POST" {
		failure(w, 405, "method_not_allowed")
		return
	}
	var input struct{ Token, Email, Password, Store string }
	if decode(w, r, &input) != nil || len(input.Password) < 12 || len(input.Password) > 72 || len(input.Store) < 1 || len(input.Store) > 120 || !strings.Contains(input.Email, "@") {
		failure(w, 400, "setup_input_invalid")
		return
	}
	if a.SetupToken == "" || Hash(input.Token) != Hash(a.SetupToken) {
		failure(w, 403, "setup_token_invalid")
		return
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(input.Password), 12)
	if e != nil {
		failure(w, 500, "password_hash_failed")
		return
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		failure(w, 503, "database_unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	tx.Exec(r.Context(), "SELECT pg_advisory_xact_lock(717332)")
	if e = tx.QueryRow(r.Context(), "SELECT count(*) FROM users").Scan(&count); e != nil || count != 0 {
		failure(w, 409, "setup_closed")
		return
	}
	var store int64
	e = tx.QueryRow(r.Context(), "INSERT INTO stores(name) VALUES($1) RETURNING id", input.Store).Scan(&store)
	if e == nil {
		_, e = tx.Exec(r.Context(), "INSERT INTO users(store_id,email,password_hash) VALUES($1,$2,$3)", store, strings.ToLower(input.Email), string(hash))
	}
	if e == nil {
		e = tx.Commit(r.Context())
	}
	if e != nil {
		failure(w, 500, "setup_failed")
		return
	}
	jsonResponse(w, 201, map[string]bool{"ok": true})
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		failure(w, 405, "method_not_allowed")
		return
	}
	var input struct{ Email, Password string }
	if decode(w, r, &input) != nil {
		failure(w, 400, "input_invalid")
		return
	}
	key := strings.ToLower(input.Email)
	a.authMu.Lock()
	at := a.authAttempts[key]
	if time.Now().After(at.until) {
		at = attempt{until: time.Now().Add(15 * time.Minute)}
	}
	at.count++
	if len(a.authAttempts) > 10000 {
		clear(a.authAttempts)
	}
	a.authAttempts[key] = at
	a.authMu.Unlock()
	if at.count > 10 {
		failure(w, 429, "login_rate_limited")
		return
	}
	var id int64
	var hash string
	e := a.DB.QueryRow(r.Context(), "SELECT id,password_hash FROM users WHERE email=$1", key).Scan(&id, &hash)
	if e != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.Password)) != nil {
		failure(w, 401, "credentials_invalid")
		return
	}
	token := Token()
	_, e = a.DB.Exec(r.Context(), "INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '12 hours')", Hash(token), id)
	if e != nil {
		failure(w, 500, "session_failed")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "openfood_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 43200})
	jsonResponse(w, 200, map[string]bool{"ok": true})
}
func (a *App) products(w http.ResponseWriter, r *http.Request, store, user int64) {
	if r.Method == "GET" {
		rows, e := a.DB.Query(r.Context(), "SELECT id,name,price_cents,stock FROM products WHERE store_id=$1 ORDER BY id", store)
		if e != nil {
			failure(w, 500, "catalog_failed")
			return
		}
		defer rows.Close()
		result := []map[string]any{}
		for rows.Next() {
			var id, price, stock int64
			var name string
			if rows.Scan(&id, &name, &price, &stock) != nil {
				failure(w, 500, "catalog_failed")
				return
			}
			result = append(result, map[string]any{"id": id, "name": name, "price_cents": price, "stock": stock})
		}
		jsonResponse(w, 200, result)
		return
	}
	if r.Method != "POST" {
		failure(w, 405, "method_not_allowed")
		return
	}
	var p struct {
		Name       string
		PriceCents int64 `json:"price_cents"`
		Stock      int64
	}
	if decode(w, r, &p) != nil || len(p.Name) < 1 || len(p.Name) > 200 || p.PriceCents < 0 || p.PriceCents > 100000000 || p.Stock < 0 {
		failure(w, 400, "product_invalid")
		return
	}
	var id int64
	e := a.DB.QueryRow(r.Context(), "INSERT INTO products(store_id,name,price_cents,stock) VALUES($1,$2,$3,$4) RETURNING id", store, p.Name, p.PriceCents, p.Stock).Scan(&id)
	if e != nil {
		failure(w, 500, "product_failed")
		return
	}
	jsonResponse(w, 201, map[string]int64{"id": id})
}

type Item struct {
	ProductID int64 `json:"product_id"`
	Quantity  int64 `json:"quantity"`
}
type OrderRequest struct {
	Items []Item `json:"items"`
}

func (a *App) CreateOrder(ctx context.Context, store, user int64, key string, input OrderRequest) (int64, error) {
	if len(key) < 8 || len(key) > 128 || len(input.Items) == 0 || len(input.Items) > 100 {
		return 0, errors.New("order_invalid")
	}
	raw, _ := json.Marshal(input)
	digest := Hash(string(raw))
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback(ctx)
	// A transaction-scoped key lock serializes duplicate requests even before an order exists.
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", fmt.Sprintf("%d:%s", store, key)); e != nil {
		return 0, e
	}
	var id int64
	var previous string
	e = tx.QueryRow(ctx, "SELECT id,request_hash FROM orders WHERE store_id=$1 AND idempotency_key=$2", store, key).Scan(&id, &previous)
	if e == nil {
		if previous != digest {
			return 0, errors.New("idempotency_conflict")
		}
		return id, tx.Commit(ctx)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return 0, e
	}
	type snapshot struct {
		Item
		Name  string
		Price int64
	}
	snapshots := []snapshot{}
	var total int64
	// Store lock gives deterministic serialization of all stock changes within one store.
	if _, e = tx.Exec(ctx, "SELECT id FROM stores WHERE id=$1 FOR UPDATE", store); e != nil {
		return 0, e
	}
	for _, item := range input.Items {
		if item.Quantity < 1 || item.Quantity > 100000 {
			return 0, errors.New("quantity_invalid")
		}
		var name string
		var price, stock int64
		if e = tx.QueryRow(ctx, "SELECT name,price_cents,stock FROM products WHERE store_id=$1 AND id=$2 FOR UPDATE", store, item.ProductID).Scan(&name, &price, &stock); e != nil {
			return 0, errors.New("product_unavailable")
		}
		if stock < item.Quantity {
			return 0, errors.New("stock_insufficient")
		}
		total += price * item.Quantity
		if total > 1000000000000 {
			return 0, errors.New("total_limit")
		}
		if _, e = tx.Exec(ctx, "UPDATE products SET stock=stock-$3 WHERE store_id=$1 AND id=$2", store, item.ProductID, item.Quantity); e != nil {
			return 0, e
		}
		snapshots = append(snapshots, snapshot{item, name, price})
	}
	e = tx.QueryRow(ctx, "INSERT INTO orders(store_id,idempotency_key,request_hash,total_cents,state) VALUES($1,$2,$3,$4,'confirmed') RETURNING id", store, key, digest, total).Scan(&id)
	if e != nil {
		return 0, e
	}
	for _, s := range snapshots {
		if _, e = tx.Exec(ctx, "INSERT INTO order_items(order_id,store_id,product_id,name,price_cents,quantity) VALUES($1,$2,$3,$4,$5,$6)", id, store, s.ProductID, s.Name, s.Price, s.Quantity); e != nil {
			return 0, e
		}
	}
	if _, e = tx.Exec(ctx, "INSERT INTO jobs(store_id,kind,payload,dedup_key) VALUES($1,'order_created',jsonb_build_object('order_id',$2::bigint),$3)", store, id, fmt.Sprintf("order:%d", id)); e != nil {
		return 0, e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO audit(store_id,user_id,action,entity_id) VALUES($1,$2,'order_created',$3)", store, user, id); e != nil {
		return 0, e
	}
	return id, tx.Commit(ctx)
}
func (a *App) orders(w http.ResponseWriter, r *http.Request, store, user int64) {
	if r.Method == "POST" {
		var input OrderRequest
		if decode(w, r, &input) != nil {
			failure(w, 400, "order_invalid")
			return
		}
		id, e := a.CreateOrder(r.Context(), store, user, r.Header.Get("Idempotency-Key"), input)
		if e != nil {
			failure(w, 409, e.Error())
			return
		}
		jsonResponse(w, 201, map[string]int64{"id": id})
		return
	}
	if r.Method != "GET" {
		failure(w, 405, "method_not_allowed")
		return
	}
	rows, e := a.DB.Query(r.Context(), "SELECT id,total_cents,state,financial_state,created_at FROM orders WHERE store_id=$1 ORDER BY id DESC LIMIT 200", store)
	if e != nil {
		failure(w, 500, "orders_failed")
		return
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var id, total int64
		var state, financial string
		var at time.Time
		if rows.Scan(&id, &total, &state, &financial, &at) != nil {
			failure(w, 500, "orders_failed")
			return
		}
		result = append(result, map[string]any{"id": id, "total_cents": total, "state": state, "financial_state": financial, "created_at": at})
	}
	jsonResponse(w, 200, result)
}
func Allowed(from, to string) bool {
	return (from == "confirmed" && (to == "preparing" || to == "cancelled")) || (from == "preparing" && to == "ready") || (from == "ready" && to == "completed")
}
func (a *App) orderState(w http.ResponseWriter, r *http.Request, store, user int64) {
	if r.Method != "POST" {
		failure(w, 405, "method_not_allowed")
		return
	}
	var input struct {
		ID    int64
		State string
	}
	if decode(w, r, &input) != nil {
		failure(w, 400, "input_invalid")
		return
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		failure(w, 503, "database_unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	var state string
	if e = tx.QueryRow(r.Context(), "SELECT state FROM orders WHERE store_id=$1 AND id=$2 FOR UPDATE", store, input.ID).Scan(&state); e != nil {
		failure(w, 404, "order_not_found")
		return
	}
	if !Allowed(state, input.State) {
		failure(w, 409, "transition_invalid")
		return
	}
	if input.State == "cancelled" {
		_, e = tx.Exec(r.Context(), "UPDATE products p SET stock=stock+i.quantity FROM (SELECT product_id,sum(quantity) quantity FROM order_items WHERE store_id=$1 AND order_id=$2 GROUP BY product_id) i WHERE p.store_id=$1 AND p.id=i.product_id", store, input.ID)
		if e != nil {
			failure(w, 500, "stock_release_failed")
			return
		}
	}
	_, e = tx.Exec(r.Context(), "UPDATE orders SET state=$3 WHERE store_id=$1 AND id=$2", store, input.ID, input.State)
	if e == nil {
		_, e = tx.Exec(r.Context(), "INSERT INTO audit(store_id,user_id,action,entity_id) VALUES($1,$2,$3,$4)", store, user, "order_"+input.State, input.ID)
	}
	if e == nil {
		e = tx.Commit(r.Context())
	}
	if e != nil {
		failure(w, 500, "transition_failed")
		return
	}
	jsonResponse(w, 200, map[string]bool{"ok": true})
}

// Process one durable job. Only local audit effects are currently enabled.
func (a *App) WorkOnce(ctx context.Context) error {
	a.gate.RLock()
	defer a.gate.RUnlock()
	var id, store int64
	var kind string
	e := a.DB.QueryRow(ctx, `WITH candidate AS (SELECT id FROM jobs WHERE (state='pending' AND available_at<=now()) OR (state='running' AND lease_until<now()) ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE jobs SET state='running',attempts=attempts+1,lease_until=now()+interval '60 seconds' FROM candidate WHERE jobs.id=candidate.id RETURNING jobs.id,store_id,kind`).Scan(&id, &store, &kind)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	// Audit and completion commit together; interrupted jobs are reclaimed after the lease.
	if kind != "order_created" {
		_, e = tx.Exec(ctx, "UPDATE jobs SET state='failed',last_error_code='unsupported_kind',lease_until=NULL WHERE id=$1", id)
	} else {
		_, e = tx.Exec(ctx, "INSERT INTO audit(store_id,action,entity_id) VALUES($1,'local_job_processed',$2)", store, id)
		if e == nil {
			_, e = tx.Exec(ctx, "UPDATE jobs SET state='done',lease_until=NULL WHERE id=$1", id)
		}
	}
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (a *App) Worker(ctx context.Context) {
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			call, cancel := context.WithTimeout(ctx, 10*time.Second)
			a.WorkOnce(call)
			cancel()
		}
	}
}
func (a *App) restore(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" || r.Header.Get("X-Confirm-Restore") != "RESTAURAR" {
		failure(w, 400, "restore_confirmation_required")
		return
	}
	if a.Ops == nil {
		failure(w, 501, "restore_not_configured")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 512<<20)
	var body []byte
	buf := make([]byte, 32768)
	for {
		n, e := r.Body.Read(buf)
		body = append(body, buf[:n]...)
		if e != nil {
			if e.Error() != "EOF" {
				failure(w, 400, "backup_invalid")
				return
			}
			break
		}
	}
	if e := a.Ops.Restore(r.Context(), body); e != nil {
		failure(w, 500, "restore_failed_previous_database_preserved")
		return
	}
	jsonResponse(w, 200, map[string]bool{"ok": true, "login_required": true})
}

func ParseInt(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n }
