package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Role struct {
	ent.Schema
}

func (Role) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (Role) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.String("description").Default(""),
		field.JSON("permissions_json", []string{}).Optional(),
	}
}

func (Role) Indexes() []ent.Index {
	return []ent.Index{index.Fields("name").Unique()}
}
