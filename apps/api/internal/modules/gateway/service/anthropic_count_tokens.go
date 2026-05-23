package service

import (
	"fmt"

	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Service) NormalizeAnthropicCountTokens(req apiopenapi.AnthropicCountTokensRequest, meta RequestMeta) (gatewaycontract.CanonicalRequest, error) {
	if len(req.Messages) == 0 {
		return gatewaycontract.CanonicalRequest{}, fmt.Errorf("messages are empty")
	}
	generateReq := apiopenapi.AnthropicMessagesRequest{
		MaxTokens:            1,
		Messages:             append([]apiopenapi.AnthropicMessage(nil), req.Messages...),
		Model:                req.Model,
		System:               anthropicCountTokensSystem(req.System),
		Thinking:             req.Thinking,
		ToolChoice:           req.ToolChoice,
		Tools:                req.Tools,
		AdditionalProperties: cloneMap(req.AdditionalProperties),
	}
	canonical := s.NormalizeAnthropicMessages(generateReq, meta)
	canonical.RequestCapabilities = append(canonical.RequestCapabilities, gatewaycontract.RequestCapability{Key: capabilitiescontract.KeyTokenCounting, Version: "v1"})
	canonical.RequestCapabilities = dedupeRequestCapabilities(canonical.RequestCapabilities)
	return canonical, nil
}

func anthropicCountTokensSystem(system *apiopenapi.AnthropicCountTokensRequest_System) *apiopenapi.AnthropicMessagesRequest_System {
	if system == nil {
		return nil
	}
	out := apiopenapi.AnthropicMessagesRequest_System{}
	if value, err := system.AsAnthropicCountTokensRequestSystem0(); err == nil {
		_ = out.FromAnthropicMessagesRequestSystem0(value)
		return &out
	}
	if value, err := system.AsAnthropicCountTokensRequestSystem1(); err == nil {
		_ = out.FromAnthropicMessagesRequestSystem1(value)
		return &out
	}
	return nil
}

func (s *Service) RenderAnthropicCountTokens(resp gatewaycontract.TokenCountResponse) apiopenapi.AnthropicCountTokensResponse {
	rendered := apiopenapi.AnthropicCountTokensResponse{
		InputTokens:          resp.TotalTokens,
		AdditionalProperties: cloneMap(resp.Metadata),
	}
	if len(resp.CompatibilityWarnings) > 0 {
		if rendered.AdditionalProperties == nil {
			rendered.AdditionalProperties = map[string]interface{}{}
		}
		rendered.AdditionalProperties["compatibilityWarnings"] = append([]string(nil), resp.CompatibilityWarnings...)
	}
	if len(rendered.AdditionalProperties) == 0 {
		rendered.AdditionalProperties = nil
	}
	return rendered
}
