package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Provider struct {
	ent.Schema
}

func (Provider) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}}
}

func (Provider) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.String("display_name").NotEmpty(),
		field.String("adapter_type").NotEmpty(),
		field.String("protocol").Default("openai-compatible"),
		field.String("status").Default("active"),
		field.JSON("capabilities_json", map[string]any{}).Optional(),
		field.JSON("config_schema_json", map[string]any{}).Optional(),
	}
}

func (Provider) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").Unique(),
		index.Fields("status"),
	}
}
