-- Add materialized subscription spend counters and API key USD cost quotas.
ALTER TABLE "user_subscriptions" ADD COLUMN "daily_usage_usd" character varying NOT NULL DEFAULT '0.00000000';
ALTER TABLE "user_subscriptions" ADD COLUMN "daily_usage_window_start" timestamptz NULL;
ALTER TABLE "user_subscriptions" ADD COLUMN "weekly_usage_usd" character varying NOT NULL DEFAULT '0.00000000';
ALTER TABLE "user_subscriptions" ADD COLUMN "weekly_usage_window_start" timestamptz NULL;
ALTER TABLE "user_subscriptions" ADD COLUMN "monthly_usage_usd" character varying NOT NULL DEFAULT '0.00000000';
ALTER TABLE "user_subscriptions" ADD COLUMN "monthly_usage_window_start" timestamptz NULL;

ALTER TABLE "api_keys" ADD COLUMN "cost_quota" character varying NULL;
ALTER TABLE "api_keys" ADD COLUMN "cost_used" character varying NOT NULL DEFAULT '0.00000000';
ALTER TABLE "api_keys" ADD COLUMN "cost_limit_5h" character varying NULL;
ALTER TABLE "api_keys" ADD COLUMN "cost_used_5h" character varying NOT NULL DEFAULT '0.00000000';
ALTER TABLE "api_keys" ADD COLUMN "cost_window_start_5h" timestamptz NULL;
ALTER TABLE "api_keys" ADD COLUMN "cost_limit_1d" character varying NULL;
ALTER TABLE "api_keys" ADD COLUMN "cost_used_1d" character varying NOT NULL DEFAULT '0.00000000';
ALTER TABLE "api_keys" ADD COLUMN "cost_window_start_1d" timestamptz NULL;
ALTER TABLE "api_keys" ADD COLUMN "cost_limit_7d" character varying NULL;
ALTER TABLE "api_keys" ADD COLUMN "cost_used_7d" character varying NOT NULL DEFAULT '0.00000000';
ALTER TABLE "api_keys" ADD COLUMN "cost_window_start_7d" timestamptz NULL;
