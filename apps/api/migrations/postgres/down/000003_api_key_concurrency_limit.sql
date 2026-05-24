-- Remove per-API-key gateway concurrency limit.
ALTER TABLE "api_keys" DROP COLUMN "concurrency_limit";
