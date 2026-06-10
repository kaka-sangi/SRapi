package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AccountAvailabilityRollup is a per-account, per-day aggregate of health
// snapshots used for availability reporting over a rolling window. It summarizes
// the finer-grained AccountHealthSnapshot rows into daily buckets.
type AccountAvailabilityRollup struct {
	ent.Schema
}

func (AccountAvailabilityRollup) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (AccountAvailabilityRollup) Fields() []ent.Field {
	return []ent.Field{
		field.Int("account_id"),
		field.Int("provider_id").Default(0),
		field.String("bucket_date"), // YYYY-MM-DD (UTC)
		field.Int("total_samples").Default(0),
		field.Int("healthy_samples").Default(0),
		field.Float("availability_ratio").Default(0),
		field.Float("avg_success_rate").Default(0),
		field.Time("computed_at"),
	}
}

func (AccountAvailabilityRollup) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_id", "bucket_date").Unique(),
		index.Fields("account_id"),
		index.Fields("bucket_date", "provider_id"),
	}
}
