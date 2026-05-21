package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type SubscriptionPlan struct {
	ent.Schema
}

func (SubscriptionPlan) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}}
}

func (SubscriptionPlan) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.String("description").Default(""),
		field.String("price").Default("0.00000000"),
		field.String("currency").Default("USD"),
		field.Int("validity_days"),
		field.JSON("entitlements_json", map[string]any{}).Optional(),
		field.Bool("for_sale").Default(true),
		field.Int("sort_order").Default(0),
		field.String("status").Default("active"),
	}
}

func (SubscriptionPlan) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("for_sale", "sort_order"),
		index.Fields("status"),
	}
}
