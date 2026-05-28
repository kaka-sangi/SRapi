package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

func TestSameProtocolRawConversationResponseAllowsClaudeCodeMessages(t *testing.T) {
	req := gatewaycontract.CanonicalRequest{
		SourceProtocol: gatewaycontract.ProtocolAnthropicCompatible,
		SourceEndpoint: "/v1/messages",
	}

	if !sameProtocolRawConversationResponse(req, "anthropic-compatible", "reverse-proxy-claude-code-cli", "", nil, nil, nil, []byte(`{"id":"msg_1"}`)) {
		t.Fatal("expected Claude Code same-protocol messages response to be eligible for raw passthrough")
	}
}

func TestGatewayEmptyCompletionErrorClassIsRetryable(t *testing.T) {
	if !gatewayShouldFailover("empty_completion", http.StatusBadGateway, 0, 2) {
		t.Fatal("expected empty completion to be eligible for failover")
	}
	if got := providerGatewayMessage("empty_completion"); got != "provider returned empty completion" {
		t.Fatalf("unexpected empty completion gateway message %q", got)
	}
	if got := geminiStatusForGatewayErrorClass("empty_completion", http.StatusBadGateway); got != "UNAVAILABLE" {
		t.Fatalf("unexpected empty completion Gemini status %q", got)
	}
}

func TestGatewayProviderPublicMessageRequiresMetadataOptIn(t *testing.T) {
	err := provideradaptercontract.ProviderError{
		Class:      "invalid_request",
		StatusCode: http.StatusBadRequest,
		Message:    `{"error":{"message":"schema mismatch: field temperature is unsupported"}}`,
	}
	candidate := schedulercontract.Candidate{
		Account: accountcontract.ProviderAccount{Metadata: map[string]any{
			"expose_provider_error_messages":          true,
			"provider_error_passthrough_status_codes": []any{http.StatusBadRequest},
			"provider_error_passthrough_classes":      []any{"invalid_request"},
			"provider_error_passthrough_keywords":     []any{"temperature"},
		}},
	}

	got := gatewayProviderPublicMessage(err, "invalid_request", http.StatusBadRequest, &candidate)
	if got != "schema mismatch: field temperature is unsupported" {
		t.Fatalf("expected sanitized provider message, got %q", got)
	}
	if got := gatewayProviderPublicMessage(err, "invalid_request", http.StatusBadRequest, nil); got != "provider rejected request" {
		t.Fatalf("expected default message without candidate policy, got %q", got)
	}
	candidate.Account.Metadata["provider_error_passthrough_keywords"] = []any{"quota"}
	if got := gatewayProviderPublicMessage(err, "invalid_request", http.StatusBadRequest, &candidate); got != "provider rejected request" {
		t.Fatalf("expected default message when keyword filter misses, got %q", got)
	}
}

func TestGatewayProviderPublicMessageRejectsSensitiveText(t *testing.T) {
	err := provideradaptercontract.ProviderError{
		Class:      "auth_failed",
		StatusCode: http.StatusUnauthorized,
		Message:    `{"error":{"message":"Authorization Bearer sk-live-secret was rejected"}}`,
	}
	candidate := schedulercontract.Candidate{
		Account: accountcontract.ProviderAccount{Metadata: map[string]any{
			"expose_provider_error_messages": true,
		}},
	}

	if got := gatewayProviderPublicMessage(err, "auth_failed", http.StatusUnauthorized, &candidate); got != "provider authentication failed" {
		t.Fatalf("expected sensitive provider message to stay generic, got %q", got)
	}
}

func TestGatewayProviderPublicMessageDoesNotExposeUnknownJSONBodies(t *testing.T) {
	err := provideradaptercontract.ProviderError{
		Class:      "provider_5xx",
		StatusCode: http.StatusInternalServerError,
		Message:    `{"trace_id":"upstream-1","diagnostic":{"reason":"internal failure"}}`,
	}
	candidate := schedulercontract.Candidate{
		Account: accountcontract.ProviderAccount{Metadata: map[string]any{
			"expose_provider_error_messages": true,
		}},
	}

	if got := gatewayProviderPublicMessage(err, "provider_5xx", http.StatusInternalServerError, &candidate); got != "provider service error" {
		t.Fatalf("expected unknown JSON body to stay generic, got %q", got)
	}
}

func TestGatewaySameCandidateRetryPolicyForPoolMode(t *testing.T) {
	policy := gatewaySameCandidateRetryPolicyFor(schedulercontract.Candidate{
		Account: accountcontract.ProviderAccount{
			Metadata: map[string]any{
				"pool_mode":                     true,
				"pool_mode_retry_count":         "4",
				"pool_mode_retry_base_delay_ms": float64(0),
				"pool_mode_retry_max_delay_ms":  float64(1),
			},
		},
	})

	if !policy.Enabled || policy.MaxRetries != 4 || !policy.RetryAuthFailures {
		t.Fatalf("unexpected pool mode retry policy: %+v", policy)
	}
	if !gatewayShouldRetrySameCandidate(policy, "auth_failed", http.StatusUnauthorized, 0) {
		t.Fatal("expected pool mode to retry transient auth failures")
	}
	if gatewayShouldRetrySameCandidate(policy, "auth_failed", http.StatusUnauthorized, 4) {
		t.Fatal("expected retry budget to cap pool mode retries")
	}
}

func TestGatewaySameCandidateRetryPolicyExplicitOptIn(t *testing.T) {
	policy := gatewaySameCandidateRetryPolicyFor(schedulercontract.Candidate{
		Account: accountcontract.ProviderAccount{
			Metadata: map[string]any{
				"same_candidate_retry_enabled":       true,
				"same_candidate_retry_count":         float64(2),
				"same_candidate_retry_base_delay_ms": float64(5),
				"same_candidate_retry_max_delay_ms":  float64(8),
			},
		},
		Provider: providercontract.Provider{Protocol: "openai-compatible"},
	})

	if !policy.Enabled || policy.MaxRetries != 2 || policy.BaseDelay != 5*time.Millisecond || policy.MaxDelay != 8*time.Millisecond {
		t.Fatalf("unexpected explicit retry policy: %+v", policy)
	}
	if !gatewayShouldRetrySameCandidate(policy, "provider_5xx", http.StatusServiceUnavailable, 1) {
		t.Fatal("expected transient provider error to retry")
	}
	if gatewayShouldRetrySameCandidate(policy, "invalid_request", http.StatusBadRequest, 0) {
		t.Fatal("did not expect invalid requests to retry")
	}
}

func TestSameProtocolRawConversationStreamAllowsEndpointMatchedSSE(t *testing.T) {
	raw := []byte(": keepalive\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n")
	req := gatewaycontract.CanonicalRequest{
		SourceProtocol: gatewaycontract.ProtocolOpenAICompatible,
		SourceEndpoint: "/v1/chat/completions",
		Stream:         true,
	}

	if !sameProtocolRawConversationStream(req, "openai-compatible", "openai-compatible", "", nil, nil, nil, raw) {
		t.Fatal("expected OpenAI same-protocol chat stream to be eligible for raw SSE replay")
	}
}

func TestSameProtocolRawConversationStreamAllowsCodexResponsesSSE(t *testing.T) {
	raw := []byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"raw\"}\n\n")
	req := gatewaycontract.CanonicalRequest{
		SourceProtocol: gatewaycontract.ProtocolOpenAICompatible,
		SourceEndpoint: "/v1/responses",
		Stream:         true,
	}

	if !sameProtocolRawConversationStream(req, "openai-compatible", "reverse-proxy-codex-cli", "", nil, nil, nil, raw) {
		t.Fatal("expected Codex Responses stream to be eligible for raw SSE replay")
	}
}

func TestSameProtocolRawConversationStreamAllowsNativeOpenAIResponsesSSE(t *testing.T) {
	raw := []byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"raw\"}\n\n")
	req := gatewaycontract.CanonicalRequest{
		SourceProtocol: gatewaycontract.ProtocolOpenAICompatible,
		SourceEndpoint: "/v1/responses",
		Stream:         true,
	}

	if !sameProtocolRawConversationStream(req, "openai-compatible", "native-openai", "", nil, nil, nil, raw) {
		t.Fatal("expected native OpenAI Responses stream to be eligible for raw SSE replay")
	}
}

func TestSameProtocolRawConversationStreamAllowsOptedInOpenAIResponsesSSE(t *testing.T) {
	raw := []byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"raw\"}\n\n")
	req := gatewaycontract.CanonicalRequest{
		SourceProtocol: gatewaycontract.ProtocolOpenAICompatible,
		SourceEndpoint: "/v1/responses",
		Stream:         true,
	}

	if !sameProtocolRawConversationStream(req, "openai-compatible", "reverse-proxy-openai-compatible", "", map[string]any{"native_responses": true}, nil, nil, raw) {
		t.Fatal("expected opted-in reverse proxy OpenAI Responses stream to be eligible for raw SSE replay")
	}
}

func TestSameProtocolRawConversationResponseAllowsCodexResponsesCompact(t *testing.T) {
	req := gatewaycontract.CanonicalRequest{
		SourceProtocol: gatewaycontract.ProtocolOpenAICompatible,
		SourceEndpoint: "/v1/responses/compact",
	}

	if !sameProtocolRawConversationResponse(req, "openai-compatible", "reverse-proxy-codex-cli", "", nil, nil, nil, []byte(`{"id":"cmp_1","object":"response.compaction"}`)) {
		t.Fatal("expected Codex Responses compact JSON to be eligible for raw passthrough")
	}
}

func TestSameProtocolRawConversationResponseAllowsNativeOpenAIResponses(t *testing.T) {
	req := gatewaycontract.CanonicalRequest{
		SourceProtocol: gatewaycontract.ProtocolOpenAICompatible,
		SourceEndpoint: "/v1/responses",
	}

	if !sameProtocolRawConversationResponse(req, "openai-compatible", "native-openai", "", nil, nil, nil, []byte(`{"id":"resp_1","object":"response"}`)) {
		t.Fatal("expected native OpenAI Responses JSON to be eligible for raw passthrough")
	}
}

func TestSameProtocolRawConversationResponseAllowsOptedInOpenAIResponses(t *testing.T) {
	req := gatewaycontract.CanonicalRequest{
		SourceProtocol: gatewaycontract.ProtocolOpenAICompatible,
		SourceEndpoint: "/v1/responses",
	}

	if !sameProtocolRawConversationResponse(req, "openai-compatible", "reverse-proxy-openai-compatible", "", nil, nil, map[string]any{"responses_passthrough": "true"}, []byte(`{"id":"resp_1","object":"response"}`)) {
		t.Fatal("expected opted-in OpenAI Responses JSON to be eligible for raw passthrough")
	}
}

func TestSameProtocolRawConversationResponseRejectsUnsafeCases(t *testing.T) {
	tests := []struct {
		name           string
		req            gatewaycontract.CanonicalRequest
		targetProtocol string
		adapterType    string
		raw            []byte
	}{
		{
			name: "empty raw",
			req: gatewaycontract.CanonicalRequest{
				SourceProtocol: gatewaycontract.ProtocolAnthropicCompatible,
				SourceEndpoint: "/v1/messages",
			},
			targetProtocol: "anthropic-compatible",
			adapterType:    "reverse-proxy-claude-code-cli",
		},
		{
			name: "streaming raw json",
			req: gatewaycontract.CanonicalRequest{
				SourceProtocol: gatewaycontract.ProtocolAnthropicCompatible,
				SourceEndpoint: "/v1/messages",
				Stream:         true,
			},
			targetProtocol: "anthropic-compatible",
			adapterType:    "reverse-proxy-claude-code-cli",
			raw:            []byte(`{"id":"msg_1"}`),
		},
		{
			name: "cross protocol",
			req: gatewaycontract.CanonicalRequest{
				SourceProtocol: gatewaycontract.ProtocolAnthropicCompatible,
				SourceEndpoint: "/v1/messages",
			},
			targetProtocol: "openai-compatible",
			adapterType:    "openai-compatible",
			raw:            []byte(`{"id":"msg_1"}`),
		},
		{
			name: "openai responses to chat adapter",
			req: gatewaycontract.CanonicalRequest{
				SourceProtocol: gatewaycontract.ProtocolOpenAICompatible,
				SourceEndpoint: "/v1/responses",
			},
			targetProtocol: "openai-compatible",
			adapterType:    "openai-compatible",
			raw:            []byte(`{"id":"resp_1"}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if sameProtocolRawConversationResponse(tt.req, tt.targetProtocol, tt.adapterType, "", nil, nil, nil, tt.raw) {
				t.Fatal("expected raw passthrough to be rejected")
			}
		})
	}
}

func TestSameProtocolRawConversationStreamRejectsUnsafeCases(t *testing.T) {
	tests := []struct {
		name           string
		req            gatewaycontract.CanonicalRequest
		targetProtocol string
		adapterType    string
		raw            []byte
	}{
		{
			name: "non stream",
			req: gatewaycontract.CanonicalRequest{
				SourceProtocol: gatewaycontract.ProtocolOpenAICompatible,
				SourceEndpoint: "/v1/chat/completions",
			},
			targetProtocol: "openai-compatible",
			adapterType:    "openai-compatible",
			raw:            []byte("data: {}\n\n"),
		},
		{
			name: "cross protocol",
			req: gatewaycontract.CanonicalRequest{
				SourceProtocol: gatewaycontract.ProtocolAnthropicCompatible,
				SourceEndpoint: "/v1/messages",
				Stream:         true,
			},
			targetProtocol: "openai-compatible",
			adapterType:    "openai-compatible",
			raw:            []byte("data: {}\n\n"),
		},
		{
			name: "responses endpoint generic openai adapter",
			req: gatewaycontract.CanonicalRequest{
				SourceProtocol: gatewaycontract.ProtocolOpenAICompatible,
				SourceEndpoint: "/v1/responses",
				Stream:         true,
			},
			targetProtocol: "openai-compatible",
			adapterType:    "openai-compatible",
			raw:            []byte("data: {}\n\n"),
		},
		{
			name: "gemini non stream action",
			req: gatewaycontract.CanonicalRequest{
				SourceProtocol: gatewaycontract.ProtocolGeminiCompatible,
				SourceEndpoint: "/v1beta/models/gemini-pro:generateContent",
				Stream:         true,
			},
			targetProtocol: "gemini-compatible",
			adapterType:    "native-gemini",
			raw:            []byte("data: {}\n\n"),
		},
		{
			name: "not sse",
			req: gatewaycontract.CanonicalRequest{
				SourceProtocol: gatewaycontract.ProtocolOpenAICompatible,
				SourceEndpoint: "/v1/chat/completions",
				Stream:         true,
			},
			targetProtocol: "openai-compatible",
			adapterType:    "openai-compatible",
			raw:            []byte(`{"id":"chatcmpl_1"}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if sameProtocolRawConversationStream(tt.req, tt.targetProtocol, tt.adapterType, "", nil, nil, nil, tt.raw) {
				t.Fatal("expected raw SSE replay to be rejected")
			}
		})
	}
}

func TestGatewayChatCompletionsReplaysSameProtocolRawSSE(t *testing.T) {
	rawSSE := ": provider keepalive\n\n" +
		"data: {\"id\":\"chunk_1\",\"choices\":[{\"delta\":{\"content\":\"raw\"}}]}\n\n" +
		"data: {\"id\":\"chunk_2\",\"choices\":[{\"delta\":{\"content\":\" replay\"},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: {\"id\":\"usage\",\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n\n" +
		"data: [DONE]\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(rawSSE))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"raw-sse-provider","display_name":"Raw SSE Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"raw-sse-model","display_name":"Raw SSE Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"raw-sse-upstream","status":"active","capability_override":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"raw-sse-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"raw-sse-model","stream":true,"messages":[{"role":"user","content":"hello"}]}`)
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected text/event-stream content type, got %q", got)
	}
	if got := rec.Body.String(); got != rawSSE {
		t.Fatalf("expected raw SSE replay\nexpected:\n%s\nactual:\n%s", rawSSE, got)
	}
	if strings.Contains(rec.Body.String(), "raw replay") {
		t.Fatalf("expected raw chunk replay, got aggregated synthetic stream: %s", rec.Body.String())
	}
}

func TestProviderConversationRequestPreservesStructuredBlocks(t *testing.T) {
	raw := json.RawMessage(`{"type":"image_url","image_url":{"url":"https://example.invalid/image.png"}}`)
	req := gatewaycontract.CanonicalRequest{
		RequestID:        "req_structured",
		SourceProtocol:   gatewaycontract.ProtocolOpenAICompatible,
		SourceEndpoint:   "/v1/chat/completions",
		ResponseProtocol: gatewaycontract.ProtocolOpenAICompatible,
		Messages: []gatewaycontract.Message{{
			Role: "user",
			Content: []gatewaycontract.ContentBlock{{
				Type:           gatewaycontract.ContentBlockImage,
				Role:           "user",
				Text:           "[image]",
				MediaURL:       "https://example.invalid/image.png",
				MIMEType:       "image/png",
				Raw:            raw,
				OriginProtocol: string(gatewaycontract.ProtocolOpenAICompatible),
			}},
		}},
	}

	providerReq := providerConversationRequest(req, schedulercontract.Candidate{})

	if len(providerReq.Messages) != 1 || len(providerReq.Messages[0].Parts) != 1 {
		t.Fatalf("expected one structured provider part, got %+v", providerReq.Messages)
	}
	part := providerReq.Messages[0].Parts[0]
	if part.MediaURL != "https://example.invalid/image.png" || part.MIMEType != "image/png" || part.OriginProtocol != string(gatewaycontract.ProtocolOpenAICompatible) || string(part.Raw) != string(raw) {
		t.Fatalf("expected structured media and raw block to be preserved, got %+v", part)
	}
}

func TestProviderConversationRequestGroupsInputItemsByRole(t *testing.T) {
	req := gatewaycontract.CanonicalRequest{
		RequestID:      "req_input_item_roles",
		SourceProtocol: gatewaycontract.ProtocolOpenAICompatible,
		SourceEndpoint: "/v1/responses",
		InputItems: []gatewaycontract.ContentBlock{
			{Type: gatewaycontract.ContentBlockText, Role: "user", Text: "What is the weather?"},
			{
				Type:              gatewaycontract.ContentBlockToolCall,
				Role:              "assistant",
				Text:              "[function_call]",
				ToolCallID:        "call_1",
				ToolName:          "lookup_weather",
				ToolArgumentsJSON: `{"city":"Boston"}`,
			},
			{
				Type:            gatewaycontract.ContentBlockToolResult,
				Role:            "tool",
				Text:            `{"forecast":"sunny"}`,
				ToolCallID:      "call_1",
				ToolResultForID: "call_1",
			},
		},
	}

	providerReq := providerConversationRequest(req, schedulercontract.Candidate{})

	if len(providerReq.Messages) != 3 {
		t.Fatalf("expected user, assistant tool call, and tool result messages, got %+v", providerReq.Messages)
	}
	if providerReq.Messages[0].Role != "user" ||
		len(providerReq.Messages[0].Parts) != 1 ||
		providerReq.Messages[0].Parts[0].Kind != "text" ||
		providerReq.Messages[0].Parts[0].Text != "What is the weather?" {
		t.Fatalf("unexpected user message: %+v", providerReq.Messages[0])
	}
	if providerReq.Messages[1].Role != "assistant" ||
		len(providerReq.Messages[1].Parts) != 1 ||
		providerReq.Messages[1].Parts[0].Kind != "tool_use" ||
		providerReq.Messages[1].Parts[0].ToolCallID != "call_1" ||
		providerReq.Messages[1].Parts[0].ToolName != "lookup_weather" {
		t.Fatalf("unexpected assistant tool call message: %+v", providerReq.Messages[1])
	}
	if providerReq.Messages[2].Role != "tool" ||
		len(providerReq.Messages[2].Parts) != 1 ||
		providerReq.Messages[2].Parts[0].Kind != "tool_result" ||
		providerReq.Messages[2].Parts[0].ToolResultForID != "call_1" ||
		providerReq.Messages[2].Parts[0].Text != `{"forecast":"sunny"}` {
		t.Fatalf("unexpected tool result message: %+v", providerReq.Messages[2])
	}
	if len(providerReq.InputParts) != 3 {
		t.Fatalf("expected flat input parts to remain available, got %+v", providerReq.InputParts)
	}
}

func TestProviderConversationRequestPreservesContextManagement(t *testing.T) {
	contextManagement := map[string]any{
		"edits": []any{
			map[string]any{"type": "clear_thinking_20251015"},
		},
	}
	req := gatewaycontract.CanonicalRequest{
		RequestID:         "req_context_management",
		SourceProtocol:    gatewaycontract.ProtocolAnthropicCompatible,
		SourceEndpoint:    "/v1/messages",
		ContextManagement: contextManagement,
	}

	providerReq := providerConversationRequest(req, schedulercontract.Candidate{})

	edits, ok := providerReq.ContextManagement["edits"].([]any)
	if !ok || len(edits) != 1 {
		t.Fatalf("expected context_management edits, got %+v", providerReq.ContextManagement)
	}
	edit, ok := edits[0].(map[string]any)
	if !ok || edit["type"] != "clear_thinking_20251015" {
		t.Fatalf("unexpected context_management edit: %+v", edits[0])
	}
	contextManagement["edits"] = []any{}
	if len(providerReq.ContextManagement["edits"].([]any)) != 1 {
		t.Fatalf("expected provider context_management to be cloned, got %+v", providerReq.ContextManagement)
	}
}
