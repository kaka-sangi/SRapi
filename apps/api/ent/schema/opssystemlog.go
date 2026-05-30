package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type OpsSystemLog struct {
	ent.Schema
}

func (OpsSystemLog) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (OpsSystemLog) Fields() []ent.Field {
	return []ent.Field{
		field.String("level").Default("info"),
		field.String("source").NotEmpty(),
		field.String("message").NotEmpty(),
		field.String("request_id").Default(""),
		field.String("trace_id").Default(""),
		field.JSON("metadata_json", map[string]any{}).Optional(),
	}
}

func (OpsSystemLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("created_at"),
		index.Fields("level", "created_at"),
		index.Fields("source", "created_at"),
		index.Fields("request_id"),
		index.Fields("trace_id"),
	}
}
