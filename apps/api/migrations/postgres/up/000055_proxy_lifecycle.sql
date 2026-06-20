-- Add operator-defined proxy expiry and fallback behavior.
ALTER TABLE "proxies" ADD COLUMN "expires_at" timestamptz NULL;
ALTER TABLE "proxies" ADD COLUMN "fallback_mode" character varying NOT NULL DEFAULT 'none';
ALTER TABLE "proxies" ADD COLUMN "backup_proxy_id" bigint NULL;

CREATE INDEX "proxy_expires_at" ON "proxies" ("expires_at");
CREATE INDEX "proxy_backup_proxy_id" ON "proxies" ("backup_proxy_id");
