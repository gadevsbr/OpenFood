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
