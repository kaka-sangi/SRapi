package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type User struct {
	ent.Schema
}

func (User) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}}
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("email").NotEmpty(),
		field.Time("email_verified_at").Optional().Nillable(),
		field.String("name").Default(""),
		field.String("password_hash").Sensitive(),
		field.String("status").Default("active"),
		field.String("balance").Default("0.00000000"),
		field.String("currency").Default("USD"),
		field.Time("last_login_at").Optional().Nillable(),
	}
}

func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("email").Unique(),
		index.Fields("status"),
		index.Fields("created_at"),
	}
}
