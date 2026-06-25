-- Reverse: restore prompt=login to codex OAuth authorize_url
UPDATE "providers"
SET "config_schema_json" = jsonb_set(
    "config_schema_json",
    '{oauth_config,authorize_url}',
    to_jsonb(
        "config_schema_json"->'oauth_config'->>'authorize_url' || '&prompt=login'
    )
)
WHERE "config_schema_json"->'oauth_config'->>'client_id' = 'app_EMoamEEZ73f0CkXaXp7hrann'
  AND "config_schema_json"->'oauth_config'->>'authorize_url' NOT LIKE '%prompt=login%';
