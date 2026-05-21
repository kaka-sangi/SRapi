package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type BillingLedger struct {
	ent.Schema
}

func (BillingLedger) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (BillingLedger) Fields() []ent.Field {
	return []ent.Field{
		field.Int("user_id"),
		field.String("type").NotEmpty(),
		field.String("amount").Default("0.00000000"),
		field.String("currency").Default("USD"),
		field.String("balance_before").Default("0.00000000"),
		field.String("balance_after").Default("0.00000000"),
		field.String("reference_type").Default(""),
		field.String("reference_id").Default(""),
		field.JSON("metadata_json", map[string]any{}).Optional(),
	}
}

func (BillingLedger) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "created_at"),
		index.Fields("reference_type", "reference_id"),
	}
}
