DROP INDEX "usagelog_aggregated_at_success_created_at";
ALTER TABLE "usage_logs" DROP COLUMN IF EXISTS "aggregated_at";
