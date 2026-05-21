package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ModelProviderMapping struct {
	ent.Schema
}

func (ModelProviderMapping) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (ModelProviderMapping) Fields() []ent.Field {
	return []ent.Field{
		field.Int("model_id"),
		field.Int("provider_id"),
		field.String("upstream_model_name").NotEmpty(),
		field.String("status").Default("active"),
		field.JSON("capability_override_json", []map[string]any{}).Optional(),
		field.JSON("pricing_override_json", map[string]any{}).Optional(),
	}
}

func (ModelProviderMapping) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("model_id", "provider_id", "upstream_model_name").Unique(),
		index.Fields("provider_id", "status"),
	}
}
