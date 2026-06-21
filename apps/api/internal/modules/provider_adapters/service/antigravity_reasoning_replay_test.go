package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

func TestAntigravityUsesReasoningReplayCacheGate(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"gemini-3-pro", true},
		{"gemini-3.1-flash-image", true},
		{"agent-default", true},
		{"flash-2", true},
		{"GEMINI-2.5-flash", true},
		{"claude-haiku", false},
		{"claude-3.5-sonnet", false},
		{"random-model", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := antigravityUsesReasoningReplayCache(tc.model); got != tc.want {
			t.Fatalf("antigravityUsesReasoningReplayCache(%q) = %v, want %v", tc.model, got, tc.want)
		}
	}
}

func TestAntigravityScopeFromRequestPrefersSessionIDFromPayload(t *testing.T) {
	req := contract.ConversationRequest{
		Mapping: modelcontract.ModelProviderMapping{UpstreamModelName: "gemini-3-pro"},
	}
	payload := []byte(`{"request":{"contents":[],"sessionId":"sess-from-inner"},"sessionId":"sess-from-outer"}`)
	scope := antigravityReasoningReplayScopeFromRequest(req, payload)
	if !scope.valid() {
		t.Fatalf("expected scope.valid()")
	}
	if scope.sessionKey != "session:sess-from-outer" {
		t.Fatalf("expected outer sessionId to win, got %q", scope.sessionKey)
	}
	if scope.modelName != "gemini-3-pro" {
		t.Fatalf("expected model preserved, got %q", scope.modelName)
	}
}

func TestAntigravityScopeFromRequestUsesInnerSessionID(t *testing.T) {
	req := contract.ConversationRequest{
		Mapping: modelcontract.ModelProviderMapping{UpstreamModelName: "gemini-3-pro"},
	}
	payload := []byte(`{"request":{"contents":[],"sessionId":"sess-from-inner"}}`)
	scope := antigravityReasoningReplayScopeFromRequest(req, payload)
	if !scope.valid() || scope.sessionKey != "session:sess-from-inner" {
		t.Fatalf("expected inner sessionId path, got %+v", scope)
	}
}

func TestAntigravityScopeNoModelFamilyDisabled(t *testing.T) {
	req := contract.ConversationRequest{
		Mapping: modelcontract.ModelProviderMapping{UpstreamModelName: "claude-haiku"},
	}
	payload := []byte(`{"sessionId":"sess-x"}`)
	scope := antigravityReasoningReplayScopeFromRequest(req, payload)
	if scope.valid() {
		t.Fatalf("expected claude family to be excluded from replay scope, got %+v", scope)
	}
}

func TestAntigravityMergeThoughtSignatureIntoExistingPart(t *testing.T) {
	contents := []any{
		map[string]any{
			"role": "model",
			"parts": []any{
				map[string]any{"text": "hello"},
				map[string]any{"text": "thinking aloud"},
			},
		},
	}
	items := [][]byte{[]byte(`{"type":"thought_signature","thoughtSignature":"sig-abcdef0123456789","contentIndex":0,"partIndex":1}`)}
	updated, changed := mergeAntigravityReasoningReplayItems(contents, items)
	if !changed {
		t.Fatalf("expected change")
	}
	part := updated[0].(map[string]any)["parts"].([]any)[1].(map[string]any)
	if got := part["thoughtSignature"]; got != "sig-abcdef0123456789" {
		t.Fatalf("expected signature written, got %v", got)
	}
}

func TestAntigravityMergeRespectsExistingSignature(t *testing.T) {
	contents := []any{
		map[string]any{
			"role": "model",
			"parts": []any{
				map[string]any{"text": "h", "thoughtSignature": "preexisting-sig-1234"},
			},
		},
	}
	items := [][]byte{[]byte(`{"type":"thought_signature","thoughtSignature":"sig-abcdef0123456789","contentIndex":0,"partIndex":0}`)}
	updated, changed := mergeAntigravityReasoningReplayItems(contents, items)
	if changed {
		t.Fatalf("expected no change when signature already present")
	}
	if updated[0].(map[string]any)["parts"].([]any)[0].(map[string]any)["thoughtSignature"] != "preexisting-sig-1234" {
		t.Fatalf("expected preexisting signature to win")
	}
}

func TestAntigravityMergeFunctionCallBeforeResponse(t *testing.T) {
	contents := []any{
		map[string]any{
			"role": "user",
			"parts": []any{
				map[string]any{"functionResponse": map[string]any{"id": "call-42", "name": "do_thing", "response": map[string]any{"ok": true}}},
			},
		},
	}
	items := [][]byte{[]byte(`{"type":"function_call_part","call_id":"call-42","name":"do_thing","args":{"x":1},"thoughtSignature":"sig-abcdef0123456789"}`)}
	updated, changed := mergeAntigravityReasoningReplayItems(contents, items)
	if !changed {
		t.Fatalf("expected change")
	}
	if len(updated) != 2 {
		t.Fatalf("expected one new model content inserted, got %d entries", len(updated))
	}
	model := updated[0].(map[string]any)
	if role := model["role"]; role != "model" {
		t.Fatalf("expected inserted entry role=model, got %v", role)
	}
	part := model["parts"].([]any)[0].(map[string]any)
	if part["thoughtSignature"] != "sig-abcdef0123456789" {
		t.Fatalf("expected signature on inserted part, got %v", part)
	}
	fc, ok := part["functionCall"].(map[string]any)
	if !ok || fc["name"] != "do_thing" || fc["id"] != "call-42" {
		t.Fatalf("unexpected functionCall, got %v", part)
	}
}

func TestAntigravityMergeFillsMissingSignatureOnExistingFunctionCall(t *testing.T) {
	contents := []any{
		map[string]any{
			"role": "model",
			"parts": []any{
				map[string]any{"functionCall": map[string]any{"id": "call-42", "name": "do_thing", "args": map[string]any{"x": 1}}},
			},
		},
	}
	items := [][]byte{[]byte(`{"type":"function_call_part","call_id":"call-42","name":"do_thing","args":{"x":1},"thoughtSignature":"sig-abcdef0123456789"}`)}
	updated, changed := mergeAntigravityReasoningReplayItems(contents, items)
	if !changed {
		t.Fatalf("expected change")
	}
	part := updated[0].(map[string]any)["parts"].([]any)[0].(map[string]any)
	if part["thoughtSignature"] != "sig-abcdef0123456789" {
		t.Fatalf("expected signature added, got %v", part)
	}
}

func TestAntigravityCaptureReplayFromResponse(t *testing.T) {
	cache := NewAntigravityReasoningReplayCache(0, 0, 0, nil)
	body := []byte(`{"response":{"candidates":[{"content":{"role":"model","parts":[
		{"text":"hi","thoughtSignature":"sig-abcdef0123456789"},
		{"functionCall":{"id":"call-42","name":"do_thing","args":{"x":1}},"thoughtSignature":"sig-fedcba9876543210"}
	]}}]},"traceId":"tr"}`)
	scope := antigravityReasoningReplayScope{modelName: "gemini-3-pro", sessionKey: "session:abc"}
	captureAntigravityReasoningReplayFromResponse(context.Background(), cache, scope, body)
	items, ok := cache.GetItems("gemini-3-pro", "session:abc")
	if !ok || len(items) != 2 {
		t.Fatalf("expected 2 cached items, got ok=%v len=%d", ok, len(items))
	}
	all := string(bytes.Join(items, []byte("|")))
	if !strings.Contains(all, `"type":"thought_signature"`) {
		t.Fatalf("expected thought_signature item, got %s", all)
	}
	if !strings.Contains(all, `"type":"function_call_part"`) {
		t.Fatalf("expected function_call_part item, got %s", all)
	}
	if !strings.Contains(all, `"call_id":"call-42"`) {
		t.Fatalf("expected call_id preserved, got %s", all)
	}
}

func TestAntigravityCaptureReplayFromSSEFrames(t *testing.T) {
	cache := NewAntigravityReasoningReplayCache(0, 0, 0, nil)
	body := []byte("data: " + `{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"hi","thoughtSignature":"sig-abcdef0123456789"}]}}]}}` + "\n\n" +
		"data: " + `{"response":{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"call-42","name":"do_thing","args":{"x":1}}}]}}]}}` + "\n\n" +
		"data: [DONE]\n\n")
	frames, err := parseSSEFrames(body)
	if err != nil {
		t.Fatalf("parseSSEFrames: %v", err)
	}
	scope := antigravityReasoningReplayScope{modelName: "gemini-3-pro", sessionKey: "session:abc"}
	captureAntigravityReasoningReplayFromSSEFrames(context.Background(), cache, scope, frames)
	items, ok := cache.GetItems("gemini-3-pro", "session:abc")
	if !ok || len(items) != 2 {
		t.Fatalf("expected 2 cached items, got ok=%v len=%d items=%v", ok, len(items), items)
	}
}

func TestAntigravityClearOnSignatureFailure(t *testing.T) {
	cache := NewAntigravityReasoningReplayCache(0, 0, 0, nil)
	scope := antigravityReasoningReplayScope{modelName: "gemini-3-pro", sessionKey: "session:abc"}
	cache.PutItems(scope.modelName, scope.sessionKey, [][]byte{[]byte(`{"type":"thought_signature","thoughtSignature":"sig-abcdef0123456789"}`)})

	// Non-400 keeps the entry.
	clearAntigravityReasoningReplayOnSignatureFailure(cache, scope, http.StatusInternalServerError, []byte(`{"error":"thoughtSignature invalid"}`))
	if cache.Len() != 1 {
		t.Fatalf("expected 500 to leave entry, got len %d", cache.Len())
	}

	// 400 without signature-related body keeps the entry.
	clearAntigravityReasoningReplayOnSignatureFailure(cache, scope, http.StatusBadRequest, []byte(`{"error":"invalid argument"}`))
	if cache.Len() != 1 {
		t.Fatalf("expected unrelated 400 to leave entry, got len %d", cache.Len())
	}

	// 400 with body referencing thoughtSignature drops it.
	clearAntigravityReasoningReplayOnSignatureFailure(cache, scope, http.StatusBadRequest, []byte(`{"error":{"message":"invalid thoughtSignature in part 0"}}`))
	if cache.Len() != 0 {
		t.Fatalf("expected signature 400 to clear cache, got len %d", cache.Len())
	}
}

func TestAntigravityApplyReplayPayloadRoundTrip(t *testing.T) {
	cache := NewAntigravityReasoningReplayCache(0, 0, 0, nil)
	scope := antigravityReasoningReplayScope{modelName: "gemini-3-pro", sessionKey: "session:abc"}
	cache.PutItems(scope.modelName, scope.sessionKey, [][]byte{[]byte(`{"type":"thought_signature","thoughtSignature":"sig-abcdef0123456789","contentIndex":0,"partIndex":1}`)})

	payload := []byte(`{"request":{"contents":[{"role":"model","parts":[{"text":"a"},{"text":"b"}]}]}}`)
	updated := applyAntigravityReasoningReplayPayload(context.Background(), cache, scope, payload)
	if bytes.Equal(updated, payload) {
		t.Fatalf("expected payload to change")
	}
	var got map[string]any
	if err := json.Unmarshal(updated, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	part := got["request"].(map[string]any)["contents"].([]any)[0].(map[string]any)["parts"].([]any)[1].(map[string]any)
	if part["thoughtSignature"] != "sig-abcdef0123456789" {
		t.Fatalf("expected signature spliced, got %v", part)
	}
}

func TestAntigravityApplyReplayPayloadNoScopeNoOp(t *testing.T) {
	cache := NewAntigravityReasoningReplayCache(0, 0, 0, nil)
	payload := []byte(`{"request":{"contents":[]}}`)
	got := applyAntigravityReasoningReplayPayload(context.Background(), cache, antigravityReasoningReplayScope{}, payload)
	if !bytes.Equal(got, payload) {
		t.Fatalf("expected invalid scope to no-op")
	}
}

func TestAntigravityScopeFromRequestUsesAdapterSessionFallback(t *testing.T) {
	// When the payload has no sessionId, fall back to antigravitySessionID
	// (which the adapter would set on the outgoing request anyway).
	req := contract.ConversationRequest{
		Mapping: modelcontract.ModelProviderMapping{UpstreamModelName: "gemini-3-pro"},
		Messages: []contract.ConversationMessage{
			{Role: "user", Parts: []contract.ContentPart{{Kind: contract.ContentPartText, Text: "stable text"}}},
		},
	}
	payload := []byte(`{"request":{"contents":[]}}`)
	scope := antigravityReasoningReplayScopeFromRequest(req, payload)
	if !scope.valid() {
		t.Fatalf("expected scope to derive from message text")
	}
	if !strings.HasPrefix(scope.sessionKey, "session:") {
		t.Fatalf("expected session:* key, got %q", scope.sessionKey)
	}
}

// Smoke test guarding that signature-failure clear-on-400 still survives a
// real Vertex-style upstream error body shape (matches what we'd actually
// see in the wild).
func TestAntigravityClearOnSignatureFailureRealBody(t *testing.T) {
	cache := NewAntigravityReasoningReplayCache(0, 0, 0, nil)
	scope := antigravityReasoningReplayScope{modelName: "gemini-3-pro", sessionKey: "session:abc"}
	cache.PutItems(scope.modelName, scope.sessionKey, [][]byte{[]byte(`{"type":"thought_signature","thoughtSignature":"sig-abcdef0123456789"}`)})
	body := []byte(`{"error":{"code":400,"message":"Invalid argument: thought_signature is malformed at request.contents[1].parts[0]","status":"INVALID_ARGUMENT"}}`)
	clearAntigravityReasoningReplayOnSignatureFailure(cache, scope, http.StatusBadRequest, body)
	if cache.Len() != 0 {
		t.Fatalf("expected real upstream 400 body to clear cache, got len %d", cache.Len())
	}
}

// Ensure the adapter test imports above (contract / accountcontract) stay
// linked even if no other reference appears in the file — used by the
// scope-from-message-text test to construct a realistic contract.
