-- Canonicalize provider_accounts.metadata_json alias keys to canonical names.
--
-- Historically, the Codex/ChatGPT import paths wrote provider-specific keys
-- (codex_email, codex_account_id, chatgpt_account_id, chatgpt_user_id,
-- codex_plan_type, codex_organization_id, codex_user_id, rpm_override) into
-- account metadata. The frontend then chased 2-3 fallback keys per field. We
-- canonicalize at write time in apps/api/internal/modules/accounts/service
-- (CanonicalizeAccountMetadata) and this migration backfills existing rows so
-- the alias chains can be deleted from the frontend without breaking older
-- data. The credential bag is NOT touched: credentials carry upstream-protocol
-- field names (e.g. chatgpt_user_id is what the JWT signer reads at dispatch)
-- and any rewrite would break outgoing signing/auth.
--
-- Idempotent: each statement gates on alias presence, so a re-run is a no-op.
-- Conflict policy: when both alias and canonical are set on the same row,
-- the canonical value wins (storage truth > stale alias) and the alias is
-- simply dropped.

-- email
UPDATE "provider_accounts"
SET "metadata_json" = (
    CASE WHEN "metadata_json" ? 'email'
         THEN "metadata_json" - 'codex_email'
         ELSE jsonb_set("metadata_json" - 'codex_email', '{email}', "metadata_json"->'codex_email')
    END)
WHERE "metadata_json" IS NOT NULL AND "metadata_json" ? 'codex_email';

-- plan_type
UPDATE "provider_accounts"
SET "metadata_json" = (
    CASE WHEN "metadata_json" ? 'plan_type'
         THEN "metadata_json" - 'codex_plan_type'
         ELSE jsonb_set("metadata_json" - 'codex_plan_type', '{plan_type}', "metadata_json"->'codex_plan_type')
    END)
WHERE "metadata_json" IS NOT NULL AND "metadata_json" ? 'codex_plan_type';

-- organization_id
UPDATE "provider_accounts"
SET "metadata_json" = (
    CASE WHEN "metadata_json" ? 'organization_id'
         THEN "metadata_json" - 'codex_organization_id'
         ELSE jsonb_set("metadata_json" - 'codex_organization_id', '{organization_id}', "metadata_json"->'codex_organization_id')
    END)
WHERE "metadata_json" IS NOT NULL AND "metadata_json" ? 'codex_organization_id';

-- upstream_account_id (from chatgpt_account_id)
UPDATE "provider_accounts"
SET "metadata_json" = (
    CASE WHEN "metadata_json" ? 'upstream_account_id'
         THEN "metadata_json" - 'chatgpt_account_id'
         ELSE jsonb_set("metadata_json" - 'chatgpt_account_id', '{upstream_account_id}', "metadata_json"->'chatgpt_account_id')
    END)
WHERE "metadata_json" IS NOT NULL AND "metadata_json" ? 'chatgpt_account_id';

-- upstream_account_id (from codex_account_id; runs after chatgpt_account_id so the
-- "canonical wins" guard catches both alias forms when present together)
UPDATE "provider_accounts"
SET "metadata_json" = (
    CASE WHEN "metadata_json" ? 'upstream_account_id'
         THEN "metadata_json" - 'codex_account_id'
         ELSE jsonb_set("metadata_json" - 'codex_account_id', '{upstream_account_id}', "metadata_json"->'codex_account_id')
    END)
WHERE "metadata_json" IS NOT NULL AND "metadata_json" ? 'codex_account_id';

-- upstream_user_id (from chatgpt_user_id)
UPDATE "provider_accounts"
SET "metadata_json" = (
    CASE WHEN "metadata_json" ? 'upstream_user_id'
         THEN "metadata_json" - 'chatgpt_user_id'
         ELSE jsonb_set("metadata_json" - 'chatgpt_user_id', '{upstream_user_id}', "metadata_json"->'chatgpt_user_id')
    END)
WHERE "metadata_json" IS NOT NULL AND "metadata_json" ? 'chatgpt_user_id';

-- upstream_user_id (from codex_user_id)
UPDATE "provider_accounts"
SET "metadata_json" = (
    CASE WHEN "metadata_json" ? 'upstream_user_id'
         THEN "metadata_json" - 'codex_user_id'
         ELSE jsonb_set("metadata_json" - 'codex_user_id', '{upstream_user_id}', "metadata_json"->'codex_user_id')
    END)
WHERE "metadata_json" IS NOT NULL AND "metadata_json" ? 'codex_user_id';

-- rpm_limit (from rpm_override)
UPDATE "provider_accounts"
SET "metadata_json" = (
    CASE WHEN "metadata_json" ? 'rpm_limit'
         THEN "metadata_json" - 'rpm_override'
         ELSE jsonb_set("metadata_json" - 'rpm_override', '{rpm_limit}', "metadata_json"->'rpm_override')
    END)
WHERE "metadata_json" IS NOT NULL AND "metadata_json" ? 'rpm_override';
