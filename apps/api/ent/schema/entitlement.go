package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Entitlement struct {
	ent.Schema
}

func (Entitlement) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (Entitlement) Fields() []ent.Field {
	return []ent.Field{
		field.Int("user_id"),
		field.String("scope_type").Default("user"),
		field.Int("scope_id"),
		field.String("feature_key").NotEmpty(),
		field.JSON("value_json", map[string]any{}).Optional(),
		field.String("quota_limit").Optional().Nillable(),
		field.Time("expires_at"),
		field.Int("source_subscription_id"),
	}
}

func (Entitlement) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "feature_key", "expires_at"),
		index.Fields("source_subscription_id", "feature_key").Unique(),
		index.Fields("scope_type", "scope_id", "feature_key"),
	}
}
