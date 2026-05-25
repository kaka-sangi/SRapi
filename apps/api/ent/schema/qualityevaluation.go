package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type QualityEvaluation struct {
	ent.Schema
}

func (QualityEvaluation) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (QualityEvaluation) Fields() []ent.Field {
	return []ent.Field{
		field.Int("feedback_id"),
		field.String("request_id").NotEmpty(),
		field.Int("decision_id"),
		field.Int("attempt_no").Default(1),
		field.Int("account_id"),
		field.Int("provider_id"),
		field.String("model").Default(""),
		field.String("source_endpoint").Default(""),
		field.String("sample_request_hash").NotEmpty(),
		field.String("judge_model").Default(""),
		field.Float("score").Default(0),
		field.JSON("rubric_json", map[string]any{}).Optional(),
		field.Time("judged_at"),
	}
}

func (QualityEvaluation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("feedback_id").Unique(),
		index.Fields("decision_id"),
		index.Fields("account_id", "model", "judged_at"),
		index.Fields("judge_model", "judged_at"),
		index.Fields("sample_request_hash"),
	}
}
