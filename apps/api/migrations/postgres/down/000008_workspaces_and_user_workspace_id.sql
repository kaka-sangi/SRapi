DROP TABLE IF EXISTS "workspaces";
DROP INDEX IF EXISTS "user_workspace_id";
ALTER TABLE "users" DROP COLUMN IF EXISTS "workspace_id";
DROP INDEX IF EXISTS "apikey_workspace_id_status";
ALTER TABLE "api_keys" DROP COLUMN IF EXISTS "workspace_id";
