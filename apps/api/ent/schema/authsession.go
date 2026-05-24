package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AuthSession struct {
	ent.Schema
}

func (AuthSession) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}}
}

func (AuthSession) Fields() []ent.Field {
	return []ent.Field{
		field.String("session_id_hash").NotEmpty().Sensitive(),
		field.String("csrf_token_hash").NotEmpty().Sensitive(),
		field.Int("user_id"),
		field.Time("expires_at"),
		field.Time("last_active_at").Optional().Nillable(),
		field.String("ip").Default(""),
		field.String("user_agent").Default(""),
		field.String("status").Default("active"),
	}
}

func (AuthSession) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id_hash").Unique(),
		index.Fields("user_id", "status"),
		index.Fields("expires_at"),
	}
}
