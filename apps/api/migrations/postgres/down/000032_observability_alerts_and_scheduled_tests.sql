-- Drop the scheduled account health-test plan tables and the configurable
-- observability alert-rule + alert-silence tables.
DROP TABLE IF EXISTS "scheduled_test_plans";
DROP TABLE IF EXISTS "scheduled_test_plan_runs";
DROP TABLE IF EXISTS "obs_alert_silences";
DROP TABLE IF EXISTS "obs_alert_rules";
