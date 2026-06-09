package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

type anthropicMessagesRequest struct {
	Model             string             `json:"model"`
	Messages          []anthropicMessage `json:"messages"`
	System            string             `json:"system,omitempty"`
	Stream            bool               `json:"stream"`
	MaxTokens         int                `json:"max_tokens"`
	Thinking          map[string]any     `json:"thinking,omitempty"`
	ContextManagement map[string]any     `json:"context_management,omitempty"`
	Temperature       *float32           `json:"temperature,omitempty"`
	TopP              *float32           `json:"top_p,omitempty"`
	StopSequences     []string           `json:"stop_sequences,omitempty"`
	Tools             []map[string]any   `json:"tools,omitempty"`
	ToolChoice        any                `json:"tool_choice,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

func anthropicCompatiblePayload(req contract.ConversationRequest) anthropicMessagesRequest {
	maxTokens := 1024
	if req.MaxOutputTokens != nil && *req.MaxOutputTokens > 0 {
		maxTokens = *req.MaxOutputTokens
	}
	return anthropicMessagesRequest{
		Model:             req.Mapping.UpstreamModelName,
		Messages:          anthropicCompatibleMessages(req),
		System:            anthropicCompatibleSystem(req),
		Stream:            req.Stream,
		MaxTokens:         maxTokens,
		Thinking:          anthropicCompatibleThinking(req.Reasoning, maxTokens),
		ContextManagement: cloneMap(req.ContextManagement),
		Temperature:       req.Temperature,
		TopP:              req.TopP,
		StopSequences:     cloneStrings(req.Stop),
		Tools:             anthropicCompatibleTools(req.Tools),
		ToolChoice:        anthropicCompatibleToolChoice(req.ToolChoice),
	}
}

func anthropicCompatibleRequestBody(req contract.ConversationRequest) ([]byte, error) {
	raw, err := anthropicCompatibleRequestBodyRaw(req)
	if err != nil {
		return nil, err
	}
	return applyPayloadTransforms(raw, req.PayloadTransforms)
}

func anthropicCompatibleRequestBodyRaw(req contract.ConversationRequest) ([]byte, error) {
	if payload, ok, err := rawSameProtocolPayload(req, rawEndpointAnthropicMessages); ok || err != nil {
		if err != nil {
			return nil, err
		}
		return json.Marshal(payload)
	}
	return json.Marshal(anthropicCompatiblePayload(req))
}

func anthropicCompatibleThinking(reasoning map[string]any, maxTokens int) map[string]any {
	thinkingType := strings.ToLower(strings.TrimSpace(metadataString(reasoning, "type")))
	switch thinkingType {
	case "enabled":
	case "adaptive":
		out := cloneMap(reasoning)
		out["type"] = "adaptive"
		delete(out, "budget_tokens")
		return out
	default:
		return nil
	}
	out := cloneMap(reasoning)
	out["type"] = "enabled"
	budget := positiveIntValue(out["budget_tokens"])
	if budget <= 0 || budget >= maxTokens {
		budget = maxTokens - 1
	}
	if budget < 1024 {
		return nil
	}
	out["budget_tokens"] = budget
	return out
}

func positiveIntValue(value any) int {
	switch typed := value.(type) {
	case int:
		if typed > 0 {
			return typed
		}
	case int64:
		if typed > 0 {
			return int(typed)
		}
	case float64:
		if typed > 0 {
			return int(typed)
		}
	case json.Number:
		if value, err := typed.Int64(); err == nil && value > 0 {
			return int(value)
		}
	}
	return 0
}

func anthropicCompatibleMessages(req contract.ConversationRequest) []anthropicMessage {
	out := make([]anthropicMessage, 0, len(req.Messages)+1)
	hasConversationMessage := false
	for _, message := range req.Messages {
		role := anthropicMessageRole(message.Role)
		switch role {
		case "":
			role = "user"
		case "system":
			continue
		}
		content := anthropicContentFromParts(message.Parts)
		if content == nil {
			continue
		}
		out = append(out, anthropicMessage{Role: role, Content: content})
		hasConversationMessage = true
	}
	if !hasConversationMessage {
		content := anthropicContentFromParts(req.InputParts)
		if content == nil {
			content = conversationPrompt(req)
		}
		out = append(out, anthropicMessage{Role: "user", Content: content})
	}
	return out
}

func anthropicMessageRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant":
		return "assistant"
	case "system":
		return "system"
	default:
		return "user"
	}
}

func anthropicContentFromParts(parts []contract.ContentPart) any {
	blocks := make([]map[string]any, 0, len(parts))
	var textParts []string
	plainTextOnly := true
	for _, part := range parts {
		switch part.Kind {
		case "", contract.ContentPartText, contract.ContentPartRefusal:
			if text := strings.TrimSpace(part.Text); text != "" {
				if anthropicPartHasBlockMetadata(part) {
					plainTextOnly = false
				}
				textParts = append(textParts, text)
				blocks = append(blocks, anthropicBlockWithMetadata(map[string]any{"type": "text", "text": text}, part))
			}
		case contract.ContentPartThinking:
			plainTextOnly = false
			if block := anthropicThinkingBlock(part); len(block) > 0 {
				blocks = append(blocks, block)
			}
		case contract.ContentPartImage:
			plainTextOnly = false
			if block := anthropicImageBlock(part); len(block) > 0 {
				blocks = append(blocks, block)
				continue
			}
			if text := strings.TrimSpace(part.Text); text != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": text})
			}
		case contract.ContentPartToolUse:
			plainTextOnly = false
			block := map[string]any{"type": "tool_use"}
			setMapString(block, "id", part.ToolCallID)
			setMapString(block, "name", part.ToolName)
			if input := jsonObjectValue(part.ToolArgumentsJSON); input != nil {
				block["input"] = input
			} else {
				block["input"] = map[string]any{}
			}
			block = anthropicBlockWithMetadata(block, part)
			blocks = append(blocks, block)
		case contract.ContentPartToolResult:
			plainTextOnly = false
			block := map[string]any{"type": "tool_result"}
			setMapString(block, "tool_use_id", firstNonEmpty(part.ToolResultForID, part.ToolCallID))
			if content := anthropicToolResultNestedContent(part); len(content) > 0 {
				block["content"] = content
			} else if text := strings.TrimSpace(part.Text); text != "" {
				block["content"] = text
			}
			if part.ToolResultIsError {
				block["is_error"] = true
			}
			block = anthropicBlockWithMetadata(block, part)
			blocks = append(blocks, block)
		default:
			if text := strings.TrimSpace(part.Text); text != "" {
				if anthropicPartHasBlockMetadata(part) {
					plainTextOnly = false
				}
				textParts = append(textParts, text)
				blocks = append(blocks, anthropicBlockWithMetadata(map[string]any{"type": "text", "text": text}, part))
			}
		}
	}
	if len(blocks) == 0 {
		return nil
	}
	if plainTextOnly {
		return strings.Join(textParts, "\n")
	}
	return blocks
}

func anthropicImageBlock(part contract.ContentPart) map[string]any {
	source := map[string]any{}
	if url := strings.TrimSpace(part.MediaURL); url != "" {
		source["type"] = "url"
		source["url"] = url
	} else if data := strings.TrimSpace(part.MediaBase64); data != "" {
		source["type"] = "base64"
		source["media_type"] = firstNonEmpty(part.MIMEType, "image/png")
		source["data"] = data
	}
	if len(source) == 0 {
		return nil
	}
	return anthropicBlockWithMetadata(map[string]any{"type": "image", "source": source}, part)
}

func anthropicToolResultNestedContent(part contract.ContentPart) []map[string]any {
	values, ok := part.Metadata["content"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		switch strings.TrimSpace(mapString(item, "type")) {
		case "text":
			text := strings.TrimSpace(mapString(item, "text"))
			if text == "" {
				continue
			}
			out = append(out, map[string]any{"type": "text", "text": text})
		case "image":
			if block := anthropicRawImageBlock(item); len(block) > 0 {
				out = append(out, block)
			}
		}
	}
	return out
}

func anthropicRawImageBlock(item map[string]any) map[string]any {
	source, _ := item["source"].(map[string]any)
	imageSource := cloneMap(source)
	if len(imageSource) == 0 {
		imageSource = map[string]any{}
		if url := mapString(item, "url"); url != "" {
			imageSource["type"] = "url"
			imageSource["url"] = url
		} else if data := mapString(item, "data"); data != "" {
			imageSource["type"] = "base64"
			imageSource["media_type"] = firstNonEmpty(mapString(item, "media_type"), "image/png")
			imageSource["data"] = data
		}
	}
	if len(imageSource) == 0 {
		return nil
	}
	return map[string]any{"type": "image", "source": imageSource}
}

func anthropicThinkingBlock(part contract.ContentPart) map[string]any {
	blockType := strings.TrimSpace(metadataString(part.Metadata, "type"))
	if blockType == "redacted_thinking" {
		data := strings.TrimSpace(metadataString(part.Metadata, "data"))
		if data == "" {
			data = strings.TrimSpace(part.Text)
		}
		if data == "" {
			return nil
		}
		return anthropicBlockWithMetadata(map[string]any{"type": "redacted_thinking", "data": data}, part)
	}
	text := strings.TrimSpace(part.Text)
	if text == "" {
		return nil
	}
	block := map[string]any{"type": "thinking", "thinking": text}
	if signature := metadataString(part.Metadata, "signature"); signature != "" {
		block["signature"] = signature
	}
	return anthropicBlockWithMetadata(block, part)
}

func anthropicPartHasBlockMetadata(part contract.ContentPart) bool {
	for _, key := range []string{"cache_control", "citations"} {
		if value, ok := part.Metadata[key]; ok && value != nil {
			return true
		}
	}
	return false
}

func anthropicBlockWithMetadata(block map[string]any, part contract.ContentPart) map[string]any {
	if len(block) == 0 || len(part.Metadata) == 0 {
		return block
	}
	for _, key := range []string{"cache_control", "citations"} {
		value, ok := part.Metadata[key]
		if !ok || value == nil {
			continue
		}
		if _, exists := block[key]; exists {
			continue
		}
		block[key] = cloneAny(value)
	}
	return block
}

func anthropicCompatibleSystem(req contract.ConversationRequest) string {
	parts := make([]string, 0, len(req.Messages)+1)
	if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
		parts = append(parts, instructions)
	}
	for _, message := range req.Messages {
		if strings.TrimSpace(message.Role) != "system" {
			continue
		}
		if content := conversationMessageText(message); content != "" {
			parts = append(parts, content)
		}
	}
	return strings.Join(uniqueTrimmedStrings(parts), "\n")
}

func anthropicCompatibleTools(values []map[string]any) []map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		if function, ok := value["function"].(map[string]any); ok {
			tool := map[string]any{}
			if name := strings.TrimSpace(fmt.Sprint(function["name"])); name != "" && name != "<nil>" {
				tool["name"] = name
			}
			if description := strings.TrimSpace(fmt.Sprint(function["description"])); description != "" && description != "<nil>" {
				tool["description"] = description
			}
			if parameters, ok := function["parameters"]; ok && parameters != nil {
				tool["input_schema"] = cloneAny(parameters)
			}
			if len(tool) > 0 {
				out = append(out, tool)
			}
			continue
		}
		tool := cloneMap(value)
		delete(tool, "type")
		if len(tool) > 0 {
			out = append(out, tool)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func anthropicCompatibleToolChoice(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		switch strings.TrimSpace(typed) {
		case "auto":
			return map[string]any{"type": "auto"}
		case "required", "any":
			return map[string]any{"type": "any"}
		case "none":
			return map[string]any{"type": "none"}
		default:
			return nil
		}
	case map[string]any:
		if choiceType, ok := typed["type"].(string); ok {
			switch strings.TrimSpace(choiceType) {
			case "auto", "any", "none":
				return cloneMap(typed)
			case "function":
				if function, ok := typed["function"].(map[string]any); ok {
					if name := strings.TrimSpace(fmt.Sprint(function["name"])); name != "" && name != "<nil>" {
						return map[string]any{"type": "tool", "name": name}
					}
				}
			}
		}
		return cloneMap(typed)
	default:
		return cloneAny(value)
	}
}

type openAIChatCompletionRequest struct {
	Model          string               `json:"model"`
	Messages       []openAIChatMessage  `json:"messages"`
	Stream         bool                 `json:"stream"`
	StreamOptions  *openAIStreamOptions `json:"stream_options,omitempty"`
	Temperature    *float32             `json:"temperature,omitempty"`
	TopP           *float32             `json:"top_p,omitempty"`
	MaxTokens      *int                 `json:"max_tokens,omitempty"`
	Stop           []string             `json:"stop,omitempty"`
	Tools          []map[string]any     `json:"tools,omitempty"`
	ToolChoice     any                  `json:"tool_choice,omitempty"`
	ResponseFormat map[string]any       `json:"response_format,omitempty"`
}

type openAIEmbeddingRequest struct {
	Model          string   `json:"model"`
	Input          []string `json:"input"`
	EncodingFormat string   `json:"encoding_format,omitempty"`
	Dimensions     *int     `json:"dimensions,omitempty"`
	User           string   `json:"user,omitempty"`
}

type openAIImageGenerationRequest struct {
	Model          string         `json:"model"`
	Prompt         string         `json:"prompt"`
	N              int            `json:"n,omitempty"`
	Size           string         `json:"size,omitempty"`
	Quality        string         `json:"quality,omitempty"`
	Style          string         `json:"style,omitempty"`
	ResponseFormat string         `json:"response_format,omitempty"`
	User           string         `json:"user,omitempty"`
	Extra          map[string]any `json:"-"`
}

type openAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type openAIChatMessage struct {
	Role             string           `json:"role"`
	Content          any              `json:"content"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
	ToolCalls        []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAIToolCall struct {
	ID       string                 `json:"id,omitempty"`
	Type     string                 `json:"type,omitempty"`
	Function openAIToolCallFunction `json:"function,omitempty"`
}

type openAIToolCallFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

func openAICompatiblePayload(req contract.ConversationRequest) openAIChatCompletionRequest {
	payload := openAIChatCompletionRequest{
		Model:          req.Mapping.UpstreamModelName,
		Messages:       openAICompatibleMessages(req),
		Stream:         req.Stream,
		Temperature:    req.Temperature,
		TopP:           req.TopP,
		MaxTokens:      req.MaxOutputTokens,
		Stop:           cloneStrings(req.Stop),
		Tools:          cloneMapSlice(req.Tools),
		ToolChoice:     cloneAny(req.ToolChoice),
		ResponseFormat: cloneMap(req.ResponseFormat),
	}
	if req.Stream {
		payload.StreamOptions = &openAIStreamOptions{IncludeUsage: true}
	}
	return payload
}

func openAICompatibleRequestBody(req contract.ConversationRequest) ([]byte, error) {
	raw, err := openAICompatibleRequestBodyRaw(req)
	if err != nil {
		return nil, err
	}
	return applyPayloadTransforms(raw, req.PayloadTransforms)
}

func openAICompatibleRequestBodyRaw(req contract.ConversationRequest) ([]byte, error) {
	if payload, ok, err := rawSameProtocolPayload(req, rawEndpointOpenAIChatCompletions); ok || err != nil {
		if err != nil {
			return nil, err
		}
		if req.Stream {
			ensureOpenAIStreamOptions(payload)
		}
		return json.Marshal(payload)
	}
	payload := openAICompatiblePayload(req)
	if req.Stream {
		payload.StreamOptions = &openAIStreamOptions{IncludeUsage: true}
	}
	return json.Marshal(payload)
}

func openAIEmbeddingPayload(req contract.EmbeddingRequest) openAIEmbeddingRequest {
	encoding := strings.TrimSpace(req.EncodingFormat)
	if encoding == "" {
		encoding = "float"
	}
	return openAIEmbeddingRequest{
		Model:          req.Mapping.UpstreamModelName,
		Input:          append([]string(nil), req.Input...),
		EncodingFormat: encoding,
		Dimensions:     cloneIntPtr(req.Dimensions),
		User:           strings.TrimSpace(req.User),
	}
}

func openAIImageGenerationPayload(req contract.ImageGenerationRequest) openAIImageGenerationRequest {
	return openAIImageGenerationRequest{
		Model:          req.Mapping.UpstreamModelName,
		Prompt:         strings.TrimSpace(req.Prompt),
		N:              req.Count,
		Size:           strings.TrimSpace(req.Size),
		Quality:        strings.TrimSpace(req.Quality),
		Style:          strings.TrimSpace(req.Style),
		ResponseFormat: strings.TrimSpace(req.ResponseFormat),
		User:           strings.TrimSpace(req.User),
		Extra:          cloneMap(req.Extra),
	}
}

func (r openAIImageGenerationRequest) MarshalJSON() ([]byte, error) {
	type alias openAIImageGenerationRequest
	raw, err := json.Marshal(alias(r))
	if err != nil {
		return nil, err
	}
	payload := map[string]any{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	delete(payload, "Extra")
	for key, value := range r.Extra {
		if _, exists := payload[key]; !exists {
			payload[key] = value
		}
	}
	return json.Marshal(payload)
}

func openAICompatibleMessages(req contract.ConversationRequest) []openAIChatMessage {
	out := make([]openAIChatMessage, 0, len(req.Messages)+2)
	if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
		out = append(out, openAIChatMessage{Role: "system", Content: instructions})
	}
	hasConversationMessage := false
	for _, message := range req.Messages {
		role := strings.TrimSpace(message.Role)
		if role == "" {
			role = "user"
		}
		messages := openAIChatMessagesFromParts(role, message.Parts)
		if len(messages) == 0 {
			continue
		}
		out = append(out, messages...)
		hasConversationMessage = true
	}
	if !hasConversationMessage {
		content := openAIContentFromParts(req.InputParts)
		if content == nil {
			content = conversationPrompt(req)
		}
		out = append(out, openAIChatMessage{Role: "user", Content: content})
	}
	return out
}

func openAIChatMessagesFromParts(role string, parts []contract.ContentPart) []openAIChatMessage {
	role = openAIMessageRole(role)
	ordinaryParts := make([]contract.ContentPart, 0, len(parts))
	toolCalls := make([]openAIToolCall, 0)
	toolMessages := make([]openAIChatMessage, 0)
	var reasoningParts []string
	for _, part := range parts {
		switch part.Kind {
		case contract.ContentPartThinking:
			if text := strings.TrimSpace(part.Text); text != "" {
				reasoningParts = append(reasoningParts, text)
			}
		case contract.ContentPartToolUse:
			if toolCall, ok := openAIToolCallFromPart(part); ok {
				toolCalls = append(toolCalls, toolCall)
			}
		case contract.ContentPartToolResult:
			content := strings.TrimSpace(part.Text)
			toolMessages = append(toolMessages, openAIChatMessage{
				Role:       "tool",
				Content:    content,
				ToolCallID: strings.TrimSpace(firstNonEmpty(part.ToolResultForID, part.ToolCallID)),
			})
		default:
			ordinaryParts = append(ordinaryParts, part)
		}
	}

	out := make([]openAIChatMessage, 0, 1+len(toolMessages))
	if len(ordinaryParts) > 0 || len(toolCalls) > 0 || len(reasoningParts) > 0 {
		message := openAIChatMessage{Role: role}
		if len(reasoningParts) > 0 {
			message.ReasoningContent = strings.Join(reasoningParts, "\n")
		}
		if len(ordinaryParts) > 0 {
			message.Content = openAIContentFromParts(ordinaryParts)
		} else {
			message.Content = ""
		}
		if len(toolCalls) > 0 {
			message.Role = "assistant"
			message.ToolCalls = toolCalls
		}
		out = append(out, message)
	}
	out = append(out, toolMessages...)
	return out
}

func openAIMessageRole(role string) string {
	switch strings.TrimSpace(role) {
	case "system", "developer", "assistant", "tool":
		return strings.TrimSpace(role)
	default:
		return "user"
	}
}

func openAIContentFromParts(parts []contract.ContentPart) any {
	blocks := make([]map[string]any, 0, len(parts))
	var textParts []string
	plainTextOnly := true
	for _, part := range parts {
		switch part.Kind {
		case "", contract.ContentPartText, contract.ContentPartThinking, contract.ContentPartRefusal:
			if text := strings.TrimSpace(part.Text); text != "" {
				if openAITextPartHasMetadata(part) {
					plainTextOnly = false
				}
				textParts = append(textParts, text)
				blocks = append(blocks, openAITextContentBlock(text, part))
			}
		case contract.ContentPartImage:
			plainTextOnly = false
			if block := openAIImageContentBlock(part); len(block) > 0 {
				blocks = append(blocks, block)
				continue
			}
			if text := strings.TrimSpace(part.Text); text != "" {
				blocks = append(blocks, openAITextContentBlock(text, part))
			}
		case contract.ContentPartAudio:
			plainTextOnly = false
			if block := openAIAudioContentBlock(part); len(block) > 0 {
				blocks = append(blocks, block)
			}
		case contract.ContentPartFile:
			plainTextOnly = false
			if block := openAIFileContentBlock(part); len(block) > 0 {
				blocks = append(blocks, block)
				continue
			}
			if text := strings.TrimSpace(part.Text); text != "" {
				blocks = append(blocks, openAITextContentBlock(text, part))
			}
		default:
			if text := strings.TrimSpace(part.Text); text != "" {
				if openAITextPartHasMetadata(part) {
					plainTextOnly = false
				}
				textParts = append(textParts, text)
				blocks = append(blocks, openAITextContentBlock(text, part))
			}
		}
	}
	if len(blocks) == 0 {
		return nil
	}
	if plainTextOnly {
		return strings.Join(textParts, "\n")
	}
	return blocks
}

func openAITextPartHasMetadata(part contract.ContentPart) bool {
	for _, key := range openAITextMetadataFields() {
		if value, ok := part.Metadata[key]; ok && value != nil {
			return true
		}
	}
	return false
}

func openAITextContentBlock(text string, part contract.ContentPart) map[string]any {
	block := map[string]any{}
	for _, key := range openAITextMetadataFields() {
		value, ok := part.Metadata[key]
		if !ok || value == nil {
			continue
		}
		block[key] = cloneAny(value)
	}
	block["type"] = "text"
	block["text"] = text
	return block
}

func openAITextMetadataFields() []string {
	return []string{"annotations"}
}

func openAIImageContentBlock(part contract.ContentPart) map[string]any {
	url := mediaURLValue(part)
	if url == "" {
		return nil
	}
	return map[string]any{
		"type":      "image_url",
		"image_url": map[string]any{"url": url},
	}
}

func openAIAudioContentBlock(part contract.ContentPart) map[string]any {
	data := strings.TrimSpace(part.MediaBase64)
	if data == "" {
		return nil
	}
	format := strings.TrimPrefix(strings.TrimSpace(part.MIMEType), "audio/")
	if format == "" {
		format = "wav"
	}
	return map[string]any{
		"type": "input_audio",
		"input_audio": map[string]any{
			"data":   data,
			"format": format,
		},
	}
}

func openAIFileContentBlock(part contract.ContentPart) map[string]any {
	file := map[string]any{}
	setMapString(file, "file_id", part.FileID)
	if url := mediaURLValue(part); url != "" {
		file["file_data"] = url
	}
	if len(file) == 0 {
		return nil
	}
	return map[string]any{"type": "file", "file": file}
}

func openAIToolCallFromPart(part contract.ContentPart) (openAIToolCall, bool) {
	id := strings.TrimSpace(part.ToolCallID)
	name := strings.TrimSpace(part.ToolName)
	arguments := part.ToolArgumentsJSON
	if id == "" && name == "" && strings.TrimSpace(arguments) == "" {
		return openAIToolCall{}, false
	}
	callType := metadataString(part.Metadata, "type")
	if callType == "" {
		callType = "function"
	}
	return openAIToolCall{
		ID:   id,
		Type: callType,
		Function: openAIToolCallFunction{
			Name:      name,
			Arguments: arguments,
		},
	}, true
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func cloneMapSlice(values []map[string]any) []map[string]any {
	if values == nil {
		return nil
	}
	out := make([]map[string]any, len(values))
	for idx, value := range values {
		out[idx] = cloneMap(value)
	}
	return out
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
	case []map[string]any:
		return cloneMapSlice(typed)
	case []any:
		out := make([]any, len(typed))
		for idx, item := range typed {
			out[idx] = cloneAny(item)
		}
		return out
	default:
		return typed
	}
}

type openAIChatCompletionResponse struct {
	Choices []struct {
		Message      openAIChatMessage `json:"message"`
		FinishReason string            `json:"finish_reason"`
	} `json:"choices"`
	Usage openAIUsage `json:"usage"`
}

type openAIEmbeddingResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string          `json:"object"`
		Embedding json.RawMessage `json:"embedding"`
		Index     int             `json:"index"`
	} `json:"data"`
	Model string      `json:"model"`
	Usage openAIUsage `json:"usage"`
}

type openAIImageGenerationResponse struct {
	Created int64 `json:"created"`
	Data    []struct {
		URL           string         `json:"url"`
		Base64JSON    string         `json:"b64_json"`
		RevisedPrompt string         `json:"revised_prompt"`
		Extra         map[string]any `json:"-"`
	} `json:"data"`
	Model string      `json:"model"`
	Usage openAIUsage `json:"usage"`
}

func parseOpenAICompatibleJSON(body []byte, statusCode int) (contract.ConversationResponse, error) {
	var decoded openAIChatCompletionResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	resp, err := decoded.ConversationResponse(statusCode)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	resp.Raw = append(json.RawMessage(nil), body...)
	return resp, nil
}

func (r openAIChatCompletionResponse) ConversationResponse(statusCode int) (contract.ConversationResponse, error) {
	if len(r.Choices) == 0 {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no choices"}
	}
	choice := r.Choices[0]
	parts := openAIMessageParts(choice.Message)
	if len(parts) == 0 {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no content"}
	}
	text := contentPartsText(parts)
	return contract.ConversationResponse{
		Parts:      parts,
		StopReason: openAIStopReason(choice.FinishReason),
		StatusCode: statusCode,
		Usage:      r.Usage.ToUsage(text),
	}, nil
}

func openAIMessageParts(message openAIChatMessage) []contract.ContentPart {
	parts := make([]contract.ContentPart, 0, 1+len(message.ToolCalls))
	if reasoning := strings.TrimSpace(message.ReasoningContent); reasoning != "" {
		parts = append(parts, contract.ContentPart{Kind: contract.ContentPartThinking, Text: reasoning, OriginProtocol: "openai-compatible"})
	}
	parts = append(parts, openAIContentParts(message.Content)...)
	for _, toolCall := range message.ToolCalls {
		if part, ok := openAIToolCallPart(toolCall); ok {
			parts = append(parts, part)
		}
	}
	return parts
}

func openAIToolCallPart(toolCall openAIToolCall) (contract.ContentPart, bool) {
	id := strings.TrimSpace(toolCall.ID)
	name := strings.TrimSpace(toolCall.Function.Name)
	arguments := toolCall.Function.Arguments
	if id == "" && name == "" && strings.TrimSpace(arguments) == "" {
		return contract.ContentPart{}, false
	}
	metadata := map[string]any{}
	if callType := strings.TrimSpace(toolCall.Type); callType != "" {
		metadata["type"] = callType
	}
	if len(metadata) == 0 {
		metadata = nil
	}
	return contract.ContentPart{
		Kind:              contract.ContentPartToolUse,
		ToolCallID:        id,
		ToolName:          name,
		ToolArgumentsJSON: arguments,
		Metadata:          metadata,
		OriginProtocol:    "openai",
	}, true
}

func openAIStopReason(reason string) contract.StopReason {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "length":
		return contract.StopReasonMaxTokens
	case "tool_calls", "function_call":
		return contract.StopReasonToolUse
	case "content_filter":
		return contract.StopReasonContentFilter
	}
	return contract.StopReasonEndTurn
}

func parseOpenAICompatibleEmbeddings(body []byte, statusCode int, fallbackModel string, input []string) (contract.EmbeddingResponse, error) {
	var decoded openAIEmbeddingResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return contract.EmbeddingResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	data := make([]contract.Embedding, 0, len(decoded.Data))
	for _, item := range decoded.Data {
		embedding, err := parseOpenAIEmbeddingValue(item.Embedding)
		if err != nil {
			return contract.EmbeddingResponse{}, err
		}
		embedding.Index = item.Index
		data = append(data, embedding)
	}
	if len(data) == 0 {
		return contract.EmbeddingResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no embeddings"}
	}
	model := strings.TrimSpace(decoded.Model)
	if model == "" {
		model = strings.TrimSpace(fallbackModel)
	}
	return contract.EmbeddingResponse{
		Data:       data,
		Model:      model,
		StatusCode: statusCode,
		Usage:      decoded.Usage.ToEmbeddingUsage(input),
	}, nil
}

func parseOpenAIEmbeddingValue(raw json.RawMessage) (contract.Embedding, error) {
	var vector []float32
	if err := json.Unmarshal(raw, &vector); err == nil && len(vector) > 0 {
		return contract.Embedding{Vector: vector}, nil
	}
	var encoded string
	if err := json.Unmarshal(raw, &encoded); err == nil && strings.TrimSpace(encoded) != "" {
		return contract.Embedding{Base64Vector: strings.TrimSpace(encoded)}, nil
	}
	return contract.Embedding{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained invalid embedding vector"}
}

func parseOpenAICompatibleImages(body []byte, statusCode int, fallbackModel string, req contract.ImageGenerationRequest) (contract.ImageGenerationResponse, error) {
	var decoded openAIImageGenerationResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	data := make([]contract.Image, 0, len(decoded.Data))
	for _, item := range decoded.Data {
		image := contract.Image{
			URL:           strings.TrimSpace(item.URL),
			Base64JSON:    strings.TrimSpace(item.Base64JSON),
			RevisedPrompt: strings.TrimSpace(item.RevisedPrompt),
			Metadata:      cloneMap(item.Extra),
		}
		if image.URL == "" && image.Base64JSON == "" {
			return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained image without url or b64_json"}
		}
		data = append(data, image)
	}
	if len(data) == 0 {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no images"}
	}
	created := decoded.Created
	if created == 0 {
		created = time.Now().Unix()
	}
	model := strings.TrimSpace(decoded.Model)
	if model == "" {
		model = strings.TrimSpace(fallbackModel)
	}
	return contract.ImageGenerationResponse{
		Created:    created,
		Data:       data,
		Model:      model,
		StatusCode: statusCode,
		Usage:      decoded.Usage.ToImageUsage(req),
	}, nil
}

func (r *openAIImageGenerationResponse) UnmarshalJSON(body []byte) error {
	var raw struct {
		Created int64            `json:"created"`
		Data    []map[string]any `json:"data"`
		Model   string           `json:"model"`
		Usage   openAIUsage      `json:"usage"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	r.Created = raw.Created
	r.Model = raw.Model
	r.Usage = raw.Usage
	r.Data = make([]struct {
		URL           string         `json:"url"`
		Base64JSON    string         `json:"b64_json"`
		RevisedPrompt string         `json:"revised_prompt"`
		Extra         map[string]any `json:"-"`
	}, 0, len(raw.Data))
	for _, item := range raw.Data {
		image := struct {
			URL           string         `json:"url"`
			Base64JSON    string         `json:"b64_json"`
			RevisedPrompt string         `json:"revised_prompt"`
			Extra         map[string]any `json:"-"`
		}{
			URL:           mapString(item, "url"),
			Base64JSON:    mapString(item, "b64_json"),
			RevisedPrompt: mapString(item, "revised_prompt"),
			Extra:         cloneMap(item),
		}
		delete(image.Extra, "url")
		delete(image.Extra, "b64_json")
		delete(image.Extra, "revised_prompt")
		r.Data = append(r.Data, image)
	}
	return nil
}

type openAIChatCompletionStreamChunk struct {
	Choices []openAIChatCompletionStreamChoice `json:"choices"`
	Usage   *openAIUsage                       `json:"usage"`
}

type openAIChatCompletionStreamChoice struct {
	Index        int                             `json:"index"`
	Delta        openAIChatCompletionStreamDelta `json:"delta"`
	FinishReason string                          `json:"finish_reason"`
}

type openAIChatCompletionStreamDelta struct {
	Content          string                  `json:"content"`
	Reasoning        json.RawMessage         `json:"reasoning,omitempty"`
	ReasoningContent json.RawMessage         `json:"reasoning_content,omitempty"`
	ReasoningSummary json.RawMessage         `json:"reasoning_summary,omitempty"`
	ToolCalls        []openAIStreamToolCall  `json:"tool_calls,omitempty"`
	FunctionCall     *openAIToolCallFunction `json:"function_call,omitempty"`
}

type openAIStreamToolCall struct {
	Index    int                    `json:"index"`
	ID       string                 `json:"id,omitempty"`
	Type     string                 `json:"type,omitempty"`
	Function openAIToolCallFunction `json:"function,omitempty"`
}

type openAIUsage struct {
	PromptTokens       *int `json:"prompt_tokens"`
	CompletionTokens   *int `json:"completion_tokens"`
	TotalTokens        *int `json:"total_tokens"`
	InputTokens        *int `json:"input_tokens"`
	OutputTokens       *int `json:"output_tokens"`
	CachedTokens       *int `json:"cached_tokens"`
	InputTokensDetails *struct {
		CachedTokens *int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	PromptTokensDetails *struct {
		CachedTokens *int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

func (u openAIUsage) ToUsage(text string) contract.Usage {
	input := valueOrZero(u.InputTokens)
	if input == 0 {
		input = valueOrZero(u.PromptTokens)
	}
	output := valueOrZero(u.OutputTokens)
	if output == 0 {
		output = valueOrZero(u.CompletionTokens)
	}
	cached := valueOrZero(u.CachedTokens)
	if cached == 0 && u.InputTokensDetails != nil {
		cached = valueOrZero(u.InputTokensDetails.CachedTokens)
	}
	if cached == 0 && u.PromptTokensDetails != nil {
		cached = valueOrZero(u.PromptTokensDetails.CachedTokens)
	}
	total := input + output + cached
	if u.TotalTokens != nil && *u.TotalTokens > 0 && total == 0 {
		total = *u.TotalTokens
	}
	if total > 0 && output == 0 {
		output = max(0, total-input-cached)
	}
	if input == 0 && output == 0 && cached == 0 {
		return estimatedUsage(text)
	}
	return contract.Usage{
		InputTokens:  input,
		OutputTokens: output,
		CachedTokens: cached,
		Estimated:    false,
	}
}

func (u openAIUsage) ToEmbeddingUsage(input []string) contract.Usage {
	usage := u.ToUsage(strings.Join(input, "\n"))
	usage.OutputTokens = 0
	return usage
}

func (u openAIUsage) ToImageUsage(req contract.ImageGenerationRequest) contract.Usage {
	if !u.HasTokenUsage() {
		return estimatedImageUsage(req)
	}
	usage := u.ToUsage(req.Prompt)
	return usage
}

func (u openAIUsage) ToModerationUsage(input []string) contract.Usage {
	usage := u.ToUsage(strings.Join(input, "\n"))
	usage.OutputTokens = 0
	return usage
}

func (u openAIUsage) HasTokenUsage() bool {
	return u.PromptTokens != nil ||
		u.CompletionTokens != nil ||
		u.TotalTokens != nil ||
		u.InputTokens != nil ||
		u.OutputTokens != nil ||
		u.CachedTokens != nil ||
		(u.InputTokensDetails != nil && u.InputTokensDetails.CachedTokens != nil) ||
		(u.PromptTokensDetails != nil && u.PromptTokensDetails.CachedTokens != nil)
}

func parseOpenAICompatibleStream(body []byte, statusCode int) (contract.ConversationResponse, error) {
	frames, err := parseSSEFrames(body)
	if err != nil {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
	}
	var builder strings.Builder
	var reasoningBuilder strings.Builder
	var usage *openAIUsage
	toolCalls := map[int]*openAIToolCall{}
	toolOrder := []int{}
	streamEvents := make([]contract.ConversationStreamEvent, 0)
	eventIndex := 0
	stopReason := contract.StopReasonEndTurn
	sawReasoning := false
	sawFinish := false
	sawToolCall := false
	finishReason := ""
	done := false
	for _, frame := range frames {
		data := strings.TrimSpace(frame.Data)
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			done = true
			break
		}
		if providerErr, ok := providerErrorFromStreamFrame(frame, data, "openai-compatible"); ok {
			return contract.ConversationResponse{}, providerErr
		}
		var chunk openAIChatCompletionStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid stream json"}
		}
		if chunk.Usage != nil {
			copied := *chunk.Usage
			usage = &copied
			streamEvents = append(streamEvents, contract.ConversationStreamEvent{
				Index:          eventIndex,
				Type:           contract.ConversationStreamEventUsage,
				Usage:          copied.ToUsage(builder.String()),
				RawEventType:   "usage",
				Raw:            append(json.RawMessage(nil), data...),
				OriginProtocol: "openai-compatible",
			})
			eventIndex++
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				builder.WriteString(choice.Delta.Content)
				streamEvents = append(streamEvents, contract.ConversationStreamEvent{
					Index:          eventIndex,
					Type:           contract.ConversationStreamEventContentDelta,
					ContentIndex:   choice.Index,
					Delta:          textContentDelta(choice.Delta.Content),
					RawEventType:   "chat.completion.chunk",
					Raw:            append(json.RawMessage(nil), data...),
					OriginProtocol: "openai-compatible",
				})
				eventIndex++
			}
			if reasoningText := openAIStreamReasoningText(choice.Delta); reasoningText != "" {
				sawReasoning = true
				reasoningBuilder.WriteString(reasoningText)
				streamEvents = append(streamEvents, contract.ConversationStreamEvent{
					Index:        eventIndex,
					Type:         contract.ConversationStreamEventReasoning,
					ContentIndex: choice.Index,
					Delta: contract.ContentPart{
						Kind:           contract.ContentPartThinking,
						Text:           reasoningText,
						OriginProtocol: "openai-compatible",
					},
					RawEventType:   "chat.completion.chunk",
					Raw:            append(json.RawMessage(nil), data...),
					OriginProtocol: "openai-compatible",
					Metadata:       openAIStreamChoiceMetadata(choice.Index),
				})
				eventIndex++
			} else if openAIStreamDeltaHasReasoning(choice.Delta) {
				sawReasoning = true
			}
			for _, delta := range choice.Delta.ToolCalls {
				sawToolCall = true
				toolCall := openAIStreamToolCallState(toolCalls, &toolOrder, delta.Index)
				if delta.ID != "" {
					toolCall.ID = delta.ID
				}
				if delta.Type != "" {
					toolCall.Type = delta.Type
				}
				toolCall.Function.Name += delta.Function.Name
				toolCall.Function.Arguments += delta.Function.Arguments
				streamEvents = append(streamEvents, contract.ConversationStreamEvent{
					Index:        eventIndex,
					Type:         contract.ConversationStreamEventToolCallDelta,
					ContentIndex: delta.Index,
					Delta: contract.ContentPart{
						Kind:              contract.ContentPartToolUse,
						ToolCallID:        strings.TrimSpace(delta.ID),
						ToolName:          delta.Function.Name,
						ToolArgumentsJSON: delta.Function.Arguments,
						Metadata:          map[string]any{"type": firstNonEmpty(delta.Type, "function")},
						OriginProtocol:    "openai-compatible",
					},
					RawEventType:   "chat.completion.chunk",
					Raw:            append(json.RawMessage(nil), data...),
					OriginProtocol: "openai-compatible",
					Metadata:       openAIStreamChoiceMetadata(choice.Index),
				})
				eventIndex++
			}
			if choice.Delta.FunctionCall != nil {
				sawToolCall = true
				toolCall := openAIStreamToolCallState(toolCalls, &toolOrder, 0)
				toolCall.Type = "function"
				toolCall.Function.Name += choice.Delta.FunctionCall.Name
				toolCall.Function.Arguments += choice.Delta.FunctionCall.Arguments
				streamEvents = append(streamEvents, contract.ConversationStreamEvent{
					Index:        eventIndex,
					Type:         contract.ConversationStreamEventToolCallDelta,
					ContentIndex: 0,
					Delta: contract.ContentPart{
						Kind:              contract.ContentPartToolUse,
						ToolName:          choice.Delta.FunctionCall.Name,
						ToolArgumentsJSON: choice.Delta.FunctionCall.Arguments,
						Metadata:          map[string]any{"type": "function"},
						OriginProtocol:    "openai-compatible",
					},
					RawEventType:   "chat.completion.chunk",
					Raw:            append(json.RawMessage(nil), data...),
					OriginProtocol: "openai-compatible",
					Metadata:       openAIStreamChoiceMetadata(choice.Index),
				})
				eventIndex++
			}
			if choice.FinishReason != "" {
				sawFinish = true
				finishReason = choice.FinishReason
				stopReason = openAIStopReason(choice.FinishReason)
				streamEvents = append(streamEvents, contract.ConversationStreamEvent{
					Index:          eventIndex,
					Type:           contract.ConversationStreamEventStop,
					ContentIndex:   choice.Index,
					StopReason:     stopReason,
					RawEventType:   "chat.completion.chunk",
					Raw:            append(json.RawMessage(nil), data...),
					OriginProtocol: "openai-compatible",
				})
				eventIndex++
			}
		}
	}
	if !done {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream ended before done"}
	}
	if len(streamEvents) > 0 && streamEvents[len(streamEvents)-1].Type != contract.ConversationStreamEventStop {
		streamEvents = append(streamEvents, contract.ConversationStreamEvent{
			Index:          eventIndex,
			Type:           contract.ConversationStreamEventStop,
			StopReason:     stopReason,
			RawEventType:   "done",
			OriginProtocol: "openai-compatible",
		})
		eventIndex++
	}
	parts := make([]contract.ContentPart, 0, 1+len(toolOrder))
	if text := strings.TrimSpace(reasoningBuilder.String()); text != "" {
		parts = append(parts, contract.ContentPart{Kind: contract.ContentPartThinking, Text: text, OriginProtocol: "openai-compatible"})
	}
	if text := strings.TrimSpace(builder.String()); text != "" {
		parts = append(parts, textContentPart(text))
	}
	for _, index := range toolOrder {
		if toolCall := toolCalls[index]; toolCall != nil {
			if part, ok := openAIToolCallPart(*toolCall); ok {
				parts = append(parts, part)
			}
		}
	}
	if len(parts) == 0 {
		if openAIStreamIsEmptyCompletion(sawFinish, finishReason, usage, sawReasoning, sawToolCall) {
			return contract.ConversationResponse{}, contract.ProviderError{Class: "empty_completion", StatusCode: http.StatusBadGateway, Message: "provider returned empty completion stream"}
		}
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider stream contained no content"}
	}
	text := contentPartsText(parts)
	parsedUsage := estimatedUsage(text)
	if usage != nil {
		parsedUsage = usage.ToUsage(text)
	}
	return contract.ConversationResponse{
		Parts:        parts,
		StopReason:   stopReason,
		StatusCode:   statusCode,
		Usage:        parsedUsage,
		Raw:          append(json.RawMessage(nil), body...),
		StreamEvents: streamEvents,
	}, nil
}

func openAIStreamReasoningText(delta openAIChatCompletionStreamDelta) string {
	for _, raw := range []json.RawMessage{delta.ReasoningContent, delta.Reasoning, delta.ReasoningSummary} {
		if !jsonRawMessageHasValue(raw) {
			continue
		}
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			if text != "" {
				return text
			}
		}
	}
	return ""
}

func openAIStreamDeltaHasReasoning(delta openAIChatCompletionStreamDelta) bool {
	return jsonRawMessageHasValue(delta.ReasoningContent) ||
		jsonRawMessageHasValue(delta.Reasoning) ||
		jsonRawMessageHasValue(delta.ReasoningSummary)
}

func jsonRawMessageHasValue(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null"
}

func openAIStreamIsEmptyCompletion(sawFinish bool, finishReason string, usage *openAIUsage, sawReasoning bool, sawToolCall bool) bool {
	return sawFinish &&
		strings.EqualFold(strings.TrimSpace(finishReason), "stop") &&
		usage == nil &&
		!sawReasoning &&
		!sawToolCall
}

func openAIStreamChoiceMetadata(choiceIndex int) map[string]any {
	if choiceIndex == 0 {
		return nil
	}
	return map[string]any{"choice_index": choiceIndex}
}

func openAIStreamToolCallState(values map[int]*openAIToolCall, order *[]int, index int) *openAIToolCall {
	toolCall := values[index]
	if toolCall == nil {
		toolCall = &openAIToolCall{}
		values[index] = toolCall
		*order = append(*order, index)
	}
	return toolCall
}

type geminiGenerateContentResponse struct {
	Candidates []struct {
		Content      geminiContent `json:"content"`
		FinishReason string        `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata geminiUsageMetadata `json:"usageMetadata"`
}

func (r geminiGenerateContentResponse) ConversationResponse(statusCode int) (contract.ConversationResponse, error) {
	parts := r.ContentParts()
	if len(parts) == 0 {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no content"}
	}
	text := contentPartsText(parts)
	return contract.ConversationResponse{
		Parts:      parts,
		StopReason: r.StopReason(),
		StatusCode: statusCode,
		Usage:      r.UsageMetadata.ToUsage(text),
	}, nil
}

func (r geminiGenerateContentResponse) ContentParts() []contract.ContentPart {
	parts := make([]contract.ContentPart, 0, len(r.Candidates))
	for _, candidate := range r.Candidates {
		for _, part := range candidate.Content.Parts {
			if converted, ok := geminiContentPart(part); ok {
				parts = append(parts, converted)
			}
		}
	}
	return parts
}

func (r geminiGenerateContentResponse) StopReason() contract.StopReason {
	for _, candidate := range r.Candidates {
		if reason := geminiStopReason(candidate.FinishReason, candidate.Content.Parts); reason != "" {
			return reason
		}
	}
	return contract.StopReasonEndTurn
}

func geminiContentPart(part geminiPart) (contract.ContentPart, bool) {
	if text := strings.TrimSpace(part.Text); text != "" {
		metadata := geminiPartMetadata(part)
		return contract.ContentPart{Kind: contract.ContentPartText, Text: part.Text, Metadata: metadata, OriginProtocol: "gemini"}, true
	}
	if len(part.FunctionCall) > 0 {
		name := mapString(part.FunctionCall, "name")
		arguments := "{}"
		if args, ok := part.FunctionCall["args"]; ok && args != nil {
			if raw, err := json.Marshal(args); err == nil {
				arguments = string(raw)
			}
		}
		if name == "" && arguments == "{}" {
			return contract.ContentPart{}, false
		}
		return contract.ContentPart{
			Kind:              contract.ContentPartToolUse,
			ToolName:          name,
			ToolArgumentsJSON: arguments,
			Metadata:          geminiPartMetadata(part, map[string]any{"type": "function"}),
			OriginProtocol:    "gemini",
		}, true
	}
	if len(part.FunctionResponse) > 0 {
		name := mapString(part.FunctionResponse, "name")
		response := ""
		if value, ok := part.FunctionResponse["response"]; ok && value != nil {
			if raw, err := json.Marshal(value); err == nil {
				response = string(raw)
			}
		}
		if name == "" && response == "" {
			return contract.ContentPart{}, false
		}
		return contract.ContentPart{
			Kind:           contract.ContentPartToolResult,
			ToolName:       name,
			Text:           response,
			OriginProtocol: "gemini",
		}, true
	}
	return contract.ContentPart{}, false
}

func geminiPartMetadata(part geminiPart, base ...map[string]any) map[string]any {
	var metadata map[string]any
	if len(base) > 0 {
		metadata = cloneMap(base[0])
	}
	if signature := strings.TrimSpace(part.ThoughtSignature); signature != "" {
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadata["thoughtSignature"] = signature
		metadata["signature"] = signature
	}
	return metadata
}

func geminiStopReason(reason string, parts []geminiPart) contract.StopReason {
	switch strings.ToUpper(strings.TrimSpace(reason)) {
	case "MAX_TOKENS":
		return contract.StopReasonMaxTokens
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII":
		return contract.StopReasonContentFilter
	case "MALFORMED_FUNCTION_CALL":
		return contract.StopReasonToolUse
	}
	for _, part := range parts {
		if len(part.FunctionCall) > 0 {
			return contract.StopReasonToolUse
		}
	}
	return ""
}

type geminiUsageMetadata struct {
	PromptTokenCount        *int `json:"promptTokenCount"`
	CandidatesTokenCount    *int `json:"candidatesTokenCount"`
	TotalTokenCount         *int `json:"totalTokenCount"`
	CachedContentTokenCount *int `json:"cachedContentTokenCount"`
}

func (u geminiUsageMetadata) ToUsage(text string) contract.Usage {
	input := valueOrZero(u.PromptTokenCount)
	output := valueOrZero(u.CandidatesTokenCount)
	cached := valueOrZero(u.CachedContentTokenCount)
	total := input + output + cached
	if u.TotalTokenCount != nil && *u.TotalTokenCount > 0 && total == 0 {
		total = *u.TotalTokenCount
	}
	if total > 0 && output == 0 {
		output = max(0, total-input-cached)
	}
	if input == 0 && output == 0 && cached == 0 {
		return estimatedUsage(text)
	}
	return contract.Usage{
		InputTokens:  input,
		OutputTokens: output,
		CachedTokens: cached,
		Estimated:    false,
	}
}

func (u *geminiUsageMetadata) Merge(next geminiUsageMetadata) {
	if u == nil {
		return
	}
	if next.PromptTokenCount != nil {
		u.PromptTokenCount = cloneIntPtr(next.PromptTokenCount)
	}
	if next.CandidatesTokenCount != nil {
		u.CandidatesTokenCount = cloneIntPtr(next.CandidatesTokenCount)
	}
	if next.TotalTokenCount != nil {
		u.TotalTokenCount = cloneIntPtr(next.TotalTokenCount)
	}
	if next.CachedContentTokenCount != nil {
		u.CachedContentTokenCount = cloneIntPtr(next.CachedContentTokenCount)
	}
}

func (u geminiUsageMetadata) HasTokenUsage() bool {
	return u.PromptTokenCount != nil ||
		u.CandidatesTokenCount != nil ||
		u.TotalTokenCount != nil ||
		u.CachedContentTokenCount != nil
}

func parseGeminiCompatibleStream(body []byte, statusCode int) (contract.ConversationResponse, error) {
	frames, err := parseSSEFrames(body)
	if err != nil {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
	}
	var usage geminiUsageMetadata
	var parts []contract.ContentPart
	streamEvents := make([]contract.ConversationStreamEvent, 0)
	eventIndex := 0
	stopReason := contract.StopReasonEndTurn
	seenChunk := false
	for _, frame := range frames {
		data := strings.TrimSpace(frame.Data)
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			break
		}
		if providerErr, ok := providerErrorFromStreamFrame(frame, data, "gemini-compatible"); ok {
			return contract.ConversationResponse{}, providerErr
		}
		var chunk geminiGenerateContentResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid stream json"}
		}
		seenChunk = true
		chunkParts := chunk.ContentParts()
		parts = appendStreamContentParts(parts, chunkParts)
		for contentIndex, part := range chunkParts {
			eventType := contract.ConversationStreamEventContentDelta
			switch part.Kind {
			case contract.ContentPartToolUse:
				eventType = contract.ConversationStreamEventToolCallDelta
			case contract.ContentPartToolResult:
				eventType = contract.ConversationStreamEventToolResult
			case contract.ContentPartThinking:
				eventType = contract.ConversationStreamEventReasoning
			}
			part.OriginProtocol = firstNonEmpty(part.OriginProtocol, "gemini-compatible")
			streamEvents = append(streamEvents, contract.ConversationStreamEvent{
				Index:          eventIndex,
				Type:           eventType,
				ContentIndex:   contentIndex,
				Delta:          part,
				RawEventType:   "generateContentResponse",
				Raw:            append(json.RawMessage(nil), data...),
				OriginProtocol: "gemini-compatible",
			})
			eventIndex++
		}
		if reason := chunk.StopReason(); reason != contract.StopReasonEndTurn {
			stopReason = reason
			streamEvents = append(streamEvents, contract.ConversationStreamEvent{
				Index:          eventIndex,
				Type:           contract.ConversationStreamEventStop,
				StopReason:     stopReason,
				RawEventType:   "generateContentResponse",
				Raw:            append(json.RawMessage(nil), data...),
				OriginProtocol: "gemini-compatible",
			})
			eventIndex++
		}
		usage.Merge(chunk.UsageMetadata)
		if chunk.UsageMetadata.HasTokenUsage() {
			streamEvents = append(streamEvents, contract.ConversationStreamEvent{
				Index:          eventIndex,
				Type:           contract.ConversationStreamEventUsage,
				Usage:          usage.ToUsage(contentPartsText(parts)),
				RawEventType:   "generateContentResponse",
				Raw:            append(json.RawMessage(nil), data...),
				OriginProtocol: "gemini-compatible",
			})
			eventIndex++
		}
	}
	if !seenChunk {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream ended before chunk"}
	}
	if len(parts) == 0 {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider stream contained no content"}
	}
	text := contentPartsText(parts)
	return contract.ConversationResponse{
		Parts:        parts,
		StopReason:   stopReason,
		StatusCode:   statusCode,
		Usage:        usage.ToUsage(text),
		Raw:          append(json.RawMessage(nil), body...),
		StreamEvents: streamEvents,
	}, nil
}

func appendStreamContentParts(parts []contract.ContentPart, next []contract.ContentPart) []contract.ContentPart {
	for _, part := range next {
		if len(parts) > 0 && part.Kind == contract.ContentPartText {
			last := &parts[len(parts)-1]
			if last.Kind == contract.ContentPartText && len(last.Metadata) == 0 && len(part.Metadata) == 0 {
				last.Text += part.Text
				continue
			}
		}
		parts = append(parts, part)
	}
	return parts
}

type anthropicMessagesResponse struct {
	Content    []anthropicContentBlock `json:"content"`
	StopReason string                  `json:"stop_reason"`
	Usage      anthropicUsage          `json:"usage"`
}

type anthropicContentBlock struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	Data      string          `json:"data"`
	Signature string          `json:"signature"`
}

type anthropicStreamChunk struct {
	Index        *int                   `json:"index"`
	Type         string                 `json:"type"`
	ContentBlock *anthropicContentBlock `json:"content_block"`
	Delta        struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		PartialJSON string `json:"partial_json"`
		Signature   string `json:"signature"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	Message *struct {
		Usage anthropicUsage `json:"usage"`
	} `json:"message"`
	Usage *anthropicUsage `json:"usage"`
}

func parseAnthropicCompatibleJSON(body []byte, statusCode int) (contract.ConversationResponse, error) {
	var decoded anthropicMessagesResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	resp, err := decoded.ConversationResponse(statusCode)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	resp.Raw = append(json.RawMessage(nil), body...)
	return resp, nil
}

func (r anthropicMessagesResponse) ConversationResponse(statusCode int) (contract.ConversationResponse, error) {
	parts := anthropicContentParts(r.Content)
	if len(parts) == 0 {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no content"}
	}
	text := contentPartsText(parts)
	return contract.ConversationResponse{
		Parts:      parts,
		StopReason: anthropicStopReason(r.StopReason),
		StatusCode: statusCode,
		Usage:      r.Usage.ToUsage(text),
	}, nil
}

func anthropicContentParts(blocks []anthropicContentBlock) []contract.ContentPart {
	parts := make([]contract.ContentPart, 0, len(blocks))
	for _, block := range blocks {
		if part, ok := anthropicContentPart(block); ok {
			parts = append(parts, part)
		}
	}
	return parts
}

func anthropicContentPart(block anthropicContentBlock) (contract.ContentPart, bool) {
	blockType := strings.ToLower(strings.TrimSpace(block.Type))
	switch blockType {
	case "", "text":
		if text := strings.TrimSpace(block.Text); text != "" {
			return textContentPart(text), true
		}
	case "tool_use", "server_tool_use":
		arguments := string(block.Input)
		if strings.TrimSpace(block.ID) == "" && strings.TrimSpace(block.Name) == "" && strings.TrimSpace(arguments) == "" {
			return contract.ContentPart{}, false
		}
		metadata := map[string]any{"type": blockType}
		if signature := strings.TrimSpace(block.Signature); signature != "" {
			metadata["signature"] = signature
		}
		return contract.ContentPart{
			Kind:              contract.ContentPartToolUse,
			ToolCallID:        strings.TrimSpace(block.ID),
			ToolName:          strings.TrimSpace(block.Name),
			ToolArgumentsJSON: arguments,
			Metadata:          metadata,
			OriginProtocol:    "anthropic",
		}, true
	case "thinking":
		text := block.Thinking
		if text == "" {
			text = block.Text
		}
		if text = strings.TrimSpace(text); text != "" {
			metadata := map[string]any(nil)
			if signature := strings.TrimSpace(block.Signature); signature != "" {
				metadata = map[string]any{"signature": signature}
			}
			return contract.ContentPart{Kind: contract.ContentPartThinking, Text: text, Metadata: metadata, OriginProtocol: "anthropic"}, true
		}
	case "redacted_thinking":
		metadata := map[string]any{"type": blockType}
		if data := strings.TrimSpace(block.Data); data != "" {
			metadata["data"] = data
		}
		raw := anthropicBlockRaw(block)
		return contract.ContentPart{Kind: contract.ContentPartThinking, Metadata: metadata, Raw: raw, OriginProtocol: "anthropic"}, true
	}
	return contract.ContentPart{}, false
}

func anthropicBlockRaw(block anthropicContentBlock) json.RawMessage {
	payload := map[string]any{"type": strings.TrimSpace(block.Type)}
	setMapString(payload, "id", block.ID)
	setMapString(payload, "name", block.Name)
	if len(block.Input) > 0 {
		var input any
		if err := json.Unmarshal(block.Input, &input); err == nil {
			payload["input"] = input
		}
	}
	setMapString(payload, "text", block.Text)
	setMapString(payload, "thinking", block.Thinking)
	setMapString(payload, "data", block.Data)
	setMapString(payload, "signature", block.Signature)
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func parseAnthropicCompatibleStream(body []byte, statusCode int) (contract.ConversationResponse, error) {
	frames, err := parseSSEFrames(body)
	if err != nil {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
	}
	var builder strings.Builder
	var usage anthropicUsage
	blocks := map[int]*anthropicContentBlock{}
	order := []int{}
	lastIndex := -1
	streamEvents := make([]contract.ConversationStreamEvent, 0)
	eventIndex := 0
	stopReason := contract.StopReasonEndTurn
	done := false
	for _, frame := range frames {
		data := strings.TrimSpace(frame.Data)
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			done = true
			break
		}
		if providerErr, ok := providerErrorFromStreamFrame(frame, data, "anthropic-compatible"); ok {
			return contract.ConversationResponse{}, providerErr
		}
		var chunk anthropicStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid stream json"}
		}
		chunkType := frame.EventType(chunk.Type)
		chunk.Type = chunkType
		switch chunkType {
		case "content_block_start":
			index := anthropicStreamIndex(chunk.Index, &lastIndex, len(order))
			block := anthropicStreamBlockState(blocks, &order, index)
			if chunk.ContentBlock != nil {
				*block = *chunk.ContentBlock
				if strings.TrimSpace(block.Type) == "" {
					block.Type = "text"
				}
				if strings.EqualFold(strings.TrimSpace(block.Type), "tool_use") || strings.EqualFold(strings.TrimSpace(block.Type), "server_tool_use") {
					streamEvents = append(streamEvents, contract.ConversationStreamEvent{
						Index:        eventIndex,
						Type:         contract.ConversationStreamEventToolCallDelta,
						ContentIndex: index,
						Delta: contract.ContentPart{
							Kind:           contract.ContentPartToolUse,
							ToolCallID:     strings.TrimSpace(block.ID),
							ToolName:       strings.TrimSpace(block.Name),
							Metadata:       map[string]any{"type": strings.TrimSpace(block.Type)},
							OriginProtocol: "anthropic-compatible",
						},
						RawEventType:   chunkType,
						Raw:            append(json.RawMessage(nil), data...),
						OriginProtocol: "anthropic-compatible",
					})
					eventIndex++
				}
				if strings.TrimSpace(chunk.ContentBlock.Text) != "" {
					builder.WriteString(chunk.ContentBlock.Text)
					streamEvents = append(streamEvents, contract.ConversationStreamEvent{
						Index:          eventIndex,
						Type:           contract.ConversationStreamEventContentDelta,
						ContentIndex:   index,
						Delta:          textContentDelta(chunk.ContentBlock.Text),
						RawEventType:   chunkType,
						Raw:            append(json.RawMessage(nil), data...),
						OriginProtocol: "anthropic-compatible",
					})
					eventIndex++
				}
			}
		case "content_block_delta":
			index := anthropicStreamIndex(chunk.Index, &lastIndex, len(order))
			block := anthropicStreamBlockState(blocks, &order, index)
			switch strings.TrimSpace(chunk.Delta.Type) {
			case "input_json_delta":
				block.Type = "tool_use"
				if strings.TrimSpace(string(block.Input)) == "{}" {
					block.Input = nil
				}
				block.Input = append(block.Input, chunk.Delta.PartialJSON...)
				streamEvents = append(streamEvents, contract.ConversationStreamEvent{
					Index:        eventIndex,
					Type:         contract.ConversationStreamEventToolCallDelta,
					ContentIndex: index,
					Delta: contract.ContentPart{
						Kind:              contract.ContentPartToolUse,
						ToolCallID:        strings.TrimSpace(block.ID),
						ToolName:          strings.TrimSpace(block.Name),
						ToolArgumentsJSON: chunk.Delta.PartialJSON,
						Metadata:          map[string]any{"type": "tool_use"},
						OriginProtocol:    "anthropic-compatible",
					},
					RawEventType:   chunkType,
					Raw:            append(json.RawMessage(nil), data...),
					OriginProtocol: "anthropic-compatible",
				})
				eventIndex++
			case "thinking_delta":
				block.Type = "thinking"
				text := chunk.Delta.Thinking
				if text == "" {
					text = chunk.Delta.Text
				}
				block.Text += text
				if text != "" {
					streamEvents = append(streamEvents, contract.ConversationStreamEvent{
						Index:          eventIndex,
						Type:           contract.ConversationStreamEventReasoning,
						ContentIndex:   index,
						Delta:          contract.ContentPart{Kind: contract.ContentPartThinking, Text: text, OriginProtocol: "anthropic-compatible"},
						RawEventType:   chunkType,
						Raw:            append(json.RawMessage(nil), data...),
						OriginProtocol: "anthropic-compatible",
					})
					eventIndex++
				}
			case "signature_delta":
				block.Type = "thinking"
				block.Signature += chunk.Delta.Signature
				if chunk.Delta.Signature != "" {
					streamEvents = append(streamEvents, contract.ConversationStreamEvent{
						Index:        eventIndex,
						Type:         contract.ConversationStreamEventReasoning,
						ContentIndex: index,
						Delta: contract.ContentPart{
							Kind:           contract.ContentPartThinking,
							Metadata:       map[string]any{"signature_delta": chunk.Delta.Signature},
							OriginProtocol: "anthropic-compatible",
						},
						RawEventType:   chunkType,
						Raw:            append(json.RawMessage(nil), data...),
						OriginProtocol: "anthropic-compatible",
					})
					eventIndex++
				}
			default:
				if strings.TrimSpace(block.Type) == "" {
					block.Type = "text"
				}
				block.Text += chunk.Delta.Text
				builder.WriteString(chunk.Delta.Text)
				if chunk.Delta.Text != "" {
					streamEvents = append(streamEvents, contract.ConversationStreamEvent{
						Index:          eventIndex,
						Type:           contract.ConversationStreamEventContentDelta,
						ContentIndex:   index,
						Delta:          textContentDelta(chunk.Delta.Text),
						RawEventType:   chunkType,
						Raw:            append(json.RawMessage(nil), data...),
						OriginProtocol: "anthropic-compatible",
					})
					eventIndex++
				}
			}
		case "message_start":
			if chunk.Message != nil {
				usage.Merge(chunk.Message.Usage)
				streamEvents = append(streamEvents, contract.ConversationStreamEvent{
					Index:          eventIndex,
					Type:           contract.ConversationStreamEventUsage,
					Usage:          chunk.Message.Usage.ToUsage(builder.String()),
					RawEventType:   chunkType,
					Raw:            append(json.RawMessage(nil), data...),
					OriginProtocol: "anthropic-compatible",
				})
				eventIndex++
			}
		case "message_delta":
			if chunk.Delta.StopReason != "" {
				stopReason = anthropicStopReason(chunk.Delta.StopReason)
				streamEvents = append(streamEvents, contract.ConversationStreamEvent{
					Index:          eventIndex,
					Type:           contract.ConversationStreamEventStop,
					StopReason:     stopReason,
					RawEventType:   chunkType,
					Raw:            append(json.RawMessage(nil), data...),
					OriginProtocol: "anthropic-compatible",
				})
				eventIndex++
			}
			if chunk.Usage != nil {
				usage.Merge(*chunk.Usage)
				streamEvents = append(streamEvents, contract.ConversationStreamEvent{
					Index:          eventIndex,
					Type:           contract.ConversationStreamEventUsage,
					Usage:          usage.ToUsage(builder.String()),
					RawEventType:   chunkType,
					Raw:            append(json.RawMessage(nil), data...),
					OriginProtocol: "anthropic-compatible",
				})
				eventIndex++
			}
		case "message_stop":
			done = true
		}
	}
	if !done {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream ended before done"}
	}
	streamEvents = appendAnthropicTerminalStopEvent(streamEvents, eventIndex, stopReason)
	return anthropicStreamResponse(body, statusCode, blocks, order, stopReason, usage, streamEvents)
}

func anthropicStreamResponse(body []byte, statusCode int, blocks map[int]*anthropicContentBlock, order []int, stopReason contract.StopReason, usage anthropicUsage, streamEvents []contract.ConversationStreamEvent) (contract.ConversationResponse, error) {
	parts := make([]contract.ContentPart, 0, len(order))
	for _, index := range order {
		if block := blocks[index]; block != nil {
			if part, ok := anthropicContentPart(*block); ok {
				parts = append(parts, part)
			}
		}
	}
	if len(parts) == 0 {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider stream contained no content"}
	}
	text := contentPartsText(parts)
	return contract.ConversationResponse{
		Parts:        parts,
		StopReason:   stopReason,
		StatusCode:   statusCode,
		Usage:        usage.ToUsage(text),
		Raw:          append(json.RawMessage(nil), body...),
		StreamEvents: streamEvents,
	}, nil
}

func appendAnthropicTerminalStopEvent(events []contract.ConversationStreamEvent, index int, stopReason contract.StopReason) []contract.ConversationStreamEvent {
	if len(events) == 0 || events[len(events)-1].Type == contract.ConversationStreamEventStop {
		return events
	}
	return append(events, contract.ConversationStreamEvent{
		Index:          index,
		Type:           contract.ConversationStreamEventStop,
		StopReason:     stopReason,
		RawEventType:   "message_stop",
		OriginProtocol: "anthropic-compatible",
	})
}

func anthropicStreamIndex(value *int, lastIndex *int, orderLen int) int {
	if value != nil {
		*lastIndex = *value
		return *value
	}
	if *lastIndex >= 0 {
		return *lastIndex
	}
	*lastIndex = orderLen
	return *lastIndex
}

func anthropicStreamBlockState(values map[int]*anthropicContentBlock, order *[]int, index int) *anthropicContentBlock {
	block := values[index]
	if block == nil {
		block = &anthropicContentBlock{}
		values[index] = block
		*order = append(*order, index)
	}
	return block
}
