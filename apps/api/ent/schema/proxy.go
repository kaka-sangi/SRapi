package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Proxy struct {
	ent.Schema
}

func (Proxy) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}}
}

func (Proxy) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.String("type").NotEmpty(),
		field.Bytes("url_ciphertext").Sensitive().Optional(),
		field.Int("url_version").Default(1),
		field.String("status").Default("active"),
		field.JSON("metadata_json", map[string]any{}).Optional(),
	}
}

func (Proxy) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").Unique(),
		index.Fields("status"),
		index.Fields("type", "status"),
	}
}
