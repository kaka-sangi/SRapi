-- Add per-API-key gateway concurrency limit.
ALTER TABLE "api_keys" ADD COLUMN "concurrency_limit" bigint NULL;
