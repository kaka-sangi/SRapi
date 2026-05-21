package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ModelRegistry struct {
	ent.Schema
}

func (ModelRegistry) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}}
}

func (ModelRegistry) Fields() []ent.Field {
	return []ent.Field{
		field.String("canonical_name").NotEmpty(),
		field.String("display_name").NotEmpty(),
		field.String("family").Default(""),
		field.Int("context_window").Optional().Nillable(),
		field.Int("max_output_tokens").Optional().Nillable(),
		field.String("quality_tier").Default("standard"),
		field.String("status").Default("active"),
		field.JSON("capabilities_json", []map[string]any{}).Optional(),
	}
}

func (ModelRegistry) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("canonical_name").Unique(),
		index.Fields("family"),
		index.Fields("status"),
	}
}
