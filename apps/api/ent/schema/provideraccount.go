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
		// platform is the provider family denormalized from Provider for fast
		// filtering without JOIN: "anthropic", "openai", "gemini", "antigravity".
		field.String("platform").Default(""),
		// account_type is the sub2api-style simplified auth type:
		// "apikey", "oauth", "setup-token", "upstream", "bedrock", "service_account".
		// Default matches the initial migration; the service layer maps
		// runtime_class → account_type via RuntimeClassToAccountType.
		field.String("account_type").Default("api_key"),
		// runtime_class is the detailed internal auth method used for dispatch.
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
		// notes is operator-supplied freetext visible on the admin panel.
		field.String("notes").Default("").Optional(),
		// concurrency is the max concurrent upstream requests for this account.
		field.Int("concurrency").Default(3),
		// rate_multiplier is a per-account billing multiplier (discount: < 1.0;
		// markup: > 1.0). Default 1.0.
		field.Float("rate_multiplier").Default(1),
		// load_factor controls load distribution weight in the scheduler.
		field.Int("load_factor").Optional().Nillable(),
		// schedulable is a direct flag the scheduler reads; false skips the
		// account without changing its status.
		field.Bool("schedulable").Default(true),
		// error_message captures the last error for operator troubleshooting.
		field.String("error_message").Default(""),
		// last_used_at tracks when this account last served a request.
		field.Time("last_used_at").Optional().Nillable(),
		// expires_at is the operator-defined account expiration. NOT the OAuth
		// token expiry — that is token_expires_at.
		field.Time("expires_at").Optional().Nillable(),
		// auto_pause_on_expired pauses scheduling when expires_at is reached.
		field.Bool("auto_pause_on_expired").Default(true),
		// rate_limited_at records when a 429 was last received.
		field.Time("rate_limited_at").Optional().Nillable(),
		// rate_limit_reset_at records when the current rate limit window expires.
		field.Time("rate_limit_reset_at").Optional().Nillable(),
		// overload_until records when a 529/overload window expires.
		field.Time("overload_until").Optional().Nillable(),
		// temp_unschedulable_until is a rule-driven exclusion window.
		field.Time("temp_unschedulable_until").Optional().Nillable(),
		field.String("temp_unschedulable_reason").Default(""),
		// session_window_* track provider session windows for some APIs.
		field.Time("session_window_start").Optional().Nillable(),
		field.Time("session_window_end").Optional().Nillable(),
		field.String("session_window_status").Default(""),
		// extra_json holds per-account config (model_mapping, base_url, quotas)
		// separate from metadata_json which is operational state.
		field.JSON("extra_json", map[string]any{}).Optional(),
		// --- OAuth refresh fields (existing) ---
		field.Time("token_expires_at").Optional().Nillable(),
		field.Time("last_refreshed_at").Optional().Nillable(),
		field.Time("needs_reauth_at").Optional().Nillable(),
		field.Int("refresh_attempts").Default(0),
		field.String("refresh_last_error").Default("").MaxLen(500),
	}
}

func (ProviderAccount) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider_id", "status"),
		index.Fields("status", "priority"),
		index.Fields("platform", "status"),
		index.Fields("platform", "priority"),
		index.Fields("schedulable"),
		index.Fields("expires_at"),
		index.Fields("rate_limited_at"),
		index.Fields("rate_limit_reset_at"),
		index.Fields("overload_until"),
		index.Fields("last_used_at"),
	}
}
