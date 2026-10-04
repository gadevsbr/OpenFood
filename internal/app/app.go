package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
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

// Payment types and interfaces
type PaymentCapabilities struct {
	SupportsRefund        bool
	SupportsPartialRefund bool
	SupportsWebhook       bool
	SupportsPixStatic     bool
	SupportsPixDynamic    bool
	MaxExpirationHours    int
	MinAmountCents        int64
	MaxAmountCents        int64
}

type ChargeRequest struct {
	OrderID          int64
	AmountCents      int64
	Description      string
	PayerName        string
	PayerEmail       string
	PayerPhone       string
	PayerDocument    string
	IdempotencyKey   string
	ExpirationMinutes int
	Metadata         map[string]string
}

type ChargeResponse struct {
	PaymentID       string
	ExternalID      string
	QRCode          string
	PixCopyPaste    string
	ExpiresAt       time.Time
	Status          string
	Provider        string
	AmountCents     int64
}

type PaymentStatus struct {
	PaymentID       string
	ExternalID      string
	Status          string
	AmountCents     int64
	PaidAt          *time.Time
	ProviderData    map[string]any
}

type RefundRequest struct {
	PaymentID     string
	AmountCents   int64
	Reason        string
	IdempotencyKey string
}

type RefundResponse struct {
	RefundID      string
	ExternalID    string
	Status        string
	AmountCents   int64
}

type PaymentConnector interface {
	// CreateCharge creates a new Pix charge
	CreateCharge(ctx context.Context, req ChargeRequest) (*ChargeResponse, error)
	
	// GetChargeStatus checks the status of a payment
	GetChargeStatus(ctx context.Context, paymentID string) (*PaymentStatus, error)
	
	// ProcessWebhook processes an incoming webhook notification
	ProcessWebhook(ctx context.Context, payload []byte, headers http.Header) (*PaymentStatus, error)
	
	// Refund processes a refund request
	Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error)
	
	// GetRefundStatus checks the status of a refund
	GetRefundStatus(ctx context.Context, refundID string) (*RefundResponse, error)
	
	// Capabilities returns the connector's capabilities
	Capabilities() PaymentCapabilities
	
	// ProviderName returns the provider identifier
	ProviderName() string
}

// PaymentConfig holds configuration for a payment connector
type PaymentConfig struct {
	Provider        string                 `json:"provider"`
	Enabled         bool                   `json:"enabled"`
	Credentials     map[string]string      `json:"credentials"`
	StoreID         int64                  `json:"store_id"`
	WebhookSecret   string                 `json:"webhook_secret"`
	Settings        map[string]any         `json:"settings"`
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
		fmt.Printf("MIGRATE: Fresh DB detected (ErrNoRows), n=%d, checksum=%s\n", n, checksum)
		if _, e = tx.Exec(ctx, schema); e != nil {
			fmt.Printf("MIGRATE: schema exec error: %v\n", e)
			return e
		}
		fmt.Printf("MIGRATE: schema exec OK\n")
		_, e = tx.Exec(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES(3,$1)", Hash(schema))
		if e != nil {
			fmt.Printf("MIGRATE: insert version error: %v\n", e)
			return e
		}
		fmt.Printf("MIGRATE: insert version OK\n")
		fmt.Printf("MIGRATE: Fresh DB setup complete, committing and returning\n")
		return tx.Commit(ctx)
	} else if e == nil {
		fmt.Printf("MIGRATE: Existing DB detected, version=%d, checksum=%s\n", n, checksum)
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
					END IF.

					IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='stores' AND column_name='org_id') THEN
						ALTER TABLE stores ADD COLUMN org_id bigint REFERENCES organizations(id) ON DELETE CASCADE;
						UPDATE stores SET org_id = default_org WHERE org_id IS NULL;
						ALTER TABLE stores ALTER COLUMN org_id SET NOT NULL;
						ALTER TABLE stores ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now();
					END IF.

					IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='org_id') THEN
						ALTER TABLE users ADD COLUMN org_id bigint REFERENCES organizations(id) ON DELETE CASCADE;
						UPDATE users SET org_id = default_org WHERE org_id IS NULL;
						ALTER TABLE users ALTER COLUMN org_id SET NOT NULL.
					END IF.

					IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='role') THEN
						ALTER TABLE users ADD COLUMN role text NOT NULL DEFAULT 'instance_admin';
						ALTER TABLE users ADD CONSTRAINT users_role_check CHECK(role IN ('instance_admin','org_admin','store_manager','attendant','kitchen','dispatch','finance_viewer'));
					END IF.

					IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='active') THEN
						ALTER TABLE users ADD COLUMN active boolean NOT NULL DEFAULT true;
					END IF.

					IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='token_version') THEN
						ALTER TABLE users ADD COLUMN token_version integer NOT NULL DEFAULT 1;
					END IF.

					IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='created_at') THEN
						ALTER TABLE users ADD COLUMN created_at timestamptz NOT NULL DEFAULT now();
					END IF.

					IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='sessions' AND column_name='token_version') THEN
						ALTER TABLE sessions ADD COLUMN token_version integer NOT NULL DEFAULT 1;
					END IF.
				END $$;
			`
			if _, e = tx.Exec(ctx, migrationSQL); e != nil {
				return fmt.Errorf("migration_v1_to_v2_failed: %w", e)
			}
			_, e = tx.Exec(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES(2,$1)", Hash(schema))
			if e != nil {
				return e
			}
			n = 2
		}
		if n == 2 {
			// Migration from v2 to v3 (Advanced Catalog, Customers, Consents, Payments)
			migrationSQL := `
				-- Categories
				CREATE TABLE IF NOT EXISTS categories(
					id bigserial PRIMARY KEY,
					store_id bigint NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
					parent_id bigint REFERENCES categories(id) ON DELETE SET NULL,
					name text NOT NULL,
					description text,
					display_order integer NOT NULL DEFAULT 0,
					active boolean NOT NULL DEFAULT true,
					created_at timestamptz NOT NULL DEFAULT now(),
					UNIQUE(store_id, parent_id, name)
				);

				-- Products (replace old products table)
				-- First check if old products table exists with old schema
				DO $$
				BEGIN
					IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='products' AND column_name='stock') AND
					   NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='products' AND column_name='stock_quantity') THEN
						ALTER TABLE products RENAME COLUMN stock TO stock_quantity;
						ALTER TABLE products ALTER COLUMN stock_quantity TYPE numeric(12,3) USING stock_quantity::numeric(12,3);
						ALTER TABLE products ALTER COLUMN stock_quantity SET DEFAULT 0;
						ALTER TABLE products ADD COLUMN IF NOT EXISTS category_id bigint REFERENCES categories(id) ON DELETE SET NULL;
						ALTER TABLE products ADD COLUMN IF NOT EXISTS description text;
						ALTER TABLE products ADD COLUMN IF NOT EXISTS unit text NOT NULL DEFAULT 'unidade' CHECK(unit IN ('unidade','kg','g','l','ml','porcao'));
						ALTER TABLE products ADD COLUMN IF NOT EXISTS sku text;
						ALTER TABLE products ADD COLUMN IF NOT EXISTS barcode text;
						ALTER TABLE products ADD COLUMN IF NOT EXISTS active boolean NOT NULL DEFAULT true;
						ALTER TABLE products ADD COLUMN IF NOT EXISTS available_from time;
						ALTER TABLE products ADD COLUMN IF NOT EXISTS available_until time;
						ALTER TABLE products ADD COLUMN IF NOT EXISTS min_order_quantity numeric(12,3) NOT NULL DEFAULT 1 CHECK(min_order_quantity > 0);
						ALTER TABLE products ADD COLUMN IF NOT EXISTS max_order_quantity numeric(12,3) CHECK(max_order_quantity >= min_order_quantity);
						ALTER TABLE products ADD COLUMN IF NOT EXISTS is_weight_based boolean NOT NULL DEFAULT false;
						ALTER TABLE products ADD COLUMN IF NOT EXISTS substitution_allowed boolean NOT NULL DEFAULT true;
						ALTER TABLE products ADD COLUMN IF NOT EXISTS preparation_time_minutes integer NOT NULL DEFAULT 0;
						ALTER TABLE products ADD COLUMN IF NOT EXISTS display_order integer NOT NULL DEFAULT 0;
						ALTER TABLE products ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();
						CREATE UNIQUE INDEX IF NOT EXISTS idx_products_store_sku ON products(store_id, sku) WHERE sku IS NOT NULL;
					END IF;
				END $$;

				-- Product variants
				CREATE TABLE IF NOT EXISTS product_variants(
					id bigserial PRIMARY KEY,
					product_id bigint NOT NULL REFERENCES products(id) ON DELETE CASCADE,
					store_id bigint NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
					name text NOT NULL,
					price_adjustment_cents bigint NOT NULL DEFAULT 0,
					stock_quantity numeric(12,3) NOT NULL DEFAULT 0 CHECK(stock_quantity >= 0),
					sku text,
					display_order integer NOT NULL DEFAULT 0,
					active boolean NOT NULL DEFAULT true,
					UNIQUE(product_id, name)
				);

				-- Addon groups
				CREATE TABLE IF NOT EXISTS addon_groups(
					id bigserial PRIMARY KEY,
					store_id bigint NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
					name text NOT NULL,
					description text,
					min_selections integer NOT NULL DEFAULT 0 CHECK(min_selections >= 0),
					max_selections integer NOT NULL DEFAULT 1 CHECK(max_selections >= min_selections),
					required boolean NOT NULL DEFAULT false,
					display_order integer NOT NULL DEFAULT 0,
					active boolean NOT NULL DEFAULT true,
					UNIQUE(store_id, name)
				);

				-- Addon options
				CREATE TABLE IF NOT EXISTS addon_options(
					id bigserial PRIMARY KEY,
					group_id bigint NOT NULL REFERENCES addon_groups(id) ON DELETE CASCADE,
					store_id bigint NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
					name text NOT NULL,
					description text,
					price_cents bigint NOT NULL DEFAULT 0 CHECK(price_cents >= 0),
					stock_quantity numeric(12,3) NOT NULL DEFAULT 0 CHECK(stock_quantity >= 0),
					is_default boolean NOT NULL DEFAULT false,
					display_order integer NOT NULL DEFAULT 0,
					active boolean NOT NULL DEFAULT true,
					UNIQUE(group_id, name)
				);

				-- Product addon groups
				CREATE TABLE IF NOT EXISTS product_addon_groups(
					product_id bigint NOT NULL REFERENCES products(id) ON DELETE CASCADE,
					store_id bigint NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
					group_id bigint NOT NULL REFERENCES addon_groups(id) ON DELETE CASCADE,
					display_order integer NOT NULL DEFAULT 0,
					PRIMARY KEY(product_id, group_id)
				);

				-- Combos
				CREATE TABLE IF NOT EXISTS combos(
					id bigserial PRIMARY KEY,
					store_id bigint NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
					name text NOT NULL,
					description text,
					price_cents bigint NOT NULL CHECK(price_cents >= 0),
					pricing_policy text NOT NULL DEFAULT 'fixed' CHECK(pricing_policy IN ('fixed','highest','sum')),
					active boolean NOT NULL DEFAULT true,
					display_order integer NOT NULL DEFAULT 0,
					created_at timestamptz NOT NULL DEFAULT now(),
					UNIQUE(store_id, name)
				);

				-- Combo items
				CREATE TABLE IF NOT EXISTS combo_items(
					combo_id bigint NOT NULL REFERENCES combos(id) ON DELETE CASCADE,
					store_id bigint NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
					product_id bigint NOT NULL REFERENCES products(id) ON DELETE CASCADE,
					quantity numeric(12,3) NOT NULL DEFAULT 1 CHECK(quantity > 0),
					is_optional boolean NOT NULL DEFAULT false,
					group_name text,
					min_selections integer NOT NULL DEFAULT 0 CHECK(min_selections >= 0),
					max_selections integer NOT NULL DEFAULT 1 CHECK(max_selections >= min_selections),
					display_order integer NOT NULL DEFAULT 0,
					PRIMARY KEY(combo_id, product_id)
				);

				-- Product images
				CREATE TABLE IF NOT EXISTS product_images(
					id bigserial PRIMARY KEY,
					product_id bigint NOT NULL REFERENCES products(id) ON DELETE CASCADE,
					store_id bigint NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
					url text NOT NULL,
					alt_text text,
					display_order integer NOT NULL DEFAULT 0,
					is_primary boolean NOT NULL DEFAULT false,
					created_at timestamptz NOT NULL DEFAULT now()
				);

				-- Update orders table
				DO $$
				BEGIN
					IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='orders' AND column_name='state') AND
					   NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='orders' AND column_name='customer_id') THEN
						ALTER TABLE orders RENAME COLUMN state TO old_state;
						ALTER TABLE orders ADD COLUMN state text NOT NULL DEFAULT 'draft' CHECK(state IN ('draft','awaiting_confirmation','awaiting_payment','confirmed','preparing','ready','completed','cancelled'));
						UPDATE orders SET state = CASE
							WHEN old_state = 'confirmed' THEN 'confirmed'
							WHEN old_state = 'preparing' THEN 'preparing'
							WHEN old_state = 'ready' THEN 'ready'
							WHEN old_state = 'completed' THEN 'completed'
							WHEN old_state = 'cancelled' THEN 'cancelled'
							ELSE 'confirmed'
						END;
						ALTER TABLE orders DROP COLUMN old_state;

						ALTER TABLE orders ALTER COLUMN financial_state TYPE text;
						ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_financial_state_check;
						ALTER TABLE orders ADD CONSTRAINT orders_financial_state_check CHECK(financial_state IN ('manual_pending','manual_confirmed','payment_pending','payment_confirmed','payment_expired','refund_pending','refund_partial','refunded'));
						ALTER TABLE orders ALTER COLUMN financial_state SET DEFAULT 'manual_pending';

						ALTER TABLE orders ADD COLUMN IF NOT EXISTS customer_id bigint;
						ALTER TABLE orders ADD COLUMN IF NOT EXISTS customer_name text;
						ALTER TABLE orders ADD COLUMN IF NOT EXISTS customer_phone text;
						ALTER TABLE orders ADD COLUMN IF NOT EXISTS customer_email text;
						ALTER TABLE orders ADD COLUMN IF NOT EXISTS delivery_type text CHECK(delivery_type IN ('takeaway','delivery'));
						ALTER TABLE orders ADD COLUMN IF NOT EXISTS delivery_address_id bigint;
						ALTER TABLE orders ADD COLUMN IF NOT EXISTS delivery_fee_cents bigint NOT NULL DEFAULT 0;
						ALTER TABLE orders ADD COLUMN IF NOT EXISTS notes text;
						ALTER TABLE orders ADD COLUMN IF NOT EXISTS confirmed_at timestamptz;
					END IF;
				END $$;

				-- Order items (replace old order_items table)
				DO $$
				BEGIN
					IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='order_items' AND column_name='quantity') AND
					   NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='order_items' AND column_name='variant_id') THEN
						ALTER TABLE order_items ADD COLUMN IF NOT EXISTS id bigserial PRIMARY KEY;
						ALTER TABLE order_items ADD COLUMN IF NOT EXISTS variant_id bigint;
						ALTER TABLE order_items ADD COLUMN IF NOT EXISTS unit text NOT NULL DEFAULT 'unidade';
						ALTER TABLE order_items ADD COLUMN IF NOT EXISTS notes text;
						ALTER TABLE order_items ALTER COLUMN quantity TYPE numeric(12,3) USING quantity::numeric(12,3);
						ALTER TABLE order_items ADD CHECK (quantity > 0);
					END IF;
				END $$;

				-- Order item addons
				CREATE TABLE IF NOT EXISTS order_item_addons(
					id bigserial PRIMARY KEY,
					order_item_id bigint NOT NULL REFERENCES order_items(id) ON DELETE CASCADE,
					store_id bigint NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
					addon_group_id bigint NOT NULL REFERENCES addon_groups(id) ON DELETE CASCADE,
					addon_option_id bigint NOT NULL REFERENCES addon_options(id) ON DELETE CASCADE,
					price_cents bigint NOT NULL DEFAULT 0,
					quantity numeric(12,3) NOT NULL DEFAULT 1 CHECK(quantity > 0)
				);

				-- Customers
				CREATE TABLE IF NOT EXISTS customers(
					id bigserial PRIMARY KEY,
					store_id bigint NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
					name text NOT NULL,
					phone text,
					email text,
					document text,
					notes text,
					marketing_consent boolean NOT NULL DEFAULT false,
					marketing_consent_at timestamptz,
					created_at timestamptz NOT NULL DEFAULT now(),
					updated_at timestamptz NOT NULL DEFAULT now()
				);
				CREATE UNIQUE INDEX IF NOT EXISTS idx_customers_store_phone ON customers(store_id, phone) WHERE phone IS NOT NULL;
				CREATE UNIQUE INDEX IF NOT EXISTS idx_customers_store_email ON customers(store_id, email) WHERE email IS NOT NULL;

				-- Consents
				CREATE TABLE IF NOT EXISTS consents(
					id bigserial PRIMARY KEY,
					store_id bigint NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
					customer_id bigint NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
					purpose text NOT NULL,
					version text NOT NULL,
					granted boolean NOT NULL DEFAULT false,
					granted_at timestamptz,
					revoked_at timestamptz,
					source text NOT NULL,
					evidence jsonb,
					UNIQUE(store_id, customer_id, purpose, version)
				);

				-- Payments
				CREATE TABLE IF NOT EXISTS payments(
					id bigserial PRIMARY KEY,
					store_id bigint NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
					order_id bigint NOT NULL,
					provider text NOT NULL,
					external_id text NOT NULL,
					amount_cents bigint NOT NULL CHECK(amount_cents > 0),
					currency text NOT NULL DEFAULT 'BRL',
					status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','confirmed','expired','cancelled','refund_pending','refunded')),
					idempotency_key text NOT NULL,
					qr_code text,
					pix_copy_paste text,
					expires_at timestamptz,
					confirmed_at timestamptz,
					refunded_amount_cents bigint NOT NULL DEFAULT 0,
					raw_webhook jsonb,
					created_at timestamptz NOT NULL DEFAULT now(),
					updated_at timestamptz NOT NULL DEFAULT now(),
					UNIQUE(store_id, provider, external_id),
					UNIQUE(store_id, order_id, idempotency_key)
				);

				-- Indexes
				CREATE INDEX IF NOT EXISTS idx_products_store_category ON products(store_id, category_id);
				CREATE INDEX IF NOT EXISTS idx_products_store_active ON products(store_id, active) WHERE active = true;
				CREATE INDEX IF NOT EXISTS idx_categories_store_parent ON categories(store_id, parent_id);
				CREATE INDEX IF NOT EXISTS idx_order_items_order ON order_items(order_id);
				CREATE INDEX IF NOT EXISTS idx_payments_order ON payments(order_id);
				CREATE INDEX IF NOT EXISTS idx_payments_status ON payments(status);
				CREATE INDEX IF NOT EXISTS idx_customers_store_phone ON customers(store_id, phone);
				CREATE INDEX IF NOT EXISTS idx_customers_store_email ON customers(store_id, email);
			`
			if _, e = tx.Exec(ctx, migrationSQL); e != nil {
				return fmt.Errorf("migration_v2_to_v3_failed: %w", e)
			}
			_, e = tx.Exec(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES(3,$1)", Hash(schema))
			if e != nil {
				return e
			}
		} else if n != 3 || checksum != Hash(schema) {
			return errors.New("migration_incompatible")
		}
	}
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}

func ValidateSchema(version int, checksum string) error {
	if version != 3 || checksum != Hash(schema) {
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
		case "/api/payment-webhook":
			a.paymentWebhookHandler(w, r)
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
		case "/api/categories":
			if !user.CanAccessStore(targetStore) {
				failure(w, 403, "forbidden_cross_store_access")
				return
			}
			a.categoriesHandler(w, r, targetStore, user)
		case "/api/product-variants":
			if !user.CanAccessStore(targetStore) {
				failure(w, 403, "forbidden_cross_store_access")
				return
			}
			a.productVariantsHandler(w, r, targetStore, user)
		case "/api/addon-groups":
			if !user.CanAccessStore(targetStore) {
				failure(w, 403, "forbidden_cross_store_access")
				return
			}
			a.addonGroupsHandler(w, r, targetStore, user)
		case "/api/addon-options":
			if !user.CanAccessStore(targetStore) {
				failure(w, 403, "forbidden_cross_store_access")
				return
			}
			a.addonOptionsHandler(w, r, targetStore, user)
		case "/api/product-addon-groups":
			if !user.CanAccessStore(targetStore) {
				failure(w, 403, "forbidden_cross_store_access")
				return
			}
			a.productAddonGroupsHandler(w, r, targetStore, user)
		case "/api/combos":
			if !user.CanAccessStore(targetStore) {
				failure(w, 403, "forbidden_cross_store_access")
				return
			}
			a.combosHandler(w, r, targetStore, user)
		case "/api/combo-items":
			if !user.CanAccessStore(targetStore) {
				failure(w, 403, "forbidden_cross_store_access")
				return
			}
			a.comboItemsHandler(w, r, targetStore, user)
		case "/api/customers":
			if !user.CanAccessStore(targetStore) {
				failure(w, 403, "forbidden_cross_store_access")
				return
			}
			a.customersHandler(w, r, targetStore, user)
		case "/api/consents":
			if !user.CanAccessStore(targetStore) {
				failure(w, 403, "forbidden_cross_store_access")
				return
			}
			a.consentsHandler(w, r, targetStore, user)
		case "/api/customer-export":
			if !user.CanAccessStore(targetStore) {
				failure(w, 403, "forbidden_cross_store_access")
				return
			}
			a.customerExportHandler(w, r, targetStore, user)
		case "/api/customer-delete":
			if !user.CanAccessStore(targetStore) {
				failure(w, 403, "forbidden_cross_store_access")
				return
			}
			a.customerDeleteHandler(w, r, targetStore, user)
		case "/api/payment-configs":
			if !user.CanAccessStore(targetStore) {
				failure(w, 403, "forbidden_cross_store_access")
				return
			}
			a.paymentConfigsHandler(w, r, targetStore, user)
		case "/api/payments":
			if !user.CanAccessStore(targetStore) {
				failure(w, 403, "forbidden_cross_store_access")
				return
			}
			a.paymentsHandler(w, r, targetStore, user)
		case "/api/payment-refund":
			if !user.CanAccessStore(targetStore) {
				failure(w, 403, "forbidden_cross_store_access")
				return
			}
			a.paymentRefundHandler(w, r, targetStore, user)
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
		rows, e := a.DB.Query(r.Context(), `SELECT id,name,description,price_cents,stock_quantity,unit,sku,barcode,active,available_from,available_until,min_order_quantity,max_order_quantity,is_weight_based,substitution_allowed,preparation_time_minutes,display_order,category_id FROM products WHERE store_id=$1 ORDER BY display_order, id`, store)
		if e != nil {
			failure(w, 500, "catalog_failed")
			return
		}
		defer rows.Close()
		result := []map[string]any{}
		for rows.Next() {
			var id int64
			var name, description string
			var priceCents int64
			var stockQuantity float64
			var unit, sku, barcode string
			var active bool
			var availableFrom, availableUntil *string
			var minOrderQty, maxOrderQty *float64
			var isWeightBased, substitutionAllowed bool
			var prepTime int
			var displayOrder int64
			var categoryID *int64
			if rows.Scan(&id, &name, &description, &priceCents, &stockQuantity, &unit, &sku, &barcode, &active, &availableFrom, &availableUntil, &minOrderQty, &maxOrderQty, &isWeightBased, &substitutionAllowed, &prepTime, &displayOrder, &categoryID) != nil {
				failure(w, 500, "catalog_failed")
				return
			}
			item := map[string]any{
				"id":                      id,
				"name":                    name,
				"description":             description,
				"price_cents":             priceCents,
				"stock_quantity":          stockQuantity,
				"unit":                    unit,
				"sku":                     sku,
				"barcode":                 barcode,
				"active":                  active,
				"available_from":          availableFrom,
				"available_until":         availableUntil,
				"min_order_quantity":      minOrderQty,
				"max_order_quantity":      maxOrderQty,
				"is_weight_based":         isWeightBased,
				"substitution_allowed":    substitutionAllowed,
				"preparation_time_minutes": prepTime,
				"display_order":           displayOrder,
				"category_id":             categoryID,
			}
			result = append(result, item)
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
		Name                 string   `json:"name"`
		Description          string   `json:"description"`
		PriceCents           int64    `json:"price_cents"`
		StockQuantity        float64  `json:"stock_quantity"`
		Unit                 string   `json:"unit"`
		SKU                  string   `json:"sku"`
		Barcode              string   `json:"barcode"`
		Active               bool     `json:"active"`
		AvailableFrom        string   `json:"available_from"`
		AvailableUntil       string   `json:"available_until"`
		MinOrderQuantity     float64  `json:"min_order_quantity"`
		MaxOrderQuantity     float64  `json:"max_order_quantity"`
		IsWeightBased        bool     `json:"is_weight_based"`
		SubstitutionAllowed  bool     `json:"substitution_allowed"`
		PreparationTimeMinutes int    `json:"preparation_time_minutes"`
		DisplayOrder         int64    `json:"display_order"`
		CategoryID           *int64   `json:"category_id"`
	}
	if decode(w, r, &p) != nil {
		failure(w, 400, "product_invalid")
		return
	}
	if len(p.Name) < 1 || len(p.Name) > 200 {
		failure(w, 400, "product_name_invalid")
		return
	}
	if p.PriceCents < 0 || p.PriceCents > 100000000 {
		failure(w, 400, "price_invalid")
		return
	}
	if p.StockQuantity < 0 {
		failure(w, 400, "stock_invalid")
		return
	}
	validUnits := map[string]bool{"unidade": true, "kg": true, "g": true, "l": true, "ml": true, "porcao": true}
	if p.Unit == "" {
		p.Unit = "unidade"
	}
	if !validUnits[p.Unit] {
		failure(w, 400, "unit_invalid")
		return
	}
	if p.MinOrderQuantity <= 0 {
		p.MinOrderQuantity = 1
	}
	if p.MaxOrderQuantity > 0 && p.MaxOrderQuantity < p.MinOrderQuantity {
		failure(w, 400, "max_order_quantity_invalid")
		return
	}
	var availFrom, availUntil interface{}
	if p.AvailableFrom != "" {
		availFrom = p.AvailableFrom
	}
	if p.AvailableUntil != "" {
		availUntil = p.AvailableUntil
	}
	var minQty, maxQty interface{}
	if p.MinOrderQuantity > 0 {
		minQty = p.MinOrderQuantity
	}
	if p.MaxOrderQuantity > 0 {
		maxQty = p.MaxOrderQuantity
	}
	var id int64
	e := a.DB.QueryRow(r.Context(), `INSERT INTO products(store_id,name,description,price_cents,stock_quantity,unit,sku,barcode,active,available_from,available_until,min_order_quantity,max_order_quantity,is_weight_based,substitution_allowed,preparation_time_minutes,display_order,category_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18) RETURNING id`,
		store, p.Name, p.Description, p.PriceCents, p.StockQuantity, p.Unit, p.SKU, p.Barcode, p.Active, availFrom, availUntil, minQty, maxQty, p.IsWeightBased, p.SubstitutionAllowed, p.PreparationTimeMinutes, p.DisplayOrder, p.CategoryID).Scan(&id)
	if e != nil {
		if strings.Contains(e.Error(), "unique") && strings.Contains(e.Error(), "sku") {
			failure(w, 409, "sku_already_exists")
			return
		}
		failure(w, 500, "product_failed")
		return
	}
	jsonResponse(w, 201, map[string]int64{"id": id})
}

// Categories handler
func (a *App) categoriesHandler(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanManageCatalog() {
		failure(w, 403, "forbidden_catalog_management_required")
		return
	}
	if r.Method == "GET" {
		rows, e := a.DB.Query(r.Context(), "SELECT id,parent_id,name,description,display_order,active,created_at FROM categories WHERE store_id=$1 ORDER BY display_order, id", store)
		if e != nil {
			failure(w, 500, "categories_failed")
			return
		}
		defer rows.Close()
		result := []map[string]any{}
		for rows.Next() {
			var id, parentID *int64
			var name, description string
			var displayOrder int64
			var active bool
			var createdAt time.Time
			if rows.Scan(&id, &parentID, &name, &description, &displayOrder, &active, &createdAt) != nil {
				failure(w, 500, "categories_scan_failed")
				return
			}
			result = append(result, map[string]any{
				"id":            id,
				"parent_id":     parentID,
				"name":          name,
				"description":   description,
				"display_order": displayOrder,
				"active":        active,
				"created_at":    createdAt,
			})
		}
		jsonResponse(w, 200, result)
		return
	}
	if r.Method == "POST" {
		var input struct {
			Name         string  `json:"name"`
			Description  string  `json:"description"`
			ParentID     *int64  `json:"parent_id"`
			DisplayOrder int64   `json:"display_order"`
			Active       bool    `json:"active"`
		}
		if decode(w, r, &input) != nil || len(input.Name) < 1 || len(input.Name) > 120 {
			failure(w, 400, "category_invalid")
			return
		}
		if input.ParentID != nil {
			var exists bool
			e := a.DB.QueryRow(r.Context(), "SELECT true FROM categories WHERE store_id=$1 AND id=$2", store, *input.ParentID).Scan(&exists)
			if e != nil || !exists {
				failure(w, 400, "parent_category_not_found")
				return
			}
		}
		var id int64
		e := a.DB.QueryRow(r.Context(), "INSERT INTO categories(store_id,parent_id,name,description,display_order,active) VALUES($1,$2,$3,$4,$5,$6) RETURNING id", store, input.ParentID, input.Name, input.Description, input.DisplayOrder, input.Active).Scan(&id)
		if e != nil {
			failure(w, 500, "category_failed")
			return
		}
		jsonResponse(w, 201, map[string]int64{"id": id})
		return
	}
	if r.Method == "PATCH" {
		var input struct {
			ID           int64    `json:"id"`
			Name         *string  `json:"name"`
			Description  *string  `json:"description"`
			ParentID     *int64   `json:"parent_id"`
			DisplayOrder *int64   `json:"display_order"`
			Active       *bool    `json:"active"`
		}
		if decode(w, r, &input) != nil || input.ID <= 0 {
			failure(w, 400, "category_patch_invalid")
			return
		}
		if input.ParentID != nil {
			var exists bool
			e := a.DB.QueryRow(r.Context(), "SELECT true FROM categories WHERE store_id=$1 AND id=$2", store, *input.ParentID).Scan(&exists)
			if e != nil || !exists {
				failure(w, 400, "parent_category_not_found")
				return
			}
			if *input.ParentID == input.ID {
				failure(w, 400, "category_cannot_be_own_parent")
				return
			}
		}
		tx, e := a.DB.Begin(r.Context())
		if e != nil {
			failure(w, 500, "database_unavailable")
			return
		}
		defer tx.Rollback(r.Context())
		query := "UPDATE categories SET "
		args := []any{}
		argNum := 1
		if input.Name != nil {
			query += fmt.Sprintf("name=$%d, ", argNum)
			args = append(args, *input.Name)
			argNum++
		}
		if input.Description != nil {
			query += fmt.Sprintf("description=$%d, ", argNum)
			args = append(args, *input.Description)
			argNum++
		}
		if input.ParentID != nil {
			query += fmt.Sprintf("parent_id=$%d, ", argNum)
			args = append(args, *input.ParentID)
			argNum++
		}
		if input.DisplayOrder != nil {
			query += fmt.Sprintf("display_order=$%d, ", argNum)
			args = append(args, *input.DisplayOrder)
			argNum++
		}
		if input.Active != nil {
			query += fmt.Sprintf("active=$%d, ", argNum)
			args = append(args, *input.Active)
			argNum++
		}
		query = strings.TrimSuffix(query, ", ")
		query += fmt.Sprintf(" WHERE store_id=$%d AND id=$%d", argNum, argNum+1)
		args = append(args, store, input.ID)
		_, e = tx.Exec(r.Context(), query, args...)
		if e != nil {
			failure(w, 500, "category_update_failed")
			return
		}
		e = tx.Commit(r.Context())
		if e != nil {
			failure(w, 500, "category_update_failed")
			return
		}
		jsonResponse(w, 200, map[string]bool{"ok": true})
		return
	}
	if r.Method == "DELETE" {
		idStr := r.URL.Query().Get("id")
		if idStr == "" {
			failure(w, 400, "category_id_required")
			return
		}
		id := ParseInt(idStr)
		if id <= 0 {
			failure(w, 400, "category_id_invalid")
			return
		}
		// Check for child categories
		var childCount int
		e := a.DB.QueryRow(r.Context(), "SELECT count(*) FROM categories WHERE store_id=$1 AND parent_id=$2", store, id).Scan(&childCount)
		if e == nil && childCount > 0 {
			failure(w, 409, "category_has_children")
			return
		}
		// Check for products using this category
		var productCount int
		e = a.DB.QueryRow(r.Context(), "SELECT count(*) FROM products WHERE store_id=$1 AND category_id=$2", store, id).Scan(&productCount)
		if e == nil && productCount > 0 {
			failure(w, 409, "category_in_use")
			return
		}
		_, e = a.DB.Exec(r.Context(), "DELETE FROM categories WHERE store_id=$1 AND id=$2", store, id)
		if e != nil {
			failure(w, 500, "category_delete_failed")
			return
		}
		jsonResponse(w, 200, map[string]bool{"ok": true})
		return
	}
	failure(w, 405, "method_not_allowed")
}

// Product variants handler
func (a *App) productVariantsHandler(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanManageCatalog() {
		failure(w, 403, "forbidden_catalog_management_required")
		return
	}
	productID := ParseInt(r.URL.Query().Get("product_id"))
	if productID <= 0 {
		failure(w, 400, "product_id_required")
		return
	}
	if r.Method == "GET" {
		rows, e := a.DB.Query(r.Context(), "SELECT id,name,price_adjustment_cents,stock_quantity,sku,display_order,active FROM product_variants WHERE store_id=$1 AND product_id=$2 ORDER BY display_order, id", store, productID)
		if e != nil {
			failure(w, 500, "variants_failed")
			return
		}
		defer rows.Close()
		result := []map[string]any{}
		for rows.Next() {
			var id int64
			var name, sku string
			var priceAdj int64
			var stockQty float64
			var displayOrder int64
			var active bool
			if rows.Scan(&id, &name, &priceAdj, &stockQty, &sku, &displayOrder, &active) != nil {
				failure(w, 500, "variants_scan_failed")
				return
			}
			result = append(result, map[string]any{
				"id":                      id,
				"name":                    name,
				"price_adjustment_cents":  priceAdj,
				"stock_quantity":          stockQty,
				"sku":                     sku,
				"display_order":           displayOrder,
				"active":                  active,
			})
		}
		jsonResponse(w, 200, result)
		return
	}
	if r.Method == "POST" {
		var input struct {
			Name                    string  `json:"name"`
			PriceAdjustmentCents    int64   `json:"price_adjustment_cents"`
			StockQuantity           float64 `json:"stock_quantity"`
			SKU                     string  `json:"sku"`
			DisplayOrder            int64   `json:"display_order"`
			Active                  bool    `json:"active"`
		}
		if decode(w, r, &input) != nil || len(input.Name) < 1 || len(input.Name) > 120 {
			failure(w, 400, "variant_invalid")
			return
		}
		if input.StockQuantity < 0 {
			failure(w, 400, "stock_invalid")
			return
		}
		var id int64
		e := a.DB.QueryRow(r.Context(), "INSERT INTO product_variants(product_id,store_id,name,price_adjustment_cents,stock_quantity,sku,display_order,active) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id", productID, store, input.Name, input.PriceAdjustmentCents, input.StockQuantity, input.SKU, input.DisplayOrder, input.Active).Scan(&id)
		if e != nil {
			failure(w, 500, "variant_failed")
			return
		}
		jsonResponse(w, 201, map[string]int64{"id": id})
		return
	}
	failure(w, 405, "method_not_allowed")
}

// Addon groups handler
func (a *App) addonGroupsHandler(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanManageCatalog() {
		failure(w, 403, "forbidden_catalog_management_required")
		return
	}
	if r.Method == "GET" {
		rows, e := a.DB.Query(r.Context(), "SELECT id,name,description,min_selections,max_selections,required,display_order,active FROM addon_groups WHERE store_id=$1 ORDER BY display_order, id", store)
		if e != nil {
			failure(w, 500, "addon_groups_failed")
			return
		}
		defer rows.Close()
		result := []map[string]any{}
		for rows.Next() {
			var id int64
			var name, description string
			var minSel, maxSel int
			var required bool
			var displayOrder int64
			var active bool
			if rows.Scan(&id, &name, &description, &minSel, &maxSel, &required, &displayOrder, &active) != nil {
				failure(w, 500, "addon_groups_scan_failed")
				return
			}
			result = append(result, map[string]any{
				"id":                id,
				"name":              name,
				"description":       description,
				"min_selections":    minSel,
				"max_selections":    maxSel,
				"required":          required,
				"display_order":     displayOrder,
				"active":            active,
			})
		}
		jsonResponse(w, 200, result)
		return
	}
	if r.Method == "POST" {
		var input struct {
			Name           string `json:"name"`
			Description    string `json:"description"`
			MinSelections  int    `json:"min_selections"`
			MaxSelections  int    `json:"max_selections"`
			Required       bool   `json:"required"`
			DisplayOrder   int64  `json:"display_order"`
			Active         bool   `json:"active"`
		}
		if decode(w, r, &input) != nil || len(input.Name) < 1 || len(input.Name) > 120 {
			failure(w, 400, "addon_group_invalid")
			return
		}
		if input.MinSelections < 0 || input.MaxSelections < input.MinSelections {
			failure(w, 400, "selection_limits_invalid")
			return
		}
		var id int64
		e := a.DB.QueryRow(r.Context(), "INSERT INTO addon_groups(store_id,name,description,min_selections,max_selections,required,display_order,active) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id", store, input.Name, input.Description, input.MinSelections, input.MaxSelections, input.Required, input.DisplayOrder, input.Active).Scan(&id)
		if e != nil {
			failure(w, 500, "addon_group_failed")
			return
		}
		jsonResponse(w, 201, map[string]int64{"id": id})
		return
	}
	failure(w, 405, "method_not_allowed")
}

// Addon options handler
func (a *App) addonOptionsHandler(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanManageCatalog() {
		failure(w, 403, "forbidden_catalog_management_required")
		return
	}
	groupID := ParseInt(r.URL.Query().Get("group_id"))
	if groupID <= 0 {
		failure(w, 400, "group_id_required")
		return
	}
	if r.Method == "GET" {
		rows, e := a.DB.Query(r.Context(), "SELECT id,name,description,price_cents,stock_quantity,is_default,display_order,active FROM addon_options WHERE store_id=$1 AND group_id=$2 ORDER BY display_order, id", store, groupID)
		if e != nil {
			failure(w, 500, "addon_options_failed")
			return
		}
		defer rows.Close()
		result := []map[string]any{}
		for rows.Next() {
			var id int64
			var name, description string
			var priceCents int64
			var stockQty float64
			var isDefault bool
			var displayOrder int64
			var active bool
			if rows.Scan(&id, &name, &description, &priceCents, &stockQty, &isDefault, &displayOrder, &active) != nil {
				failure(w, 500, "addon_options_scan_failed")
				return
			}
			result = append(result, map[string]any{
				"id":                id,
				"name":              name,
				"description":       description,
				"price_cents":       priceCents,
				"stock_quantity":    stockQty,
				"is_default":        isDefault,
				"display_order":     displayOrder,
				"active":            active,
			})
		}
		jsonResponse(w, 200, result)
		return
	}
	if r.Method == "POST" {
		var input struct {
			Name            string  `json:"name"`
			Description     string  `json:"description"`
			PriceCents      int64   `json:"price_cents"`
			StockQuantity   float64 `json:"stock_quantity"`
			IsDefault       bool    `json:"is_default"`
			DisplayOrder    int64   `json:"display_order"`
			Active          bool    `json:"active"`
		}
		if decode(w, r, &input) != nil || len(input.Name) < 1 || len(input.Name) > 120 {
			failure(w, 400, "addon_option_invalid")
			return
		}
		if input.PriceCents < 0 {
			failure(w, 400, "price_invalid")
			return
		}
		if input.StockQuantity < 0 {
			failure(w, 400, "stock_invalid")
			return
		}
		var id int64
		e := a.DB.QueryRow(r.Context(), "INSERT INTO addon_options(group_id,store_id,name,description,price_cents,stock_quantity,is_default,display_order,active) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id", groupID, store, input.Name, input.Description, input.PriceCents, input.StockQuantity, input.IsDefault, input.DisplayOrder, input.Active).Scan(&id)
		if e != nil {
			failure(w, 500, "addon_option_failed")
			return
		}
		jsonResponse(w, 201, map[string]int64{"id": id})
		return
	}
	failure(w, 405, "method_not_allowed")
}

// Product addon groups handler (link products to addon groups)
func (a *App) productAddonGroupsHandler(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanManageCatalog() {
		failure(w, 403, "forbidden_catalog_management_required")
		return
	}
	productID := ParseInt(r.URL.Query().Get("product_id"))
	if productID <= 0 {
		failure(w, 400, "product_id_required")
		return
	}
	if r.Method == "GET" {
		rows, e := a.DB.Query(r.Context(), "SELECT pag.group_id,ag.name,pag.display_order FROM product_addon_groups pag JOIN addon_groups ag ON ag.id=pag.group_id WHERE pag.store_id=$1 AND pag.product_id=$2 ORDER BY pag.display_order", store, productID)
		if e != nil {
			failure(w, 500, "product_addon_groups_failed")
			return
		}
		defer rows.Close()
		result := []map[string]any{}
		for rows.Next() {
			var groupID int64
			var name string
			var displayOrder int64
			if rows.Scan(&groupID, &name, &displayOrder) != nil {
				failure(w, 500, "product_addon_groups_scan_failed")
				return
			}
			result = append(result, map[string]any{
				"group_id":      groupID,
				"name":          name,
				"display_order": displayOrder,
			})
		}
		jsonResponse(w, 200, result)
		return
	}
	if r.Method == "POST" {
		var input struct {
			GroupID      int64 `json:"group_id"`
			DisplayOrder int64 `json:"display_order"`
		}
		if decode(w, r, &input) != nil || input.GroupID <= 0 {
			failure(w, 400, "product_addon_group_invalid")
			return
		}
		// Verify group exists and belongs to store
		var exists bool
		e := a.DB.QueryRow(r.Context(), "SELECT true FROM addon_groups WHERE store_id=$1 AND id=$2", store, input.GroupID).Scan(&exists)
		if e != nil || !exists {
			failure(w, 400, "addon_group_not_found")
			return
		}
		_, e = a.DB.Exec(r.Context(), "INSERT INTO product_addon_groups(product_id,store_id,group_id,display_order) VALUES($1,$2,$3,$4) ON CONFLICT (product_id,group_id) DO UPDATE SET display_order=$4", productID, store, input.GroupID, input.DisplayOrder)
		if e != nil {
			failure(w, 500, "product_addon_group_failed")
			return
		}
		jsonResponse(w, 200, map[string]bool{"ok": true})
		return
	}
	if r.Method == "DELETE" {
		groupID := ParseInt(r.URL.Query().Get("group_id"))
		if groupID <= 0 {
			failure(w, 400, "group_id_required")
			return
		}
		_, e := a.DB.Exec(r.Context(), "DELETE FROM product_addon_groups WHERE store_id=$1 AND product_id=$2 AND group_id=$3", store, productID, groupID)
		if e != nil {
			failure(w, 500, "product_addon_group_delete_failed")
			return
		}
		jsonResponse(w, 200, map[string]bool{"ok": true})
		return
	}
	failure(w, 405, "method_not_allowed")
}

// Combos handler
func (a *App) combosHandler(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanManageCatalog() {
		failure(w, 403, "forbidden_catalog_management_required")
		return
	}
	if r.Method == "GET" {
		rows, e := a.DB.Query(r.Context(), "SELECT id,name,description,price_cents,pricing_policy,active,display_order,created_at FROM combos WHERE store_id=$1 ORDER BY display_order, id", store)
		if e != nil {
			failure(w, 500, "combos_failed")
			return
		}
		defer rows.Close()
		result := []map[string]any{}
		for rows.Next() {
			var id int64
			var name, description, pricingPolicy string
			var priceCents int64
			var active bool
			var displayOrder int64
			var createdAt time.Time
			if rows.Scan(&id, &name, &description, &priceCents, &pricingPolicy, &active, &displayOrder, &createdAt) != nil {
				failure(w, 500, "combos_scan_failed")
				return
			}
			result = append(result, map[string]any{
				"id":               id,
				"name":             name,
				"description":      description,
				"price_cents":      priceCents,
				"pricing_policy":   pricingPolicy,
				"active":           active,
				"display_order":    displayOrder,
				"created_at":       createdAt,
			})
		}
		jsonResponse(w, 200, result)
		return
	}
	if r.Method == "POST" {
		var input struct {
			Name            string  `json:"name"`
			Description     string  `json:"description"`
			PriceCents      int64   `json:"price_cents"`
			PricingPolicy   string  `json:"pricing_policy"`
			Active          bool    `json:"active"`
			DisplayOrder    int64   `json:"display_order"`
		}
		if decode(w, r, &input) != nil || len(input.Name) < 1 || len(input.Name) > 120 {
			failure(w, 400, "combo_invalid")
			return
		}
		validPolicies := map[string]bool{"fixed": true, "highest": true, "sum": true}
		if input.PricingPolicy == "" {
			input.PricingPolicy = "fixed"
		}
		if !validPolicies[input.PricingPolicy] {
			failure(w, 400, "pricing_policy_invalid")
			return
		}
		if input.PriceCents < 0 {
			failure(w, 400, "price_invalid")
			return
		}
		var id int64
		e := a.DB.QueryRow(r.Context(), "INSERT INTO combos(store_id,name,description,price_cents,pricing_policy,active,display_order) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id", store, input.Name, input.Description, input.PriceCents, input.PricingPolicy, input.Active, input.DisplayOrder).Scan(&id)
		if e != nil {
			failure(w, 500, "combo_failed")
			return
		}
		jsonResponse(w, 201, map[string]int64{"id": id})
		return
	}
	failure(w, 405, "method_not_allowed")
}

// Combo items handler
func (a *App) comboItemsHandler(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanManageCatalog() {
		failure(w, 403, "forbidden_catalog_management_required")
		return
	}
	comboID := ParseInt(r.URL.Query().Get("combo_id"))
	if comboID <= 0 {
		failure(w, 400, "combo_id_required")
		return
	}
	if r.Method == "GET" {
		rows, e := a.DB.Query(r.Context(), `SELECT ci.product_id,p.name,ci.quantity,ci.is_optional,ci.group_name,ci.min_selections,ci.max_selections,ci.display_order 
			FROM combo_items ci JOIN products p ON p.store_id=ci.store_id AND p.id=ci.product_id 
			WHERE ci.store_id=$1 AND ci.combo_id=$2 ORDER BY ci.display_order`, store, comboID)
		if e != nil {
			failure(w, 500, "combo_items_failed")
			return
		}
		defer rows.Close()
		result := []map[string]any{}
		for rows.Next() {
			var productID int64
			var name string
			var quantity float64
			var isOptional bool
			var groupName string
			var minSel, maxSel int
			var displayOrder int64
			if rows.Scan(&productID, &name, &quantity, &isOptional, &groupName, &minSel, &maxSel, &displayOrder) != nil {
				failure(w, 500, "combo_items_scan_failed")
				return
			}
			result = append(result, map[string]any{
				"product_id":      productID,
				"name":            name,
				"quantity":        quantity,
				"is_optional":     isOptional,
				"group_name":      groupName,
				"min_selections":  minSel,
				"max_selections":  maxSel,
				"display_order":   displayOrder,
			})
		}
		jsonResponse(w, 200, result)
		return
	}
	if r.Method == "POST" {
		var input struct {
			ProductID      int64   `json:"product_id"`
			Quantity       float64 `json:"quantity"`
			IsOptional     bool    `json:"is_optional"`
			GroupName      string  `json:"group_name"`
			MinSelections  int     `json:"min_selections"`
			MaxSelections  int     `json:"max_selections"`
			DisplayOrder   int64   `json:"display_order"`
		}
		if decode(w, r, &input) != nil || input.ProductID <= 0 {
			failure(w, 400, "combo_item_invalid")
			return
		}
		if input.Quantity <= 0 {
			failure(w, 400, "quantity_invalid")
			return
		}
		if input.MinSelections < 0 || input.MaxSelections < input.MinSelections {
			failure(w, 400, "selection_limits_invalid")
			return
		}
		// Verify product exists and belongs to store
		var exists bool
		e := a.DB.QueryRow(r.Context(), "SELECT true FROM products WHERE store_id=$1 AND id=$2", store, input.ProductID).Scan(&exists)
		if e != nil || !exists {
			failure(w, 400, "product_not_found")
			return
		}
		_, e = a.DB.Exec(r.Context(), "INSERT INTO combo_items(combo_id,store_id,product_id,quantity,is_optional,group_name,min_selections,max_selections,display_order) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT (combo_id,product_id) DO UPDATE SET quantity=$4,is_optional=$5,group_name=$6,min_selections=$7,max_selections=$8,display_order=$9", comboID, store, input.ProductID, input.Quantity, input.IsOptional, input.GroupName, input.MinSelections, input.MaxSelections, input.DisplayOrder)
		if e != nil {
			failure(w, 500, "combo_item_failed")
			return
		}
		jsonResponse(w, 200, map[string]bool{"ok": true})
		return
	}
	if r.Method == "DELETE" {
		productID := ParseInt(r.URL.Query().Get("product_id"))
		if productID <= 0 {
			failure(w, 400, "product_id_required")
			return
		}
		_, e := a.DB.Exec(r.Context(), "DELETE FROM combo_items WHERE store_id=$1 AND combo_id=$2 AND product_id=$3", store, comboID, productID)
		if e != nil {
			failure(w, 500, "combo_item_delete_failed")
			return
		}
		jsonResponse(w, 200, map[string]bool{"ok": true})
		return
	}
	failure(w, 405, "method_not_allowed")
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

// Customers handler
func (a *App) customersHandler(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanViewFinancials() && !user.CanManageUsers() {
		failure(w, 403, "forbidden_customers_access")
		return
	}
	if r.Method == "GET" {
		rows, e := a.DB.Query(r.Context(), "SELECT id,name,phone,email,document,notes,marketing_consent,marketing_consent_at,created_at,updated_at FROM customers WHERE store_id=$1 ORDER BY created_at DESC", store)
		if e != nil {
			failure(w, 500, "customers_failed")
			return
		}
		defer rows.Close()
		result := []map[string]any{}
		for rows.Next() {
			var id int64
			var name, phone, email, document, notes string
			var marketingConsent bool
			var marketingConsentAt, createdAt, updatedAt time.Time
			if rows.Scan(&id, &name, &phone, &email, &document, &notes, &marketingConsent, &marketingConsentAt, &createdAt, &updatedAt) != nil {
				failure(w, 500, "customers_scan_failed")
				return
			}
			result = append(result, map[string]any{
				"id":                   id,
				"name":                 name,
				"phone":                phone,
				"email":                email,
				"document":             document,
				"notes":                notes,
				"marketing_consent":    marketingConsent,
				"marketing_consent_at": marketingConsentAt,
				"created_at":           createdAt,
				"updated_at":           updatedAt,
			})
		}
		jsonResponse(w, 200, result)
		return
	}
	if r.Method == "POST" {
		var input struct {
			Name            string `json:"name"`
			Phone           string `json:"phone"`
			Email           string `json:"email"`
			Document        string `json:"document"`
			Notes           string `json:"notes"`
			MarketingConsent bool  `json:"marketing_consent"`
		}
		if decode(w, r, &input) != nil || len(input.Name) < 1 || len(input.Name) > 200 {
			failure(w, 400, "customer_invalid")
			return
		}
		if input.Phone != "" && len(input.Phone) > 32 {
			failure(w, 400, "phone_invalid")
			return
		}
		if input.Email != "" && (len(input.Email) > 254 || !strings.Contains(input.Email, "@")) {
			failure(w, 400, "email_invalid")
			return
		}
		var marketingConsentAt interface{}
		if input.MarketingConsent {
			marketingConsentAt = time.Now()
		}
		var id int64
		e := a.DB.QueryRow(r.Context(), `INSERT INTO customers(store_id,name,phone,email,document,notes,marketing_consent,marketing_consent_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
			store, input.Name, input.Phone, input.Email, input.Document, input.Notes, input.MarketingConsent, marketingConsentAt).Scan(&id)
		if e != nil {
			if strings.Contains(e.Error(), "unique") && strings.Contains(e.Error(), "phone") {
				failure(w, 409, "phone_already_exists")
				return
			}
			if strings.Contains(e.Error(), "unique") && strings.Contains(e.Error(), "email") {
				failure(w, 409, "email_already_exists")
				return
			}
			failure(w, 500, "customer_failed")
			return
		}
		jsonResponse(w, 201, map[string]int64{"id": id})
		return
	}
	if r.Method == "PATCH" {
		var input struct {
			ID               int64   `json:"id"`
			Name             *string `json:"name"`
			Phone            *string `json:"phone"`
			Email            *string `json:"email"`
			Document         *string `json:"document"`
			Notes            *string `json:"notes"`
			MarketingConsent *bool   `json:"marketing_consent"`
		}
		if decode(w, r, &input) != nil || input.ID <= 0 {
			failure(w, 400, "customer_patch_invalid")
			return
		}
		// Verify customer belongs to store
		var exists bool
		e := a.DB.QueryRow(r.Context(), "SELECT true FROM customers WHERE store_id=$1 AND id=$2", store, input.ID).Scan(&exists)
		if e != nil || !exists {
			failure(w, 404, "customer_not_found")
			return
		}
		tx, e := a.DB.Begin(r.Context())
		if e != nil {
			failure(w, 500, "database_unavailable")
			return
		}
		defer tx.Rollback(r.Context())
		
		query := "UPDATE customers SET updated_at=now()"
		args := []any{}
		argNum := 1
		
		if input.Name != nil {
			if len(*input.Name) < 1 || len(*input.Name) > 200 {
				failure(w, 400, "name_invalid")
				return
			}
			query += fmt.Sprintf(", name=$%d", argNum)
			args = append(args, *input.Name)
			argNum++
		}
		if input.Phone != nil {
			if *input.Phone != "" && len(*input.Phone) > 32 {
				failure(w, 400, "phone_invalid")
				return
			}
			query += fmt.Sprintf(", phone=$%d", argNum)
			args = append(args, *input.Phone)
			argNum++
		}
		if input.Email != nil {
			if *input.Email != "" && (len(*input.Email) > 254 || !strings.Contains(*input.Email, "@")) {
				failure(w, 400, "email_invalid")
				return
			}
			query += fmt.Sprintf(", email=$%d", argNum)
			args = append(args, *input.Email)
			argNum++
		}
		if input.Document != nil {
			query += fmt.Sprintf(", document=$%d", argNum)
			args = append(args, *input.Document)
			argNum++
		}
		if input.Notes != nil {
			query += fmt.Sprintf(", notes=$%d", argNum)
			args = append(args, *input.Notes)
			argNum++
		}
		if input.MarketingConsent != nil {
			query += fmt.Sprintf(", marketing_consent=$%d", argNum)
			args = append(args, *input.MarketingConsent)
			argNum++
			if *input.MarketingConsent {
				query += fmt.Sprintf(", marketing_consent_at=$%d", argNum)
				args = append(args, time.Now())
				argNum++
			} else {
				query += ", marketing_consent_at=NULL"
			}
		}
		
		query += fmt.Sprintf(" WHERE store_id=$%d AND id=$%d", argNum, argNum+1)
		args = append(args, store, input.ID)
		
		_, e = tx.Exec(r.Context(), query, args...)
		if e != nil {
			failure(w, 500, "customer_update_failed")
			return
		}
		e = tx.Commit(r.Context())
		if e != nil {
			failure(w, 500, "customer_update_failed")
			return
		}
		jsonResponse(w, 200, map[string]bool{"ok": true})
		return
	}
	if r.Method == "DELETE" {
		idStr := r.URL.Query().Get("id")
		if idStr == "" {
			failure(w, 400, "customer_id_required")
			return
		}
		id := ParseInt(idStr)
		if id <= 0 {
			failure(w, 400, "customer_id_invalid")
			return
		}
		// Check for orders referencing this customer
		var orderCount int
		e := a.DB.QueryRow(r.Context(), "SELECT count(*) FROM orders WHERE store_id=$1 AND customer_id=$2", store, id).Scan(&orderCount)
		if e == nil && orderCount > 0 {
			failure(w, 409, "customer_has_orders")
			return
		}
		_, e = a.DB.Exec(r.Context(), "DELETE FROM customers WHERE store_id=$1 AND id=$2", store, id)
		if e != nil {
			failure(w, 500, "customer_delete_failed")
			return
		}
		jsonResponse(w, 200, map[string]bool{"ok": true})
		return
	}
	failure(w, 405, "method_not_allowed")
}

// Consents handler
func (a *App) consentsHandler(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanViewFinancials() && !user.CanManageUsers() {
		failure(w, 403, "forbidden_consents_access")
		return
	}
	if r.Method == "GET" {
		customerID := ParseInt(r.URL.Query().Get("customer_id"))
		query := "SELECT id,customer_id,purpose,version,granted,granted_at,revoked_at,source,evidence FROM consents WHERE store_id=$1"
		args := []any{store}
		if customerID > 0 {
			query += " AND customer_id=$2"
			args = append(args, customerID)
		}
		query += " ORDER BY created_at DESC"
		rows, e := a.DB.Query(r.Context(), query, args...)
		if e != nil {
			failure(w, 500, "consents_failed")
			return
		}
		defer rows.Close()
		result := []map[string]any{}
		for rows.Next() {
			var id, custID int64
			var purpose, version, source string
			var granted bool
			var grantedAt, revokedAt *time.Time
			var evidence []byte
			if rows.Scan(&id, &custID, &purpose, &version, &granted, &grantedAt, &revokedAt, &source, &evidence) != nil {
				failure(w, 500, "consents_scan_failed")
				return
			}
			result = append(result, map[string]any{
				"id":            id,
				"customer_id":   custID,
				"purpose":       purpose,
				"version":       version,
				"granted":       granted,
				"granted_at":    grantedAt,
				"revoked_at":    revokedAt,
				"source":        source,
				"evidence":      evidence,
			})
		}
		jsonResponse(w, 200, result)
		return
	}
	if r.Method == "POST" {
		var input struct {
			CustomerID int64  `json:"customer_id"`
			Purpose    string `json:"purpose"`
			Version    string `json:"version"`
			Granted    bool   `json:"granted"`
			Source     string `json:"source"`
			Evidence   any    `json:"evidence"`
		}
		if decode(w, r, &input) != nil || input.CustomerID <= 0 || len(input.Purpose) < 1 || len(input.Version) < 1 || len(input.Source) < 1 {
			failure(w, 400, "consent_invalid")
			return
		}
		// Verify customer exists and belongs to store
		var exists bool
		e := a.DB.QueryRow(r.Context(), "SELECT true FROM customers WHERE store_id=$1 AND id=$2", store, input.CustomerID).Scan(&exists)
		if e != nil || !exists {
			failure(w, 404, "customer_not_found")
			return
		}
		evidenceJSON, _ := json.Marshal(input.Evidence)
		var grantedAt interface{}
		if input.Granted {
			grantedAt = time.Now()
		}
		_, e = a.DB.Exec(r.Context(), `INSERT INTO consents(store_id,customer_id,purpose,version,granted,granted_at,source,evidence) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (store_id,customer_id,purpose,version) DO UPDATE SET granted=$5,granted_at=$6,source=$7,evidence=$8,revoked_at=NULL`,
			store, input.CustomerID, input.Purpose, input.Version, input.Granted, grantedAt, input.Source, evidenceJSON)
		if e != nil {
			failure(w, 500, "consent_failed")
			return
		}
		jsonResponse(w, 200, map[string]bool{"ok": true})
		return
	}
	if r.Method == "PATCH" {
		// Revoke consent
		var input struct {
			ID int64 `json:"id"`
		}
		if decode(w, r, &input) != nil || input.ID <= 0 {
			failure(w, 400, "consent_revoke_invalid")
			return
		}
		// Verify consent belongs to store
		var exists bool
		e := a.DB.QueryRow(r.Context(), "SELECT true FROM consents WHERE store_id=$1 AND id=$2", store, input.ID).Scan(&exists)
		if e != nil || !exists {
			failure(w, 404, "consent_not_found")
			return
		}
		_, e = a.DB.Exec(r.Context(), "UPDATE consents SET revoked_at=now() WHERE store_id=$1 AND id=$2", store, input.ID)
		if e != nil {
			failure(w, 500, "consent_revoke_failed")
			return
		}
		jsonResponse(w, 200, map[string]bool{"ok": true})
		return
	}
	failure(w, 405, "method_not_allowed")
}

// Customer data export (LGPD - Right to data portability)
func (a *App) customerExportHandler(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanViewFinancials() && !user.CanManageUsers() {
		failure(w, 403, "forbidden_export_access")
		return
	}
	if r.Method != "GET" {
		failure(w, 405, "method_not_allowed")
		return
	}
	customerID := ParseInt(r.URL.Query().Get("customer_id"))
	if customerID <= 0 {
		failure(w, 400, "customer_id_required")
		return
	}
	
	// Verify customer belongs to store
	var customerName string
	e := a.DB.QueryRow(r.Context(), "SELECT name FROM customers WHERE store_id=$1 AND id=$2", store, customerID).Scan(&customerName)
	if e != nil {
		failure(w, 404, "customer_not_found")
		return
	}
	
	// Collect all data related to customer
	ctx := r.Context()
	export := map[string]any{
		"customer": map[string]any{},
		"orders":   []map[string]any{},
		"consents": []map[string]any{},
		"payments": []map[string]any{},
	}
	
	// Customer data
	var customer struct {
		ID                int64
		Name              string
		Phone             *string
		Email             *string
		Document          *string
		Notes             *string
		MarketingConsent  bool
		MarketingConsentAt *time.Time
		CreatedAt         time.Time
		UpdatedAt         time.Time
	}
	a.DB.QueryRow(ctx, "SELECT id,name,phone,email,document,notes,marketing_consent,marketing_consent_at,created_at,updated_at FROM customers WHERE store_id=$1 AND id=$2", store, customerID).Scan(
		&customer.ID, &customer.Name, &customer.Phone, &customer.Email, &customer.Document, &customer.Notes, &customer.MarketingConsent, &customer.MarketingConsentAt, &customer.CreatedAt, &customer.UpdatedAt)
	export["customer"] = customer
	
	// Orders
	rows, e := a.DB.Query(ctx, "SELECT id,total_cents,state,financial_state,delivery_type,delivery_fee_cents,notes,created_at,confirmed_at FROM orders WHERE store_id=$1 AND customer_id=$2 ORDER BY created_at DESC", store, customerID)
	if e == nil {
		defer rows.Close()
		orders := []map[string]any{}
		for rows.Next() {
			var id, total int64
			var state, financial, deliveryType string
			var deliveryFee int64
			var notes string
			var createdAt, confirmedAt *time.Time
			rows.Scan(&id, &total, &state, &financial, &deliveryType, &deliveryFee, &notes, &createdAt, &confirmedAt)
			orders = append(orders, map[string]any{
				"id": id, "total_cents": total, "state": state, "financial_state": financial,
				"delivery_type": deliveryType, "delivery_fee_cents": deliveryFee, "notes": notes,
				"created_at": createdAt, "confirmed_at": confirmedAt,
			})
		}
		export["orders"] = orders
	}
	
	// Order items for each order
	for _, order := range export["orders"].([]map[string]any) {
		orderID := order["id"].(int64)
		itemRows, e := a.DB.Query(ctx, "SELECT oi.product_id,p.name,oi.name,oi.price_cents,oi.quantity,oi.unit,oi.notes FROM order_items oi JOIN products p ON p.store_id=oi.store_id AND p.id=oi.product_id WHERE oi.store_id=$1 AND oi.order_id=$2", store, orderID)
		if e == nil {
			items := []map[string]any{}
			defer itemRows.Close()
			for itemRows.Next() {
				var productID int64
				var prodName, itemName string
				var priceCents int64
				var quantity float64
				var unit, notes string
				itemRows.Scan(&productID, &prodName, &itemName, &priceCents, &quantity, &unit, &notes)
				items = append(items, map[string]any{
					"product_id": productID, "product_name": prodName, "name": itemName,
					"price_cents": priceCents, "quantity": quantity, "unit": unit, "notes": notes,
				})
			}
			order["items"] = items
		}
	}
	
	// Consents
	consentRows, e := a.DB.Query(ctx, "SELECT id,purpose,version,granted,granted_at,revoked_at,source,evidence FROM consents WHERE store_id=$1 AND customer_id=$2 ORDER BY created_at DESC", store, customerID)
	if e == nil {
		consents := []map[string]any{}
		defer consentRows.Close()
		for consentRows.Next() {
			var id int64
			var purpose, version, source string
			var granted bool
			var grantedAt, revokedAt *time.Time
			var evidence []byte
			consentRows.Scan(&id, &purpose, &version, &granted, &grantedAt, &revokedAt, &source, &evidence)
			consents = append(consents, map[string]any{
				"id": id, "purpose": purpose, "version": version, "granted": granted,
				"granted_at": grantedAt, "revoked_at": revokedAt, "source": source, "evidence": evidence,
			})
		}
		export["consents"] = consents
	}
	
	// Payments
	paymentRows, e := a.DB.Query(ctx, "SELECT id,provider,external_id,amount_cents,currency,status,qr_code,pix_copy_paste,expires_at,confirmed_at,refunded_amount_cents,created_at FROM payments WHERE store_id=$1 AND order_id IN (SELECT id FROM orders WHERE store_id=$1 AND customer_id=$2)", store, customerID)
	if e == nil {
		payments := []map[string]any{}
		defer paymentRows.Close()
		for paymentRows.Next() {
			var id int64
			var provider, externalID, currency, status string
			var amountCents int64
			var qrCode, pixCopyPaste string
			var expiresAt, confirmedAt *time.Time
			var refundedAmount int64
			var createdAt time.Time
			paymentRows.Scan(&id, &provider, &externalID, &amountCents, &currency, &status, &qrCode, &pixCopyPaste, &expiresAt, &confirmedAt, &refundedAmount, &createdAt)
			payments = append(payments, map[string]any{
				"id": id, "provider": provider, "external_id": externalID, "amount_cents": amountCents,
				"currency": currency, "status": status, "qr_code": qrCode, "pix_copy_paste": pixCopyPaste,
				"expires_at": expiresAt, "confirmed_at": confirmedAt, "refunded_amount_cents": refundedAmount,
				"created_at": createdAt,
			})
		}
		export["payments"] = payments
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=customer-export-%d-%s.json", customerID, time.Now().Format("20060102")))
	json.NewEncoder(w).Encode(export)
}

// Customer data deletion (LGPD - Right to erasure)
func (a *App) customerDeleteHandler(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanManageUsers() {
		failure(w, 403, "forbidden_deletion_access")
		return
	}
	if r.Method != "DELETE" {
		failure(w, 405, "method_not_allowed")
		return
	}
	customerID := ParseInt(r.URL.Query().Get("customer_id"))
	if customerID <= 0 {
		failure(w, 400, "customer_id_required")
		return
	}
	
	// Check if customer has orders that need to be retained for fiscal reasons
	var orderCount int
	e := a.DB.QueryRow(r.Context(), "SELECT count(*) FROM orders WHERE store_id=$1 AND customer_id=$2 AND state NOT IN ('cancelled')", store, customerID).Scan(&orderCount)
	if e == nil && orderCount > 0 {
		// Anonymize instead of delete - keep orders for fiscal compliance but remove PII
		tx, e := a.DB.Begin(r.Context())
		if e != nil {
			failure(w, 500, "database_unavailable")
			return
		}
		defer tx.Rollback(r.Context())
		
		// Anonymize customer data
		_, e = tx.Exec(r.Context(), `UPDATE customers SET 
			name='Cliente Anonimizado', 
			phone=NULL, 
			email=NULL, 
			document=NULL, 
			notes='Dados anonimizados por solicitação LGPD', 
			marketing_consent=false, 
			marketing_consent_at=NULL,
			updated_at=now()
		WHERE store_id=$1 AND id=$2`, store, customerID)
		if e != nil {
			failure(w, 500, "anonymization_failed")
			return
		}
		
		// Revoke all consents
		_, e = tx.Exec(r.Context(), "UPDATE consents SET revoked_at=now() WHERE store_id=$1 AND customer_id=$2", store, customerID)
		if e != nil {
			failure(w, 500, "consent_revocation_failed")
			return
		}
		
		// Anonymize order customer data
		_, e = tx.Exec(r.Context(), `UPDATE orders SET 
			customer_name='Cliente Anonimizado',
			customer_phone=NULL,
			customer_email=NULL
		WHERE store_id=$1 AND customer_id=$2`, store, customerID)
		if e != nil {
			failure(w, 500, "order_anonymization_failed")
			return
		}
		
		e = tx.Commit(r.Context())
		if e != nil {
			failure(w, 500, "anonymization_failed")
			return
		}
		jsonResponse(w, 200, map[string]any{"ok": true, "anonymized": true, "message": "Dados do cliente anonimizados. Pedidos mantidos para conformidade fiscal."})
		return
	}
	
	// No active orders - full deletion allowed
	_, e = a.DB.Exec(r.Context(), "DELETE FROM customers WHERE store_id=$1 AND id=$2", store, customerID)
	if e != nil {
		failure(w, 500, "customer_delete_failed")
		return
	}
	jsonResponse(w, 200, map[string]any{"ok": true, "deleted": true})
}

// =============================================================================
// Payment Connectors Implementation
// =============================================================================

// BasePaymentConnector provides common functionality for payment connectors
type BasePaymentConnector struct {
	Config    PaymentConfig
	HTTPClient *http.Client
}

func NewBasePaymentConnector(config PaymentConfig) *BasePaymentConnector {
	return &BasePaymentConnector{
		Config: config,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (b *BasePaymentConnector) getCredential(key string) string {
	return b.Config.Credentials[key]
}

func (b *BasePaymentConnector) makeRequest(ctx context.Context, method, url string, headers map[string]string, body any) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(jsonBody)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return b.HTTPClient.Do(req)
}

// MercadoPagoConnector implements PaymentConnector for Mercado Pago
type MercadoPagoConnector struct {
	*BasePaymentConnector
}

func NewMercadoPagoConnector(config PaymentConfig) *MercadoPagoConnector {
	return &MercadoPagoConnector{
		BasePaymentConnector: NewBasePaymentConnector(config),
	}
}

func (m *MercadoPagoConnector) ProviderName() string {
	return "mercadopago"
}

func (m *MercadoPagoConnector) Capabilities() PaymentCapabilities {
	return PaymentCapabilities{
		SupportsRefund:        true,
		SupportsPartialRefund: true,
		SupportsWebhook:       true,
		SupportsPixStatic:     false,
		SupportsPixDynamic:    true,
		MaxExpirationHours:    24,
		MinAmountCents:        100,
		MaxAmountCents:        100000000,
	}
}

func (m *MercadoPagoConnector) getAccessToken(ctx context.Context) (string, error) {
	// In production, this should use OAuth2 with client_id/client_secret
	// For now, use the access token from credentials
	token := m.getCredential("access_token")
	if token == "" {
		return "", errors.New("mercadopago: access_token not configured")
	}
	return token, nil
}

func (m *MercadoPagoConnector) CreateCharge(ctx context.Context, req ChargeRequest) (*ChargeResponse, error) {
	token, err := m.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	
	baseURL := m.getCredential("base_url")
	if baseURL == "" {
		baseURL = "https://api.mercadopago.com"
	}
	
	expiration := time.Now().Add(time.Duration(req.ExpirationMinutes) * time.Minute)
	
	payload := map[string]any{
		"transaction_amount": float64(req.AmountCents) / 100.0,
		"description":        req.Description,
		"payment_method_id":  "pix",
		"payer": map[string]any{
			"email": req.PayerEmail,
			"first_name": req.PayerName,
			"identification": map[string]string{
				"type": "CPF",
				"number": strings.ReplaceAll(strings.ReplaceAll(req.PayerDocument, ".", ""), "-", ""),
			},
			"phone": map[string]string{
				"area_code": strings.TrimPrefix(req.PayerPhone, "+55 ")[:2],
				"number": strings.TrimPrefix(req.PayerPhone, "+55 ")[2:],
			},
		},
		"date_of_expiration": expiration.Format(time.RFC3339),
		"notification_url": m.getCredential("webhook_url"),
		"external_reference": req.IdempotencyKey,
		"metadata": req.Metadata,
	}
	
	resp, err := m.makeRequest(ctx, "POST", baseURL+"/v1/payments", map[string]string{
		"Authorization": "Bearer " + token,
		"X-Idempotency-Key": req.IdempotencyKey,
	}, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("mercadopago: %v", result)
	}
	
	// Extract Pix data
	var qrCode, pixCopyPaste string
	if pointOfInteraction, ok := result["point_of_interaction"].(map[string]any); ok {
		if txData, ok := pointOfInteraction["transaction_data"].(map[string]any); ok {
			qrCode = getString(txData, "qr_code")
			pixCopyPaste = getString(txData, "qr_code_base64")
		}
	}
	
	expiresAt, _ := time.Parse(time.RFC3339, getString(result, "date_of_expiration"))
	
	return &ChargeResponse{
		PaymentID:    getString(result, "id"),
		ExternalID:   getString(result, "id"),
		QRCode:       qrCode,
		PixCopyPaste: pixCopyPaste,
		ExpiresAt:    expiresAt,
		Status:       getString(result, "status"),
		Provider:     "mercadopago",
		AmountCents:  req.AmountCents,
	}, nil
}

func (m *MercadoPagoConnector) GetChargeStatus(ctx context.Context, paymentID string) (*PaymentStatus, error) {
	token, err := m.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	
	baseURL := m.getCredential("base_url")
	if baseURL == "" {
		baseURL = "https://api.mercadopago.com"
	}
	
	resp, err := m.makeRequest(ctx, "GET", baseURL+"/v1/payments/"+paymentID, map[string]string{
		"Authorization": "Bearer " + token,
	}, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("mercadopago: %v", result)
	}
	
	var paidAt *time.Time
	if dateApproved := getString(result, "date_approved"); dateApproved != "" {
		t, _ := time.Parse(time.RFC3339, dateApproved)
		paidAt = &t
	}
	
	return &PaymentStatus{
		PaymentID:    paymentID,
		ExternalID:   getString(result, "id"),
		Status:       mapMercadoPagoStatus(getString(result, "status")),
		AmountCents:  int64(getFloat64(result, "transaction_amount") * 100),
		PaidAt:       paidAt,
		ProviderData: result,
	}, nil
}

func (m *MercadoPagoConnector) ProcessWebhook(ctx context.Context, payload []byte, headers http.Header) (*PaymentStatus, error) {
	// Verify webhook signature
	signature := headers.Get("X-Signature")
	if signature == "" || m.Config.WebhookSecret == "" {
		return nil, errors.New("mercadopago: missing webhook signature")
	}
	
	// Mercado Pago signature verification
	// Format: ts=<timestamp>,v1=<signature>
	parts := strings.Split(signature, ",")
	var ts, v1 string
	for _, part := range parts {
		kv := strings.Split(part, "=")
		if len(kv) == 2 {
			if kv[0] == "ts" {
				ts = kv[1]
			} else if kv[0] == "v1" {
				v1 = kv[1]
			}
		}
	}
	
	if ts == "" || v1 == "" {
		return nil, errors.New("mercadopago: invalid signature format")
	}
	
	manifest := ts + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(m.Config.WebhookSecret))
	mac.Write([]byte(manifest))
	expectedSignature := hex.EncodeToString(mac.Sum(nil))
	
	if !hmac.Equal([]byte(expectedSignature), []byte(v1)) {
		return nil, errors.New("mercadopago: invalid signature")
	}
	
	var webhook map[string]any
	if err := json.Unmarshal(payload, &webhook); err != nil {
		return nil, err
	}
	
	// Extract payment ID from webhook
	paymentID := getString(webhook, "data.id")
	if paymentID == "" {
		return nil, errors.New("mercadopago: payment id not found in webhook")
	}
	
	// Fetch payment details
	return m.GetChargeStatus(ctx, paymentID)
}

func (m *MercadoPagoConnector) Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error) {
	token, err := m.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	
	baseURL := m.getCredential("base_url")
	if baseURL == "" {
		baseURL = "https://api.mercadopago.com"
	}
	
	payload := map[string]any{
		"amount": float64(req.AmountCents) / 100.0,
	}
	
	resp, err := m.makeRequest(ctx, "POST", baseURL+"/v1/payments/"+req.PaymentID+"/refunds", map[string]string{
		"Authorization": "Bearer " + token,
		"X-Idempotency-Key": req.IdempotencyKey,
	}, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("mercadopago refund: %v", result)
	}
	
	return &RefundResponse{
		RefundID:    getString(result, "id"),
		ExternalID:  getString(result, "id"),
		Status:      mapMercadoPagoRefundStatus(getString(result, "status")),
		AmountCents: int64(getFloat64(result, "amount") * 100),
	}, nil
}

func (m *MercadoPagoConnector) GetRefundStatus(ctx context.Context, refundID string) (*RefundResponse, error) {
	token, err := m.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	
	baseURL := m.getCredential("base_url")
	if baseURL == "" {
		baseURL = "https://api.mercadopago.com"
	}
	
	resp, err := m.makeRequest(ctx, "GET", baseURL+"/v1/refunds/"+refundID, map[string]string{
		"Authorization": "Bearer " + token,
	}, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("mercadopago refund status: %v", result)
	}
	
	return &RefundResponse{
		RefundID:    refundID,
		ExternalID:  getString(result, "id"),
		Status:      mapMercadoPagoRefundStatus(getString(result, "status")),
		AmountCents: int64(getFloat64(result, "amount") * 100),
	}, nil
}

// AsaasConnector implements PaymentConnector for Asaas
type AsaasConnector struct {
	*BasePaymentConnector
}

func NewAsaasConnector(config PaymentConfig) *AsaasConnector {
	return &AsaasConnector{
		BasePaymentConnector: NewBasePaymentConnector(config),
	}
}

func (a *AsaasConnector) ProviderName() string {
	return "asaas"
}

func (a *AsaasConnector) Capabilities() PaymentCapabilities {
	return PaymentCapabilities{
		SupportsRefund:        true,
		SupportsPartialRefund: true,
		SupportsWebhook:       true,
		SupportsPixStatic:     true,
		SupportsPixDynamic:    true,
		MaxExpirationHours:    720, // 30 days
		MinAmountCents:        100,
		MaxAmountCents:        100000000,
	}
}

func (a *AsaasConnector) getAccessToken() string {
	return a.getCredential("access_token")
}

func (a *AsaasConnector) getBaseURL() string {
	url := a.getCredential("base_url")
	if url == "" {
		url = "https://api.asaas.com/v3"
	}
	return url
}

func (a *AsaasConnector) CreateCharge(ctx context.Context, req ChargeRequest) (*ChargeResponse, error) {
	token := a.getAccessToken()
	if token == "" {
		return nil, errors.New("asaas: access_token not configured")
	}
	
	// First, ensure customer exists in Asaas
	customerID, err := a.ensureCustomer(ctx, req)
	if err != nil {
		return nil, err
	}
	
	dueDate := time.Now().Add(time.Duration(req.ExpirationMinutes) * time.Minute).Format("2006-01-02")
	
	payload := map[string]any{
		"customer": customerID,
		"billingType": "PIX",
		"value": float64(req.AmountCents) / 100.0,
		"dueDate": dueDate,
		"description": req.Description,
		"externalReference": req.IdempotencyKey,
	}
	
	resp, err := a.makeRequest(ctx, "POST", a.getBaseURL()+"/payments", map[string]string{
		"access_token": token,
	}, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("asaas: %v", result)
	}
	
	paymentID := getString(result, "id")
	
	// Get Pix details
	pixResp, err := a.makeRequest(ctx, "GET", a.getBaseURL()+"/payments/"+paymentID+"/pixQrCode", map[string]string{
		"access_token": token,
	}, nil)
	if err != nil {
		return nil, err
	}
	defer pixResp.Body.Close()
	
	var pixResult map[string]any
	json.NewDecoder(pixResp.Body).Decode(&pixResult)
	
	expiresAt, _ := time.Parse("2006-01-02", getString(result, "dueDate"))
	
	return &ChargeResponse{
		PaymentID:    paymentID,
		ExternalID:   paymentID,
		QRCode:       getString(pixResult, "payload"),
		PixCopyPaste: getString(pixResult, "payload"),
		ExpiresAt:    expiresAt,
		Status:       mapAsaasStatus(getString(result, "status")),
		Provider:     "asaas",
		AmountCents:  req.AmountCents,
	}, nil
}

func (a *AsaasConnector) ensureCustomer(ctx context.Context, req ChargeRequest) (string, error) {
	// Check if customer exists by email or CPF/CNPJ
	doc := strings.ReplaceAll(strings.ReplaceAll(req.PayerDocument, ".", ""), "-", "")
	
	// Search for existing customer
	resp, err := a.makeRequest(ctx, "GET", a.getBaseURL()+"/customers?email="+url.QueryEscape(req.PayerEmail), map[string]string{
		"access_token": a.getAccessToken(),
	}, nil)
	if err == nil {
		defer resp.Body.Close()
		var result map[string]any
		json.NewDecoder(resp.Body).Decode(&result)
		if data, ok := result["data"].([]any); ok && len(data) > 0 {
			if customer, ok := data[0].(map[string]any); ok {
				return getString(customer, "id"), nil
			}
		}
	}
	
	// Create new customer
	payload := map[string]any{
		"name": req.PayerName,
		"email": req.PayerEmail,
		"cpfCnpj": doc,
		"phone": req.PayerPhone,
	}
	
	resp, err = a.makeRequest(ctx, "POST", a.getBaseURL()+"/customers", map[string]string{
		"access_token": a.getAccessToken(),
	}, payload)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("asaas create customer: %v", result)
	}
	
	return getString(result, "id"), nil
}

func (a *AsaasConnector) GetChargeStatus(ctx context.Context, paymentID string) (*PaymentStatus, error) {
	token := a.getAccessToken()
	if token == "" {
		return nil, errors.New("asaas: access_token not configured")
	}
	
	resp, err := a.makeRequest(ctx, "GET", a.getBaseURL()+"/payments/"+paymentID, map[string]string{
		"access_token": token,
	}, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("asaas: %v", result)
	}
	
	var paidAt *time.Time
	if paymentDate := getString(result, "paymentDate"); paymentDate != "" {
		t, _ := time.Parse("2006-01-02", paymentDate)
		paidAt = &t
	}
	
	return &PaymentStatus{
		PaymentID:    paymentID,
		ExternalID:   paymentID,
		Status:       mapAsaasStatus(getString(result, "status")),
		AmountCents:  int64(getFloat64(result, "value") * 100),
		PaidAt:       paidAt,
		ProviderData: result,
	}, nil
}

func (a *AsaasConnector) ProcessWebhook(ctx context.Context, payload []byte, headers http.Header) (*PaymentStatus, error) {
	// Asaas uses webhook signature verification
	signature := headers.Get("Asaas-Signature")
	if signature == "" || a.Config.WebhookSecret == "" {
		return nil, errors.New("asaas: missing webhook signature")
	}
	
	// Verify signature
	mac := hmac.New(sha256.New, []byte(a.Config.WebhookSecret))
	mac.Write(payload)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))
	
	if !hmac.Equal([]byte(expectedSignature), []byte(signature)) {
		return nil, errors.New("asaas: invalid signature")
	}
	
	var webhook map[string]any
	if err := json.Unmarshal(payload, &webhook); err != nil {
		return nil, err
	}
	
	// Asaas webhook has payment object directly
	paymentID := getString(webhook, "payment.id")
	if paymentID == "" {
		return nil, errors.New("asaas: payment id not found in webhook")
	}
	
	return a.GetChargeStatus(ctx, paymentID)
}

func (a *AsaasConnector) Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error) {
	token := a.getAccessToken()
	if token == "" {
		return nil, errors.New("asaas: access_token not configured")
	}
	
	// Asaas doesn't have a direct refund API for Pix, need to use the refund endpoint
	payload := map[string]any{
		"value": float64(req.AmountCents) / 100.0,
		"description": req.Reason,
	}
	
	resp, err := a.makeRequest(ctx, "POST", a.getBaseURL()+"/payments/"+req.PaymentID+"/refund", map[string]string{
		"access_token": token,
	}, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("asaas refund: %v", result)
	}
	
	return &RefundResponse{
		RefundID:    getString(result, "id"),
		ExternalID:  getString(result, "id"),
		Status:      "completed",
		AmountCents: int64(getFloat64(result, "value") * 100),
	}, nil
}

func (a *AsaasConnector) GetRefundStatus(ctx context.Context, refundID string) (*RefundResponse, error) {
	// Asaas doesn't have a separate refund status endpoint
	// Return basic info
	return &RefundResponse{
		RefundID:    refundID,
		ExternalID:  refundID,
		Status:      "completed",
		AmountCents: 0,
	}, nil
}

// EfiConnector implements PaymentConnector for Efí (formerly Gerencianet)
type EfiConnector struct {
	*BasePaymentConnector
	Token string
	TokenExpiry time.Time
}

func NewEfiConnector(config PaymentConfig) *EfiConnector {
	return &EfiConnector{
		BasePaymentConnector: NewBasePaymentConnector(config),
	}
}

func (e *EfiConnector) ProviderName() string {
	return "efi"
}

func (e *EfiConnector) Capabilities() PaymentCapabilities {
	return PaymentCapabilities{
		SupportsRefund:        true,
		SupportsPartialRefund: true,
		SupportsWebhook:       true,
		SupportsPixStatic:     true,
		SupportsPixDynamic:    true,
		MaxExpirationHours:    24,
		MinAmountCents:        100,
		MaxAmountCents:        100000000,
	}
}

func (e *EfiConnector) getBaseURL() string {
	url := e.getCredential("base_url")
	if url == "" {
		url = "https://api.efipay.com.br"
	}
	return url
}

func (e *EfiConnector) getAuth(ctx context.Context) (string, error) {
	if e.Token != "" && time.Now().Before(e.TokenExpiry) {
		return e.Token, nil
	}
	
	clientID := e.getCredential("client_id")
	clientSecret := e.getCredential("client_secret")
	if clientID == "" || clientSecret == "" {
		return "", errors.New("efi: client_id and client_secret required")
	}
	
	// OAuth2 client credentials flow
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	
	req, err := http.NewRequestWithContext(ctx, "POST", e.getBaseURL()+"/oauth/token", strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)
	
	resp, err := e.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("efi auth: %v", result)
	}
	
	e.Token = getString(result, "access_token")
	expiresIn := getInt(result, "expires_in")
	e.TokenExpiry = time.Now().Add(time.Duration(expiresIn-60) * time.Second)
	
	return e.Token, nil
}

func (e *EfiConnector) makeAuthenticatedRequest(ctx context.Context, method, path string, body any) (*http.Response, error) {
	token, err := e.getAuth(ctx)
	if err != nil {
		return nil, err
	}
	
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(jsonBody)
	}
	
	req, err := http.NewRequestWithContext(ctx, method, e.getBaseURL()+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	
	return e.HTTPClient.Do(req)
}

func (e *EfiConnector) CreateCharge(ctx context.Context, req ChargeRequest) (*ChargeResponse, error) {
	expiration := time.Now().Add(time.Duration(req.ExpirationMinutes) * time.Minute)
	
	payload := map[string]any{
		"calendario": map[string]any{
			"expiracao": int(expiration.Sub(time.Now()).Seconds()),
		},
		"devedor": map[string]any{
			"cpf": strings.ReplaceAll(strings.ReplaceAll(req.PayerDocument, ".", ""), "-", ""),
			"nome": req.PayerName,
		},
		"valor": map[string]any{
			"original": fmt.Sprintf("%.2f", float64(req.AmountCents)/100.0),
		},
		"chave": e.getCredential("pix_key"),
		"solicitacaoPagador": req.Description,
		"infoAdicionais": []map[string]string{
			{"nome": "external_reference", "valor": req.IdempotencyKey},
		},
	}
	
	resp, err := e.makeAuthenticatedRequest(ctx, "POST", "/v2/cob", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("efi: %v", result)
	}
	
	txid := getString(result, "txid")
	
	// Get Pix QR code
	pixResp, err := e.makeAuthenticatedRequest(ctx, "GET", "/v2/loc/"+getString(result, "loc.id")+"/qrcode", nil)
	if err != nil {
		return nil, err
	}
	defer pixResp.Body.Close()
	
	var pixResult map[string]any
	json.NewDecoder(pixResp.Body).Decode(&pixResult)
	
	expiresAt, _ := time.Parse(time.RFC3339, getString(result, "calendario.expiracao"))
	
	return &ChargeResponse{
		PaymentID:    txid,
		ExternalID:   txid,
		QRCode:       getString(pixResult, "imagemQrcode"),
		PixCopyPaste: getString(pixResult, "qrcode"),
		ExpiresAt:    expiresAt,
		Status:       "pending",
		Provider:     "efi",
		AmountCents:  req.AmountCents,
	}, nil
}

func (e *EfiConnector) GetChargeStatus(ctx context.Context, paymentID string) (*PaymentStatus, error) {
	resp, err := e.makeAuthenticatedRequest(ctx, "GET", "/v2/cob/"+paymentID, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("efi: %v", result)
	}
	
	var paidAt *time.Time
	if pix := getStringMap(result, "pix"); pix != nil {
		if pixData, ok := pix[0].(map[string]any); ok {
			if endToEndId := getString(pixData, "endToEndId"); endToEndId != "" {
				// Would need to fetch transaction details for exact paid time
			}
		}
	}
	
	return &PaymentStatus{
		PaymentID:    paymentID,
		ExternalID:   paymentID,
		Status:       mapEfiStatus(getString(result, "status")),
		AmountCents:  int64(getFloat64(result, "valor.original") * 100),
		PaidAt:       paidAt,
		ProviderData: result,
	}, nil
}

func (e *EfiConnector) ProcessWebhook(ctx context.Context, payload []byte, headers http.Header) (*PaymentStatus, error) {
	// Efí uses x-skip-signature for webhook verification
	signature := headers.Get("X-Skip-Signature")
	if signature == "" || e.Config.WebhookSecret == "" {
		return nil, errors.New("efi: missing webhook signature")
	}
	
	// Verify signature
	mac := hmac.New(sha256.New, []byte(e.Config.WebhookSecret))
	mac.Write(payload)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))
	
	if !hmac.Equal([]byte(expectedSignature), []byte(signature)) {
		return nil, errors.New("efi: invalid signature")
	}
	
	var webhook map[string]any
	if err := json.Unmarshal(payload, &webhook); err != nil {
		return nil, err
	}
	
	// Efí webhook structure
	paymentID := getString(webhook, "pix.txid")
	if paymentID == "" {
		paymentID = getString(webhook, "cob.txid")
	}
	if paymentID == "" {
		return nil, errors.New("efi: payment id not found in webhook")
	}
	
	return e.GetChargeStatus(ctx, paymentID)
}

func (e *EfiConnector) Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error) {
	// Efí supports devolução (refund) for Pix
	payload := map[string]any{
		"valor": fmt.Sprintf("%.2f", float64(req.AmountCents)/100.0),
		"devolucao": map[string]any{
			"descricao": req.Reason,
		},
	}
	
	resp, err := e.makeAuthenticatedRequest(ctx, "PUT", "/v2/pix/"+req.PaymentID+"/devolucao", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("efi refund: %v", result)
	}
	
	return &RefundResponse{
		RefundID:    getString(result, "id"),
		ExternalID:  getString(result, "id"),
		Status:      mapEfiRefundStatus(getString(result, "status")),
		AmountCents: int64(getFloat64(result, "valor") * 100),
	}, nil
}

func (e *EfiConnector) GetRefundStatus(ctx context.Context, refundID string) (*RefundResponse, error) {
	resp, err := e.makeAuthenticatedRequest(ctx, "GET", "/v2/pix/devolucao/"+refundID, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("efi refund status: %v", result)
	}
	
	return &RefundResponse{
		RefundID:    refundID,
		ExternalID:  refundID,
		Status:      mapEfiRefundStatus(getString(result, "status")),
		AmountCents: int64(getFloat64(result, "valor") * 100),
	}, nil
}

// Helper functions for payment connectors
func getString(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getFloat64(m map[string]any, key string) float64 {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return val
		case float32:
			return float64(val)
		case int:
			return float64(val)
		case int64:
			return float64(val)
		case string:
			f, _ := strconv.ParseFloat(val, 64)
			return f
		}
	}
	return 0
}

func getInt(m map[string]any, key string) int {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return int(val)
		case int:
			return val
		case int64:
			return int(val)
		}
	}
	return 0
}

func getStringMap(m map[string]any, key string) []any {
	if v, ok := m[key]; ok {
		if arr, ok := v.([]any); ok {
			return arr
		}
	}
	return nil
}

func mapMercadoPagoStatus(status string) string {
	switch status {
	case "approved", "accredited":
		return "confirmed"
	case "pending", "in_process":
		return "pending"
	case "rejected", "cancelled", "refunded", "charged_back":
		return "cancelled"
	case "expired":
		return "expired"
	default:
		return "pending"
	}
}

func mapMercadoPagoRefundStatus(status string) string {
	switch status {
	case "approved", "completed":
		return "completed"
	case "pending", "in_process":
		return "pending"
	default:
		return "pending"
	}
}

func mapAsaasStatus(status string) string {
	switch status {
	case "RECEIVED", "CONFIRMED":
		return "confirmed"
	case "PENDING", "OVERDUE":
		return "pending"
	case "REFUNDED", "RECEIVED_IN_CASH":
		return "refunded"
	case "CANCELLED":
		return "cancelled"
	default:
		return "pending"
	}
}

func mapEfiStatus(status string) string {
	switch status {
	case "ativa", "concluida":
		return "confirmed"
	case "expirada", "removida_pelo_usuario":
		return "expired"
	default:
		return "pending"
	}
}

func mapEfiRefundStatus(status string) string {
	switch status {
	case "concluida":
		return "completed"
	case "pendente", "em_processamento":
		return "pending"
	default:
		return "pending"
	}
}

// PaymentConnectorFactory creates payment connectors based on provider
func NewPaymentConnector(config PaymentConfig) (PaymentConnector, error) {
	switch config.Provider {
	case "mercadopago":
		return NewMercadoPagoConnector(config), nil
	case "asaas":
		return NewAsaasConnector(config), nil
	case "efi":
		return NewEfiConnector(config), nil
	default:
		return nil, fmt.Errorf("unsupported payment provider: %s", config.Provider)
	}
}

// Payment management handlers
func (a *App) paymentConfigsHandler(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanViewFinancials() {
		failure(w, 403, "forbidden_payments_access")
		return
	}
	
	if r.Method == "GET" {
		rows, e := a.DB.Query(r.Context(), "SELECT id,provider,enabled,credentials,settings,created_at,updated_at FROM payment_configs WHERE store_id=$1 ORDER BY id", store)
		if e != nil {
			failure(w, 500, "payment_configs_failed")
			return
		}
		defer rows.Close()
		result := []map[string]any{}
		for rows.Next() {
			var id int64
			var provider string
			var enabled bool
			var credentials, settings []byte
			var createdAt, updatedAt time.Time
			if rows.Scan(&id, &provider, &enabled, &credentials, &settings, &createdAt, &updatedAt) != nil {
				failure(w, 500, "payment_configs_scan_failed")
				return
			}
			// Mask credentials for security
			var credMap map[string]string
			json.Unmarshal(credentials, &credMap)
			if credMap != nil {
				for k := range credMap {
					if strings.Contains(strings.ToLower(k), "secret") || strings.Contains(strings.ToLower(k), "token") || strings.Contains(strings.ToLower(k), "key") {
						credMap[k] = "***"
					}
				}
			}
			result = append(result, map[string]any{
				"id": id, "provider": provider, "enabled": enabled,
				"credentials": credMap, "settings": settings,
				"created_at": createdAt, "updated_at": updatedAt,
			})
		}
		jsonResponse(w, 200, result)
		return
	}
	
	if r.Method == "POST" {
		var input struct {
			Provider      string            `json:"provider"`
			Enabled       bool              `json:"enabled"`
			Credentials   map[string]string `json:"credentials"`
			Settings      map[string]any    `json:"settings"`
			WebhookSecret string            `json:"webhook_secret"`
		}
		if decode(w, r, &input) != nil || input.Provider == "" {
			failure(w, 400, "payment_config_invalid")
			return
		}
		
		validProviders := map[string]bool{"mercadopago": true, "asaas": true, "efi": true}
		if !validProviders[input.Provider] {
			failure(w, 400, "provider_invalid")
			return
		}
		
		credJSON, _ := json.Marshal(input.Credentials)
		settingsJSON, _ := json.Marshal(input.Settings)
		
		var id int64
		e := a.DB.QueryRow(r.Context(), `INSERT INTO payment_configs(store_id,provider,enabled,credentials,settings,webhook_secret) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`,
			store, input.Provider, input.Enabled, credJSON, settingsJSON, input.WebhookSecret).Scan(&id)
		if e != nil {
			failure(w, 500, "payment_config_failed")
			return
		}
		jsonResponse(w, 201, map[string]int64{"id": id})
		return
	}
	
	if r.Method == "PATCH" {
		var input struct {
			ID            int64               `json:"id"`
			Enabled       *bool               `json:"enabled"`
			Credentials   map[string]string   `json:"credentials"`
			Settings      map[string]any      `json:"settings"`
			WebhookSecret *string             `json:"webhook_secret"`
		}
		if decode(w, r, &input) != nil || input.ID <= 0 {
			failure(w, 400, "payment_config_patch_invalid")
			return
		}
		
		tx, e := a.DB.Begin(r.Context())
		if e != nil {
			failure(w, 500, "database_unavailable")
			return
		}
		defer tx.Rollback(r.Context())
		
		query := "UPDATE payment_configs SET updated_at=now()"
		args := []any{}
		argNum := 1
		
		if input.Enabled != nil {
			query += fmt.Sprintf(", enabled=$%d", argNum)
			args = append(args, *input.Enabled)
			argNum++
		}
		if input.Credentials != nil {
			credJSON, _ := json.Marshal(input.Credentials)
			query += fmt.Sprintf(", credentials=$%d", argNum)
			args = append(args, credJSON)
			argNum++
		}
		if input.Settings != nil {
			settingsJSON, _ := json.Marshal(input.Settings)
			query += fmt.Sprintf(", settings=$%d", argNum)
			args = append(args, settingsJSON)
			argNum++
		}
		if input.WebhookSecret != nil {
			query += fmt.Sprintf(", webhook_secret=$%d", argNum)
			args = append(args, *input.WebhookSecret)
			argNum++
		}
		
		query += fmt.Sprintf(" WHERE store_id=$%d AND id=$%d", argNum, argNum+1)
		args = append(args, store, input.ID)
		
		_, e = tx.Exec(r.Context(), query, args...)
		if e != nil {
			failure(w, 500, "payment_config_update_failed")
			return
		}
		e = tx.Commit(r.Context())
		if e != nil {
			failure(w, 500, "payment_config_update_failed")
			return
		}
		jsonResponse(w, 200, map[string]bool{"ok": true})
		return
	}
	
	if r.Method == "DELETE" {
		idStr := r.URL.Query().Get("id")
		if idStr == "" {
			failure(w, 400, "config_id_required")
			return
		}
		id := ParseInt(idStr)
		if id <= 0 {
			failure(w, 400, "config_id_invalid")
			return
		}
		_, e := a.DB.Exec(r.Context(), "DELETE FROM payment_configs WHERE store_id=$1 AND id=$2", store, id)
		if e != nil {
			failure(w, 500, "payment_config_delete_failed")
			return
		}
		jsonResponse(w, 200, map[string]bool{"ok": true})
		return
	}
	
	failure(w, 405, "method_not_allowed")
}

func (a *App) paymentsHandler(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanViewFinancials() {
		failure(w, 403, "forbidden_payments_access")
		return
	}
	
	if r.Method == "GET" {
		rows, e := a.DB.Query(r.Context(), `SELECT p.id,p.provider,p.external_id,p.amount_cents,p.currency,p.status,p.qr_code,p.pix_copy_paste,p.expires_at,p.confirmed_at,p.refunded_amount_cents,p.created_at,o.id as order_id
			FROM payments p LEFT JOIN orders o ON o.store_id=p.store_id AND o.id=p.order_id
			WHERE p.store_id=$1 ORDER BY p.created_at DESC LIMIT 200`, store)
		if e != nil {
			failure(w, 500, "payments_failed")
			return
		}
		defer rows.Close()
		result := []map[string]any{}
		for rows.Next() {
			var id, amountCents, refundedAmount, orderID int64
			var provider, externalID, currency, status, qrCode, pixCopyPaste string
			var expiresAt, confirmedAt, createdAt *time.Time
			if rows.Scan(&id, &provider, &externalID, &amountCents, &currency, &status, &qrCode, &pixCopyPaste, &expiresAt, &confirmedAt, &refundedAmount, &createdAt, &orderID) != nil {
				failure(w, 500, "payments_scan_failed")
				return
			}
			result = append(result, map[string]any{
				"id": id, "provider": provider, "external_id": externalID, "amount_cents": amountCents,
				"currency": currency, "status": status, "qr_code": qrCode, "pix_copy_paste": pixCopyPaste,
				"expires_at": expiresAt, "confirmed_at": confirmedAt, "refunded_amount_cents": refundedAmount,
				"created_at": createdAt, "order_id": orderID,
			})
		}
		jsonResponse(w, 200, result)
		return
	}
	
	if r.Method == "POST" {
		var input struct {
			OrderID          int64  `json:"order_id"`
			Provider         string `json:"provider"`
			AmountCents      int64  `json:"amount_cents"`
			Description      string `json:"description"`
			PayerName        string `json:"payer_name"`
			PayerEmail       string `json:"payer_email"`
			PayerPhone       string `json:"payer_phone"`
			PayerDocument    string `json:"payer_document"`
			ExpirationMinutes int   `json:"expiration_minutes"`
		}
		if decode(w, r, &input) != nil || input.OrderID <= 0 || input.Provider == "" || input.AmountCents <= 0 {
			failure(w, 400, "payment_create_invalid")
			return
		}
		
		// Get payment config
		var configID int64
		var credJSON, settingsJSON []byte
		var webhookSecret string
		e := a.DB.QueryRow(r.Context(), "SELECT id,credentials,settings,webhook_secret FROM payment_configs WHERE store_id=$1 AND provider=$2 AND enabled=true", store, input.Provider).Scan(&configID, &credJSON, &settingsJSON, &webhookSecret)
		if e != nil {
			failure(w, 404, "payment_config_not_found")
			return
		}
		
		var credentials, settings map[string]any
		json.Unmarshal(credJSON, &credentials)
		json.Unmarshal(settingsJSON, &settings)
		
		config := PaymentConfig{
			Provider:      input.Provider,
			Enabled:       true,
			Credentials:   map[string]string{},
			StoreID:       store,
			WebhookSecret: webhookSecret,
			Settings:      settings,
		}
		for k, v := range credentials {
			if s, ok := v.(string); ok {
				config.Credentials[k] = s
			}
		}
		
		connector, err := NewPaymentConnector(config)
		if err != nil {
			failure(w, 500, "connector_creation_failed")
			return
		}
		
		// Get order details
		var order struct {
			CustomerName  string
			CustomerPhone string
			CustomerEmail string
		}
		a.DB.QueryRow(r.Context(), "SELECT customer_name,customer_phone,customer_email FROM orders WHERE store_id=$1 AND id=$2", store, input.OrderID).Scan(&order.CustomerName, &order.CustomerPhone, &order.CustomerEmail)
		
		idempotencyKey := fmt.Sprintf("pay-%d-%s-%d", store, input.Provider, time.Now().UnixNano())
		
		chargeReq := ChargeRequest{
			OrderID:           input.OrderID,
			AmountCents:       input.AmountCents,
			Description:       input.Description,
			PayerName:         order.CustomerName,
			PayerEmail:        order.CustomerEmail,
			PayerPhone:        order.CustomerPhone,
			PayerDocument:     input.PayerDocument,
			IdempotencyKey:    idempotencyKey,
			ExpirationMinutes: input.ExpirationMinutes,
		}
		if chargeReq.ExpirationMinutes == 0 {
			chargeReq.ExpirationMinutes = 60
		}
		
		chargeResp, err := connector.CreateCharge(r.Context(), chargeReq)
		if err != nil {
			failure(w, 500, "charge_creation_failed: "+err.Error())
			return
		}
		
		// Save payment record
		var paymentID int64
		e = a.DB.QueryRow(r.Context(), `INSERT INTO payments(store_id,order_id,provider,external_id,amount_cents,currency,status,idempotency_key,qr_code,pix_copy_paste,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
			store, input.OrderID, input.Provider, chargeResp.ExternalID, chargeResp.AmountCents, "BRL", "pending", idempotencyKey, chargeResp.QRCode, chargeResp.PixCopyPaste, chargeResp.ExpiresAt).Scan(&paymentID)
		if e != nil {
			failure(w, 500, "payment_save_failed")
			return
		}
		
		// Update order financial state
		_, e = a.DB.Exec(r.Context(), "UPDATE orders SET financial_state='payment_pending' WHERE store_id=$1 AND id=$2", store, input.OrderID)
		if e != nil {
			failure(w, 500, "order_update_failed")
			return
		}
		
		jsonResponse(w, 201, map[string]any{
			"payment_id": paymentID,
			"external_id": chargeResp.ExternalID,
			"qr_code": chargeResp.QRCode,
			"pix_copy_paste": chargeResp.PixCopyPaste,
			"expires_at": chargeResp.ExpiresAt,
		})
		return
	}
	
	failure(w, 405, "method_not_allowed")
}

func (a *App) paymentWebhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		failure(w, 405, "method_not_allowed")
		return
	}
	
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		failure(w, 400, "provider_required")
		return
	}
	
	// Get payment config for this provider
	var storeID int64
	var credJSON, settingsJSON []byte
	var webhookSecret string
	e := a.DB.QueryRow(r.Context(), "SELECT store_id,credentials,settings,webhook_secret FROM payment_configs WHERE provider=$1 AND enabled=true LIMIT 1", provider).Scan(&storeID, &credJSON, &settingsJSON, &webhookSecret)
	if e != nil {
		failure(w, 404, "payment_config_not_found")
		return
	}
	
	var credentials, settings map[string]any
	json.Unmarshal(credJSON, &credentials)
	json.Unmarshal(settingsJSON, &settings)
	
	config := PaymentConfig{
		Provider:      provider,
		Enabled:       true,
		Credentials:   map[string]string{},
		StoreID:       storeID,
		WebhookSecret: webhookSecret,
		Settings:      settings,
	}
	for k, v := range credentials {
		if s, ok := v.(string); ok {
			config.Credentials[k] = s
		}
	}
	
	connector, err := NewPaymentConnector(config)
	if err != nil {
		failure(w, 500, "connector_creation_failed")
		return
	}
	
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		failure(w, 400, "webhook_body_invalid")
		return
	}
	
	status, err := connector.ProcessWebhook(r.Context(), body, r.Header)
	if err != nil {
		// Log error but don't fail - some providers require 200 OK
		log.Printf("webhook processing error for %s: %v", provider, err)
	}
	
	if status != nil {
		// Update payment record
		tx, err := a.DB.Begin(r.Context())
		if err != nil {
			log.Printf("webhook db error: %v", err)
		} else {
			var paymentID int64
			err = tx.QueryRow(r.Context(), "SELECT id FROM payments WHERE store_id=$1 AND provider=$2 AND external_id=$3", storeID, provider, status.ExternalID).Scan(&paymentID)
			if err == nil {
				var confirmedAt interface{}
				if status.PaidAt != nil {
					confirmedAt = *status.PaidAt
				}
				_, err = tx.Exec(r.Context(), `UPDATE payments SET status=$1,confirmed_at=$2,raw_webhook=$3,updated_at=now() WHERE id=$4`,
					status.Status, confirmedAt, body, paymentID)
				if err == nil {
					// Update order financial state
					if status.Status == "confirmed" {
						_, err = tx.Exec(r.Context(), `UPDATE orders SET financial_state='payment_confirmed' WHERE store_id=$1 AND id=(SELECT order_id FROM payments WHERE id=$2)`, storeID, paymentID)
					} else if status.Status == "expired" || status.Status == "cancelled" {
						_, err = tx.Exec(r.Context(), `UPDATE orders SET financial_state='payment_expired' WHERE store_id=$1 AND id=(SELECT order_id FROM payments WHERE id=$2)`, storeID, paymentID)
					}
				}
				if err == nil {
					tx.Commit(r.Context())
				} else {
					tx.Rollback(r.Context())
				}
			} else {
				tx.Rollback(r.Context())
			}
		}
	}
	
	jsonResponse(w, 200, map[string]bool{"ok": true})
}

func (a *App) paymentRefundHandler(w http.ResponseWriter, r *http.Request, store int64, user *SessionUser) {
	if !user.CanViewFinancials() {
		failure(w, 403, "forbidden_refund_access")
		return
	}
	
	if r.Method != "POST" {
		failure(w, 405, "method_not_allowed")
		return
	}
	
	var input struct {
		PaymentID     int64  `json:"payment_id"`
		AmountCents   int64  `json:"amount_cents"`
		Reason        string `json:"reason"`
	}
	if decode(w, r, &input) != nil || input.PaymentID <= 0 || input.AmountCents <= 0 || input.Reason == "" {
		failure(w, 400, "refund_invalid")
		return
	}
	
	// Get payment
	var payment struct {
		ID           int64
		Provider     string
		ExternalID   string
		AmountCents  int64
		Status       string
		RefundedCents int64
	}
	e := a.DB.QueryRow(r.Context(), "SELECT id,provider,external_id,amount_cents,status,refunded_amount_cents FROM payments WHERE store_id=$1 AND id=$2", store, input.PaymentID).Scan(&payment.ID, &payment.Provider, &payment.ExternalID, &payment.AmountCents, &payment.Status, &payment.RefundedCents)
	if e != nil {
		failure(w, 404, "payment_not_found")
		return
	}
	
	if payment.Status != "confirmed" {
		failure(w, 409, "payment_not_confirmed")
		return
	}
	
	if payment.RefundedCents+input.AmountCents > payment.AmountCents {
		failure(w, 400, "refund_exceeds_amount")
		return
	}
	
	// Get payment config
	var configID int64
	var credJSON, settingsJSON []byte
	var webhookSecret string
	e = a.DB.QueryRow(r.Context(), "SELECT id,credentials,settings,webhook_secret FROM payment_configs WHERE store_id=$1 AND provider=$2 AND enabled=true", store, payment.Provider).Scan(&configID, &credJSON, &settingsJSON, &webhookSecret)
	if e != nil {
		failure(w, 404, "payment_config_not_found")
		return
	}
	
	var credentials, settings map[string]any
	json.Unmarshal(credJSON, &credentials)
	json.Unmarshal(settingsJSON, &settings)
	
	config := PaymentConfig{
		Provider:      payment.Provider,
		Enabled:       true,
		Credentials:   map[string]string{},
		StoreID:       store,
		WebhookSecret: webhookSecret,
		Settings:      settings,
	}
	for k, v := range credentials {
		if s, ok := v.(string); ok {
			config.Credentials[k] = s
		}
	}
	
	connector, err := NewPaymentConnector(config)
	if err != nil {
		failure(w, 500, "connector_creation_failed")
		return
	}
	
	idempotencyKey := fmt.Sprintf("refund-%d-%s-%d", store, payment.Provider, time.Now().UnixNano())
	
	refundReq := RefundRequest{
		PaymentID:      payment.ExternalID,
		AmountCents:    input.AmountCents,
		Reason:         input.Reason,
		IdempotencyKey: idempotencyKey,
	}
	
	refundResp, err := connector.Refund(r.Context(), refundReq)
	if err != nil {
		failure(w, 500, "refund_failed: "+err.Error())
		return
	}
	
	// Save refund record (could add refunds table, for now update payment)
	_, e = a.DB.Exec(r.Context(), `UPDATE payments SET refunded_amount_cents=refunded_amount_cents+$1,status=CASE WHEN refunded_amount_cents+$1>=amount_cents THEN 'refunded' ELSE 'refund_partial' END,updated_at=now() WHERE id=$2`,
		input.AmountCents, payment.ID)
	if e != nil {
		failure(w, 500, "refund_save_failed")
		return
	}
	
	jsonResponse(w, 200, map[string]any{
		"refund_id": refundResp.RefundID,
		"status": refundResp.Status,
		"amount_cents": refundResp.AmountCents,
	})
}

func ParseInt(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
