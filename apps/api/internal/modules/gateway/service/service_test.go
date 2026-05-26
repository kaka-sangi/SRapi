package service

import (
	"testing"

	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestNormalizeChatCompletionsProducesCanonicalRequest(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	text := "hello"
	image := apiopenapi.JsonObject{"url": "https://example.invalid/image.png"}
	req := apiopenapi.ChatCompletionRequest{
		Model: "gpt-4o-mini",
		Messages: []apiopenapi.ChatMessage{
			{
				Role: apiopenapi.ChatMessageRoleUser,
				Content: mustChatContentBlocks(t, []apiopenapi.ContentBlock{
					{Type: apiopenapi.ContentBlockTypeText, Text: &text},
					{Type: apiopenapi.ContentBlockTypeImageUrl, ImageUrl: &image},
				}),
			},
		},
		ResponseFormat: &apiopenapi.JsonObject{"type": "json_object"},
	}

	canonical := svc.NormalizeChatCompletions(req, RequestMeta{
		RequestID:      "req_1",
		SourceEndpoint: "/v1/chat/completions",
		UserID:         7,
		APIKeyID:       11,
		CanonicalModel: "gpt-4o-mini",
	})

	if canonical.SourceProtocol != gatewaycontract.ProtocolOpenAICompatible || canonical.ResponseProtocol != gatewaycontract.ProtocolOpenAICompatible {
		t.Fatalf("unexpected protocol mapping: %+v", canonical)
	}
	if canonical.Prompt != "user: hello\n[image]" {
		t.Fatalf("unexpected prompt: %q", canonical.Prompt)
	}
	if len(canonical.CompatibilityWarnings) != 1 || !stringSliceContains(canonical.CompatibilityWarnings, "vision_ignored") {
		t.Fatalf("expected compatibility warnings, got %+v", canonical.CompatibilityWarnings)
	}
	if canonical.ResponseFormat["type"] != "json_object" {
		t.Fatalf("expected response format to be preserved, got %+v", canonical.ResponseFormat)
	}
	if !requestCapabilityContains(canonical.RequestCapabilities, capabilitiescontract.KeyVisionInput) || !requestCapabilityContains(canonical.RequestCapabilities, capabilitiescontract.KeyStructuredOutput) {
		t.Fatalf("expected request capabilities, got %+v", canonical.RequestCapabilities)
	}
}

func TestNormalizeResponsesPreservesInstructionsAndWarnings(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	input := apiopenapi.ResponsesRequest_Input{}
	if err := input.FromResponsesRequestInput0("summarize this"); err != nil {
		t.Fatalf("set input: %v", err)
	}
	instructions := "be concise"
	req := apiopenapi.ResponsesRequest{
		Model:        "gpt-4o-mini",
		Input:        input,
		Instructions: &instructions,
		Reasoning:    &apiopenapi.JsonObject{"effort": "low"},
	}

	canonical := svc.NormalizeResponses(req, RequestMeta{SourceEndpoint: "/v1/responses"})

	if canonical.Instructions != "be concise" {
		t.Fatalf("unexpected instructions: %q", canonical.Instructions)
	}
	if canonical.Prompt != "instructions: be concise\nsummarize this" {
		t.Fatalf("unexpected prompt: %q", canonical.Prompt)
	}
	if !stringSliceContains(canonical.CompatibilityWarnings, "reasoning_ignored") {
		t.Fatalf("expected reasoning warning, got %+v", canonical.CompatibilityWarnings)
	}
}

func TestNormalizeRealtimeWebSocketRequiresRealtimeCapability(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	canonical := svc.NormalizeRealtimeWebSocket("gpt-realtime-2", RequestMeta{
		RequestID:      "req_realtime",
		SourceEndpoint: string(gatewaycontract.EndpointRealtime),
		UserID:         7,
		APIKeyID:       11,
		CanonicalModel: "gpt-realtime-2",
	})

	if canonical.SourceEndpoint != string(gatewaycontract.EndpointRealtime) || !canonical.Stream {
		t.Fatalf("unexpected realtime canonical request: %+v", canonical)
	}
	if !requestCapabilityContains(canonical.RequestCapabilities, capabilitiescontract.KeyRealtimeWebSocket) ||
		!requestCapabilityContains(canonical.RequestCapabilities, capabilitiescontract.KeyStreaming) {
		t.Fatalf("expected realtime websocket and streaming capabilities, got %+v", canonical.RequestCapabilities)
	}
}

func TestNormalizeGeminiGenerateContentProducesCanonicalRequest(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	systemRole := apiopenapi.GeminiContentRoleUser
	userRole := apiopenapi.GeminiContentRoleUser
	modelRole := apiopenapi.GeminiContentRoleModel
	systemText := "be brief"
	userText := "hello gemini"
	modelText := "previous answer"
	inlineData := apiopenapi.JsonObject{"mime_type": "image/png", "data": "abc"}
	req := apiopenapi.GeminiGenerateContentRequest{
		SystemInstruction: &apiopenapi.GeminiContent{Role: &systemRole, Parts: []apiopenapi.GeminiPart{{Text: &systemText}}},
		Contents: []apiopenapi.GeminiContent{
			{Role: &userRole, Parts: []apiopenapi.GeminiPart{{Text: &userText}, {InlineData: &inlineData}}},
			{Role: &modelRole, Parts: []apiopenapi.GeminiPart{{Text: &modelText}}},
		},
		GenerationConfig: &apiopenapi.GeminiGenerationConfig{
			MaxOutputTokens:  ptrInt(32),
			ResponseMimeType: ptrString("application/json"),
			TopK:             ptrInt(40),
		},
		SafetySettings: &[]apiopenapi.JsonObject{{"category": "HARM_CATEGORY_DANGEROUS_CONTENT"}},
	}

	canonical := svc.NormalizeGeminiGenerateContent(req, "gemini-test", true, RequestMeta{
		RequestID:      "req_gemini",
		SourceEndpoint: "/v1beta/models/gemini-test:streamGenerateContent",
		UserID:         7,
		APIKeyID:       11,
		CanonicalModel: "gemini-test",
	})

	if canonical.SourceProtocol != gatewaycontract.ProtocolGeminiCompatible || canonical.ResponseProtocol != gatewaycontract.ProtocolGeminiCompatible || !canonical.Stream {
		t.Fatalf("unexpected protocol mapping: %+v", canonical)
	}
	if canonical.Instructions != "be brief" || canonical.Prompt != "system: be brief\nuser: hello gemini\n[image]\nassistant: previous answer" {
		t.Fatalf("unexpected prompt or instructions: prompt=%q instructions=%q", canonical.Prompt, canonical.Instructions)
	}
	if canonical.MaxOutputTokens == nil || *canonical.MaxOutputTokens != 32 || canonical.ResponseFormat["type"] != "application/json" {
		t.Fatalf("expected generation config fields, got %+v", canonical)
	}
	for _, warning := range []string{"vision_ignored", "safety_settings_ignored", "top_k_ignored"} {
		if !stringSliceContains(canonical.CompatibilityWarnings, warning) {
			t.Fatalf("expected warning %s, got %+v", warning, canonical.CompatibilityWarnings)
		}
	}
	if !requestCapabilityContains(canonical.RequestCapabilities, capabilitiescontract.KeyStreaming) || !requestCapabilityContains(canonical.RequestCapabilities, capabilitiescontract.KeyVisionInput) || !requestCapabilityContains(canonical.RequestCapabilities, capabilitiescontract.KeyStructuredOutput) {
		t.Fatalf("expected request capabilities, got %+v", canonical.RequestCapabilities)
	}
}

func TestRenderProtocolResponses(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	resp := svc.BuildConversationResponse("gpt-4o-mini", "gpt-4o-mini", "hello back", []string{"tools_ignored"})

	chat := svc.RenderChatCompletions(resp)
	if chat.Model != "gpt-4o-mini" || len(chat.Choices) != 1 {
		t.Fatalf("unexpected chat response: %+v", chat)
	}
	responses := svc.RenderResponses(resp)
	if responses.CompatibilityWarnings == nil || len(*responses.CompatibilityWarnings) != 1 {
		t.Fatalf("expected responses warnings, got %+v", responses.CompatibilityWarnings)
	}
	anthropic := svc.RenderAnthropicMessages(resp)
	if anthropic.CompatibilityWarnings == nil || len(*anthropic.CompatibilityWarnings) != 1 {
		t.Fatalf("expected anthropic warnings, got %+v", anthropic.CompatibilityWarnings)
	}
	gemini := svc.RenderGeminiGenerateContent(resp)
	if gemini.CompatibilityWarnings == nil || len(*gemini.CompatibilityWarnings) != 1 || len(gemini.Candidates) != 1 || gemini.Candidates[0].Content.Parts[0].Text == nil {
		t.Fatalf("expected gemini content and warnings, got %+v", gemini)
	}
	if gemini.UsageMetadata == nil || gemini.UsageMetadata.TotalTokenCount == nil || *gemini.UsageMetadata.TotalTokenCount == 0 {
		t.Fatalf("expected gemini usage metadata, got %+v", gemini.UsageMetadata)
	}
}

func TestRenderProtocolResponsesPreservesToolCallOutput(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	resp := gatewaycontract.CanonicalResponse{
		ID:         "resp_tool",
		Model:      "gpt-4o-mini",
		StopReason: "tool_use",
		OutputItems: []gatewaycontract.ContentBlock{{
			Type:              gatewaycontract.ContentBlockToolCall,
			Role:              "assistant",
			ToolCallID:        "call_1",
			ToolName:          "lookup",
			ToolArgumentsJSON: `{"query":"weather"}`,
		}},
		Usage: gatewaycontract.Usage{InputTokens: 3, OutputTokens: 1},
	}

	chat := svc.RenderChatCompletions(resp)
	if got := *chat.Choices[0].FinishReason; got != "tool_calls" {
		t.Fatalf("expected chat tool_calls finish reason, got %q", got)
	}
	if chat.Choices[0].Message.ToolCalls == nil || len(*chat.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("expected chat tool_calls field, got %+v", chat.Choices[0].Message)
	}
	chatToolCall := (*chat.Choices[0].Message.ToolCalls)[0]
	if chatToolCall.Id != "call_1" || chatToolCall.Function["name"] != "lookup" || chatToolCall.Function["arguments"] != `{"query":"weather"}` {
		t.Fatalf("expected chat tool call payload, got %+v", chatToolCall)
	}
	chatContent, err := chat.Choices[0].Message.Content.AsChatMessageContent0()
	if err != nil || chatContent != "" {
		t.Fatalf("expected empty chat content with tool call, content=%q err=%v", chatContent, err)
	}

	responses := svc.RenderResponses(resp)
	if len(responses.Output) != 1 || responses.Output[0].Type != "function_call" {
		t.Fatalf("expected responses function_call item, got %+v", responses.Output)
	}
	if name, _ := responses.Output[0].Get("name"); name != "lookup" {
		t.Fatalf("expected responses tool name, got %+v", responses.Output[0])
	}

	anthropic := svc.RenderAnthropicMessages(resp)
	if anthropic.StopReason == nil || *anthropic.StopReason != "tool_use" || len(anthropic.Content) != 1 || anthropic.Content[0].Type != apiopenapi.AnthropicContentBlockTypeToolUse {
		t.Fatalf("expected anthropic tool_use response, got %+v", anthropic)
	}
	input, _ := anthropic.Content[0].Get("input")
	inputMap, _ := input.(map[string]any)
	if inputMap["query"] != "weather" {
		t.Fatalf("expected anthropic tool input, got %+v", anthropic.Content[0])
	}

	gemini := svc.RenderGeminiGenerateContent(resp)
	if len(gemini.Candidates) != 1 || len(gemini.Candidates[0].Content.Parts) != 1 || gemini.Candidates[0].Content.Parts[0].FunctionCall == nil {
		t.Fatalf("expected gemini function_call part, got %+v", gemini)
	}
	functionCall := *gemini.Candidates[0].Content.Parts[0].FunctionCall
	if functionCall["name"] != "lookup" {
		t.Fatalf("expected gemini function name, got %+v", functionCall)
	}
	args, _ := functionCall["args"].(map[string]any)
	if args["query"] != "weather" {
		t.Fatalf("expected gemini function args, got %+v", functionCall)
	}
}

func TestRenderProtocolStreamEventsPreservesToolCallOutput(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	resp := gatewaycontract.CanonicalResponse{
		ID:         "resp_tool_stream",
		Model:      "gpt-4o-mini",
		StopReason: "tool_use",
		OutputItems: []gatewaycontract.ContentBlock{{
			Type:              gatewaycontract.ContentBlockToolCall,
			Role:              "assistant",
			ToolCallID:        "call_1",
			ToolName:          "lookup",
			ToolArgumentsJSON: `{"query":"weather"}`,
		}},
		Usage: gatewaycontract.Usage{InputTokens: 3, OutputTokens: 1},
	}

	chatChunk := svc.RenderChatStreamChunk(resp)
	choices, _ := chatChunk["choices"].([]map[string]any)
	if len(choices) != 1 || choices[0]["finish_reason"] != "tool_calls" {
		t.Fatalf("expected chat stream tool_calls finish reason, got %+v", chatChunk)
	}
	delta, _ := choices[0]["delta"].(map[string]any)
	toolCalls, _ := delta["tool_calls"].([]map[string]any)
	if len(toolCalls) != 1 || toolCalls[0]["id"] != "call_1" {
		t.Fatalf("expected chat stream tool call delta, got %+v", delta)
	}
	function, _ := toolCalls[0]["function"].(map[string]any)
	if function["name"] != "lookup" || function["arguments"] != `{"query":"weather"}` {
		t.Fatalf("expected chat stream function payload, got %+v", toolCalls[0])
	}

	responsesEvents := svc.RenderResponsesStreamEvents(resp)
	added := streamEventByName(responsesEvents, "response.output_item.added")
	if added == nil {
		t.Fatalf("expected responses output item event, got %+v", responsesEvents)
	}
	item, _ := added.Data["item"].(map[string]any)
	if item["type"] != "function_call" || item["name"] != "lookup" {
		t.Fatalf("expected responses function_call item, got %+v", item)
	}
	argsDelta := streamEventByName(responsesEvents, "response.function_call_arguments.delta")
	if argsDelta == nil || argsDelta.Data["delta"] != `{"query":"weather"}` {
		t.Fatalf("expected responses function arguments delta, got %+v", responsesEvents)
	}

	anthropicEvents := svc.RenderAnthropicMessagesStreamEvents(resp)
	blockStart := streamEventByName(anthropicEvents, "content_block_start")
	if blockStart == nil {
		t.Fatalf("expected anthropic content block start, got %+v", anthropicEvents)
	}
	contentBlock, _ := blockStart.Data["content_block"].(map[string]any)
	if contentBlock["type"] != "tool_use" || contentBlock["id"] != "call_1" || contentBlock["name"] != "lookup" {
		t.Fatalf("expected anthropic tool_use block, got %+v", contentBlock)
	}
	input, _ := contentBlock["input"].(map[string]any)
	if input["query"] != "weather" {
		t.Fatalf("expected anthropic tool input, got %+v", contentBlock)
	}
	blockDelta := streamEventByName(anthropicEvents, "content_block_delta")
	if blockDelta == nil {
		t.Fatalf("expected anthropic tool delta, got %+v", anthropicEvents)
	}
	deltaPayload, _ := blockDelta.Data["delta"].(map[string]any)
	if deltaPayload["type"] != "input_json_delta" || deltaPayload["partial_json"] != `{"query":"weather"}` {
		t.Fatalf("expected anthropic input json delta, got %+v", deltaPayload)
	}
	messageDelta := streamEventByName(anthropicEvents, "message_delta")
	if messageDelta == nil {
		t.Fatalf("expected anthropic message delta, got %+v", anthropicEvents)
	}
	stopDelta, _ := messageDelta.Data["delta"].(map[string]any)
	if stopDelta["stop_reason"] != "tool_use" {
		t.Fatalf("expected anthropic tool_use stop reason, got %+v", stopDelta)
	}

	geminiEvents := svc.RenderGeminiGenerateContentStreamEvents(resp)
	candidates, _ := geminiEvents[0].Data["candidates"].([]apiopenapi.GeminiCandidate)
	if len(candidates) != 1 || len(candidates[0].Content.Parts) != 1 || candidates[0].Content.Parts[0].FunctionCall == nil {
		t.Fatalf("expected gemini stream function call, got %+v", geminiEvents)
	}
	geminiFunction := *candidates[0].Content.Parts[0].FunctionCall
	if geminiFunction["name"] != "lookup" {
		t.Fatalf("expected gemini stream function name, got %+v", geminiFunction)
	}
}

func streamEventByName(events []StreamEvent, name string) *StreamEvent {
	for idx := range events {
		if events[idx].Event == name {
			return &events[idx]
		}
	}
	return nil
}

func mustChatContentBlocks(t *testing.T, blocks []apiopenapi.ContentBlock) apiopenapi.ChatMessage_Content {
	t.Helper()
	content := apiopenapi.ChatMessage_Content{}
	if err := content.FromChatMessageContent1(blocks); err != nil {
		t.Fatalf("set chat content blocks: %v", err)
	}
	return content
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func requestCapabilityContains(values []gatewaycontract.RequestCapability, target string) bool {
	for _, value := range values {
		if value.Key == target {
			return true
		}
	}
	return false
}
