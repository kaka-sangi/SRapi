package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type RedeemCode struct {
	ent.Schema
}

func (RedeemCode) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (RedeemCode) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").NotEmpty(),
		field.String("type").NotEmpty(),
		field.String("status").Default("active"),
		field.String("value").Default(""),
		field.String("currency").Default("USD"),
		field.Int("max_redemptions").Default(1),
		field.Int("redeemed_count").Default(0),
		field.Time("expires_at").Optional().Nillable(),
	}
}

func (RedeemCode) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("code").Unique(),
		index.Fields("status", "created_at"),
		index.Fields("expires_at"),
	}
}
