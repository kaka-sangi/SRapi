package service

import (
	"fmt"
	"strings"
	"time"

	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Service) RenderChatCompletions(resp gatewaycontract.CanonicalResponse) apiopenapi.ChatCompletionResponse {
	now := time.Now().UTC()
	systemFingerprint := "srapi"
	msg := apiopenapi.ChatMessage{}
	contentBlocks := chatMessageContentBlocks(resp.OutputItems)
	if chatContentShouldRenderAsBlocks(contentBlocks) {
		_ = msg.Content.FromChatMessageContent1(outputOpenAIContentBlocks(contentBlocks))
	} else {
		_ = msg.Content.FromChatMessageContent0(outputTextFromBlocks(contentBlocks))
	}
	if toolCalls := outputOpenAIChatToolCalls(resp.OutputItems); len(toolCalls) > 0 {
		msg.ToolCalls = &toolCalls
	}
	if reasoning := openAIReasoningContentFromBlocks(resp.OutputItems); reasoning != "" {
		msg.ReasoningContent = &reasoning
	}
	msg.Role = apiopenapi.ChatMessageRoleAssistant
	return apiopenapi.ChatCompletionResponse{
		Choices: []apiopenapi.ChatCompletionChoice{{
			Index:        0,
			FinishReason: ptrString(openAIChatFinishReason(resp.StopReason)),
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
	status := responsesStatus(resp.StopReason)
	rendered := apiopenapi.ResponsesResponse{
		CreatedAt:         int(now.Unix()),
		Id:                "resp_" + responseID(resp),
		IncompleteDetails: responsesIncompleteDetails(resp.StopReason),
		Model:             resp.Model,
		Object:            apiopenapi.Response,
		Output:            responseOutputItems(resp.OutputItems),
		Status:            &status,
		Usage:             tokenUsage(resp.Usage),
	}
	if len(resp.CompatibilityWarnings) > 0 {
		warnings := append([]string(nil), resp.CompatibilityWarnings...)
		rendered.CompatibilityWarnings = &warnings
	}
	return rendered
}

func (s *Service) RenderAnthropicMessages(resp gatewaycontract.CanonicalResponse) apiopenapi.AnthropicMessagesResponse {
	stopReason := anthropicStopReason(resp.StopReason)
	rendered := apiopenapi.AnthropicMessagesResponse{
		Content:    outputAnthropicContentBlocks(resp.OutputItems),
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
	rendered := apiopenapi.GeminiGenerateContentResponse{
		Candidates: []apiopenapi.GeminiCandidate{{
			Content: apiopenapi.GeminiContent{
				Role:  &role,
				Parts: outputGeminiParts(resp.OutputItems),
			},
			FinishReason: geminiFinishReason(resp.StopReason),
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

func (s *Service) RenderAudioTranscription(resp gatewaycontract.AudioTranscriptionResponse) apiopenapi.AudioTranscriptionResponse {
	rendered := apiopenapi.AudioTranscriptionResponse{
		Text:                 resp.Text,
		AdditionalProperties: map[string]interface{}{},
	}
	if resp.Task != "" {
		rendered.Task = ptrString(resp.Task)
	}
	if resp.Language != "" {
		rendered.Language = ptrString(resp.Language)
	}
	if resp.Duration != nil {
		rendered.Duration = cloneFloat32(resp.Duration)
	}
	if len(resp.Segments) > 0 {
		segments := make([]apiopenapi.AudioTranscriptionSegment, 0, len(resp.Segments))
		for _, item := range resp.Segments {
			segments = append(segments, apiopenapi.AudioTranscriptionSegment{
				AvgLogprob:           cloneFloat32(item.AvgLogprob),
				CompressionRatio:     cloneFloat32(item.CompressionRatio),
				End:                  cloneFloat32(item.End),
				Id:                   cloneInt(item.ID),
				NoSpeechProb:         cloneFloat32(item.NoSpeechProb),
				Seek:                 cloneInt(item.Seek),
				Start:                cloneFloat32(item.Start),
				Temperature:          cloneFloat32(item.Temperature),
				Text:                 ptrStringIfNotEmpty(item.Text),
				Tokens:               cloneIntSlicePtr(item.Tokens),
				AdditionalProperties: cloneMap(item.Metadata),
			})
		}
		rendered.Segments = &segments
	}
	if resp.Usage.InputTokens > 0 || resp.Usage.OutputTokens > 0 || resp.Usage.CachedTokens > 0 {
		rendered.Usage = tokenUsage(resp.Usage)
	}
	if len(rendered.AdditionalProperties) == 0 {
		rendered.AdditionalProperties = nil
	}
	return rendered
}

func (s *Service) RenderRerank(resp gatewaycontract.RerankResponse) apiopenapi.RerankResponse {
	results := make([]apiopenapi.RerankResult, 0, len(resp.Results))
	for _, item := range resp.Results {
		result := apiopenapi.RerankResult{
			Index:                item.Index,
			RelevanceScore:       item.RelevanceScore,
			AdditionalProperties: cloneMap(item.Metadata),
		}
		if item.Document != nil {
			document := rerankDocumentObject(*item.Document)
			result.Document = &document
		}
		results = append(results, result)
	}
	return apiopenapi.RerankResponse{
		Id:      resp.ID,
		Model:   resp.Model,
		Results: results,
		Usage:   tokenUsage(resp.Usage),
	}
}

func (s *Service) RenderChatStreamChunk(resp gatewaycontract.CanonicalResponse) map[string]any {
	blocks := normalizeOutputItems(resp.OutputItems)
	delta := map[string]any{"role": "assistant"}
	if text := outputTextFromBlocks(blocks); text != "" {
		delta["content"] = text
	}
	if reasoning := openAIReasoningContentFromBlocks(blocks); reasoning != "" {
		delta["reasoning_content"] = reasoning
	}
	if toolCalls := chatStreamToolCalls(blocks); len(toolCalls) > 0 {
		delta["tool_calls"] = toolCalls
	}
	if len(delta) == 1 {
		delta["content"] = ""
	}
	chunk := map[string]any{
		"id":      "chatcmpl_" + responseID(resp),
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   resp.Model,
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         delta,
				"finish_reason": openAIChatFinishReason(resp.StopReason),
			},
		},
	}
	if len(resp.CompatibilityWarnings) > 0 {
		chunk["compatibility_warnings"] = append([]string(nil), resp.CompatibilityWarnings...)
	}
	return chunk
}

func (s *Service) RenderChatStreamChunks(resp gatewaycontract.CanonicalResponse) []map[string]any {
	events := normalizeStreamEvents(resp.StreamEvents)
	if len(events) == 0 {
		return []map[string]any{s.RenderChatStreamChunk(resp)}
	}
	chunks := make([]map[string]any, 0, len(events))
	toolIndexes := newChatStreamToolCallIndexes()
	for _, event := range events {
		switch event.Type {
		case gatewaycontract.StreamEventContentDelta, gatewaycontract.StreamEventToolResult:
			if text := event.Delta.Text; text != "" {
				chunks = append(chunks, chatStreamChunkWithIndex(resp, streamEventChoiceIndex(event), map[string]any{"content": text}, nil, nil))
			}
		case gatewaycontract.StreamEventReasoning:
			if text := event.Delta.Text; text != "" {
				chunks = append(chunks, chatStreamChunkWithIndex(resp, streamEventChoiceIndex(event), map[string]any{"reasoning_content": text}, nil, nil))
			}
		case gatewaycontract.StreamEventToolCallDelta:
			toolCall := chatStreamToolCallDelta(event, toolIndexes.indexFor(event))
			if toolCall != nil {
				chunks = append(chunks, chatStreamChunkWithIndex(resp, streamEventChoiceIndex(event), map[string]any{"tool_calls": []map[string]any{toolCall}}, nil, nil))
			}
		case gatewaycontract.StreamEventUsage:
			chunk := chatStreamChunk(resp, nil, nil, tokenUsage(event.Usage))
			chunk["choices"] = []map[string]any{}
			chunks = append(chunks, chunk)
		case gatewaycontract.StreamEventStop:
			reason := firstNonEmpty(event.StopReason, resp.StopReason)
			chunks = append(chunks, chatStreamChunkWithIndex(resp, streamEventChoiceIndex(event), map[string]any{}, openAIChatFinishReason(reason), nil))
		}
	}
	if len(chunks) == 0 {
		return []map[string]any{s.RenderChatStreamChunk(resp)}
	}
	return chunks
}

func chatStreamChunk(resp gatewaycontract.CanonicalResponse, delta map[string]any, finishReason any, usage *apiopenapi.TokenUsage) map[string]any {
	return chatStreamChunkWithIndex(resp, 0, delta, finishReason, usage)
}

func chatStreamChunkWithIndex(resp gatewaycontract.CanonicalResponse, choiceIndex int, delta map[string]any, finishReason any, usage *apiopenapi.TokenUsage) map[string]any {
	chunk := map[string]any{
		"id":      "chatcmpl_" + responseID(resp),
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   resp.Model,
		"choices": []map[string]any{
			{
				"index":         choiceIndex,
				"delta":         delta,
				"finish_reason": finishReason,
			},
		},
	}
	if usage != nil {
		chunk["usage"] = usage
	}
	if len(resp.CompatibilityWarnings) > 0 {
		chunk["compatibility_warnings"] = append([]string(nil), resp.CompatibilityWarnings...)
	}
	return chunk
}

func streamEventChoiceIndex(event gatewaycontract.StreamEvent) int {
	if index := positiveIntFromAny(event.Metadata["choice_index"]); index >= 0 {
		return index
	}
	if isResponsesStyleStreamEvent(event) {
		return 0
	}
	return event.ContentIndex
}

func positiveIntFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return -1
	}
}

func chatStreamToolCallDelta(event gatewaycontract.StreamEvent, toolCallIndex int) map[string]any {
	if toolCallIndex < 0 {
		toolCallIndex = 0
	}
	call := map[string]any{
		"index": toolCallIndex,
		"type":  "function",
	}
	if id := strings.TrimSpace(event.Delta.ToolCallID); id != "" {
		call["id"] = id
	}
	function := map[string]any{}
	if name := strings.TrimSpace(event.Delta.ToolName); name != "" {
		function["name"] = name
	}
	if args := event.Delta.ToolArgumentsJSON; args != "" {
		function["arguments"] = args
	}
	if len(function) > 0 {
		call["function"] = function
	}
	if len(call) <= 2 {
		return nil
	}
	return call
}

type chatStreamToolCallIndexes struct {
	next                  int
	byResponseOutputIndex map[int]int
}

func newChatStreamToolCallIndexes() *chatStreamToolCallIndexes {
	return &chatStreamToolCallIndexes{byResponseOutputIndex: map[int]int{}}
}

func (s *chatStreamToolCallIndexes) indexFor(event gatewaycontract.StreamEvent) int {
	if !isResponsesStyleStreamEvent(event) {
		return event.ContentIndex
	}
	outputIndex := event.ContentIndex
	if outputIndex < 0 {
		outputIndex = 0
	}
	if index, ok := s.byResponseOutputIndex[outputIndex]; ok {
		return index
	}
	index := s.next
	s.byResponseOutputIndex[outputIndex] = index
	s.next++
	return index
}

func isResponsesStyleStreamEvent(event gatewaycontract.StreamEvent) bool {
	return strings.HasPrefix(strings.TrimSpace(event.RawEventType), "response.")
}

func (s *Service) renderResponsesCanonicalStreamEvents(resp gatewaycontract.CanonicalResponse) []StreamEvent {
	events := normalizeStreamEvents(resp.StreamEvents)
	if !streamEventsHaveRenderableOutput(events) && !streamEventsHaveResponsesTerminal(events) {
		return nil
	}
	out := []StreamEvent{
		{
			Event: "response.created",
			Data: map[string]any{
				"type": "response.created",
				"response": map[string]any{
					"id":         "resp_" + responseID(resp),
					"object":     "response",
					"created_at": time.Now().Unix(),
					"model":      resp.Model,
					"status":     "in_progress",
				},
			},
		},
	}
	nextOutputIndex := 0
	textStates := newResponseStreamTextStates(resp.OutputItems)
	imageStates := newResponseStreamImageStates(resp.OutputItems)
	toolStates := newStreamToolCallStates(resp.OutputItems)
	for _, event := range events {
		switch event.Type {
		case gatewaycontract.StreamEventContentDelta, gatewaycontract.StreamEventToolResult:
			if isResponsesImageGenerationPartialEvent(event) {
				state := imageStates.stateFor(event)
				if state != nil && state.OutputIndex < 0 {
					state.OutputIndex = nextOutputIndex
					nextOutputIndex++
					out = append(out, responseStreamImageGenerationStartEvent(state))
				}
				if partial, ok := responseImageGenerationPartialStreamEvent(event, state); ok {
					out = append(out, partial)
				}
				continue
			}
			delta := event.Delta.Text
			state := textStates.stateFor(event, responseStreamDeltaTextBlockType(event, gatewaycontract.ContentBlockText))
			state.mergeMetadata(event.Delta.Metadata)
			if delta == "" && len(event.Delta.Metadata) == 0 {
				continue
			}
			if state.OutputIndex < 0 {
				state.OutputIndex = nextOutputIndex
				nextOutputIndex++
				out = append(out, responseStreamTextStartEvents(state.ItemID, state.OutputIndex, state.BlockType, state.Metadata)...)
			}
			if delta == "" {
				continue
			}
			state.Text.WriteString(delta)
			out = append(out, responseStreamTextDeltaEvent(state.ItemID, state.OutputIndex, state.BlockType, delta, state.Metadata))
		case gatewaycontract.StreamEventReasoning:
			delta := event.Delta.Text
			state := textStates.stateFor(event, gatewaycontract.ContentBlockReasoning)
			state.mergeMetadata(event.Delta.Metadata)
			if delta == "" && len(event.Delta.Metadata) == 0 {
				continue
			}
			if state.OutputIndex < 0 {
				state.OutputIndex = nextOutputIndex
				nextOutputIndex++
				out = append(out, responseStreamTextStartEvents(state.ItemID, state.OutputIndex, state.BlockType, state.Metadata)...)
			}
			if delta == "" {
				continue
			}
			state.Text.WriteString(delta)
			out = append(out, responseStreamTextDeltaEvent(state.ItemID, state.OutputIndex, state.BlockType, delta, state.Metadata))
		case gatewaycontract.StreamEventToolCallDelta:
			state := toolStates.stateFor(event)
			if state.OutputIndex < 0 {
				state.OutputIndex = nextOutputIndex
				nextOutputIndex++
				out = append(out, responseStreamToolCallStartEvent(state))
			}
			if delta := event.Delta.ToolArgumentsJSON; delta != "" {
				state.Arguments.WriteString(delta)
				if shouldSuppressHostedWebSearchArgumentDelta(state, delta) {
					continue
				}
				out = append(out, StreamEvent{
					Event: "response.function_call_arguments.delta",
					Data: map[string]any{
						"type":         "response.function_call_arguments.delta",
						"item_id":      state.ItemID,
						"output_index": state.OutputIndex,
						"delta":        delta,
					},
				})
			}
		}
	}
	doneGroups := make([]responseStreamDoneEventGroup, 0)
	for _, state := range textStates.openStates() {
		text := state.Text.String()
		doneGroups = append(doneGroups, responseStreamDoneEventGroup{
			OutputIndex: state.OutputIndex,
			Events: []StreamEvent{
				responseStreamTextDoneEvent(state.ItemID, state.OutputIndex, state.BlockType, text, state.Metadata),
				responseStreamContentPartDoneEvent(state.ItemID, state.OutputIndex, state.BlockType, text, state.Metadata),
				responseStreamMessageDoneEvent(state.ItemID, state.OutputIndex, state.BlockType, text, state.Metadata),
			},
		})
	}
	for _, state := range toolStates.openStates() {
		arguments := state.Arguments.String()
		if arguments == "" {
			arguments = state.Block.ToolArgumentsJSON
		}
		if group, ok := hostedWebSearchStreamDoneGroup(state, arguments); ok {
			doneGroups = append(doneGroups, group)
			continue
		}
		doneGroups = append(doneGroups, responseStreamDoneEventGroup{
			OutputIndex: state.OutputIndex,
			Events: []StreamEvent{
				{
					Event: "response.function_call_arguments.done",
					Data: map[string]any{
						"type":         "response.function_call_arguments.done",
						"item_id":      state.ItemID,
						"output_index": state.OutputIndex,
						"arguments":    arguments,
					},
				},
				{
					Event: "response.output_item.done",
					Data: map[string]any{
						"type":         "response.output_item.done",
						"output_index": state.OutputIndex,
						"item":         responseStreamFunctionCallItem(state.ItemID, state.completedBlock(arguments)),
					},
				},
			},
		})
	}
	for _, state := range imageStates.openStates() {
		doneGroups = append(doneGroups, responseStreamDoneEventGroup{
			OutputIndex: state.OutputIndex,
			Events:      []StreamEvent{responseStreamImageGenerationDoneEvent(state)},
		})
	}
	sortResponseStreamDoneEventGroups(doneGroups)
	for _, group := range doneGroups {
		out = append(out, group.Events...)
	}
	terminalEventName := responsesTerminalEventName(resp.StopReason)
	if rawTerminalEventName := responsesRawTerminalEventName(events); rawTerminalEventName != "" {
		terminalEventName = rawTerminalEventName
	}
	terminalResponse := s.RenderResponses(resp)
	if terminalEventName == "response.failed" {
		failedStatus := "failed"
		terminalResponse.Status = &failedStatus
	}
	out = append(out,
		StreamEvent{
			Event: terminalEventName,
			Data: map[string]any{
				"type":     terminalEventName,
				"response": terminalResponse,
			},
		},
	)
	return out
}

func streamEventsHaveResponsesTerminal(events []gatewaycontract.StreamEvent) bool {
	return responsesRawTerminalEventName(events) != ""
}

func responsesRawTerminalEventName(events []gatewaycontract.StreamEvent) string {
	for idx := len(events) - 1; idx >= 0; idx-- {
		event := events[idx]
		if event.Type != gatewaycontract.StreamEventStop {
			continue
		}
		switch strings.TrimSpace(event.RawEventType) {
		case "response.completed", "response.done", "response.incomplete", "response.cancelled", "response.canceled", "response.failed":
			return strings.TrimSpace(event.RawEventType)
		}
	}
	return ""
}

func responseStreamTextStartEvents(itemID string, outputIndex int, blockType gatewaycontract.ContentBlockType, metadata map[string]any) []StreamEvent {
	part := responseStreamTextPart(blockType, "", metadata)
	return []StreamEvent{
		{
			Event: "response.output_item.added",
			Data: map[string]any{
				"type":         "response.output_item.added",
				"output_index": outputIndex,
				"item": map[string]any{
					"id":      itemID,
					"type":    "message",
					"role":    "assistant",
					"content": []any{},
				},
			},
		},
		{
			Event: "response.content_part.added",
			Data: map[string]any{
				"type":          "response.content_part.added",
				"item_id":       itemID,
				"output_index":  outputIndex,
				"content_index": 0,
				"part":          part,
			},
		},
	}
}

func responseImageGenerationPartialStreamEvent(event gatewaycontract.StreamEvent, state *responseStreamImageState) (StreamEvent, bool) {
	if !isResponsesImageGenerationPartialEvent(event) {
		return StreamEvent{}, false
	}
	outputIndex := event.ContentIndex
	itemID := firstNonEmpty(mapStringAny(event.Delta.Metadata, "item_id"), mapStringAny(event.Delta.Metadata, "id"))
	if state != nil {
		outputIndex = state.OutputIndex
		itemID = firstNonEmpty(itemID, mapStringAny(state.Block.Metadata, "id"))
	}
	data := map[string]any{
		"type":              "response.image_generation_call.partial_image",
		"partial_image_b64": mapStringAny(event.Delta.Metadata, "partial_image_b64"),
	}
	if outputIndex >= 0 {
		data["output_index"] = outputIndex
	}
	if itemID != "" {
		data["item_id"] = itemID
	}
	if format := mapStringAny(event.Delta.Metadata, "output_format"); format != "" {
		data["output_format"] = format
	}
	if background := mapStringAny(event.Delta.Metadata, "background"); background != "" {
		data["background"] = background
	}
	if index, ok := event.Delta.Metadata["partial_image_index"]; ok && index != nil {
		data["partial_image_index"] = index
	}
	return StreamEvent{Event: "response.image_generation_call.partial_image", Data: data}, true
}

func responseStreamImageGenerationStartEvent(state *responseStreamImageState) StreamEvent {
	item := responseImageGenerationOutputItem(state.Block).AdditionalProperties
	if item == nil {
		item = map[string]any{}
	}
	if id := firstNonEmpty(mapStringAny(item, "id"), state.ItemID); id != "" {
		item["id"] = id
	}
	item["type"] = "image_generation_call"
	delete(item, "result")
	return StreamEvent{
		Event: "response.output_item.added",
		Data: map[string]any{
			"type":         "response.output_item.added",
			"output_index": state.OutputIndex,
			"item":         item,
		},
	}
}

func responseStreamImageGenerationDoneEvent(state *responseStreamImageState) StreamEvent {
	item := responseImageGenerationOutputItem(state.Block).AdditionalProperties
	if item == nil {
		item = map[string]any{}
	}
	if id := firstNonEmpty(mapStringAny(item, "id"), state.ItemID); id != "" {
		item["id"] = id
	}
	item["type"] = "image_generation_call"
	return StreamEvent{
		Event: "response.output_item.done",
		Data: map[string]any{
			"type":         "response.output_item.done",
			"output_index": state.OutputIndex,
			"item":         item,
		},
	}
}

func responseStreamTextPart(blockType gatewaycontract.ContentBlockType, text string, metadata map[string]any) map[string]any {
	part := cloneMap(metadata)
	if part == nil {
		part = map[string]any{}
	}
	delete(part, "reasoning_event_type")
	part["type"] = responseStreamContentPartTypeForMetadata(blockType, metadata)
	if blockType == gatewaycontract.ContentBlockRefusal {
		part["refusal"] = text
	} else {
		part["text"] = text
	}
	return part
}

func responseStreamContentPartTypeForMetadata(blockType gatewaycontract.ContentBlockType, metadata map[string]any) string {
	if blockType == gatewaycontract.ContentBlockReasoning && responseReasoningIsSummary(metadata) {
		return "summary_text"
	}
	return responseStreamContentPartType(blockType)
}

func responseStreamTextDeltaEvent(itemID string, outputIndex int, blockType gatewaycontract.ContentBlockType, delta string, metadata map[string]any) StreamEvent {
	eventName := responseStreamTextEventName(blockType, "delta", metadata)
	return StreamEvent{
		Event: eventName,
		Data: map[string]any{
			"type":          eventName,
			"item_id":       itemID,
			"output_index":  outputIndex,
			"content_index": 0,
			"delta":         delta,
		},
	}
}

func responseStreamTextDoneEvent(itemID string, outputIndex int, blockType gatewaycontract.ContentBlockType, text string, metadata map[string]any) StreamEvent {
	eventName := responseStreamTextEventName(blockType, "done", metadata)
	data := map[string]any{
		"type":          eventName,
		"item_id":       itemID,
		"output_index":  outputIndex,
		"content_index": 0,
	}
	if blockType == gatewaycontract.ContentBlockRefusal {
		data["refusal"] = text
	} else {
		data["text"] = text
	}
	return StreamEvent{
		Event: eventName,
		Data:  data,
	}
}

func responseStreamContentPartDoneEvent(itemID string, outputIndex int, blockType gatewaycontract.ContentBlockType, text string, metadata map[string]any) StreamEvent {
	part := responseStreamTextPart(blockType, text, metadata)
	return StreamEvent{
		Event: "response.content_part.done",
		Data: map[string]any{
			"type":          "response.content_part.done",
			"item_id":       itemID,
			"output_index":  outputIndex,
			"content_index": 0,
			"part":          part,
		},
	}
}

func responseStreamMessageDoneEvent(itemID string, outputIndex int, blockType gatewaycontract.ContentBlockType, text string, metadata map[string]any) StreamEvent {
	content := responseStreamTextPart(blockType, text, metadata)
	return StreamEvent{
		Event: "response.output_item.done",
		Data: map[string]any{
			"type":         "response.output_item.done",
			"output_index": outputIndex,
			"item": map[string]any{
				"id":      itemID,
				"type":    "message",
				"role":    "assistant",
				"content": []map[string]any{content},
			},
		},
	}
}

func responseStreamTextEventName(blockType gatewaycontract.ContentBlockType, suffix string, metadata map[string]any) string {
	if blockType == gatewaycontract.ContentBlockReasoning {
		if responseReasoningIsSummary(metadata) {
			return "response.reasoning_summary_text." + suffix
		}
		return "response.reasoning_text." + suffix
	}
	if blockType == gatewaycontract.ContentBlockRefusal {
		return "response.refusal." + suffix
	}
	return "response.output_text." + suffix
}

func responseReasoningIsSummary(metadata map[string]any) bool {
	value := strings.TrimSpace(mapStringAny(metadata, "reasoning_event_type"))
	if value == "" {
		value = strings.TrimSpace(mapStringAny(metadata, "type"))
	}
	return value == "reasoning_summary_text" || value == "summary_text"
}

func responseStreamToolCallStartEvent(state *streamToolCallState) StreamEvent {
	if event, ok := hostedWebSearchStreamStartEvent(state); ok {
		return event
	}
	return StreamEvent{
		Event: "response.output_item.added",
		Data: map[string]any{
			"type":         "response.output_item.added",
			"output_index": state.OutputIndex,
			"item":         responseStreamFunctionCallStartItem(state),
		},
	}
}

func responseStreamFunctionCallStartItem(state *streamToolCallState) map[string]any {
	item := responseFunctionCallItem(state.startBlock(), "in_progress").AdditionalProperties
	if item == nil {
		item = map[string]any{}
	}
	item["id"] = state.ItemID
	item["status"] = "in_progress"
	delete(item, "arguments")
	delete(item, "input")
	return item
}

type streamToolCallStates struct {
	byContentIndex map[int]*streamToolCallState
	order          []*streamToolCallState
}

type streamToolCallState struct {
	ContentIndex int
	OutputIndex  int
	ItemID       string
	Block        gatewaycontract.ContentBlock
	Arguments    strings.Builder
}

func newStreamToolCallStates(blocks []gatewaycontract.ContentBlock) *streamToolCallStates {
	states := &streamToolCallStates{
		byContentIndex: map[int]*streamToolCallState{},
		order:          make([]*streamToolCallState, 0),
	}
	toolIndex := 0
	for blockIndex, block := range normalizeOutputItems(blocks) {
		if block.Type != gatewaycontract.ContentBlockToolCall {
			continue
		}
		state := &streamToolCallState{
			ContentIndex: toolIndex,
			OutputIndex:  -1,
			ItemID:       responseStreamItemID(toolIndex, block),
			Block:        block,
		}
		states.byContentIndex[blockIndex] = state
		states.order = append(states.order, state)
		toolIndex++
	}
	return states
}

func (s *streamToolCallStates) stateFor(event gatewaycontract.StreamEvent) *streamToolCallState {
	if strings.Contains(strings.ToLower(event.OriginProtocol), "openai") && event.ContentIndex >= 0 && event.ContentIndex < len(s.order) {
		state := s.order[event.ContentIndex]
		state.mergeDelta(event.Delta)
		return state
	}
	if state := s.byContentIndex[event.ContentIndex]; state != nil {
		state.mergeDelta(event.Delta)
		return state
	}
	if event.ContentIndex >= 0 && event.ContentIndex < len(s.order) {
		state := s.order[event.ContentIndex]
		state.mergeDelta(event.Delta)
		return state
	}
	block := event.Delta
	block.Type = gatewaycontract.ContentBlockToolCall
	state := &streamToolCallState{
		ContentIndex: event.ContentIndex,
		OutputIndex:  -1,
		ItemID:       responseStreamItemID(len(s.order), block),
		Block:        block,
	}
	if strings.TrimSpace(state.ItemID) == "" {
		state.ItemID = fmt.Sprintf("fc_%d", len(s.order))
	}
	state.mergeDelta(event.Delta)
	s.byContentIndex[event.ContentIndex] = state
	s.order = append(s.order, state)
	return state
}

func (s *streamToolCallStates) openStates() []*streamToolCallState {
	out := make([]*streamToolCallState, 0, len(s.order))
	for _, state := range s.order {
		if state.OutputIndex >= 0 {
			out = append(out, state)
		}
	}
	return out
}

func (s *streamToolCallState) mergeDelta(delta gatewaycontract.ContentBlock) {
	if id := strings.TrimSpace(delta.ToolCallID); id != "" {
		s.Block.ToolCallID = id
		s.ItemID = id
	}
	if name := strings.TrimSpace(delta.ToolName); name != "" {
		s.Block.ToolName = name
	}
	if len(delta.Metadata) > 0 {
		if s.Block.Metadata == nil {
			s.Block.Metadata = map[string]any{}
		}
		for key, value := range delta.Metadata {
			s.Block.Metadata[key] = value
		}
	}
}

func (s *streamToolCallState) startBlock() gatewaycontract.ContentBlock {
	block := s.Block
	block.Type = gatewaycontract.ContentBlockToolCall
	block.ToolArgumentsJSON = ""
	return block
}

func (s *streamToolCallState) completedBlock(arguments string) gatewaycontract.ContentBlock {
	block := s.Block
	block.Type = gatewaycontract.ContentBlockToolCall
	block.ToolArgumentsJSON = arguments
	return block
}

func streamEventsHaveRenderableOutput(events []gatewaycontract.StreamEvent) bool {
	for _, event := range events {
		switch event.Type {
		case gatewaycontract.StreamEventContentDelta, gatewaycontract.StreamEventReasoning, gatewaycontract.StreamEventToolResult:
			if event.Delta.Text != "" || isResponsesImageGenerationPartialEvent(event) {
				return true
			}
		case gatewaycontract.StreamEventToolCallDelta:
			if event.Delta.ToolCallID != "" || event.Delta.ToolName != "" || event.Delta.ToolArgumentsJSON != "" || isHostedWebSearchBlock(event.Delta) {
				return true
			}
		}
	}
	return false
}

func isResponsesImageGenerationPartialEvent(event gatewaycontract.StreamEvent) bool {
	return strings.TrimSpace(event.RawEventType) == "response.image_generation_call.partial_image" &&
		strings.TrimSpace(mapStringAny(event.Delta.Metadata, "partial_image_b64")) != ""
}

func streamEventsCanRenderGemini(events []gatewaycontract.StreamEvent) bool {
	renderable := false
	for _, event := range events {
		switch event.Type {
		case gatewaycontract.StreamEventContentDelta, gatewaycontract.StreamEventReasoning, gatewaycontract.StreamEventToolResult:
			if event.Delta.Text != "" {
				renderable = true
			}
		case gatewaycontract.StreamEventToolCallDelta:
			if event.Delta.ToolArgumentsJSON != "" && len(parseJSONObject(event.Delta.ToolArgumentsJSON)) == 0 {
				return false
			}
			if event.Delta.ToolCallID != "" || event.Delta.ToolName != "" || event.Delta.ToolArgumentsJSON != "" {
				renderable = true
			}
		}
	}
	return renderable
}

func (s *Service) RenderResponsesStreamEvents(resp gatewaycontract.CanonicalResponse) []StreamEvent {
	if events := s.renderResponsesCanonicalStreamEvents(resp); len(events) > 0 {
		return events
	}
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
	events := []StreamEvent{
		{
			Event: "response.created",
			Data: map[string]any{
				"type":     "response.created",
				"response": created,
			},
		},
	}
	events = append(events, responseStreamOutputEvents(resp.OutputItems)...)
	terminalEventName := responsesTerminalEventName(resp.StopReason)
	events = append(events,
		StreamEvent{
			Event: terminalEventName,
			Data: map[string]any{
				"type":     terminalEventName,
				"response": completed,
			},
		},
	)
	return events
}

func (s *Service) RenderAnthropicMessagesStreamEvents(resp gatewaycontract.CanonicalResponse) []StreamEvent {
	if events := s.renderAnthropicCanonicalStreamEvents(resp); len(events) > 0 {
		return events
	}
	id := "msg_" + responseID(resp)
	events := []StreamEvent{
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
					"usage":         anthropicStreamUsage(ptrInt(resp.Usage.InputTokens), ptrInt(0), resp.Usage.CachedTokens),
				},
			},
		},
	}
	for index, block := range normalizeOutputItems(resp.OutputItems) {
		events = append(events, StreamEvent{
			Event: "content_block_start",
			Data: map[string]any{
				"type":          "content_block_start",
				"index":         index,
				"content_block": anthropicStreamContentBlock(block),
			},
		})
		if delta := anthropicStreamContentDelta(block); len(delta) > 0 {
			events = append(events, StreamEvent{
				Event: "content_block_delta",
				Data: map[string]any{
					"type":  "content_block_delta",
					"index": index,
					"delta": delta,
				},
			})
		}
		events = append(events, StreamEvent{
			Event: "content_block_stop",
			Data: map[string]any{
				"type":  "content_block_stop",
				"index": index,
			},
		})
	}
	events = append(events,
		StreamEvent{
			Event: "message_delta",
			Data: map[string]any{
				"type": "message_delta",
				"delta": map[string]any{
					"stop_reason":   anthropicStopReason(resp.StopReason),
					"stop_sequence": nil,
				},
				"usage": anthropicStreamUsage(nil, ptrInt(resp.Usage.OutputTokens), 0),
			},
		},
		StreamEvent{
			Event: "message_stop",
			Data: map[string]any{
				"type": "message_stop",
			},
		},
	)
	return events
}

func (s *Service) renderAnthropicCanonicalStreamEvents(resp gatewaycontract.CanonicalResponse) []StreamEvent {
	events := normalizeStreamEvents(resp.StreamEvents)
	if !streamEventsHaveRenderableOutput(events) {
		return nil
	}
	out := []StreamEvent{{
		Event: "message_start",
		Data: map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id":            "msg_" + responseID(resp),
				"type":          "message",
				"role":          "assistant",
				"model":         resp.Model,
				"content":       []any{},
				"stop_reason":   nil,
				"stop_sequence": nil,
				"usage":         anthropicStreamUsage(ptrInt(resp.Usage.InputTokens), ptrInt(0), resp.Usage.CachedTokens),
			},
		},
	}}
	openBlocks := map[int]bool{}
	openBlockOrder := make([]int, 0)
	toolStates := newStreamToolCallStates(resp.OutputItems)
	textStates := newResponseStreamTextStates(resp.OutputItems)
	var pendingUsage *gatewaycontract.Usage
	pendingStopReason := ""
	for _, event := range events {
		switch event.Type {
		case gatewaycontract.StreamEventContentDelta, gatewaycontract.StreamEventReasoning, gatewaycontract.StreamEventToolCallDelta, gatewaycontract.StreamEventToolResult:
			if event.Type == gatewaycontract.StreamEventReasoning && mapStringAny(event.Delta.Metadata, "signature_delta") != "" {
				signature := textStates.appendSignature(event)
				out = append(out, StreamEvent{
					Event: "content_block_delta",
					Data: map[string]any{
						"type":  "content_block_delta",
						"index": event.ContentIndex,
						"delta": map[string]any{
							"type":      "signature_delta",
							"signature": signature,
						},
					},
				})
				continue
			}
			if event.Type == gatewaycontract.StreamEventReasoning {
				_ = textStates.stateFor(event, gatewaycontract.ContentBlockReasoning)
			}
			startBlock := anthropicStreamEventStartBlock(event)
			if event.Type == gatewaycontract.StreamEventToolCallDelta {
				state := toolStates.stateFor(event)
				startBlock = state.startBlock()
			}
			if !openBlocks[event.ContentIndex] {
				openBlocks[event.ContentIndex] = true
				openBlockOrder = append(openBlockOrder, event.ContentIndex)
				out = append(out, StreamEvent{
					Event: "content_block_start",
					Data: map[string]any{
						"type":          "content_block_start",
						"index":         event.ContentIndex,
						"content_block": anthropicStreamContentBlock(startBlock),
					},
				})
			}
			if delta := anthropicStreamEventDelta(event); len(delta) > 0 {
				out = append(out, StreamEvent{
					Event: "content_block_delta",
					Data: map[string]any{
						"type":  "content_block_delta",
						"index": event.ContentIndex,
						"delta": delta,
					},
				})
			}
		case gatewaycontract.StreamEventUsage:
			copied := event.Usage
			pendingUsage = &copied
		case gatewaycontract.StreamEventStop:
			pendingStopReason = firstNonEmpty(event.StopReason, pendingStopReason)
		}
	}
	for _, index := range openBlockOrder {
		out = append(out, StreamEvent{
			Event: "content_block_stop",
			Data: map[string]any{
				"type":  "content_block_stop",
				"index": index,
			},
		})
	}
	if pendingUsage != nil || pendingStopReason != "" {
		outputTokens := resp.Usage.OutputTokens
		if pendingUsage != nil {
			outputTokens = pendingUsage.OutputTokens
		}
		delta := map[string]any{}
		if pendingStopReason != "" {
			delta["stop_reason"] = anthropicStopReason(firstNonEmpty(pendingStopReason, resp.StopReason))
			delta["stop_sequence"] = nil
		}
		out = append(out, StreamEvent{
			Event: "message_delta",
			Data: map[string]any{
				"type":  "message_delta",
				"delta": delta,
				"usage": anthropicStreamUsage(nil, ptrInt(outputTokens), 0),
			},
		})
	}
	out = append(out, StreamEvent{
		Event: "message_stop",
		Data: map[string]any{
			"type": "message_stop",
		},
	})
	return out
}

func anthropicStreamUsage(inputTokens *int, outputTokens *int, cachedTokens int) map[string]any {
	usage := map[string]any{}
	if inputTokens != nil {
		usage["input_tokens"] = *inputTokens
	}
	if outputTokens != nil {
		usage["output_tokens"] = *outputTokens
	}
	if cachedTokens > 0 {
		usage["cache_read_input_tokens"] = cachedTokens
	}
	return usage
}

func anthropicStreamEventStartBlock(event gatewaycontract.StreamEvent) gatewaycontract.ContentBlock {
	block := event.Delta
	switch event.Type {
	case gatewaycontract.StreamEventToolCallDelta:
		block.Type = gatewaycontract.ContentBlockToolCall
		block.ToolArgumentsJSON = ""
	case gatewaycontract.StreamEventReasoning:
		block.Type = gatewaycontract.ContentBlockReasoning
		block.Text = ""
	default:
		block.Type = gatewaycontract.ContentBlockText
		block.Text = ""
	}
	return block
}

func anthropicStreamEventDelta(event gatewaycontract.StreamEvent) map[string]any {
	switch event.Type {
	case gatewaycontract.StreamEventToolCallDelta:
		if args := event.Delta.ToolArgumentsJSON; args != "" {
			return map[string]any{
				"type":         "input_json_delta",
				"partial_json": args,
			}
		}
	case gatewaycontract.StreamEventReasoning:
		if text := event.Delta.Text; text != "" {
			return map[string]any{
				"type": "thinking_delta",
				"text": text,
			}
		}
	default:
		if text := event.Delta.Text; text != "" {
			return map[string]any{
				"type": "text_delta",
				"text": text,
			}
		}
	}
	return nil
}

func (s *Service) RenderGeminiGenerateContentStreamEvents(resp gatewaycontract.CanonicalResponse) []StreamEvent {
	if events := s.renderGeminiCanonicalStreamEvents(resp); len(events) > 0 {
		return events
	}
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

func (s *Service) renderGeminiCanonicalStreamEvents(resp gatewaycontract.CanonicalResponse) []StreamEvent {
	events := normalizeStreamEvents(resp.StreamEvents)
	if !streamEventsCanRenderGemini(events) {
		return nil
	}
	out := make([]StreamEvent, 0, len(events))
	for _, event := range events {
		switch event.Type {
		case gatewaycontract.StreamEventContentDelta, gatewaycontract.StreamEventReasoning, gatewaycontract.StreamEventToolCallDelta, gatewaycontract.StreamEventToolResult:
			part, ok := outputGeminiStreamPart(event.Delta)
			if !ok {
				continue
			}
			out = append(out, StreamEvent{Data: map[string]any{
				"candidates": []apiopenapi.GeminiCandidate{{
					Index: event.ContentIndex,
					Content: apiopenapi.GeminiContent{
						Parts: []apiopenapi.GeminiPart{part},
					},
				}},
			}})
		case gatewaycontract.StreamEventUsage:
			out = append(out, StreamEvent{Data: map[string]any{
				"candidates":    []apiopenapi.GeminiCandidate{},
				"usageMetadata": geminiUsage(event.Usage),
			}})
		case gatewaycontract.StreamEventStop:
			out = append(out, StreamEvent{Data: map[string]any{
				"candidates": []apiopenapi.GeminiCandidate{{
					Index:        event.ContentIndex,
					FinishReason: geminiFinishReason(firstNonEmpty(event.StopReason, resp.StopReason)),
					Content: apiopenapi.GeminiContent{
						Parts: []apiopenapi.GeminiPart{},
					},
				}},
			}})
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func outputGeminiStreamPart(block gatewaycontract.ContentBlock) (apiopenapi.GeminiPart, bool) {
	part := apiopenapi.GeminiPart{}
	switch block.Type {
	case gatewaycontract.ContentBlockToolCall:
		call := outputBlockProperties(block)
		if args := parseJSONObject(block.ToolArgumentsJSON); len(args) > 0 {
			call["args"] = args
		}
		functionCall := apiopenapi.JsonObject(call)
		part.FunctionCall = &functionCall
	case gatewaycontract.ContentBlockToolResult:
		response := outputBlockProperties(block)
		if block.Text != "" {
			response["response"] = block.Text
		}
		functionResponse := apiopenapi.JsonObject(response)
		part.FunctionResponse = &functionResponse
	default:
		text := block.Text
		if text == "" {
			return apiopenapi.GeminiPart{}, false
		}
		part.Text = &text
	}
	return part, true
}
