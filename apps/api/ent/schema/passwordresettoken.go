package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type PasswordResetToken struct {
	ent.Schema
}

func (PasswordResetToken) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (PasswordResetToken) Fields() []ent.Field {
	return []ent.Field{
		field.Int("user_id"),
		field.String("token_hash").NotEmpty().Sensitive(),
		field.String("token_version").Default("v1"),
		field.Time("expires_at"),
		field.Time("used_at").Optional().Nillable(),
	}
}

func (PasswordResetToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("token_hash").Unique(),
		index.Fields("user_id", "created_at"),
		index.Fields("expires_at"),
		index.Fields("used_at"),
	}
}
