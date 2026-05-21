package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type SchedulerFeedback struct {
	ent.Schema
}

func (SchedulerFeedback) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (SchedulerFeedback) Fields() []ent.Field {
	return []ent.Field{
		field.String("request_id").NotEmpty(),
		field.Int("decision_id"),
		field.Int("attempt_no").Default(1),
		field.Int("account_id"),
		field.Int("provider_id"),
		field.String("model").Default(""),
		field.Bool("success").Default(false),
		field.String("error_class").Optional().Nillable(),
		field.Int("status_code").Optional().Nillable(),
		field.Int("latency_ms").Default(0),
		field.Int("input_tokens").Default(0),
		field.Int("output_tokens").Default(0),
		field.Int("cached_tokens").Default(0),
		field.String("actual_cost").Default("0.00000000"),
		field.String("currency").Default("USD"),
	}
}

func (SchedulerFeedback) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("decision_id"),
		index.Fields("request_id", "attempt_no"),
		index.Fields("account_id", "created_at"),
		index.Fields("provider_id", "created_at"),
		index.Fields("error_class", "created_at"),
	}
}
