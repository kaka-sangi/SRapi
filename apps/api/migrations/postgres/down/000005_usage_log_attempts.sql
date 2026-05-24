-- Restore request-level uniqueness for usage logs.
DROP INDEX "usagelog_request_id_attempt_no";
ALTER TABLE "usage_logs" DROP COLUMN "attempt_no";
CREATE UNIQUE INDEX "usagelog_request_id" ON "usage_logs" ("request_id");
