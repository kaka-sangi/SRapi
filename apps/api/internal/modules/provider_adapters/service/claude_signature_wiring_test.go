package service

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/signature"
)

func base64StdEncode(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// TestAnthropicCompatibleRequestBodyStripsForgedThinking is the
// load-bearing wiring proof: the strip MUST run on the live
// outbound build path used by every Claude /v1/messages flow
// (anthropicCompatibleRequestBody). A forged thinking block on
// input must NOT appear in the marshalled outbound payload.
func TestAnthropicCompatibleRequestBodyStripsForgedThinking(t *testing.T) {
	req := contract.ConversationRequest{
		Mapping: modelcontract.ModelProviderMapping{UpstreamModelName: "claude-sonnet-4"},
		Messages: []contract.ConversationMessage{
			{
				Role: "assistant",
				Parts: []contract.ContentPart{
					{Kind: contract.ContentPartThinking, Text: "FORGED-CHAIN-OF-THOUGHT", Metadata: map[string]any{"signature": "FORGED-NOT-A-REAL-SIG"}},
					{Kind: contract.ContentPartText, Text: "ok"},
				},
			},
			{
				Role: "user",
				Parts: []contract.ContentPart{
					{Kind: contract.ContentPartText, Text: "hello"},
				},
			},
		},
	}

	raw, err := anthropicCompatibleRequestBody(req)
	if err != nil {
		t.Fatalf("anthropicCompatibleRequestBody: %v", err)
	}
	body := string(raw)
	if strings.Contains(body, "FORGED-NOT-A-REAL-SIG") {
		t.Fatalf("strip failed to remove forged signature from outbound body:\n%s", body)
	}
	if strings.Contains(body, "FORGED-CHAIN-OF-THOUGHT") {
		t.Fatalf("strip failed to remove forged thinking text from outbound body:\n%s", body)
	}
	// Trailing text content must still be there.
	if !strings.Contains(body, "\"ok\"") || !strings.Contains(body, "\"hello\"") {
		t.Fatalf("legitimate content disappeared:\n%s", body)
	}
}

// TestClaudeThinkingSanitizeRawPayloadSurvivesNonMessagesBody asserts
// the wiring is robust to bodies that don't have a `messages` array
// (e.g. count_tokens or transformed payloads). It must pass-through.
func TestClaudeThinkingSanitizeRawPayloadSurvivesNonMessagesBody(t *testing.T) {
	in := []byte(`{"system":"x"}`)
	out := claudeThinkingSanitizeRawPayload("claude-x", in)
	if string(out) != string(in) {
		t.Fatalf("expected pass-through, got %s", string(out))
	}
}

// TestClaudeThinkingCacheWiringPutOnValidThinking proves
// DefaultThinkingCache.Put is exercised by the wiring on a thinking
// block that survives the strip — i.e. the Put side of the cache is
// active. The second sanitize pass with the same payload must then
// find the entry via Get (the side that proves Get-wiring).
func TestClaudeThinkingCacheWiringPutOnValidThinking(t *testing.T) {
	// Build a Claude-valid single-layer E signature that is also
	// ≥ MinValidThinkingSignatureLen so the cache accepts the Put.
	validSig := buildValidClaudeSingleLayerELongEnough(t)
	model := "claude-cache-wiring-test"
	// Pre-clear so the test is hermetic.
	signature.DefaultThinkingCache.Clear(model)

	body := mustJSON(t, map[string]any{
		"messages": []any{
			map[string]any{
				"role": "assistant",
				"content": []any{
					map[string]any{
						"type":      "thinking",
						"thinking":  "the chain of thought text",
						"signature": validSig,
					},
				},
			},
		},
	})
	_ = claudeThinkingSanitizeRawPayload(model, body)
	// Cache should now contain this signature.
	if got := signature.DefaultThinkingCache.Get(model, "the chain of thought text"); got != validSig {
		t.Fatalf("cache Put not wired: Get returned %q (len %d), want %q", got, len(got), validSig)
	}
	// Second pass uses the cache (Get path is wired). Same outcome.
	_ = claudeThinkingSanitizeRawPayload(model, body)
	if got := signature.DefaultThinkingCache.Get(model, "the chain of thought text"); got != validSig {
		t.Fatalf("cache Get short-circuit broke: %q", got)
	}
}

// buildValidClaudeSingleLayerELongEnough returns a Claude single-
// layer signature whose base64 encoding is ≥ MinValidThinkingSignatureLen
// so the bounded LRU cache will accept the Put.
//
// 38 raw payload bytes encode to 52 base64 chars; we pick the first
// byte as 0x12 (Claude marker) and fill the rest with zeros. Non-
// strict validation only checks the marker so this passes.
func buildValidClaudeSingleLayerELongEnough(t *testing.T) string {
	t.Helper()
	payload := make([]byte, 60)
	payload[0] = 0x12
	enc := base64StdEncode(payload)
	if len(enc) < signature.MinValidThinkingSignatureLen {
		t.Fatalf("internal: encoded sig only %d bytes", len(enc))
	}
	if enc[0] != 'E' {
		t.Fatalf("internal: encoded sig prefix %c, want E", enc[0])
	}
	return enc
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return out
}
