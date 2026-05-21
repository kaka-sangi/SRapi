package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type APIKeyGroup struct {
	ent.Schema
}

func (APIKeyGroup) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (APIKeyGroup) Fields() []ent.Field {
	return []ent.Field{
		field.Int("api_key_id"),
		field.Int("account_group_id"),
	}
}

func (APIKeyGroup) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("api_key_id", "account_group_id").Unique(),
		index.Fields("account_group_id"),
	}
}
