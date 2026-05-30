-- Drop the WP-1220 per-model max_concurrency column.
ALTER TABLE "model_rate_limits" DROP COLUMN IF EXISTS "max_concurrency";
