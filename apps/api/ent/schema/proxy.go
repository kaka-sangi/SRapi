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
		field.Bytes("url_ciphertext").Sensitive().Optional(),
		field.Int("url_version").Default(1),
		field.String("status").Default("active"),
		field.JSON("metadata_json", map[string]any{}).Optional(),
		field.String("country_code").MaxLen(2).Default("").Optional().
			Comment("ISO-3166-1 alpha-2 country code (operator-supplied)."),
		field.String("country_name").MaxLen(128).Default("").Optional().
			Comment("Display name for the country, snapshotted at write time."),
		field.Time("expires_at").Optional().Nillable().
			Comment("Optional operator-defined expiry; expired proxies follow fallback_mode."),
		field.String("fallback_mode").Default("none").
			Comment("Expiry fallback mode: none, direct, or proxy."),
		field.Int("backup_proxy_id").Optional().Nillable().
			Comment("Proxy definition id used when fallback_mode is proxy."),
		field.Time("last_probed_at").Optional().Nillable().
			Comment("Last time the probe worker tested this proxy."),
		field.Int("probe_success_count").Default(0).
			Comment("Successful probes since last counter reset (~7 days). Used for availability %."),
		field.Int("probe_failure_count").Default(0).
			Comment("Failed probes since last counter reset (~7 days)."),
		field.Int("last_probe_latency_ms").Default(0).
			Comment("Latency of the most recent successful probe, in milliseconds. 0 if never succeeded."),
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
