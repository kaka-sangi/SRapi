package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ScheduledTestPlan is an operator-managed plan that periodically runs a real
// generative connectivity probe against a scope of provider accounts (one
// account, an account group, or all accounts). It complements the global
// env-only interval prober with named, admin-configurable schedules.
type ScheduledTestPlan struct {
	ent.Schema
}

func (ScheduledTestPlan) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (ScheduledTestPlan) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.Bool("enabled").Default(true),
		// scope_type: all | account | group
		field.String("scope_type").Default("all"),
		// scope_id is the account_id or group_id when scope_type is account/group.
		field.Int("scope_id").Optional().Nillable(),
		// interval_seconds drives the cadence; the next run is last_run_at + interval.
		field.Int("interval_seconds").Default(3600),
		// cron_expression is an optional human-facing label / future cron source;
		// when present and parseable it overrides interval_seconds for due math.
		field.String("cron_expression").Optional(),
		// probe_model overrides account/provider metadata probe model keys.
		field.String("probe_model").Default(""),
		// max_results bounds how many accounts a single run probes (0 = unbounded).
		field.Int("max_results").Default(0),
		// auto_recover flips a recovered-but-cooled account back to active.
		field.Bool("auto_recover").Default(false),
		field.Time("last_run_at").Optional().Nillable(),
		field.String("last_status").Default(""),
		field.String("last_summary").Default(""),
	}
}

func (ScheduledTestPlan) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled"),
		index.Fields("scope_type", "scope_id"),
	}
}
