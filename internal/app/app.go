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

const Version = "0.2.0-alpha.1"

// Roles supported by OpenFood RBAC
const (
	RoleInstanceAdmin = "instance_admin"
	RoleOrgAdmin      = "org_admin"
	RoleStoreManager  = "store_manager"
	RoleAttendant     = "attendant"
	RoleKitchen       = "kitchen"
	RoleDispatch      = "dispatch"
	RoleFinanceViewer = "finance_viewer"
)

var validRoles = map[string]bool{
	RoleInstanceAdmin: true,
	RoleOrgAdmin:      true,
	RoleStoreManager:  true,
	RoleAttendant:     true,
	RoleKitchen:       true,
	RoleDispatch:      true,
	RoleFinanceViewer: true,
}

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

type SessionUser struct {
	ID           int64
	OrgID        int64
	StoreID      *int64
	Email        string
	Role         string
	Active       bool
	TokenVersion int
}

func Token() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func Hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

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
		_, e = tx.Exec(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES(2,$1)", Hash(schema))
	} else if e == nil {
		if n == 1 {
			// Migration from v1 to v2 (Organizations, RBAC, Store scoping)
			migrationSQL := `
				CREATE TABLE IF NOT EXISTS organizations(id bigserial PRIMARY KEY, name text NOT NULL, created_at timestamptz NOT NULL DEFAULT now());
				INSERT INTO organizations(name) SELECT name FROM stores ORDER BY id LIMIT 1 ON CONFLICT DO NOTHING;
				DO $$
				DECLARE
					default_org bigint;
				BEGIN
					SELECT id INTO default_org FROM organizations ORDER BY id LIMIT 1;
					IF default_org IS NULL THEN
						INSERT INTO organizations(name) VALUES('Organização Principal') RETURNING id INTO default_org;
					END IF;

					IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='stores' AND column_name='org_id') THEN
						ALTER TABLE stores ADD COLUMN org_id bigint REFERENCES organizations(id) ON DELETE CASCADE;
						UPDATE stores SET org_id = default_org WHERE org_id IS NULL;
						ALTER TABLE stores ALTER COLUMN org_id SET NOT NULL;
						ALTER TABLE stores ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now();
					END IF;

					IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='org_id') THEN
						ALTER TABLE users ADD COLUMN org_id bigint REFERENCES organizations(id) ON DELETE CASCADE;
						UPDATE users SET org_id = default_org WHERE org_id IS NULL;
						ALTER TABLE users ALTER COLUMN org_id SET NOT NULL;
					END IF;

					IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='role') THEN
						ALTER TABLE users ADD COLUMN role text NOT NULL DEFAULT 'instance_admin';
						ALTER TABLE users ADD CONSTRAINT users_role_check CHECK(role IN ('instance_admin','org_admin','store_manager','attendant','kitchen','dispatch','finance_viewer'));
					END IF;

					IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='active') THEN
						ALTER TABLE users ADD COLUMN active boolean NOT NULL DEFAULT true;
					END IF;

					IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='token_version') THEN
						ALTER TABLE users ADD COLUMN token_version integer NOT NULL DEFAULT 1;
					END IF;

					IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='created_at') THEN
						ALTER TABLE users ADD COLUMN created_at timestamptz NOT NULL DEFAULT now();
					END IF;

					IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='sessions' AND column_name='token_version') THEN
						ALTER TABLE sessions ADD COLUMN token_version integer NOT NULL DEFAULT 1;
					END IF;
				END $$;
			`
			if _, e = tx.Exec(ctx, migrationSQL); e != nil {
				return fmt.Errorf("migration_v1_to_v2_failed: %w", e)
			}
			_, e = tx.Exec(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES(2,$1)", Hash(schema))
			if e != nil {
				return e
			}
		} else if n != 2 || checksum != Hash(schema) {
			return errors.New("migration_incompatible")
		}
	}
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}

func ValidateSchema(version int, checksum string) error {
	if version != 2 || checksum != Hash(schema) {
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

func (a *App) authenticateSession(ctx context.Context, cookieValue string) (*SessionUser, error) {
	var u SessionUser
	query := `
		SELECT u.id, u.org_id, u.store_id, u.email, u.role, u.active, u.token_version
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1
		  AND s.expires_at > now()
		  AND s.token_version = u.token_version
		  AND u.active = true
	`
	e := a.DB.QueryRow(ctx, query, Hash(cookieValue)).Scan(
		&u.ID, &u.OrgID, &u.StoreID, &u.Email, &u.Role, &u.Active, &u.TokenVersion,
	)
	if e != nil {
		return nil, e
	}
	return &u, nil
}

func (u *SessionUser) CanAccessStore(targetStore int64) bool {
	if !u.Active {
		return false
	}
	if u.Role == RoleInstanceAdmin || u.Role == RoleOrgAdmin {
		return true
	}
	return u.StoreID != nil && *u.StoreID == targetStore
}

func (u *SessionUser) CanManageCatalog() bool {
	return u.Role == RoleInstanceAdmin || u.Role == RoleOrgAdmin || u.Role == RoleStoreManager
}

func (u *SessionUser) CanCreateOrders() bool {
	return u.Role == RoleInstanceAdmin || u.Role == RoleOrgAdmin || u.Role == RoleStoreManager || u.Role == RoleAttendant
}

func (u *SessionUser) CanManageUsers() bool {
	return u.Role == RoleInstanceAdmin || u.Role == RoleOrgAdmin || u.Role == RoleStoreManager
}

func (u *SessionUser) CanTransitionState(targetState string) bool {
	switch u.Role {
	case RoleInstanceAdmin, RoleOrgAdmin, RoleStoreManager:
		return true
	case RoleAttendant:
		return targetState == "preparing" || targetState == "cancelled"
	case RoleKitchen:
		return targetState == "preparing" || targetState == "ready"
	case RoleDispatch:
		return targetState == "completed"
	default:
		return false
	}
}

func (u *SessionUser) CanViewFinancials() bool {
	return u.Role == RoleInstanceAdmin || u.Role == RoleOrgAdmin || u.Role == RoleStoreManager || u.Role == RoleFinanceViewer
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

		user, e := a.authenticateSession(ctx, cookie.Value)
		if e != nil {
			failure(w, 401, "session_invalid")
			return
		}

		// Determine target store context (header or user default store)
		targetStore := int64(0)
		if storeHeader := r.Header.Get("X-Store-ID"); storeHeader != "" {
			targetStore = ParseInt(storeHeader)
		} else if user.StoreID != nil {
			targetStore = *user.StoreID
		} else {
			// Find first accessible store for org
			a.DB.QueryRow(ctx, "SELECT id FROM stores WHERE org_id=$1 ORDER BY id LIMIT 1", user.OrgID).Scan(&targetStore)
		}

		switch r.URL.Path {
		case "/api/me":
			if r.Method != "GET" {
				failure(w, 405, "method_not_allowed")
				return
			}
			jsonResponse(w, 200, map[string]any{
				"id":       user.ID,
				"email":    user.Email,
				"role":     user.Role,
				"org_id":   user.OrgID,
				"store_id": user.StoreID,
			})
		case "/api/shutdown":
			if r.Method != "POST" {
				failure(w, 405, "method_not_allowed")
				return
			}
			if user.Role != RoleInstanceAdmin {
				failure(w, 403, "forbidden_instance_admin_required")
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
		case "/api/stores":
			a.storesHandler(w, r, user)
		case "/api/users":
			a.usersHandler(w, r, user)
		case "/api/status":
			if targetStore > 0 && !user.CanAccessStore(targetStore) {
				failure(w, 403, "forbidden_cross_store_access")
				return
			}
			var name string
			var pending, failed int
			a.DB.QueryRow(ctx, "SELECT name FROM stores WHERE id=$1", targetStore).Scan(&name)
			a.DB.QueryRow(ctx, "SELECT count(*) FILTER(WHERE state IN ('pending','running')),count(*) FILTER(WHERE state='failed') FROM jobs WHERE store_id=$1", targetStore).Scan(&pending, &failed)
			jsonResponse(w, 200, map[string]any{
				"store":           name,
				"store_id":        targetStore,
				"version":         Version,
				"user_role":       user.Role,
				"pending_jobs":    pending,
				"failed_jobs":     failed,
				"integrations":    "disabled",
				"public_webhooks": "not_configured",
				"local_only":      true,
			})
		case "/api/products":
			if !user.CanAccessStore(targetStore) {
				failure(w, 403, "forbidden_cross_store_access")
				return
			}
			a.products(w, r, targetStore, user)
		case "/api/orders":
			if !user.CanAccessStore(targetStore) {
				failure(w, 403, "forbidden_cross_store_access")
				return
			}
			a.orders(w, r, targetStore, user)
		case "/api/order-state":
			if !user.CanAccessStore(targetStore) {
				failure(w, 403, "forbidden_cross_store_access")
				return
			}
			a.orderState(w, r, targetStore, user)
		case "/api/backup":
			if r.Method != "POST" {
				failure(w, 405, "method_not_allowed")
				return
			}
			if user.Role != RoleInstanceAdmin && user.Role != RoleOrgAdmin {
				failure(w, 403, "forbidden_admin_required")
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
			if user.Role != RoleInstanceAdmin {
				failure(w, 403, "forbidden_instance_admin_required")
				return
			}
			a.restore(w, r)
		case "/api/diagnostics":
			if user.Role != RoleInstanceAdmin && user.Role != RoleOrgAdmin {
				failure(w, 403, "forbidden_admin_required")
				return
			}
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
	var input struct {
		Token        string `json:"token"`
		Email        string `json:"email"`
		Password     string `json:"password"`
		Store        string `json:"store"`
		Organization string `json:"organization"`
	}
	if decode(w, r, &input) != nil || len(input.Password) < 12 || len(input.Password) > 72 || len(input.Store) < 1 || len(input.Store) > 120 || !strings.Contains(input.Email, "@") {
		failure(w, 400, "setup_input_invalid")
		return
	}
	if a.SetupToken == "" || Hash(input.Token) != Hash(a.SetupToken) {
		failure(w, 403, "setup_token_invalid")
		return
	}
	orgName := input.Organization
	if orgName == "" {
		orgName = input.Store + " Org"
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
	var orgID, storeID int64
	e = tx.QueryRow(r.Context(), "INSERT INTO organizations(name) VALUES($1) RETURNING id", orgName).Scan(&orgID)
	if e == nil {
		e = tx.QueryRow(r.Context(), "INSERT INTO stores(org_id,name) VALUES($1,$2) RETURNING id", orgID, input.Store).Scan(&storeID)
	}
	if e == nil {
		_, e = tx.Exec(r.Context(), "INSERT INTO users(org_id,store_id,email,password_hash,role,active) VALUES($1,$2,$3,$4,$5,true)",
			orgID, storeID, strings.ToLower(input.Email), string(hash), RoleInstanceAdmin)
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
	var active bool
	var tokenVersion int
	e := a.DB.QueryRow(r.Context(), "SELECT id,password_hash,active,token_version FROM users WHERE email=$1", key).Scan(&id, &hash, &active, &tokenVersion)
	if e != nil || !active || bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.Password)) != nil {
		failure(w, 401, "credentials_invalid")
		return
	}
	token := Token()
	_, e = a.DB.Exec(r.Context(), "INSERT INTO sessions(token_hash,user_id,token_version,expires_at) VALUES($1,$2,$3,now()+interval '12 hours')", Hash(token), id, tokenVersion)
	if e != nil {
		failure(w, 500, "session_failed")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "openfood_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 43200})
	jsonResponse(w, 200, map[string]bool{"ok": true})
}

func (a *App) storesHandler(w http.ResponseWriter, r *http.Request, u *SessionUser) {
	if r.Method == "GET" {
		var rows pgx.Rows
		var e error
		if u.Role == RoleInstanceAdmin {
			rows, e = a.DB.Query(r.Context(), "SELECT id, org_id, name, created_at FROM stores ORDER BY id")
		} else if u.Role == RoleOrgAdmin {
			rows, e = a.DB.Query(r.Context(), "SELECT id, org_id, name, created_at FROM stores WHERE org_id=$1 ORDER BY id", u.OrgID)
		} else if u.StoreID != nil {
			rows, e = a.DB.Query(r.Context(), "SELECT id, org_id, name, created_at FROM stores WHERE id=$1", *u.StoreID)
		} else {
			failure(w, 403, "no_store_assigned")
			return
		}
		if e != nil {
			failure(w, 500, "stores_query_failed")
			return
		}
		defer rows.Close()
		stores := []map[string]any{}
		for rows.Next() {
			var id, orgID int64
			var name string
			var createdAt time.Time
			if rows.Scan(&id, &orgID, &name, &createdAt) != nil {
				failure(w, 500, "stores_scan_failed")
				return
			}
			stores = append(stores, map[string]any{
				"id":         id,
				"org_id":     orgID,
				"name":       name,
				"created_at": createdAt,
			})
		}
		jsonResponse(w, 200, stores)
		return
	}
	if r.Method == "POST" {
		if u.Role != RoleInstanceAdmin && u.Role != RoleOrgAdmin {
			failure(w, 403, "forbidden_admin_required")
			return
		}
		var input struct {
			Name  string `json:"name"`
			OrgID *int64 `json:"org_id"`
		}
		if decode(w, r, &input) != nil || len(input.Name) < 1 || len(input.Name) > 120 {
			failure(w, 400, "store_name_invalid")
			return
		}
		targetOrg := u.OrgID
		if u.Role == RoleInstanceAdmin && input.OrgID != nil {
			targetOrg = *input.OrgID
		}
		var storeID int64
		e := a.DB.QueryRow(r.Context(), "INSERT INTO stores(org_id, name) VALUES($1, $2) RETURNING id", targetOrg, input.Name).Scan(&storeID)
		if e != nil {
			failure(w, 500, "store_creation_failed")
			return
		}
		jsonResponse(w, 201, map[string]int64{"id": storeID})
		return
	}
	failure(w, 405, "method_not_allowed")
}

func (a *App) usersHandler(w http.ResponseWriter, r *http.Request, u *SessionUser) {
	if !u.CanManageUsers() {
		failure(w, 403, "forbidden_insufficient_role")
		return
	}
	if r.Method == "GET" {
		var rows pgx.Rows
		var e error
		if u.Role == RoleInstanceAdmin {
			rows, e = a.DB.Query(r.Context(), "SELECT id, org_id, store_id, email, role, active, created_at FROM users ORDER BY id")
		} else if u.Role == RoleOrgAdmin {
			rows, e = a.DB.Query(r.Context(), "SELECT id, org_id, store_id, email, role, active, created_at FROM users WHERE org_id=$1 ORDER BY id", u.OrgID)
		} else if u.Role == RoleStoreManager && u.StoreID != nil {
			rows, e = a.DB.Query(r.Context(), "SELECT id, org_id, store_id, email, role, active, created_at FROM users WHERE store_id=$1 ORDER BY id", *u.StoreID)
		} else {
			failure(w, 403, "forbidden")
			return
		}
		if e != nil {
			failure(w, 500, "users_query_failed")
			return
		}
		defer rows.Close()
		users := []map[string]any{}
		for rows.Next() {
			var id, orgID int64
			var storeID *int64
			var email, role string
			var active bool
			var createdAt time.Time
			if rows.Scan(&id, &orgID, &storeID, &email, &role, &active, &createdAt) != nil {
				failure(w, 500, "users_scan_failed")
				return
			}
			users = append(users, map[string]any{
				"id":         id,
				"org_id":     orgID,
				"store_id":   storeID,
				"email":      email,
				"role":       role,
				"active":     active,
				"created_at": createdAt,
			})
		}
		jsonResponse(w, 200, users)
		return
	}
	if r.Method == "POST" {
		var input struct {
			Email    string `json:"email"`
			Password string `json:"password"`
			Role     string `json:"role"`
			StoreID  *int64 `json:"store_id"`
			OrgID    *int64 `json:"org_id"`
		}
		if decode(w, r, &input) != nil || len(input.Password) < 12 || len(input.Password) > 72 || !strings.Contains(input.Email, "@") || !validRoles[input.Role] {
			failure(w, 400, "user_input_invalid")
			return
		}
		// Authorization checks for creating roles
		if u.Role == RoleStoreManager {
			if input.Role == RoleInstanceAdmin || input.Role == RoleOrgAdmin || input.Role == RoleStoreManager {
				failure(w, 403, "store_manager_cannot_create_admins")
				return
			}
			input.StoreID = u.StoreID
			input.OrgID = &u.OrgID
		} else if u.Role == RoleOrgAdmin {
			if input.Role == RoleInstanceAdmin {
				failure(w, 403, "org_admin_cannot_create_instance_admin")
				return
			}
			input.OrgID = &u.OrgID
		}
		targetOrg := u.OrgID
		if u.Role == RoleInstanceAdmin && input.OrgID != nil {
			targetOrg = *input.OrgID
		}

		hash, e := bcrypt.GenerateFromPassword([]byte(input.Password), 12)
		if e != nil {
			failure(w, 500, "password_hash_failed")
			return
		}
		var newID int64
		e = a.DB.QueryRow(r.Context(), `
			INSERT INTO users(org_id, store_id, email, password_hash, role, active)
			VALUES($1, $2, $3, $4, $5, true)
			RETURNING id
		`, targetOrg, input.StoreID, strings.ToLower(input.Email), string(hash), input.Role).Scan(&newID)
		if e != nil {
			if strings.Contains(e.Error(), "unique") {
				failure(w, 409, "email_already_registered")
				return
			}
			failure(w, 500, "user_creation_failed")
			return
		}
		jsonResponse(w, 201, map[string]int64{"id": newID})
		return
	}
	if r.Method == "PATCH" {
		var input struct {
			ID     int64   `json:"id"`
			Active *bool   `json:"active"`
			Role   *string `json:"role"`
		}
		if decode(w, r, &input) != nil || input.ID <= 0 {
			failure(w, 400, "user_patch_invalid")
			return
		}
		if input.Role != nil && !validRoles[*input.Role] {
			failure(w, 400, "role_invalid")
			return
		}
		// Check target user's org/store
		var targetOrg int64
		var targetStore *int64
		var targetRole string
		e := a.DB.QueryRow(r.Context(), "SELECT org_id, store_id, role FROM users WHERE id=$1", input.ID).Scan(&targetOrg, &targetStore, &targetRole)
		if e != nil {
			failure(w, 404, "user_not_found")
			return
		}
		if u.Role == RoleOrgAdmin && targetOrg != u.OrgID {
			failure(w, 403, "forbidden_cross_org")
			return
		}
		if u.Role == RoleStoreManager && (targetStore == nil || *targetStore != *u.StoreID) {
			failure(w, 403, "forbidden_cross_store")
			return
		}

		tx, e := a.DB.Begin(r.Context())
		if e != nil {
			failure(w, 500, "database_unavailable")
			return
		}
		defer tx.Rollback(r.Context())

		// Increment token_version to immediately revoke existing active sessions for this user
		if input.Active != nil && input.Role != nil {
			_, e = tx.Exec(r.Context(), "UPDATE users SET active=$1, role=$2, token_version=token_version+1 WHERE id=$3", *input.Active, *input.Role, input.ID)
		} else if input.Active != nil {
			_, e = tx.Exec(r.Context(), "UPDATE users SET active=$1, token_version=token_version+1 WHERE id=$2", *input.Active, input.ID)
		} else if input.Role != nil {
			_, e = tx.Exec(r.Context(), "UPDATE users SET role=$1, token_version=token_version+1 WHERE id=$2", *input.Role, input.ID)
		}

		if e == nil {
			e = tx.Commit(r.Context())
		}
		if e != nil {
			failure(w, 500, "user_update_failed")
			return
		}
		jsonResponse(w, 200, map[string]bool{"ok": true})
		return
	}
	failure(w, 405, "method_not_allowed")
}

func (a *App) products(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
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
	if !user.CanManageCatalog() {
		failure(w, 403, "forbidden_catalog_management_required")
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

func (a *App) orders(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if r.Method == "POST" {
		if !user.CanCreateOrders() {
			failure(w, 403, "forbidden_order_creation_required")
			return
		}
		var input OrderRequest
		if decode(w, r, &input) != nil {
			failure(w, 400, "order_invalid")
			return
		}
		id, e := a.CreateOrder(r.Context(), store, user.ID, r.Header.Get("Idempotency-Key"), input)
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
		item := map[string]any{"id": id, "total_cents": total, "state": state, "financial_state": financial, "created_at": at}
		if !user.CanViewFinancials() {
			delete(item, "financial_state")
		}
		result = append(result, item)
	}
	jsonResponse(w, 200, result)
}

func Allowed(from, to string) bool {
	return (from == "confirmed" && (to == "preparing" || to == "cancelled")) || (from == "preparing" && to == "ready") || (from == "ready" && to == "completed")
}

func (a *App) orderState(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
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
	if !user.CanTransitionState(input.State) {
		failure(w, 403, "forbidden_role_transition")
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
		_, e = tx.Exec(r.Context(), "INSERT INTO audit(store_id,user_id,action,entity_id) VALUES($1,$2,$3,$4)", store, user.ID, "order_"+input.State, input.ID)
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

func ParseInt(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
