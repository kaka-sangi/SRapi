package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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
	maxTokens := anthropicCompatibleMaxTokens(req)
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

func anthropicCompatibleMaxTokens(req contract.ConversationRequest) int {
	if req.MaxOutputTokens != nil && *req.MaxOutputTokens > 0 {
		return *req.MaxOutputTokens
	}
	maxTokens := 1024
	if budget, ok := anthropicThinkingBudget(req.Reasoning); ok && budget >= maxTokens {
		return budget + 1000
	}
	return maxTokens
}

func anthropicCompatibleRequestBody(req contract.ConversationRequest) ([]byte, error) {
	raw, err := anthropicCompatibleRequestBodyRaw(req)
	if err != nil {
		return nil, err
	}
	// PR-X security wiring: strip forged Claude thinking blocks before
	// any payload transform / upstream forward. See
	// claude_signature_wiring.go for the rationale (forged thinking
	// blocks can bypass upstream signature checks on permissive
	// Claude-compatible backends). The strip is idempotent and a no-op
	// for payloads with no thinking content, so we can run it on every
	// outbound build without conditioning on adapter type.
	raw = claudeThinkingSanitizeRawPayload(req.Mapping.UpstreamModelName, raw)
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
		return anthropicEnabledThinking(reasoning, maxTokens)
	case "adaptive":
		return map[string]any{"type": "adaptive"}
	case "disabled":
		return map[string]any{"type": "disabled"}
	}

	effort := strings.ToLower(strings.TrimSpace(metadataString(reasoning, "effort")))
	if effort == "" {
		effort = strings.ToLower(strings.TrimSpace(metadataString(reasoning, "level")))
	}
	if effort == "" {
		return nil
	}
	switch effort {
	case "none":
		return map[string]any{"type": "disabled"}
	case "auto":
		return map[string]any{"type": "enabled"}
	default:
		budget, ok := anthropicThinkingBudgetForEffort(effort)
		if !ok {
			return nil
		}
		return anthropicEnabledThinking(map[string]any{"budget_tokens": budget}, maxTokens)
	}
}

func anthropicEnabledThinking(reasoning map[string]any, maxTokens int) map[string]any {
	budget := positiveIntValue(reasoning["budget_tokens"])
	if budget <= 0 || budget >= maxTokens {
		budget = maxTokens - 1
	}
	if budget < 1024 {
		return nil
	}
	return map[string]any{
		"type":          "enabled",
		"budget_tokens": budget,
	}
}

func anthropicThinkingBudget(reasoning map[string]any) (int, bool) {
	if budget := positiveIntValue(reasoning["budget_tokens"]); budget > 0 {
		return budget, true
	}
	effort := strings.ToLower(strings.TrimSpace(metadataString(reasoning, "effort")))
	if effort == "" {
		effort = strings.ToLower(strings.TrimSpace(metadataString(reasoning, "level")))
	}
	return anthropicThinkingBudgetForEffort(effort)
}

func anthropicThinkingBudgetForEffort(effort string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "none":
		return 0, true
	case "auto":
		return -1, true
	case "minimal":
		return 512, true
	case "low":
		return 1024, true
	case "medium":
		return 8192, true
	case "high":
		return 24576, true
	case "xhigh":
		return 32768, true
	case "max":
		return 128000, true
	default:
		return 0, false
	}
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
			setMapString(block, "id", sanitizeAnthropicToolUseID(part.ToolCallID))
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
			setMapString(block, "tool_use_id", sanitizeAnthropicToolUseID(firstNonEmpty(part.ToolResultForID, part.ToolCallID)))
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

func sanitizeAnthropicToolUseID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(id))
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
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

type geminiGenerateContentResponse struct {
	Candidates []struct {
		Content      geminiContent `json:"content"`
		FinishReason string        `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata      geminiUsageMetadata `json:"usageMetadata"`
	UsageMetadataSnake geminiUsageMetadata `json:"usage_metadata"`
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
		Usage:      r.Usage().ToUsage(text),
	}, nil
}

func (r geminiGenerateContentResponse) Usage() geminiUsageMetadata {
	if r.UsageMetadata.HasTokenUsage() || !r.UsageMetadataSnake.HasTokenUsage() {
		return r.UsageMetadata
	}
	return r.UsageMetadataSnake
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
	if part.Thought {
		// Gemini marks reasoning parts with thought:true. Surface them as a
		// thinking block (carrying any thoughtSignature) instead of leaking the
		// chain-of-thought to the client as visible assistant text. An empty-text
		// part that only carries a signature still emits a thinking block so the
		// orphan signature can be passed back upstream on the next turn.
		metadata := geminiPartMetadata(part)
		if strings.TrimSpace(part.Text) == "" && len(metadata) == 0 {
			return contract.ContentPart{}, false
		}
		return contract.ContentPart{Kind: contract.ContentPartThinking, Text: part.Text, Metadata: metadata, OriginProtocol: "gemini"}, true
	}
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
		// Gemini functionCall carries no id, but OpenAI/Anthropic clients require a
		// tool-call id to correlate the result. Synthesize a deterministic one from
		// the call so the client echoes it back on the tool_result.
		return contract.ContentPart{
			Kind:              contract.ContentPartToolUse,
			ToolCallID:        "call_" + sha256HexPrefix([]byte(name+":"+arguments), 8),
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
	ThoughtsTokenCount      *int `json:"thoughtsTokenCount"`
	TotalTokenCount         *int `json:"totalTokenCount"`
	CachedContentTokenCount *int `json:"cachedContentTokenCount"`
}

func (u geminiUsageMetadata) ToUsage(text string) contract.Usage {
	cached := valueOrZero(u.CachedContentTokenCount)
	input := max(0, valueOrZero(u.PromptTokenCount)-cached)
	output := valueOrZero(u.CandidatesTokenCount) + valueOrZero(u.ThoughtsTokenCount)
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

func (u geminiUsageMetadata) ToImageUsage(req contract.ImageGenerationRequest) contract.Usage {
	usage := u.ToUsage(req.Prompt)
	if usage.Estimated {
		return estimatedImageUsage(req)
	}
	usage.ImageOutputTokens = usage.OutputTokens
	return usage
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
	if next.ThoughtsTokenCount != nil {
		u.ThoughtsTokenCount = cloneIntPtr(next.ThoughtsTokenCount)
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
		u.ThoughtsTokenCount != nil ||
		u.TotalTokenCount != nil ||
		u.CachedContentTokenCount != nil
}

// geminiStreamParseState drives a Gemini streamGenerateContent SSE stream,
// accumulating content parts and emitting canonical stream events per frame.
// The batch parser and the per-frame cross-protocol driver share this one state
// machine so they cannot drift.
type geminiStreamParseState struct {
	usage        geminiUsageMetadata
	parts        []contract.ContentPart
	streamEvents []contract.ConversationStreamEvent
	eventIndex   int
	stopReason   contract.StopReason
	seenChunk    bool
}

func newGeminiStreamParseState() *geminiStreamParseState {
	return &geminiStreamParseState{streamEvents: make([]contract.ConversationStreamEvent, 0), stopReason: contract.StopReasonEndTurn}
}

// FeedFrame processes one Gemini SSE frame and returns the canonical stream
// events it produced. Gemini carries its stop inline (the chunk's finishReason),
// so done is signalled only by a [DONE] sentinel.
func (s *geminiStreamParseState) FeedFrame(frame sseFrame) ([]contract.ConversationStreamEvent, bool, error) {
	data := strings.TrimSpace(frame.Data)
	if data == "" {
		return nil, false, nil
	}
	if data == "[DONE]" {
		return nil, true, nil
	}
	if providerErr, ok := providerErrorFromStreamFrame(frame, data, "gemini-compatible"); ok {
		return nil, false, providerErr
	}
	var chunk geminiGenerateContentResponse
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return nil, false, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid stream json"}
	}
	before := len(s.streamEvents)
	s.seenChunk = true
	chunkParts := chunk.ContentParts()
	s.parts = appendStreamContentParts(s.parts, chunkParts)
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
		s.streamEvents = append(s.streamEvents, contract.ConversationStreamEvent{
			Index:          s.eventIndex,
			Type:           eventType,
			ContentIndex:   contentIndex,
			Delta:          part,
			RawEventType:   "generateContentResponse",
			Raw:            append(json.RawMessage(nil), data...),
			OriginProtocol: "gemini-compatible",
		})
		s.eventIndex++
	}
	if reason := chunk.StopReason(); reason != contract.StopReasonEndTurn {
		s.stopReason = reason
		s.streamEvents = append(s.streamEvents, contract.ConversationStreamEvent{
			Index:          s.eventIndex,
			Type:           contract.ConversationStreamEventStop,
			StopReason:     s.stopReason,
			RawEventType:   "generateContentResponse",
			Raw:            append(json.RawMessage(nil), data...),
			OriginProtocol: "gemini-compatible",
		})
		s.eventIndex++
	}
	chunkUsage := chunk.Usage()
	s.usage.Merge(chunkUsage)
	if chunkUsage.HasTokenUsage() {
		s.streamEvents = append(s.streamEvents, contract.ConversationStreamEvent{
			Index:          s.eventIndex,
			Type:           contract.ConversationStreamEventUsage,
			Usage:          s.usage.ToUsage(contentPartsText(s.parts)),
			RawEventType:   "generateContentResponse",
			Raw:            append(json.RawMessage(nil), data...),
			OriginProtocol: "gemini-compatible",
		})
		s.eventIndex++
	}
	return cloneConversationStreamEventsTail(s.streamEvents, before), false, nil
}

// Finalize returns no trailing events — Gemini's stop is carried inline by the
// chunk that sets finishReason.
func (s *geminiStreamParseState) Finalize() []contract.ConversationStreamEvent { return nil }

func parseGeminiCompatibleStream(body []byte, statusCode int) (contract.ConversationResponse, error) {
	frames, err := parseSSEFrames(body)
	if err != nil {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
	}
	state := newGeminiStreamParseState()
	for _, frame := range frames {
		if _, done, feedErr := state.FeedFrame(frame); feedErr != nil {
			return contract.ConversationResponse{}, feedErr
		} else if done {
			break
		}
	}
	if !state.seenChunk {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream ended before chunk"}
	}
	if len(state.parts) == 0 {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider stream contained no content"}
	}
	text := contentPartsText(state.parts)
	return contract.ConversationResponse{
		Parts:        state.parts,
		StopReason:   state.stopReason,
		StatusCode:   statusCode,
		Usage:        state.usage.ToUsage(text),
		Raw:          append(json.RawMessage(nil), body...),
		StreamEvents: state.streamEvents,
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
	state := newAnthropicStreamParseState()
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
		if state.handleAnthropicStreamEvent(data, chunkType, chunk) {
			done = true
		}
	}
	if !done {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream ended before done"}
	}
	state.streamEvents = appendAnthropicTerminalStopEvent(state.streamEvents, state.eventIndex, state.stopReason)
	return anthropicStreamResponse(body, statusCode, state.blocks, state.order, state.stopReason, state.usage, state.streamEvents)
}

type anthropicStreamParseState struct {
	builder      strings.Builder
	usage        anthropicUsage
	blocks       map[int]*anthropicContentBlock
	order        []int
	lastIndex    int
	streamEvents []contract.ConversationStreamEvent
	eventIndex   int
	stopReason   contract.StopReason
}

func newAnthropicStreamParseState() *anthropicStreamParseState {
	return &anthropicStreamParseState{
		blocks:       map[int]*anthropicContentBlock{},
		order:        []int{},
		lastIndex:    -1,
		streamEvents: make([]contract.ConversationStreamEvent, 0),
		stopReason:   contract.StopReasonEndTurn,
	}
}

func (s *anthropicStreamParseState) handleAnthropicStreamEvent(data string, chunkType string, chunk anthropicStreamChunk) bool {
	switch chunkType {
	case "content_block_start":
		s.handleAnthropicContentBlockStart(data, chunkType, chunk)
	case "content_block_delta":
		s.handleAnthropicContentBlockDelta(data, chunkType, chunk)
	case "message_start":
		s.handleAnthropicMessageStart(data, chunkType, chunk)
	case "message_delta":
		s.handleAnthropicMessageDelta(data, chunkType, chunk)
	case "message_stop":
		return true
	}
	return false
}

func (s *anthropicStreamParseState) handleAnthropicContentBlockStart(data string, chunkType string, chunk anthropicStreamChunk) {
	index := anthropicStreamIndex(chunk.Index, &s.lastIndex, len(s.order))
	block := anthropicStreamBlockState(s.blocks, &s.order, index)
	if chunk.ContentBlock == nil {
		return
	}
	*block = *chunk.ContentBlock
	if strings.TrimSpace(block.Type) == "" {
		block.Type = "text"
	}
	if strings.EqualFold(strings.TrimSpace(block.Type), "tool_use") || strings.EqualFold(strings.TrimSpace(block.Type), "server_tool_use") {
		s.streamEvents = append(s.streamEvents, contract.ConversationStreamEvent{
			Index:        s.eventIndex,
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
		s.eventIndex++
	}
	if strings.TrimSpace(chunk.ContentBlock.Text) == "" {
		return
	}
	s.builder.WriteString(chunk.ContentBlock.Text)
	s.streamEvents = append(s.streamEvents, contract.ConversationStreamEvent{
		Index:          s.eventIndex,
		Type:           contract.ConversationStreamEventContentDelta,
		ContentIndex:   index,
		Delta:          textContentDelta(chunk.ContentBlock.Text),
		RawEventType:   chunkType,
		Raw:            append(json.RawMessage(nil), data...),
		OriginProtocol: "anthropic-compatible",
	})
	s.eventIndex++
}

func (s *anthropicStreamParseState) handleAnthropicContentBlockDelta(data string, chunkType string, chunk anthropicStreamChunk) {
	index := anthropicStreamIndex(chunk.Index, &s.lastIndex, len(s.order))
	block := anthropicStreamBlockState(s.blocks, &s.order, index)
	switch strings.TrimSpace(chunk.Delta.Type) {
	case "input_json_delta":
		s.handleAnthropicInputJSONDelta(data, chunkType, index, block, chunk)
	case "thinking_delta":
		s.handleAnthropicThinkingDelta(data, chunkType, index, block, chunk)
	case "signature_delta":
		s.handleAnthropicSignatureDelta(data, chunkType, index, block, chunk)
	default:
		s.handleAnthropicTextDelta(data, chunkType, index, block, chunk)
	}
}

func (s *anthropicStreamParseState) handleAnthropicInputJSONDelta(data string, chunkType string, index int, block *anthropicContentBlock, chunk anthropicStreamChunk) {
	block.Type = "tool_use"
	if strings.TrimSpace(string(block.Input)) == "{}" {
		block.Input = nil
	}
	block.Input = append(block.Input, chunk.Delta.PartialJSON...)
	s.streamEvents = append(s.streamEvents, contract.ConversationStreamEvent{
		Index:        s.eventIndex,
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
	s.eventIndex++
}

func (s *anthropicStreamParseState) handleAnthropicThinkingDelta(data string, chunkType string, index int, block *anthropicContentBlock, chunk anthropicStreamChunk) {
	block.Type = "thinking"
	text := chunk.Delta.Thinking
	if text == "" {
		text = chunk.Delta.Text
	}
	block.Text += text
	if text == "" {
		return
	}
	s.streamEvents = append(s.streamEvents, contract.ConversationStreamEvent{
		Index:          s.eventIndex,
		Type:           contract.ConversationStreamEventReasoning,
		ContentIndex:   index,
		Delta:          contract.ContentPart{Kind: contract.ContentPartThinking, Text: text, OriginProtocol: "anthropic-compatible"},
		RawEventType:   chunkType,
		Raw:            append(json.RawMessage(nil), data...),
		OriginProtocol: "anthropic-compatible",
	})
	s.eventIndex++
}

func (s *anthropicStreamParseState) handleAnthropicSignatureDelta(data string, chunkType string, index int, block *anthropicContentBlock, chunk anthropicStreamChunk) {
	block.Type = "thinking"
	block.Signature += chunk.Delta.Signature
	if chunk.Delta.Signature == "" {
		return
	}
	s.streamEvents = append(s.streamEvents, contract.ConversationStreamEvent{
		Index:        s.eventIndex,
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
	s.eventIndex++
}

func (s *anthropicStreamParseState) handleAnthropicTextDelta(data string, chunkType string, index int, block *anthropicContentBlock, chunk anthropicStreamChunk) {
	if strings.TrimSpace(block.Type) == "" {
		block.Type = "text"
	}
	block.Text += chunk.Delta.Text
	s.builder.WriteString(chunk.Delta.Text)
	if chunk.Delta.Text == "" {
		return
	}
	s.streamEvents = append(s.streamEvents, contract.ConversationStreamEvent{
		Index:          s.eventIndex,
		Type:           contract.ConversationStreamEventContentDelta,
		ContentIndex:   index,
		Delta:          textContentDelta(chunk.Delta.Text),
		RawEventType:   chunkType,
		Raw:            append(json.RawMessage(nil), data...),
		OriginProtocol: "anthropic-compatible",
	})
	s.eventIndex++
}

func (s *anthropicStreamParseState) handleAnthropicMessageStart(data string, chunkType string, chunk anthropicStreamChunk) {
	if chunk.Message == nil {
		return
	}
	s.usage.Merge(chunk.Message.Usage)
	s.streamEvents = append(s.streamEvents, contract.ConversationStreamEvent{
		Index:          s.eventIndex,
		Type:           contract.ConversationStreamEventUsage,
		Usage:          chunk.Message.Usage.ToUsage(s.builder.String()),
		RawEventType:   chunkType,
		Raw:            append(json.RawMessage(nil), data...),
		OriginProtocol: "anthropic-compatible",
	})
	s.eventIndex++
}

func (s *anthropicStreamParseState) handleAnthropicMessageDelta(data string, chunkType string, chunk anthropicStreamChunk) {
	if chunk.Delta.StopReason != "" {
		s.stopReason = anthropicStopReason(chunk.Delta.StopReason)
		s.streamEvents = append(s.streamEvents, contract.ConversationStreamEvent{
			Index:          s.eventIndex,
			Type:           contract.ConversationStreamEventStop,
			StopReason:     s.stopReason,
			RawEventType:   chunkType,
			Raw:            append(json.RawMessage(nil), data...),
			OriginProtocol: "anthropic-compatible",
		})
		s.eventIndex++
	}
	if chunk.Usage == nil {
		return
	}
	s.usage.Merge(*chunk.Usage)
	s.streamEvents = append(s.streamEvents, contract.ConversationStreamEvent{
		Index:          s.eventIndex,
		Type:           contract.ConversationStreamEventUsage,
		Usage:          s.usage.ToUsage(s.builder.String()),
		RawEventType:   chunkType,
		Raw:            append(json.RawMessage(nil), data...),
		OriginProtocol: "anthropic-compatible",
	})
	s.eventIndex++
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
