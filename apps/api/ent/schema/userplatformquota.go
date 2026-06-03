package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UserPlatformQuota is an operator-managed spend ceiling for one user on one
// upstream platform (provider family, e.g. "anthropic" / "openai" / "gemini").
// Limits are decimal USD strings; a nil window limit means that window is
// uncapped. Enforced at the gateway as a hard cap once the user's rolling spend
// on that platform reaches the limit. Per-user rows override the plan default
// carried in subscription entitlements.
type UserPlatformQuota struct {
	ent.Schema
}

func (UserPlatformQuota) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (UserPlatformQuota) Fields() []ent.Field {
	return []ent.Field{
		field.Int("user_id"),
		field.String("platform").NotEmpty(),
		field.String("daily_limit").Optional().Nillable(),   // nil = uncapped
		field.String("weekly_limit").Optional().Nillable(),  // nil = uncapped
		field.String("monthly_limit").Optional().Nillable(), // nil = uncapped
		field.String("currency").Default("USD"),
		field.Bool("enabled").Default(true),
	}
}

func (UserPlatformQuota) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "platform").Unique(),
		index.Fields("user_id", "enabled"),
	}
}
