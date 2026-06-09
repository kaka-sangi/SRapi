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
		field.Int("attempt_no").Default(1),
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
		field.Int("cache_creation_tokens").Default(0),
		field.Int("total_tokens").Default(0),
		field.Bool("usage_estimated").Default(false),
		field.Int("latency_ms").Default(0),
		field.Bool("success").Default(false),
		field.String("error_class").Optional().Nillable(),
		field.String("cost").Default("0.00000000"),
		field.String("actual_cost").Default("0.00000000"),
		field.String("rate_multiplier").Default("1.00000000"),
		// billable_cost is the portion of cost charged to the user's balance after
		// subscription allowance coverage. Equals cost unless a subscription in
		// allowance mode covered part/all of the request (WP-1180).
		field.String("billable_cost").Default("0.00000000"),
		field.String("input_cost").Default("0.00000000"),
		field.String("output_cost").Default("0.00000000"),
		field.String("cache_read_cost").Default("0.00000000"),
		field.String("cache_write_cost").Default("0.00000000"),
		field.String("requested_model").Default(""),
		field.String("upstream_model").Default(""),
		field.String("billing_mode").Default("token"),
		field.String("currency").Default("USD"),
		field.Time("charged_at").Optional().Nillable(),
		field.JSON("compatibility_warnings_json", []string{}).Optional(),
	}
}

func (UsageLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("request_id", "attempt_no").Unique(),
		index.Fields("user_id", "created_at"),
		index.Fields("charged_at", "success", "created_at"),
		index.Fields("api_key_id", "created_at"),
		index.Fields("account_id", "created_at"),
		index.Fields("source_endpoint", "created_at"),
		index.Fields("model", "created_at"),
	}
}
