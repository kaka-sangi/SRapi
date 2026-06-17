package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type RedeemCode struct {
	ent.Schema
}

func (RedeemCode) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (RedeemCode) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").NotEmpty(),
		field.String("type").NotEmpty(),
		field.String("status").Default("active"),
		field.String("value").Default(""),
		field.String("currency").Default("USD"),
		field.Int("max_redemptions").Default(1),
		field.Int("redeemed_count").Default(0),
		field.Time("expires_at").Optional().Nillable(),
		// note carries an operator-supplied audit comment, written by bulk-disable
		// (and reserved for any future per-row admin action). Free-text up to
		// ~500 chars; validation lives in the service layer so the schema stays
		// permissive for replay/migration.
		field.String("note").Default("").Optional(),
		// disabled_reason classifies WHY a code ended up in disabled status, so
		// the bulk-disable response can give operators a real per-row breakdown
		// instead of an opaque "failed" count. Values:
		//   - "admin_action"      operator-driven disable
		//   - "already_disabled"  no-op skip (was already disabled)
		//   - "expired"           disabled because expires_at had passed
		//   - ""                  active/used/unset
		field.String("disabled_reason").Default("").Optional(),
	}
}

func (RedeemCode) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("code").Unique(),
		index.Fields("status", "created_at"),
		index.Fields("expires_at"),
	}
}
