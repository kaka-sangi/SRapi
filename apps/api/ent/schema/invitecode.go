package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type InviteCode struct {
	ent.Schema
}

func (InviteCode) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (InviteCode) Fields() []ent.Field {
	return []ent.Field{
		field.Int("user_id"),
		field.String("code").NotEmpty(),
		field.String("status").Default("active"),
		field.Time("expires_at").Optional().Nillable(),
	}
}

func (InviteCode) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("code").Unique(),
		index.Fields("user_id", "status"),
		index.Fields("expires_at"),
	}
}
