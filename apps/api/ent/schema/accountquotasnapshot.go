package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AccountQuotaSnapshot struct {
	ent.Schema
}

func (AccountQuotaSnapshot) Fields() []ent.Field {
	return []ent.Field{
		field.Int("account_id"),
		field.Int("provider_id"),
		field.String("quota_type").NotEmpty(),
		field.String("remaining").Default("0"),
		field.String("used").Default("0"),
		field.String("quota_limit").Default("0"),
		field.Float("remaining_ratio").Default(0),
		field.Time("reset_at").Optional().Nillable(),
		field.Time("snapshot_at"),
	}
}

func (AccountQuotaSnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_id", "quota_type", "snapshot_at"),
		index.Fields("reset_at"),
	}
}
