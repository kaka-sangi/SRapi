package httpserver

import (
	"encoding/json"
	"strconv"
	"strings"

	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

type gatewayModelSuffix struct {
	RequestedModel string
	BaseModel      string
	HasSuffix      bool
	RawSuffix      string
	Reasoning      map[string]any
}

func gatewayModelSuffixFromModel(model string) gatewayModelSuffix {
	requested := strings.TrimSpace(model)
	suffix := accountModelSuffix(requested)
	if suffix == "" {
		return gatewayModelSuffix{RequestedModel: requested, BaseModel: requested}
	}
	base := strings.TrimSpace(strings.TrimSuffix(requested, suffix))
	raw := strings.TrimSuffix(strings.TrimPrefix(suffix, "("), ")")
	return gatewayModelSuffix{
		RequestedModel: requested,
		BaseModel:      base,
		HasSuffix:      true,
		RawSuffix:      raw,
		Reasoning:      gatewayReasoningFromModelSuffix(raw),
	}
}

func gatewayReasoningFromModelSuffix(raw string) map[string]any {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch raw {
	case "":
		return nil
	case "none":
		return map[string]any{"effort": "none", "type": "disabled", "budget_tokens": 0}
	case "auto", "-1":
		return map[string]any{"effort": "auto", "type": "enabled", "budget_tokens": -1}
	case "minimal", "low", "medium", "high", "xhigh", "max":
		return map[string]any{"effort": raw}
	}
	budget, err := strconv.Atoi(raw)
	if err != nil || budget < 0 {
		return nil
	}
	if budget == 0 {
		return map[string]any{"effort": "none", "type": "disabled", "budget_tokens": 0}
	}
	return map[string]any{"type": "enabled", "budget_tokens": budget}
}

func applyGatewayModelSuffix(canonical *gatewaycontract.CanonicalRequest, suffix gatewayModelSuffix) {
	if canonical == nil || !suffix.HasSuffix {
		return
	}
	if len(suffix.Reasoning) > 0 {
		reasoning := cloneAnyMap(canonical.Reasoning)
		if reasoning == nil {
			reasoning = map[string]any{}
		}
		for key, value := range suffix.Reasoning {
			reasoning[key] = value
		}
		canonical.Reasoning = reasoning
		if !gatewayRequestCapabilityContains(canonical.RequestCapabilities, capabilitiescontract.KeyReasoningControl) {
			canonical.RequestCapabilities = append(canonical.RequestCapabilities, gatewaycontract.RequestCapability{Key: capabilitiescontract.KeyReasoningControl, Version: "v1"})
		}
	}
	canonical.RawBody = applyGatewayModelSuffixRawBody(canonical.RawBody, canonical, suffix)
}

func applyGatewayModelSuffixRawBody(raw []byte, canonical *gatewaycontract.CanonicalRequest, suffix gatewayModelSuffix) []byte {
	if len(raw) == 0 || canonical == nil || !suffix.HasSuffix {
		return raw
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil || doc == nil {
		return raw
	}
	protocol := strings.ToLower(strings.TrimSpace(string(canonical.SourceProtocol)))
	switch protocol {
	case string(gatewaycontract.ProtocolOpenAICompatible):
		if strings.TrimSpace(suffix.BaseModel) != "" {
			doc["model"] = suffix.BaseModel
		}
	case string(gatewaycontract.ProtocolAnthropicCompatible):
		if strings.TrimSpace(suffix.BaseModel) != "" {
			doc["model"] = suffix.BaseModel
		}
	}
	if len(suffix.Reasoning) == 0 {
		updated, err := json.Marshal(doc)
		if err != nil {
			return raw
		}
		return updated
	}
	switch protocol {
	case string(gatewaycontract.ProtocolOpenAICompatible):
		applyOpenAIModelSuffixRawBody(doc, canonical, suffix)
	case string(gatewaycontract.ProtocolAnthropicCompatible):
		applyAnthropicModelSuffixRawBody(doc, suffix)
	case string(gatewaycontract.ProtocolGeminiCompatible):
		applyGeminiModelSuffixRawBody(doc, suffix)
	}
	updated, err := json.Marshal(doc)
	if err != nil {
		return raw
	}
	return updated
}

func applyOpenAIModelSuffixRawBody(doc map[string]any, canonical *gatewaycontract.CanonicalRequest, suffix gatewayModelSuffix) {
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(canonical.SourceEndpoint)), "/chat/completions") {
		if effort := strings.TrimSpace(mapString(suffix.Reasoning, "effort")); effort != "" {
			doc["reasoning_effort"] = effort
		}
		return
	}
	reasoning, _ := doc["reasoning"].(map[string]any)
	if reasoning == nil {
		reasoning = map[string]any{}
		doc["reasoning"] = reasoning
	}
	if effort := strings.TrimSpace(mapString(suffix.Reasoning, "effort")); effort != "" {
		reasoning["effort"] = effort
	}
	if budget, ok := suffix.Reasoning["budget_tokens"]; ok {
		reasoning["budget_tokens"] = budget
	}
	if value := strings.TrimSpace(mapString(suffix.Reasoning, "type")); value != "" {
		reasoning["type"] = value
	}
}

func applyAnthropicModelSuffixRawBody(doc map[string]any, suffix gatewayModelSuffix) {
	thinking := map[string]any{}
	if value := strings.TrimSpace(mapString(suffix.Reasoning, "type")); value == "disabled" {
		thinking["type"] = "disabled"
	} else {
		thinking["type"] = "enabled"
		if budget, ok := positiveIntFromAny(suffix.Reasoning["budget_tokens"]); ok {
			thinking["budget_tokens"] = budget
		}
	}
	doc["thinking"] = thinking
}

func applyGeminiModelSuffixRawBody(doc map[string]any, suffix gatewayModelSuffix) {
	generationConfig, _ := doc["generationConfig"].(map[string]any)
	if generationConfig == nil {
		generationConfig = map[string]any{}
		doc["generationConfig"] = generationConfig
	}
	if effort := strings.TrimSpace(mapString(suffix.Reasoning, "effort")); effort != "" {
		if budget, ok := gatewayThinkingBudgetForEffort(effort); ok {
			generationConfig["thinkingConfig"] = map[string]any{
				"thinkingBudget":  budget,
				"includeThoughts": budget != 0,
			}
			return
		}
	}
	if budget, ok := intFromAny(suffix.Reasoning["budget_tokens"]); ok {
		generationConfig["thinkingConfig"] = map[string]any{
			"thinkingBudget":  budget,
			"includeThoughts": budget != 0,
		}
	}
}

func gatewayThinkingBudgetForEffort(effort string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "auto":
		return -1, true
	case "none":
		return 0, true
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
	case int64:
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

func positiveIntFromAny(value any) (int, bool) {
	parsed, ok := intFromAny(value)
	return parsed, ok && parsed > 0
}

func gatewayRequestCapabilityContains(values []gatewaycontract.RequestCapability, key string) bool {
	for _, value := range values {
		if value.Key == key {
			return true
		}
	}
	return false
}

func chatRequestModelSuffix(req apiopenapi.ChatCompletionRequest) gatewayModelSuffix {
	return gatewayModelSuffixFromModel(req.Model)
}

func responsesRequestModelSuffix(req apiopenapi.ResponsesRequest) gatewayModelSuffix {
	return gatewayModelSuffixFromModel(req.Model)
}
