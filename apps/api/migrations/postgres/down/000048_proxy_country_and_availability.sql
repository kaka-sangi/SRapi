ALTER TABLE "proxies"
  DROP COLUMN IF EXISTS "country_code",
  DROP COLUMN IF EXISTS "country_name",
  DROP COLUMN IF EXISTS "last_probed_at",
  DROP COLUMN IF EXISTS "probe_success_count",
  DROP COLUMN IF EXISTS "probe_failure_count",
  DROP COLUMN IF EXISTS "last_probe_latency_ms";
