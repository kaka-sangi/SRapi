package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/translator"
	_ "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/translator/translators"
)

type geminiGenerateContentRequest struct {
	Contents          []geminiContent         `json:"contents"`
	SystemInstruction *geminiContent          `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
	Tools             []map[string]any        `json:"tools,omitempty"`
	ToolConfig        any                     `json:"toolConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string         `json:"text,omitempty"`
	Thought          bool           `json:"thought,omitempty"`
	InlineData       map[string]any `json:"inlineData,omitempty"`
	InlineDataSnake  map[string]any `json:"inline_data,omitempty"`
	FileData         map[string]any `json:"fileData,omitempty"`
	FunctionCall     map[string]any `json:"functionCall,omitempty"`
	FunctionResponse map[string]any `json:"functionResponse,omitempty"`
	ThoughtSignature string         `json:"thoughtSignature,omitempty"`
}

const geminiDummyThoughtSignature = "skip_thought_signature_validator"

type geminiGenerationConfig struct {
	Temperature      *float32       `json:"temperature,omitempty"`
	TopP             *float32       `json:"topP,omitempty"`
	MaxOutputTokens  *int           `json:"maxOutputTokens,omitempty"`
	StopSequences    []string       `json:"stopSequences,omitempty"`
	ResponseMimeType string         `json:"responseMimeType,omitempty"`
	ResponseSchema   map[string]any `json:"responseSchema,omitempty"`
	ThinkingConfig   map[string]any `json:"thinkingConfig,omitempty"`
}

func geminiCompatiblePayload(req contract.ConversationRequest) geminiGenerateContentRequest {
	payload := geminiGenerateContentRequest{
		Contents:         geminiCompatibleContents(req),
		GenerationConfig: geminiCompatibleGenerationConfig(req),
		Tools:            geminiCompatibleTools(req.Tools),
		ToolConfig:       geminiCompatibleToolConfig(req.ToolChoice),
	}
	if system := geminiCompatibleSystem(req); system != "" {
		payload.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: system}},
		}
	}
	return payload
}

// geminiCompatibleRequestBody builds the gemini /v1beta:generateContent
// outbound payload. The registered translator for the
// gemini_request → openai_responses pair is an identity (see
// translator/translators/gemini_request_to_openai_responses.go for the
// rationale — srapi's gemini handling is upstream-native, no
// cross-format rewriting is required at the request side). Consulting
// the registry here keeps the call-site on the hot path so future
// per-pair transforms become a one-file translator edit instead of a
// service/ edit.
func geminiCompatibleRequestBody(req contract.ConversationRequest) ([]byte, error) {
	raw, err := geminiCompatibleRequestBodyRaw(req)
	if err != nil {
		return nil, err
	}
	raw = translator.Default().TranslateRequest(
		translator.FormatGeminiRequest,
		translator.FormatOpenAIResponses,
		req.Mapping.UpstreamModelName,
		raw,
		req.Stream,
	)
	return applyPayloadTransforms(raw, req.PayloadTransforms)
}

func geminiCompatibleRequestBodyRaw(req contract.ConversationRequest) ([]byte, error) {
	if payload, ok, err := rawSameProtocolPayload(req, rawEndpointGeminiGenerateContent); ok || err != nil {
		if err != nil {
			return nil, err
		}
		return json.Marshal(payload)
	}
	return json.Marshal(geminiCompatiblePayload(req))
}

func geminiCompatibleContents(req contract.ConversationRequest) []geminiContent {
	out := make([]geminiContent, 0, len(req.Messages)+1)
	// Gemini's functionResponse must carry the function name that matches the
	// earlier functionCall, but an inbound tool_result references the call by id,
	// not name. Build a callID->name map across the whole history so a tool_result
	// with no name can recover it.
	toolNames := geminiToolNameByCallID(req)
	hasConversationMessage := false
	for _, message := range req.Messages {
		role := geminiRole(message.Role)
		if role == "system" {
			continue
		}
		parts := geminiPartsFromContentParts(message.Parts, toolNames)
		if len(parts) == 0 {
			continue
		}
		out = append(out, geminiContent{Role: role, Parts: parts})
		hasConversationMessage = true
	}
	if !hasConversationMessage {
		parts := geminiPartsFromContentParts(req.InputParts, toolNames)
		if len(parts) == 0 {
			prompt := conversationPrompt(req)
			if prompt == "" {
				prompt = strings.TrimSpace(req.Instructions)
			}
			parts = []geminiPart{{Text: prompt}}
		}
		out = append(out, geminiContent{Role: "user", Parts: parts})
	}
	return out
}

func geminiRole(role string) string {
	switch strings.TrimSpace(role) {
	case "assistant", "model":
		return "model"
	case "system":
		return "system"
	default:
		return "user"
	}
}

// geminiToolNameByCallID maps each tool_use call id to its function name across
// the whole request so a later tool_result (which references the call by id, not
// name) can recover the name Gemini's functionResponse requires.
func geminiToolNameByCallID(req contract.ConversationRequest) map[string]string {
	names := map[string]string{}
	collect := func(parts []contract.ContentPart) {
		for _, part := range parts {
			if part.Kind != contract.ContentPartToolUse {
				continue
			}
			id := strings.TrimSpace(part.ToolCallID)
			name := strings.TrimSpace(part.ToolName)
			if id != "" && name != "" {
				names[id] = name
			}
		}
	}
	for _, message := range req.Messages {
		collect(message.Parts)
	}
	collect(req.InputParts)
	return names
}

func geminiPartsFromContentParts(parts []contract.ContentPart, toolNames map[string]string) []geminiPart {
	out := make([]geminiPart, 0, len(parts))
	for _, part := range parts {
		switch part.Kind {
		case contract.ContentPartImage, contract.ContentPartAudio, contract.ContentPartFile:
			if value, ok := geminiMediaPart(part); ok {
				out = append(out, value)
				continue
			}
			if text := strings.TrimSpace(part.Text); text != "" {
				out = append(out, geminiPart{Text: text})
			}
		case contract.ContentPartToolUse:
			call := map[string]any{}
			if name := strings.TrimSpace(part.ToolName); name != "" {
				call["name"] = name
			}
			if args := jsonObjectValue(part.ToolArgumentsJSON); args != nil {
				call["args"] = args
			} else {
				call["args"] = map[string]any{}
			}
			if len(call) > 0 {
				out = append(out, geminiPart{FunctionCall: call, ThoughtSignature: geminiThoughtSignature(part, true)})
			}
		case contract.ContentPartToolResult:
			response := map[string]any{}
			name := strings.TrimSpace(part.ToolName)
			if name == "" {
				name = toolNames[firstNonEmpty(part.ToolResultForID, part.ToolCallID)]
			}
			if name != "" {
				response["name"] = name
			}
			if payload := jsonObjectValue(part.Text); payload != nil {
				response["response"] = payload
			} else if text := strings.TrimSpace(part.Text); text != "" {
				response["response"] = map[string]any{"text": text}
			} else {
				response["response"] = map[string]any{}
			}
			if len(response) > 0 {
				out = append(out, geminiPart{FunctionResponse: response})
			}
		case contract.ContentPartThinking:
			// Claude/Anthropic thinking forwarded to a Gemini upstream must be a
			// reasoning part (thought:true) carrying the thoughtSignature (a dummy
			// placeholder when none is available), not visible text — matching
			// sub2api's antigravity request_transformer "thinking" case.
			if text := strings.TrimSpace(part.Text); text != "" {
				out = append(out, geminiPart{
					Text:             part.Text,
					Thought:          true,
					ThoughtSignature: geminiThoughtSignature(part, true),
				})
			}
		default:
			if text := strings.TrimSpace(part.Text); text != "" {
				out = append(out, geminiPart{Text: text, ThoughtSignature: geminiThoughtSignature(part, false)})
			}
		}
	}
	return out
}

func geminiThoughtSignature(part contract.ContentPart, useDummyForFunctionCall bool) string {
	// Pass the upstream-origin signature through as the Gemini thoughtSignature,
	// including an Anthropic thinking block's "signature" (sub2api forwards it
	// directly rather than gating it out by origin).
	signature := metadataString(part.Metadata, "thoughtSignature")
	if signature == "" {
		signature = metadataString(part.Metadata, "thought_signature")
	}
	if signature == "" {
		signature = metadataString(part.Metadata, "signature")
	}
	if signature == "" && useDummyForFunctionCall {
		return geminiDummyThoughtSignature
	}
	return signature
}

func geminiMediaPart(part contract.ContentPart) (geminiPart, bool) {
	mimeType := strings.TrimSpace(part.MIMEType)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	if data := strings.TrimSpace(part.MediaBase64); data != "" {
		return geminiPart{InlineData: map[string]any{
			"mimeType": mimeType,
			"data":     data,
		}}, true
	}
	if uri := strings.TrimSpace(part.MediaURL); uri != "" {
		return geminiPart{FileData: map[string]any{
			"mimeType": mimeType,
			"fileUri":  uri,
		}}, true
	}
	if fileID := strings.TrimSpace(part.FileID); fileID != "" {
		return geminiPart{FileData: map[string]any{
			"mimeType": mimeType,
			"fileUri":  fileID,
		}}, true
	}
	return geminiPart{}, false
}

func geminiCompatibleSystem(req contract.ConversationRequest) string {
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

func geminiCompatibleGenerationConfig(req contract.ConversationRequest) *geminiGenerationConfig {
	cfg := &geminiGenerationConfig{
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		MaxOutputTokens: req.MaxOutputTokens,
		StopSequences:   cloneStrings(req.Stop),
	}
	if len(req.ResponseFormat) > 0 {
		cfg.ResponseMimeType = geminiResponseMimeType(req.ResponseFormat)
		cfg.ResponseSchema = geminiResponseSchema(req.ResponseFormat)
	}
	cfg.ThinkingConfig = geminiThinkingConfig(req.Reasoning)
	if cfg.Temperature == nil && cfg.TopP == nil && cfg.MaxOutputTokens == nil && len(cfg.StopSequences) == 0 && cfg.ResponseMimeType == "" && len(cfg.ResponseSchema) == 0 && len(cfg.ThinkingConfig) == 0 {
		return nil
	}
	return cfg
}

func geminiThinkingConfig(reasoning map[string]any) map[string]any {
	if len(reasoning) == 0 {
		return nil
	}
	if budget, ok := geminiThinkingBudget(reasoning); ok {
		return map[string]any{
			"thinkingBudget":  budget,
			"includeThoughts": budget != 0,
		}
	}
	effort := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		metadataString(reasoning, "effort"),
		metadataString(reasoning, "level"),
	)))
	if effort == "" {
		return nil
	}
	if effort == "none" {
		return map[string]any{
			"thinkingBudget":  0,
			"includeThoughts": false,
		}
	}
	if budget, ok := geminiThinkingBudgetForEffort(effort); ok {
		return map[string]any{
			"thinkingBudget":  budget,
			"includeThoughts": true,
		}
	}
	return nil
}

func geminiThinkingBudget(reasoning map[string]any) (int, bool) {
	for _, key := range []string{"budget_tokens", "thinking_budget", "thinkingBudget", "budget"} {
		if budget, ok := intFromAny(reasoning[key]); ok && budget >= -1 {
			return budget, true
		}
	}
	return 0, false
}

func geminiThinkingBudgetForEffort(effort string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(effort)) {
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

func intFromAny(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int8:
		return int(typed), true
	case int16:
		return int(typed), true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float32:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed), true
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func geminiResponseMimeType(format map[string]any) string {
	for _, key := range []string{"responseMimeType", "response_mime_type", "mime_type", "type"} {
		value := strings.TrimSpace(fmt.Sprint(format[key]))
		if value != "" && value != "<nil>" {
			if value == "json_object" || value == "json_schema" {
				return "application/json"
			}
			return value
		}
	}
	if len(format) > 0 {
		return "application/json"
	}
	return ""
}

func geminiResponseSchema(format map[string]any) map[string]any {
	for _, key := range []string{"responseSchema", "response_schema", "schema"} {
		if value, ok := format[key].(map[string]any); ok {
			return cloneMap(value)
		}
	}
	if value, ok := format["json_schema"].(map[string]any); ok {
		if schema, ok := value["schema"].(map[string]any); ok {
			return cloneMap(schema)
		}
		return cloneMap(value)
	}
	return nil
}

func geminiCompatibleTools(values []map[string]any) []map[string]any {
	if len(values) == 0 {
		return nil
	}
	functionDeclarations := make([]map[string]any, 0, len(values))
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		if declaration := geminiFunctionDeclaration(value); len(declaration) > 0 {
			functionDeclarations = append(functionDeclarations, declaration)
			continue
		}
		out = append(out, cloneMap(value))
	}
	if len(functionDeclarations) > 0 {
		out = append(out, map[string]any{"functionDeclarations": functionDeclarations})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func geminiFunctionDeclaration(value map[string]any) map[string]any {
	function, ok := value["function"].(map[string]any)
	if !ok {
		return nil
	}
	declaration := map[string]any{}
	if name := strings.TrimSpace(fmt.Sprint(function["name"])); name != "" && name != "<nil>" {
		declaration["name"] = name
	}
	if description := strings.TrimSpace(fmt.Sprint(function["description"])); description != "" && description != "<nil>" {
		declaration["description"] = description
	}
	if parameters, ok := function["parameters"]; ok && parameters != nil {
		declaration["parameters"] = cloneAny(parameters)
	}
	return declaration
}

func geminiCompatibleToolConfig(value any) any {
	if value == nil {
		return nil
	}
	return cloneAny(value)
}
