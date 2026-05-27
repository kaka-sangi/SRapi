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

func TestGoldenResponseTerminalConversions(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	resp := gatewaycontract.CanonicalResponse{
		ID:         "resp_golden_terminal",
		RequestID:  "req_golden_terminal",
		Model:      "canonical-text-model",
		StopReason: "max_tokens",
		OutputItems: []gatewaycontract.ContentBlock{{
			Type: gatewaycontract.ContentBlockText,
			Role: "assistant",
			Text: "partial",
		}},
		StreamEvents: []gatewaycontract.StreamEvent{
			{
				Index:        0,
				Type:         gatewaycontract.StreamEventContentDelta,
				ContentIndex: 0,
				Delta: gatewaycontract.ContentBlock{
					Type: gatewaycontract.ContentBlockText,
					Role: "assistant",
					Text: "partial",
				},
			},
			{
				Index:      1,
				Type:       gatewaycontract.StreamEventStop,
				StopReason: "max_tokens",
			},
		},
		Usage: gatewaycontract.Usage{InputTokens: 11, OutputTokens: 7, CachedTokens: 3},
	}

	actual := mustMarshalTerminalGolden(t, terminalConversionGolden{
		Responses:       projectResponsesTerminal(svc.RenderResponses(resp)),
		ResponsesStream: projectResponsesStreamTerminal(svc.RenderResponsesStreamEvents(resp)),
		Chat:            projectChatTerminal(svc.RenderChatCompletions(resp)),
		ChatStream:      projectChatStreamTerminal(svc.RenderChatStreamChunks(resp)),
		Anthropic:       projectAnthropicTerminal(svc.RenderAnthropicMessages(resp)),
		AnthropicStream: projectAnthropicStreamTerminal(svc.RenderAnthropicMessagesStreamEvents(resp)),
		Gemini:          projectGeminiTerminal(svc.RenderGeminiGenerateContent(resp)),
		GeminiStream:    projectGeminiStreamTerminal(svc.RenderGeminiGenerateContentStreamEvents(resp)),
	})
	expected := readTestdata(t, "response_terminal_max_tokens.json")
	assertGoldenJSON(t, expected, actual)
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

func mustMarshalTerminalGolden(t *testing.T, projected terminalConversionGolden) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(projected, "", "  ")
	if err != nil {
		t.Fatalf("marshal terminal conversion golden: %v", err)
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

type terminalConversionGolden struct {
	Responses       responseTerminalGold  `json:"responses"`
	ResponsesStream streamTerminalGold    `json:"responses_stream"`
	Chat            chatTerminalGold      `json:"chat"`
	ChatStream      chatTerminalGold      `json:"chat_stream"`
	Anthropic       anthropicTerminalGold `json:"anthropic"`
	AnthropicStream anthropicTerminalGold `json:"anthropic_stream"`
	Gemini          geminiTerminalGold    `json:"gemini"`
	GeminiStream    geminiTerminalGold    `json:"gemini_stream"`
}

type responseTerminalGold struct {
	Status                  string        `json:"status"`
	IncompleteDetailsReason string        `json:"incomplete_details_reason,omitempty"`
	Usage                   terminalUsage `json:"usage"`
}

type streamTerminalGold struct {
	Event                   string `json:"event"`
	Type                    string `json:"type"`
	Status                  string `json:"status"`
	IncompleteDetailsReason string `json:"incomplete_details_reason,omitempty"`
}

type chatTerminalGold struct {
	FinishReason string        `json:"finish_reason"`
	Usage        terminalUsage `json:"usage,omitempty"`
}

type anthropicTerminalGold struct {
	StopReason string        `json:"stop_reason"`
	Usage      terminalUsage `json:"usage,omitempty"`
}

type geminiTerminalGold struct {
	FinishReason string        `json:"finish_reason"`
	Usage        terminalUsage `json:"usage,omitempty"`
}

type terminalUsage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
	CachedTokens int `json:"cached_tokens,omitempty"`
	TotalTokens  int `json:"total_tokens,omitempty"`
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

func projectResponsesTerminal(resp apiopenapi.ResponsesResponse) responseTerminalGold {
	out := responseTerminalGold{Usage: projectTokenUsage(resp.Usage)}
	if resp.Status != nil {
		out.Status = *resp.Status
	}
	if resp.IncompleteDetails != nil {
		out.IncompleteDetailsReason = resp.IncompleteDetails.Reason
	}
	return out
}

func projectResponsesStreamTerminal(events []StreamEvent) streamTerminalGold {
	for _, event := range events {
		eventType := stringFromAny(event.Data["type"])
		if eventType != "response.completed" && eventType != "response.incomplete" {
			continue
		}
		out := streamTerminalGold{Event: event.Event, Type: eventType}
		response, _ := event.Data["response"].(apiopenapi.ResponsesResponse)
		if response.Status != nil {
			out.Status = *response.Status
		}
		if response.IncompleteDetails != nil {
			out.IncompleteDetailsReason = response.IncompleteDetails.Reason
		}
		return out
	}
	return streamTerminalGold{}
}

func projectChatTerminal(resp apiopenapi.ChatCompletionResponse) chatTerminalGold {
	out := chatTerminalGold{Usage: projectTokenUsage(resp.Usage)}
	if len(resp.Choices) > 0 && resp.Choices[0].FinishReason != nil {
		out.FinishReason = *resp.Choices[0].FinishReason
	}
	return out
}

func projectChatStreamTerminal(chunks []map[string]any) chatTerminalGold {
	for _, chunk := range chunks {
		choices, ok := chunk["choices"].([]map[string]any)
		if !ok || len(choices) == 0 {
			continue
		}
		if finishReason := stringFromAny(choices[0]["finish_reason"]); finishReason != "" {
			return chatTerminalGold{FinishReason: finishReason}
		}
	}
	return chatTerminalGold{}
}

func projectAnthropicTerminal(resp apiopenapi.AnthropicMessagesResponse) anthropicTerminalGold {
	out := anthropicTerminalGold{Usage: projectAnthropicUsage(resp.Usage)}
	if resp.StopReason != nil {
		out.StopReason = *resp.StopReason
	}
	return out
}

func projectAnthropicStreamTerminal(events []StreamEvent) anthropicTerminalGold {
	for _, event := range events {
		if event.Event != "message_delta" {
			continue
		}
		delta, _ := event.Data["delta"].(map[string]any)
		if stopReason := stringFromAny(delta["stop_reason"]); stopReason != "" {
			out := anthropicTerminalGold{StopReason: stopReason}
			if usage, ok := event.Data["usage"].(apiopenapi.AnthropicUsage); ok {
				out.Usage = projectAnthropicUsage(&usage)
			}
			return out
		}
	}
	return anthropicTerminalGold{}
}

func projectGeminiTerminal(resp apiopenapi.GeminiGenerateContentResponse) geminiTerminalGold {
	out := geminiTerminalGold{Usage: projectGeminiUsage(resp.UsageMetadata)}
	if len(resp.Candidates) > 0 {
		out.FinishReason = resp.Candidates[0].FinishReason
	}
	return out
}

func projectGeminiStreamTerminal(events []StreamEvent) geminiTerminalGold {
	for _, event := range events {
		candidates, ok := event.Data["candidates"].([]apiopenapi.GeminiCandidate)
		if !ok || len(candidates) == 0 {
			continue
		}
		if candidates[0].FinishReason != "" {
			return geminiTerminalGold{FinishReason: candidates[0].FinishReason}
		}
	}
	return geminiTerminalGold{}
}

func projectTokenUsage(usage *apiopenapi.TokenUsage) terminalUsage {
	if usage == nil {
		return terminalUsage{}
	}
	out := terminalUsage{}
	if usage.InputTokens != nil {
		out.InputTokens = *usage.InputTokens
	}
	if usage.OutputTokens != nil {
		out.OutputTokens = *usage.OutputTokens
	}
	if usage.CachedTokens != nil {
		out.CachedTokens = *usage.CachedTokens
	}
	if usage.TotalTokens != nil {
		out.TotalTokens = *usage.TotalTokens
	}
	return out
}

func projectAnthropicUsage(usage *apiopenapi.AnthropicUsage) terminalUsage {
	if usage == nil {
		return terminalUsage{}
	}
	out := terminalUsage{}
	if usage.InputTokens != nil {
		out.InputTokens = *usage.InputTokens
	}
	if usage.OutputTokens != nil {
		out.OutputTokens = *usage.OutputTokens
	}
	if usage.CacheReadInputTokens != nil {
		out.CachedTokens = *usage.CacheReadInputTokens
	}
	return out
}

func projectGeminiUsage(usage *apiopenapi.GeminiUsageMetadata) terminalUsage {
	if usage == nil {
		return terminalUsage{}
	}
	out := terminalUsage{}
	if usage.PromptTokenCount != nil {
		out.InputTokens = *usage.PromptTokenCount
	}
	if usage.CandidatesTokenCount != nil {
		out.OutputTokens = *usage.CandidatesTokenCount
	}
	if usage.CachedContentTokenCount != nil {
		out.CachedTokens = *usage.CachedContentTokenCount
	}
	if usage.TotalTokenCount != nil {
		out.TotalTokens = *usage.TotalTokenCount
	}
	return out
}

func stringFromAny(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
