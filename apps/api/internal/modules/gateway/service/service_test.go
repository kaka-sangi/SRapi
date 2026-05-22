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
	resp := svc.BuildTextResponse("gpt-4o-mini", "gpt-4o-mini", "hello back", []string{"tools_ignored"})

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
