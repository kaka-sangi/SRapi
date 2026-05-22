package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type PaymentOrder struct {
	ent.Schema
}

func (PaymentOrder) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (PaymentOrder) Fields() []ent.Field {
	return []ent.Field{
		field.Int("user_id"),
		field.String("order_no").NotEmpty(),
		field.Int("provider_instance_id"),
		field.String("amount").Default("0.00000000"),
		field.String("currency").Default("USD"),
		field.String("status").Default("pending"),
		field.String("product_type").NotEmpty(),
		field.String("product_id").Default(""),
		field.String("provider_transaction_id").Optional().Nillable(),
		field.JSON("provider_snapshot_json", map[string]any{}).Optional(),
		field.Time("expires_at").Optional().Nillable(),
		field.Time("paid_at").Optional().Nillable(),
		field.Time("closed_at").Optional().Nillable(),
		field.JSON("metadata_json", map[string]any{}).Optional(),
	}
}

func (PaymentOrder) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("order_no").Unique(),
		index.Fields("user_id", "created_at"),
		index.Fields("status", "created_at"),
		index.Fields("provider_transaction_id"),
		index.Fields("provider_instance_id", "created_at"),
		index.Fields("expires_at"),
	}
}
