-- Remove materialized subscription spend counters and API key USD cost quotas.
ALTER TABLE "api_keys" DROP COLUMN "cost_window_start_7d";
ALTER TABLE "api_keys" DROP COLUMN "cost_used_7d";
ALTER TABLE "api_keys" DROP COLUMN "cost_limit_7d";
ALTER TABLE "api_keys" DROP COLUMN "cost_window_start_1d";
ALTER TABLE "api_keys" DROP COLUMN "cost_used_1d";
ALTER TABLE "api_keys" DROP COLUMN "cost_limit_1d";
ALTER TABLE "api_keys" DROP COLUMN "cost_window_start_5h";
ALTER TABLE "api_keys" DROP COLUMN "cost_used_5h";
ALTER TABLE "api_keys" DROP COLUMN "cost_limit_5h";
ALTER TABLE "api_keys" DROP COLUMN "cost_used";
ALTER TABLE "api_keys" DROP COLUMN "cost_quota";

ALTER TABLE "user_subscriptions" DROP COLUMN "monthly_usage_window_start";
ALTER TABLE "user_subscriptions" DROP COLUMN "monthly_usage_usd";
ALTER TABLE "user_subscriptions" DROP COLUMN "weekly_usage_window_start";
ALTER TABLE "user_subscriptions" DROP COLUMN "weekly_usage_usd";
ALTER TABLE "user_subscriptions" DROP COLUMN "daily_usage_window_start";
ALTER TABLE "user_subscriptions" DROP COLUMN "daily_usage_usd";
