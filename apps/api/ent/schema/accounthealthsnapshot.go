package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AccountHealthSnapshot struct {
	ent.Schema
}

func (AccountHealthSnapshot) Fields() []ent.Field {
	return []ent.Field{
		field.Int("account_id"),
		field.Int("provider_id"),
		field.String("status").Default("healthy"),
		field.Float("success_rate").Default(0),
		field.Float("error_rate").Default(0),
		field.Int("latency_p50_ms").Default(0),
		field.Int("latency_p95_ms").Default(0),
		field.Int("rate_limit_count").Default(0),
		field.Int("timeout_count").Default(0),
		field.Time("cooldown_until").Optional().Nillable(),
		field.String("circuit_state").Default("closed"),
		field.Time("snapshot_at"),
	}
}

func (AccountHealthSnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_id", "snapshot_at"),
		index.Fields("provider_id", "snapshot_at"),
		index.Fields("status"),
	}
}
