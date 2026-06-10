-- Remove sub2api parity pricing dimensions.
ALTER TABLE "pricing_rules" DROP COLUMN "long_context_multiplier";
ALTER TABLE "pricing_rules" DROP COLUMN "long_context_threshold_tokens";
ALTER TABLE "pricing_rules" DROP COLUMN "service_tier_multipliers_json";
ALTER TABLE "pricing_rules" DROP COLUMN "image_output_price_per_million";
ALTER TABLE "pricing_rules" DROP COLUMN "cache_write_1h_price_per_million";
ALTER TABLE "pricing_rules" DROP COLUMN "cache_write_5m_price_per_million";
ALTER TABLE "pricing_rules" DROP COLUMN "model_family";
