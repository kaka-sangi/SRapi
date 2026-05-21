package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type APIKey struct {
	ent.Schema
}

func (APIKey) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}}
}

func (APIKey) Fields() []ent.Field {
	return []ent.Field{
		field.Int("user_id"),
		field.String("name").NotEmpty(),
		field.String("prefix").NotEmpty(),
		field.String("hash").Sensitive(),
		field.String("status").Default("active"),
		field.JSON("scopes_json", []string{}).Optional(),
		field.JSON("allowed_models_json", []string{}).Optional(),
		field.Int("rpm_limit").Optional().Nillable(),
		field.Int("tpm_limit").Optional().Nillable(),
		field.Time("expires_at").Optional().Nillable(),
		field.Time("last_used_at").Optional().Nillable(),
	}
}

func (APIKey) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("prefix").Unique(),
		index.Fields("user_id", "status"),
		index.Fields("expires_at"),
	}
}
