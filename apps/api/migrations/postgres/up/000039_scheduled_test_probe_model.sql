-- Add explicit scheduled test probe model override.
ALTER TABLE "scheduled_test_plans" ADD COLUMN "probe_model" character varying NOT NULL DEFAULT '';
