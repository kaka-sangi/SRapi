-- Modify "proxies" table: add country + 7d availability fields.
-- Split into one ADD COLUMN per ALTER TABLE so the ent-schema parity test
-- (parseAlterTableAddColumnStatement) can match each column individually.
ALTER TABLE "proxies" ADD COLUMN "country_code" character varying NULL DEFAULT '';
ALTER TABLE "proxies" ADD COLUMN "country_name" character varying NULL DEFAULT '';
ALTER TABLE "proxies" ADD COLUMN "last_probed_at" timestamptz NULL;
ALTER TABLE "proxies" ADD COLUMN "probe_success_count" bigint NOT NULL DEFAULT 0;
ALTER TABLE "proxies" ADD COLUMN "probe_failure_count" bigint NOT NULL DEFAULT 0;
ALTER TABLE "proxies" ADD COLUMN "last_probe_latency_ms" bigint NOT NULL DEFAULT 0;
