package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Proxy struct {
	ent.Schema
}

func (Proxy) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (Proxy) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.String("type").NotEmpty(),
		// --- Structured proxy fields (sub2api style) ---
		// protocol is the proxy scheme: "http", "https", "socks5", "socks5h".
		field.String("protocol").Default("http"),
		// host is the proxy server hostname or IP.
		field.String("host").Default(""),
		// port is the proxy port number.
		field.Int("port").Default(0),
		// username is the optional proxy auth username.
		field.String("username").Default("").Optional(),
		// password_ciphertext is the AES-GCM encrypted proxy password.
		field.Bytes("password_ciphertext").Sensitive().Optional(),
		// --- Legacy encrypted URL blob (kept for migration) ---
		field.Bytes("url_ciphertext").Sensitive().Optional(),
		field.Int("url_version").Default(1),
		// ---
		field.String("status").Default("active"),
		field.JSON("metadata_json", map[string]any{}).Optional(),
		field.String("country_code").MaxLen(2).Default("").Optional(),
		field.String("country_name").MaxLen(128).Default("").Optional(),
		field.Time("expires_at").Optional().Nillable(),
		field.String("fallback_mode").Default("none"),
		field.Int("backup_proxy_id").Optional().Nillable(),
		// expiry_warn_days is how many days before expiry to surface a warning.
		field.Int("expiry_warn_days").Default(0),
		field.Time("last_probed_at").Optional().Nillable(),
		field.Int("probe_success_count").Default(0),
		field.Int("probe_failure_count").Default(0),
		field.Int("last_probe_latency_ms").Default(0),
	}
}

func (Proxy) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").Unique(),
		index.Fields("status"),
		index.Fields("type", "status"),
		index.Fields("expires_at"),
		index.Fields("backup_proxy_id"),
	}
}
