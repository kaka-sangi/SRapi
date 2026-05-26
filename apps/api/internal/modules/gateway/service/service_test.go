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
	if len(canonical.CompatibilityWarnings) != 0 {
		t.Fatalf("did not expect compatibility warnings for preserved image input, got %+v", canonical.CompatibilityWarnings)
	}
	if len(canonical.Messages) != 1 || len(canonical.Messages[0].Content) != 2 || canonical.Messages[0].Content[1].MediaURL != "https://example.invalid/image.png" {
		t.Fatalf("expected image media URL to be preserved, got %+v", canonical.Messages)
	}
	if canonical.ResponseFormat["type"] != "json_object" {
		t.Fatalf("expected response format to be preserved, got %+v", canonical.ResponseFormat)
	}
	if !requestCapabilityContains(canonical.RequestCapabilities, capabilitiescontract.KeyVisionInput) || !requestCapabilityContains(canonical.RequestCapabilities, capabilitiescontract.KeyStructuredOutput) {
		t.Fatalf("expected request capabilities, got %+v", canonical.RequestCapabilities)
	}
}

func TestNormalizeResponsesPreservesInstructionsAndReasoning(t *testing.T) {
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
	if len(canonical.CompatibilityWarnings) != 0 {
		t.Fatalf("did not expect reasoning to be marked ignored, got %+v", canonical.CompatibilityWarnings)
	}
	if canonical.Reasoning["effort"] != "low" {
		t.Fatalf("expected reasoning control to be preserved, got %+v", canonical.Reasoning)
	}
}

func TestNormalizeResponsesRequiresWebSearchCapability(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	input := apiopenapi.ResponsesRequest_Input{}
	if err := input.FromResponsesRequestInput0("search the latest release notes"); err != nil {
		t.Fatalf("set input: %v", err)
	}
	tools := []apiopenapi.ToolDefinition{{
		Type: "web_search",
		AdditionalProperties: map[string]interface{}{
			"search_context_size": "low",
		},
	}}
	req := apiopenapi.ResponsesRequest{
		Model: "gpt-5.5",
		Input: input,
		Tools: &tools,
	}

	canonical := svc.NormalizeResponses(req, RequestMeta{SourceEndpoint: "/v1/responses"})

	if !requestCapabilityContains(canonical.RequestCapabilities, capabilitiescontract.KeyToolCalling) ||
		!requestCapabilityContains(canonical.RequestCapabilities, capabilitiescontract.KeyWebSearch) {
		t.Fatalf("expected tool calling and web search capabilities, got %+v", canonical.RequestCapabilities)
	}
	if len(canonical.Tools) != 1 || canonical.Tools[0]["type"] != "web_search" || canonical.Tools[0]["search_context_size"] != "low" {
		t.Fatalf("expected Responses web_search tool to be preserved, got %+v", canonical.Tools)
	}
}

func TestNormalizeResponsesPreservesRawFunctionItems(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	input := apiopenapi.ResponsesRequest_Input{}
	if err := input.FromResponsesRequestInput1(nil); err != nil {
		t.Fatalf("set empty input: %v", err)
	}
	req := apiopenapi.ResponsesRequest{
		Model: "gpt-5.4",
		Input: input,
	}
	rawBody := []byte(`{
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"What is the weather?"}]},
			{"type":"function_call","call_id":"call_1","name":"lookup_weather","arguments":{"city":"Boston"}},
			{"type":"function_call_output","call_id":"call_1","output":{"forecast":"sunny"}}
		]
	}`)

	canonical := svc.NormalizeResponses(req, RequestMeta{SourceEndpoint: "/v1/responses", RawBody: rawBody})

	if len(canonical.InputItems) != 3 {
		t.Fatalf("expected text, function_call, and function_call_output items, got %+v", canonical.InputItems)
	}
	if canonical.InputItems[0].Type != gatewaycontract.ContentBlockText ||
		canonical.InputItems[0].Role != "user" ||
		canonical.InputItems[0].Text != "What is the weather?" {
		t.Fatalf("unexpected text item: %+v", canonical.InputItems[0])
	}
	call := canonical.InputItems[1]
	if call.Type != gatewaycontract.ContentBlockToolCall ||
		call.Role != "assistant" ||
		call.ToolCallID != "call_1" ||
		call.ToolName != "lookup_weather" ||
		call.ToolArgumentsJSON != `{"city":"Boston"}` {
		t.Fatalf("unexpected function_call item: %+v", call)
	}
	output := canonical.InputItems[2]
	if output.Type != gatewaycontract.ContentBlockToolResult ||
		output.ToolResultForID != "call_1" ||
		output.Text != `{"forecast":"sunny"}` {
		t.Fatalf("unexpected function_call_output item: %+v", output)
	}
	if !requestCapabilityContains(canonical.RequestCapabilities, capabilitiescontract.KeyToolCalling) {
		t.Fatalf("expected tool calling capability, got %+v", canonical.RequestCapabilities)
	}
	if canonical.Prompt != "What is the weather?\n[function_call]\n{\"forecast\":\"sunny\"}" {
		t.Fatalf("unexpected prompt: %q", canonical.Prompt)
	}
}

func TestNormalizeResponsesPreservesRawContextItems(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	input := apiopenapi.ResponsesRequest_Input{}
	if err := input.FromResponsesRequestInput1(nil); err != nil {
		t.Fatalf("set empty input: %v", err)
	}
	req := apiopenapi.ResponsesRequest{
		Model: "gpt-5.4",
		Input: input,
	}
	rawBody := []byte(`{
		"input":[
			{"type":"reasoning","id":"rs_1","encrypted_content":"gAAA","summary":[{"type":"summary_text","text":"kept"}]},
			{"type":"item_reference","id":"fc_1"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"continue"}]}
		]
	}`)

	canonical := svc.NormalizeResponses(req, RequestMeta{SourceEndpoint: "/v1/responses", RawBody: rawBody})

	if len(canonical.InputItems) != 3 {
		t.Fatalf("expected reasoning, item_reference, and message input items, got %+v", canonical.InputItems)
	}
	reasoning := canonical.InputItems[0]
	if reasoning.Type != gatewaycontract.ContentBlockMetadata ||
		reasoning.Role != "assistant" ||
		reasoning.OriginProtocol != string(gatewaycontract.ProtocolOpenAICompatible) ||
		reasoning.Metadata["responses_item_type"] != "reasoning" ||
		string(reasoning.Raw) == "" {
		t.Fatalf("expected raw reasoning item metadata, got %+v", reasoning)
	}
	reference := canonical.InputItems[1]
	if reference.Type != gatewaycontract.ContentBlockMetadata ||
		reference.Metadata["responses_item_type"] != "item_reference" ||
		string(reference.Raw) == "" {
		t.Fatalf("expected raw item_reference metadata, got %+v", reference)
	}
	if canonical.InputItems[2].Type != gatewaycontract.ContentBlockText ||
		canonical.InputItems[2].Role != "user" ||
		canonical.InputItems[2].Text != "continue" {
		t.Fatalf("unexpected message item: %+v", canonical.InputItems[2])
	}
	if canonical.Prompt != "continue" {
		t.Fatalf("unexpected prompt: %q", canonical.Prompt)
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
	for _, warning := range []string{"safety_settings_ignored", "top_k_ignored"} {
		if !stringSliceContains(canonical.CompatibilityWarnings, warning) {
			t.Fatalf("expected warning %s, got %+v", warning, canonical.CompatibilityWarnings)
		}
	}
	if len(canonical.Messages) != 3 || len(canonical.Messages[1].Content) != 2 || canonical.Messages[1].Content[1].MediaBase64 != "abc" || canonical.Messages[1].Content[1].MIMEType != "image/png" {
		t.Fatalf("expected Gemini inline media to be preserved, got %+v", canonical.Messages)
	}
	if !requestCapabilityContains(canonical.RequestCapabilities, capabilitiescontract.KeyStreaming) || !requestCapabilityContains(canonical.RequestCapabilities, capabilitiescontract.KeyVisionInput) || !requestCapabilityContains(canonical.RequestCapabilities, capabilitiescontract.KeyStructuredOutput) {
		t.Fatalf("expected request capabilities, got %+v", canonical.RequestCapabilities)
	}
}

func TestNormalizeAnthropicMessagesPreservesWebSearchServerTool(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	content := apiopenapi.AnthropicMessage_Content{}
	if err := content.FromAnthropicMessageContent0("search docs"); err != nil {
		t.Fatalf("set message content: %v", err)
	}
	tools := []apiopenapi.JsonObject{{
		"type": "web_search_20250305",
		"name": "web_search",
	}}
	req := apiopenapi.AnthropicMessagesRequest{
		Model:     "claude-sonnet",
		MaxTokens: 32,
		Messages: []apiopenapi.AnthropicMessage{{
			Role:    apiopenapi.AnthropicMessageRoleUser,
			Content: content,
		}},
		Tools: &tools,
	}

	canonical := svc.NormalizeAnthropicMessages(req, RequestMeta{SourceEndpoint: "/v1/messages"})

	if !requestCapabilityContains(canonical.RequestCapabilities, capabilitiescontract.KeyToolCalling) ||
		!requestCapabilityContains(canonical.RequestCapabilities, capabilitiescontract.KeyWebSearch) {
		t.Fatalf("expected tool calling and web search capabilities, got %+v", canonical.RequestCapabilities)
	}
	if len(canonical.Tools) != 1 || canonical.Tools[0]["type"] != "web_search_20250305" || canonical.Tools[0]["name"] != "web_search" {
		t.Fatalf("expected Anthropic web search server tool to be preserved, got %+v", canonical.Tools)
	}
	if _, ok := canonical.Tools[0]["function"]; ok {
		t.Fatalf("did not expect hosted web search tool to be rewritten as a function, got %+v", canonical.Tools[0])
	}
}

func TestNormalizeAnthropicMessagesDoesNotTreatFunctionNamedWebSearchAsHosted(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	content := apiopenapi.AnthropicMessage_Content{}
	if err := content.FromAnthropicMessageContent0("call my local tool"); err != nil {
		t.Fatalf("set message content: %v", err)
	}
	tools := []apiopenapi.JsonObject{{
		"name":         "web_search",
		"description":  "local project search",
		"input_schema": map[string]any{"type": "object"},
	}}
	req := apiopenapi.AnthropicMessagesRequest{
		Model:     "claude-sonnet",
		MaxTokens: 32,
		Messages: []apiopenapi.AnthropicMessage{{
			Role:    apiopenapi.AnthropicMessageRoleUser,
			Content: content,
		}},
		Tools: &tools,
	}

	canonical := svc.NormalizeAnthropicMessages(req, RequestMeta{SourceEndpoint: "/v1/messages"})

	if requestCapabilityContains(canonical.RequestCapabilities, capabilitiescontract.KeyWebSearch) {
		t.Fatalf("did not expect local function named web_search to require hosted web search, got %+v", canonical.RequestCapabilities)
	}
	if len(canonical.Tools) != 1 || canonical.Tools[0]["type"] != "function" {
		t.Fatalf("expected local web_search tool to remain a function, got %+v", canonical.Tools)
	}
}

func TestNormalizeAnthropicMessagesPreservesToolResultImages(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	blocks := []apiopenapi.AnthropicContentBlock{{
		Type: apiopenapi.AnthropicContentBlockTypeToolResult,
		AdditionalProperties: map[string]any{
			"tool_use_id": "toolu_1",
			"content": []any{
				map[string]any{"type": "text", "text": "File metadata: 800x600 PNG"},
				map[string]any{
					"type": "image",
					"source": map[string]any{
						"type":       "base64",
						"media_type": "image/png",
						"data":       "iVBOR",
					},
				},
			},
		},
	}}
	content := apiopenapi.AnthropicMessage_Content{}
	if err := content.FromAnthropicMessageContent1(blocks); err != nil {
		t.Fatalf("set message content: %v", err)
	}
	req := apiopenapi.AnthropicMessagesRequest{
		Model:     "claude-sonnet",
		MaxTokens: 32,
		Messages: []apiopenapi.AnthropicMessage{{
			Role:    apiopenapi.AnthropicMessageRoleUser,
			Content: content,
		}},
	}

	canonical := svc.NormalizeAnthropicMessages(req, RequestMeta{SourceEndpoint: "/v1/messages"})

	if len(canonical.Messages) != 1 || len(canonical.Messages[0].Content) != 2 {
		t.Fatalf("expected tool result plus nested image block, got %+v", canonical.Messages)
	}
	toolResult := canonical.Messages[0].Content[0]
	if toolResult.Type != gatewaycontract.ContentBlockToolResult ||
		toolResult.ToolResultForID != "toolu_1" ||
		toolResult.Text != "File metadata: 800x600 PNG" {
		t.Fatalf("unexpected tool result block: %+v", toolResult)
	}
	image := canonical.Messages[0].Content[1]
	if image.Type != gatewaycontract.ContentBlockImage ||
		image.MediaBase64 != "iVBOR" ||
		image.MIMEType != "image/png" ||
		image.OriginProtocol != string(gatewaycontract.ProtocolAnthropicCompatible) {
		t.Fatalf("unexpected nested image block: %+v", image)
	}
	if !requestCapabilityContains(canonical.RequestCapabilities, capabilitiescontract.KeyVisionInput) ||
		!requestCapabilityContains(canonical.RequestCapabilities, capabilitiescontract.KeyToolCalling) {
		t.Fatalf("expected vision and tool calling capabilities, got %+v", canonical.RequestCapabilities)
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

func TestRenderAnthropicMessagesPreservesThinkingBlocks(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	resp := gatewaycontract.CanonicalResponse{
		ID:         "resp_anthropic_thinking",
		Model:      "claude-local",
		StopReason: "end_turn",
		OutputItems: []gatewaycontract.ContentBlock{
			{Type: gatewaycontract.ContentBlockReasoning, Role: "assistant", Text: "private chain", Metadata: map[string]any{"signature": "sig_think_1"}},
			{Type: gatewaycontract.ContentBlockReasoning, Role: "assistant", Metadata: map[string]any{"type": "redacted_thinking", "data": "enc_think_1"}},
		},
		Usage: gatewaycontract.Usage{InputTokens: 3, OutputTokens: 4},
	}

	anthropic := svc.RenderAnthropicMessages(resp)
	if len(anthropic.Content) != 2 {
		t.Fatalf("expected thinking and redacted_thinking blocks, got %+v", anthropic.Content)
	}
	if anthropic.Content[0].Type != apiopenapi.AnthropicContentBlockTypeThinking {
		t.Fatalf("expected thinking block type, got %+v", anthropic.Content[0])
	}
	if thinking, _ := anthropic.Content[0].Get("thinking"); thinking != "private chain" {
		t.Fatalf("expected thinking payload, got %+v", anthropic.Content[0])
	}
	if signature, _ := anthropic.Content[0].Get("signature"); signature != "sig_think_1" {
		t.Fatalf("expected thinking signature, got %+v", anthropic.Content[0])
	}
	if anthropic.Content[1].Type != apiopenapi.AnthropicContentBlockTypeRedactedThinking {
		t.Fatalf("expected redacted_thinking block type, got %+v", anthropic.Content[1])
	}
	if data, _ := anthropic.Content[1].Get("data"); data != "enc_think_1" {
		t.Fatalf("expected redacted_thinking data, got %+v", anthropic.Content[1])
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

func TestRenderResponsesPreservesHostedWebSearchCall(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	resp := gatewaycontract.CanonicalResponse{
		ID:         "resp_web_search",
		Model:      "gpt-5.5",
		StopReason: "tool_use",
		OutputItems: []gatewaycontract.ContentBlock{{
			Type:              gatewaycontract.ContentBlockToolCall,
			Role:              "assistant",
			ToolCallID:        "ws_1",
			ToolName:          "web_search",
			ToolArgumentsJSON: `{"query":"latest OpenAI docs"}`,
			Metadata:          map[string]any{"type": "web_search_call"},
		}},
		Usage: gatewaycontract.Usage{InputTokens: 5, OutputTokens: 1},
	}

	responses := svc.RenderResponses(resp)
	if len(responses.Output) != 1 || responses.Output[0].Type != "web_search_call" {
		t.Fatalf("expected Responses web_search_call item, got %+v", responses.Output)
	}
	if _, ok := responses.Output[0].Get("arguments"); ok {
		t.Fatalf("did not expect web_search_call to expose function arguments, got %+v", responses.Output[0])
	}
	action, _ := responses.Output[0].Get("action")
	actionMap, _ := action.(map[string]any)
	if actionMap["type"] != "search" || actionMap["query"] != "latest OpenAI docs" {
		t.Fatalf("expected search action, got %+v", responses.Output[0])
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

func TestRenderResponsesStreamEventsPreservesHostedWebSearchCall(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	resp := gatewaycontract.CanonicalResponse{
		ID:         "resp_web_search_stream",
		Model:      "gpt-5.5",
		StopReason: "tool_use",
		OutputItems: []gatewaycontract.ContentBlock{{
			Type:              gatewaycontract.ContentBlockToolCall,
			Role:              "assistant",
			ToolCallID:        "ws_1",
			ToolName:          "web_search",
			ToolArgumentsJSON: `{"query":"latest OpenAI docs"}`,
			Metadata:          map[string]any{"type": "web_search_call"},
		}},
		StreamEvents: []gatewaycontract.StreamEvent{
			{
				Index:        0,
				Type:         gatewaycontract.StreamEventToolCallDelta,
				ContentIndex: 0,
				Delta: gatewaycontract.ContentBlock{
					Type:              gatewaycontract.ContentBlockToolCall,
					Role:              "assistant",
					ToolCallID:        "ws_1",
					ToolName:          "web_search",
					ToolArgumentsJSON: `{"query":"latest OpenAI docs"}`,
					Metadata:          map[string]any{"type": "web_search_call"},
				},
				OriginProtocol: "openai-compatible",
			},
			{
				Index:      1,
				Type:       gatewaycontract.StreamEventStop,
				StopReason: "tool_use",
			},
		},
		Usage: gatewaycontract.Usage{InputTokens: 5, OutputTokens: 1},
	}

	responsesEvents := svc.RenderResponsesStreamEvents(resp)
	added := streamEventByName(responsesEvents, "response.output_item.added")
	if added == nil {
		t.Fatalf("expected responses output item event, got %+v", responsesEvents)
	}
	item, _ := added.Data["item"].(map[string]any)
	if item["type"] != "web_search_call" || item["status"] != "in_progress" {
		t.Fatalf("expected in-progress web_search_call item, got %+v", item)
	}
	if argsEvents := streamEventsByName(responsesEvents, "response.function_call_arguments.delta"); len(argsEvents) != 0 {
		t.Fatalf("did not expect function argument deltas for web_search_call, got %+v", argsEvents)
	}
	done := streamEventByName(responsesEvents, "response.output_item.done")
	if done == nil {
		t.Fatalf("expected done event, got %+v", responsesEvents)
	}
	doneItem, _ := done.Data["item"].(map[string]any)
	action, _ := doneItem["action"].(map[string]any)
	if doneItem["type"] != "web_search_call" || action["query"] != "latest OpenAI docs" {
		t.Fatalf("expected completed web_search_call with action, got %+v", doneItem)
	}
}

func TestRenderCanonicalStreamEventsPreservesTextDeltas(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	resp := gatewaycontract.CanonicalResponse{
		ID:         "resp_delta_stream",
		Model:      "gpt-4o-mini",
		StopReason: "end_turn",
		OutputItems: []gatewaycontract.ContentBlock{{
			Type: gatewaycontract.ContentBlockText,
			Role: "assistant",
			Text: "hello stream",
		}},
		StreamEvents: []gatewaycontract.StreamEvent{
			{
				Index: 0,
				Type:  gatewaycontract.StreamEventContentDelta,
				Delta: gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockText, Role: "assistant", Text: "hello"},
			},
			{
				Index: 1,
				Type:  gatewaycontract.StreamEventContentDelta,
				Delta: gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockText, Role: "assistant", Text: " stream"},
			},
			{
				Index: 2,
				Type:  gatewaycontract.StreamEventUsage,
				Usage: gatewaycontract.Usage{InputTokens: 3, OutputTokens: 2},
			},
			{
				Index:      3,
				Type:       gatewaycontract.StreamEventStop,
				StopReason: "end_turn",
			},
		},
		Usage: gatewaycontract.Usage{InputTokens: 3, OutputTokens: 2},
	}

	chatChunks := svc.RenderChatStreamChunks(resp)
	if len(chatChunks) != 4 {
		t.Fatalf("expected four chat chunks, got %+v", chatChunks)
	}
	firstDelta := chatStreamContentDelta(t, chatChunks[0])
	secondDelta := chatStreamContentDelta(t, chatChunks[1])
	if firstDelta != "hello" || secondDelta != " stream" {
		t.Fatalf("expected preserved chat deltas, got %q and %q", firstDelta, secondDelta)
	}
	if choices, _ := chatChunks[2]["choices"].([]map[string]any); len(choices) != 0 || chatChunks[2]["usage"] == nil {
		t.Fatalf("expected usage-only chat stream chunk, got %+v", chatChunks[2])
	}

	responsesEvents := svc.RenderResponsesStreamEvents(resp)
	responseDeltas := streamEventsByName(responsesEvents, "response.output_text.delta")
	if len(responseDeltas) != 2 || responseDeltas[0].Data["delta"] != "hello" || responseDeltas[1].Data["delta"] != " stream" {
		t.Fatalf("expected preserved responses deltas, got %+v", responseDeltas)
	}

	anthropicEvents := svc.RenderAnthropicMessagesStreamEvents(resp)
	anthropicDeltas := streamEventsByName(anthropicEvents, "content_block_delta")
	if len(anthropicDeltas) != 2 {
		t.Fatalf("expected two anthropic deltas, got %+v", anthropicEvents)
	}
	firstAnthropicDelta, _ := anthropicDeltas[0].Data["delta"].(map[string]any)
	secondAnthropicDelta, _ := anthropicDeltas[1].Data["delta"].(map[string]any)
	if firstAnthropicDelta["text"] != "hello" || secondAnthropicDelta["text"] != " stream" {
		t.Fatalf("expected preserved anthropic deltas, got %+v and %+v", firstAnthropicDelta, secondAnthropicDelta)
	}

	geminiEvents := svc.RenderGeminiGenerateContentStreamEvents(resp)
	if len(geminiEvents) < 2 {
		t.Fatalf("expected Gemini delta events, got %+v", geminiEvents)
	}
	if got := geminiStreamText(t, geminiEvents[0]); got != "hello" {
		t.Fatalf("expected first Gemini delta, got %q", got)
	}
	if got := geminiStreamText(t, geminiEvents[1]); got != " stream" {
		t.Fatalf("expected second Gemini delta, got %q", got)
	}
}

func TestRenderCanonicalStreamEventsPreservesResponsesReasoningDeltas(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	resp := gatewaycontract.CanonicalResponse{
		ID:         "resp_reasoning_delta_stream",
		Model:      "gpt-4o-mini",
		StopReason: "end_turn",
		OutputItems: []gatewaycontract.ContentBlock{
			{Type: gatewaycontract.ContentBlockReasoning, Role: "assistant", Text: "think first"},
			{Type: gatewaycontract.ContentBlockText, Role: "assistant", Text: "answer"},
		},
		StreamEvents: []gatewaycontract.StreamEvent{
			{
				Index:        0,
				Type:         gatewaycontract.StreamEventReasoning,
				ContentIndex: 0,
				Delta:        gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockReasoning, Role: "assistant", Text: "think "},
			},
			{
				Index:        1,
				Type:         gatewaycontract.StreamEventReasoning,
				ContentIndex: 0,
				Delta:        gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockReasoning, Role: "assistant", Text: "first"},
			},
			{
				Index:        2,
				Type:         gatewaycontract.StreamEventContentDelta,
				ContentIndex: 1,
				Delta:        gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockText, Role: "assistant", Text: "answer"},
			},
			{
				Index:      3,
				Type:       gatewaycontract.StreamEventStop,
				StopReason: "end_turn",
			},
		},
		Usage: gatewaycontract.Usage{InputTokens: 3, OutputTokens: 4},
	}

	responsesEvents := svc.RenderResponsesStreamEvents(resp)
	reasoningDeltas := streamEventsByName(responsesEvents, "response.reasoning_text.delta")
	if len(reasoningDeltas) != 2 || reasoningDeltas[0].Data["delta"] != "think " || reasoningDeltas[1].Data["delta"] != "first" {
		t.Fatalf("expected preserved responses reasoning deltas, got %+v", reasoningDeltas)
	}
	if outputTextDeltas := streamEventsByName(responsesEvents, "response.output_text.delta"); len(outputTextDeltas) != 1 || outputTextDeltas[0].Data["delta"] != "answer" {
		t.Fatalf("expected reasoning to stay out of output_text deltas, got %+v", outputTextDeltas)
	}
	reasoningDone := streamEventByName(responsesEvents, "response.reasoning_text.done")
	if reasoningDone == nil || reasoningDone.Data["text"] != "think first" {
		t.Fatalf("expected completed responses reasoning text, got %+v", responsesEvents)
	}
	contentPartDone := streamEventsByName(responsesEvents, "response.content_part.done")
	if len(contentPartDone) != 2 {
		t.Fatalf("expected responses content part done events, got %+v", responsesEvents)
	}
	firstDonePart, _ := contentPartDone[0].Data["part"].(map[string]any)
	secondDonePart, _ := contentPartDone[1].Data["part"].(map[string]any)
	if firstDonePart["type"] != "reasoning_text" || secondDonePart["type"] != "output_text" {
		t.Fatalf("expected content part done to preserve part types, got %+v and %+v", firstDonePart, secondDonePart)
	}
	var reasoningPart map[string]any
	for _, event := range responsesEvents {
		if event.Event != "response.content_part.added" {
			continue
		}
		part, _ := event.Data["part"].(map[string]any)
		if part["type"] == "reasoning_text" {
			reasoningPart = part
			break
		}
	}
	if reasoningPart == nil {
		t.Fatalf("expected responses reasoning content part, got %+v", responsesEvents)
	}
	completed := svc.RenderResponses(resp)
	if len(completed.Output) == 0 || completed.Output[0].Content == nil || len(*completed.Output[0].Content) == 0 {
		t.Fatalf("expected completed responses content, got %+v", completed)
	}
	if (*completed.Output[0].Content)[0].Type != apiopenapi.ContentBlockType("reasoning_text") {
		t.Fatalf("expected final responses output to preserve reasoning_text, got %+v", (*completed.Output[0].Content)[0])
	}
}

func TestRenderResponsesStreamEventsPreservesTextContentIndexes(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	resp := gatewaycontract.CanonicalResponse{
		ID:         "resp_multi_text_stream",
		Model:      "gpt-4o-mini",
		StopReason: "end_turn",
		OutputItems: []gatewaycontract.ContentBlock{
			{Type: gatewaycontract.ContentBlockText, Role: "assistant", Text: "first block"},
			{Type: gatewaycontract.ContentBlockText, Role: "assistant", Text: "second block"},
		},
		StreamEvents: []gatewaycontract.StreamEvent{
			{
				Index:        0,
				Type:         gatewaycontract.StreamEventContentDelta,
				ContentIndex: 0,
				Delta:        gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockText, Role: "assistant", Text: "first "},
			},
			{
				Index:        1,
				Type:         gatewaycontract.StreamEventContentDelta,
				ContentIndex: 1,
				Delta:        gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockText, Role: "assistant", Text: "second "},
			},
			{
				Index:        2,
				Type:         gatewaycontract.StreamEventContentDelta,
				ContentIndex: 0,
				Delta:        gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockText, Role: "assistant", Text: "block"},
			},
			{
				Index:        3,
				Type:         gatewaycontract.StreamEventContentDelta,
				ContentIndex: 1,
				Delta:        gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockText, Role: "assistant", Text: "block"},
			},
			{
				Index:      4,
				Type:       gatewaycontract.StreamEventStop,
				StopReason: "end_turn",
			},
		},
		Usage: gatewaycontract.Usage{InputTokens: 4, OutputTokens: 4},
	}

	responsesEvents := svc.RenderResponsesStreamEvents(resp)
	added := streamEventsByName(responsesEvents, "response.output_item.added")
	if len(added) != 2 {
		t.Fatalf("expected two responses output items, got %+v", responsesEvents)
	}
	firstDone := streamEventByOutputIndex(responsesEvents, "response.output_item.done", 0)
	secondDone := streamEventByOutputIndex(responsesEvents, "response.output_item.done", 1)
	if firstDone == nil || secondDone == nil {
		t.Fatalf("expected output item done per content index, got %+v", responsesEvents)
	}
	if outputItemText(firstDone) != "first block" || outputItemText(secondDone) != "second block" {
		t.Fatalf("expected separate completed text blocks, got %+v and %+v", firstDone, secondDone)
	}
	deltas := streamEventsByName(responsesEvents, "response.output_text.delta")
	if len(deltas) != 4 || deltas[0].Data["output_index"] != 0 || deltas[1].Data["output_index"] != 1 || deltas[2].Data["output_index"] != 0 || deltas[3].Data["output_index"] != 1 {
		t.Fatalf("expected deltas to preserve output indexes, got %+v", deltas)
	}
}

func TestRenderResponsesStreamEventsFallbackPreservesReasoningPartLifecycle(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	resp := gatewaycontract.CanonicalResponse{
		ID:         "resp_reasoning_fallback_stream",
		Model:      "gpt-4o-mini",
		StopReason: "end_turn",
		OutputItems: []gatewaycontract.ContentBlock{{
			Type: gatewaycontract.ContentBlockReasoning,
			Role: "assistant",
			Text: "fallback thinking",
		}},
		Usage: gatewaycontract.Usage{InputTokens: 3, OutputTokens: 4},
	}

	responsesEvents := svc.RenderResponsesStreamEvents(resp)
	if outputDeltas := streamEventsByName(responsesEvents, "response.output_text.delta"); len(outputDeltas) != 0 {
		t.Fatalf("did not expect reasoning fallback as output_text delta, got %+v", outputDeltas)
	}
	reasoningDeltas := streamEventsByName(responsesEvents, "response.reasoning_text.delta")
	if len(reasoningDeltas) != 1 || reasoningDeltas[0].Data["delta"] != "fallback thinking" {
		t.Fatalf("expected reasoning fallback delta, got %+v", responsesEvents)
	}
	contentPartDone := streamEventByName(responsesEvents, "response.content_part.done")
	if contentPartDone == nil {
		t.Fatalf("expected fallback content part done event, got %+v", responsesEvents)
	}
	part, _ := contentPartDone.Data["part"].(map[string]any)
	if part["type"] != "reasoning_text" || part["text"] != "fallback thinking" {
		t.Fatalf("expected reasoning content part done payload, got %+v", part)
	}
	itemDone := streamEventByName(responsesEvents, "response.output_item.done")
	if itemDone == nil {
		t.Fatalf("expected fallback output item done event, got %+v", responsesEvents)
	}
	item, _ := itemDone.Data["item"].(map[string]any)
	content, _ := item["content"].([]map[string]any)
	if len(content) != 1 || content[0]["type"] != "reasoning_text" {
		t.Fatalf("expected fallback output item to preserve reasoning_text, got %+v", item)
	}
}

func TestRenderAnthropicStreamEventsPreservesThinkingSignatureDeltas(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	resp := gatewaycontract.CanonicalResponse{
		ID:         "resp_anthropic_thinking_stream",
		Model:      "claude-local",
		StopReason: "end_turn",
		OutputItems: []gatewaycontract.ContentBlock{{
			Type: gatewaycontract.ContentBlockReasoning,
			Role: "assistant",
			Text: "private chain",
		}},
		StreamEvents: []gatewaycontract.StreamEvent{
			{
				Index:        0,
				Type:         gatewaycontract.StreamEventReasoning,
				ContentIndex: 0,
				Delta:        gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockReasoning, Role: "assistant", Text: "private "},
			},
			{
				Index:        1,
				Type:         gatewaycontract.StreamEventReasoning,
				ContentIndex: 0,
				Delta:        gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockReasoning, Role: "assistant", Text: "chain"},
			},
			{
				Index:        2,
				Type:         gatewaycontract.StreamEventReasoning,
				ContentIndex: 0,
				Delta:        gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockReasoning, Role: "assistant", Metadata: map[string]any{"signature_delta": "sig_"}},
			},
			{
				Index:        3,
				Type:         gatewaycontract.StreamEventReasoning,
				ContentIndex: 0,
				Delta:        gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockReasoning, Role: "assistant", Metadata: map[string]any{"signature_delta": "think"}},
			},
		},
		Usage: gatewaycontract.Usage{InputTokens: 3, OutputTokens: 4},
	}

	events := svc.RenderAnthropicMessagesStreamEvents(resp)
	blockStart := streamEventByName(events, "content_block_start")
	if blockStart == nil {
		t.Fatalf("expected thinking block start, got %+v", events)
	}
	block, _ := blockStart.Data["content_block"].(map[string]any)
	if block["type"] != "thinking" {
		t.Fatalf("expected thinking block start, got %+v", block)
	}
	deltas := streamEventsByName(events, "content_block_delta")
	if len(deltas) != 4 {
		t.Fatalf("expected two thinking and two signature deltas, got %+v", deltas)
	}
	firstDelta, _ := deltas[0].Data["delta"].(map[string]any)
	thirdDelta, _ := deltas[2].Data["delta"].(map[string]any)
	fourthDelta, _ := deltas[3].Data["delta"].(map[string]any)
	if firstDelta["type"] != "thinking_delta" || firstDelta["text"] != "private " {
		t.Fatalf("expected thinking text delta, got %+v", firstDelta)
	}
	if thirdDelta["type"] != "signature_delta" || thirdDelta["signature"] != "sig_" {
		t.Fatalf("expected first signature delta, got %+v", thirdDelta)
	}
	if fourthDelta["type"] != "signature_delta" || fourthDelta["signature"] != "sig_think" {
		t.Fatalf("expected aggregated signature delta, got %+v", fourthDelta)
	}
}

func TestRenderCanonicalStreamEventsPreservesToolCallDeltas(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	resp := gatewaycontract.CanonicalResponse{
		ID:         "resp_tool_delta_stream",
		Model:      "gpt-4o-mini",
		StopReason: "tool_use",
		OutputItems: []gatewaycontract.ContentBlock{{
			Type:              gatewaycontract.ContentBlockToolCall,
			Role:              "assistant",
			ToolCallID:        "call_1",
			ToolName:          "lookup",
			ToolArgumentsJSON: `{"query":"weather"}`,
		}},
		StreamEvents: []gatewaycontract.StreamEvent{
			{
				Index:        0,
				Type:         gatewaycontract.StreamEventToolCallDelta,
				ContentIndex: 0,
				Delta: gatewaycontract.ContentBlock{
					Type:              gatewaycontract.ContentBlockToolCall,
					Role:              "assistant",
					ToolCallID:        "call_1",
					ToolName:          "lookup",
					ToolArgumentsJSON: `{"query":`,
				},
				OriginProtocol: "openai-compatible",
			},
			{
				Index:        1,
				Type:         gatewaycontract.StreamEventToolCallDelta,
				ContentIndex: 0,
				Delta: gatewaycontract.ContentBlock{
					Type:              gatewaycontract.ContentBlockToolCall,
					Role:              "assistant",
					ToolArgumentsJSON: `"weather"}`,
				},
				OriginProtocol: "openai-compatible",
			},
			{
				Index:      2,
				Type:       gatewaycontract.StreamEventStop,
				StopReason: "tool_use",
			},
		},
		Usage: gatewaycontract.Usage{InputTokens: 3, OutputTokens: 1},
	}

	chatChunks := svc.RenderChatStreamChunks(resp)
	if len(chatChunks) != 3 {
		t.Fatalf("expected two tool chunks plus stop, got %+v", chatChunks)
	}
	firstTool := chatStreamToolDelta(t, chatChunks[0])
	secondTool := chatStreamToolDelta(t, chatChunks[1])
	firstFunction, _ := firstTool["function"].(map[string]any)
	secondFunction, _ := secondTool["function"].(map[string]any)
	if firstTool["id"] != "call_1" || firstFunction["name"] != "lookup" || firstFunction["arguments"] != `{"query":` {
		t.Fatalf("expected first chat tool delta, got %+v", firstTool)
	}
	if secondFunction["arguments"] != `"weather"}` {
		t.Fatalf("expected second chat tool arguments delta, got %+v", secondTool)
	}

	responsesEvents := svc.RenderResponsesStreamEvents(resp)
	responsesDeltas := streamEventsByName(responsesEvents, "response.function_call_arguments.delta")
	if len(responsesDeltas) != 2 || responsesDeltas[0].Data["delta"] != `{"query":` || responsesDeltas[1].Data["delta"] != `"weather"}` {
		t.Fatalf("expected preserved responses tool argument deltas, got %+v", responsesDeltas)
	}
	responsesDone := streamEventByName(responsesEvents, "response.function_call_arguments.done")
	if responsesDone == nil || responsesDone.Data["arguments"] != `{"query":"weather"}` {
		t.Fatalf("expected completed responses arguments, got %+v", responsesEvents)
	}

	anthropicEvents := svc.RenderAnthropicMessagesStreamEvents(resp)
	blockStart := streamEventByName(anthropicEvents, "content_block_start")
	if blockStart == nil {
		t.Fatalf("expected anthropic tool block start, got %+v", anthropicEvents)
	}
	contentBlock, _ := blockStart.Data["content_block"].(map[string]any)
	if contentBlock["type"] != "tool_use" || contentBlock["id"] != "call_1" || contentBlock["name"] != "lookup" {
		t.Fatalf("expected anthropic tool block identity, got %+v", contentBlock)
	}
	anthropicDeltas := streamEventsByName(anthropicEvents, "content_block_delta")
	if len(anthropicDeltas) != 2 {
		t.Fatalf("expected anthropic tool argument deltas, got %+v", anthropicEvents)
	}
	firstAnthropicDelta, _ := anthropicDeltas[0].Data["delta"].(map[string]any)
	secondAnthropicDelta, _ := anthropicDeltas[1].Data["delta"].(map[string]any)
	if firstAnthropicDelta["partial_json"] != `{"query":` || secondAnthropicDelta["partial_json"] != `"weather"}` {
		t.Fatalf("expected preserved anthropic partial json deltas, got %+v and %+v", firstAnthropicDelta, secondAnthropicDelta)
	}

	geminiEvents := svc.RenderGeminiGenerateContentStreamEvents(resp)
	if len(geminiEvents) != 1 || len(geminiEvents[0].Data) == 0 {
		t.Fatalf("expected Gemini to fall back to final render for invalid partial JSON, got %+v", geminiEvents)
	}
}

func TestRenderCanonicalStreamEventsPreservesResponsesStyleToolCalls(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	resp := gatewaycontract.CanonicalResponse{
		ID:         "resp_responses_tool_stream",
		Model:      "gpt-4o-mini",
		StopReason: "tool_use",
		OutputItems: []gatewaycontract.ContentBlock{{
			Type:              gatewaycontract.ContentBlockToolCall,
			Role:              "assistant",
			ToolCallID:        "call_1",
			ToolName:          "lookup",
			ToolArgumentsJSON: `{"query":"weather"}`,
		}},
		StreamEvents: []gatewaycontract.StreamEvent{
			{
				Index:        0,
				Type:         gatewaycontract.StreamEventToolCallDelta,
				ContentIndex: 0,
				Delta: gatewaycontract.ContentBlock{
					Type:              gatewaycontract.ContentBlockToolCall,
					Role:              "assistant",
					ToolCallID:        "call_1",
					ToolName:          "lookup",
					ToolArgumentsJSON: `{"query":"weather"}`,
				},
				OriginProtocol: "openai-compatible",
				RawEventType:   "response.output_item.done",
			},
			{
				Index:      1,
				Type:       gatewaycontract.StreamEventStop,
				StopReason: "tool_use",
			},
		},
		Usage: gatewaycontract.Usage{InputTokens: 4, OutputTokens: 2},
	}

	responsesEvents := svc.RenderResponsesStreamEvents(resp)
	added := streamEventByName(responsesEvents, "response.output_item.added")
	argsDelta := streamEventByName(responsesEvents, "response.function_call_arguments.delta")
	argsDone := streamEventByName(responsesEvents, "response.function_call_arguments.done")
	if added == nil || argsDelta == nil || argsDone == nil {
		t.Fatalf("expected responses function call stream events, got %+v", responsesEvents)
	}
	item, _ := added.Data["item"].(map[string]any)
	if item["type"] != "function_call" || item["call_id"] != "call_1" || item["name"] != "lookup" {
		t.Fatalf("expected responses function call identity, got %+v", item)
	}
	if argsDelta.Data["delta"] != `{"query":"weather"}` || argsDone.Data["arguments"] != `{"query":"weather"}` {
		t.Fatalf("expected responses function call arguments, got delta=%+v done=%+v", argsDelta, argsDone)
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

func streamEventsByName(events []StreamEvent, name string) []StreamEvent {
	out := make([]StreamEvent, 0)
	for _, event := range events {
		if event.Event == name {
			out = append(out, event)
		}
	}
	return out
}

func streamEventByOutputIndex(events []StreamEvent, name string, outputIndex int) *StreamEvent {
	for idx := range events {
		if events[idx].Event == name && events[idx].Data["output_index"] == outputIndex {
			return &events[idx]
		}
	}
	return nil
}

func outputItemText(event *StreamEvent) string {
	if event == nil {
		return ""
	}
	item, _ := event.Data["item"].(map[string]any)
	content, _ := item["content"].([]map[string]any)
	if len(content) == 0 {
		return ""
	}
	text, _ := content[0]["text"].(string)
	return text
}

func chatStreamToolDelta(t *testing.T, chunk map[string]any) map[string]any {
	t.Helper()
	choices, _ := chunk["choices"].([]map[string]any)
	if len(choices) != 1 {
		t.Fatalf("expected one chat choice, got %+v", chunk)
	}
	delta, _ := choices[0]["delta"].(map[string]any)
	toolCalls, _ := delta["tool_calls"].([]map[string]any)
	if len(toolCalls) != 1 {
		t.Fatalf("expected one tool call delta, got %+v", delta)
	}
	return toolCalls[0]
}

func chatStreamContentDelta(t *testing.T, chunk map[string]any) string {
	t.Helper()
	choices, _ := chunk["choices"].([]map[string]any)
	if len(choices) != 1 {
		t.Fatalf("expected one chat choice, got %+v", chunk)
	}
	delta, _ := choices[0]["delta"].(map[string]any)
	text, _ := delta["content"].(string)
	return text
}

func geminiStreamText(t *testing.T, event StreamEvent) string {
	t.Helper()
	candidates, _ := event.Data["candidates"].([]apiopenapi.GeminiCandidate)
	if len(candidates) != 1 || len(candidates[0].Content.Parts) != 1 || candidates[0].Content.Parts[0].Text == nil {
		t.Fatalf("expected Gemini text candidate, got %+v", event)
	}
	return *candidates[0].Content.Parts[0].Text
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
