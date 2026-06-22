package service

import (
	"strings"
)

// canonicalMetadataAliases enumerates the legacy alias → canonical mappings
// that historically polluted ProviderAccount.Metadata. Sub2api/Codex import
// paths used to write provider-specific names (codex_email, codex_account_id,
// chatgpt_user_id, …) which forced the admin UI to chase 2–3 fallback keys
// per field. Every metadata write — Create, Update, BatchUpdateFields, the
// Codex import path — runs through CanonicalizeAccountMetadata so storage
// holds canonical keys only.
//
// IMPORTANT: this map applies to ProviderAccount.Metadata exclusively. The
// credential map carries upstream-protocol field names (e.g. chatgpt_user_id
// is what the Codex JWT signer reads at dispatch time) and must NOT be
// rewritten — leave credentials alone.
var canonicalMetadataAliases = map[string]string{
	"codex_email":           "email",
	"codex_plan_type":       "plan_type",
	"codex_organization_id": "organization_id",
	"codex_account_id":      "upstream_account_id",
	"chatgpt_account_id":    "upstream_account_id",
	"chatgpt_user_id":       "upstream_user_id",
	"codex_user_id":         "upstream_user_id",
	"rpm_override":          "rpm_limit",
}

// CanonicalizeAccountMetadata rewrites alias keys into their canonical form
// and drops the aliases. Canonical wins when both are present (storage truth >
// stale alias). Returns the input map unchanged when nothing needs touching
// so the common path is allocation-free.
func CanonicalizeAccountMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return metadata
	}
	// Snapshot which aliases are present so we never mutate the input.
	var aliases []string
	for alias := range canonicalMetadataAliases {
		if _, ok := metadata[alias]; ok {
			aliases = append(aliases, alias)
		}
	}
	if len(aliases) == 0 {
		return metadata
	}
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		out[key] = value
	}
	for _, alias := range aliases {
		canonical := canonicalMetadataAliases[alias]
		value := out[alias]
		delete(out, alias)
		if _, has := out[canonical]; has {
			// Canonical already set — drop the alias without overwriting.
			continue
		}
		if value == nil {
			continue
		}
		if str, ok := value.(string); ok && strings.TrimSpace(str) == "" {
			continue
		}
		out[canonical] = value
	}
	return out
}
