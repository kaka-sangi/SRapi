package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func outputTextFromBlocks(blocks []gatewaycontract.ContentBlock) string {
	var parts []string
	for _, block := range blocks {
		switch block.Type {
		case "", gatewaycontract.ContentBlockText, gatewaycontract.ContentBlockRefusal, gatewaycontract.ContentBlockToolResult:
			if text := strings.TrimSpace(block.Text); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func responsesStatus(stopReason string) string {
	switch strings.TrimSpace(stopReason) {
	case "max_tokens":
		return "incomplete"
	default:
		return "completed"
	}
}

func responsesIncompleteDetails(stopReason string) *apiopenapi.ResponsesIncompleteDetails {
	switch strings.TrimSpace(stopReason) {
	case "max_tokens":
		return &apiopenapi.ResponsesIncompleteDetails{Reason: "max_output_tokens"}
	default:
		return nil
	}
}

func responsesTerminalEventName(stopReason string) string {
	switch responsesStatus(stopReason) {
	case "incomplete":
		return "response.incomplete"
	default:
		return "response.completed"
	}
}

func normalizeOutputItems(blocks []gatewaycontract.ContentBlock) []gatewaycontract.ContentBlock {
	out := make([]gatewaycontract.ContentBlock, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "" {
			block.Type = gatewaycontract.ContentBlockText
		}
		if strings.TrimSpace(block.Role) == "" {
			block.Role = "assistant"
		}
		block.Text = strings.TrimSpace(block.Text)
		block.Metadata = cloneMap(block.Metadata)
		block.Raw = append([]byte(nil), block.Raw...)
		block.OriginProtocol = strings.TrimSpace(block.OriginProtocol)
		out = append(out, block)
	}
	if len(out) == 0 {
		return []gatewaycontract.ContentBlock{{Type: gatewaycontract.ContentBlockText, Role: "assistant"}}
	}
	return out
}

func normalizeStreamEvents(events []gatewaycontract.StreamEvent) []gatewaycontract.StreamEvent {
	out := make([]gatewaycontract.StreamEvent, 0, len(events))
	for _, event := range events {
		event.RawEventType = strings.TrimSpace(event.RawEventType)
		event.Raw = append([]byte(nil), event.Raw...)
		event.OriginProtocol = strings.TrimSpace(event.OriginProtocol)
		event.Metadata = cloneMap(event.Metadata)
		event.Delta = normalizeStreamDelta(event.Delta)
		out = append(out, event)
	}
	return out
}

func normalizeStreamDelta(block gatewaycontract.ContentBlock) gatewaycontract.ContentBlock {
	if block.Type == "" {
		block.Type = gatewaycontract.ContentBlockText
	}
	if strings.TrimSpace(block.Role) == "" {
		block.Role = "assistant"
	}
	block.Metadata = cloneMap(block.Metadata)
	block.Raw = append([]byte(nil), block.Raw...)
	block.OriginProtocol = strings.TrimSpace(block.OriginProtocol)
	return block
}

func chatContentShouldRenderAsBlocks(blocks []gatewaycontract.ContentBlock) bool {
	for _, block := range blocks {
		if block.Type == gatewaycontract.ContentBlockReasoning {
			continue
		}
		if block.Type != "" && block.Type != gatewaycontract.ContentBlockText {
			return true
		}
		if len(block.Metadata) > 0 {
			return true
		}
	}
	return false
}

func chatMessageContentBlocks(blocks []gatewaycontract.ContentBlock) []gatewaycontract.ContentBlock {
	blocks = normalizeOutputItems(blocks)
	out := make([]gatewaycontract.ContentBlock, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == gatewaycontract.ContentBlockToolCall {
			continue
		}
		out = append(out, block)
	}
	if len(out) == 0 {
		return []gatewaycontract.ContentBlock{{Type: gatewaycontract.ContentBlockText, Role: "assistant"}}
	}
	return out
}

func openAIReasoningContentFromBlocks(blocks []gatewaycontract.ContentBlock) string {
	var parts []string
	for _, block := range normalizeOutputItems(blocks) {
		if block.Type != gatewaycontract.ContentBlockReasoning {
			continue
		}
		if text := strings.TrimSpace(block.Text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func outputOpenAIChatToolCalls(blocks []gatewaycontract.ContentBlock) []apiopenapi.ChatToolCall {
	blocks = normalizeOutputItems(blocks)
	out := make([]apiopenapi.ChatToolCall, 0)
	for _, block := range blocks {
		if block.Type != gatewaycontract.ContentBlockToolCall {
			continue
		}
		function := apiopenapi.JsonObject{}
		if name := strings.TrimSpace(block.ToolName); name != "" {
			function["name"] = name
		}
		function["arguments"] = strings.TrimSpace(block.ToolArgumentsJSON)
		callType := "function"
		if value, ok := block.Metadata["type"].(string); ok && strings.TrimSpace(value) != "" {
			callType = strings.TrimSpace(value)
		}
		out = append(out, apiopenapi.ChatToolCall{
			Id:       strings.TrimSpace(block.ToolCallID),
			Type:     callType,
			Function: function,
		})
	}
	return out
}

func responseOutputItems(blocks []gatewaycontract.ContentBlock) []apiopenapi.ResponsesOutputItem {
	blocks = normalizeOutputItems(blocks)
	role := "assistant"
	var messageBlocks []gatewaycontract.ContentBlock
	out := make([]apiopenapi.ResponsesOutputItem, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == gatewaycontract.ContentBlockToolResult {
			if item, ok := responseFunctionCallOutputItem(block); ok {
				out = append(out, item)
			}
			continue
		}
		if block.Type != gatewaycontract.ContentBlockToolCall {
			messageBlocks = append(messageBlocks, block)
			continue
		}
		if isHostedWebSearchBlock(block) {
			props := hostedWebSearchOutputItem(block)
			out = append(out, apiopenapi.ResponsesOutputItem{
				Type:                 responsesWebSearchCallType,
				AdditionalProperties: props,
			})
			continue
		}
		props := outputBlockProperties(block)
		props["status"] = "completed"
		out = append(out, apiopenapi.ResponsesOutputItem{
			Type:                 "function_call",
			AdditionalProperties: props,
		})
	}
	if len(messageBlocks) > 0 {
		content := outputResponsesContentBlocks(messageBlocks)
		out = append([]apiopenapi.ResponsesOutputItem{{
			Type:    "message",
			Role:    &role,
			Content: &content,
		}}, out...)
	}
	if len(out) == 0 {
		content := outputResponsesContentBlocks(nil)
		out = append(out, apiopenapi.ResponsesOutputItem{Type: "message", Role: &role, Content: &content})
	}
	return out
}

func responseFunctionCallOutputItem(block gatewaycontract.ContentBlock) (apiopenapi.ResponsesOutputItem, bool) {
	callID := strings.TrimSpace(firstNonEmpty(block.ToolResultForID, block.ToolCallID))
	if callID == "" {
		return apiopenapi.ResponsesOutputItem{}, false
	}
	props := outputBlockProperties(block)
	props["call_id"] = callID
	props["output"] = strings.TrimSpace(block.Text)
	delete(props, "tool_result_for_id")
	if block.ToolResultIsError {
		props["is_error"] = true
	}
	return apiopenapi.ResponsesOutputItem{
		Type:                 "function_call_output",
		AdditionalProperties: props,
	}, true
}

func outputResponsesContentBlocks(blocks []gatewaycontract.ContentBlock) []apiopenapi.ContentBlock {
	blocks = normalizeOutputItems(blocks)
	out := make([]apiopenapi.ContentBlock, 0, len(blocks))
	for _, block := range blocks {
		item := apiopenapi.ContentBlock{
			Type:                 apiopenapi.ContentBlockType(responseStreamContentPartType(block.Type)),
			AdditionalProperties: outputBlockProperties(block),
		}
		if block.Type == gatewaycontract.ContentBlockRefusal {
			setStringProperty(item.AdditionalProperties, "refusal", block.Text)
		} else if block.Type != gatewaycontract.ContentBlockToolCall {
			if text := strings.TrimSpace(block.Text); text != "" {
				item.Text = &text
			}
		}
		out = append(out, item)
	}
	return out
}

func outputOpenAIContentBlocks(blocks []gatewaycontract.ContentBlock) []apiopenapi.ContentBlock {
	blocks = normalizeOutputItems(blocks)
	out := make([]apiopenapi.ContentBlock, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == gatewaycontract.ContentBlockReasoning {
			continue
		}
		item := apiopenapi.ContentBlock{
			Type:                 openAIContentBlockType(block.Type),
			AdditionalProperties: outputBlockProperties(block),
		}
		if block.Type != gatewaycontract.ContentBlockToolCall {
			if text := strings.TrimSpace(block.Text); text != "" {
				item.Text = &text
			}
		}
		out = append(out, item)
	}
	return out
}

func chatStreamToolCalls(blocks []gatewaycontract.ContentBlock) []map[string]any {
	var out []map[string]any
	index := 0
	for _, block := range blocks {
		if block.Type != gatewaycontract.ContentBlockToolCall {
			continue
		}
		call := map[string]any{
			"index": index,
			"id":    strings.TrimSpace(block.ToolCallID),
			"type":  "function",
			"function": map[string]any{
				"name":      strings.TrimSpace(block.ToolName),
				"arguments": strings.TrimSpace(block.ToolArgumentsJSON),
			},
		}
		out = append(out, call)
		index++
	}
	return out
}

func responseStreamOutputEvents(blocks []gatewaycontract.ContentBlock) []StreamEvent {
	blocks = normalizeOutputItems(blocks)
	events := make([]StreamEvent, 0, len(blocks)*4)
	for outputIndex, block := range blocks {
		itemID := responseStreamItemID(outputIndex, block)
		if block.Type == gatewaycontract.ContentBlockToolCall {
			if isHostedWebSearchBlock(block) {
				item := hostedWebSearchOutputItem(block)
				item["id"] = itemID
				events = append(events,
					StreamEvent{
						Event: "response.output_item.added",
						Data: map[string]any{
							"type":         "response.output_item.added",
							"output_index": outputIndex,
							"item":         item,
						},
					},
					StreamEvent{
						Event: "response.output_item.done",
						Data: map[string]any{
							"type":         "response.output_item.done",
							"output_index": outputIndex,
							"item":         item,
						},
					},
				)
				continue
			}
			item := responseStreamFunctionCallItem(itemID, block)
			events = append(events, StreamEvent{
				Event: "response.output_item.added",
				Data: map[string]any{
					"type":         "response.output_item.added",
					"output_index": outputIndex,
					"item":         item,
				},
			})
			if args := strings.TrimSpace(block.ToolArgumentsJSON); args != "" {
				events = append(events, StreamEvent{
					Event: "response.function_call_arguments.delta",
					Data: map[string]any{
						"type":         "response.function_call_arguments.delta",
						"item_id":      itemID,
						"output_index": outputIndex,
						"delta":        args,
					},
				})
			}
			events = append(events, StreamEvent{
				Event: "response.function_call_arguments.done",
				Data: map[string]any{
					"type":         "response.function_call_arguments.done",
					"item_id":      itemID,
					"output_index": outputIndex,
					"arguments":    strings.TrimSpace(block.ToolArgumentsJSON),
				},
			})
			continue
		}

		contentPart := responseStreamContentPart(block)
		events = append(events,
			StreamEvent{
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
			StreamEvent{
				Event: "response.content_part.added",
				Data: map[string]any{
					"type":          "response.content_part.added",
					"item_id":       itemID,
					"output_index":  outputIndex,
					"content_index": 0,
					"part":          contentPart,
				},
			},
		)
		if text := strings.TrimSpace(block.Text); text != "" {
			events = append(events, responseStreamTextDeltaEvent(itemID, outputIndex, block.Type, text))
		}
		events = append(events,
			responseStreamTextDoneEvent(itemID, outputIndex, block.Type, strings.TrimSpace(block.Text)),
			responseStreamContentPartDoneEvent(itemID, outputIndex, block.Type, strings.TrimSpace(block.Text)),
			responseStreamMessageDoneEvent(itemID, outputIndex, block.Type, strings.TrimSpace(block.Text)),
		)
	}
	return events
}

type responseStreamDoneEventGroup struct {
	OutputIndex int
	Events      []StreamEvent
}

func sortResponseStreamDoneEventGroups(groups []responseStreamDoneEventGroup) {
	sort.SliceStable(groups, func(i, j int) bool {
		return groups[i].OutputIndex < groups[j].OutputIndex
	})
}

type responseStreamTextStates struct {
	byKey map[responseStreamTextStateKey]*responseStreamTextState
	order []*responseStreamTextState
}

type responseStreamTextStateKey struct {
	ContentIndex int
	BlockType    gatewaycontract.ContentBlockType
}

type responseStreamTextState struct {
	ContentIndex int
	OutputIndex  int
	ItemID       string
	BlockType    gatewaycontract.ContentBlockType
	Text         strings.Builder
	Signature    strings.Builder
}

func newResponseStreamTextStates(blocks []gatewaycontract.ContentBlock) *responseStreamTextStates {
	states := &responseStreamTextStates{
		byKey: map[responseStreamTextStateKey]*responseStreamTextState{},
		order: make([]*responseStreamTextState, 0),
	}
	for index, block := range normalizeOutputItems(blocks) {
		if !responseStreamBlockHasTextPart(block.Type) {
			continue
		}
		state := &responseStreamTextState{
			ContentIndex: index,
			OutputIndex:  -1,
			ItemID:       responseStreamTextItemID(index, block.Type),
			BlockType:    block.Type,
		}
		states.byKey[responseStreamTextStateKey{ContentIndex: index, BlockType: block.Type}] = state
		states.order = append(states.order, state)
	}
	return states
}

func (s *responseStreamTextStates) stateFor(event gatewaycontract.StreamEvent, fallbackType gatewaycontract.ContentBlockType) *responseStreamTextState {
	key := responseStreamTextStateKey{ContentIndex: event.ContentIndex, BlockType: fallbackType}
	if state := s.byKey[key]; state != nil {
		return state
	}
	state := &responseStreamTextState{
		ContentIndex: event.ContentIndex,
		OutputIndex:  -1,
		ItemID:       responseStreamTextItemID(len(s.order), fallbackType),
		BlockType:    fallbackType,
	}
	s.byKey[key] = state
	s.order = append(s.order, state)
	return state
}

func (s *responseStreamTextStates) openStates() []*responseStreamTextState {
	out := make([]*responseStreamTextState, 0, len(s.order))
	for _, state := range s.order {
		if state.OutputIndex >= 0 {
			out = append(out, state)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].OutputIndex < out[j].OutputIndex
	})
	return out
}

func (s *responseStreamTextStates) appendSignature(event gatewaycontract.StreamEvent) string {
	state := s.stateFor(event, gatewaycontract.ContentBlockReasoning)
	state.Signature.WriteString(mapStringAny(event.Delta.Metadata, "signature_delta"))
	return state.Signature.String()
}

func responseStreamDeltaTextBlockType(event gatewaycontract.StreamEvent, fallback gatewaycontract.ContentBlockType) gatewaycontract.ContentBlockType {
	if event.Type == gatewaycontract.StreamEventToolResult {
		return gatewaycontract.ContentBlockToolResult
	}
	if responseStreamBlockHasTextPart(event.Delta.Type) {
		return event.Delta.Type
	}
	return fallback
}

func responseStreamBlockHasTextPart(blockType gatewaycontract.ContentBlockType) bool {
	switch blockType {
	case gatewaycontract.ContentBlockText, gatewaycontract.ContentBlockReasoning, gatewaycontract.ContentBlockRefusal, gatewaycontract.ContentBlockToolResult:
		return true
	default:
		return false
	}
}

func responseStreamTextItemID(index int, blockType gatewaycontract.ContentBlockType) string {
	if blockType == gatewaycontract.ContentBlockReasoning {
		return fmt.Sprintf("rs_%d", index)
	}
	return fmt.Sprintf("msg_%d", index)
}

func responseStreamItemID(index int, block gatewaycontract.ContentBlock) string {
	if block.ToolCallID != "" {
		return strings.TrimSpace(block.ToolCallID)
	}
	return fmt.Sprintf("msg_%d", index)
}

func responseStreamFunctionCallItem(itemID string, block gatewaycontract.ContentBlock) map[string]any {
	item := outputBlockProperties(block)
	item["id"] = itemID
	item["type"] = "function_call"
	item["status"] = "completed"
	setStringProperty(item, "call_id", block.ToolCallID)
	setStringProperty(item, "name", block.ToolName)
	setStringProperty(item, "arguments", block.ToolArgumentsJSON)
	return item
}

func responseStreamContentPart(block gatewaycontract.ContentBlock) map[string]any {
	part := outputBlockProperties(block)
	part["type"] = responseStreamContentPartType(block.Type)
	if block.Type == gatewaycontract.ContentBlockRefusal {
		setStringProperty(part, "refusal", block.Text)
	} else {
		setStringProperty(part, "text", block.Text)
	}
	return part
}

func responseStreamContentPartType(value gatewaycontract.ContentBlockType) string {
	switch value {
	case gatewaycontract.ContentBlockReasoning:
		return "reasoning_text"
	case gatewaycontract.ContentBlockRefusal:
		return "refusal"
	case gatewaycontract.ContentBlockToolResult:
		return "tool_result"
	default:
		return "output_text"
	}
}

func outputAnthropicContentBlocks(blocks []gatewaycontract.ContentBlock) []apiopenapi.AnthropicContentBlock {
	blocks = normalizeOutputItems(blocks)
	out := make([]apiopenapi.AnthropicContentBlock, 0, len(blocks))
	for _, block := range blocks {
		item := apiopenapi.AnthropicContentBlock{
			Type:                 outputAnthropicBlockType(block),
			AdditionalProperties: outputAnthropicBlockProperties(block),
		}
		if block.Type == gatewaycontract.ContentBlockToolCall {
			setStringProperty(item.AdditionalProperties, "id", block.ToolCallID)
			setStringProperty(item.AdditionalProperties, "name", block.ToolName)
			if input := parseJSONObject(block.ToolArgumentsJSON); len(input) > 0 {
				item.Set("input", input)
			}
		} else if block.Type == gatewaycontract.ContentBlockReasoning {
			if text := strings.TrimSpace(block.Text); text != "" {
				item.Set("thinking", text)
			}
		} else {
			if text := strings.TrimSpace(block.Text); text != "" {
				item.Text = &text
			}
		}
		out = append(out, item)
	}
	return out
}

func anthropicStreamContentBlock(block gatewaycontract.ContentBlock) map[string]any {
	contentBlock := outputAnthropicBlockProperties(block)
	contentBlock["type"] = string(outputAnthropicBlockType(block))
	switch block.Type {
	case gatewaycontract.ContentBlockToolCall:
		setStringProperty(contentBlock, "id", block.ToolCallID)
		setStringProperty(contentBlock, "name", block.ToolName)
		if input := parseJSONObject(block.ToolArgumentsJSON); len(input) > 0 {
			contentBlock["input"] = input
		} else {
			contentBlock["input"] = map[string]any{}
		}
	case gatewaycontract.ContentBlockToolResult:
		setStringProperty(contentBlock, "tool_use_id", block.ToolResultForID)
		if text := strings.TrimSpace(block.Text); text != "" {
			contentBlock["content"] = text
		}
	case gatewaycontract.ContentBlockReasoning:
		if anthropicReasoningBlockType(block) != "redacted_thinking" {
			setStringProperty(contentBlock, "thinking", block.Text)
		}
	default:
		setStringProperty(contentBlock, "text", block.Text)
	}
	return contentBlock
}

func anthropicStreamContentDelta(block gatewaycontract.ContentBlock) map[string]any {
	switch block.Type {
	case gatewaycontract.ContentBlockToolCall:
		if args := strings.TrimSpace(block.ToolArgumentsJSON); args != "" {
			return map[string]any{
				"type":         "input_json_delta",
				"partial_json": args,
			}
		}
	case gatewaycontract.ContentBlockReasoning:
		if anthropicReasoningBlockType(block) == "redacted_thinking" {
			return nil
		}
		if text := strings.TrimSpace(block.Text); text != "" {
			return map[string]any{
				"type": "thinking_delta",
				"text": text,
			}
		}
	default:
		if text := strings.TrimSpace(block.Text); text != "" {
			return map[string]any{
				"type": "text_delta",
				"text": text,
			}
		}
	}
	return nil
}

func outputGeminiParts(blocks []gatewaycontract.ContentBlock) []apiopenapi.GeminiPart {
	blocks = normalizeOutputItems(blocks)
	out := make([]apiopenapi.GeminiPart, 0, len(blocks))
	for _, block := range blocks {
		part := apiopenapi.GeminiPart{AdditionalProperties: outputGeminiPartProperties(block)}
		switch block.Type {
		case gatewaycontract.ContentBlockToolCall:
			call := outputGeminiPartProperties(block)
			if args := parseJSONObject(block.ToolArgumentsJSON); len(args) > 0 {
				call["args"] = args
			}
			functionCall := apiopenapi.JsonObject(call)
			part.FunctionCall = &functionCall
		case gatewaycontract.ContentBlockToolResult:
			response := outputBlockProperties(block)
			if text := strings.TrimSpace(block.Text); text != "" {
				response["response"] = text
			}
			functionResponse := apiopenapi.JsonObject(response)
			part.FunctionResponse = &functionResponse
		default:
			text := strings.TrimSpace(block.Text)
			part.Text = &text
		}
		out = append(out, part)
	}
	return out
}

func outputGeminiPartProperties(block gatewaycontract.ContentBlock) map[string]any {
	props := outputBlockProperties(block)
	if signature := firstNonEmpty(mapStringAny(block.Metadata, "thoughtSignature"), mapStringAny(block.Metadata, "signature")); signature != "" {
		props["thoughtSignature"] = signature
	}
	return props
}

func outputBlockProperties(block gatewaycontract.ContentBlock) map[string]any {
	props := cloneMap(block.Metadata)
	if props == nil {
		props = map[string]any{}
	}
	if block.Type != "" {
		props["srapi_type"] = string(block.Type)
	}
	setStringProperty(props, "media_url", block.MediaURL)
	setStringProperty(props, "media_base64", block.MediaBase64)
	setStringProperty(props, "mime_type", block.MIMEType)
	setStringProperty(props, "file_id", block.FileID)
	setStringProperty(props, "id", block.ToolCallID)
	setStringProperty(props, "call_id", block.ToolCallID)
	setStringProperty(props, "name", block.ToolName)
	setStringProperty(props, "arguments", block.ToolArgumentsJSON)
	setStringProperty(props, "tool_result_for_id", block.ToolResultForID)
	if block.ToolResultIsError {
		props["is_error"] = true
	}
	return props
}

func setStringProperty(values map[string]any, key string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	values[key] = value
}

func parseJSONObject(value string) map[string]any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		return nil
	}
	return parsed
}

func openAIContentBlockType(value gatewaycontract.ContentBlockType) apiopenapi.ContentBlockType {
	switch value {
	case gatewaycontract.ContentBlockImage:
		return apiopenapi.ContentBlockTypeImageUrl
	case gatewaycontract.ContentBlockToolCall:
		return apiopenapi.ContentBlockTypeToolCall
	case gatewaycontract.ContentBlockToolResult:
		return apiopenapi.ContentBlockTypeToolResult
	default:
		return apiopenapi.ContentBlockTypeText
	}
}

func anthropicContentBlockType(value gatewaycontract.ContentBlockType) apiopenapi.AnthropicContentBlockType {
	switch value {
	case gatewaycontract.ContentBlockImage:
		return apiopenapi.AnthropicContentBlockTypeImage
	case gatewaycontract.ContentBlockReasoning:
		return apiopenapi.AnthropicContentBlockTypeThinking
	case gatewaycontract.ContentBlockToolCall:
		return apiopenapi.AnthropicContentBlockTypeToolUse
	case gatewaycontract.ContentBlockToolResult:
		return apiopenapi.AnthropicContentBlockTypeToolResult
	default:
		return apiopenapi.AnthropicContentBlockTypeText
	}
}

func outputAnthropicBlockType(block gatewaycontract.ContentBlock) apiopenapi.AnthropicContentBlockType {
	if block.Type == gatewaycontract.ContentBlockReasoning && anthropicReasoningBlockType(block) == "redacted_thinking" {
		return apiopenapi.AnthropicContentBlockTypeRedactedThinking
	}
	return anthropicContentBlockType(block.Type)
}

func outputAnthropicBlockProperties(block gatewaycontract.ContentBlock) map[string]any {
	props := map[string]any{}
	switch block.Type {
	case gatewaycontract.ContentBlockReasoning:
		switch anthropicReasoningBlockType(block) {
		case "redacted_thinking":
			setStringProperty(props, "data", firstNonEmpty(mapStringAny(block.Metadata, "data"), block.Text))
		default:
			setStringProperty(props, "signature", mapStringAny(block.Metadata, "signature"))
		}
	case gatewaycontract.ContentBlockText, gatewaycontract.ContentBlockImage, gatewaycontract.ContentBlockToolCall, gatewaycontract.ContentBlockToolResult:
		if value, ok := block.Metadata["cache_control"]; ok && value != nil {
			props["cache_control"] = cloneAny(value)
		}
		if value, ok := block.Metadata["citations"]; ok && value != nil {
			props["citations"] = cloneAny(value)
		}
	}
	return props
}

func anthropicReasoningBlockType(block gatewaycontract.ContentBlock) string {
	blockType := strings.TrimSpace(mapStringAny(block.Metadata, "type"))
	if blockType == "redacted_thinking" {
		return "redacted_thinking"
	}
	return "thinking"
}

func openAIChatFinishReason(value string) string {
	switch strings.TrimSpace(value) {
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	case "content_filter", "refusal":
		return "content_filter"
	default:
		return "stop"
	}
}

func anthropicStopReason(value string) string {
	switch strings.TrimSpace(value) {
	case "max_tokens":
		return "max_tokens"
	case "tool_use":
		return "tool_use"
	case "content_filter", "refusal":
		return "refusal"
	default:
		return "end_turn"
	}
}

func geminiFinishReason(value string) string {
	switch strings.TrimSpace(value) {
	case "max_tokens":
		return "MAX_TOKENS"
	case "content_filter", "refusal":
		return "SAFETY"
	default:
		return "STOP"
	}
}

func rawResponsesInput(rawBody []byte) ([]gatewaycontract.ContentBlock, string, []string) {
	var payload map[string]any
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return nil, "", nil
	}
	blocks, instructions, warnings := rawResponsesInputValue(payload["input"], "user")
	return blocks, strings.Join(uniqueStrings(instructions), "\n"), uniqueStrings(warnings)
}

func validateRawResponsesInput(rawBody []byte) error {
	var payload map[string]any
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return nil
	}
	return validateRawResponsesInputValue(payload["input"])
}

func validateRawResponsesInputValue(value any) error {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if err := validateRawResponsesInputValue(item); err != nil {
				return err
			}
		}
	case map[string]any:
		if strings.TrimSpace(rawMapString(typed, "type")) == "function_call_output" && strings.TrimSpace(rawMapString(typed, "call_id")) == "" {
			return fmt.Errorf("Responses function_call_output input item requires call_id")
		}
		return validateRawResponsesInputValue(typed["content"])
	}
	return nil
}

func rawResponsesInputValue(value any, defaultRole string) ([]gatewaycontract.ContentBlock, []string, []string) {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil, nil, nil
		}
		return []gatewaycontract.ContentBlock{{Type: gatewaycontract.ContentBlockText, Role: defaultRole, Text: text}}, nil, nil
	case []any:
		var blocks []gatewaycontract.ContentBlock
		var instructions []string
		var warnings []string
		for _, item := range typed {
			itemBlocks, itemInstructions, itemWarnings := rawResponsesInputValue(item, defaultRole)
			blocks = append(blocks, itemBlocks...)
			instructions = append(instructions, itemInstructions...)
			warnings = append(warnings, itemWarnings...)
		}
		return blocks, instructions, warnings
	case map[string]any:
		return rawResponsesInputObject(typed, defaultRole)
	default:
		return nil, nil, nil
	}
}

func rawResponsesInputObject(value map[string]any, defaultRole string) ([]gatewaycontract.ContentBlock, []string, []string) {
	role := strings.TrimSpace(rawMapString(value, "role"))
	switch rawType := strings.TrimSpace(rawMapString(value, "type")); rawType {
	case "function_call":
		role = firstNonEmpty(role, "assistant")
		if block, ok := rawResponsesFunctionCallBlock(value, role); ok {
			return []gatewaycontract.ContentBlock{block}, nil, nil
		}
		return nil, nil, nil
	case "function_call_output":
		role = firstNonEmpty(role, "tool")
		if block, ok := rawResponsesFunctionCallOutputBlock(value, role); ok {
			return []gatewaycontract.ContentBlock{block}, nil, nil
		}
		return nil, nil, nil
	case "input_image", "image_url":
		if block, ok := rawResponsesImageBlock(value, role); ok {
			return []gatewaycontract.ContentBlock{block}, nil, nil
		}
		return nil, nil, nil
	case "input_text", "output_text":
		role = firstNonEmpty(role, defaultRole)
		if text := strings.TrimSpace(rawMapString(value, "text")); text != "" {
			block := gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockText, Role: role, Text: text, Metadata: cloneMap(value), OriginProtocol: string(gatewaycontract.ProtocolOpenAICompatible), Raw: marshalRawJSON(value)}
			if role == "system" || role == "developer" {
				return nil, []string{text}, nil
			}
			return []gatewaycontract.ContentBlock{block}, nil, nil
		}
	case "message":
	default:
		if rawType != "" {
			if block, ok := rawResponsesContextBlock(value, role, rawType); ok {
				return []gatewaycontract.ContentBlock{block}, nil, nil
			}
		}
	}
	role = firstNonEmpty(role, defaultRole)
	if text := strings.TrimSpace(rawMapString(value, "text")); text != "" {
		block := gatewaycontract.ContentBlock{Type: gatewaycontract.ContentBlockText, Role: role, Text: text}
		if role == "system" || role == "developer" {
			return nil, []string{text}, nil
		}
		return []gatewaycontract.ContentBlock{block}, nil, nil
	}
	blocks, instructions, warnings := rawResponsesInputValue(value["content"], role)
	if role == "system" || role == "developer" {
		text := textFromBlocks(blocks)
		if text == "" {
			return nil, instructions, warnings
		}
		return nil, append(instructions, text), warnings
	}
	return blocks, instructions, warnings
}

func rawResponsesContextBlock(value map[string]any, role string, rawType string) (gatewaycontract.ContentBlock, bool) {
	raw := marshalRawJSON(value)
	if len(raw) == 0 {
		return gatewaycontract.ContentBlock{}, false
	}
	if role == "" {
		role = "assistant"
	}
	metadata := cloneMap(value)
	metadata["responses_item_type"] = rawType
	return gatewaycontract.ContentBlock{
		Type:           gatewaycontract.ContentBlockMetadata,
		Role:           role,
		Metadata:       metadata,
		Raw:            raw,
		OriginProtocol: string(gatewaycontract.ProtocolOpenAICompatible),
	}, true
}

func rawResponsesFunctionCallBlock(value map[string]any, role string) (gatewaycontract.ContentBlock, bool) {
	callID := firstNonEmpty(rawMapString(value, "call_id"), rawMapString(value, "id"))
	name := rawMapString(value, "name")
	arguments := rawResponsesArguments(value["arguments"])
	if callID == "" && name == "" && arguments == "" {
		return gatewaycontract.ContentBlock{}, false
	}
	if role == "" {
		role = "assistant"
	}
	return gatewaycontract.ContentBlock{
		Type:              gatewaycontract.ContentBlockToolCall,
		Role:              role,
		Text:              "[function_call]",
		ToolCallID:        callID,
		ToolName:          name,
		ToolArgumentsJSON: arguments,
		Metadata:          cloneMap(value),
		OriginProtocol:    string(gatewaycontract.ProtocolOpenAICompatible),
		Raw:               marshalRawJSON(value),
	}, true
}

func rawResponsesFunctionCallOutputBlock(value map[string]any, role string) (gatewaycontract.ContentBlock, bool) {
	callID := firstNonEmpty(rawMapString(value, "call_id"), rawMapString(value, "id"))
	output := rawResponsesOutputText(value["output"])
	if callID == "" && output == "" {
		return gatewaycontract.ContentBlock{}, false
	}
	if role == "" {
		role = "tool"
	}
	return gatewaycontract.ContentBlock{
		Type:            gatewaycontract.ContentBlockToolResult,
		Role:            role,
		Text:            output,
		ToolCallID:      callID,
		ToolResultForID: callID,
		Metadata:        cloneMap(value),
		OriginProtocol:  string(gatewaycontract.ProtocolOpenAICompatible),
		Raw:             marshalRawJSON(value),
	}, true
}

func rawResponsesImageBlock(value map[string]any, role string) (gatewaycontract.ContentBlock, bool) {
	props := cloneMap(value)
	imageURL, _ := value["image_url"].(map[string]any)
	if len(imageURL) > 0 {
		for key, item := range imageURL {
			props[key] = cloneAny(item)
		}
	}
	url := firstNonEmpty(rawMapString(value, "image_url"), rawMapString(value, "url"), mapStringAny(imageURL, "url"))
	base64Data, mimeType := splitDataURL(url)
	if base64Data != "" {
		url = ""
	}
	if base64Data == "" {
		base64Data = firstNonEmpty(rawMapString(value, "data"), rawMapString(value, "media_base64"))
	}
	mimeType = firstNonEmpty(mimeType, rawMapString(value, "mime_type"), rawMapString(value, "media_type"))
	fileID := firstNonEmpty(rawMapString(value, "file_id"), rawMapString(value, "fileId"))
	if url == "" && base64Data == "" && fileID == "" {
		return gatewaycontract.ContentBlock{}, false
	}
	return gatewaycontract.ContentBlock{
		Type:           gatewaycontract.ContentBlockImage,
		Role:           role,
		Text:           "[image]",
		MediaURL:       url,
		MediaBase64:    base64Data,
		MIMEType:       mimeType,
		FileID:         fileID,
		Metadata:       props,
		OriginProtocol: string(gatewaycontract.ProtocolOpenAICompatible),
		Raw:            marshalRawJSON(value),
	}, true
}

func rawResponsesArguments(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	default:
		return jsonStringOrMarshal(typed)
	}
}

func rawResponsesOutputText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	default:
		return jsonStringOrMarshal(typed)
	}
}

func rawMapString(value map[string]any, key string) string {
	raw, ok := value[key]
	if !ok || raw == nil {
		return ""
	}
	text, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}
