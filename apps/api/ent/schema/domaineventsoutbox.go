package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type DomainEventsOutbox struct {
	ent.Schema
}

func (DomainEventsOutbox) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (DomainEventsOutbox) Fields() []ent.Field {
	return []ent.Field{
		field.String("event_id").NotEmpty(),
		field.String("event_type").NotEmpty(),
		field.String("event_version").Default("v1"),
		field.String("producer_module").NotEmpty(),
		field.String("aggregate_type").Default(""),
		field.String("aggregate_id").Default(""),
		field.String("correlation_id").Default(""),
		field.String("causation_id").Default(""),
		field.String("idempotency_key").Default(""),
		field.JSON("payload_json", map[string]any{}).Optional(),
		field.JSON("metadata_json", map[string]any{}).Optional(),
		field.String("status").Default("pending"),
		field.Int("attempt_count").Default(0),
		field.Time("next_retry_at").Optional().Nillable(),
		field.String("last_error").Optional().Nillable(),
		field.Time("published_at").Optional().Nillable(),
	}
}

func (DomainEventsOutbox) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("event_id").Unique(),
		index.Fields("producer_module", "idempotency_key").Unique(),
		index.Fields("status", "next_retry_at"),
		index.Fields("event_type", "created_at"),
		index.Fields("aggregate_type", "aggregate_id", "created_at"),
		index.Fields("correlation_id"),
	}
}
