package httpserver

import (
	"encoding/json"
	"testing"

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
			name: "streaming",
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
