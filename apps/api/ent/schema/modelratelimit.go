package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ModelRateLimit is an operator-managed global requests-per-minute ceiling for a
// single model, enforced at gateway admission across all users to protect an
// upstream model from overload (complements the per-API-key / per-user RPM).
type ModelRateLimit struct {
	ent.Schema
}

func (ModelRateLimit) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (ModelRateLimit) Fields() []ent.Field {
	return []ent.Field{
		field.Int("model_id"),
		field.Int("rpm_limit").Default(0), // 0 = unlimited / disabled
		field.Bool("enabled").Default(true),
	}
}

func (ModelRateLimit) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("model_id").Unique(),
		index.Fields("enabled"),
	}
}
