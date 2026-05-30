package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type UserRedeemCodeRedemption struct {
	ent.Schema
}

func (UserRedeemCodeRedemption) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (UserRedeemCodeRedemption) Fields() []ent.Field {
	return []ent.Field{
		field.Int("user_id"),
		field.Int("redeem_code_id"),
		field.String("code_digest").NotEmpty(),
		field.String("type").NotEmpty(),
		field.String("amount").Default("0.00000000"),
		field.String("currency").Default("USD"),
		field.String("balance_before").Default("0.00000000"),
		field.String("balance_after").Default("0.00000000"),
		field.Int("billing_ledger_id").Optional().Nillable(),
		field.Int("user_subscription_id").Optional().Nillable(),
		field.Time("redeemed_at"),
		field.JSON("metadata_json", map[string]any{}).Optional(),
	}
}

func (UserRedeemCodeRedemption) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "redeem_code_id").Unique(),
		index.Fields("redeem_code_id"),
		index.Fields("user_id", "redeemed_at"),
		index.Fields("code_digest"),
	}
}
