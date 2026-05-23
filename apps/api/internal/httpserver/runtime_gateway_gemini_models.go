package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Server) handleListGeminiModels(w http.ResponseWriter, r *http.Request) {
	authed, err := s.requireGatewayKey(r)
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
	visible := filterGatewayModels(toGatewayModels(models), authed.Key.AllowedModels)
	geminiModels := toGeminiModels(models, visible)
	page, nextToken := paginateGeminiModels(geminiModels, pageSize, offset)
	resp := apiopenapi.GeminiModelList{Models: page}
	if nextToken != "" {
		resp.NextPageToken = &nextToken
	}
	writeJSONAny(w, http.StatusOK, resp)
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
