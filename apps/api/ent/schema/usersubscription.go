package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type UserSubscription struct {
	ent.Schema
}

func (UserSubscription) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (UserSubscription) Fields() []ent.Field {
	return []ent.Field{
		field.Int("user_id"),
		field.Int("plan_id"),
		field.String("status").Default("active"),
		field.Time("starts_at"),
		field.Time("expires_at"),
		field.JSON("entitlements_snapshot_json", map[string]any{}).Optional(),
		field.String("source_type").Default(""),
		field.String("source_id").Default(""),
		field.String("daily_usage_usd").Default("0.00000000"),
		field.Time("daily_usage_window_start").Optional().Nillable(),
		field.String("weekly_usage_usd").Default("0.00000000"),
		field.Time("weekly_usage_window_start").Optional().Nillable(),
		field.String("monthly_usage_usd").Default("0.00000000"),
		field.Time("monthly_usage_window_start").Optional().Nillable(),
	}
}

func (UserSubscription) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "status"),
		index.Fields("expires_at"),
		index.Fields("plan_id"),
	}
}
