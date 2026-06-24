package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type PaymentProviderInstance struct {
	ent.Schema
}

func (PaymentProviderInstance) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (PaymentProviderInstance) Fields() []ent.Field {
	return []ent.Field{
		field.String("provider").NotEmpty(),
		field.String("name").NotEmpty(),
		field.String("status").Default("active"),
		field.Bytes("config_ciphertext").Sensitive().Optional(),
		field.Int("config_version").Default(1),
		field.JSON("supported_methods_json", []string{}).Optional(),
		field.JSON("limits_json", map[string]any{}).Optional(),
		field.Int("sort_order").Default(0),
		field.String("fee_rate").Default("0.00000000"),
		field.Int("weight").Default(1),
		field.JSON("metadata_json", map[string]any{}).Optional(),
	}
}

func (PaymentProviderInstance) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider", "name").Unique(),
		index.Fields("provider", "status"),
		index.Fields("status", "sort_order"),
	}
}
