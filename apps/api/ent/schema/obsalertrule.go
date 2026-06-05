package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ObsAlertRule is a configurable, generic metric alert rule that fires
// AlertEvents independently of the SLO burn-rate evaluator.
type ObsAlertRule struct {
	ent.Schema
}

func (ObsAlertRule) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (ObsAlertRule) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.String("metric_type").Default("error_rate"),
		field.String("operator").Default("gt"),
		field.Float("threshold"),
		field.String("severity").Default("warning"),
		field.Bool("enabled").Default(true),
		field.Int("window_seconds").Default(3600),
		field.Int("cooldown_seconds").Default(0),
		field.Int("min_request_count").Default(0),
		field.JSON("scope_json", map[string]any{}).Optional(),
	}
}

func (ObsAlertRule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").Unique(),
		index.Fields("enabled", "metric_type"),
		index.Fields("severity", "enabled"),
	}
}
