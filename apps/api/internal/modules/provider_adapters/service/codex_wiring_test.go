package service

import (
	"encoding/base64"
	"encoding/json"
	"sync"
	"testing"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
)

// codex_identity_confuse metadata flag is the operator's opt-in for the
// PR-1 identity_confuse module. Default OFF (back-compat with operators who
// pin specific identifiers in their integration tests); flag ON activates
// the verbatim CLIProxyAPI rewrite per-account.
func TestCodexIdentityConfuseConfigForAccountGatedOnMetadataFlag(t *testing.T) {
	cases := []struct {
		name     string
		metadata map[string]any
		want     bool
	}{
		{"absent_flag_defaults_off", nil, false},
		{"explicit_false_off", map[string]any{"codex_identity_confuse": false}, false},
		{"empty_string_off", map[string]any{"codex_identity_confuse": ""}, false},
		{"bool_true_on", map[string]any{"codex_identity_confuse": true}, true},
		{"string_true_on", map[string]any{"codex_identity_confuse": "true"}, true},
		{"string_TRUE_on", map[string]any{"codex_identity_confuse": "TRUE"}, true},
		{"string_1_on", map[string]any{"codex_identity_confuse": "1"}, true},
		{"string_yes_on", map[string]any{"codex_identity_confuse": "yes"}, true},
		{"string_on_on", map[string]any{"codex_identity_confuse": "on"}, true},
		{"string_garbage_off", map[string]any{"codex_identity_confuse": "maybe"}, false},
		{"non_string_non_bool_off", map[string]any{"codex_identity_confuse": 42}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := codexIdentityConfuseConfigForAccount(accountcontract.ProviderAccount{
				ID:       7,
				Metadata: tc.metadata,
			})
			got := CodexIdentityConfuseEnabled(cfg)
			if got != tc.want {
				t.Errorf("Enabled = %v, want %v (metadata=%v)", got, tc.want, tc.metadata)
			}
		})
	}
}

// When the flag is on, the outbound wiring actually rewrites
// prompt_cache_key in the body. The original is stashed in state so the
// response-side reverse-map can restore it.
func TestCodexApplyOutboundWiringRewritesPromptCacheKeyWhenEnabled(t *testing.T) {
	body := []byte(`{
		"model":"codex-upstream",
		"prompt_cache_key":"original-session-id",
		"input":[{"role":"user","content":"hi"}]
	}`)
	account := accountcontract.ProviderAccount{
		ID: 42,
		Metadata: map[string]any{
			"codex_identity_confuse": true,
		},
	}
	newBody, state := codexApplyOutboundWiring(account, nil, body)
	if !state.Enabled {
		t.Fatalf("state.Enabled = false; expected true for flagged account")
	}
	if state.OriginalPromptCacheKey != "original-session-id" {
		t.Errorf("state.OriginalPromptCacheKey = %q, want %q", state.OriginalPromptCacheKey, "original-session-id")
	}
	if state.PromptCacheKey == "" || state.PromptCacheKey == "original-session-id" {
		t.Errorf("state.PromptCacheKey was not rewritten: %q", state.PromptCacheKey)
	}
	var parsed map[string]any
	if err := json.Unmarshal(newBody, &parsed); err != nil {
		t.Fatalf("unmarshal new body: %v", err)
	}
	if got := parsed["prompt_cache_key"]; got != state.PromptCacheKey {
		t.Errorf("body prompt_cache_key = %v, want rewritten %q", got, state.PromptCacheKey)
	}
}

// When the flag is OFF the wiring is a pure no-op — the same byte slice
// comes back and state is zero. This is the path every pre-existing Codex
// test relies on.
func TestCodexApplyOutboundWiringIsNoopWhenDisabled(t *testing.T) {
	body := []byte(`{"model":"codex-upstream","prompt_cache_key":"keep-me"}`)
	account := accountcontract.ProviderAccount{ID: 42, Metadata: nil}
	newBody, state := codexApplyOutboundWiring(account, nil, body)
	if state.Enabled {
		t.Errorf("state.Enabled = true; expected false for unflagged account")
	}
	if string(newBody) != string(body) {
		t.Errorf("body was modified despite the flag being off:\n  before: %s\n   after: %s", body, newBody)
	}
}

// Cache capture walks the response's `output` array and persists each
// reasoning item under (model, original prompt_cache_key). With the flag
// on, the state's OriginalPromptCacheKey is what we key on (NOT the
// rewritten value — subsequent turns from the same client send the
// original key so the lookup-side must match).
//
// The Fernet shape check inside the cache normalizer is real (port from
// CLIProxyAPI verbatim), so the test sample mirrors the existing
// validCodexReasoningReplayEncryptedContentForTest helper from
// codex_reasoning_replay_cache_test.go: a 73-byte payload with version
// byte 0x80 at offset 0, base64url-encoded.
func TestCodexCaptureInboundWiringPopulatesReplayCache(t *testing.T) {
	model := "codex-upstream"
	original := "session-key-wiring-cache-test"
	state := CodexIdentityConfuseState{
		Enabled:                true,
		AuthID:                 "srapi-acct-42",
		OriginalPromptCacheKey: original,
		PromptCacheKey:         "anything-confused",
	}

	// Build a Fernet-shape valid encrypted_content the same way the
	// cache's own test suite does — version byte 0x80, 16-byte IV, AES-
	// block aligned ciphertext + HMAC tag, base64url-encoded.
	payload := make([]byte, 1+8+16+16+32)
	payload[0] = 0x80
	for i := 9; i < len(payload); i++ {
		payload[i] = byte(7 + i)
	}
	encryptedContent := base64.RawURLEncoding.EncodeToString(payload)

	responseBody := []byte(`{
		"output":[
			{"type":"reasoning","summary":[],"content":null,"encrypted_content":"` + encryptedContent + `"},
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}
		]
	}`)

	// Fresh cache for this test so we don't pick up state from other tests
	// in the package. Reset the sync.Once so codexReasoningReplayCache()
	// won't re-init on a future call inside this test.
	codexReasoningReplayCacheOnce = sync.Once{}
	codexReasoningReplayCacheInst = NewCodexReasoningReplayCache(
		codexReasoningReplayCacheMaxEntries,
		codexReasoningReplayCacheEvictBatch,
		codexReasoningReplayCacheTTL,
		time.Now,
	)
	codexCaptureInboundWiring(state, model, responseBody)

	got, ok := codexReasoningReplayCacheInst.GetItems(model, original)
	if !ok {
		t.Fatalf("expected cached items under (%q, %q); cache miss", model, original)
	}
	if len(got) == 0 {
		t.Fatalf("cached item list is empty")
	}
}
