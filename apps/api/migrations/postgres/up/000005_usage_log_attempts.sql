-- Allow one durable usage evidence row per Gateway attempt.
ALTER TABLE "usage_logs" ADD COLUMN "attempt_no" bigint NOT NULL DEFAULT 1;
DROP INDEX "usagelog_request_id";
CREATE UNIQUE INDEX "usagelog_request_id_attempt_no" ON "usage_logs" ("request_id", "attempt_no");
