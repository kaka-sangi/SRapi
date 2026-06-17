ALTER TABLE "provider_accounts"
  DROP COLUMN IF EXISTS "token_expires_at",
  DROP COLUMN IF EXISTS "last_refreshed_at",
  DROP COLUMN IF EXISTS "needs_reauth_at",
  DROP COLUMN IF EXISTS "refresh_attempts",
  DROP COLUMN IF EXISTS "refresh_last_error";
