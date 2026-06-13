package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type PromoCode struct {
	ent.Schema
}

func (PromoCode) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (PromoCode) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").NotEmpty(),
		field.String("status").Default("active"),
		field.String("discount_type").NotEmpty(),
		field.String("discount_value").Default("0.00000000"),
		field.String("currency").Default("USD"),
		field.Int("max_uses").Default(1),
		field.Int("per_user_limit").Default(0),
		field.String("min_order_amount").Default(""),
		field.Int("used_count").Default(0),
		field.Time("starts_at").Optional().Nillable(),
		field.Time("expires_at").Optional().Nillable(),
	}
}

func (PromoCode) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("code").Unique(),
		index.Fields("status", "created_at"),
		index.Fields("expires_at"),
	}
}
