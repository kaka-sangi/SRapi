-- Add structured fields to provider_accounts (sub2api alignment).
-- Moves scheduling, rate-limiting, and lifecycle state from metadata_json
-- into typed, indexed columns so the scheduler and admin list can filter
-- without JSON path traversal.

-- provider_accounts: new columns
ALTER TABLE "provider_accounts" ADD COLUMN IF NOT EXISTS "platform" character varying NOT NULL DEFAULT '';
ALTER TABLE "provider_accounts" ADD COLUMN IF NOT EXISTS "notes" character varying NULL DEFAULT '';
ALTER TABLE "provider_accounts" ADD COLUMN IF NOT EXISTS "concurrency" bigint NOT NULL DEFAULT 3;
ALTER TABLE "provider_accounts" ADD COLUMN IF NOT EXISTS "rate_multiplier" double precision NOT NULL DEFAULT 1;
ALTER TABLE "provider_accounts" ADD COLUMN IF NOT EXISTS "load_factor" bigint NULL;
ALTER TABLE "provider_accounts" ADD COLUMN IF NOT EXISTS "schedulable" boolean NOT NULL DEFAULT true;
ALTER TABLE "provider_accounts" ADD COLUMN IF NOT EXISTS "error_message" character varying NOT NULL DEFAULT '';
ALTER TABLE "provider_accounts" ADD COLUMN IF NOT EXISTS "last_used_at" timestamptz NULL;
ALTER TABLE "provider_accounts" ADD COLUMN IF NOT EXISTS "expires_at" timestamptz NULL;
ALTER TABLE "provider_accounts" ADD COLUMN IF NOT EXISTS "auto_pause_on_expired" boolean NOT NULL DEFAULT true;
ALTER TABLE "provider_accounts" ADD COLUMN IF NOT EXISTS "rate_limited_at" timestamptz NULL;
ALTER TABLE "provider_accounts" ADD COLUMN IF NOT EXISTS "rate_limit_reset_at" timestamptz NULL;
ALTER TABLE "provider_accounts" ADD COLUMN IF NOT EXISTS "overload_until" timestamptz NULL;
ALTER TABLE "provider_accounts" ADD COLUMN IF NOT EXISTS "temp_unschedulable_until" timestamptz NULL;
ALTER TABLE "provider_accounts" ADD COLUMN IF NOT EXISTS "temp_unschedulable_reason" character varying NOT NULL DEFAULT '';
ALTER TABLE "provider_accounts" ADD COLUMN IF NOT EXISTS "session_window_start" timestamptz NULL;
ALTER TABLE "provider_accounts" ADD COLUMN IF NOT EXISTS "session_window_end" timestamptz NULL;
ALTER TABLE "provider_accounts" ADD COLUMN IF NOT EXISTS "session_window_status" character varying NOT NULL DEFAULT '';
ALTER TABLE "provider_accounts" ADD COLUMN IF NOT EXISTS "extra_json" jsonb NULL;

-- provider_accounts: indexes for scheduling hot path
CREATE INDEX IF NOT EXISTS "provideraccount_platform_status"
    ON "provider_accounts" ("platform", "status");
CREATE INDEX IF NOT EXISTS "provideraccount_platform_priority"
    ON "provider_accounts" ("platform", "priority");
CREATE INDEX IF NOT EXISTS "provideraccount_schedulable"
    ON "provider_accounts" ("schedulable");
CREATE INDEX IF NOT EXISTS "provideraccount_expires_at"
    ON "provider_accounts" ("expires_at");
CREATE INDEX IF NOT EXISTS "provideraccount_rate_limited_at"
    ON "provider_accounts" ("rate_limited_at");
CREATE INDEX IF NOT EXISTS "provideraccount_rate_limit_reset_at"
    ON "provider_accounts" ("rate_limit_reset_at");
CREATE INDEX IF NOT EXISTS "provideraccount_overload_until"
    ON "provider_accounts" ("overload_until");
CREATE INDEX IF NOT EXISTS "provideraccount_last_used_at"
    ON "provider_accounts" ("last_used_at");

-- Backfill account_type from runtime_class to sub2api-style simplified types.
UPDATE "provider_accounts" SET "account_type" = 'apikey'
    WHERE "runtime_class" = 'api_key' AND "account_type" = 'api_key';
UPDATE "provider_accounts" SET "account_type" = 'oauth'
    WHERE "runtime_class" IN ('oauth_refresh', 'oauth_device_code', 'web_session_cookie')
    AND "account_type" IN ('oauth_refresh', 'oauth_device_code', 'web_session_cookie');
UPDATE "provider_accounts" SET "account_type" = 'setup-token'
    WHERE "runtime_class" = 'cli_client_token' AND "account_type" = 'cli_client_token';
UPDATE "provider_accounts" SET "account_type" = 'upstream'
    WHERE "runtime_class" = 'custom_reverse_proxy' AND "account_type" = 'custom_reverse_proxy';
UPDATE "provider_accounts" SET "account_type" = 'service_account'
    WHERE "runtime_class" = 'service_account_json' AND "account_type" = 'service_account_json';

-- Backfill platform from provider adapter_type.
UPDATE "provider_accounts" pa
SET "platform" = CASE
    WHEN p."adapter_type" ILIKE '%anthropic%' OR p."name" ILIKE '%anthropic%' OR p."name" ILIKE '%claude%' THEN 'anthropic'
    WHEN p."adapter_type" ILIKE '%openai%' OR p."name" ILIKE '%openai%' OR p."adapter_type" ILIKE '%codex%' THEN 'openai'
    WHEN p."adapter_type" ILIKE '%gemini%' OR p."name" ILIKE '%gemini%' OR p."adapter_type" ILIKE '%vertex%' THEN 'gemini'
    WHEN p."adapter_type" ILIKE '%antigravity%' OR p."name" ILIKE '%antigravity%' THEN 'antigravity'
    ELSE ''
END
FROM "providers" p
WHERE pa."provider_id" = p."id" AND pa."platform" = '';

-- Backfill concurrency from metadata_json (max_concurrency key).
UPDATE "provider_accounts"
SET "concurrency" = ("metadata_json"->>'max_concurrency')::int
WHERE "metadata_json" IS NOT NULL
  AND "metadata_json" ? 'max_concurrency'
  AND ("metadata_json"->>'max_concurrency')::int > 0;

-- Backfill temp_unschedulable from metadata_json (manual_pause_until key).
UPDATE "provider_accounts"
SET "temp_unschedulable_until" = ("metadata_json"->>'manual_pause_until')::timestamptz,
    "temp_unschedulable_reason" = COALESCE("metadata_json"->>'manual_pause_reason', '')
WHERE "metadata_json" IS NOT NULL
  AND "metadata_json" ? 'manual_pause_until'
  AND ("metadata_json"->>'manual_pause_until') IS NOT NULL;

-- proxies: add structured fields
ALTER TABLE "proxies" ADD COLUMN IF NOT EXISTS "protocol" character varying NOT NULL DEFAULT 'http';
ALTER TABLE "proxies" ADD COLUMN IF NOT EXISTS "host" character varying NOT NULL DEFAULT '';
ALTER TABLE "proxies" ADD COLUMN IF NOT EXISTS "port" bigint NOT NULL DEFAULT 0;
ALTER TABLE "proxies" ADD COLUMN IF NOT EXISTS "username" character varying NULL DEFAULT '';
ALTER TABLE "proxies" ADD COLUMN IF NOT EXISTS "password_ciphertext" bytea NULL;
ALTER TABLE "proxies" ADD COLUMN IF NOT EXISTS "expiry_warn_days" bigint NOT NULL DEFAULT 0;
