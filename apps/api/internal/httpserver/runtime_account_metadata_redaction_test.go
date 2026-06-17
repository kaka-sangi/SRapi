package httpserver

import (
	"testing"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
)

// Two load-bearing redaction paths share one allowlist (sensitiveMetadataKey):
//   - toAPIAccount: GET /admin/accounts/{id} response body (operator-visible).
//   - accountAuditSnapshot: written to the audit ledger (persists forever).
//
// Both used to echo account.Metadata verbatim. Anything an operator (or an
// upstream provider, e.g. ChatGPT-web sentinel state) tucked into metadata
// under a token-ish key would leak. This pins the redaction down at both
// surfaces so a future "tidy up" PR can't quietly drop the call.

var redactionLeakySamples = map[string]any{
	"refresh_token":         "ya29-leak-me",
	"access_token":          "Bearer-NOPE",
	"oauth_client_secret":   "shhh",
	"cf_clearance_cookie":   "cookie-leak",
	"some_api_key":          "k-1234",
	"chatgpt_session_token": "session-leak",
	"private_key":           "-----BEGIN-----",
	"password":              "hunter2",
	// non-sensitive keys must SURVIVE
	"upstream_client": "codex_cli",
	"display_name":    "alice's codex",
	"region":          "us-west",
}

func assertRedacted(t *testing.T, label string, out map[string]any) {
	t.Helper()
	leakyKeys := []string{
		"refresh_token", "access_token", "oauth_client_secret",
		"cf_clearance_cookie", "some_api_key", "chatgpt_session_token",
		"private_key", "password",
	}
	for _, key := range leakyKeys {
		if _, present := out[key]; present {
			t.Errorf("%s: sensitive key %q leaked through redaction", label, key)
		}
	}
	for _, keep := range []string{"upstream_client", "display_name", "region"} {
		if _, present := out[keep]; !present {
			t.Errorf("%s: non-sensitive key %q dropped by redaction (should survive)", label, keep)
		}
	}
}

func TestToAPIAccountRedactsSensitiveMetadata(t *testing.T) {
	account := accountcontract.ProviderAccount{
		ID:           42,
		ProviderID:   1,
		Name:         "leaky-acct",
		RuntimeClass: "oauth_refresh",
		Status:       "active",
		Metadata:     cloneRedactionSample(),
	}
	api := toAPIAccount(account)
	if api.Metadata == nil {
		t.Fatalf("expected Metadata pointer, got nil")
	}
	assertRedacted(t, "toAPIAccount", map[string]any(*api.Metadata))
}

func TestAccountAuditSnapshotRedactsSensitiveMetadata(t *testing.T) {
	account := accountcontract.ProviderAccount{
		ID:           42,
		ProviderID:   1,
		Name:         "leaky-acct",
		RuntimeClass: "oauth_refresh",
		Status:       "active",
		Metadata:     cloneRedactionSample(),
	}
	snapshot := accountAuditSnapshot(account)
	rawMeta, ok := snapshot["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected snapshot[metadata] to be map[string]any, got %T", snapshot["metadata"])
	}
	assertRedacted(t, "accountAuditSnapshot", rawMeta)
}

func TestProxyAuditSnapshotRedactsSensitiveMetadata(t *testing.T) {
	proxy := accountcontract.ProxyDefinition{
		ID:       7,
		Name:     "leaky-proxy",
		Type:     "http",
		Status:   "active",
		Metadata: cloneRedactionSample(),
	}
	snapshot := proxyAuditSnapshot(proxy)
	rawMeta, ok := snapshot["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected snapshot[metadata] to be map[string]any, got %T", snapshot["metadata"])
	}
	assertRedacted(t, "proxyAuditSnapshot", rawMeta)
}

// Pin the allowlist directly — without this, dropping a marker from
// sensitiveMetadataKey would silently start leaking in production while
// the higher-level tests still pass (they only spot-check specific keys).
func TestSensitiveMetadataKeyMatchesAllExpectedMarkers(t *testing.T) {
	cases := map[string]bool{
		"authorization": true,
		"bearer_token":  true,
		"cookie":        true,
		"credential":    true,
		"password":      true,
		"passwd":        true,
		"private_key":   true,
		"secret":        true,
		"oauth_secret":  true,
		"api_key":       true,
		"some_key":      true, // *_key suffix
		"token":         true,
		"refresh_token": true,
		// negatives
		"upstream_client": false,
		"display_name":    false,
		"region":          false,
		"":                false,
	}
	for key, want := range cases {
		if got := sensitiveMetadataKey(key); got != want {
			t.Errorf("sensitiveMetadataKey(%q) = %v, want %v", key, got, want)
		}
	}
}

func cloneRedactionSample() map[string]any {
	out := make(map[string]any, len(redactionLeakySamples))
	for k, v := range redactionLeakySamples {
		out[k] = v
	}
	return out
}
