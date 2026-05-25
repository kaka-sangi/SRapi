package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Workspace struct {
	ent.Schema
}

func (Workspace) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}}
}

func (Workspace) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.String("slug").NotEmpty(),
		field.Int("owner_user_id").Optional().Nillable(),
		field.String("type").Default("personal"),
		field.String("status").Default("active"),
		field.JSON("metadata_json", map[string]any{}).Optional(),
	}
}

func (Workspace) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("slug").Unique(),
		index.Fields("owner_user_id", "status"),
		index.Fields("status"),
	}
}
