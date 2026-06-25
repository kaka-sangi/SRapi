-- Remove prompt=login from codex-cli / chatgpt-web OAuth authorize_url.
-- Old: ...oauth/authorize?codex_cli_simplified_flow=true&id_token_add_organizations=true&prompt=login
-- New: ...oauth/authorize?codex_cli_simplified_flow=true&id_token_add_organizations=true
--
-- Also ensure redirect_uri is set to localhost:1455 for OpenAI OAuth providers.

UPDATE "providers"
SET "config_schema_json" = jsonb_set(
    "config_schema_json",
    '{oauth_config,authorize_url}',
    to_jsonb(replace(
        "config_schema_json"->'oauth_config'->>'authorize_url',
        '&prompt=login', ''
    ))
)
WHERE "config_schema_json"->'oauth_config'->>'authorize_url' LIKE '%prompt=login%';

UPDATE "providers"
SET "config_schema_json" = jsonb_set(
    "config_schema_json",
    '{oauth_config,redirect_uri}',
    '"http://localhost:1455/auth/callback"'::jsonb
)
WHERE "config_schema_json"->'oauth_config'->>'client_id' = 'app_EMoamEEZ73f0CkXaXp7hrann'
  AND ("config_schema_json"->'oauth_config'->>'redirect_uri' IS NULL
       OR "config_schema_json"->'oauth_config'->>'redirect_uri' = '');
