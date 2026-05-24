package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type SchedulerDecision struct {
	ent.Schema
}

func (SchedulerDecision) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (SchedulerDecision) Fields() []ent.Field {
	return []ent.Field{
		field.String("request_id").NotEmpty(),
		field.Int("attempt_no").Default(1),
		field.Int("user_id"),
		field.Int("api_key_id"),
		field.String("source_protocol").Default("openai-compatible"),
		field.String("source_endpoint").Default(""),
		field.String("target_protocol").Default(""),
		field.String("model").Default(""),
		field.String("strategy").Default("balanced"),
		field.String("strategy_version").Default("v1"),
		field.String("strategy_config_hash").Default(""),
		field.Int("fallback_from_decision_id").Optional().Nillable(),
		field.Int("selected_provider_id").Optional().Nillable(),
		field.Int("selected_account_id").Optional().Nillable(),
		field.Int("candidate_count").Default(0),
		field.Int("rejected_count").Default(0),
		field.JSON("scores_json", map[string]any{}).Optional(),
		field.JSON("reject_reasons_json", map[string]any{}).Optional(),
		field.JSON("strategy_weights_json", map[string]any{}).Optional(),
		field.JSON("compatibility_warnings_json", []string{}).Optional(),
		field.Bool("sticky_hit").Default(false),
		field.Bool("cache_affinity_hit").Default(false),
		field.String("estimated_cost").Default("0.00000000"),
		field.String("currency").Default("USD"),
	}
}

func (SchedulerDecision) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("request_id", "attempt_no").Unique(),
		index.Fields("user_id", "created_at"),
		index.Fields("api_key_id", "created_at"),
		index.Fields("selected_account_id", "created_at"),
		index.Fields("strategy", "created_at"),
	}
}
