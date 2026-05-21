package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type CapabilityDefinition struct {
	ent.Schema
}

func (CapabilityDefinition) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (CapabilityDefinition) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").NotEmpty(),
		field.String("version").Default("v1"),
		field.String("category").NotEmpty(),
		field.String("status").Default("stable"),
		field.String("description").Default(""),
		field.JSON("schema_json", map[string]any{}).Optional(),
		field.String("replacement_key").Optional().Nillable(),
	}
}

func (CapabilityDefinition) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("key", "version").Unique(),
		index.Fields("category", "status"),
	}
}
