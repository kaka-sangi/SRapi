package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ObsNotificationDelivery records every Ops alert notification attempt.
type ObsNotificationDelivery struct {
	ent.Schema
}

func (ObsNotificationDelivery) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (ObsNotificationDelivery) Fields() []ent.Field {
	return []ent.Field{
		field.Int("channel_id"),
		field.Int("alert_event_id"),
		field.String("alert_status").Default("firing"),
		field.String("severity").Default("warning"),
		field.String("status").Default("pending"),
		field.String("target").NotEmpty(),
		field.Int("attempt_count").Default(0),
		field.String("last_error").Default(""),
		field.Time("next_attempt_at"),
		field.Time("delivered_at").Optional().Nillable(),
		field.Time("last_attempt_at").Optional().Nillable(),
	}
}

func (ObsNotificationDelivery) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "next_attempt_at"),
		index.Fields("channel_id", "created_at"),
		index.Fields("alert_event_id", "created_at"),
		index.Fields("channel_id", "alert_event_id", "alert_status", "target").Unique(),
	}
}
