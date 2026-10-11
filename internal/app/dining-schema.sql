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
