ALTER TABLE "error_passthrough_rules"
  DROP COLUMN IF EXISTS "custom_message",
  DROP COLUMN IF EXISTS "response_status";
