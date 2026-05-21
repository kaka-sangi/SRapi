package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AccountGroupMember struct {
	ent.Schema
}

func (AccountGroupMember) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (AccountGroupMember) Fields() []ent.Field {
	return []ent.Field{
		field.Int("account_id"),
		field.Int("account_group_id"),
	}
}

func (AccountGroupMember) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_id", "account_group_id").Unique(),
		index.Fields("account_group_id"),
	}
}
