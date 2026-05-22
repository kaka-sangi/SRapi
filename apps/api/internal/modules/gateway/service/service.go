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

func (s *Service) NormalizeGeminiGenerateContent(req apiopenapi.GeminiGenerateContentRequest, model string, stream bool, meta RequestMeta) gatewaycontract.CanonicalRequest {
	var warnings []string
	var parts []string
	var messages []gatewaycontract.Message
	instructions := geminiContentText(req.SystemInstruction)
	if instructions != "" {
		parts = append(parts, "system: "+instructions)
	}
	for _, content := range req.Contents {
		role := geminiRole(content.Role)
		blocks := geminiContentBlocks(content, role)
		if len(blocks) > 0 {
			messages = append(messages, gatewaycontract.Message{Role: role, Content: blocks})
		}
		text := textFromBlocks(blocks)
		if text != "" {
			parts = append(parts, role+": "+text)
		}
		if geminiContentHasMedia(content) {
			warnings = append(warnings, "vision_ignored")
		}
	}
	if req.SystemInstruction != nil && geminiContentHasMedia(*req.SystemInstruction) {
		warnings = append(warnings, "vision_ignored")
	}
	if req.SafetySettings != nil && len(*req.SafetySettings) > 0 {
		warnings = append(warnings, "safety_settings_ignored")
	}
	if req.GenerationConfig != nil && req.GenerationConfig.TopK != nil {
		warnings = append(warnings, "top_k_ignored")
	}
	canonical := canonical(meta, gatewaycontract.ProtocolGeminiCompatible, gatewaycontract.ProtocolGeminiCompatible, model, "", stream, strings.Join(parts, "\n"), messages, nil, instructions, uniqueStrings(warnings))
	if cfg := req.GenerationConfig; cfg != nil {
		canonical.Temperature = cfg.Temperature
		canonical.TopP = cfg.TopP
		canonical.MaxOutputTokens = cloneInt(cfg.MaxOutputTokens)
		canonical.Stop = cloneStringSlicePtr(cfg.StopSequences)
		canonical.ResponseFormat = geminiResponseFormat(cfg)
	}
	canonical.Tools = cloneJSONMaps(req.Tools)
	if len(canonical.Tools) > 0 {
		canonical.ToolChoice = geminiToolChoice(req.ToolConfig)
	}
	canonical.CompatibilityWarnings = uniqueStrings(warnings)
	refreshRequestCapabilities(&canonical)
	return canonical
}

func (s *Service) NormalizeEmbeddings(req apiopenapi.EmbeddingRequest, meta RequestMeta) (gatewaycontract.CanonicalRequest, error) {
	input, err := embeddingInput(req.Input)
	if err != nil {
		return gatewaycontract.CanonicalRequest{}, err
	}
	encoding := "float"
	if req.EncodingFormat != nil && strings.TrimSpace(string(*req.EncodingFormat)) != "" {
		encoding = strings.TrimSpace(string(*req.EncodingFormat))
	}
	canonical := canonical(meta, gatewaycontract.ProtocolOpenAICompatible, gatewaycontract.ProtocolOpenAICompatible, req.Model, "", false, strings.Join(input, "\n"), nil, embeddingContentBlocks(input), "", nil)
	canonical.EmbeddingInput = append([]string(nil), input...)
	canonical.EmbeddingEncoding = encoding
	canonical.EmbeddingDimensions = cloneInt(req.Dimensions)
	if req.User != nil {
		canonical.EmbeddingUser = strings.TrimSpace(*req.User)
	}
	canonical.RequestCapabilities = append(canonical.RequestCapabilities, gatewaycontract.RequestCapability{Key: capabilitiescontract.KeyEmbeddings, Version: "v1"})
	return canonical, nil
}

func (s *Service) NormalizeImageGeneration(req apiopenapi.ImageGenerationRequest, meta RequestMeta) (gatewaycontract.CanonicalRequest, error) {
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return gatewaycontract.CanonicalRequest{}, fmt.Errorf("image prompt is empty")
	}
	count := 1
	if req.N != nil {
		count = *req.N
	}
	if count < 1 || count > 10 {
		return gatewaycontract.CanonicalRequest{}, fmt.Errorf("image n must be between 1 and 10")
	}
	canonical := canonical(meta, gatewaycontract.ProtocolOpenAICompatible, gatewaycontract.ProtocolOpenAICompatible, req.Model, "", false, prompt, nil, imageContentBlocks(prompt), "", nil)
	canonical.ImagePrompt = prompt
	canonical.ImageCount = count
	canonical.ImageSize = enumString(req.Size)
	canonical.ImageQuality = enumString(req.Quality)
	canonical.ImageStyle = enumString(req.Style)
	canonical.ImageResponseFormat = enumString(req.ResponseFormat)
	if canonical.ImageResponseFormat == "" {
		canonical.ImageResponseFormat = "url"
	}
	if req.User != nil {
		canonical.ImageUser = strings.TrimSpace(*req.User)
	}
	canonical.ImageExtra = cloneMap(req.AdditionalProperties)
	canonical.RequestCapabilities = append(canonical.RequestCapabilities, gatewaycontract.RequestCapability{Key: capabilitiescontract.KeyImages, Version: "v1"})
	return canonical, nil
}

func (s *Service) NormalizeModerations(req apiopenapi.ModerationRequest, meta RequestMeta) (gatewaycontract.CanonicalRequest, error) {
	input, err := moderationInput(req.Input)
	if err != nil {
		return gatewaycontract.CanonicalRequest{}, err
	}
	canonical := canonical(meta, gatewaycontract.ProtocolOpenAICompatible, gatewaycontract.ProtocolOpenAICompatible, req.Model, "", false, strings.Join(input, "\n"), nil, moderationContentBlocks(input), "", nil)
	canonical.ModerationInput = append([]string(nil), input...)
	if req.User != nil {
		canonical.ModerationUser = strings.TrimSpace(*req.User)
	}
	canonical.RequestCapabilities = append(canonical.RequestCapabilities, gatewaycontract.RequestCapability{Key: capabilitiescontract.KeyModerations, Version: "v1"})
	return canonical, nil
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

func (s *Service) BuildCanonicalEmbeddingResponse(req gatewaycontract.CanonicalRequest, embeddings []gatewaycontract.Embedding, usage gatewaycontract.Usage) gatewaycontract.EmbeddingResponse {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = req.CanonicalModel
	}
	canonicalModel := strings.TrimSpace(req.CanonicalModel)
	if canonicalModel == "" {
		canonicalModel = model
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.CachedTokens == 0 {
		usage = embeddingEstimatedUsage(req.EmbeddingInput)
	}
	return gatewaycontract.EmbeddingResponse{
		ID:                    randomHexString(12),
		RequestID:             strings.TrimSpace(req.RequestID),
		Model:                 model,
		CanonicalModel:        canonicalModel,
		Data:                  cloneEmbeddings(embeddings),
		Usage:                 usage,
		CompatibilityWarnings: uniqueStrings(req.CompatibilityWarnings),
	}
}

func (s *Service) BuildCanonicalImageGenerationResponse(req gatewaycontract.CanonicalRequest, images []gatewaycontract.Image, created int64, usage gatewaycontract.Usage) gatewaycontract.ImageGenerationResponse {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = req.CanonicalModel
	}
	canonicalModel := strings.TrimSpace(req.CanonicalModel)
	if canonicalModel == "" {
		canonicalModel = model
	}
	if created == 0 {
		created = time.Now().Unix()
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.CachedTokens == 0 {
		usage = imageEstimatedUsage(req)
	}
	return gatewaycontract.ImageGenerationResponse{
		ID:                    randomHexString(12),
		RequestID:             strings.TrimSpace(req.RequestID),
		Model:                 model,
		CanonicalModel:        canonicalModel,
		Created:               created,
		Data:                  cloneImages(images),
		Usage:                 usage,
		CompatibilityWarnings: uniqueStrings(req.CompatibilityWarnings),
	}
}

func (s *Service) BuildCanonicalModerationResponse(req gatewaycontract.CanonicalRequest, id string, results []gatewaycontract.ModerationResult, usage gatewaycontract.Usage) gatewaycontract.ModerationResponse {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = req.CanonicalModel
	}
	canonicalModel := strings.TrimSpace(req.CanonicalModel)
	if canonicalModel == "" {
		canonicalModel = model
	}
	id = strings.TrimSpace(id)
	if id == "" {
		id = "modr_" + randomHexString(12)
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.CachedTokens == 0 {
		usage = moderationEstimatedUsage(req.ModerationInput)
	}
	return gatewaycontract.ModerationResponse{
		ID:                    id,
		RequestID:             strings.TrimSpace(req.RequestID),
		Model:                 model,
		CanonicalModel:        canonicalModel,
		Results:               cloneModerationResults(results),
		Usage:                 usage,
		CompatibilityWarnings: uniqueStrings(req.CompatibilityWarnings),
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

func (s *Service) RenderGeminiGenerateContent(resp gatewaycontract.CanonicalResponse) apiopenapi.GeminiGenerateContentResponse {
	role := apiopenapi.GeminiContentRoleModel
	text := resp.Message
	rendered := apiopenapi.GeminiGenerateContentResponse{
		Candidates: []apiopenapi.GeminiCandidate{{
			Content: apiopenapi.GeminiContent{
				Role:  &role,
				Parts: []apiopenapi.GeminiPart{{Text: &text}},
			},
			FinishReason: "STOP",
			Index:        0,
		}},
		ModelVersion:  ptrString(resp.Model),
		ResponseId:    ptrString("gemini_" + responseID(resp)),
		UsageMetadata: geminiUsage(resp.Usage),
	}
	if len(resp.CompatibilityWarnings) > 0 {
		warnings := append([]string(nil), resp.CompatibilityWarnings...)
		rendered.CompatibilityWarnings = &warnings
	}
	return rendered
}

func (s *Service) RenderEmbeddings(resp gatewaycontract.EmbeddingResponse) apiopenapi.EmbeddingResponse {
	data := make([]apiopenapi.EmbeddingObject, 0, len(resp.Data))
	for _, item := range resp.Data {
		vector := apiopenapi.EmbeddingVector{}
		if item.Base64Vector != "" {
			_ = vector.FromEmbeddingVector1(item.Base64Vector)
		} else {
			_ = vector.FromEmbeddingVector0(append([]float32(nil), item.Vector...))
		}
		data = append(data, apiopenapi.EmbeddingObject{
			Object:    apiopenapi.Embedding,
			Embedding: vector,
			Index:     item.Index,
		})
	}
	return apiopenapi.EmbeddingResponse{
		Object: apiopenapi.EmbeddingResponseObjectList,
		Data:   data,
		Model:  resp.Model,
		Usage:  *tokenUsage(resp.Usage),
	}
}

func (s *Service) RenderImageGeneration(resp gatewaycontract.ImageGenerationResponse) apiopenapi.ImageGenerationResponse {
	data := make([]apiopenapi.ImageGenerationObject, 0, len(resp.Data))
	for _, item := range resp.Data {
		image := apiopenapi.ImageGenerationObject{
			AdditionalProperties: cloneMap(item.Metadata),
		}
		if value := strings.TrimSpace(item.URL); value != "" {
			image.Url = &value
		}
		if value := strings.TrimSpace(item.Base64JSON); value != "" {
			image.B64Json = &value
		}
		if value := strings.TrimSpace(item.RevisedPrompt); value != "" {
			image.RevisedPrompt = &value
		}
		data = append(data, image)
	}
	return apiopenapi.ImageGenerationResponse{
		Created: resp.Created,
		Data:    data,
	}
}

func (s *Service) RenderModerations(resp gatewaycontract.ModerationResponse) apiopenapi.ModerationResponse {
	results := make([]apiopenapi.ModerationResult, 0, len(resp.Results))
	for _, item := range resp.Results {
		result := apiopenapi.ModerationResult{
			Categories:     cloneBoolMap(item.Categories),
			CategoryScores: cloneFloat32Map(item.CategoryScores),
			Flagged:        item.Flagged,
		}
		if len(item.CategoryAppliedInputTypes) > 0 {
			applied := cloneStringSliceMap(item.CategoryAppliedInputTypes)
			result.CategoryAppliedInputTypes = &applied
		}
		results = append(results, result)
	}
	return apiopenapi.ModerationResponse{
		Id:      resp.ID,
		Model:   resp.Model,
		Results: results,
	}
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

func (s *Service) RenderGeminiGenerateContentStreamEvents(resp gatewaycontract.CanonicalResponse) []StreamEvent {
	rendered := s.RenderGeminiGenerateContent(resp)
	return []StreamEvent{{
		Data: map[string]any{
			"candidates":            rendered.Candidates,
			"usageMetadata":         rendered.UsageMetadata,
			"modelVersion":          rendered.ModelVersion,
			"responseId":            rendered.ResponseId,
			"compatibilityWarnings": rendered.CompatibilityWarnings,
		},
	}}
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

func geminiContentBlocks(content apiopenapi.GeminiContent, role string) []gatewaycontract.ContentBlock {
	out := make([]gatewaycontract.ContentBlock, 0, len(content.Parts))
	for _, part := range content.Parts {
		if part.Text != nil {
			text := strings.TrimSpace(*part.Text)
			if text != "" {
				out = append(out, gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockText, Role: role, Text: text})
			}
		}
		if part.InlineData != nil || part.FileData != nil {
			out = append(out, gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockImage, Role: role, Text: "[image]", Metadata: geminiPartMediaMetadata(part)})
		}
		if part.FunctionCall != nil {
			out = append(out, gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockToolCall, Role: role, Text: "[function_call]", Metadata: cloneMap(*part.FunctionCall)})
		}
		if part.FunctionResponse != nil {
			out = append(out, gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockToolCall, Role: role, Text: "[function_response]", Metadata: cloneMap(*part.FunctionResponse)})
		}
	}
	return out
}

func geminiPartMediaMetadata(part apiopenapi.GeminiPart) map[string]any {
	out := map[string]any{}
	if part.InlineData != nil {
		out["inline_data"] = cloneMap(*part.InlineData)
	}
	if part.FileData != nil {
		out["file_data"] = cloneMap(*part.FileData)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func geminiContentText(content *apiopenapi.GeminiContent) string {
	if content == nil {
		return ""
	}
	return textFromBlocks(geminiContentBlocks(*content, "system"))
}

func geminiRole(role *apiopenapi.GeminiContentRole) string {
	if role == nil {
		return "user"
	}
	switch *role {
	case apiopenapi.GeminiContentRoleModel:
		return "assistant"
	case apiopenapi.GeminiContentRoleUser:
		return "user"
	default:
		value := strings.TrimSpace(string(*role))
		if value == "" {
			return "user"
		}
		return value
	}
}

func geminiContentHasMedia(content apiopenapi.GeminiContent) bool {
	for _, part := range content.Parts {
		if part.InlineData != nil || part.FileData != nil {
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

func embeddingInput(input apiopenapi.EmbeddingRequest_Input) ([]string, error) {
	if text, err := input.AsEmbeddingRequestInput0(); err == nil {
		text = strings.TrimSpace(text)
		if text == "" {
			return nil, fmt.Errorf("embedding input is empty")
		}
		return []string{text}, nil
	}
	if values, err := input.AsEmbeddingRequestInput1(); err == nil {
		out := make([]string, 0, len(values))
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value != "" {
				out = append(out, value)
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("embedding input is empty")
		}
		return out, nil
	}
	return nil, fmt.Errorf("embedding token-array input is not supported")
}

func embeddingContentBlocks(values []string) []gatewaycontract.ContentBlock {
	out := make([]gatewaycontract.ContentBlock, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockText, Role: "user", Text: value})
	}
	return out
}

func imageContentBlocks(prompt string) []gatewaycontract.ContentBlock {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil
	}
	return []gatewaycontract.ContentBlock{{Type: gatewaycontract.ContentBlockText, Role: "user", Text: prompt}}
}

func moderationInput(input apiopenapi.ModerationRequest_Input) ([]string, error) {
	if text, err := input.AsModerationRequestInput0(); err == nil {
		text = strings.TrimSpace(text)
		if text == "" {
			return nil, fmt.Errorf("moderation input is empty")
		}
		return []string{text}, nil
	}
	if values, err := input.AsModerationRequestInput1(); err == nil {
		out := make([]string, 0, len(values))
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value != "" {
				out = append(out, value)
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("moderation input is empty")
		}
		return out, nil
	}
	return nil, fmt.Errorf("moderation input must be a string or string array")
}

func moderationContentBlocks(values []string) []gatewaycontract.ContentBlock {
	out := make([]gatewaycontract.ContentBlock, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockText, Role: "user", Text: value})
	}
	return out
}

func embeddingEstimatedUsage(values []string) gatewaycontract.Usage {
	return gatewaycontract.Usage{
		InputTokens: estimateTokens(strings.Join(values, "\n")),
		Estimated:   true,
	}
}

func imageEstimatedUsage(req gatewaycontract.CanonicalRequest) gatewaycontract.Usage {
	output := req.ImageCount
	if output <= 0 {
		output = 1
	}
	return gatewaycontract.Usage{
		InputTokens:  estimateTokens(req.ImagePrompt),
		OutputTokens: output,
		Estimated:    true,
	}
}

func moderationEstimatedUsage(values []string) gatewaycontract.Usage {
	return gatewaycontract.Usage{
		InputTokens: estimateTokens(strings.Join(values, "\n")),
		Estimated:   true,
	}
}

func cloneEmbeddings(values []gatewaycontract.Embedding) []gatewaycontract.Embedding {
	if values == nil {
		return nil
	}
	out := make([]gatewaycontract.Embedding, len(values))
	for idx, value := range values {
		out[idx] = value
		out[idx].Vector = append([]float32(nil), value.Vector...)
	}
	return out
}

func cloneImages(values []gatewaycontract.Image) []gatewaycontract.Image {
	if values == nil {
		return nil
	}
	out := make([]gatewaycontract.Image, len(values))
	for idx, value := range values {
		out[idx] = value
		out[idx].Metadata = cloneMap(value.Metadata)
	}
	return out
}

func cloneModerationResults(values []gatewaycontract.ModerationResult) []gatewaycontract.ModerationResult {
	if values == nil {
		return nil
	}
	out := make([]gatewaycontract.ModerationResult, len(values))
	for idx, value := range values {
		out[idx] = gatewaycontract.ModerationResult{
			Flagged:                   value.Flagged,
			Categories:                cloneBoolMap(value.Categories),
			CategoryScores:            cloneFloat32Map(value.CategoryScores),
			CategoryAppliedInputTypes: cloneStringSliceMap(value.CategoryAppliedInputTypes),
		}
	}
	return out
}

func cloneBoolMap(values map[string]bool) map[string]bool {
	if values == nil {
		return nil
	}
	out := make(map[string]bool, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneFloat32Map(values map[string]float32) map[string]float32 {
	if values == nil {
		return nil
	}
	out := make(map[string]float32, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneStringSliceMap(values map[string][]string) map[string][]string {
	if values == nil {
		return nil
	}
	out := make(map[string][]string, len(values))
	for key, value := range values {
		out[key] = append([]string(nil), value...)
	}
	return out
}

func enumString[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(string(*value))
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

func geminiResponseFormat(cfg *apiopenapi.GeminiGenerationConfig) map[string]any {
	if cfg == nil {
		return nil
	}
	out := map[string]any{}
	if cfg.ResponseMimeType != nil && strings.TrimSpace(*cfg.ResponseMimeType) != "" {
		out["type"] = strings.TrimSpace(*cfg.ResponseMimeType)
	}
	if cfg.ResponseSchema != nil {
		out["schema"] = cloneMap(*cfg.ResponseSchema)
	}
	for key, item := range cfg.AdditionalProperties {
		if _, ok := out[key]; !ok {
			out[key] = cloneAny(item)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func geminiToolChoice(value *apiopenapi.JsonObject) any {
	raw := cloneJSONMap(value)
	if len(raw) == 0 {
		return nil
	}
	return raw
}

func cloneJSONMaps(values *[]apiopenapi.JsonObject) []map[string]any {
	if values == nil || len(*values) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(*values))
	for _, value := range *values {
		out = append(out, cloneMap(value))
	}
	return out
}

func cloneStringSlicePtr(value *[]string) []string {
	if value == nil || len(*value) == 0 {
		return nil
	}
	out := make([]string, 0, len(*value))
	for _, item := range *value {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
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

func geminiUsage(usage gatewaycontract.Usage) *apiopenapi.GeminiUsageMetadata {
	total := usage.InputTokens + usage.OutputTokens + usage.CachedTokens
	return &apiopenapi.GeminiUsageMetadata{
		PromptTokenCount:        ptrInt(usage.InputTokens),
		CandidatesTokenCount:    ptrInt(usage.OutputTokens),
		TotalTokenCount:         ptrInt(total),
		CachedContentTokenCount: ptrInt(usage.CachedTokens),
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
