package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type UserTOTPSecret struct {
	ent.Schema
}

func (UserTOTPSecret) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (UserTOTPSecret) Fields() []ent.Field {
	return []ent.Field{
		field.Int("user_id"),
		field.Bytes("secret_ciphertext").Sensitive(),
		field.String("secret_version").Default("v1"),
		field.Bool("enabled").Default(false),
		field.JSON("recovery_code_hashes_json", []string{}).Optional(),
		field.Time("last_used_at").Optional().Nillable(),
	}
}

func (UserTOTPSecret) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id").Unique(),
		index.Fields("enabled", "user_id"),
	}
}
