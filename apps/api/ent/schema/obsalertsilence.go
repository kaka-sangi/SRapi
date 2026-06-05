package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ObsAlertSilence suppresses matching AlertEvents within a bounded window.
type ObsAlertSilence struct {
	ent.Schema
}

func (ObsAlertSilence) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (ObsAlertSilence) Fields() []ent.Field {
	return []ent.Field{
		field.String("comment").Optional(),
		field.JSON("matcher_json", map[string]any{}).Optional(),
		field.Time("starts_at"),
		field.Time("ends_at"),
		field.Int("created_by").Optional().Nillable(),
	}
}

func (ObsAlertSilence) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("starts_at", "ends_at"),
	}
}
