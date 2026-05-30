-- Modify "account_group_rate_limits" table
ALTER TABLE "account_group_rate_limits" ADD COLUMN "max_concurrency" bigint NOT NULL DEFAULT 0;
