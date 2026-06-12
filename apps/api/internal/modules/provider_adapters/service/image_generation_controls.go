package service

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

type disableImageGenerationMode int

const (
	disableImageGenerationOff disableImageGenerationMode = iota
	disableImageGenerationAll
	disableImageGenerationChat
)

func applyDisableImageGenerationToResponsesPayload(req contract.ConversationRequest, payload map[string]any) {
	if payload == nil || !imageGenerationDisabledForConversation(req) {
		return
	}
	removeResponsesImageGenerationTool(payload)
	removeResponsesImageGenerationToolChoice(payload)
}

func imageGenerationDisabledForConversation(req contract.ConversationRequest) bool {
	mode := disableImageGenerationModeForConversation(req)
	switch mode {
	case disableImageGenerationAll:
		return true
	case disableImageGenerationChat:
		return !sourceEndpointIsImages(req.SourceEndpoint)
	default:
		return false
	}
}

func imageGenerationDisabledForImages(req contract.ImageGenerationRequest) bool {
	return disableImageGenerationModeForImageGeneration(req) == disableImageGenerationAll
}

func imageGenerationDisabledForImageEdit(req contract.ImageEditRequest) bool {
	return disableImageGenerationModeForImageEdit(req) == disableImageGenerationAll
}

func imageGenerationDisabledForImageVariation(req contract.ImageVariationRequest) bool {
	return disableImageGenerationModeForImageVariation(req) == disableImageGenerationAll
}

func disableImageGenerationModeForConversation(req contract.ConversationRequest) disableImageGenerationMode {
	return disableImageGenerationModeFromMaps(
		req.RequestSettings,
		req.Credential,
		req.Account.Metadata,
		req.Provider.ConfigSchema,
		req.Provider.Capabilities,
	)
}

func disableImageGenerationModeForImageGeneration(req contract.ImageGenerationRequest) disableImageGenerationMode {
	return disableImageGenerationModeFromMaps(
		req.Credential,
		req.Account.Metadata,
		req.Provider.ConfigSchema,
		req.Provider.Capabilities,
	)
}

func disableImageGenerationModeForImageEdit(req contract.ImageEditRequest) disableImageGenerationMode {
	return disableImageGenerationModeFromMaps(
		req.Credential,
		req.Account.Metadata,
		req.Provider.ConfigSchema,
		req.Provider.Capabilities,
	)
}

func disableImageGenerationModeForImageVariation(req contract.ImageVariationRequest) disableImageGenerationMode {
	return disableImageGenerationModeFromMaps(
		req.Credential,
		req.Account.Metadata,
		req.Provider.ConfigSchema,
		req.Provider.Capabilities,
	)
}

func disableImageGenerationModeFromMaps(values ...map[string]any) disableImageGenerationMode {
	for _, items := range values {
		if items == nil {
			continue
		}
		for _, key := range disableImageGenerationKeys() {
			raw, ok := items[key]
			if !ok {
				continue
			}
			mode, ok := parseDisableImageGenerationMode(raw)
			if ok {
				return mode
			}
		}
	}
	return disableImageGenerationOff
}

func disableImageGenerationKeys() []string {
	return []string{
		"disable_image_generation",
		"disable-image-generation",
	}
}

func parseDisableImageGenerationMode(value any) (disableImageGenerationMode, bool) {
	switch typed := value.(type) {
	case nil:
		return disableImageGenerationOff, true
	case bool:
		if typed {
			return disableImageGenerationAll, true
		}
		return disableImageGenerationOff, true
	case string:
		return parseDisableImageGenerationString(typed)
	case json.Number:
		return parseDisableImageGenerationString(typed.String())
	case int:
		if typed != 0 {
			return disableImageGenerationAll, true
		}
		return disableImageGenerationOff, true
	case int64:
		if typed != 0 {
			return disableImageGenerationAll, true
		}
		return disableImageGenerationOff, true
	case float64:
		if typed != 0 {
			return disableImageGenerationAll, true
		}
		return disableImageGenerationOff, true
	default:
		return parseDisableImageGenerationString(codexStringValue(value))
	}
}

func parseDisableImageGenerationString(value string) (disableImageGenerationMode, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "false", "0", "off", "no":
		return disableImageGenerationOff, true
	case "true", "1", "on", "yes":
		return disableImageGenerationAll, true
	case "chat":
		return disableImageGenerationChat, true
	default:
		return disableImageGenerationOff, false
	}
}

func sourceEndpointIsImages(endpoint string) bool {
	normalized := strings.ToLower(strings.TrimSpace(endpoint))
	if normalized == "" {
		return false
	}
	return strings.HasSuffix(normalized, "/images/generations") ||
		strings.HasSuffix(normalized, "/images/edits") ||
		strings.HasSuffix(normalized, "/images/variations")
}

func removeResponsesImageGenerationTool(payload map[string]any) {
	rawTools, ok := payload["tools"]
	if !ok || rawTools == nil {
		return
	}
	tools, ok := responseToolsSlice(rawTools)
	if !ok {
		return
	}
	filtered := make([]any, 0, len(tools))
	for _, rawTool := range tools {
		if !responseToolHasType(rawTool, "image_generation") {
			filtered = append(filtered, rawTool)
		}
	}
	if len(filtered) == 0 {
		delete(payload, "tools")
		return
	}
	payload["tools"] = filtered
}

func responseToolsSlice(value any) ([]any, bool) {
	switch tools := value.(type) {
	case []any:
		return tools, true
	case []map[string]any:
		items := make([]any, 0, len(tools))
		for _, tool := range tools {
			items = append(items, tool)
		}
		return items, true
	default:
		return nil, false
	}
}

func responseToolHasType(value any, toolType string) bool {
	tool, ok := value.(map[string]any)
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(mapString(tool, "type")), toolType)
}

func removeResponsesImageGenerationToolChoice(payload map[string]any) {
	choice, ok := payload["tool_choice"]
	if !ok || choice == nil {
		return
	}
	switch typed := choice.(type) {
	case string:
		if strings.EqualFold(strings.TrimSpace(typed), "image_generation") {
			delete(payload, "tool_choice")
		}
	case map[string]any:
		if responseToolChoiceTargetsImageGeneration(typed) {
			delete(payload, "tool_choice")
		}
	case map[string]string:
		items := make(map[string]any, len(typed))
		for key, value := range typed {
			items[key] = value
		}
		if responseToolChoiceTargetsImageGeneration(items) {
			delete(payload, "tool_choice")
		}
	}
}

func responseToolChoiceTargetsImageGeneration(choice map[string]any) bool {
	if strings.EqualFold(strings.TrimSpace(mapString(choice, "type")), "image_generation") {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(mapString(choice, "name")), "image_generation")
}

func imageGenerationDisabledError() contract.ProviderError {
	return contract.ProviderError{
		Class:      "not_supported",
		StatusCode: http.StatusBadRequest,
		Message:    "image generation is disabled for this provider account",
	}
}
