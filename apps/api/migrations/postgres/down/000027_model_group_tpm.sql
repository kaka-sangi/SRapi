-- Drop the WP-1260 per-model / per-group tpm_limit columns.
ALTER TABLE "model_rate_limits" DROP COLUMN IF EXISTS "tpm_limit";
ALTER TABLE "account_group_rate_limits" DROP COLUMN IF EXISTS "tpm_limit";
