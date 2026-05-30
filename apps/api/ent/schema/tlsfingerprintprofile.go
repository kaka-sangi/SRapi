package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TLSFingerprintProfile is an operator-managed, named egress fingerprint profile
// (uTLS template + HTTP version policy + user agent + static headers). Accounts
// reference a profile by name so operators can manage fingerprints centrally
// instead of repeating egress_profile metadata on every account.
type TLSFingerprintProfile struct {
	ent.Schema
}

func (TLSFingerprintProfile) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (TLSFingerprintProfile) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.String("tls_template").Default(""),
		field.String("http_version_policy").Default("prefer_h2"),
		field.String("user_agent").Default(""),
		field.JSON("extra_headers", map[string]string{}).Optional(),
		field.Bool("enabled").Default(true),
	}
}

func (TLSFingerprintProfile) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").Unique(),
		index.Fields("enabled"),
	}
}
