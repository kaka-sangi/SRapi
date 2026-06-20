DROP INDEX IF EXISTS "proxy_backup_proxy_id";
DROP INDEX IF EXISTS "proxy_expires_at";

ALTER TABLE "proxies"
  DROP COLUMN IF EXISTS "expires_at",
  DROP COLUMN IF EXISTS "fallback_mode",
  DROP COLUMN IF EXISTS "backup_proxy_id";
