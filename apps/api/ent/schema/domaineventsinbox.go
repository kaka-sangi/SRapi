package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type DomainEventsInbox struct {
	ent.Schema
}

func (DomainEventsInbox) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (DomainEventsInbox) Fields() []ent.Field {
	return []ent.Field{
		field.String("event_id").NotEmpty(),
		field.String("consumer_name").NotEmpty(),
		field.String("event_type").NotEmpty(),
		field.String("status").Default("pending"),
		field.Int("attempt_count").Default(0),
		field.String("last_error").Optional().Nillable(),
		field.Time("processed_at").Optional().Nillable(),
	}
}

func (DomainEventsInbox) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("event_id", "consumer_name").Unique(),
		index.Fields("consumer_name", "status", "created_at"),
	}
}
