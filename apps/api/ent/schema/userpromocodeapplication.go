package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type UserPromoCodeApplication struct {
	ent.Schema
}

func (UserPromoCodeApplication) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (UserPromoCodeApplication) Fields() []ent.Field {
	return []ent.Field{
		field.Int("user_id"),
		field.Int("promo_code_id"),
		field.String("code_digest").NotEmpty(),
		field.Int("payment_order_id"),
		field.String("order_no").NotEmpty(),
		field.String("original_amount").Default("0.00000000"),
		field.String("discount_amount").Default("0.00000000"),
		field.String("final_amount").Default("0.00000000"),
		field.String("currency").Default("USD"),
		field.String("discount_type").NotEmpty(),
		field.Time("applied_at"),
		field.JSON("metadata_json", map[string]any{}).Optional(),
	}
}

func (UserPromoCodeApplication) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("payment_order_id").Unique(),
		index.Fields("order_no").Unique(),
		index.Fields("promo_code_id"),
		index.Fields("user_id", "applied_at"),
		index.Fields("code_digest"),
	}
}
