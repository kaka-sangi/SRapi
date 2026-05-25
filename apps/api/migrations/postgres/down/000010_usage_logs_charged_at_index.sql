DROP INDEX "usagelog_charged_at_success_created_at";
CREATE INDEX "usagelog_charged_at" ON "usage_logs" ("charged_at");
