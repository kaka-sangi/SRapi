-- Reverse the changes from "000052_ops_error_log_api_key_prefix.sql"
DROP INDEX IF EXISTS "opserrorlog_api_key_prefix_occurred_at";

ALTER TABLE "ops_error_logs"
  DROP COLUMN IF EXISTS "api_key_prefix";
