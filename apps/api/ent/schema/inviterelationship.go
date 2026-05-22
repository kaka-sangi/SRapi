package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type InviteRelationship struct {
	ent.Schema
}

func (InviteRelationship) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (InviteRelationship) Fields() []ent.Field {
	return []ent.Field{
		field.Int("inviter_user_id"),
		field.Int("invitee_user_id"),
		field.Int("invite_code_id"),
		field.String("status").Default("active"),
		field.Time("first_paid_at").Optional().Nillable(),
	}
}

func (InviteRelationship) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("invitee_user_id").Unique(),
		index.Fields("inviter_user_id", "created_at"),
		index.Fields("invite_code_id"),
		index.Fields("status", "created_at"),
	}
}
