-- Remove scheduled test probe model override.
ALTER TABLE "scheduled_test_plans" DROP COLUMN IF EXISTS "probe_model";
