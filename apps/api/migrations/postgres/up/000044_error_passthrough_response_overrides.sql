ALTER TABLE "error_passthrough_rules" ADD COLUMN "response_status" bigint NULL;
ALTER TABLE "error_passthrough_rules" ADD COLUMN "custom_message" character varying NOT NULL DEFAULT '';
