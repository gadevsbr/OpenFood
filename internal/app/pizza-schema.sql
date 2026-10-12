ALTER TABLE products ADD COLUMN pizza_configuration jsonb NOT NULL DEFAULT '{"enabled":false,"pricing_policy":"highest","max_flavors":2,"flavors":[]}'::jsonb;
