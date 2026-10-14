ALTER TABLE stores ADD COLUMN timezone text NOT NULL DEFAULT 'America/Sao_Paulo';
ALTER TABLE stores ADD COLUMN sale_schedule jsonb NOT NULL DEFAULT '{"enabled":false,"windows":[]}';
ALTER TABLE stores ADD COLUMN schedule_version bigint NOT NULL DEFAULT 1;
ALTER TABLE categories ADD COLUMN sale_schedule jsonb NOT NULL DEFAULT '{"enabled":false,"windows":[]}';
ALTER TABLE categories ADD COLUMN schedule_version bigint NOT NULL DEFAULT 1;

CREATE FUNCTION openfood_schedule_open(rule jsonb, local_at timestamp) RETURNS boolean
LANGUAGE sql IMMUTABLE AS $$
 SELECT NOT (rule->>'enabled')::boolean OR EXISTS (
  SELECT 1 FROM jsonb_array_elements(rule->'windows') w
  CROSS JOIN LATERAL (SELECT extract(dow FROM local_at)::int AS day,
    extract(hour FROM local_at)*60+extract(minute FROM local_at)+extract(second FROM local_at)/60 AS minute) t
  WHERE CASE WHEN (w->>'start')::int < (w->>'end')::int
   THEN t.day=(w->>'day')::int AND t.minute >= (w->>'start')::int AND t.minute < (w->>'end')::int
   ELSE (t.day=(w->>'day')::int AND t.minute >= (w->>'start')::int)
     OR (t.day=((w->>'day')::int+1)%7 AND t.minute < (w->>'end')::int) END
 )
$$;

CREATE FUNCTION openfood_product_open(wanted_store bigint, wanted_product bigint, at_time timestamptz)
RETURNS boolean LANGUAGE sql STABLE AS $$
 WITH RECURSIVE category_chain AS (
  SELECT c.id,c.parent_id,c.active,c.sale_schedule,ARRAY[c.id] AS path
  FROM products p JOIN categories c ON c.id=p.category_id AND c.store_id=p.store_id
  WHERE p.id=wanted_product AND p.store_id=wanted_store
  UNION ALL
  SELECT c.id,c.parent_id,c.active,c.sale_schedule,ch.path||c.id
  FROM category_chain ch JOIN categories c ON c.id=ch.parent_id AND c.store_id=wanted_store
  WHERE NOT c.id=ANY(ch.path)
 )
 SELECT COALESCE((SELECT openfood_schedule_open(s.sale_schedule, at_time AT TIME ZONE s.timezone)
  AND NOT EXISTS(SELECT 1 FROM category_chain ch WHERE NOT ch.active
    OR NOT openfood_schedule_open(ch.sale_schedule,at_time AT TIME ZONE s.timezone)
    OR ch.parent_id=ANY(ch.path)
    OR (ch.parent_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM categories c WHERE c.id=ch.parent_id AND c.store_id=s.id)))
  AND (p.category_id IS NULL OR EXISTS(SELECT 1 FROM category_chain))
  AND CASE WHEN p.available_from IS NULL OR p.available_until IS NULL THEN true
    WHEN p.available_from<=p.available_until THEN (at_time AT TIME ZONE s.timezone)::time BETWEEN p.available_from AND p.available_until
    ELSE (at_time AT TIME ZONE s.timezone)::time>=p.available_from OR (at_time AT TIME ZONE s.timezone)::time<=p.available_until END
  FROM products p JOIN stores s ON s.id=p.store_id WHERE p.id=wanted_product AND p.store_id=wanted_store),false)
$$;
