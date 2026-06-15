package service

import (
	"testing"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
)

// TestSelectCodexAccountsCheckAccountPrefersDefaultOverStoredOrgFieldMatch proves
// the reordered selection: when the stored organization_id only field-matches a
// NON-default account (via a nested account.id, not a direct accounts-map key),
// but a DIFFERENT account carries account.is_default == true, the is_default
// account must win. This mirrors sub2api's openai_privacy_service.go, which
// resolves is_default at the same priority as the org-ID match.
func TestSelectCodexAccountsCheckAccountPrefersDefaultOverStoredOrgFieldMatch(t *testing.T) {
	const storedOrgID = "org-stored-id"

	// Map keys deliberately differ from storedOrgID so the direct accounts-map
	// key lookup misses and the stored-org-ID FIELD match loop is exercised.
	accounts := map[string]any{
		"acct-key-A": map[string]any{
			// Stored organization_id field-matches this account's nested id,
			// but it is NOT the default account.
			"account":     map[string]any{"id": storedOrgID, "plan_type": "team", "is_default": false},
			"entitlement": map[string]any{"subscription_plan": "team"},
		},
		"acct-key-B": map[string]any{
			// A different account that is the user's current default org.
			"account":     map[string]any{"id": "some-other-id", "plan_type": "pro", "is_default": true},
			"entitlement": map[string]any{"subscription_plan": "pro"},
		},
	}

	req := contract.ProbeRequest{
		Provider: providercontract.Provider{
			ID:          22,
			AdapterType: "reverse-proxy-codex-cli",
		},
		Account: accountcontract.ProviderAccount{
			ID:       220,
			Metadata: map[string]any{},
		},
		Credential: map[string]any{"organization_id": storedOrgID},
	}

	got := selectCodexAccountsCheckAccount(accounts, req)
	if got == nil {
		t.Fatalf("selectCodexAccountsCheckAccount returned nil, want the is_default account")
	}

	if !nestedBoolField(got, "account", "is_default") {
		t.Fatalf("expected the is_default account to be selected, got non-default account: %#v", got)
	}
	if plan := codexAccountsCheckPlan(got); plan != "pro" {
		t.Fatalf("expected default account plan %q, got %q (account: %#v)", "pro", plan, got)
	}
}

// TestQuotaAccountWithCloudflareTLSDefault proves the Cloudflare quota fix: a quota
// fetch to chatgpt.com (behind Cloudflare's JS challenge) defaults the egress TLS
// template to Chrome so the handshake is not fingerprinted as a bot, while
// non-Cloudflare endpoints and operator-configured TLS profiles are untouched.
func TestQuotaAccountWithCloudflareTLSDefault(t *testing.T) {
	const cfEndpoint = "https://chatgpt.com/backend-api/accounts/check/v4-2023-04-27"

	// chatgpt.com with no TLS config -> Chrome default injected on a copy.
	original := accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": "https://chatgpt.com/backend-api/codex"}}
	got := quotaAccountWithCloudflareTLSDefault(original, cfEndpoint)
	if mapString(got.Metadata, "tls_template") != "chrome" {
		t.Fatalf("expected chrome tls_template default for chatgpt.com, got %q", mapString(got.Metadata, "tls_template"))
	}
	if _, mutated := original.Metadata["tls_template"]; mutated {
		t.Fatal("original account metadata must not be mutated")
	}

	// An operator-configured TLS template is respected, not overridden.
	configured := accountcontract.ProviderAccount{Metadata: map[string]any{"tls_template": "chrome_133"}}
	if got := quotaAccountWithCloudflareTLSDefault(configured, cfEndpoint); mapString(got.Metadata, "tls_template") != "chrome_133" {
		t.Fatalf("operator tls_template must be respected, got %q", mapString(got.Metadata, "tls_template"))
	}

	// A non-Cloudflare quota endpoint is left unchanged.
	other := accountcontract.ProviderAccount{Metadata: map[string]any{}}
	if got := quotaAccountWithCloudflareTLSDefault(other, "https://api.anthropic.com/v1/me/usage"); mapString(got.Metadata, "tls_template") != "" {
		t.Fatalf("non-cloudflare endpoint must not get a tls default, got %q", mapString(got.Metadata, "tls_template"))
	}
}
