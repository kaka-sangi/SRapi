package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type PendingOAuthSession struct {
	ent.Schema
}

func (PendingOAuthSession) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (PendingOAuthSession) Fields() []ent.Field {
	return []ent.Field{
		field.String("session_token_hash").NotEmpty().Sensitive(),
		field.String("intent").NotEmpty(),
		field.String("provider").NotEmpty(),
		field.String("provider_key").NotEmpty(),
		field.String("provider_subject_hash").NotEmpty().Sensitive(),
		field.String("subject_hint").Default(""),
		field.Int("target_user_id").Optional().Nillable(),
		field.String("redirect_to").Default("/"),
		field.String("resolved_email").Default(""),
		field.String("display_name").Default(""),
		field.Bool("email_verified").Default(false),
		field.String("avatar_url").Default(""),
		field.Time("expires_at"),
		field.Time("consumed_at").Optional().Nillable(),
	}
}

func (PendingOAuthSession) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_token_hash").Unique(),
		index.Fields("target_user_id"),
		index.Fields("expires_at"),
		index.Fields("consumed_at"),
		index.Fields("provider", "provider_key", "provider_subject_hash"),
	}
}
