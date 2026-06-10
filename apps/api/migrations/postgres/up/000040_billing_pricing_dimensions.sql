-- Add sub2api parity pricing dimensions.
ALTER TABLE "pricing_rules" ADD COLUMN "model_family" character varying NOT NULL DEFAULT '';
ALTER TABLE "pricing_rules" ADD COLUMN "cache_write_5m_price_per_million" character varying NOT NULL DEFAULT '0.00000000';
ALTER TABLE "pricing_rules" ADD COLUMN "cache_write_1h_price_per_million" character varying NOT NULL DEFAULT '0.00000000';
ALTER TABLE "pricing_rules" ADD COLUMN "image_output_price_per_million" character varying NOT NULL DEFAULT '0.00000000';
ALTER TABLE "pricing_rules" ADD COLUMN "service_tier_multipliers_json" jsonb NULL;
ALTER TABLE "pricing_rules" ADD COLUMN "long_context_threshold_tokens" bigint NULL;
ALTER TABLE "pricing_rules" ADD COLUMN "long_context_multiplier" character varying NOT NULL DEFAULT '0.00000000';
