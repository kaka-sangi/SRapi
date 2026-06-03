-- Remove per-request prompt-cache write (creation) token accounting from usage logs.
ALTER TABLE "usage_logs" DROP COLUMN "cache_creation_tokens";
