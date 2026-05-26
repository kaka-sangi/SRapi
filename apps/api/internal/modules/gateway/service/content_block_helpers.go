package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func openAIContentBlock(block apiopenapi.ContentBlock, role string) (gatewaycontract.ContentBlock, bool) {
	base := sourceContentBlockBase(role, gatewaycontract.ProtocolOpenAICompatible, block)
	switch block.Type {
	case apiopenapi.ContentBlockTypeImageUrl, apiopenapi.ContentBlockTypeInputImage:
		return openAIImageContentBlock(block, base)
	case apiopenapi.ContentBlockTypeToolCall:
		return openAIToolCallContentBlock(block, base)
	case apiopenapi.ContentBlockTypeToolResult:
		return openAIToolResultContentBlock(block, base)
	default:
		if block.ImageUrl != nil {
			return openAIImageContentBlock(block, base)
		}
		if block.Text == nil {
			return gatewaycontract.ContentBlock{}, false
		}
		text := strings.TrimSpace(*block.Text)
		if text == "" {
			return gatewaycontract.ContentBlock{}, false
		}
		base.Type = gatewaycontract.ContentBlockText
		base.Text = text
		return base, true
	}
}

func openAIImageContentBlock(block apiopenapi.ContentBlock, base gatewaycontract.ContentBlock) (gatewaycontract.ContentBlock, bool) {
	props := mergedJSONProperties(jsonObjectToMap(block.ImageUrl), block.AdditionalProperties)
	url := firstNonEmpty(mapStringAny(props, "url"), mapStringAny(props, "image_url"), mapStringAny(props, "media_url"))
	base64Data, mimeType := splitDataURL(url)
	if base64Data != "" {
		url = ""
	}
	if base64Data == "" {
		base64Data = firstNonEmpty(mapStringAny(props, "data"), mapStringAny(props, "media_base64"))
	}
	mimeType = firstNonEmpty(mimeType, mapStringAny(props, "mime_type"), mapStringAny(props, "media_type"))
	fileID := firstNonEmpty(mapStringAny(props, "file_id"), mapStringAny(props, "fileId"))
	if url == "" && base64Data == "" && fileID == "" {
		return gatewaycontract.ContentBlock{}, false
	}
	base.Type = gatewaycontract.ContentBlockImage
	base.Text = "[image]"
	base.MediaURL = url
	base.MediaBase64 = base64Data
	base.MIMEType = mimeType
	base.FileID = fileID
	base.Metadata = props
	return base, true
}

func openAIToolCallContentBlock(block apiopenapi.ContentBlock, base gatewaycontract.ContentBlock) (gatewaycontract.ContentBlock, bool) {
	props := cloneMap(block.AdditionalProperties)
	id := firstNonEmpty(mapStringAny(props, "id"), mapStringAny(props, "call_id"))
	name := mapStringAny(props, "name")
	arguments := mapStringAny(props, "arguments")
	if function, ok := props["function"].(map[string]any); ok {
		name = firstNonEmpty(name, mapStringAny(function, "name"))
		arguments = firstNonEmpty(arguments, jsonStringOrMarshal(function["arguments"]))
	}
	if id == "" && name == "" && arguments == "" {
		return gatewaycontract.ContentBlock{}, false
	}
	base.Type = gatewaycontract.ContentBlockToolCall
	base.Text = "[function_call]"
	base.ToolCallID = id
	base.ToolName = name
	base.ToolArgumentsJSON = arguments
	base.Metadata = props
	return base, true
}

func openAIToolResultContentBlock(block apiopenapi.ContentBlock, base gatewaycontract.ContentBlock) (gatewaycontract.ContentBlock, bool) {
	props := cloneMap(block.AdditionalProperties)
	text := ""
	if block.Text != nil {
		text = strings.TrimSpace(*block.Text)
	}
	if text == "" {
		text = jsonStringOrMarshal(props["output"])
	}
	id := firstNonEmpty(mapStringAny(props, "tool_call_id"), mapStringAny(props, "call_id"), mapStringAny(props, "id"))
	if text == "" && id == "" {
		return gatewaycontract.ContentBlock{}, false
	}
	base.Type = gatewaycontract.ContentBlockToolResult
	base.Text = text
	base.ToolCallID = id
	base.ToolResultForID = id
	base.ToolName = mapStringAny(props, "name")
	base.ToolResultIsError = mapBoolAny(props, "is_error")
	base.Metadata = props
	return base, true
}

func chatToolCallBlocks(toolCalls []apiopenapi.ChatToolCall) []gatewaycontract.ContentBlock {
	out := make([]gatewaycontract.ContentBlock, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		arguments := jsonStringOrMarshal(toolCall.Function["arguments"])
		if arguments == "" {
			arguments = "{}"
		}
		block := sourceContentBlockBase("assistant", gatewaycontract.ProtocolOpenAICompatible, toolCall)
		block.Type = gatewaycontract.ContentBlockToolCall
		block.Text = "[function_call]"
		block.ToolCallID = strings.TrimSpace(toolCall.Id)
		block.ToolName = mapStringAny(toolCall.Function, "name")
		block.ToolArgumentsJSON = arguments
		block.Metadata = cloneMap(toolCall.AdditionalProperties)
		if callType := strings.TrimSpace(toolCall.Type); callType != "" {
			if block.Metadata == nil {
				block.Metadata = map[string]any{}
			}
			block.Metadata["type"] = callType
		}
		if block.ToolCallID != "" || block.ToolName != "" || block.ToolArgumentsJSON != "" {
			out = append(out, block)
		}
	}
	return out
}

func chatToolResultBlocks(blocks []gatewaycontract.ContentBlock, toolCallID *string) []gatewaycontract.ContentBlock {
	if len(blocks) == 0 {
		return nil
	}
	callID := ""
	if toolCallID != nil {
		callID = strings.TrimSpace(*toolCallID)
	}
	out := make([]gatewaycontract.ContentBlock, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == gatewaycontract.ContentBlockText {
			block.Type = gatewaycontract.ContentBlockToolResult
			block.ToolCallID = firstNonEmpty(block.ToolCallID, callID)
			block.ToolResultForID = firstNonEmpty(block.ToolResultForID, callID)
		}
		out = append(out, block)
	}
	return out
}

func anthropicContentBlock(block apiopenapi.AnthropicContentBlock, role string) (gatewaycontract.ContentBlock, bool) {
	base := sourceContentBlockBase(role, gatewaycontract.ProtocolAnthropicCompatible, block)
	props := cloneMap(block.AdditionalProperties)
	switch block.Type {
	case apiopenapi.AnthropicContentBlockTypeImage:
		source, _ := props["source"].(map[string]any)
		url := firstNonEmpty(mapStringAny(source, "url"), mapStringAny(props, "url"))
		base64Data := firstNonEmpty(mapStringAny(source, "data"), mapStringAny(props, "data"))
		mimeType := firstNonEmpty(mapStringAny(source, "media_type"), mapStringAny(props, "media_type"))
		if url == "" && base64Data == "" {
			return gatewaycontract.ContentBlock{}, false
		}
		base.Type = gatewaycontract.ContentBlockImage
		base.Text = "[image]"
		base.MediaURL = url
		base.MediaBase64 = base64Data
		base.MIMEType = mimeType
		base.Metadata = props
		return base, true
	case apiopenapi.AnthropicContentBlockTypeToolUse:
		base.Type = gatewaycontract.ContentBlockToolCall
		base.Text = "[tool_use]"
		base.ToolCallID = mapStringAny(props, "id")
		base.ToolName = mapStringAny(props, "name")
		base.ToolArgumentsJSON = jsonStringOrMarshal(props["input"])
		base.Metadata = props
		return base, base.ToolCallID != "" || base.ToolName != "" || base.ToolArgumentsJSON != ""
	case apiopenapi.AnthropicContentBlockTypeToolResult:
		base.Type = gatewaycontract.ContentBlockToolResult
		base.ToolCallID = mapStringAny(props, "tool_use_id")
		base.ToolResultForID = base.ToolCallID
		base.ToolResultIsError = mapBoolAny(props, "is_error")
		base.Text = anthropicToolResultText(block, props)
		base.Metadata = props
		return base, base.ToolCallID != "" || base.Text != ""
	default:
		if block.Text == nil {
			return gatewaycontract.ContentBlock{}, false
		}
		text := strings.TrimSpace(*block.Text)
		if text == "" {
			return gatewaycontract.ContentBlock{}, false
		}
		base.Type = gatewaycontract.ContentBlockText
		base.Text = text
		return base, true
	}
}

func anthropicToolResultText(block apiopenapi.AnthropicContentBlock, props map[string]any) string {
	if block.Text != nil {
		return strings.TrimSpace(*block.Text)
	}
	if text := mapStringAny(props, "content"); text != "" {
		return text
	}
	return jsonStringOrMarshal(props["content"])
}

func geminiContentBlock(part apiopenapi.GeminiPart, role string) (gatewaycontract.ContentBlock, bool) {
	base := sourceContentBlockBase(role, gatewaycontract.ProtocolGeminiCompatible, part)
	if part.Text != nil {
		text := strings.TrimSpace(*part.Text)
		if text == "" {
			return gatewaycontract.ContentBlock{}, false
		}
		base.Type = gatewaycontract.ContentBlockText
		base.Text = text
		return base, true
	}
	if part.InlineData != nil {
		values := jsonObjectToMap(part.InlineData)
		base.Type = gatewaycontract.ContentBlockImage
		base.Text = "[image]"
		base.MediaBase64 = mapStringAny(values, "data")
		base.MIMEType = firstNonEmpty(mapStringAny(values, "mimeType"), mapStringAny(values, "mime_type"))
		base.Metadata = map[string]any{"inline_data": values}
		return base, base.MediaBase64 != ""
	}
	if part.FileData != nil {
		values := jsonObjectToMap(part.FileData)
		base.Type = gatewaycontract.ContentBlockImage
		base.Text = "[image]"
		base.MediaURL = firstNonEmpty(mapStringAny(values, "fileUri"), mapStringAny(values, "file_uri"))
		base.MIMEType = firstNonEmpty(mapStringAny(values, "mimeType"), mapStringAny(values, "mime_type"))
		base.Metadata = map[string]any{"file_data": values}
		return base, base.MediaURL != ""
	}
	if part.FunctionCall != nil {
		values := jsonObjectToMap(part.FunctionCall)
		base.Type = gatewaycontract.ContentBlockToolCall
		base.Text = "[function_call]"
		base.ToolName = mapStringAny(values, "name")
		base.ToolArgumentsJSON = jsonStringOrMarshal(values["args"])
		base.Metadata = values
		return base, base.ToolName != "" || base.ToolArgumentsJSON != ""
	}
	if part.FunctionResponse != nil {
		values := jsonObjectToMap(part.FunctionResponse)
		base.Type = gatewaycontract.ContentBlockToolResult
		base.Text = jsonStringOrMarshal(values["response"])
		base.ToolName = mapStringAny(values, "name")
		base.Metadata = values
		return base, base.ToolName != "" || base.Text != ""
	}
	return gatewaycontract.ContentBlock{}, false
}

func sourceContentBlockBase(role string, origin gatewaycontract.Protocol, raw any) gatewaycontract.ContentBlock {
	return gatewaycontract.ContentBlock{
		Role:           strings.TrimSpace(role),
		OriginProtocol: string(origin),
		Raw:            marshalRawJSON(raw),
	}
}

func mergedJSONProperties(primary map[string]any, fallback map[string]any) map[string]any {
	out := cloneMap(primary)
	if out == nil {
		out = map[string]any{}
	}
	for key, value := range fallback {
		if _, exists := out[key]; !exists {
			out[key] = cloneAny(value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mapStringAny(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func mapBoolAny(values map[string]any, key string) bool {
	value, ok := values[key]
	if !ok || value == nil {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func jsonStringOrMarshal(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}

func marshalRawJSON(value any) json.RawMessage {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil || !json.Valid(raw) {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func splitDataURL(value string) (string, string) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "data:") {
		return "", ""
	}
	header, data, ok := strings.Cut(strings.TrimPrefix(value, "data:"), ",")
	if !ok || strings.TrimSpace(data) == "" {
		return "", ""
	}
	if _, err := base64.StdEncoding.DecodeString(data); err != nil {
		return "", ""
	}
	mimeType := strings.TrimSpace(strings.TrimSuffix(header, ";base64"))
	if mimeType == header {
		mimeType = ""
	}
	return data, mimeType
}
