ALTER TABLE order_items ALTER COLUMN product_id DROP NOT NULL;
ALTER TABLE customers ADD COLUMN anonymized_at timestamptz;
ALTER TABLE consents ADD COLUMN terms_text text NOT NULL DEFAULT '';
