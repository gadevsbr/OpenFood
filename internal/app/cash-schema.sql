CREATE TABLE cash_sessions (
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
