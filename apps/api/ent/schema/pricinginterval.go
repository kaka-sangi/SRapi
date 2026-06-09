package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PricingInterval stores token-range or image-tier prices for a pricing rule.
type PricingInterval struct {
	ent.Schema
}

func (PricingInterval) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (PricingInterval) Fields() []ent.Field {
	return []ent.Field{
		field.Int("pricing_rule_id"),
		field.Int("min_tokens").Default(0),
		field.Int("max_tokens").Optional().Nillable(),
		field.String("tier_label").Default(""),
		field.String("image_size").Default(""),
		field.String("input_price_per_million").Default("0.00000000"),
		field.String("output_price_per_million").Default("0.00000000"),
		field.String("cache_read_price_per_million").Default("0.00000000"),
		field.String("cache_write_price_per_million").Default("0.00000000"),
		field.String("per_image_price").Default("0.00000000"),
	}
}

func (PricingInterval) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("pricing_rule_id"),
		index.Fields("pricing_rule_id", "min_tokens", "max_tokens"),
		index.Fields("pricing_rule_id", "image_size"),
	}
}
