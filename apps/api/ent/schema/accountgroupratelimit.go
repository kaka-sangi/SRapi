package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AccountGroupRateLimit is an operator-managed requests-per-minute capacity
// ceiling for an account group, enforced after account selection across all
// traffic routed through the group (complements per-account / per-model RPM).
type AccountGroupRateLimit struct {
	ent.Schema
}

func (AccountGroupRateLimit) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (AccountGroupRateLimit) Fields() []ent.Field {
	return []ent.Field{
		field.Int("account_group_id"),
		field.Int("rpm_limit").Default(0), // 0 = unlimited / disabled
		field.Bool("enabled").Default(true),
	}
}

func (AccountGroupRateLimit) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_group_id").Unique(),
		index.Fields("enabled"),
	}
}
