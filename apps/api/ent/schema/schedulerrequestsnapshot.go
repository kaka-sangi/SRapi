package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type SchedulerRequestSnapshot struct {
	ent.Schema
}

func (SchedulerRequestSnapshot) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (SchedulerRequestSnapshot) Fields() []ent.Field {
	return []ent.Field{
		field.String("request_id").NotEmpty(),
		field.Int("attempt_no").Default(1),
		field.Int("decision_id"),
		field.JSON("request_profile_json", map[string]any{}).Optional(),
		field.JSON("candidate_snapshot_json", []map[string]any{}).Optional(),
		field.JSON("rejected_snapshot_json", map[string]any{}).Optional(),
		field.JSON("ranked_account_ids_json", []int{}).Optional(),
		field.Int("selected_account_id").Optional().Nillable(),
		field.Int("selected_provider_id").Optional().Nillable(),
		field.String("strategy").Default("balanced"),
		field.String("strategy_version").Default("v1"),
		field.String("strategy_config_hash").Default(""),
		field.JSON("strategy_weights_json", map[string]any{}).Optional(),
		field.JSON("compatibility_warnings_json", []string{}).Optional(),
	}
}

func (SchedulerRequestSnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("request_id", "attempt_no").Unique(),
		index.Fields("decision_id").Unique(),
		index.Fields("strategy", "created_at"),
		index.Fields("selected_account_id", "created_at"),
	}
}
