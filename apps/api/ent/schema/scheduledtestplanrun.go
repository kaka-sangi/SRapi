package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ScheduledTestPlanRun records one execution of a ScheduledTestPlan: how many
// accounts were selected/probed/failed/unhealthy and whether any were
// auto-recovered. It is the run-history backing for the admin results view.
type ScheduledTestPlanRun struct {
	ent.Schema
}

func (ScheduledTestPlanRun) Fields() []ent.Field {
	return []ent.Field{
		field.Int("plan_id"),
		field.String("trigger").Default("schedule"), // schedule | manual
		field.String("status").Default("ok"),        // ok | partial | failed
		field.Int("selected").Default(0),
		field.Int("probed").Default(0),
		field.Int("skipped").Default(0),
		field.Int("failed").Default(0),
		field.Int("unhealthy").Default(0),
		field.Int("recovered").Default(0),
		field.String("summary").Default(""),
		field.Time("started_at"),
		field.Time("finished_at"),
	}
}

func (ScheduledTestPlanRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("plan_id", "started_at"),
	}
}
