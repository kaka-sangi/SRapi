-- Remove per-API-key multi-window request limits and IP allow/deny lists.
ALTER TABLE "api_keys" DROP COLUMN "denied_ips_json";
ALTER TABLE "api_keys" DROP COLUMN "allowed_ips_json";
ALTER TABLE "api_keys" DROP COLUMN "request_limit_7d";
ALTER TABLE "api_keys" DROP COLUMN "request_limit_1d";
ALTER TABLE "api_keys" DROP COLUMN "request_limit_5h";
