package service

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestGoldenEndpointConversions(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	tests := []struct {
		name           string
		requestFile    string
		goldenFile     string
		sourceEndpoint string
		normalize      func([]byte, RequestMeta) (gatewaycontract.CanonicalRequest, error)
	}{
		{
			name:           "chat_completions",
			requestFile:    "chat_completions_request.json",
			goldenFile:     "chat_completions_canonical.json",
			sourceEndpoint: string(gatewaycontract.EndpointChatCompletions),
			normalize: func(raw []byte, meta RequestMeta) (gatewaycontract.CanonicalRequest, error) {
				var req apiopenapi.ChatCompletionRequest
				if err := json.Unmarshal(raw, &req); err != nil {
					return gatewaycontract.CanonicalRequest{}, err
				}
				return svc.NormalizeChatCompletions(req, meta), nil
			},
		},
		{
			name:           "responses",
			requestFile:    "responses_request.json",
			goldenFile:     "responses_canonical.json",
			sourceEndpoint: string(gatewaycontract.EndpointResponses),
			normalize: func(raw []byte, meta RequestMeta) (gatewaycontract.CanonicalRequest, error) {
				var req apiopenapi.ResponsesRequest
				if err := json.Unmarshal(raw, &req); err != nil {
					return gatewaycontract.CanonicalRequest{}, err
				}
				return svc.NormalizeResponses(req, meta), nil
			},
		},
		{
			name:           "anthropic_messages",
			requestFile:    "anthropic_messages_request.json",
			goldenFile:     "anthropic_messages_canonical.json",
			sourceEndpoint: string(gatewaycontract.EndpointMessages),
			normalize: func(raw []byte, meta RequestMeta) (gatewaycontract.CanonicalRequest, error) {
				var req apiopenapi.AnthropicMessagesRequest
				if err := json.Unmarshal(raw, &req); err != nil {
					return gatewaycontract.CanonicalRequest{}, err
				}
				return svc.NormalizeAnthropicMessages(req, meta), nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := readTestdata(t, tt.requestFile)
			canonical, err := tt.normalize(raw, goldenMeta(tt.sourceEndpoint))
			if err != nil {
				t.Fatalf("normalize %s: %v", tt.requestFile, err)
			}

			actual := mustMarshalCanonicalGolden(t, canonical)
			expected := readTestdata(t, tt.goldenFile)
			assertGoldenJSON(t, expected, actual)
		})
	}
}

func goldenMeta(sourceEndpoint string) RequestMeta {
	return RequestMeta{
		RequestID:      "req_golden_001",
		SourceEndpoint: sourceEndpoint,
		UserID:         101,
		APIKeyID:       202,
		CanonicalModel: "canonical-text-model",
	}
}

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "golden", name))
	if err != nil {
		t.Fatalf("read testdata %s: %v", name, err)
	}
	return raw
}

func mustMarshalCanonicalGolden(t *testing.T, req gatewaycontract.CanonicalRequest) []byte {
	t.Helper()
	projected := canonicalRequestGolden{
		RequestID:             req.RequestID,
		SourceProtocol:        string(req.SourceProtocol),
		SourceEndpoint:        req.SourceEndpoint,
		ResponseProtocol:      string(req.ResponseProtocol),
		UserID:                req.UserID,
		APIKeyID:              req.APIKeyID,
		Model:                 req.Model,
		CanonicalModel:        req.CanonicalModel,
		InputItems:            projectContentBlocks(req.InputItems),
		Messages:              projectMessages(req.Messages),
		Instructions:          req.Instructions,
		Stream:                req.Stream,
		MaxOutputTokens:       req.MaxOutputTokens,
		Tools:                 req.Tools,
		ToolChoice:            req.ToolChoice,
		ResponseFormat:        req.ResponseFormat,
		Reasoning:             req.Reasoning,
		Prompt:                req.Prompt,
		CompatibilityWarnings: append([]string(nil), req.CompatibilityWarnings...),
		RequestCapabilities:   projectCapabilities(req.RequestCapabilities),
	}
	raw, err := json.MarshalIndent(projected, "", "  ")
	if err != nil {
		t.Fatalf("marshal canonical request golden: %v", err)
	}
	return append(raw, '\n')
}

func assertGoldenJSON(t *testing.T, expected, actual []byte) {
	t.Helper()
	expected = bytes.TrimSpace(expected)
	actual = bytes.TrimSpace(actual)
	if !bytes.Equal(expected, actual) {
		t.Fatalf("golden mismatch\nexpected:\n%s\nactual:\n%s", expected, actual)
	}
}

type canonicalRequestGolden struct {
	RequestID             string             `json:"request_id"`
	SourceProtocol        string             `json:"source_protocol"`
	SourceEndpoint        string             `json:"source_endpoint"`
	ResponseProtocol      string             `json:"response_protocol"`
	UserID                int                `json:"user_id"`
	APIKeyID              int                `json:"api_key_id"`
	Model                 string             `json:"model"`
	CanonicalModel        string             `json:"canonical_model"`
	InputItems            []contentBlockGold `json:"input_items,omitempty"`
	Messages              []messageGold      `json:"messages,omitempty"`
	Instructions          string             `json:"instructions,omitempty"`
	Stream                bool               `json:"stream"`
	MaxOutputTokens       *int               `json:"max_output_tokens,omitempty"`
	Tools                 []map[string]any   `json:"tools,omitempty"`
	ToolChoice            any                `json:"tool_choice,omitempty"`
	ResponseFormat        map[string]any     `json:"response_format,omitempty"`
	Reasoning             map[string]any     `json:"reasoning,omitempty"`
	Prompt                string             `json:"prompt"`
	CompatibilityWarnings []string           `json:"compatibility_warnings,omitempty"`
	RequestCapabilities   []capabilityGold   `json:"request_capabilities,omitempty"`
}

type messageGold struct {
	Role    string             `json:"role"`
	Content []contentBlockGold `json:"content"`
}

type contentBlockGold struct {
	Type     string         `json:"type"`
	Role     string         `json:"role,omitempty"`
	Text     string         `json:"text,omitempty"`
	MediaURL string         `json:"media_url,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type capabilityGold struct {
	Key     string `json:"key"`
	Version string `json:"version"`
}

func projectMessages(messages []gatewaycontract.Message) []messageGold {
	if len(messages) == 0 {
		return nil
	}
	out := make([]messageGold, 0, len(messages))
	for _, message := range messages {
		out = append(out, messageGold{
			Role:    message.Role,
			Content: projectContentBlocks(message.Content),
		})
	}
	return out
}

func projectContentBlocks(blocks []gatewaycontract.ContentBlock) []contentBlockGold {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]contentBlockGold, 0, len(blocks))
	for _, block := range blocks {
		out = append(out, contentBlockGold{
			Type:     string(block.Type),
			Role:     block.Role,
			Text:     block.Text,
			MediaURL: block.MediaURL,
			Metadata: block.Metadata,
		})
	}
	return out
}

func projectCapabilities(capabilities []gatewaycontract.RequestCapability) []capabilityGold {
	if len(capabilities) == 0 {
		return nil
	}
	out := make([]capabilityGold, 0, len(capabilities))
	for _, capability := range capabilities {
		out = append(out, capabilityGold{Key: capability.Key, Version: capability.Version})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key == out[j].Key {
			return out[i].Version < out[j].Version
		}
		return out[i].Key < out[j].Key
	})
	return out
}
