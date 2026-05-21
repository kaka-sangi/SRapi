package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ModelAlias struct {
	ent.Schema
}

func (ModelAlias) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (ModelAlias) Fields() []ent.Field {
	return []ent.Field{
		field.String("alias").NotEmpty(),
		field.Int("model_id"),
		field.String("strategy_hint").Default(""),
		field.JSON("fallback_models_json", []string{}).Optional(),
		field.String("status").Default("active"),
	}
}

func (ModelAlias) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("alias").Unique(),
		index.Fields("model_id"),
	}
}
