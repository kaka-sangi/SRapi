-- Drop billing mode, pricing interval, and usage cost breakdown additions.
ALTER TABLE "usage_logs" DROP COLUMN IF EXISTS "billing_mode";
ALTER TABLE "usage_logs" DROP COLUMN IF EXISTS "upstream_model";
ALTER TABLE "usage_logs" DROP COLUMN IF EXISTS "requested_model";
ALTER TABLE "usage_logs" DROP COLUMN IF EXISTS "cache_write_cost";
ALTER TABLE "usage_logs" DROP COLUMN IF EXISTS "cache_read_cost";
ALTER TABLE "usage_logs" DROP COLUMN IF EXISTS "output_cost";
ALTER TABLE "usage_logs" DROP COLUMN IF EXISTS "input_cost";

DROP TABLE IF EXISTS "pricing_intervals";

ALTER TABLE "pricing_rules" DROP COLUMN IF EXISTS "per_request_price";
ALTER TABLE "pricing_rules" DROP COLUMN IF EXISTS "billing_mode";
