-- Drop account-group billing multiplier and usage-log actual cost snapshots.
ALTER TABLE "usage_logs" DROP COLUMN IF EXISTS "rate_multiplier";
ALTER TABLE "usage_logs" DROP COLUMN IF EXISTS "actual_cost";

ALTER TABLE "account_groups" DROP COLUMN IF EXISTS "rate_multiplier";
