-- Drop the WP-1210 per-account-group max_concurrency column.
ALTER TABLE "account_group_rate_limits" DROP COLUMN IF EXISTS "max_concurrency";
