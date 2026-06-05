package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MonitorRunResult is one persisted outcome of running a monitor definition. A
// single run can produce several per-account / per-model check results, all
// sharing a run_id, so the run-history can group them. results holds the raw
// per-model CheckResult list produced by the underlying probe path.
type MonitorRunResult struct {
	ent.Schema
}

func (MonitorRunResult) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (MonitorRunResult) Fields() []ent.Field {
	return []ent.Field{
		field.Int("monitor_id"),
		field.String("run_id").NotEmpty(),
		field.Bool("ok").Default(false),
		field.Int("checked_count").Default(0),
		field.Int("ok_count").Default(0),
		field.Int("latency_ms").Default(0),
		field.String("trigger").Default("manual"), // manual | scheduled
		field.JSON("results", []map[string]any{}).Optional(),
	}
}

func (MonitorRunResult) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("monitor_id"),
		index.Fields("run_id"),
	}
}
