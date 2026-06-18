-- Snapshot the authenticated gateway API key prefix on ops_error_logs rows.
-- This is low-sensitivity operational evidence: the raw key remains hashed
-- only in api_keys, while the prefix survives later key deletion or renaming.
ALTER TABLE "ops_error_logs"
  ADD COLUMN "api_key_prefix" character varying NOT NULL DEFAULT '';

CREATE INDEX "opserrorlog_api_key_prefix_occurred_at"
  ON "ops_error_logs" ("api_key_prefix", "occurred_at");
