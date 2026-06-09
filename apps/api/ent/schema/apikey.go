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
		field.Int("workspace_id").Optional().Nillable(),
		field.String("name").NotEmpty(),
		field.String("prefix").NotEmpty(),
		field.String("hash").Sensitive(),
		field.String("status").Default("active"),
		field.JSON("scopes_json", []string{}).Optional(),
		field.JSON("allowed_models_json", []string{}).Optional(),
		field.Int("rpm_limit").Optional().Nillable(),
		field.Int("tpm_limit").Optional().Nillable(),
		field.Int("concurrency_limit").Optional().Nillable(),
		field.Int("request_limit_5h").Optional().Nillable(),
		field.Int("request_limit_1d").Optional().Nillable(),
		field.Int("request_limit_7d").Optional().Nillable(),
		field.String("cost_quota").Optional().Nillable(),
		field.String("cost_used").Default("0.00000000"),
		field.String("cost_limit_5h").Optional().Nillable(),
		field.String("cost_used_5h").Default("0.00000000"),
		field.Time("cost_window_start_5h").Optional().Nillable(),
		field.String("cost_limit_1d").Optional().Nillable(),
		field.String("cost_used_1d").Default("0.00000000"),
		field.Time("cost_window_start_1d").Optional().Nillable(),
		field.String("cost_limit_7d").Optional().Nillable(),
		field.String("cost_used_7d").Default("0.00000000"),
		field.Time("cost_window_start_7d").Optional().Nillable(),
		field.JSON("allowed_ips_json", []string{}).Optional(),
		field.JSON("denied_ips_json", []string{}).Optional(),
		field.Time("expires_at").Optional().Nillable(),
		field.Time("last_used_at").Optional().Nillable(),
	}
}

func (APIKey) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("prefix").Unique(),
		index.Fields("workspace_id", "status"),
		index.Fields("user_id", "status"),
		index.Fields("expires_at"),
	}
}
