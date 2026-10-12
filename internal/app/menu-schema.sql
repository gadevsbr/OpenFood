ALTER TABLE products ADD COLUMN configuration_version bigint NOT NULL DEFAULT 1;
ALTER TABLE order_items ADD COLUMN selection jsonb NOT NULL DEFAULT '{}';
ALTER TABLE addon_groups DROP CONSTRAINT IF EXISTS addon_groups_store_id_name_key;
CREATE TABLE product_photos(
 store_id bigint NOT NULL,product_id bigint NOT NULL,data bytea NOT NULL CHECK(octet_length(data) BETWEEN 1 AND 2097152),
 updated_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(store_id,product_id),
 FOREIGN KEY(store_id,product_id) REFERENCES products(store_id,id) ON DELETE CASCADE
);
