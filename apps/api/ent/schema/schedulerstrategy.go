package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type SchedulerStrategy struct {
	ent.Schema
}

func (SchedulerStrategy) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (SchedulerStrategy) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.String("version").Default("v1"),
		field.String("status").Default("draft"),
		field.String("scope_type").Default("global"),
		field.Int("scope_id").Optional().Nillable(),
		field.JSON("config_json", map[string]any{}).Optional(),
		field.String("config_hash").NotEmpty(),
		field.String("description").Default(""),
		field.Int("created_by").Optional().Nillable(),
		field.Time("activated_at").Optional().Nillable(),
		field.Time("deprecated_at").Optional().Nillable(),
	}
}

func (SchedulerStrategy) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name", "version", "scope_type", "scope_id").Unique(),
		index.Fields("status", "scope_type", "scope_id"),
		index.Fields("name", "status"),
	}
}
