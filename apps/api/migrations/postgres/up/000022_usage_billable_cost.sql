-- Modify "usage_logs" table
ALTER TABLE "usage_logs" ADD COLUMN "billable_cost" character varying NOT NULL DEFAULT '0.00000000';
-- Backfill existing rows so the balance charger (which now sums billable_cost)
-- bills the full cost for usage recorded before WP-1180.
UPDATE "usage_logs" SET "billable_cost" = "cost";
