package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type UserAuthIdentity struct {
	ent.Schema
}

func (UserAuthIdentity) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}}
}

func (UserAuthIdentity) Fields() []ent.Field {
	return []ent.Field{
		field.Int("user_id"),
		field.String("provider").NotEmpty(),
		field.String("provider_key").NotEmpty(),
		field.String("provider_subject_hash").NotEmpty().Sensitive(),
		field.String("subject_hint").Default(""),
		field.String("display_name").Default(""),
		field.String("email").Default(""),
		field.Bool("email_verified").Default(false),
		field.String("avatar_url").Default(""),
		field.Time("verified_at").Optional().Nillable(),
		field.Time("last_used_at").Optional().Nillable(),
	}
}

func (UserAuthIdentity) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider", "provider_key", "provider_subject_hash").Unique(),
		index.Fields("user_id", "provider"),
		index.Fields("user_id"),
		index.Fields("last_used_at"),
	}
}
