-- Reverse structured fields migration.

DROP INDEX CONCURRENTLY IF EXISTS "provideraccount_platform_status";
DROP INDEX CONCURRENTLY IF EXISTS "provideraccount_platform_priority";
DROP INDEX CONCURRENTLY IF EXISTS "provideraccount_schedulable";
DROP INDEX CONCURRENTLY IF EXISTS "provideraccount_expires_at";
DROP INDEX CONCURRENTLY IF EXISTS "provideraccount_rate_limited_at";
DROP INDEX CONCURRENTLY IF EXISTS "provideraccount_rate_limit_reset_at";
DROP INDEX CONCURRENTLY IF EXISTS "provideraccount_overload_until";
DROP INDEX CONCURRENTLY IF EXISTS "provideraccount_last_used_at";

ALTER TABLE "provider_accounts" DROP COLUMN IF EXISTS "platform";
ALTER TABLE "provider_accounts" DROP COLUMN IF EXISTS "notes";
ALTER TABLE "provider_accounts" DROP COLUMN IF EXISTS "concurrency";
ALTER TABLE "provider_accounts" DROP COLUMN IF EXISTS "rate_multiplier";
ALTER TABLE "provider_accounts" DROP COLUMN IF EXISTS "load_factor";
ALTER TABLE "provider_accounts" DROP COLUMN IF EXISTS "schedulable";
ALTER TABLE "provider_accounts" DROP COLUMN IF EXISTS "error_message";
ALTER TABLE "provider_accounts" DROP COLUMN IF EXISTS "last_used_at";
ALTER TABLE "provider_accounts" DROP COLUMN IF EXISTS "expires_at";
ALTER TABLE "provider_accounts" DROP COLUMN IF EXISTS "auto_pause_on_expired";
ALTER TABLE "provider_accounts" DROP COLUMN IF EXISTS "rate_limited_at";
ALTER TABLE "provider_accounts" DROP COLUMN IF EXISTS "rate_limit_reset_at";
ALTER TABLE "provider_accounts" DROP COLUMN IF EXISTS "overload_until";
ALTER TABLE "provider_accounts" DROP COLUMN IF EXISTS "temp_unschedulable_until";
ALTER TABLE "provider_accounts" DROP COLUMN IF EXISTS "temp_unschedulable_reason";
ALTER TABLE "provider_accounts" DROP COLUMN IF EXISTS "session_window_start";
ALTER TABLE "provider_accounts" DROP COLUMN IF EXISTS "session_window_end";
ALTER TABLE "provider_accounts" DROP COLUMN IF EXISTS "session_window_status";
ALTER TABLE "provider_accounts" DROP COLUMN IF EXISTS "extra_json";

ALTER TABLE "proxies" DROP COLUMN IF EXISTS "protocol";
ALTER TABLE "proxies" DROP COLUMN IF EXISTS "host";
ALTER TABLE "proxies" DROP COLUMN IF EXISTS "port";
ALTER TABLE "proxies" DROP COLUMN IF EXISTS "username";
ALTER TABLE "proxies" DROP COLUMN IF EXISTS "password_ciphertext";
ALTER TABLE "proxies" DROP COLUMN IF EXISTS "expiry_warn_days";
