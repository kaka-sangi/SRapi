-- Modify "provider_accounts" table: add OAuth proactive-refresh bookkeeping.
-- Split into one ADD COLUMN per ALTER TABLE so the ent-schema parity test
-- (parseAlterTableAddColumnStatement) can match each column individually.
ALTER TABLE "provider_accounts" ADD COLUMN "token_expires_at" timestamptz NULL;
ALTER TABLE "provider_accounts" ADD COLUMN "last_refreshed_at" timestamptz NULL;
ALTER TABLE "provider_accounts" ADD COLUMN "needs_reauth_at" timestamptz NULL;
ALTER TABLE "provider_accounts" ADD COLUMN "refresh_attempts" bigint NOT NULL DEFAULT 0;
ALTER TABLE "provider_accounts" ADD COLUMN "refresh_last_error" character varying NOT NULL DEFAULT '';
