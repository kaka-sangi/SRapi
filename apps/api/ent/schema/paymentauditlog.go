package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type PaymentAuditLog struct {
	ent.Schema
}

func (PaymentAuditLog) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (PaymentAuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.Int("order_id").Default(0),
		field.Int("provider_instance_id").Default(0),
		field.String("event_type").NotEmpty(),
		field.String("idempotency_key").NotEmpty(),
		field.JSON("payload_json", map[string]any{}).Optional(),
		field.Bool("signature_valid").Default(false),
	}
}

func (PaymentAuditLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("idempotency_key").Unique(),
		index.Fields("order_id", "created_at"),
		index.Fields("provider_instance_id", "created_at"),
		index.Fields("event_type", "created_at"),
	}
}
