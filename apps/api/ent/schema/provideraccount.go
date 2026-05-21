package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ProviderAccount struct {
	ent.Schema
}

func (ProviderAccount) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}}
}

func (ProviderAccount) Fields() []ent.Field {
	return []ent.Field{
		field.Int("provider_id"),
		field.String("name").NotEmpty(),
		field.String("account_type").Default("api_key"),
		field.String("runtime_class").Default("api_key"),
		field.String("upstream_client").Optional().Nillable(),
		field.Bytes("credential_ciphertext").Sensitive().Optional(),
		field.Int("credential_version").Default(1),
		field.String("proxy_id").Optional().Nillable(),
		field.String("status").Default("active"),
		field.Int("priority").Default(0),
		field.Float("weight").Default(1),
		field.String("risk_level").Default("normal"),
		field.JSON("metadata_json", map[string]any{}).Optional(),
	}
}

func (ProviderAccount) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider_id", "status"),
		index.Fields("status", "priority"),
	}
}
