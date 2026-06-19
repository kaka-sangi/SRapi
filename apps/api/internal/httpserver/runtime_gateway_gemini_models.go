package httpserver

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Server) handleListGeminiModels(w http.ResponseWriter, r *http.Request) {
	authed, err := s.requireGeminiGatewayKey(r)
	if err != nil {
		writeGeminiGatewayAuthError(w, err)
		return
	}
	pageSize, offset, err := geminiModelListPagination(r)
	if err != nil {
		writeGeminiGatewayError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}
	models, err := s.runtime.models.List(r.Context())
	if err != nil {
		writeGeminiGatewayError(w, http.StatusInternalServerError, "INTERNAL", "failed to list models")
		return
	}
	models, err = s.filterGeminiModelsForForcedProvider(r, models)
	if err != nil {
		writeGeminiGatewayError(w, http.StatusInternalServerError, "INTERNAL", "failed to list models")
		return
	}
	hidden := s.runtime.modelsHiddenByAvailability(r.Context(), models, authed.Key, gatewaySourceEndpoint(r.Context(), "/v1beta/models"), gatewayForcedProviderKey(r.Context()))
	visible := filterGatewayModels(hideGatewayModels(toGatewayModels(models), hidden), authed.Key.AllowedModels)
	geminiModels := toGeminiModels(models, visible)
	page, nextToken := paginateGeminiModels(geminiModels, pageSize, offset)
	resp := apiopenapi.GeminiModelList{Models: page}
	if nextToken != "" {
		resp.NextPageToken = &nextToken
	}
	writeJSONAny(w, http.StatusOK, resp)
}

func (s *Server) handleGetGeminiModel(w http.ResponseWriter, r *http.Request) {
	authed, err := s.requireGeminiGatewayKey(r)
	if err != nil {
		writeGeminiGatewayAuthError(w, err)
		return
	}
	modelRef, err := geminiModelNameFromPath(r.URL.EscapedPath())
	if err != nil {
		writeGeminiGatewayError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}
	modelResolution, err := s.runtime.resolveModelCached(r.Context(), modelRef)
	if err != nil || modelResolution.Model.Status != modelcontract.StatusActive {
		writeGeminiGatewayError(w, http.StatusNotFound, "NOT_FOUND", "model not found")
		return
	}
	if !s.geminiModelHasForcedProviderMapping(r, modelResolution.Model.ID) {
		writeGeminiGatewayError(w, http.StatusNotFound, "NOT_FOUND", "model not found")
		return
	}
	if !apiKeyAllowsModelReference(authed.Key.AllowedModels, modelResolution) {
		writeGeminiGatewayError(w, http.StatusForbidden, "PERMISSION_DENIED", "model not allowed for this api key")
		return
	}
	if s.runtime.geminiModelHiddenByAvailability(r, authed.Key, modelResolution.Model) {
		writeGeminiGatewayError(w, http.StatusNotFound, "NOT_FOUND", "model not found")
		return
	}
	writeJSONAny(w, http.StatusOK, geminiModelInfo(modelResolution.Model))
}

func (rt *runtimeState) geminiModelHiddenByAvailability(r *http.Request, apiKey apikeycontract.APIKey, model modelcontract.Model) bool {
	hidden := rt.modelsHiddenByAvailability(r.Context(), []modelcontract.Model{model}, apiKey, gatewaySourceEndpoint(r.Context(), "/v1beta/models"), gatewayForcedProviderKey(r.Context()))
	_, ok := hidden[model.CanonicalName]
	return ok
}

func (s *Server) filterGeminiModelsForForcedProvider(r *http.Request, models []modelcontract.Model) ([]modelcontract.Model, error) {
	forcedProviderKey := gatewayForcedProviderKey(r.Context())
	if forcedProviderKey == "" {
		return models, nil
	}
	providers, err := s.runtime.providers.List(r.Context())
	if err != nil {
		return nil, err
	}
	providerID, ok := geminiProviderIDByName(providers, forcedProviderKey)
	if !ok {
		return []modelcontract.Model{}, nil
	}
	out := make([]modelcontract.Model, 0, len(models))
	for _, model := range models {
		mappings, err := s.runtime.models.ListMappingsByModel(r.Context(), model.ID)
		if err != nil {
			return nil, err
		}
		if mappingsIncludeActiveProvider(mappings, providerID) {
			out = append(out, model)
		}
	}
	return out, nil
}

func (s *Server) geminiModelHasForcedProviderMapping(r *http.Request, modelID int) bool {
	forcedProviderKey := gatewayForcedProviderKey(r.Context())
	if forcedProviderKey == "" {
		return true
	}
	providers, err := s.runtime.providers.List(r.Context())
	if err != nil {
		return false
	}
	providerID, ok := geminiProviderIDByName(providers, forcedProviderKey)
	if !ok {
		return false
	}
	mappings, err := s.runtime.models.ListMappingsByModel(r.Context(), modelID)
	if err != nil {
		return false
	}
	return mappingsIncludeActiveProvider(mappings, providerID)
}

func geminiProviderIDByName(providers []providercontract.Provider, name string) (int, bool) {
	for _, provider := range providers {
		if strings.EqualFold(strings.TrimSpace(provider.Name), strings.TrimSpace(name)) {
			return provider.ID, true
		}
	}
	return 0, false
}

func mappingsIncludeActiveProvider(mappings []modelcontract.ModelProviderMapping, providerID int) bool {
	for _, mapping := range mappings {
		if mapping.ProviderID == providerID && mapping.Status == modelcontract.StatusActive {
			return true
		}
	}
	return false
}

func geminiModelNameFromPath(escapedPath string) (string, error) {
	raw := strings.TrimPrefix(escapedPath, "/v1beta/models/")
	if raw == escapedPath || strings.TrimSpace(raw) == "" {
		return "", errors.New("model is required")
	}
	name, err := url.PathUnescape(raw)
	if err != nil || strings.TrimSpace(name) == "" {
		return "", errors.New("model is invalid")
	}
	return strings.TrimPrefix(strings.TrimSpace(name), "models/"), nil
}

func geminiModelListPagination(r *http.Request) (int, int, error) {
	pageSize := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("pageSize")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > 1000 {
			return 0, 0, errors.New("pageSize must be an integer between 1 and 1000")
		}
		pageSize = parsed
	}
	offset := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("pageToken")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			return 0, 0, errors.New("pageToken is invalid")
		}
		offset = parsed
	}
	return pageSize, offset, nil
}

func toGeminiModels(models []modelcontract.Model, visible []apiopenapi.OpenAIModel) []apiopenapi.GeminiModelInfo {
	visibleSet := make(map[string]struct{}, len(visible))
	for _, model := range visible {
		visibleSet[model.Id] = struct{}{}
	}
	out := make([]apiopenapi.GeminiModelInfo, 0, len(visible))
	for _, model := range models {
		if _, ok := visibleSet[model.CanonicalName]; !ok || model.Status != modelcontract.StatusActive {
			continue
		}
		out = append(out, geminiModelInfo(model))
	}
	return out
}

func geminiModelInfo(model modelcontract.Model) apiopenapi.GeminiModelInfo {
	name := strings.TrimPrefix(model.CanonicalName, "models/")
	description := "SRapi model registry entry for " + model.CanonicalName
	return apiopenapi.GeminiModelInfo{
		Name:                       "models/" + name,
		BaseModelId:                name,
		Version:                    geminiModelVersion(model),
		DisplayName:                model.DisplayName,
		Description:                &description,
		InputTokenLimit:            positiveIntValue(model.ContextWindow),
		OutputTokenLimit:           positiveIntValue(model.MaxOutputTokens),
		SupportedGenerationMethods: geminiSupportedGenerationMethods(model),
	}
}

func geminiModelVersion(model modelcontract.Model) string {
	for _, value := range []*string{model.Family, model.QualityTier} {
		if value != nil && strings.TrimSpace(*value) != "" {
			return strings.TrimSpace(*value)
		}
	}
	return "srapi"
}

func geminiSupportedGenerationMethods(model modelcontract.Model) []apiopenapi.GeminiModelInfoSupportedGenerationMethods {
	methods := []apiopenapi.GeminiModelInfoSupportedGenerationMethods{apiopenapi.GenerateContent}
	if modelHasCapability(model, capabilitiescontract.KeyStreaming) {
		methods = append(methods, apiopenapi.StreamGenerateContent)
	}
	if modelHasCapability(model, capabilitiescontract.KeyTokenCounting) {
		methods = append(methods, apiopenapi.CountTokens)
	}
	return methods
}

func modelHasCapability(model modelcontract.Model, key string) bool {
	for _, descriptor := range model.Capabilities {
		if strings.EqualFold(strings.TrimSpace(descriptor.Key), key) {
			return true
		}
	}
	return false
}

func positiveIntValue(value *int) int {
	if value == nil || *value < 0 {
		return 0
	}
	return *value
}

func paginateGeminiModels(models []apiopenapi.GeminiModelInfo, pageSize int, offset int) ([]apiopenapi.GeminiModelInfo, string) {
	if offset >= len(models) {
		return []apiopenapi.GeminiModelInfo{}, ""
	}
	if pageSize <= 0 || offset+pageSize >= len(models) {
		return models[offset:], ""
	}
	return models[offset : offset+pageSize], strconv.Itoa(offset + pageSize)
}
