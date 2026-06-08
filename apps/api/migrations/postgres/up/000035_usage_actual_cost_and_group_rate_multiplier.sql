-- Add account-group billing multipliers and usage-log actual cost snapshots.
ALTER TABLE "account_groups" ADD COLUMN "rate_multiplier" character varying NOT NULL DEFAULT '1.00000000';

ALTER TABLE "usage_logs" ADD COLUMN "actual_cost" character varying NOT NULL DEFAULT '0.00000000';
ALTER TABLE "usage_logs" ADD COLUMN "rate_multiplier" character varying NOT NULL DEFAULT '1.00000000';

-- Existing rows predate account-group multipliers, so their actual charged cost
-- equals the recorded standard cost.
UPDATE "usage_logs" SET "actual_cost" = "cost";
