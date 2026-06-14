ALTER TABLE "usage_logs" ADD COLUMN "aggregated_at" timestamptz NULL;
-- Pre-existing rows were already aggregated by the synchronous-era billing path.
-- Mark only the recent window (45 days) as aggregated: that is the only range the
-- reconciler ever sweeps (its floor is shorter — see usage_aggregation_reconciler),
-- so older rows can stay NULL and are never reprocessed. Bounding the backfill to
-- recent rows keeps this UPDATE's write volume small on large tables, so it cannot
-- blow the migration boot timeout by rewriting the whole history.
UPDATE "usage_logs" SET "aggregated_at" = "created_at"
  WHERE "aggregated_at" IS NULL AND "created_at" >= now() - interval '45 days';
CREATE INDEX "usagelog_aggregated_at_success_created_at" ON "usage_logs" ("aggregated_at", "success", "created_at");
