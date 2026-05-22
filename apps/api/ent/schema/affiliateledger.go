package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AffiliateLedger struct {
	ent.Schema
}

func (AffiliateLedger) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (AffiliateLedger) Fields() []ent.Field {
	return []ent.Field{
		field.Int("user_id"),
		field.Int("related_user_id"),
		field.Int("payment_order_id").Optional().Nillable(),
		field.Int("subscription_id").Optional().Nillable(),
		field.String("type").NotEmpty(),
		field.String("amount").Default("0.00000000"),
		field.String("currency").Default("USD"),
		field.String("status").Default("pending"),
		field.String("reference_id").NotEmpty(),
		field.JSON("metadata_json", map[string]any{}).Optional(),
		field.Time("settled_at").Optional().Nillable(),
	}
}

func (AffiliateLedger) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("reference_id").Unique(),
		index.Fields("user_id", "created_at"),
		index.Fields("related_user_id", "created_at"),
		index.Fields("payment_order_id"),
		index.Fields("type", "created_at"),
	}
}
