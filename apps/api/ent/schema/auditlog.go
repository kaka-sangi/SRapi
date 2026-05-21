package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AuditLog struct {
	ent.Schema
}

func (AuditLog) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (AuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.Int("actor_user_id").Optional().Nillable(),
		field.String("action").NotEmpty(),
		field.String("resource_type").NotEmpty(),
		field.String("resource_id").Default(""),
		field.JSON("before_json", map[string]any{}).Optional(),
		field.JSON("after_json", map[string]any{}).Optional(),
		field.String("ip").Default(""),
		field.String("user_agent").Default(""),
		field.String("trace_id").Default(""),
	}
}

func (AuditLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("actor_user_id", "created_at"),
		index.Fields("resource_type", "resource_id"),
		index.Fields("action", "created_at"),
	}
}
