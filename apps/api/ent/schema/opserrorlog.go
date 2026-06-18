package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type OpsErrorLog struct {
	ent.Schema
}

func (OpsErrorLog) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (OpsErrorLog) Fields() []ent.Field {
	return []ent.Field{
		field.Time("occurred_at"),
		field.String("request_id").Default(""),
		field.String("trace_id").Default(""),
		field.Int("user_id").Optional().Nillable(),
		field.Int("api_key_id").Optional().Nillable(),
		field.Int("account_id").Optional().Nillable(),
		field.Int("provider_id").Optional().Nillable(),
		field.String("platform").Default(""),
		field.String("source_endpoint").Default(""),
		field.String("target_protocol").Default(""),
		field.String("model").Default(""),
		field.Int("status_code").Optional().Nillable(),
		field.String("upstream_request_id").Default(""),
		field.Int("attempt_no").Default(1),
		field.Int("latency_ms").Default(0),
		field.Int("input_tokens").Default(0),
		field.Int("output_tokens").Default(0),
		field.Bool("usage_estimated").Default(false),
		field.String("error_class").Default("unknown"),
		field.String("error_phase").Default("upstream"),
		field.String("error_owner").Default("provider"),
		field.String("error_source").Default("upstream_http"),
		field.Text("error_message").Default(""),
		field.Text("error_body_excerpt").Default(""),
		field.JSON("upstream_errors_json", []map[string]any{}).Optional(),
		field.String("resolution").Default("open"),
		field.Text("resolution_note").Default(""),
		field.Time("resolved_at").Optional().Nillable(),
		field.Int("resolved_by_id").Optional().Nillable(),
	}
}

func (OpsErrorLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("occurred_at"),
		index.Fields("resolution", "occurred_at"),
		index.Fields("error_class", "occurred_at"),
		index.Fields("platform", "occurred_at"),
		index.Fields("target_protocol", "occurred_at"),
		index.Fields("user_id", "occurred_at"),
		index.Fields("api_key_id", "occurred_at"),
		index.Fields("account_id", "occurred_at"),
		index.Fields("provider_id", "occurred_at"),
		index.Fields("status_code", "occurred_at"),
		index.Fields("upstream_request_id"),
		index.Fields("request_id"),
		index.Fields("trace_id"),
	}
}
