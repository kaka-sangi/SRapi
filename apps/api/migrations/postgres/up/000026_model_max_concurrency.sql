-- Modify "model_rate_limits" table
ALTER TABLE "model_rate_limits" ADD COLUMN "max_concurrency" bigint NOT NULL DEFAULT 0;
