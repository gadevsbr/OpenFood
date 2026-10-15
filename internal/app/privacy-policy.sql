-- Append-only removal identities survive restoring an older snapshot via the supported restore path.
-- Historical orders have no last-transition timestamp: start their clock at
-- migration rather than risk removing a recently completed old order.
ALTER TABLE orders ADD COLUMN privacy_activity_at timestamptz NOT NULL DEFAULT now();
CREATE FUNCTION privacy_order_activity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN NEW.privacy_activity_at=now(); RETURN NEW; END $$;
CREATE TRIGGER privacy_order_activity BEFORE UPDATE OF state,financial_state ON orders
 FOR EACH ROW EXECUTE FUNCTION privacy_order_activity();
CREATE TABLE privacy_removals (
 store_id bigint NOT NULL, store_created_at timestamptz NOT NULL,
 customer_id bigint NOT NULL, customer_created_at timestamptz NOT NULL,
 removed_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(store_id,store_created_at,customer_id,customer_created_at)
);
CREATE TABLE privacy_policies (
 store_id bigint PRIMARY KEY REFERENCES stores(id),
 revision text NOT NULL DEFAULT md5(random()::text||clock_timestamp()::text),
 version bigint NOT NULL DEFAULT 1,
 enabled boolean NOT NULL DEFAULT false,
 inactive_days integer NOT NULL DEFAULT 0 CHECK(inactive_days BETWEEN 0 AND 36500),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(NOT enabled OR inactive_days>0)
);
INSERT INTO privacy_policies(store_id) SELECT id FROM stores;
INSERT INTO privacy_removals(store_id,store_created_at,customer_id,customer_created_at,removed_at)
 SELECT c.store_id,s.created_at,c.id,c.created_at,c.anonymized_at FROM customers c JOIN stores s ON s.id=c.store_id WHERE c.anonymized_at IS NOT NULL;
