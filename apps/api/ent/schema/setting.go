package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Setting struct {
	ent.Schema
}

func (Setting) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (Setting) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").NotEmpty(),
		field.JSON("value_json", map[string]any{}).Optional(),
		field.Bytes("value_ciphertext").Sensitive().Optional(),
		field.Bool("is_secret").Default(false),
		field.String("description").Default(""),
		field.Int("updated_by").Optional().Nillable(),
	}
}

func (Setting) Indexes() []ent.Index {
	return []ent.Index{index.Fields("key").Unique()}
}
