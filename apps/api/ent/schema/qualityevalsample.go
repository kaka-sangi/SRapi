package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type QualityEvalSample struct {
	ent.Schema
}

func (QualityEvalSample) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (QualityEvalSample) Fields() []ent.Field {
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
		field.Bytes("sample_payload_ciphertext"),
		field.String("payload_version").Default("v1"),
		field.Time("captured_at"),
	}
}

func (QualityEvalSample) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("feedback_id").Unique(),
		index.Fields("decision_id"),
		index.Fields("request_id", "attempt_no"),
		index.Fields("account_id", "model", "captured_at"),
		index.Fields("sample_request_hash"),
	}
}
