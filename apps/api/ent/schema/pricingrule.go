package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type PricingRule struct {
	ent.Schema
}

func (PricingRule) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (PricingRule) Fields() []ent.Field {
	return []ent.Field{
		field.Int("model_id"),
		field.String("model_family").Default(""),
		field.Int("provider_id"),
		field.String("billing_mode").Default("token"),
		field.String("input_price_per_million").Default("0.00000000"),
		field.String("output_price_per_million").Default("0.00000000"),
		field.String("cache_read_price_per_million").Default("0.00000000"),
		field.String("cache_write_price_per_million").Default("0.00000000"),
		field.String("cache_write_5m_price_per_million").Default("0.00000000"),
		field.String("cache_write_1h_price_per_million").Default("0.00000000"),
		field.String("image_output_price_per_million").Default("0.00000000"),
		field.String("per_request_price").Default("0.00000000"),
		field.JSON("service_tier_multipliers_json", map[string]string{}).Optional(),
		field.Int("long_context_threshold_tokens").Optional().Nillable(),
		field.String("long_context_multiplier").Default("0.00000000"),
		field.String("currency").Default("USD"),
		field.Time("effective_from").Optional().Nillable(),
		field.Time("effective_to").Optional().Nillable(),
	}
}

func (PricingRule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("model_id", "provider_id"),
		index.Fields("billing_mode"),
		index.Fields("effective_from", "effective_to"),
	}
}
