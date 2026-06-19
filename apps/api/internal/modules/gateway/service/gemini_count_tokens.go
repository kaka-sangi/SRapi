package service

import (
	"fmt"
	"strings"

	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Service) NormalizeGeminiCountTokens(req apiopenapi.GeminiCountTokensRequest, model string, meta RequestMeta) (gatewaycontract.CanonicalRequest, error) {
	generateReq, err := geminiCountTokensGenerateRequest(req)
	if err != nil {
		return gatewaycontract.CanonicalRequest{}, err
	}
	canonical := s.NormalizeGeminiGenerateContent(generateReq, model, false, meta)
	canonical.SourceEndpoint = strings.TrimSpace(meta.SourceEndpoint)
	canonical.RequestCapabilities = append(canonical.RequestCapabilities, gatewaycontract.RequestCapability{Key: capabilitiescontract.KeyGeminiCountTokens, Version: "v1"})
	canonical.RequestCapabilities = append(canonical.RequestCapabilities, gatewaycontract.RequestCapability{Key: capabilitiescontract.KeyTokenCounting, Version: "v1"})
	canonical.RequestCapabilities = dedupeRequestCapabilities(canonical.RequestCapabilities)
	return canonical, nil
}

func geminiCountTokensGenerateRequest(req apiopenapi.GeminiCountTokensRequest) (apiopenapi.GeminiGenerateContentRequest, error) {
	if req.GenerateContentRequest != nil {
		if len(req.GenerateContentRequest.Contents) == 0 {
			return apiopenapi.GeminiGenerateContentRequest{}, fmt.Errorf("generateContentRequest contents is empty")
		}
		return *req.GenerateContentRequest, nil
	}
	if req.Contents == nil || len(*req.Contents) == 0 {
		return apiopenapi.GeminiGenerateContentRequest{}, fmt.Errorf("countTokens contents is empty")
	}
	return apiopenapi.GeminiGenerateContentRequest{
		Contents:             append([]apiopenapi.GeminiContent(nil), (*req.Contents)...),
		SystemInstruction:    req.SystemInstruction,
		GenerationConfig:     req.GenerationConfig,
		SafetySettings:       req.SafetySettings,
		Tools:                req.Tools,
		ToolConfig:           req.ToolConfig,
		AdditionalProperties: cloneMap(req.AdditionalProperties),
	}, nil
}

func (s *Service) BuildCanonicalTokenCountResponse(req gatewaycontract.CanonicalRequest, total int, cached *int, promptDetails []gatewaycontract.ModalityTokenCount, cacheDetails []gatewaycontract.ModalityTokenCount, metadata map[string]any) gatewaycontract.TokenCountResponse {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = req.CanonicalModel
	}
	canonicalModel := strings.TrimSpace(req.CanonicalModel)
	if canonicalModel == "" {
		canonicalModel = model
	}
	if total < 0 {
		total = 0
	}
	return gatewaycontract.TokenCountResponse{
		RequestID:               strings.TrimSpace(req.RequestID),
		Model:                   model,
		CanonicalModel:          canonicalModel,
		TotalTokens:             total,
		CachedContentTokenCount: cloneInt(cached),
		PromptTokensDetails:     cloneModalityTokenCounts(promptDetails),
		CacheTokensDetails:      cloneModalityTokenCounts(cacheDetails),
		Metadata:                cloneMap(metadata),
		CompatibilityWarnings:   uniqueStrings(req.CompatibilityWarnings),
	}
}

func (s *Service) RenderGeminiCountTokens(resp gatewaycontract.TokenCountResponse) apiopenapi.GeminiCountTokensResponse {
	rendered := apiopenapi.GeminiCountTokensResponse{
		TotalTokens:          resp.TotalTokens,
		AdditionalProperties: cloneMap(resp.Metadata),
	}
	if resp.CachedContentTokenCount != nil {
		rendered.CachedContentTokenCount = cloneInt(resp.CachedContentTokenCount)
	}
	if len(resp.PromptTokensDetails) > 0 {
		details := geminiModalityTokenCounts(resp.PromptTokensDetails)
		rendered.PromptTokensDetails = &details
	}
	if len(resp.CacheTokensDetails) > 0 {
		details := geminiModalityTokenCounts(resp.CacheTokensDetails)
		rendered.CacheTokensDetails = &details
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

func cloneModalityTokenCounts(values []gatewaycontract.ModalityTokenCount) []gatewaycontract.ModalityTokenCount {
	if values == nil {
		return nil
	}
	out := make([]gatewaycontract.ModalityTokenCount, len(values))
	for idx, value := range values {
		out[idx] = gatewaycontract.ModalityTokenCount{
			Modality:   value.Modality,
			TokenCount: value.TokenCount,
			Metadata:   cloneMap(value.Metadata),
		}
	}
	return out
}

func geminiModalityTokenCounts(values []gatewaycontract.ModalityTokenCount) []apiopenapi.GeminiModalityTokenCount {
	out := make([]apiopenapi.GeminiModalityTokenCount, 0, len(values))
	for _, value := range values {
		out = append(out, apiopenapi.GeminiModalityTokenCount{
			Modality:             value.Modality,
			TokenCount:           value.TokenCount,
			AdditionalProperties: cloneMap(value.Metadata),
		})
	}
	return out
}
