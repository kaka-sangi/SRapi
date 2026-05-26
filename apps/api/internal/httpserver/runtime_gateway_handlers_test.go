package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

func TestSameProtocolRawConversationResponseAllowsClaudeCodeMessages(t *testing.T) {
	req := gatewaycontract.CanonicalRequest{
		SourceProtocol: gatewaycontract.ProtocolAnthropicCompatible,
		SourceEndpoint: "/v1/messages",
	}

	if !sameProtocolRawConversationResponse(req, "anthropic-compatible", "reverse-proxy-claude-code-cli", []byte(`{"id":"msg_1"}`)) {
		t.Fatal("expected Claude Code same-protocol messages response to be eligible for raw passthrough")
	}
}

func TestSameProtocolRawConversationStreamAllowsEndpointMatchedSSE(t *testing.T) {
	raw := []byte(": keepalive\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n")
	req := gatewaycontract.CanonicalRequest{
		SourceProtocol: gatewaycontract.ProtocolOpenAICompatible,
		SourceEndpoint: "/v1/chat/completions",
		Stream:         true,
	}

	if !sameProtocolRawConversationStream(req, "openai-compatible", "openai-compatible", raw) {
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

	if !sameProtocolRawConversationStream(req, "openai-compatible", "reverse-proxy-codex-cli", raw) {
		t.Fatal("expected Codex Responses stream to be eligible for raw SSE replay")
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
			if sameProtocolRawConversationResponse(tt.req, tt.targetProtocol, tt.adapterType, tt.raw) {
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
			if sameProtocolRawConversationStream(tt.req, tt.targetProtocol, tt.adapterType, tt.raw) {
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
