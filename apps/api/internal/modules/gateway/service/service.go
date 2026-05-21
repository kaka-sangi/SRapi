package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

type Service struct{}

func New() (*Service, error) {
	return &Service{}, nil
}

type StreamEvent struct {
	Event string
	Data  map[string]any
}

func (s *Service) NormalizeChatCompletions(req apiopenapi.ChatCompletionRequest, meta RequestMeta) gatewaycontract.CanonicalRequest {
	var parts []string
	var warnings []string
	var messages []gatewaycontract.Message
	for _, msg := range req.Messages {
		role := string(msg.Role)
		if chatMessageHasImage(msg.Content) {
			warnings = append(warnings, "vision_ignored")
		}
		blocks := chatContentBlocks(msg.Content)
		if len(blocks) > 0 {
			messages = append(messages, gatewaycontract.Message{Role: role, Content: blocks})
		}
		text := textFromBlocks(blocks)
		if text != "" {
			parts = append(parts, role+": "+text)
		}
	}
	canonical := canonical(meta, gatewaycontract.ProtocolOpenAICompatible, gatewaycontract.ProtocolOpenAICompatible, req.Model, "", req.Stream != nil && *req.Stream, strings.Join(parts, "\n"), messages, nil, "", uniqueStrings(warnings))
	canonical.Temperature = req.Temperature
	canonical.TopP = req.TopP
	canonical.MaxOutputTokens = cloneInt(req.MaxTokens)
	canonical.Stop = chatStopStrings(req.Stop)
	canonical.Tools = toolDefinitionsToMaps(req.Tools)
	canonical.ToolChoice = chatToolChoice(req.ToolChoice)
	canonical.ResponseFormat = cloneJSONMap(req.ResponseFormat)
	canonical.CompatibilityWarnings = uniqueStrings(warnings)
	refreshRequestCapabilities(&canonical)
	return canonical
}

func (s *Service) NormalizeResponses(req apiopenapi.ResponsesRequest, meta RequestMeta) gatewaycontract.CanonicalRequest {
	var warnings []string
	var parts []string
	var inputItems []gatewaycontract.ContentBlock
	if value, err := req.Input.AsResponsesRequestInput0(); err == nil {
		text := strings.TrimSpace(value)
		if text != "" {
			inputItems = append(inputItems, gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockText, Role: "user", Text: text})
			parts = append(parts, text)
		}
	} else if blocks, err := req.Input.AsResponsesRequestInput1(); err == nil {
		if contentBlocksHaveImage(blocks) {
			warnings = append(warnings, "vision_ignored")
		}
		inputItems = append(inputItems, openAIContentBlocks(blocks, "user")...)
		parts = append(parts, extractContentBlocksText(blocks))
	}
	instructions := ""
	if req.Instructions != nil {
		instructions = strings.TrimSpace(*req.Instructions)
		if instructions != "" {
			parts = append([]string{"instructions: " + instructions}, parts...)
		}
	}
	canonical := canonical(meta, gatewaycontract.ProtocolOpenAICompatible, gatewaycontract.ProtocolOpenAICompatible, req.Model, "", req.Stream != nil && *req.Stream, strings.Join(parts, "\n"), nil, inputItems, instructions, uniqueStrings(warnings))
	canonical.Temperature = req.Temperature
	canonical.TopP = req.TopP
	canonical.MaxOutputTokens = cloneInt(req.MaxOutputTokens)
	canonical.Tools = toolDefinitionsToMaps(req.Tools)
	canonical.ResponseFormat = responseFormatFromResponsesText(req.Text)
	canonical.Reasoning = cloneJSONMap(req.Reasoning)
	if len(canonical.Reasoning) > 0 {
		warnings = append(warnings, "reasoning_ignored")
	}
	canonical.CompatibilityWarnings = uniqueStrings(warnings)
	refreshRequestCapabilities(&canonical)
	return canonical
}

func (s *Service) NormalizeAnthropicMessages(req apiopenapi.AnthropicMessagesRequest, meta RequestMeta) gatewaycontract.CanonicalRequest {
	var warnings []string
	var parts []string
	var messages []gatewaycontract.Message
	instructions := ""
	if req.System != nil {
		if value, err := req.System.AsAnthropicMessagesRequestSystem0(); err == nil {
			instructions = strings.TrimSpace(value)
			if instructions != "" {
				parts = append(parts, "system: "+instructions)
			}
		} else if blocks, err := req.System.AsAnthropicMessagesRequestSystem1(); err == nil {
			if anthropicContentBlocksHaveImage(blocks) {
				warnings = append(warnings, "vision_ignored")
			}
			instructions = extractAnthropicContentBlocksText(blocks)
			if instructions != "" {
				parts = append(parts, "system: "+instructions)
			}
		}
	}
	for _, msg := range req.Messages {
		role := string(msg.Role)
		if anthropicMessageHasImage(msg.Content) {
			warnings = append(warnings, "vision_ignored")
		}
		blocks := anthropicMessageBlocks(msg.Content, role)
		if len(blocks) > 0 {
			messages = append(messages, gatewaycontract.Message{Role: role, Content: blocks})
		}
		text := textFromBlocks(blocks)
		if text != "" {
			parts = append(parts, role+": "+text)
		}
	}
	canonical := canonical(meta, gatewaycontract.ProtocolAnthropicCompatible, gatewaycontract.ProtocolAnthropicCompatible, req.Model, "", req.Stream != nil && *req.Stream, strings.Join(parts, "\n"), messages, nil, instructions, uniqueStrings(warnings))
	canonical.Temperature = req.Temperature
	canonical.TopP = req.TopP
	canonical.MaxOutputTokens = positiveIntPtr(req.MaxTokens)
	canonical.Tools = anthropicToolsToOpenAITools(req.Tools)
	canonical.ToolChoice = anthropicToolChoice(req.ToolChoice)
	canonical.Reasoning = cloneJSONMap(req.Thinking)
	if len(canonical.Reasoning) > 0 {
		warnings = append(warnings, "thinking_ignored")
	}
	canonical.CompatibilityWarnings = uniqueStrings(warnings)
	refreshRequestCapabilities(&canonical)
	return canonical
}

func (s *Service) BuildTextResponse(model, canonicalModel, text string, warnings []string) gatewaycontract.CanonicalResponse {
	return s.buildTextResponse("", model, canonicalModel, text, estimateUsage(text), warnings)
}

func (s *Service) BuildCanonicalTextResponse(req gatewaycontract.CanonicalRequest, text string, usage gatewaycontract.Usage) gatewaycontract.CanonicalResponse {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = req.CanonicalModel
	}
	canonicalModel := strings.TrimSpace(req.CanonicalModel)
	if canonicalModel == "" {
		canonicalModel = model
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.CachedTokens == 0 {
		usage = estimateUsage(text)
	}
	return s.buildTextResponse(req.RequestID, model, canonicalModel, text, usage, req.CompatibilityWarnings)
}

func (s *Service) buildTextResponse(requestID, model, canonicalModel, text string, usage gatewaycontract.Usage, warnings []string) gatewaycontract.CanonicalResponse {
	return gatewaycontract.CanonicalResponse{
		ID:             randomHexString(12),
		RequestID:      strings.TrimSpace(requestID),
		Model:          model,
		CanonicalModel: canonicalModel,
		Message:        text,
		OutputItems: []gatewaycontract.ContentBlock{{
			Type: gatewaycontract.ContentBlockText,
			Role: "assistant",
			Text: text,
		}},
		Usage:                 usage,
		CompatibilityWarnings: uniqueStrings(warnings),
	}
}

func (s *Service) RenderChatCompletions(resp gatewaycontract.CanonicalResponse) apiopenapi.ChatCompletionResponse {
	now := time.Now().UTC()
	systemFingerprint := "srapi"
	msg := apiopenapi.ChatMessage{}
	_ = msg.Content.FromChatMessageContent0(resp.Message)
	msg.Role = apiopenapi.ChatMessageRoleAssistant
	return apiopenapi.ChatCompletionResponse{
		Choices: []apiopenapi.ChatCompletionChoice{{
			Index:        0,
			FinishReason: ptrString("stop"),
			Message:      msg,
		}},
		Created:           int(now.Unix()),
		Id:                "chatcmpl_" + responseID(resp),
		Model:             resp.Model,
		Object:            apiopenapi.ChatCompletionResponseObject("chat.completion"),
		SystemFingerprint: &systemFingerprint,
		Usage:             tokenUsage(resp.Usage),
	}
}

func (s *Service) RenderResponses(resp gatewaycontract.CanonicalResponse) apiopenapi.ResponsesResponse {
	now := time.Now().UTC()
	role := "assistant"
	text := resp.Message
	content := []apiopenapi.ContentBlock{{Type: apiopenapi.ContentBlockTypeText, Text: &text}}
	status := "completed"
	rendered := apiopenapi.ResponsesResponse{
		CreatedAt: int(now.Unix()),
		Id:        "resp_" + responseID(resp),
		Model:     resp.Model,
		Object:    apiopenapi.Response,
		Output: []apiopenapi.ResponsesOutputItem{{
			Type:    "message",
			Role:    &role,
			Content: &content,
		}},
		Status: &status,
		Usage:  tokenUsage(resp.Usage),
	}
	if len(resp.CompatibilityWarnings) > 0 {
		warnings := append([]string(nil), resp.CompatibilityWarnings...)
		rendered.CompatibilityWarnings = &warnings
	}
	return rendered
}

func (s *Service) RenderAnthropicMessages(resp gatewaycontract.CanonicalResponse) apiopenapi.AnthropicMessagesResponse {
	text := resp.Message
	stopReason := "end_turn"
	rendered := apiopenapi.AnthropicMessagesResponse{
		Content:    []apiopenapi.AnthropicContentBlock{{Type: apiopenapi.AnthropicContentBlockTypeText, Text: &text}},
		Id:         "msg_" + responseID(resp),
		Model:      resp.Model,
		Role:       apiopenapi.AnthropicMessagesResponseRoleAssistant,
		StopReason: &stopReason,
		Type:       apiopenapi.Message,
		Usage:      anthropicUsage(resp.Usage),
	}
	if len(resp.CompatibilityWarnings) > 0 {
		warnings := append([]string(nil), resp.CompatibilityWarnings...)
		rendered.CompatibilityWarnings = &warnings
	}
	return rendered
}

func (s *Service) RenderChatStreamChunk(resp gatewaycontract.CanonicalResponse) map[string]any {
	chunk := map[string]any{
		"id":      "chatcmpl_" + responseID(resp),
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   resp.Model,
		"choices": []map[string]any{
			{
				"index": 0,
				"delta": map[string]any{
					"role":    "assistant",
					"content": resp.Message,
				},
				"finish_reason": "stop",
			},
		},
	}
	if len(resp.CompatibilityWarnings) > 0 {
		chunk["compatibility_warnings"] = append([]string(nil), resp.CompatibilityWarnings...)
	}
	return chunk
}

func (s *Service) RenderResponsesStreamEvents(resp gatewaycontract.CanonicalResponse) []StreamEvent {
	id := "resp_" + responseID(resp)
	completed := s.RenderResponses(resp)
	createdAt := time.Now().Unix()
	created := map[string]any{
		"id":         id,
		"object":     "response",
		"created_at": createdAt,
		"model":      resp.Model,
		"status":     "in_progress",
	}
	if len(resp.CompatibilityWarnings) > 0 {
		created["compatibility_warnings"] = append([]string(nil), resp.CompatibilityWarnings...)
	}
	return []StreamEvent{
		{
			Event: "response.created",
			Data: map[string]any{
				"type":     "response.created",
				"response": created,
			},
		},
		{
			Event: "response.output_text.delta",
			Data: map[string]any{
				"type":          "response.output_text.delta",
				"item_id":       "msg_0",
				"output_index":  0,
				"content_index": 0,
				"delta":         resp.Message,
			},
		},
		{
			Event: "response.output_text.done",
			Data: map[string]any{
				"type":          "response.output_text.done",
				"item_id":       "msg_0",
				"output_index":  0,
				"content_index": 0,
				"text":          resp.Message,
			},
		},
		{
			Event: "response.completed",
			Data: map[string]any{
				"type":     "response.completed",
				"response": completed,
			},
		},
	}
}

func (s *Service) RenderAnthropicMessagesStreamEvents(resp gatewaycontract.CanonicalResponse) []StreamEvent {
	id := "msg_" + responseID(resp)
	return []StreamEvent{
		{
			Event: "message_start",
			Data: map[string]any{
				"type": "message_start",
				"message": map[string]any{
					"id":            id,
					"type":          "message",
					"role":          "assistant",
					"model":         resp.Model,
					"content":       []any{},
					"stop_reason":   nil,
					"stop_sequence": nil,
					"usage": map[string]any{
						"input_tokens":  resp.Usage.InputTokens,
						"output_tokens": 0,
					},
				},
			},
		},
		{
			Event: "content_block_start",
			Data: map[string]any{
				"type":  "content_block_start",
				"index": 0,
				"content_block": map[string]any{
					"type": "text",
					"text": "",
				},
			},
		},
		{
			Event: "content_block_delta",
			Data: map[string]any{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]any{
					"type": "text_delta",
					"text": resp.Message,
				},
			},
		},
		{
			Event: "content_block_stop",
			Data: map[string]any{
				"type":  "content_block_stop",
				"index": 0,
			},
		},
		{
			Event: "message_delta",
			Data: map[string]any{
				"type": "message_delta",
				"delta": map[string]any{
					"stop_reason":   "end_turn",
					"stop_sequence": nil,
				},
				"usage": map[string]any{
					"output_tokens": resp.Usage.OutputTokens,
				},
			},
		},
		{
			Event: "message_stop",
			Data: map[string]any{
				"type": "message_stop",
			},
		},
	}
}

func CapabilityDescriptors(req gatewaycontract.CanonicalRequest) []capabilitiescontract.Descriptor {
	values := make([]capabilitiescontract.Descriptor, 0, len(req.RequestCapabilities))
	for _, capability := range req.RequestCapabilities {
		key := strings.TrimSpace(capability.Key)
		if key == "" {
			continue
		}
		version := strings.TrimSpace(capability.Version)
		if version == "" {
			version = "v1"
		}
		values = append(values, capabilitiescontract.Descriptor{
			Key:     key,
			Level:   capabilitiescontract.DescriptorLevelRequired,
			Status:  capabilitiescontract.DescriptorStatusStable,
			Version: version,
		})
	}
	return dedupeCapabilityDescriptors(values)
}

type RequestMeta struct {
	RequestID      string
	SourceEndpoint string
	UserID         int
	APIKeyID       int
	CanonicalModel string
}

func canonical(meta RequestMeta, sourceProtocol, responseProtocol gatewaycontract.Protocol, model, canonicalModel string, stream bool, prompt string, messages []gatewaycontract.Message, inputItems []gatewaycontract.ContentBlock, instructions string, warnings []string) gatewaycontract.CanonicalRequest {
	if canonicalModel == "" {
		canonicalModel = meta.CanonicalModel
	}
	if canonicalModel == "" {
		canonicalModel = model
	}
	req := gatewaycontract.CanonicalRequest{
		RequestID:             strings.TrimSpace(meta.RequestID),
		SourceProtocol:        sourceProtocol,
		SourceEndpoint:        strings.TrimSpace(meta.SourceEndpoint),
		ResponseProtocol:      responseProtocol,
		UserID:                meta.UserID,
		APIKeyID:              meta.APIKeyID,
		Model:                 strings.TrimSpace(model),
		CanonicalModel:        strings.TrimSpace(canonicalModel),
		InputItems:            inputItems,
		Messages:              messages,
		Instructions:          strings.TrimSpace(instructions),
		Stream:                stream,
		Prompt:                strings.TrimSpace(prompt),
		CompatibilityWarnings: warnings,
	}
	refreshRequestCapabilities(&req)
	return req
}

func chatContentBlocks(content apiopenapi.ChatMessage_Content) []gatewaycontract.ContentBlock {
	if text, err := content.AsChatMessageContent0(); err == nil {
		text = strings.TrimSpace(text)
		if text == "" {
			return nil
		}
		return []gatewaycontract.ContentBlock{{Type: gatewaycontract.ContentBlockText, Text: text}}
	}
	if blocks, err := content.AsChatMessageContent1(); err == nil {
		return openAIContentBlocks(blocks, "")
	}
	return nil
}

func openAIContentBlocks(blocks []apiopenapi.ContentBlock, role string) []gatewaycontract.ContentBlock {
	out := make([]gatewaycontract.ContentBlock, 0, len(blocks))
	for _, block := range blocks {
		if block.Text != nil {
			text := strings.TrimSpace(*block.Text)
			if text != "" {
				out = append(out, gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockText, Role: role, Text: text})
			}
		}
		if block.ImageUrl != nil || block.Type == apiopenapi.ContentBlockTypeImageUrl || block.Type == apiopenapi.ContentBlockTypeInputImage {
			out = append(out, gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockImage, Role: role, Text: "[image]", Metadata: jsonObjectToMap(block.ImageUrl)})
		}
	}
	return out
}

func anthropicMessageBlocks(content apiopenapi.AnthropicMessage_Content, role string) []gatewaycontract.ContentBlock {
	if text, err := content.AsAnthropicMessageContent0(); err == nil {
		text = strings.TrimSpace(text)
		if text == "" {
			return nil
		}
		return []gatewaycontract.ContentBlock{{Type: gatewaycontract.ContentBlockText, Role: role, Text: text}}
	}
	if blocks, err := content.AsAnthropicMessageContent1(); err == nil {
		return anthropicContentBlocks(blocks, role)
	}
	return nil
}

func anthropicContentBlocks(blocks []apiopenapi.AnthropicContentBlock, role string) []gatewaycontract.ContentBlock {
	out := make([]gatewaycontract.ContentBlock, 0, len(blocks))
	for _, block := range blocks {
		if block.Text != nil {
			text := strings.TrimSpace(*block.Text)
			if text != "" {
				out = append(out, gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockText, Role: role, Text: text})
			}
			continue
		}
		if block.Type == apiopenapi.AnthropicContentBlockTypeImage {
			out = append(out, gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockImage, Role: role, Text: "[image]"})
		}
	}
	return out
}

func textFromBlocks(blocks []gatewaycontract.ContentBlock) string {
	var parts []string
	for _, block := range blocks {
		text := strings.TrimSpace(block.Text)
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func chatMessageHasImage(content apiopenapi.ChatMessage_Content) bool {
	blocks, err := content.AsChatMessageContent1()
	return err == nil && contentBlocksHaveImage(blocks)
}

func contentBlocksHaveImage(blocks []apiopenapi.ContentBlock) bool {
	for _, block := range blocks {
		if block.ImageUrl != nil || block.Type == apiopenapi.ContentBlockTypeImageUrl || block.Type == apiopenapi.ContentBlockTypeInputImage {
			return true
		}
	}
	return false
}

func anthropicMessageHasImage(content apiopenapi.AnthropicMessage_Content) bool {
	blocks, err := content.AsAnthropicMessageContent1()
	return err == nil && anthropicContentBlocksHaveImage(blocks)
}

func anthropicContentBlocksHaveImage(blocks []apiopenapi.AnthropicContentBlock) bool {
	for _, block := range blocks {
		if block.Type == apiopenapi.AnthropicContentBlockTypeImage {
			return true
		}
	}
	return false
}

func extractContentBlocksText(blocks []apiopenapi.ContentBlock) string {
	return textFromBlocks(openAIContentBlocks(blocks, ""))
}

func extractAnthropicContentBlocksText(blocks []apiopenapi.AnthropicContentBlock) string {
	return textFromBlocks(anthropicContentBlocks(blocks, ""))
}

func refreshRequestCapabilities(req *gatewaycontract.CanonicalRequest) {
	if req == nil {
		return
	}
	req.RequestCapabilities = requestCapabilities(*req)
}

func requestCapabilities(req gatewaycontract.CanonicalRequest) []gatewaycontract.RequestCapability {
	out := make([]gatewaycontract.RequestCapability, 0, len(req.CompatibilityWarnings)+4)
	if req.Stream {
		out = append(out, gatewaycontract.RequestCapability{Key: capabilitiescontract.KeyStreaming, Version: "v1"})
	}
	if len(req.Tools) > 0 || req.ToolChoice != nil {
		out = append(out, gatewaycontract.RequestCapability{Key: capabilitiescontract.KeyToolCalling, Version: "v1"})
	}
	if len(req.ResponseFormat) > 0 {
		out = append(out, gatewaycontract.RequestCapability{Key: capabilitiescontract.KeyStructuredOutput, Version: "v1"})
	}
	if len(req.Reasoning) > 0 {
		out = append(out, gatewaycontract.RequestCapability{Key: capabilitiescontract.KeyReasoningControl, Version: "v1"})
	}
	for _, warning := range req.CompatibilityWarnings {
		switch warning {
		case "tools_ignored", "tool_choice_ignored":
			out = append(out, gatewaycontract.RequestCapability{Key: capabilitiescontract.KeyToolCalling, Version: "v1"})
		case "response_format_ignored", "text_object_ignored":
			out = append(out, gatewaycontract.RequestCapability{Key: capabilitiescontract.KeyStructuredOutput, Version: "v1"})
		case "reasoning_ignored", "thinking_ignored":
			out = append(out, gatewaycontract.RequestCapability{Key: capabilitiescontract.KeyReasoningControl, Version: "v1"})
		case "vision_ignored":
			out = append(out, gatewaycontract.RequestCapability{Key: capabilitiescontract.KeyVisionInput, Version: "v1"})
		}
	}
	return dedupeRequestCapabilities(out)
}

func chatStopStrings(value *apiopenapi.ChatCompletionRequest_Stop) []string {
	if value == nil {
		return nil
	}
	if single, err := value.AsChatCompletionRequestStop0(); err == nil {
		single = strings.TrimSpace(single)
		if single == "" {
			return nil
		}
		return []string{single}
	}
	if values, err := value.AsChatCompletionRequestStop1(); err == nil {
		out := make([]string, 0, len(values))
		for _, item := range values {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	}
	return nil
}

func toolDefinitionsToMaps(values *[]apiopenapi.ToolDefinition) []map[string]any {
	if values == nil || len(*values) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(*values))
	for _, value := range *values {
		tool := map[string]any{"type": strings.TrimSpace(value.Type)}
		if tool["type"] == "" {
			tool["type"] = "function"
		}
		if value.Function != nil {
			tool["function"] = cloneMap(*value.Function)
		}
		for key, item := range value.AdditionalProperties {
			if _, exists := tool[key]; !exists {
				tool[key] = cloneAny(item)
			}
		}
		out = append(out, tool)
	}
	return out
}

func chatToolChoice(value *apiopenapi.ChatCompletionRequest_ToolChoice) any {
	if value == nil {
		return nil
	}
	if choice, err := value.AsChatCompletionRequestToolChoice0(); err == nil {
		choice = strings.TrimSpace(choice)
		if choice == "" {
			return nil
		}
		return choice
	}
	if choice, err := value.AsJsonObject(); err == nil {
		return cloneMap(choice)
	}
	return nil
}

func responseFormatFromResponsesText(value *apiopenapi.JsonObject) map[string]any {
	raw := cloneJSONMap(value)
	if len(raw) == 0 {
		return nil
	}
	if format, ok := raw["format"]; ok {
		if typed, ok := format.(map[string]any); ok {
			return typed
		}
	}
	return raw
}

func anthropicToolsToOpenAITools(values *[]apiopenapi.JsonObject) []map[string]any {
	if values == nil || len(*values) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(*values))
	for _, value := range *values {
		tool := map[string]any{"type": "function"}
		function := map[string]any{}
		if name, ok := value["name"].(string); ok && strings.TrimSpace(name) != "" {
			function["name"] = strings.TrimSpace(name)
		}
		if description, ok := value["description"].(string); ok && strings.TrimSpace(description) != "" {
			function["description"] = strings.TrimSpace(description)
		}
		if schema, ok := value["input_schema"]; ok {
			function["parameters"] = cloneAny(schema)
		}
		if len(function) > 0 {
			tool["function"] = function
		}
		out = append(out, tool)
	}
	return out
}

func anthropicToolChoice(value *apiopenapi.JsonObject) any {
	raw := cloneJSONMap(value)
	if len(raw) == 0 {
		return nil
	}
	if choiceType, ok := raw["type"].(string); ok {
		switch strings.TrimSpace(choiceType) {
		case "auto", "any":
			return "auto"
		case "none":
			return "none"
		case "tool":
			if name, ok := raw["name"].(string); ok && strings.TrimSpace(name) != "" {
				return map[string]any{
					"type": "function",
					"function": map[string]any{
						"name": strings.TrimSpace(name),
					},
				}
			}
		}
	}
	return raw
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func positiveIntPtr(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}

func cloneJSONMap(value *apiopenapi.JsonObject) map[string]any {
	if value == nil {
		return nil
	}
	return cloneMap(*value)
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = cloneAny(item)
	}
	return out
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		out := make([]any, len(typed))
		for idx, item := range typed {
			out[idx] = cloneAny(item)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, len(typed))
		for idx, item := range typed {
			out[idx] = cloneMap(item)
		}
		return out
	default:
		return typed
	}
}

func dedupeRequestCapabilities(values []gatewaycontract.RequestCapability) []gatewaycontract.RequestCapability {
	seen := map[string]gatewaycontract.RequestCapability{}
	for _, value := range values {
		if value.Key == "" {
			continue
		}
		seen[value.Key] = value
	}
	out := make([]gatewaycontract.RequestCapability, 0, len(seen))
	for _, value := range seen {
		out = append(out, value)
	}
	return out
}

func dedupeCapabilityDescriptors(values []capabilitiescontract.Descriptor) []capabilitiescontract.Descriptor {
	seen := map[string]capabilitiescontract.Descriptor{}
	for _, value := range values {
		if value.Key == "" {
			continue
		}
		seen[value.Key] = value
	}
	out := make([]capabilitiescontract.Descriptor, 0, len(seen))
	for _, value := range seen {
		out = append(out, value)
	}
	return out
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func estimateUsage(text string) gatewaycontract.Usage {
	tokens := estimateTokens(text)
	return gatewaycontract.Usage{
		InputTokens:  tokens / 2,
		OutputTokens: tokens / 2,
		Estimated:    true,
	}
}

func tokenUsage(usage gatewaycontract.Usage) *apiopenapi.TokenUsage {
	total := usage.InputTokens + usage.OutputTokens + usage.CachedTokens
	return &apiopenapi.TokenUsage{
		PromptTokens:     ptrInt(usage.InputTokens),
		CompletionTokens: ptrInt(usage.OutputTokens),
		TotalTokens:      ptrInt(total),
		InputTokens:      ptrInt(usage.InputTokens),
		OutputTokens:     ptrInt(usage.OutputTokens),
		CachedTokens:     ptrInt(usage.CachedTokens),
	}
}

func anthropicUsage(usage gatewaycontract.Usage) *apiopenapi.AnthropicUsage {
	return &apiopenapi.AnthropicUsage{
		InputTokens:  ptrInt(usage.InputTokens),
		OutputTokens: ptrInt(usage.OutputTokens),
	}
}

func estimateTokens(text string) int {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		if text == "" {
			return 1
		}
		return max(1, len(text)/4)
	}
	return max(1, len(fields)*2)
}

func responseID(resp gatewaycontract.CanonicalResponse) string {
	if strings.TrimSpace(resp.ID) != "" {
		return resp.ID
	}
	return randomHexString(12)
}

func randomHexString(size int) string {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func jsonObjectToMap(value *apiopenapi.JsonObject) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(*value))
	for key, item := range *value {
		out[key] = item
	}
	return out
}

func ptrInt(value int) *int {
	return &value
}

func ptrString(value string) *string {
	return &value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
