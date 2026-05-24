package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type UsageLog struct {
	ent.Schema
}

func (UsageLog) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (UsageLog) Fields() []ent.Field {
	return []ent.Field{
		field.String("request_id").NotEmpty(),
		field.Int("user_id"),
		field.Int("api_key_id"),
		field.Int("provider_id").Optional().Nillable(),
		field.Int("account_id").Optional().Nillable(),
		field.String("source_protocol").Default("openai-compatible"),
		field.String("source_endpoint").Default(""),
		field.String("target_protocol").Default(""),
		field.String("model").Default(""),
		field.Int("input_tokens").Default(0),
		field.Int("output_tokens").Default(0),
		field.Int("cached_tokens").Default(0),
		field.Int("total_tokens").Default(0),
		field.Bool("usage_estimated").Default(false),
		field.Int("latency_ms").Default(0),
		field.Bool("success").Default(false),
		field.String("error_class").Optional().Nillable(),
		field.String("cost").Default("0.00000000"),
		field.String("currency").Default("USD"),
		field.Time("charged_at").Optional().Nillable(),
		field.JSON("compatibility_warnings_json", []string{}).Optional(),
	}
}

func (UsageLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("request_id").Unique(),
		index.Fields("user_id", "created_at"),
		index.Fields("charged_at"),
		index.Fields("api_key_id", "created_at"),
		index.Fields("account_id", "created_at"),
		index.Fields("source_endpoint", "created_at"),
		index.Fields("model", "created_at"),
	}
}
