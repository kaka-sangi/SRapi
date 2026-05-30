package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UserAttributeValue stores one user's value for a UserAttributeDefinition (the
// "value" half of the EAV model).
type UserAttributeValue struct {
	ent.Schema
}

func (UserAttributeValue) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (UserAttributeValue) Fields() []ent.Field {
	return []ent.Field{
		field.Int("user_id"),
		field.Int("definition_id"),
		field.String("value").Default(""),
	}
}

func (UserAttributeValue) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "definition_id").Unique(),
		index.Fields("definition_id"),
	}
}
