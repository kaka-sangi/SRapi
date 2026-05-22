package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ObsSLODefinition struct {
	ent.Schema
}

func (ObsSLODefinition) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (ObsSLODefinition) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.String("sli_type").Default("availability"),
		field.Float("objective"),
		field.Int("window_days").Default(28),
		field.String("status").Default("active"),
		field.JSON("filter_json", map[string]any{}).Optional(),
		field.JSON("alert_policy_json", map[string]any{}).Optional(),
	}
}

func (ObsSLODefinition) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").Unique(),
		index.Fields("status", "sli_type"),
	}
}
