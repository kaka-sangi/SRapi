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
		// token_expires_at is snapshotted from the OAuth credential map after a
		// refresh so the admin list/worker can filter on expiry without having
		// to decrypt credential_ciphertext. The authoritative expires_at still
		// lives inside the encrypted credential.
		field.Time("token_expires_at").Optional().Nillable(),
		// last_refreshed_at is the wall-clock time of the most recent successful
		// OAuth refresh. Cleared until the first success.
		field.Time("last_refreshed_at").Optional().Nillable(),
		// needs_reauth_at is set by the refresh worker (or the on-demand admin
		// endpoint) when refresh has become hopeless — either the upstream
		// returned a permanent OAuth error (invalid_grant et al.) or
		// refresh_attempts crossed the failure threshold. While non-nil, the
		// worker skips this account so it stops hammering the upstream.
		field.Time("needs_reauth_at").Optional().Nillable(),
		// refresh_attempts is the consecutive-failure counter. Reset to 0 on a
		// successful refresh.
		field.Int("refresh_attempts").Default(0),
		// refresh_last_error captures the most recent refresh error message
		// (truncated server-side to 500 chars) so operators can see WHY the
		// account flipped into needs_reauth.
		field.String("refresh_last_error").Default("").MaxLen(500),
	}
}

func (ProviderAccount) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider_id", "status"),
		index.Fields("status", "priority"),
	}
}

// Scale trigger: when active provider accounts exceed 5,000, promote hot
// scheduling metadata keys into typed indexed columns. See docs/constraints/CAPABILITY_BOUNDARIES.md.
