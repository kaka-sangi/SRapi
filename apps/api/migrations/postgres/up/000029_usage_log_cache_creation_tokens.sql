-- Add per-request prompt-cache write (creation) token accounting to usage logs.
ALTER TABLE "usage_logs" ADD COLUMN "cache_creation_tokens" bigint NOT NULL DEFAULT 0;
