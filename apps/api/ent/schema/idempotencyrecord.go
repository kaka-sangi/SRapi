package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type IdempotencyRecord struct {
	ent.Schema
}

func (IdempotencyRecord) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (IdempotencyRecord) Fields() []ent.Field {
	return []ent.Field{
		field.String("idempotency_key").NotEmpty(),
		field.String("method").NotEmpty(),
		field.String("path").NotEmpty(),
		field.String("request_hash").NotEmpty(),
		field.String("status").Default("pending"),
		field.JSON("response_snapshot_json", map[string]any{}).Optional(),
		field.Time("locked_until").Optional().Nillable(),
		field.Time("expires_at"),
	}
}

func (IdempotencyRecord) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("idempotency_key", "method", "path").Unique(),
		index.Fields("expires_at"),
	}
}
