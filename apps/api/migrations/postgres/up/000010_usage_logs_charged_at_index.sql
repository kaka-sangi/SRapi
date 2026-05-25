-- Optimize balance_charger pending usage scans by matching charged_at/success filters
-- and oldest-first batch ordering.
DROP INDEX "usagelog_charged_at";
CREATE INDEX "usagelog_charged_at_success_created_at" ON "usage_logs" ("charged_at", "success", "created_at");
