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
CREATE INDEX idx_payments_status ON payments(status);