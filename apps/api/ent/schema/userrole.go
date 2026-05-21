package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type UserRole struct {
	ent.Schema
}

func (UserRole) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (UserRole) Fields() []ent.Field {
	return []ent.Field{
		field.Int("user_id"),
		field.Int("role_id"),
	}
}

func (UserRole) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "role_id").Unique(),
		index.Fields("role_id"),
	}
}
