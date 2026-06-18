package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

// codex_identity_confuse metadata flag is opt-OUT for the identity_confuse
// module: default ON, mirroring CLIProxyAPI's default routing-strategy
// behaviour. Operators with a single-tenant Codex account (or integration
// tests that pin specific identifiers) explicitly set the flag to a falsy
// value to disable. Every other case — absent metadata, truthy values,
// even garbage strings — leaves identity confusion enabled.
func TestCodexIdentityConfuseConfigForAccountGatedOnMetadataFlag(t *testing.T) {
	cases := []struct {
		name     string
		metadata map[string]any
		want     bool
	}{
		{"absent_flag_defaults_on", nil, true},
		{"explicit_true_on", map[string]any{"codex_identity_confuse": true}, true},
		{"string_true_on", map[string]any{"codex_identity_confuse": "true"}, true},
		{"string_TRUE_on", map[string]any{"codex_identity_confuse": "TRUE"}, true},
		{"string_1_on", map[string]any{"codex_identity_confuse": "1"}, true},
		{"string_yes_on", map[string]any{"codex_identity_confuse": "yes"}, true},
		{"string_on_on", map[string]any{"codex_identity_confuse": "on"}, true},
		{"non_string_non_bool_on", map[string]any{"codex_identity_confuse": 42}, true},
		{"empty_string_on", map[string]any{"codex_identity_confuse": ""}, true},
		{"string_garbage_on", map[string]any{"codex_identity_confuse": "maybe"}, true},
		{"explicit_false_off", map[string]any{"codex_identity_confuse": false}, false},
		{"string_false_off", map[string]any{"codex_identity_confuse": "false"}, false},
		{"string_FALSE_off", map[string]any{"codex_identity_confuse": "FALSE"}, false},
		{"string_0_off", map[string]any{"codex_identity_confuse": "0"}, false},
		{"string_no_off", map[string]any{"codex_identity_confuse": "no"}, false},
		{"string_off_off", map[string]any{"codex_identity_confuse": "off"}, false},
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

// When the operator explicitly opts out (metadata
// codex_identity_confuse=false) the wiring is a pure no-op — the same
// byte slice comes back and state is zero. This is the escape hatch for
// single-tenant accounts and for the integration tests in service_test.go
// that pin specific identifiers on the upstream wire.
func TestCodexApplyOutboundWiringIsNoopWhenDisabled(t *testing.T) {
	body := []byte(`{"model":"codex-upstream","prompt_cache_key":"keep-me"}`)
	account := accountcontract.ProviderAccount{
		ID:       42,
		Metadata: map[string]any{"codex_identity_confuse": false},
	}
	newBody, state := codexApplyOutboundWiring(account, nil, body)
	if state.Enabled {
		t.Errorf("state.Enabled = true; expected false for explicitly opted-out account")
	}
	if string(newBody) != string(body) {
		t.Errorf("body was modified despite explicit opt-out:\n  before: %s\n   after: %s", body, newBody)
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

	cache := resetCodexReasoningReplayCacheForTest()
	sessionKey := codexPromptCacheReplaySessionKey(original)
	codexCaptureInboundWiring(state, codexReasoningReplayScope{modelName: model, sessionKey: sessionKey}, responseBody)

	got, ok := cache.GetItems(model, sessionKey)
	if !ok {
		t.Fatalf("expected cached items under (%q, %q); cache miss", model, sessionKey)
	}
	if len(got) == 0 {
		t.Fatalf("cached item list is empty")
	}
}

func TestCodexCaptureInboundWiringPopulatesReplayCacheFromSSE(t *testing.T) {
	model := "gpt-5.4"
	sessionKey := "prompt-cache:stream-session"
	encryptedContent := validCodexReasoningReplayEncryptedContentForTest(11)
	body := []byte(
		"data: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":{\"type\":\"reasoning\",\"summary\":[],\"content\":null,\"encrypted_content\":\"" + encryptedContent + "\"}}\n\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"output\":[]}}\n\n",
	)

	cache := resetCodexReasoningReplayCacheForTest()
	codexCacheOutputItems(model, sessionKey, body)

	got, ok := cache.GetItems(model, sessionKey)
	if !ok || len(got) != 1 {
		t.Fatalf("expected one cached SSE item, got ok=%v len=%d", ok, len(got))
	}
}

func TestCodexCaptureInboundWiringUsesReplayScopeSessionKey(t *testing.T) {
	model := "gpt-5.4"
	sessionKey := "claude:session-json-1"
	encryptedContent := validCodexReasoningReplayEncryptedContentForTest(14)
	responseBody := []byte(`{"output":[{"type":"reasoning","summary":[],"content":null,"encrypted_content":"` + encryptedContent + `"}]}`)

	cache := resetCodexReasoningReplayCacheForTest()
	codexCaptureInboundWiring(CodexIdentityConfuseState{}, codexReasoningReplayScope{modelName: model, sessionKey: sessionKey}, responseBody)

	got, ok := cache.GetItems(model, sessionKey)
	if !ok || len(got) != 1 {
		t.Fatalf("expected scoped replay cache item, got ok=%v len=%d", ok, len(got))
	}
}

func TestCodexApplyReasoningReplayInjectsAnthropicSessionReasoning(t *testing.T) {
	encryptedContent := validCodexReasoningReplayEncryptedContentForTest(12)
	cache := resetCodexReasoningReplayCacheForTest()
	cache.PutItem("gpt-5.4", "claude:session-json-1", []byte(`{"type":"reasoning","summary":[],"content":null,"encrypted_content":"`+encryptedContent+`"}`))

	payload := map[string]any{
		"model": "gpt-5.4",
		"input": []any{
			map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{map[string]any{
					"type": "input_text",
					"text": "next",
				}},
			},
		},
	}
	req := contract.ConversationRequest{
		SourceProtocol: "anthropic-compatible",
		RawBody:        []byte(`{"metadata":{"user_id":"{\"device_id\":\"dev\",\"account_uuid\":\"acct\",\"session_id\":\"session-json-1\"}"},"messages":[{"role":"user","content":"next"}]}`),
	}

	codexApplyReasoningReplay(req, payload)

	input, ok := payload["input"].([]any)
	if !ok || len(input) != 2 {
		t.Fatalf("expected replay item plus user message, got %#v", payload["input"])
	}
	reasoning, ok := input[0].(map[string]any)
	if !ok || reasoning["type"] != "reasoning" || reasoning["encrypted_content"] != encryptedContent {
		t.Fatalf("unexpected injected reasoning item: %#v", input[0])
	}
}

func TestCodexApplyReasoningReplaySkipsNativeOpenAIResponses(t *testing.T) {
	encryptedContent := validCodexReasoningReplayEncryptedContentForTest(13)
	cache := resetCodexReasoningReplayCacheForTest()
	cache.PutItem("gpt-5.4", "prompt-cache:native-session", []byte(`{"type":"reasoning","summary":[],"content":null,"encrypted_content":"`+encryptedContent+`"}`))

	payload := map[string]any{
		"model":            "gpt-5.4",
		"prompt_cache_key": "native-session",
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": "native"},
		},
	}
	req := contract.ConversationRequest{SourceProtocol: "openai-compatible"}

	codexApplyReasoningReplay(req, payload)

	input, ok := payload["input"].([]any)
	if !ok || len(input) != 1 {
		t.Fatalf("native response request should not be replay-injected, got %#v", payload["input"])
	}
}

func TestCodexApplyReasoningReplayMatchesSanitizedClaudeToolID(t *testing.T) {
	cache := resetCodexReasoningReplayCacheForTest()
	cache.PutItem("gpt-5.4", "claude:session-json-1", []byte(`{"type":"function_call","call_id":"call.id/with:chars","name":"lookup","arguments":"{\"q\":\"weather\"}"}`))

	payload := map[string]any{
		"model": "gpt-5.4",
		"input": []any{
			map[string]any{
				"type":    "function_call_output",
				"call_id": "call_id_with_chars",
				"output":  "sunny",
			},
			map[string]any{
				"type":    "message",
				"role":    "user",
				"content": "continue",
			},
		},
	}
	req := contract.ConversationRequest{
		SourceProtocol: "anthropic-compatible",
		RawBody:        []byte(`{"metadata":{"user_id":"{\"session_id\":\"session-json-1\"}"},"messages":[]}`),
	}

	codexApplyReasoningReplay(req, payload)

	input, ok := payload["input"].([]any)
	if !ok || len(input) != 3 {
		t.Fatalf("expected replay function_call injection, got %#v", payload["input"])
	}
	call, ok := input[0].(map[string]any)
	if !ok || call["type"] != "function_call" || call["call_id"] != "call_id_with_chars" {
		t.Fatalf("expected sanitized replay call before output, got %#v", input[0])
	}
}

func TestCodexClearReasoningReplayOnInvalidSignature(t *testing.T) {
	cache := resetCodexReasoningReplayCacheForTest()
	scope := codexReasoningReplayScope{modelName: "gpt-5.4", sessionKey: "claude:session-json-1"}
	cache.PutItem(scope.modelName, scope.sessionKey, validCodexReasoningReplayItemForTest(15))

	codexClearReasoningReplayOnInvalidSignature(
		scope,
		http.StatusBadRequest,
		[]byte(`{"error":{"type":"invalid_request_error","message":"invalid signature in thinking block"}}`),
	)

	if _, ok := cache.GetItems(scope.modelName, scope.sessionKey); ok {
		t.Fatal("invalid thinking signature should clear cached reasoning replay")
	}
}

func TestCodexCaptureInboundWiringClearsReplayOnInvalidSignatureSSE(t *testing.T) {
	cache := resetCodexReasoningReplayCacheForTest()
	scope := codexReasoningReplayScope{modelName: "gpt-5.4", sessionKey: "claude:session-json-1"}
	cache.PutItem(scope.modelName, scope.sessionKey, validCodexReasoningReplayItemForTest(18))

	body := []byte("event: response.failed\n" +
		"data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"type\":\"invalid_request_error\",\"code\":\"invalid_encrypted_content\",\"message\":\"The encrypted content could not be verified.\"}}}\n\n")

	codexCaptureInboundWiring(CodexIdentityConfuseState{}, scope, body)

	if _, ok := cache.GetItems(scope.modelName, scope.sessionKey); ok {
		t.Fatal("invalid encrypted_content SSE terminal error should clear cached reasoning replay")
	}
}

func TestCodexStreamConversationAppliesAndCapturesReasoningReplay(t *testing.T) {
	cache := resetCodexReasoningReplayCacheForTest()
	cache.PutItem("gpt-5.4", "claude:stream-session", validCodexReasoningReplayItemForTest(16))
	nextEncrypted := validCodexReasoningReplayEncryptedContentForTest(17)
	runtime := &capturingStreamRuntime{
		streamResponse: reverseproxycontract.StreamResponse{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(bytes.NewReader([]byte(
				"data: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":{\"type\":\"reasoning\",\"summary\":[],\"content\":null,\"encrypted_content\":\"" + nextEncrypted + "\"}}\n\n" +
					"data: {\"type\":\"response.output_text.delta\",\"delta\":\"answer\"}\n\n" +
					"data: {\"type\":\"response.output_text.done\",\"text\":\"answer\"}\n\n" +
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_stream\",\"output\":[],\"usage\":{\"input_tokens\":4,\"output_tokens\":2}}}\n\n" +
					"data: [DONE]\n\n",
			))),
		},
	}
	svc, err := NewWithReverseProxy(nil, runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.StreamConversation(context.Background(), codexStreamReplayRequest())
	if err != nil {
		t.Fatalf("stream codex conversation: %v", err)
	}
	raw, err := io.ReadAll(resp.StreamBody)
	if err != nil {
		t.Fatalf("read stream body: %v", err)
	}
	if err := resp.StreamBody.Close(); err != nil {
		t.Fatalf("close stream body: %v", err)
	}
	if _, err := resp.StreamParse(raw, resp.StatusCode); err != nil {
		t.Fatalf("parse stream body: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(runtime.request.Body, &payload); err != nil {
		t.Fatalf("decode outbound payload: %v", err)
	}
	input, ok := payload["input"].([]any)
	if !ok || len(input) < 2 {
		t.Fatalf("expected replay-injected input, got %#v", payload["input"])
	}
	reasoning, ok := input[0].(map[string]any)
	if !ok || reasoning["type"] != "reasoning" {
		t.Fatalf("expected cached reasoning first, got %#v", input[0])
	}
	got, ok := cache.GetItems("gpt-5.4", "claude:stream-session")
	if !ok || len(got) != 1 || !bytes.Contains(got[0], []byte(nextEncrypted)) {
		t.Fatalf("expected stream parse to refresh replay cache, got ok=%v items=%q", ok, got)
	}
}

func TestCodexStreamConversationClearsReplayOnInvalidSignatureSSE(t *testing.T) {
	cache := resetCodexReasoningReplayCacheForTest()
	cache.PutItem("gpt-5.4", "claude:stream-session", validCodexReasoningReplayItemForTest(19))
	runtime := &capturingStreamRuntime{
		streamResponse: reverseproxycontract.StreamResponse{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(bytes.NewReader([]byte(
				"data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"type\":\"invalid_request_error\",\"code\":\"invalid_encrypted_content\",\"message\":\"The encrypted content could not be verified.\"}}}\n\n" +
					"data: [DONE]\n\n",
			))),
		},
	}
	svc, err := NewWithReverseProxy(nil, runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.StreamConversation(context.Background(), codexStreamReplayRequest())
	if err != nil {
		t.Fatalf("stream codex conversation: %v", err)
	}
	raw, err := io.ReadAll(resp.StreamBody)
	if err != nil {
		t.Fatalf("read stream body: %v", err)
	}
	if err := resp.StreamBody.Close(); err != nil {
		t.Fatalf("close stream body: %v", err)
	}
	if _, err := resp.StreamParse(raw, resp.StatusCode); err != nil {
		t.Fatalf("parse stream body: %v", err)
	}
	if _, ok := cache.GetItems("gpt-5.4", "claude:stream-session"); ok {
		t.Fatal("stream invalid_encrypted_content terminal error should clear replay cache")
	}
}

func TestCodexExposeStreamBodyRestoresOriginalPromptCacheKey(t *testing.T) {
	state := CodexIdentityConfuseState{
		Enabled:                true,
		OriginalPromptCacheKey: "client-session",
		PromptCacheKey:         "confused-session",
	}
	body := io.NopCloser(bytes.NewReader([]byte(
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"prompt_cache_key\":\"confused-session\",\"output\":[]}}\n\n",
	)))
	exposed, err := io.ReadAll(codexExposeStreamBody(body, state, nil))
	if err != nil {
		t.Fatalf("read exposed stream: %v", err)
	}
	if !bytes.Contains(exposed, []byte("client-session")) || bytes.Contains(exposed, []byte("confused-session")) {
		t.Fatalf("stream body was not exposed correctly: %s", exposed)
	}
}

func codexStreamReplayRequest() contract.ConversationRequest {
	return contract.ConversationRequest{
		RequestID:      "req_codex_stream_replay",
		SourceProtocol: "anthropic-compatible",
		SourceEndpoint: "/v1/messages",
		TargetProtocol: "openai-compatible",
		Model:          "gpt-5.4",
		Stream:         true,
		InputParts:     []contract.ContentPart{{Kind: contract.ContentPartText, Text: "continue"}},
		RawBody:        []byte(`{"metadata":{"user_id":"{\"session_id\":\"stream-session\"}"},"messages":[{"role":"user","content":"continue"}]}`),
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:           9,
			RuntimeClass: accountcontract.RuntimeClassCliClientToken,
			Metadata: map[string]any{
				"base_url":               "https://codex.example.test/backend-api/codex",
				"codex_identity_confuse": false,
			},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5.4"},
		Credential: map[string]any{"cli_client_token": "codex-token"},
	}
}

type capturingStreamRuntime struct {
	request        reverseproxycontract.Request
	streamResponse reverseproxycontract.StreamResponse
	err            error
}

func (r *capturingStreamRuntime) Do(context.Context, reverseproxycontract.Request) (reverseproxycontract.Response, error) {
	return reverseproxycontract.Response{}, nil
}

func (r *capturingStreamRuntime) DoStream(_ context.Context, req reverseproxycontract.Request) (reverseproxycontract.StreamResponse, error) {
	r.request = req
	if r.err != nil {
		return reverseproxycontract.StreamResponse{}, r.err
	}
	return r.streamResponse, nil
}

func (r *capturingStreamRuntime) ManagedEgressClient(reverseproxycontract.AccountRuntime) (*http.Client, bool, error) {
	return nil, false, nil
}

func resetCodexReasoningReplayCacheForTest() *CodexReasoningReplayCache {
	codexReasoningReplayCacheOnce = sync.Once{}
	return codexReasoningReplayCache()
}
