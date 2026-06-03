-- Add per-API-key multi-window request limits and IP allow/deny lists.
ALTER TABLE "api_keys" ADD COLUMN "request_limit_5h" bigint NULL;
ALTER TABLE "api_keys" ADD COLUMN "request_limit_1d" bigint NULL;
ALTER TABLE "api_keys" ADD COLUMN "request_limit_7d" bigint NULL;
ALTER TABLE "api_keys" ADD COLUMN "allowed_ips_json" jsonb NULL;
ALTER TABLE "api_keys" ADD COLUMN "denied_ips_json" jsonb NULL;
