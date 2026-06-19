-- Add low-cardinality stream terminal evidence to operator-facing gateway errors.
-- Values are controlled by the gateway runtime (for example: completed,
-- interrupted, idle_timeout, failed, unknown) and must not contain provider
-- native frames, prompt text, request bodies, credentials, or headers.
ALTER TABLE "ops_error_logs"
  ADD COLUMN "stream_completion_state" character varying NOT NULL DEFAULT '';

CREATE INDEX "opserrorlog_stream_completion_state_occurred_at"
  ON "ops_error_logs" ("stream_completion_state", "occurred_at");
