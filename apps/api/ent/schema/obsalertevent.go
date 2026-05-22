package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ObsAlertEvent struct {
	ent.Schema
}

func (ObsAlertEvent) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (ObsAlertEvent) Fields() []ent.Field {
	return []ent.Field{
		field.Int("slo_id").Optional().Nillable(),
		field.String("rule_id").NotEmpty(),
		field.String("severity").Default("warning"),
		field.String("status").Default("firing"),
		field.String("fingerprint").NotEmpty(),
		field.String("summary").NotEmpty(),
		field.JSON("details_json", map[string]any{}).Optional(),
		field.Time("started_at"),
		field.Time("resolved_at").Optional().Nillable(),
		field.Time("acknowledged_at").Optional().Nillable(),
		field.Int("acknowledged_by").Optional().Nillable(),
		field.String("suppressed_by").Optional().Nillable(),
	}
}

func (ObsAlertEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("fingerprint", "status"),
		index.Fields("rule_id", "started_at"),
		index.Fields("severity", "status"),
		index.Fields("slo_id", "started_at"),
	}
}
