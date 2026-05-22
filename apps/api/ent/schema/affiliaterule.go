package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AffiliateRule struct {
	ent.Schema
}

func (AffiliateRule) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (AffiliateRule) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.String("status").Default("active"),
		field.String("trigger_type").NotEmpty(),
		field.String("rate").Default("0.00000000"),
		field.String("fixed_amount").Default("0.00000000"),
		field.String("currency").Default("USD"),
		field.String("max_rebate_amount").Default("0.00000000"),
		field.Time("valid_from").Optional().Nillable(),
		field.Time("valid_to").Optional().Nillable(),
		field.JSON("metadata_json", map[string]any{}).Optional(),
	}
}

func (AffiliateRule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("trigger_type", "currency", "status"),
		index.Fields("valid_from", "valid_to"),
	}
}
