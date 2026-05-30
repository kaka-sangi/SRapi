-- Modify "account_group_rate_limits" table
ALTER TABLE "account_group_rate_limits" ADD COLUMN "tpm_limit" bigint NOT NULL DEFAULT 0;
-- Modify "model_rate_limits" table
ALTER TABLE "model_rate_limits" ADD COLUMN "tpm_limit" bigint NOT NULL DEFAULT 0;
