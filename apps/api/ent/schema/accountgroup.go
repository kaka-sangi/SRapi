package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AccountGroup struct {
	ent.Schema
}

func (AccountGroup) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (AccountGroup) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.String("description").Default(""),
		field.JSON("provider_scope_json", map[string]any{}).Optional(),
		field.JSON("model_scope_json", map[string]any{}).Optional(),
		field.String("strategy_hint").Default("balanced"),
		field.String("status").Default("active"),
	}
}

func (AccountGroup) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").Unique(),
		index.Fields("status"),
	}
}
