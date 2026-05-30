package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UserAttributeDefinition describes an operator-defined custom user profile field
// (the "definition" half of an EAV model). Values are stored per user in
// UserAttributeValue.
type UserAttributeDefinition struct {
	ent.Schema
}

func (UserAttributeDefinition) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (UserAttributeDefinition) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").NotEmpty(),
		field.String("name").NotEmpty(),
		field.String("data_type").Default("string"), // string | number | boolean | select
		field.JSON("options_json", []string{}).Optional(),
		field.Bool("required").Default(false),
		field.Int("display_order").Default(0),
		field.Bool("enabled").Default(true),
	}
}

func (UserAttributeDefinition) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("key").Unique(),
		index.Fields("enabled", "display_order"),
	}
}
