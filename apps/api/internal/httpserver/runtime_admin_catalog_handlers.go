package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	groupratelimitscontract "github.com/srapi/srapi/apps/api/internal/modules/group_rate_limits/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	modelservice "github.com/srapi/srapi/apps/api/internal/modules/models/service"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	providerservice "github.com/srapi/srapi/apps/api/internal/modules/providers/service"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	usersservice "github.com/srapi/srapi/apps/api/internal/modules/users/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Server) handleAdminOverview(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	providers, err := s.runtime.providers.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list providers", requestID)
		return
	}
	models, err := s.runtime.models.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list models", requestID)
		return
	}
	accounts, err := s.runtime.accounts.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list accounts", requestID)
		return
	}
	usageLogs, err := s.runtime.usage.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list usage logs", requestID)
		return
	}
	decisions, err := s.runtime.scheduler.ListDecisions(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list scheduler decisions", requestID)
		return
	}
	users, err := s.runtime.users.List(r.Context(), usersservice.ListRequest{})
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list users", requestID)
		return
	}
	dailyAggregates, err := s.runtime.usage.Aggregate(r.Context(), usagecontract.QueryFilter{}, usagecontract.AggregateDimensionDay)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to aggregate usage", requestID)
		return
	}
	activeAccounts := 0
	for _, account := range accounts {
		if account.Status == accountcontract.StatusActive {
			activeAccounts++
		}
	}
	totalRequestCount := 0
	totalTokenCount := 0
	totalCost := "0.00000000"
	currency := "USD"
	if len(dailyAggregates) > 0 {
		for _, aggregate := range dailyAggregates {
			totalRequestCount += aggregate.RequestCount
			totalTokenCount += aggregate.TotalTokens
			totalCost = addDecimalStrings(totalCost, aggregate.TotalCost)
			if aggregate.Currency != "" {
				currency = aggregate.Currency
			}
		}
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AdminOverviewResponse{
		Data: apiopenapi.AdminOverview{
			AccountCount:           len(accounts),
			ActiveAccountCount:     activeAccounts,
			Currency:               currency,
			ModelCount:             len(models),
			ProviderCount:          len(providers),
			RequestSuccessRate:     usageSuccessRate(usageLogs),
			SchedulerDecisionCount: len(decisions),
			TotalCost:              totalCost,
			TotalRequestCount:      totalRequestCount,
			TotalTokenCount:        totalTokenCount,
			UsageLogCount:          len(usageLogs),
			UserCount:              len(users),
		},
		RequestId: requestID,
	})
}

func (s *Server) handleListAdminProviders(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	providers, err := s.runtime.providers.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list providers", requestID)
		return
	}
	providers = filterProviders(providers, r.URL.Query().Get("status"), r.URL.Query().Get("q"))
	data := make([]apiopenapi.Provider, 0, len(providers))
	for _, provider := range providers {
		data = append(data, toAPIProvider(provider))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminProvider(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	var body apiopenapi.CreateProviderRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid provider request", requestID)
		return
	}
	provider, err := s.runtime.providers.Create(r.Context(), providercontract.CreateRequest{
		Name:         body.Name,
		DisplayName:  body.DisplayName,
		AdapterType:  string(body.AdapterType),
		Protocol:     string(body.Protocol),
		Status:       toProviderStatusPtr(body.Status),
		Capabilities: jsonObjectToMap(body.Capabilities),
		ConfigSchema: jsonObjectToMap(body.ConfigSchema),
	})
	if err != nil {
		switch {
		case errors.Is(err, providerservice.ErrProviderExists):
			writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "provider already exists", requestID)
		case errors.Is(err, providerservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid provider request", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to create provider", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider.create", "provider", strconv.Itoa(provider.ID), nil, map[string]any{
		"name":         provider.Name,
		"display_name": provider.DisplayName,
		"adapter_type": provider.AdapterType,
		"protocol":     provider.Protocol,
		"status":       provider.Status,
	}))
	writeJSONAny(w, http.StatusCreated, apiopenapi.ProviderResponse{
		Data:      toAPIProvider(provider),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminProvider(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	providerID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid provider id", requestID)
		return
	}
	before, err := s.runtime.providers.FindByID(r.Context(), providerID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "provider not found", requestID)
		return
	}
	var body apiopenapi.UpdateProviderRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid provider update request", requestID)
		return
	}
	provider, err := s.runtime.providers.Update(r.Context(), providerID, providercontract.UpdateRequest{
		DisplayName:  body.DisplayName,
		AdapterType:  providerAdapterTypeString(body.AdapterType),
		Protocol:     providerProtocolString(body.Protocol),
		Status:       toProviderStatusPtr(body.Status),
		Capabilities: jsonObjectToMapPtr(body.Capabilities),
		ConfigSchema: jsonObjectToMapPtr(body.ConfigSchema),
	})
	if err != nil {
		switch {
		case errors.Is(err, providerservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid provider update request", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to update provider", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider.update", "provider", strconv.Itoa(provider.ID), providerAuditSnapshot(before), providerAuditSnapshot(provider)))
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderResponse{
		Data:      toAPIProvider(provider),
		RequestId: requestID,
	})
}

func (s *Server) handleDeleteAdminProvider(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	providerID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || providerID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid provider id", requestID)
		return
	}
	before, err := s.runtime.providers.FindByID(r.Context(), providerID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "provider not found", requestID)
		return
	}
	// Guard: refuse to delete a provider that still has live upstream accounts.
	// Those accounts are the scheduler's candidates; deleting the provider would
	// orphan them. The operator must archive/remove the accounts first.
	accounts, err := s.runtime.accounts.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to check provider accounts", requestID)
		return
	}
	inUse := 0
	for _, account := range accounts {
		if account.ProviderID == providerID && account.Status != accountcontract.StatusArchived {
			inUse++
		}
	}
	if inUse > 0 {
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "provider still has accounts; remove or archive them first", requestID)
		return
	}
	if err := s.runtime.providers.Delete(r.Context(), providerID); err != nil {
		switch {
		case errors.Is(err, providerservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid provider id", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to delete provider", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider.delete", "provider", strconv.Itoa(providerID), providerAuditSnapshot(before), nil))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"id": providerID, "deleted": true},
		"request_id": requestID,
	})
}

func (s *Server) handleTestAdminProvider(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	providerID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid provider id", requestID)
		return
	}
	provider, err := s.runtime.providers.FindByID(r.Context(), providerID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "provider not found", requestID)
		return
	}
	startedAt := time.Now()
	result := s.runtime.testProvider(r.Context(), provider, startedAt)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider.test", "provider", strconv.Itoa(provider.ID), nil, map[string]any{
		"ok":     result.Ok,
		"status": result.Status,
		"checks": result.Checks,
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.AdminTestResultResponse{
		Data:      result,
		RequestId: requestID,
	})
}

func (s *Server) handleListAdminModels(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	models, err := s.runtime.models.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list models", requestID)
		return
	}
	models = filterModels(models, r.URL.Query().Get("status"), r.URL.Query().Get("q"))
	data := make([]apiopenapi.Model, 0, len(models))
	for _, model := range models {
		data = append(data, toAPIModel(model))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.ModelListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminModel(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	var body apiopenapi.CreateModelRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid model request", requestID)
		return
	}
	model, err := s.runtime.models.Create(r.Context(), modelcontract.CreateRequest{
		CanonicalName:   body.CanonicalName,
		DisplayName:     body.DisplayName,
		Family:          body.Family,
		ContextWindow:   body.ContextWindow,
		MaxOutputTokens: body.MaxOutputTokens,
		QualityTier:     body.QualityTier,
		Status:          toModelStatusPtr(body.Status),
		Capabilities:    toCapabilityDescriptors(body.Capabilities),
	})
	if err != nil {
		switch {
		case errors.Is(err, modelservice.ErrModelExists):
			writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "model already exists", requestID)
		case errors.Is(err, modelservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model request", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to create model", requestID)
		}
		return
	}
	s.runtime.invalidateModelCache(model.CanonicalName)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "model.create", "model", strconv.Itoa(model.ID), nil, map[string]any{
		"canonical_name": model.CanonicalName,
		"display_name":   model.DisplayName,
		"status":         model.Status,
	}))
	writeJSONAny(w, http.StatusCreated, apiopenapi.ModelResponse{
		Data:      toAPIModel(model),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminModel(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	modelID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model id", requestID)
		return
	}
	before, err := s.runtime.models.FindByID(r.Context(), modelID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "model not found", requestID)
		return
	}
	var body apiopenapi.UpdateModelRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid model update request", requestID)
		return
	}
	model, err := s.runtime.models.Update(r.Context(), modelID, modelcontract.UpdateRequest{
		DisplayName:     body.DisplayName,
		Family:          optionalNullableString(body.Family),
		ContextWindow:   optionalNullableInt(body.ContextWindow),
		MaxOutputTokens: optionalNullableInt(body.MaxOutputTokens),
		QualityTier:     optionalNullableString(body.QualityTier),
		Status:          toModelStatusPtr(body.Status),
		Capabilities:    toCapabilityDescriptorsPtrContract(body.Capabilities),
	})
	if err != nil {
		switch {
		case errors.Is(err, modelservice.ErrModelNotFound):
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "model not found", requestID)
		case errors.Is(err, modelservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model update request", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to update model", requestID)
		}
		return
	}
	s.runtime.invalidateModelCache(model.CanonicalName, before.CanonicalName)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "model.update", "model", strconv.Itoa(model.ID), modelAuditSnapshot(before), modelAuditSnapshot(model)))
	writeJSONAny(w, http.StatusOK, apiopenapi.ModelResponse{
		Data:      toAPIModel(model),
		RequestId: requestID,
	})
}

func (s *Server) handleDeleteAdminModel(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	modelID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || modelID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model id", requestID)
		return
	}
	before, err := s.runtime.models.FindByID(r.Context(), modelID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "model not found", requestID)
		return
	}
	if err := s.runtime.models.Delete(r.Context(), modelID); err != nil {
		switch {
		case errors.Is(err, modelservice.ErrModelNotFound):
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "model not found", requestID)
		case errors.Is(err, modelservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model id", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to delete model", requestID)
		}
		return
	}
	s.runtime.invalidateModelCache(before.CanonicalName)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "model.delete", "model", strconv.Itoa(modelID), modelAuditSnapshot(before), nil))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"id": modelID, "deleted": true},
		"request_id": requestID,
	})
}

func (s *Server) handleCreateAdminModelAlias(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	modelID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model id", requestID)
		return
	}
	var body apiopenapi.CreateModelAliasRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid model alias request", requestID)
		return
	}
	alias, err := s.runtime.models.CreateAlias(r.Context(), modelID, modelcontract.CreateAliasRequest{
		Alias:          body.Alias,
		StrategyHint:   body.StrategyHint,
		FallbackModels: derefStrings(body.FallbackModels),
		Status:         toModelStatusPtr(body.Status),
	})
	if err != nil {
		switch {
		case errors.Is(err, modelservice.ErrAliasExists):
			writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "model alias already exists", requestID)
		case errors.Is(err, modelservice.ErrModelNotFound):
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "model not found", requestID)
		case errors.Is(err, modelservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model alias request", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to create model alias", requestID)
		}
		return
	}
	s.runtime.invalidateModelCache(alias.Alias)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "model_alias.create", "model_alias", strconv.Itoa(alias.ID), nil, map[string]any{
		"alias":           alias.Alias,
		"model_id":        alias.ModelID,
		"fallback_models": alias.FallbackModels,
		"status":          alias.Status,
	}))
	writeJSONAny(w, http.StatusCreated, apiopenapi.ModelAliasResponse{
		Data:      toAPIModelAlias(alias),
		RequestId: requestID,
	})
}

func (s *Server) handleCreateAdminModelMapping(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	modelID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model id", requestID)
		return
	}
	var body apiopenapi.CreateModelProviderMappingRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid model mapping request", requestID)
		return
	}
	providerID, err := strconv.Atoi(string(body.ProviderId))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid provider id", requestID)
		return
	}
	if _, err := s.runtime.providers.FindByID(r.Context(), providerID); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "provider not found", requestID)
		return
	}
	mapping, err := s.runtime.models.CreateMapping(r.Context(), modelID, modelcontract.CreateMappingRequest{
		ProviderID:         providerID,
		UpstreamModelName:  body.UpstreamModelName,
		Status:             toModelStatusPtr(body.Status),
		CapabilityOverride: toCapabilityDescriptors(body.CapabilityOverride),
		PricingOverride:    jsonObjectToMap(body.PricingOverride),
	})
	if err != nil {
		switch {
		case errors.Is(err, modelservice.ErrMappingExists):
			writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "model provider mapping already exists", requestID)
		case errors.Is(err, modelservice.ErrModelNotFound):
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "model not found", requestID)
		case errors.Is(err, modelservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model mapping request", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to create model mapping", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "model_provider_mapping.create", "model_provider_mapping", strconv.Itoa(mapping.ID), nil, map[string]any{
		"model_id":            mapping.ModelID,
		"provider_id":         mapping.ProviderID,
		"upstream_model_name": mapping.UpstreamModelName,
		"status":              mapping.Status,
	}))
	writeJSONAny(w, http.StatusCreated, apiopenapi.ModelProviderMappingResponse{
		Data:      toAPIModelProviderMapping(mapping),
		RequestId: requestID,
	})
}

func (s *Server) handleListAdminModelAliases(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	modelID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || modelID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model id", requestID)
		return
	}
	aliases, err := s.runtime.models.ListAliasesByModel(r.Context(), modelID)
	if err != nil {
		switch {
		case errors.Is(err, modelservice.ErrModelNotFound):
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "model not found", requestID)
		case errors.Is(err, modelservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model id", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list model aliases", requestID)
		}
		return
	}
	data := make([]apiopenapi.ModelAlias, 0, len(aliases))
	for _, alias := range aliases {
		data = append(data, toAPIModelAlias(alias))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.ModelAliasListResponse{
		Data:      data,
		RequestId: requestID,
	})
}

func (s *Server) handleDeleteAdminModelAlias(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	modelID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || modelID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model id", requestID)
		return
	}
	aliasID, err := strconv.Atoi(r.PathValue("aliasId"))
	if err != nil || aliasID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model alias id", requestID)
		return
	}
	if err := s.runtime.models.DeleteAlias(r.Context(), modelID, aliasID); err != nil {
		switch {
		case errors.Is(err, modelservice.ErrAliasNotFound):
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "model alias not found", requestID)
		case errors.Is(err, modelservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model alias id", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to delete model alias", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "model_alias.delete", "model_alias", strconv.Itoa(aliasID), map[string]any{"model_id": modelID}, nil))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"id": aliasID, "deleted": true},
		"request_id": requestID,
	})
}

func (s *Server) handleListAdminModelMappings(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	modelID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || modelID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model id", requestID)
		return
	}
	mappings, err := s.runtime.models.ListMappingsByModel(r.Context(), modelID)
	if err != nil {
		switch {
		case errors.Is(err, modelservice.ErrModelNotFound):
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "model not found", requestID)
		case errors.Is(err, modelservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model id", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list model mappings", requestID)
		}
		return
	}
	data := make([]apiopenapi.ModelProviderMapping, 0, len(mappings))
	for _, mapping := range mappings {
		data = append(data, toAPIModelProviderMapping(mapping))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.ModelProviderMappingListResponse{
		Data:      data,
		RequestId: requestID,
	})
}

func (s *Server) handleDeleteAdminModelMapping(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	modelID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || modelID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model id", requestID)
		return
	}
	mappingID, err := strconv.Atoi(r.PathValue("mappingId"))
	if err != nil || mappingID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model mapping id", requestID)
		return
	}
	if err := s.runtime.models.DeleteMapping(r.Context(), modelID, mappingID); err != nil {
		switch {
		case errors.Is(err, modelservice.ErrMappingNotFound):
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "model provider mapping not found", requestID)
		case errors.Is(err, modelservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model mapping id", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to delete model mapping", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "model_provider_mapping.delete", "model_provider_mapping", strconv.Itoa(mappingID), map[string]any{"model_id": modelID}, nil))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"id": mappingID, "deleted": true},
		"request_id": requestID,
	})
}

func (s *Server) handleListAdminAccounts(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	accounts, err := s.runtime.accounts.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list accounts", requestID)
		return
	}
	accounts = filterAccounts(accounts, r.URL.Query().Get("status"), r.URL.Query().Get("provider_id"))
	data := make([]apiopenapi.ProviderAccount, 0, len(accounts))
	for _, account := range accounts {
		data = append(data, s.apiAccount(r.Context(), account))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderAccountListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleGetAdminAccount(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	accountID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || accountID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return
	}
	account, err := s.runtime.accounts.FindByID(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found", requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderAccountResponse{
		Data:      s.apiAccount(r.Context(), account),
		RequestId: requestID,
	})
}

func (s *Server) handleAdminAccountRpmStatus(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	accountID, ok := adminPathID(w, r, requestID, "account")
	if !ok {
		return
	}
	status, err := s.runtime.accounts.RPMStatus(r.Context(), accountID)
	if err != nil {
		writeAccountServiceError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountRpmStatusResponse{
		Data: apiopenapi.AccountRpmStatus{
			AccountId:     apiopenapi.Id(strconv.Itoa(status.AccountID)),
			ResetAt:       status.ResetAt,
			RpmLimit:      status.RPMLimit,
			RpmUsed:       status.RPMUsed,
			WindowSeconds: status.WindowSeconds,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleAdminAccountProxyQuality(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	accountID, ok := adminPathID(w, r, requestID, "account")
	if !ok {
		return
	}
	quality, err := s.runtime.accounts.ProxyQuality(r.Context(), accountID)
	if err != nil {
		writeAccountServiceError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountProxyQualityResponse{
		Data: apiopenapi.AccountProxyQuality{
			AccountId:     apiopenapi.Id(strconv.Itoa(quality.AccountID)),
			ErrorRate:     quality.ErrorRate,
			LastCheckedAt: quality.LastCheckedAt,
			LatencyP95Ms:  quality.LatencyP95MS,
			Metadata:      mapToJsonObjectPtr(quality.Metadata),
			ProxyId:       quality.ProxyID,
			SampleCount:   quality.SampleCount,
			SuccessRate:   quality.SuccessRate,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleCreateAdminAccount(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	var body apiopenapi.CreateProviderAccountRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid account request", requestID)
		return
	}
	providerID, err := strconv.Atoi(string(body.ProviderId))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid provider id", requestID)
		return
	}
	provider, err := s.runtime.providers.FindByID(r.Context(), providerID)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "provider not found", requestID)
		return
	}
	if !accountRuntimeClassAllowed(provider.ConfigSchema, accountcontract.RuntimeClass(body.RuntimeClass)) {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "authentication method not allowed for this provider", requestID)
		return
	}
	credential := derefMap(body.Credential)
	metadata := jsonObjectToMap(body.Metadata)
	credential, err = s.refreshImportCredential(r.Context(), accountcontract.RuntimeClass(body.RuntimeClass), body.UpstreamClient, metadata, body.ProxyId, credential)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "oauth refresh failed", requestID)
		return
	}
	account, err := s.runtime.accounts.Create(r.Context(), accountcontract.CreateRequest{
		ProviderID:     providerID,
		Name:           body.Name,
		RuntimeClass:   accountcontract.RuntimeClass(body.RuntimeClass),
		Credential:     credential,
		Metadata:       metadata,
		ProxyID:        body.ProxyId,
		Status:         toAccountStatusPtr(body.Status),
		Priority:       body.Priority,
		Weight:         body.Weight,
		RiskLevel:      stringPtrFromAPI(body.RiskLevel),
		UpstreamClient: body.UpstreamClient,
	})
	if err != nil {
		switch {
		case errors.Is(err, accountservice.ErrCredentialMissing), errors.Is(err, accountservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account request", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to create account", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.create", "provider_account", strconv.Itoa(account.ID), nil, map[string]any{
		"provider_id":   account.ProviderID,
		"name":          account.Name,
		"runtime_class": account.RuntimeClass,
		"status":        account.Status,
		"priority":      account.Priority,
		"weight":        account.Weight,
	}))
	writeJSONAny(w, http.StatusCreated, apiopenapi.ProviderAccountResponse{
		Data:      s.apiAccount(r.Context(), account),
		RequestId: requestID,
	})
}

func (s *Server) handleExportAdminAccounts(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	accounts, err := s.runtime.accounts.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to export accounts", requestID)
		return
	}
	data := make([]apiopenapi.ProviderAccountExportItem, 0, len(accounts))
	for _, account := range accounts {
		groupIDs, _ := s.runtime.accounts.ListGroupIDsByAccount(r.Context(), account.ID)
		data = append(data, apiopenapi.ProviderAccountExportItem{
			CredentialExported: false,
			GroupIds:           apiIDsPtr(groupIDs),
			Metadata:           mapToJsonObjectPtr(sanitizedExportMetadata(account.Metadata)),
			Name:               account.Name,
			Priority:           account.Priority,
			ProviderId:         apiopenapi.Id(strconv.Itoa(account.ProviderID)),
			ProxyId:            account.ProxyID,
			RiskLevel:          apiStringPtr[apiopenapi.ProviderAccountExportItemRiskLevel](account.RiskLevel),
			RuntimeClass:       apiopenapi.RuntimeClass(account.RuntimeClass),
			Status:             apiopenapi.ProviderAccountStatus(account.Status),
			UpstreamClient:     account.UpstreamClient,
			Weight:             account.Weight,
		})
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderAccountExportResponse{
		Data:      data,
		RequestId: requestID,
	})
}

func (s *Server) handleBatchUpdateAdminAccounts(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	var body apiopenapi.BatchUpdateAccountsRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid account batch update request", requestID)
		return
	}
	accountIDs, err := apiIDsValueToInts(body.AccountIds)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account ids", requestID)
		return
	}
	result := s.runtime.accounts.BatchUpdateStatus(r.Context(), accountIDs, accountcontract.Status(body.Status))
	updatedIDs := make([]apiopenapi.Id, 0, len(result.Updated))
	for _, updated := range result.Updated {
		updatedIDs = append(updatedIDs, apiopenapi.Id(strconv.Itoa(updated.ID)))
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.batch_update", "provider_account", "bulk", nil, map[string]any{
		"account_ids":   accountIDs,
		"status":        body.Status,
		"updated_ids":   updatedIDs,
		"updated_count": len(updatedIDs),
		"errors":        result.Errors,
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.BatchUpdateAccountsResponse{
		Data: apiopenapi.BatchUpdateAccountsResult{
			Errors:       result.Errors,
			UpdatedCount: len(updatedIDs),
			UpdatedIds:   updatedIDs,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleBatchActionAdminAccounts(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	var body apiopenapi.BatchAccountActionRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid account batch action request", requestID)
		return
	}
	if !body.Action.Valid() {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid batch action", requestID)
		return
	}
	accountIDs, err := apiIDsValueToInts(body.AccountIds)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account ids", requestID)
		return
	}
	var result accountcontract.BatchUpdateResult
	switch body.Action {
	case apiopenapi.Recover:
		result = s.runtime.accounts.BatchRecover(r.Context(), accountIDs)
	default:
		result = s.runtime.accounts.BatchClearErrorState(r.Context(), accountIDs)
	}
	updatedIDs := make([]apiopenapi.Id, 0, len(result.Updated))
	for _, updated := range result.Updated {
		updatedIDs = append(updatedIDs, apiopenapi.Id(strconv.Itoa(updated.ID)))
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.batch_action", "provider_account", "bulk", nil, map[string]any{
		"action":        string(body.Action),
		"account_ids":   accountIDs,
		"updated_ids":   updatedIDs,
		"updated_count": len(updatedIDs),
		"errors":        result.Errors,
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.BatchUpdateAccountsResponse{
		Data: apiopenapi.BatchUpdateAccountsResult{
			Errors:       result.Errors,
			UpdatedCount: len(updatedIDs),
			UpdatedIds:   updatedIDs,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminAccount(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	accountID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return
	}
	before, err := s.runtime.accounts.FindByID(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found", requestID)
		return
	}
	var body apiopenapi.UpdateProviderAccountRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid account update request", requestID)
		return
	}
	credential := optionalCredential(body.Credential)
	metadata := jsonObjectToMapPtr(body.Metadata)
	runtimeClass := before.RuntimeClass
	if body.RuntimeClass != nil {
		runtimeClass = accountcontract.RuntimeClass(*body.RuntimeClass)
	}
	if body.RuntimeClass != nil && runtimeClass != before.RuntimeClass {
		if provider, err := s.runtime.providers.FindByID(r.Context(), before.ProviderID); err == nil {
			if !accountRuntimeClassAllowed(provider.ConfigSchema, runtimeClass) {
				writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "authentication method not allowed for this provider", requestID)
				return
			}
		}
	}
	upstreamClient := before.UpstreamClient
	if body.UpstreamClient != nil {
		upstreamClient = body.UpstreamClient
	}
	proxyID := before.ProxyID
	if body.ProxyId != nil {
		proxyID = body.ProxyId
	}
	if credential != nil {
		effectiveMetadata := mergeAccountMetadata(before.Metadata, metadata)
		refreshed, err := s.refreshImportCredential(r.Context(), runtimeClass, upstreamClient, effectiveMetadata, proxyID, *credential)
		if err != nil {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "oauth refresh failed", requestID)
			return
		}
		credential = &refreshed
	}
	account, err := s.runtime.accounts.Update(r.Context(), accountID, accountcontract.UpdateRequest{
		Name:           body.Name,
		RuntimeClass:   toAccountRuntimeClassPtr(body.RuntimeClass),
		Credential:     credential,
		Metadata:       metadata,
		ProxyID:        optionalNullableString(body.ProxyId),
		Status:         toAccountStatusPtr(body.Status),
		Priority:       body.Priority,
		Weight:         body.Weight,
		RiskLevel:      stringPtrFromAPI(body.RiskLevel),
		UpstreamClient: optionalNullableString(body.UpstreamClient),
	})
	if err != nil {
		switch {
		case errors.Is(err, accountservice.ErrCredentialMissing), errors.Is(err, accountservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account update request", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to update account", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.update", "provider_account", strconv.Itoa(account.ID), accountAuditSnapshot(before), accountAuditSnapshot(account)))
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderAccountResponse{
		Data:      s.apiAccount(r.Context(), account),
		RequestId: requestID,
	})
}

func (s *Server) refreshImportCredential(ctx context.Context, runtimeClass accountcontract.RuntimeClass, upstreamClient *string, metadata map[string]any, proxyID *string, credential map[string]any) (map[string]any, error) {
	if !isRefreshTokenOnlyImportCredential(runtimeClass, upstreamClient, credential) {
		return credential, nil
	}
	resp, err := s.runtime.reverseProxy.Refresh(ctx, reverseproxycontract.RefreshRequest{
		Account: reverseproxycontract.AccountRuntime{
			RuntimeClass:   string(runtimeClass),
			UpstreamClient: upstreamClient,
			ProxyID:        proxyID,
			UserAgent:      mapString(metadata, "user_agent"),
			Metadata:       metadata,
			Credential:     credential,
		},
	})
	if err != nil {
		return nil, err
	}
	return resp.Credential, nil
}

func isRefreshTokenOnlyImportCredential(runtimeClass accountcontract.RuntimeClass, upstreamClient *string, credential map[string]any) bool {
	if runtimeClass != accountcontract.RuntimeClassOauthRefresh && runtimeClass != accountcontract.RuntimeClassOauthDeviceCode {
		return false
	}
	if !supportsRefreshTokenOnlyImport(upstreamClient) {
		return false
	}
	return mapString(credential, "refresh_token") != "" && mapString(credential, "access_token") == ""
}

func supportsRefreshTokenOnlyImport(upstreamClient *string) bool {
	if upstreamClient == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(*upstreamClient)) {
	case "codex_cli", "claude_code_cli", "antigravity_desktop", "antigravity":
		return true
	default:
		return false
	}
}

func mergeAccountMetadata(existing map[string]any, incoming *map[string]any) map[string]any {
	if incoming == nil && existing == nil {
		return nil
	}
	merged := make(map[string]any, len(existing))
	for key, value := range existing {
		merged[key] = value
	}
	if incoming == nil {
		return merged
	}
	for key, value := range *incoming {
		merged[key] = value
	}
	return merged
}

func (s *Server) handleDeleteAdminAccount(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	accountID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || accountID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return
	}
	before, err := s.runtime.accounts.FindByID(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found", requestID)
		return
	}
	if err := s.runtime.accounts.Delete(r.Context(), accountID); err != nil {
		switch {
		case errors.Is(err, accountservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to delete account", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "account.delete", "account", strconv.Itoa(accountID), accountAuditSnapshot(before), nil))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"id": accountID, "deleted": true},
		"request_id": requestID,
	})
}

func (s *Server) handleBindAdminAccountProxy(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	accountID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return
	}
	before, err := s.runtime.accounts.FindByID(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found", requestID)
		return
	}
	var body apiopenapi.BindProviderAccountProxyRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid account proxy request", requestID)
		return
	}
	account, err := s.runtime.accounts.BindProxy(r.Context(), accountID, body.ProxyId)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account proxy request", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.proxy_bind", "provider_account", strconv.Itoa(account.ID), accountAuditSnapshot(before), accountAuditSnapshot(account)))
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderAccountResponse{
		Data:      s.apiAccount(r.Context(), account),
		RequestId: requestID,
	})
}

func (s *Server) handleDisableAdminAccount(w http.ResponseWriter, r *http.Request) {
	s.handleSetAdminAccountStatus(w, r, accountcontract.StatusDisabled, "provider_account.disable")
}

func (s *Server) handleEnableAdminAccount(w http.ResponseWriter, r *http.Request) {
	s.handleSetAdminAccountStatus(w, r, accountcontract.StatusActive, "provider_account.enable")
}

func (s *Server) handleRecoverAdminAccount(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	accountID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return
	}
	before, err := s.runtime.accounts.FindByID(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found", requestID)
		return
	}
	account, err := s.runtime.accounts.Recover(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to recover account", requestID)
		return
	}
	s.runtime.recordAccountRecoverySnapshot(r.Context(), account)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.recover", "provider_account", strconv.Itoa(account.ID), accountAuditSnapshot(before), accountAuditSnapshot(account)))
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderAccountResponse{
		Data:      s.apiAccount(r.Context(), account),
		RequestId: requestID,
	})
}

func (s *Server) handleClearAdminAccountError(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	accountID, ok := adminPathID(w, r, requestID, "account")
	if !ok {
		return
	}
	before, err := s.runtime.accounts.FindByID(r.Context(), accountID)
	if err != nil {
		writeAccountServiceError(w, err, requestID)
		return
	}
	account, err := s.runtime.accounts.ClearErrorState(r.Context(), accountID)
	if err != nil {
		writeAccountServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAccountRecoverySnapshot(r.Context(), account)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.clear_error", "provider_account", strconv.Itoa(account.ID), accountAuditSnapshot(before), accountAuditSnapshot(account)))
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderAccountResponse{
		Data:      s.apiAccount(r.Context(), account),
		RequestId: requestID,
	})
}

func (s *Server) handleTestAdminAccount(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	accountID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return
	}
	account, err := s.runtime.accounts.FindByID(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found", requestID)
		return
	}
	provider, err := s.runtime.providers.FindByID(r.Context(), account.ProviderID)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "provider not found", requestID)
		return
	}
	opts := adminAccountTestOptions{}
	if r.Body != nil {
		var body struct {
			Mode    string `json:"mode"`
			Model   string `json:"model"`
			ModelID string `json:"model_id"`
			Prompt  string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account test request body", requestID)
			return
		} else {
			opts.Mode = strings.TrimSpace(body.Mode)
			opts.Model = strings.TrimSpace(firstNonEmpty(body.Model, body.ModelID))
			opts.Prompt = strings.TrimSpace(body.Prompt)
		}
	}
	if opts.Mode == "" {
		opts.Mode = strings.TrimSpace(r.URL.Query().Get("mode"))
	}
	if opts.Model == "" {
		opts.Model = strings.TrimSpace(firstNonEmpty(r.URL.Query().Get("model"), r.URL.Query().Get("model_id")))
	}
	if opts.Prompt == "" {
		opts.Prompt = strings.TrimSpace(r.URL.Query().Get("prompt"))
	}
	mode, ok := normalizeAdminAccountTestMode(opts.Mode)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account test mode", requestID)
		return
	}
	opts.Mode = mode
	startedAt := time.Now()
	result := s.runtime.testAccount(r.Context(), provider, account, startedAt, opts)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.test", "provider_account", strconv.Itoa(account.ID), nil, map[string]any{
		"ok":     result.Ok,
		"status": result.Status,
		"checks": result.Checks,
	}))
	s.runtime.recordAccountTestHealthSnapshot(r.Context(), account, result)
	writeJSONAny(w, http.StatusOK, apiopenapi.AdminTestResultResponse{
		Data:      result,
		RequestId: requestID,
	})
}

func (s *Server) handleDiscoverAdminAccountModels(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	accountID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || accountID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return
	}
	var body apiopenapi.DiscoverAccountModelsRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := s.decodeJSONBody(w, r, &body); err != nil {
			writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid model discovery request", requestID)
			return
		}
	}
	account, err := s.runtime.accounts.FindByID(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found", requestID)
		return
	}
	provider, err := s.runtime.providers.FindByID(r.Context(), account.ProviderID)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "provider not found", requestID)
		return
	}
	discovery, err := s.runtime.discoverAccountModels(r.Context(), provider, account, body)
	if err != nil {
		switch {
		case errors.Is(err, errModelDiscoveryUnsupported), errors.Is(err, errModelDiscoveryInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, err.Error(), requestID)
		case errors.Is(err, errModelDiscoveryAuth):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "account credential is not usable for model discovery", requestID)
		case errors.Is(err, errModelDiscoveryUpstream):
			writeStandardError(w, http.StatusBadGateway, apiopenapi.INTERNALERROR, "upstream model discovery failed", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to discover account models", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.discover_models", "provider_account", strconv.Itoa(account.ID), nil, map[string]any{
		"provider_id": account.ProviderID,
		"account_id":  account.ID,
		"source":      discovery.Source,
		"model_count": len(discovery.ModelIds),
		"persisted":   discovery.Persisted,
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountModelDiscoveryResponse{
		Data:      discovery,
		RequestId: requestID,
	})
}

func (s *Server) handleSetAdminAccountStatus(w http.ResponseWriter, r *http.Request, status accountcontract.Status, action string) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	accountID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return
	}
	before, err := s.runtime.accounts.FindByID(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found", requestID)
		return
	}
	account, err := s.runtime.accounts.Update(r.Context(), accountID, accountcontract.UpdateRequest{Status: &status})
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to update account status", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, action, "provider_account", strconv.Itoa(account.ID), accountAuditSnapshot(before), accountAuditSnapshot(account)))
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderAccountResponse{
		Data:      s.apiAccount(r.Context(), account),
		RequestId: requestID,
	})
}

func writeAccountServiceError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, accountservice.ErrInvalidInput), errors.Is(err, accountservice.ErrCredentialMissing):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account request", requestID)
	default:
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found", requestID)
	}
}

func (s *Server) handleAdminAccountsHealthSummary(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	accounts, err := s.runtime.accountStore.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list accounts", requestID)
		return
	}
	usageLogs, err := s.runtime.usage.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list usage logs", requestID)
		return
	}
	now := time.Now().UTC()
	accountIDs := make([]int, 0, len(accounts))
	for _, account := range accounts {
		accountIDs = append(accountIDs, account.ID)
	}
	latestHealthByAccount, err := s.runtime.accounts.LatestHealthSnapshotsByAccounts(r.Context(), accountIDs)
	if err != nil {
		latestHealthByAccount = nil
	}
	latestQuotasByAccount, err := s.runtime.accounts.LatestQuotaSnapshotsByAccounts(r.Context(), accountIDs)
	if err != nil {
		latestQuotasByAccount = nil
	}
	snapshots := make([]apiopenapi.AccountHealthSnapshot, 0, len(accounts))
	for _, account := range accounts {
		snap := buildAccountHealthSnapshot(account, usageLogsForAccount(usageLogs, account.ID), now)
		if latest, ok := latestHealthByAccount[account.ID]; ok {
			overlayAccountHealthSnapshot(&snap, latest)
		}
		if quotas := latestQuotasByAccount[account.ID]; len(quotas) > 0 {
			overlayAccountQuotaWindowsOnHealth(&snap, quotas)
			if constrained, ok := mostConstrainedRealQuotaSnapshot(quotas); ok {
				overlayAccountQuotaOnHealth(&snap, constrained)
			}
		}
		snapshots = append(snapshots, snap)
	}
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       snapshots,
		"request_id": requestID,
	})
}

func (s *Server) handleAdminAccountHealth(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	accountID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return
	}
	account, err := s.runtime.accounts.FindByID(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found", requestID)
		return
	}
	usageLogs, err := s.runtime.usage.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list usage logs", requestID)
		return
	}
	snapshot := buildAccountHealthSnapshot(account, usageLogsForAccount(usageLogs, account.ID), time.Now().UTC())
	if latest, err := s.runtime.accounts.LatestHealthSnapshotByAccount(r.Context(), account.ID); err == nil {
		overlayAccountHealthSnapshot(&snapshot, latest)
	}
	if quotas, err := s.runtime.accounts.ListQuotaSnapshotsByAccount(r.Context(), account.ID, 1); err == nil && len(quotas) > 0 {
		overlayAccountQuotaWindowsOnHealth(&snapshot, quotas)
		if constrained, ok := mostConstrainedRealQuotaSnapshot(quotas); ok {
			overlayAccountQuotaOnHealth(&snapshot, constrained)
		}
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountHealthResponse{
		Data:      snapshot,
		RequestId: requestID,
	})
}

func (s *Server) handleAdminAccountQuota(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	accountID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return
	}
	account, err := s.runtime.accounts.FindByID(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found", requestID)
		return
	}
	usageLogs, err := s.runtime.usage.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list usage logs", requestID)
		return
	}
	snapshots, err := s.runtime.accounts.ListQuotaSnapshotsByAccount(r.Context(), account.ID, 50)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list account quota snapshots", requestID)
		return
	}
	data := make([]apiopenapi.AccountQuotaSnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		data = append(data, toAPIAccountQuotaSnapshot(snapshot))
	}
	if len(data) == 0 {
		data = append(data, buildAccountQuotaSnapshot(account, usageLogsForAccount(usageLogs, account.ID), time.Now().UTC()))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountQuotaListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleListAdminAccountGroups(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	groups, err := s.runtime.accounts.ListGroups(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list account groups", requestID)
		return
	}
	data := make([]apiopenapi.AccountGroup, 0, len(groups))
	for _, group := range groups {
		data = append(data, toAPIAccountGroup(group))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountGroupListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminAccountGroup(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	var body apiopenapi.CreateAccountGroupRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid account group request", requestID)
		return
	}
	description := ""
	if body.Description != nil {
		description = *body.Description
	}
	group, err := s.runtime.accounts.CreateGroup(r.Context(), accountcontract.CreateGroupRequest{
		Name:          body.Name,
		Description:   description,
		ProviderScope: jsonObjectToMap(body.ProviderScope),
		ModelScope:    jsonObjectToMap(body.ModelScope),
		StrategyHint:  body.StrategyHint,
		Status:        toAccountGroupStatusPtr(body.Status),
	})
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account group request", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "account_group.create", "account_group", strconv.Itoa(group.ID), nil, accountGroupAuditSnapshot(group)))
	writeJSONAny(w, http.StatusCreated, apiopenapi.AccountGroupResponse{
		Data:      toAPIAccountGroup(group),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminAccountGroup(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	groupID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account group id", requestID)
		return
	}
	before, err := s.runtime.accounts.FindGroupByID(r.Context(), groupID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account group not found", requestID)
		return
	}
	var body apiopenapi.UpdateAccountGroupRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid account group update request", requestID)
		return
	}
	group, err := s.runtime.accounts.UpdateGroup(r.Context(), groupID, accountcontract.UpdateGroupRequest{
		Name:          body.Name,
		Description:   body.Description,
		ProviderScope: jsonObjectToMapPtr(body.ProviderScope),
		ModelScope:    jsonObjectToMapPtr(body.ModelScope),
		StrategyHint:  body.StrategyHint,
		Status:        toAccountGroupStatusPtr(body.Status),
	})
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account group update request", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "account_group.update", "account_group", strconv.Itoa(group.ID), accountGroupAuditSnapshot(before), accountGroupAuditSnapshot(group)))
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountGroupResponse{
		Data:      toAPIAccountGroup(group),
		RequestId: requestID,
	})
}

func (s *Server) handleDeleteAdminAccountGroup(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	groupID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || groupID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account group id", requestID)
		return
	}
	before, err := s.runtime.accounts.FindGroupByID(r.Context(), groupID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account group not found", requestID)
		return
	}
	if err := s.runtime.accounts.DeleteGroup(r.Context(), groupID); err != nil {
		switch {
		case errors.Is(err, accountservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account group id", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to delete account group", requestID)
		}
		return
	}
	// Cascade the group's rate-limit policy (cross-module). A group with no rate
	// limit configured is the common case, so a not-found result is not an error.
	if err := s.runtime.groupRateLimits.DeleteLimit(r.Context(), groupID); err != nil && !errors.Is(err, groupratelimitscontract.ErrNotFound) {
		s.logger.Warn("failed to clear group rate limit on group delete", "group_id", groupID, "error", err)
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "account_group.delete", "account_group", strconv.Itoa(groupID), accountGroupAuditSnapshot(before), nil))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"id": groupID, "deleted": true},
		"request_id": requestID,
	})
}

func (s *Server) handleListAdminAccountGroupMembers(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	groupID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || groupID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account group id", requestID)
		return
	}
	if _, err := s.runtime.accounts.FindGroupByID(r.Context(), groupID); err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account group not found", requestID)
		return
	}
	members, err := s.runtime.accounts.ListGroupMembers(r.Context(), groupID)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list account group members", requestID)
		return
	}
	data := make([]apiopenapi.AccountGroupMember, 0, len(members))
	for _, member := range members {
		data = append(data, toAPIAccountGroupMember(member))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountGroupMemberListResponse{
		Data:      data,
		RequestId: requestID,
	})
}

func (s *Server) handleAddAdminAccountGroupMember(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	groupID, accountID, ok := accountGroupMemberPathIDs(w, r, requestID)
	if !ok {
		return
	}
	member, err := s.runtime.accounts.AddAccountToGroup(r.Context(), accountID, groupID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account or group not found", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "account_group.member_add", "account_group", strconv.Itoa(groupID), nil, map[string]any{
		"account_id": accountID,
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountGroupMemberResponse{
		Data:      toAPIAccountGroupMember(member),
		RequestId: requestID,
	})
}

func (s *Server) handleRemoveAdminAccountGroupMember(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	groupID, accountID, ok := accountGroupMemberPathIDs(w, r, requestID)
	if !ok {
		return
	}
	if err := s.runtime.accounts.RemoveAccountFromGroup(r.Context(), accountID, groupID); err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to remove account group membership", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "account_group.member_remove", "account_group", strconv.Itoa(groupID), map[string]any{
		"account_id": accountID,
	}, nil))
	w.WriteHeader(http.StatusNoContent)
}
