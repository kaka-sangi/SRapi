-- Reverse the changes from "000054_ops_error_log_stream_completion_state.sql"
DROP INDEX IF EXISTS "opserrorlog_stream_completion_state_occurred_at";

ALTER TABLE "ops_error_logs"
  DROP COLUMN IF EXISTS "stream_completion_state";
