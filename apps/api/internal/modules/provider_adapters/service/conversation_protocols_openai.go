package service

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

type openAIChatCompletionRequest struct {
	Model           string               `json:"model"`
	Messages        []openAIChatMessage  `json:"messages"`
	Stream          bool                 `json:"stream"`
	StreamOptions   *openAIStreamOptions `json:"stream_options,omitempty"`
	ReasoningEffort string               `json:"reasoning_effort,omitempty"`
	Temperature     *float32             `json:"temperature,omitempty"`
	TopP            *float32             `json:"top_p,omitempty"`
	MaxTokens       *int                 `json:"max_tokens,omitempty"`
	Stop            []string             `json:"stop,omitempty"`
	Tools           []map[string]any     `json:"tools,omitempty"`
	ToolChoice      any                  `json:"tool_choice,omitempty"`
	ResponseFormat  map[string]any       `json:"response_format,omitempty"`
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
	if effort := strings.TrimSpace(metadataString(req.Reasoning, "effort")); effort != "" {
		payload.ReasoningEffort = effort
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
		normalizeOpenAICompatibleServiceTier(payload)
		return json.Marshal(payload)
	}
	payload := openAICompatiblePayload(req)
	if req.Stream {
		payload.StreamOptions = &openAIStreamOptions{IncludeUsage: true}
	}
	return json.Marshal(payload)
}

func normalizeOpenAICompatibleServiceTier(payload map[string]any) {
	if payload == nil {
		return
	}
	raw, ok := payload["service_tier"].(string)
	if !ok {
		return
	}
	if strings.EqualFold(strings.TrimSpace(raw), "fast") {
		payload["service_tier"] = "priority"
	}
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
	PromptTokens             *int `json:"prompt_tokens"`
	CompletionTokens         *int `json:"completion_tokens"`
	TotalTokens              *int `json:"total_tokens"`
	InputTokens              *int `json:"input_tokens"`
	OutputTokens             *int `json:"output_tokens"`
	CachedTokens             *int `json:"cached_tokens"`
	CacheReadTokens          *int `json:"cache_read_tokens"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens"`
	CacheCreationTokens      *int `json:"cache_creation_tokens"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens"`
	CacheCreation5mTokens    *int `json:"cache_creation_ephemeral_5m_input_tokens"`
	CacheCreation1hTokens    *int `json:"cache_creation_ephemeral_1h_input_tokens"`
	InputTokensDetails       *struct {
		CachedTokens        *int `json:"cached_tokens"`
		CacheCreationTokens *int `json:"cache_creation_tokens"`
	} `json:"input_tokens_details"`
	PromptTokensDetails *struct {
		CachedTokens *int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokensDetails *struct {
		ReasoningTokens *int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
	OutputTokensDetails *struct {
		ImageTokens     *int `json:"image_tokens"`
		ReasoningTokens *int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

func (u openAIUsage) ToUsage(text string) contract.Usage {
	rawInput := valueOrZero(u.InputTokens)
	if rawInput == 0 {
		rawInput = valueOrZero(u.PromptTokens)
	}
	output := valueOrZero(u.OutputTokens)
	if output == 0 {
		output = valueOrZero(u.CompletionTokens)
	}
	imageOutput := 0
	if u.OutputTokensDetails != nil {
		imageOutput = valueOrZero(u.OutputTokensDetails.ImageTokens)
	}
	reasoningOutput := 0
	if u.OutputTokensDetails != nil {
		reasoningOutput = valueOrZero(u.OutputTokensDetails.ReasoningTokens)
	}
	if reasoningOutput == 0 && u.CompletionTokensDetails != nil {
		reasoningOutput = valueOrZero(u.CompletionTokensDetails.ReasoningTokens)
	}
	cached := valueOrZero(u.CacheReadInputTokens)
	if cached == 0 {
		cached = valueOrZero(u.CacheReadTokens)
	}
	if cached == 0 {
		cached = valueOrZero(u.CachedTokens)
	}
	if cached == 0 && u.InputTokensDetails != nil {
		cached = valueOrZero(u.InputTokensDetails.CachedTokens)
	}
	if cached == 0 && u.PromptTokensDetails != nil {
		cached = valueOrZero(u.PromptTokensDetails.CachedTokens)
	}
	cacheCreation5mRaw := valueOrZero(u.CacheCreation5mTokens)
	cacheCreation1hRaw := valueOrZero(u.CacheCreation1hTokens)
	cacheCreation := valueOrZero(u.CacheCreationInputTokens)
	if cacheCreation == 0 {
		cacheCreation = valueOrZero(u.CacheCreationTokens)
	}
	if cacheCreation == 0 && u.InputTokensDetails != nil {
		cacheCreation = valueOrZero(u.InputTokensDetails.CacheCreationTokens)
	}
	if cacheCreation == 0 {
		cacheCreation = cacheCreation5mRaw + cacheCreation1hRaw
	}
	cacheCreation5m, cacheCreation1h := openAICacheCreationBuckets(cacheCreation, cacheCreation5mRaw, cacheCreation1hRaw)
	input := max(0, rawInput-cached)
	total := input + output + cached
	if u.TotalTokens != nil && *u.TotalTokens > 0 && total == 0 {
		total = *u.TotalTokens
	}
	if total > 0 && output == 0 {
		output = max(0, total-input-cached)
	}
	if output < imageOutput {
		output = imageOutput
	}
	if output < reasoningOutput {
		output = reasoningOutput
	}
	if input == 0 && output == 0 && cached == 0 && imageOutput == 0 && reasoningOutput == 0 && cacheCreation == 0 && !u.HasTokenUsage() {
		return estimatedUsage(text)
	}
	return contract.Usage{
		InputTokens:           input,
		OutputTokens:          output,
		ImageOutputTokens:     imageOutput,
		CachedTokens:          cached,
		CacheCreationTokens:   cacheCreation,
		CacheCreation5mTokens: cacheCreation5m,
		CacheCreation1hTokens: cacheCreation1h,
		Observed:              true,
		Estimated:             false,
	}
}

func openAICacheCreationBuckets(total int, fiveMinutes int, oneHour int) (int, int) {
	if total > 0 && fiveMinutes == 0 && oneHour == 0 {
		return total, 0
	}
	if fiveMinutes+oneHour < total {
		fiveMinutes += total - fiveMinutes - oneHour
	}
	return fiveMinutes, oneHour
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
		u.CacheCreationInputTokens != nil ||
		u.CacheCreation5mTokens != nil ||
		u.CacheCreation1hTokens != nil ||
		(u.InputTokensDetails != nil && u.InputTokensDetails.CachedTokens != nil) ||
		(u.PromptTokensDetails != nil && u.PromptTokensDetails.CachedTokens != nil) ||
		(u.CompletionTokensDetails != nil && u.CompletionTokensDetails.ReasoningTokens != nil) ||
		(u.OutputTokensDetails != nil && (u.OutputTokensDetails.ImageTokens != nil || u.OutputTokensDetails.ReasoningTokens != nil))
}

func parseOpenAICompatibleStream(body []byte, statusCode int) (contract.ConversationResponse, error) {
	frames, err := parseSSEFrames(body)
	if err != nil {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
	}
	state := newOpenAIStreamParseState()
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
		state.handleOpenAIStreamChunk(data, chunk)
	}
	if !done {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream ended before done"}
	}
	return state.openAIStreamResponse(body, statusCode)
}

type openAIStreamParseState struct {
	builder          strings.Builder
	reasoningBuilder strings.Builder
	usage            *openAIUsage
	toolCalls        map[int]*openAIToolCall
	toolOrder        []int
	streamEvents     []contract.ConversationStreamEvent
	eventIndex       int
	stopReason       contract.StopReason
	sawReasoning     bool
	sawFinish        bool
	sawToolCall      bool
	finishReason     string
}

func newOpenAIStreamParseState() *openAIStreamParseState {
	return &openAIStreamParseState{
		toolCalls:    map[int]*openAIToolCall{},
		toolOrder:    []int{},
		streamEvents: make([]contract.ConversationStreamEvent, 0),
		stopReason:   contract.StopReasonEndTurn,
	}
}

func (s *openAIStreamParseState) handleOpenAIStreamChunk(data string, chunk openAIChatCompletionStreamChunk) {
	if chunk.Usage != nil {
		copied := *chunk.Usage
		s.usage = &copied
		s.streamEvents = append(s.streamEvents, contract.ConversationStreamEvent{
			Index:          s.eventIndex,
			Type:           contract.ConversationStreamEventUsage,
			Usage:          copied.ToUsage(s.builder.String()),
			RawEventType:   "usage",
			Raw:            append(json.RawMessage(nil), data...),
			OriginProtocol: "openai-compatible",
		})
		s.eventIndex++
	}
	for _, choice := range chunk.Choices {
		s.handleOpenAIStreamChoice(data, choice)
	}
}

func (s *openAIStreamParseState) handleOpenAIStreamChoice(data string, choice openAIChatCompletionStreamChoice) {
	s.handleOpenAIStreamContent(data, choice)
	s.handleOpenAIStreamReasoning(data, choice)
	s.handleOpenAIStreamToolCalls(data, choice)
	s.handleOpenAIStreamFunctionCall(data, choice)
	if choice.FinishReason == "" {
		return
	}
	s.sawFinish = true
	s.finishReason = choice.FinishReason
	s.stopReason = openAIStopReason(choice.FinishReason)
	s.streamEvents = append(s.streamEvents, contract.ConversationStreamEvent{
		Index:          s.eventIndex,
		Type:           contract.ConversationStreamEventStop,
		ContentIndex:   choice.Index,
		StopReason:     s.stopReason,
		RawEventType:   "chat.completion.chunk",
		Raw:            append(json.RawMessage(nil), data...),
		OriginProtocol: "openai-compatible",
	})
	s.eventIndex++
}

func (s *openAIStreamParseState) handleOpenAIStreamContent(data string, choice openAIChatCompletionStreamChoice) {
	if choice.Delta.Content == "" {
		return
	}
	s.builder.WriteString(choice.Delta.Content)
	s.streamEvents = append(s.streamEvents, contract.ConversationStreamEvent{
		Index:          s.eventIndex,
		Type:           contract.ConversationStreamEventContentDelta,
		ContentIndex:   choice.Index,
		Delta:          textContentDelta(choice.Delta.Content),
		RawEventType:   "chat.completion.chunk",
		Raw:            append(json.RawMessage(nil), data...),
		OriginProtocol: "openai-compatible",
	})
	s.eventIndex++
}

func (s *openAIStreamParseState) handleOpenAIStreamReasoning(data string, choice openAIChatCompletionStreamChoice) {
	reasoningText := openAIStreamReasoningText(choice.Delta)
	if reasoningText == "" {
		if openAIStreamDeltaHasReasoning(choice.Delta) {
			s.sawReasoning = true
		}
		return
	}
	s.sawReasoning = true
	s.reasoningBuilder.WriteString(reasoningText)
	s.streamEvents = append(s.streamEvents, contract.ConversationStreamEvent{
		Index:        s.eventIndex,
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
	s.eventIndex++
}

func (s *openAIStreamParseState) handleOpenAIStreamToolCalls(data string, choice openAIChatCompletionStreamChoice) {
	for _, delta := range choice.Delta.ToolCalls {
		s.sawToolCall = true
		toolCall := openAIStreamToolCallState(s.toolCalls, &s.toolOrder, delta.Index)
		if delta.ID != "" {
			toolCall.ID = delta.ID
		}
		if delta.Type != "" {
			toolCall.Type = delta.Type
		}
		toolCall.Function.Name += delta.Function.Name
		toolCall.Function.Arguments += delta.Function.Arguments
		s.streamEvents = append(s.streamEvents, contract.ConversationStreamEvent{
			Index:        s.eventIndex,
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
		s.eventIndex++
	}
}

func (s *openAIStreamParseState) handleOpenAIStreamFunctionCall(data string, choice openAIChatCompletionStreamChoice) {
	if choice.Delta.FunctionCall == nil {
		return
	}
	s.sawToolCall = true
	toolCall := openAIStreamToolCallState(s.toolCalls, &s.toolOrder, 0)
	toolCall.Type = "function"
	toolCall.Function.Name += choice.Delta.FunctionCall.Name
	toolCall.Function.Arguments += choice.Delta.FunctionCall.Arguments
	s.streamEvents = append(s.streamEvents, contract.ConversationStreamEvent{
		Index:        s.eventIndex,
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
	s.eventIndex++
}

func (s *openAIStreamParseState) openAIStreamResponse(body []byte, statusCode int) (contract.ConversationResponse, error) {
	if len(s.streamEvents) > 0 && s.streamEvents[len(s.streamEvents)-1].Type != contract.ConversationStreamEventStop {
		s.streamEvents = append(s.streamEvents, contract.ConversationStreamEvent{
			Index:          s.eventIndex,
			Type:           contract.ConversationStreamEventStop,
			StopReason:     s.stopReason,
			RawEventType:   "done",
			OriginProtocol: "openai-compatible",
		})
		s.eventIndex++
	}
	parts := make([]contract.ContentPart, 0, 1+len(s.toolOrder))
	if text := strings.TrimSpace(s.reasoningBuilder.String()); text != "" {
		parts = append(parts, contract.ContentPart{Kind: contract.ContentPartThinking, Text: text, OriginProtocol: "openai-compatible"})
	}
	if text := strings.TrimSpace(s.builder.String()); text != "" {
		parts = append(parts, textContentPart(text))
	}
	for _, index := range s.toolOrder {
		if toolCall := s.toolCalls[index]; toolCall != nil {
			if part, ok := openAIToolCallPart(*toolCall); ok {
				parts = append(parts, part)
			}
		}
	}
	if len(parts) == 0 {
		if openAIStreamIsEmptyCompletion(s.sawFinish, s.finishReason, s.usage, s.sawReasoning, s.sawToolCall) {
			return contract.ConversationResponse{}, contract.ProviderError{Class: "empty_completion", StatusCode: http.StatusBadGateway, Message: "provider returned empty completion stream"}
		}
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider stream contained no content"}
	}
	text := contentPartsText(parts)
	parsedUsage := estimatedUsage(text)
	if s.usage != nil {
		parsedUsage = s.usage.ToUsage(text)
	}
	return contract.ConversationResponse{
		Parts:        parts,
		StopReason:   s.stopReason,
		StatusCode:   statusCode,
		Usage:        parsedUsage,
		Raw:          append(json.RawMessage(nil), body...),
		StreamEvents: s.streamEvents,
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
