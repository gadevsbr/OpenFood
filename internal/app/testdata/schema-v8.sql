CREATE TABLE organizations(
    id bigserial PRIMARY KEY,
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE stores(
    id bigserial PRIMARY KEY,
    org_id bigint NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users(
    id bigserial PRIMARY KEY,
    org_id bigint NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    store_id bigint REFERENCES stores(id) ON DELETE SET NULL,
    email text UNIQUE NOT NULL,
    password_hash text NOT NULL,
    role text NOT NULL CHECK(role IN ('instance_admin','org_admin','store_manager','attendant','kitchen','dispatch','finance_viewer')),
    active boolean NOT NULL DEFAULT true,
    token_version integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions(
    token_hash text PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_version integer NOT NULL DEFAULT 1,
    expires_at timestamptz NOT NULL
);

-- Advanced Catalog Tables
CREATE TABLE categories(
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

CREATE TABLE products(
    id bigserial PRIMARY KEY,
    store_id bigint NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    category_id bigint REFERENCES categories(id) ON DELETE SET NULL,
    name text NOT NULL,
    description text,
    price_cents bigint NOT NULL CHECK(price_cents BETWEEN 0 AND 100000000),
    stock_quantity numeric(12,3) NOT NULL DEFAULT 0 CHECK(stock_quantity >= 0),
    unit text NOT NULL DEFAULT 'unidade' CHECK(unit IN ('unidade','kg','g','l','ml','porcao')),
    sku text,
    barcode text,
    active boolean NOT NULL DEFAULT true,
    available_from time,
    available_until time,
    min_order_quantity numeric(12,3) NOT NULL DEFAULT 1 CHECK(min_order_quantity > 0),
    max_order_quantity numeric(12,3) CHECK(max_order_quantity >= min_order_quantity),
    is_weight_based boolean NOT NULL DEFAULT false,
    substitution_allowed boolean NOT NULL DEFAULT true,
    preparation_time_minutes integer NOT NULL DEFAULT 0,
    display_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(store_id, id)
);
CREATE UNIQUE INDEX idx_products_store_sku ON products(store_id, sku) WHERE sku IS NOT NULL;

CREATE TABLE product_variants(
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

CREATE TABLE addon_groups(
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

CREATE TABLE addon_options(
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

CREATE TABLE product_addon_groups(
    product_id bigint NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    store_id bigint NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    group_id bigint NOT NULL REFERENCES addon_groups(id) ON DELETE CASCADE,
    display_order integer NOT NULL DEFAULT 0,
    PRIMARY KEY(product_id, group_id)
);

CREATE TABLE combos(
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

CREATE TABLE combo_items(
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

CREATE TABLE product_images(
    id bigserial PRIMARY KEY,
    product_id bigint NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    store_id bigint NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    url text NOT NULL,
    alt_text text,
    display_order integer NOT NULL DEFAULT 0,
    is_primary boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE orders(
    id bigserial PRIMARY KEY,
    store_id bigint NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    idempotency_key text NOT NULL,
    request_hash text NOT NULL,
    total_cents bigint NOT NULL CHECK(total_cents>=0),
    state text NOT NULL CHECK(state IN ('draft','awaiting_confirmation','awaiting_payment','confirmed','preparing','ready','completed','cancelled')),
    financial_state text NOT NULL DEFAULT 'manual_pending' CHECK(financial_state IN ('manual_pending','manual_confirmed','payment_pending','payment_confirmed','payment_expired','refund_pending','refund_partial','refunded')),
    customer_id bigint,
    customer_name text,
    customer_phone text,
    customer_email text,
    delivery_type text CHECK(delivery_type IN ('takeaway','delivery')),
    delivery_address_id bigint,
    delivery_fee_cents bigint NOT NULL DEFAULT 0,
    notes text,
    created_at timestamptz NOT NULL DEFAULT now(),
    confirmed_at timestamptz,
    UNIQUE(store_id,idempotency_key),
    UNIQUE(store_id,id)
);

CREATE TABLE order_items(
    id bigserial PRIMARY KEY,
    order_id bigint NOT NULL,
    store_id bigint NOT NULL,
    product_id bigint NOT NULL,
    variant_id bigint,
    name text NOT NULL,
    price_cents bigint NOT NULL,
    quantity numeric(12,3) NOT NULL CHECK(quantity > 0),
    unit text NOT NULL DEFAULT 'unidade',
    notes text,
    FOREIGN KEY(store_id,order_id) REFERENCES orders(store_id,id) ON DELETE CASCADE,
    FOREIGN KEY(store_id,product_id) REFERENCES products(store_id,id) ON DELETE CASCADE
);

CREATE TABLE order_item_addons(
    id bigserial PRIMARY KEY,
    order_item_id bigint NOT NULL REFERENCES order_items(id) ON DELETE CASCADE,
    store_id bigint NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    addon_group_id bigint NOT NULL REFERENCES addon_groups(id) ON DELETE CASCADE,
    addon_option_id bigint NOT NULL REFERENCES addon_options(id) ON DELETE CASCADE,
    price_cents bigint NOT NULL DEFAULT 0,
    quantity numeric(12,3) NOT NULL DEFAULT 1 CHECK(quantity > 0)
);

CREATE TABLE audit(
    id bigserial PRIMARY KEY,
    store_id bigint NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    user_id bigint REFERENCES users(id) ON DELETE SET NULL,
    action text NOT NULL,
    entity_id bigint,
    details jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE jobs(
    id bigserial PRIMARY KEY,
    store_id bigint NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    kind text NOT NULL,
    payload jsonb NOT NULL,
    dedup_key text NOT NULL UNIQUE,
    attempts integer NOT NULL DEFAULT 0,
    available_at timestamptz NOT NULL DEFAULT now(),
    lease_until timestamptz,
    state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','running','done','failed')),
    last_error_code text
);

CREATE TABLE customers(
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
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(store_id, id)
);
CREATE UNIQUE INDEX idx_customers_store_phone ON customers(store_id, phone) WHERE phone IS NOT NULL;
CREATE UNIQUE INDEX idx_customers_store_email ON customers(store_id, email) WHERE email IS NOT NULL;

CREATE TABLE consents(
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

CREATE TABLE payments(
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

CREATE INDEX jobs_available ON jobs(available_at) WHERE state IN ('pending','running');
CREATE INDEX idx_products_store_category ON products(store_id, category_id);
CREATE INDEX idx_products_store_active ON products(store_id, active) WHERE active = true;
CREATE INDEX idx_categories_store_parent ON categories(store_id, parent_id);
CREATE INDEX idx_order_items_order ON order_items(order_id);
CREATE INDEX idx_payments_order ON payments(order_id);
CREATE INDEX idx_payments_status ON payments(status);CREATE TABLE cash_sessions (
 id bigserial PRIMARY KEY, store_id bigint NOT NULL REFERENCES stores(id),
 opened_by bigint NOT NULL REFERENCES users(id), opened_at timestamptz NOT NULL DEFAULT now(),
 opening_cents bigint NOT NULL CHECK(opening_cents BETWEEN 0 AND 1000000000000),
 closed_by bigint REFERENCES users(id), closed_at timestamptz, counted_cents bigint CHECK(counted_cents BETWEEN 0 AND 1000000000000),
 expected_cents bigint, difference_cents bigint, note text NOT NULL DEFAULT '', close_note text NOT NULL DEFAULT '',
 idempotency_key text NOT NULL, request_hash text NOT NULL, close_key text, close_hash text,
 UNIQUE(store_id,close_key), UNIQUE(store_id,id), UNIQUE(store_id,idempotency_key),
 CHECK ((closed_at IS NULL AND closed_by IS NULL AND counted_cents IS NULL AND expected_cents IS NULL AND difference_cents IS NULL) OR
 (closed_at IS NOT NULL AND closed_by IS NOT NULL AND counted_cents IS NOT NULL AND expected_cents IS NOT NULL AND difference_cents=counted_cents-expected_cents))
);
CREATE UNIQUE INDEX cash_one_open ON cash_sessions(store_id) WHERE closed_at IS NULL;
CREATE TABLE manual_receipts (
 id bigserial PRIMARY KEY, store_id bigint NOT NULL REFERENCES stores(id),
 session_id bigint NOT NULL, order_id bigint NOT NULL,
 method text NOT NULL CHECK(method IN ('cash','card','pix_manual')),
 amount_cents bigint NOT NULL CHECK(amount_cents<>0 AND amount_cents BETWEEN -1000000000000 AND 1000000000000),
 currency text NOT NULL DEFAULT 'BRL' CHECK(currency='BRL'),
 user_id bigint NOT NULL REFERENCES users(id), created_at timestamptz NOT NULL DEFAULT now(),
 note text NOT NULL DEFAULT '', refund_of bigint, idempotency_key text NOT NULL, request_hash text NOT NULL,
 UNIQUE(store_id,id), UNIQUE(store_id,idempotency_key), UNIQUE(refund_of),
 FOREIGN KEY(store_id,session_id) REFERENCES cash_sessions(store_id,id),
 FOREIGN KEY(store_id,order_id) REFERENCES orders(store_id,id),
 FOREIGN KEY(store_id,refund_of) REFERENCES manual_receipts(store_id,id),
 CHECK((amount_cents>0 AND refund_of IS NULL) OR (amount_cents<0 AND refund_of IS NOT NULL AND length(note)>0))
);
CREATE INDEX manual_receipts_order ON manual_receipts(store_id,order_id);
CREATE INDEX manual_receipts_session ON manual_receipts(store_id,session_id);
CREATE TABLE pos_checkouts(
 id bigserial PRIMARY KEY, store_id bigint NOT NULL REFERENCES stores(id), order_id bigint NOT NULL,
 user_id bigint NOT NULL REFERENCES users(id), session_id bigint NOT NULL,
 idempotency_key text NOT NULL, request_hash text NOT NULL, discount_cents bigint NOT NULL DEFAULT 0 CHECK(discount_cents>=0), discount_note text NOT NULL DEFAULT '', service text NOT NULL CHECK(service IN ('counter','kitchen')),
 payments jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(store_id,idempotency_key), UNIQUE(store_id,order_id),
 FOREIGN KEY(store_id,order_id) REFERENCES orders(store_id,id), FOREIGN KEY(store_id,session_id) REFERENCES cash_sessions(store_id,id)
);
CREATE TABLE cash_adjustments(
 id bigserial PRIMARY KEY,store_id bigint NOT NULL REFERENCES stores(id),session_id bigint NOT NULL,user_id bigint NOT NULL REFERENCES users(id),
 amount_cents bigint NOT NULL CHECK(amount_cents<>0 AND amount_cents BETWEEN -1000000000000 AND 1000000000000),note text NOT NULL CHECK(length(note)>0),created_at timestamptz NOT NULL DEFAULT now(),
 idempotency_key text NOT NULL,request_hash text NOT NULL,UNIQUE(store_id,idempotency_key),
 FOREIGN KEY(store_id,session_id) REFERENCES cash_sessions(store_id,id)
);
CREATE INDEX cash_adjustments_session ON cash_adjustments(store_id,session_id);
CREATE TABLE dining_tables(
 id bigserial PRIMARY KEY,store_id bigint NOT NULL REFERENCES stores(id),name text NOT NULL CHECK(length(name) BETWEEN 1 AND 80),capacity integer NOT NULL DEFAULT 4 CHECK(capacity BETWEEN 1 AND 100),active boolean NOT NULL DEFAULT true,
 UNIQUE(store_id,id),UNIQUE(store_id,name)
);
CREATE TABLE dining_tabs(
 id bigserial PRIMARY KEY,store_id bigint NOT NULL REFERENCES stores(id),table_id bigint,label text NOT NULL CHECK(length(label) BETWEEN 1 AND 80),opened_by bigint NOT NULL REFERENCES users(id),opened_at timestamptz NOT NULL DEFAULT now(),closed_by bigint REFERENCES users(id),closed_at timestamptz,state text NOT NULL DEFAULT 'open' CHECK(state IN ('open','closed','cancelled')),version bigint NOT NULL DEFAULT 1,closing_payments jsonb NOT NULL DEFAULT '[]',
 UNIQUE(store_id,id),FOREIGN KEY(store_id,table_id) REFERENCES dining_tables(store_id,id)
);
CREATE UNIQUE INDEX dining_one_open_table ON dining_tabs(store_id,table_id) WHERE state='open' AND table_id IS NOT NULL;
CREATE UNIQUE INDEX dining_one_open_label ON dining_tabs(store_id,lower(label)) WHERE state='open';
CREATE TABLE dining_orders(
 store_id bigint NOT NULL,tab_id bigint NOT NULL,order_id bigint NOT NULL,PRIMARY KEY(store_id,order_id),
 FOREIGN KEY(store_id,tab_id) REFERENCES dining_tabs(store_id,id),FOREIGN KEY(store_id,order_id) REFERENCES orders(store_id,id)
);
CREATE INDEX dining_orders_tab ON dining_orders(store_id,tab_id);
CREATE TABLE dining_operations(
 store_id bigint NOT NULL REFERENCES stores(id),idempotency_key text NOT NULL,request_hash text NOT NULL,result_id bigint NOT NULL,action text NOT NULL,PRIMARY KEY(store_id,idempotency_key)
);
ALTER TABLE products ADD COLUMN configuration_version bigint NOT NULL DEFAULT 1;
ALTER TABLE order_items ADD COLUMN selection jsonb NOT NULL DEFAULT '{}';
ALTER TABLE addon_groups DROP CONSTRAINT IF EXISTS addon_groups_store_id_name_key;
CREATE TABLE product_photos(
 store_id bigint NOT NULL,product_id bigint NOT NULL,data bytea NOT NULL CHECK(octet_length(data) BETWEEN 1 AND 2097152),
 updated_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(store_id,product_id),
 FOREIGN KEY(store_id,product_id) REFERENCES products(store_id,id) ON DELETE CASCADE
);
ALTER TABLE order_items ALTER COLUMN product_id DROP NOT NULL;
ALTER TABLE customers ADD COLUMN anonymized_at timestamptz;
ALTER TABLE consents ADD COLUMN terms_text text NOT NULL DEFAULT '';
