-- Drop the WP-1180 billable_cost column.
ALTER TABLE "usage_logs" DROP COLUMN IF EXISTS "billable_cost";
