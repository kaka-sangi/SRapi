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
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	modelservice "github.com/srapi/srapi/apps/api/internal/modules/models/service"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	providerpreset "github.com/srapi/srapi/apps/api/internal/modules/providers/preset"
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
		s.logger.Error("failed to list providers for admin overview", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list providers", requestID)
		return
	}
	models, err := s.runtime.models.List(r.Context())
	if err != nil {
		s.logger.Error("failed to list models for admin overview", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list models", requestID)
		return
	}
	accounts, err := s.runtime.accounts.List(r.Context())
	if err != nil {
		s.logger.Error("failed to list accounts for admin overview", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list accounts", requestID)
		return
	}
	usageLogs, err := s.runtime.usage.List(r.Context())
	if err != nil {
		s.logger.Error("failed to list usage logs for admin overview", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list usage logs", requestID)
		return
	}
	decisions, err := s.runtime.scheduler.ListDecisions(r.Context())
	if err != nil {
		s.logger.Error("failed to list scheduler decisions for admin overview", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list scheduler decisions", requestID)
		return
	}
	users, err := s.runtime.users.List(r.Context(), usersservice.ListRequest{})
	if err != nil {
		s.logger.Error("failed to list users for admin overview", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list users", requestID)
		return
	}
	dailyAggregates, err := s.runtime.usage.Aggregate(r.Context(), usagecontract.QueryFilter{}, usagecontract.AggregateDimensionDay)
	if err != nil {
		s.logger.Error("failed to aggregate usage for admin overview", "error", err, "request_id", requestID)
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
		s.logger.Error("failed to list providers", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list providers", requestID)
		return
	}
	providers = filterProviders(providers, r.URL.Query().Get("status"), r.URL.Query().Get("q"))
	data := make([]apiopenapi.Provider, 0, len(providers))
	for _, provider := range providers {
		data = append(data, toAPIProvider(provider))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderListResponse{
		Data:       data,
		Pagination: pg,
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
			s.logger.Error("failed to create provider", "error", err, "request_id", requestID)
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
			s.logger.Error("failed to update provider", "error", err, "provider_id", providerID, "request_id", requestID)
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
		s.logger.Error("failed to check provider accounts before delete", "error", err, "provider_id", providerID, "request_id", requestID)
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
	// Cascade-delete model_provider_mappings that reference this provider so
	// they don't orphan once the provider row is gone (H11).
	if mappings, err := s.runtime.models.ListMappings(r.Context()); err == nil {
		for _, mapping := range mappings {
			if mapping.ProviderID == providerID {
				if delErr := s.runtime.models.DeleteMapping(r.Context(), mapping.ModelID, mapping.ID); delErr != nil {
					s.logger.Error("failed to cascade-delete model provider mapping", "mapping_id", mapping.ID, "model_id", mapping.ModelID, "provider_id", providerID, "error", delErr, "request_id", requestID)
				}
			}
		}
	} else {
		s.logger.Error("failed to list model mappings for cascade delete", "provider_id", providerID, "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to cascade delete model mappings", requestID)
		return
	}
	if err := s.runtime.providers.Delete(r.Context(), providerID); err != nil {
		switch {
		case errors.Is(err, providerservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid provider id", requestID)
		default:
			s.logger.Error("failed to delete provider", "error", err, "provider_id", providerID, "request_id", requestID)
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to delete provider", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider.delete", "provider", strconv.Itoa(providerID), providerAuditSnapshot(before), nil))
	writeJSONAny(w, http.StatusOK, deleteResponse(true, requestID))
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
		s.logger.Error("failed to list models", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list models", requestID)
		return
	}
	models = filterModels(models, r.URL.Query().Get("status"), r.URL.Query().Get("q"))
	data := make([]apiopenapi.Model, 0, len(models))
	for _, model := range models {
		data = append(data, toAPIModel(model))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.ModelListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleListAdminModelMappingsAll(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	mappings, err := s.runtime.models.ListMappings(r.Context())
	if err != nil {
		s.logger.Error("failed to list model mappings", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list model mappings", requestID)
		return
	}
	mappings = filterModelMappings(mappings, r.URL.Query().Get("status"))
	data := make([]apiopenapi.ModelProviderMapping, 0, len(mappings))
	for _, mapping := range mappings {
		data = append(data, toAPIModelProviderMapping(mapping))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.ModelProviderMappingPagedListResponse{
		Data:       data,
		Pagination: pg,
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
			s.logger.Error("failed to create model", "error", err, "request_id", requestID)
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
			s.logger.Error("failed to update model", "error", err, "model_id", modelID, "request_id", requestID)
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
			s.logger.Error("failed to delete model", "error", err, "model_id", modelID, "request_id", requestID)
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to delete model", requestID)
		}
		return
	}
	s.runtime.invalidateModelCache(before.CanonicalName)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "model.delete", "model", strconv.Itoa(modelID), modelAuditSnapshot(before), nil))
	writeJSONAny(w, http.StatusOK, deleteResponse(true, requestID))
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
			s.logger.Error("failed to create model alias", "error", err, "model_id", modelID, "request_id", requestID)
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
			s.logger.Error("failed to create model mapping", "error", err, "model_id", modelID, "provider_id", providerID, "request_id", requestID)
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
			s.logger.Error("failed to list model aliases", "error", err, "model_id", modelID, "request_id", requestID)
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
			s.logger.Error("failed to delete model alias", "error", err, "model_id", modelID, "alias_id", aliasID, "request_id", requestID)
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to delete model alias", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "model_alias.delete", "model_alias", strconv.Itoa(aliasID), map[string]any{"model_id": modelID}, nil))
	writeJSONAny(w, http.StatusOK, deleteResponse(true, requestID))
}

func (s *Server) handleUpdateAdminModelAlias(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.UpdateModelAliasRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid model alias update request", requestID)
		return
	}
	before, err := s.runtime.models.FindAliasByID(r.Context(), aliasID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "model alias not found", requestID)
		return
	}
	alias, err := s.runtime.models.UpdateAlias(r.Context(), modelID, aliasID, modelcontract.UpdateAliasRequest{
		Alias:          body.Alias,
		StrategyHint:   optionalNullableString(body.StrategyHint),
		FallbackModels: body.FallbackModels,
		Status:         toModelStatusPtr(body.Status),
	})
	if err != nil {
		switch {
		case errors.Is(err, modelservice.ErrAliasNotFound):
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "model alias not found", requestID)
		case errors.Is(err, modelservice.ErrAliasExists):
			writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "model alias already exists", requestID)
		case errors.Is(err, modelservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model alias update request", requestID)
		default:
			s.logger.Error("failed to update model alias", "error", err, "model_id", modelID, "alias_id", aliasID, "request_id", requestID)
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to update model alias", requestID)
		}
		return
	}
	// Invalidate both the old alias string (if it changed) and the new one.
	s.runtime.invalidateModelCache(before.Alias, alias.Alias)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "model_alias.update", "model_alias", strconv.Itoa(alias.ID), map[string]any{
		"alias":           before.Alias,
		"fallback_models": before.FallbackModels,
		"status":          before.Status,
	}, map[string]any{
		"alias":           alias.Alias,
		"fallback_models": alias.FallbackModels,
		"status":          alias.Status,
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.ModelAliasResponse{
		Data:      toAPIModelAlias(alias),
		RequestId: requestID,
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
			s.logger.Error("failed to list model mappings by model", "error", err, "model_id", modelID, "request_id", requestID)
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
			s.logger.Error("failed to delete model mapping", "error", err, "model_id", modelID, "mapping_id", mappingID, "request_id", requestID)
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to delete model mapping", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "model_provider_mapping.delete", "model_provider_mapping", strconv.Itoa(mappingID), map[string]any{"model_id": modelID}, nil))
	writeJSONAny(w, http.StatusOK, deleteResponse(true, requestID))
}

func (s *Server) handleUpdateAdminModelMapping(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.UpdateModelProviderMappingRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid model mapping update request", requestID)
		return
	}
	before, err := s.runtime.models.FindMappingByID(r.Context(), mappingID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "model provider mapping not found", requestID)
		return
	}
	mapping, err := s.runtime.models.UpdateMapping(r.Context(), modelID, mappingID, modelcontract.UpdateMappingRequest{
		UpstreamModelName:  body.UpstreamModelName,
		Status:             toModelStatusPtr(body.Status),
		CapabilityOverride: toCapabilityDescriptorsPtrContract(body.CapabilityOverride),
		PricingOverride:    jsonObjectToMapPtr(body.PricingOverride),
	})
	if err != nil {
		switch {
		case errors.Is(err, modelservice.ErrMappingNotFound):
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "model provider mapping not found", requestID)
		case errors.Is(err, modelservice.ErrMappingExists):
			writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "model provider mapping already exists", requestID)
		case errors.Is(err, modelservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model mapping update request", requestID)
		default:
			s.logger.Error("failed to update model mapping", "error", err, "model_id", modelID, "mapping_id", mappingID, "request_id", requestID)
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to update model mapping", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "model_provider_mapping.update", "model_provider_mapping", strconv.Itoa(mapping.ID), map[string]any{
		"upstream_model_name": before.UpstreamModelName,
		"status":              before.Status,
	}, map[string]any{
		"upstream_model_name": mapping.UpstreamModelName,
		"status":              mapping.Status,
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.ModelProviderMappingResponse{
		Data:      toAPIModelProviderMapping(mapping),
		RequestId: requestID,
	})
}

// handleListAdminAccounts serves GET /api/v1/admin/accounts. Filtering
// (status / provider_id / runtime_class / search / group_id), ORDER BY id
// DESC, and LIMIT/OFFSET all run in the database via accounts.ListPage — the
// prior path loaded every provider_account row before filtering and slicing in
// Go memory, which dominated wall-clock once the fleet grew past a few
// hundred accounts.
func (s *Server) handleListAdminAccounts(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	filter, ok := accountListFilterFromRequest(r, requestID, w)
	if !ok {
		return
	}
	limit, offset, page, pageSize := paginationParams(r)
	result, err := s.runtime.accounts.ListPage(r.Context(), filter, limit, offset)
	if err != nil {
		s.logger.Error("failed to list accounts", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list accounts", requestID)
		return
	}
	data := make([]apiopenapi.ProviderAccount, 0, len(result.Items))
	for _, account := range result.Items {
		data = append(data, s.apiAccount(r.Context(), account))
	}
	var pg apiopenapi.Pagination
	if pageSize == 0 {
		// No page_size sent — the legacy "give me everything" shape. Total ==
		// returned len so callers that want the full list (e.g. dropdowns)
		// keep working without paging metadata changes.
		pg = apiopenapi.Pagination{Page: 1, PageSize: len(data), Total: result.Total, HasNext: false}
	} else {
		pg = paginationFromTotal(result.Total, page, pageSize)
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderAccountListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

// accountListFilterFromRequest parses ?status, ?provider_id, ?runtime_class,
// ?search, ?group_id into the typed contract filter. Returns (_, false) and
// writes a 400 when a numeric field is malformed so the caller can short
// circuit.
func accountListFilterFromRequest(r *http.Request, requestID string, w http.ResponseWriter) (accountcontract.ListFilter, bool) {
	q := r.URL.Query()
	filter := accountcontract.ListFilter{
		Status:       accountcontract.Status(strings.TrimSpace(q.Get("status"))),
		RuntimeClass: accountcontract.RuntimeClass(strings.TrimSpace(q.Get("runtime_class"))),
		Search:       strings.TrimSpace(q.Get("search")),
	}
	if raw := strings.TrimSpace(q.Get("provider_id")); raw != "" {
		pid, err := strconv.Atoi(raw)
		if err != nil || pid <= 0 {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid provider id", requestID)
			return accountcontract.ListFilter{}, false
		}
		filter.ProviderID = &pid
	}
	if raw := strings.TrimSpace(q.Get("group_id")); raw != "" {
		gid, err := strconv.Atoi(raw)
		if err != nil || gid <= 0 {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid group id", requestID)
			return accountcontract.ListFilter{}, false
		}
		filter.GroupID = &gid
	}
	return filter, true
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

// applyProviderTemplateMetadata fills account metadata with the provider preset's
// AccountTemplate.DefaultMetadata for any key the caller did not set, so accounts
// created via the plain create / generic-import paths carry the same defaults
// (e.g. base_url) that quick-setup already applies. Caller-provided values always
// win. This keeps the account-creation paths from drifting apart.
func applyProviderTemplateMetadata(provider providercontract.Provider, userMeta map[string]any) map[string]any {
	defaults := providerTemplateDefaultMetadata(provider)
	if len(defaults) == 0 {
		return userMeta
	}
	merged := make(map[string]any, len(defaults)+len(userMeta))
	for k, v := range defaults {
		merged[k] = v
	}
	for k, v := range userMeta {
		merged[k] = v
	}
	return merged
}

func providerTemplateDefaultMetadata(provider providercontract.Provider) map[string]any {
	// Preset-installed providers carry the template under config_schema.
	if at, ok := provider.ConfigSchema["account_template"].(map[string]any); ok {
		if dm, ok := at["default_metadata"].(map[string]any); ok && len(dm) > 0 {
			return dm
		}
	}
	// Fall back to the registry preset matched by provider key or adapter type
	// (covers manually-created / custom-named providers whose config_schema was
	// not seeded from the preset).
	if preset, ok := presetForProvider(provider); ok && preset.AccountTemplate != nil {
		return preset.AccountTemplate.DefaultMetadata
	}
	return nil
}

func platformFromProvider(provider providercontract.Provider) string {
	at := strings.ToLower(strings.TrimSpace(provider.AdapterType))
	name := strings.ToLower(strings.TrimSpace(provider.Name))
	switch {
	case strings.Contains(at, "anthropic") || strings.Contains(name, "anthropic") || strings.Contains(name, "claude"):
		return "anthropic"
	case strings.Contains(at, "openai") || strings.Contains(name, "openai") || strings.Contains(at, "codex"):
		return "openai"
	case strings.Contains(at, "gemini") || strings.Contains(name, "gemini") || strings.Contains(at, "vertex"):
		return "gemini"
	case strings.Contains(at, "antigravity") || strings.Contains(name, "antigravity"):
		return "antigravity"
	default:
		return ""
	}
}

func presetForProvider(provider providercontract.Provider) (providerpreset.Preset, bool) {
	if p, ok := providerpreset.Default().Lookup(provider.Name); ok {
		return p, true
	}
	switch strings.ToLower(strings.TrimSpace(provider.AdapterType)) {
	case "reverse-proxy-codex-cli":
		return providerpreset.Default().Lookup("codex-cli")
	case "reverse-proxy-chatgpt-web":
		return providerpreset.Default().Lookup("chatgpt-web")
	case "reverse-proxy-antigravity":
		return providerpreset.Default().Lookup("antigravity")
	}
	return providerpreset.Preset{}, false
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
	metadata := applyProviderTemplateMetadata(provider, jsonObjectToMap(body.Metadata))
	credential, err = s.refreshImportCredential(r.Context(), accountcontract.RuntimeClass(body.RuntimeClass), body.UpstreamClient, metadata, body.ProxyId, credential)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "oauth refresh failed", requestID)
		return
	}
	var groupIDs []int
	if body.GroupIds != nil {
		groupIDs = *body.GroupIds
	}
	var rateMultiplier *float64
	if body.RateMultiplier != nil {
		v := float64(*body.RateMultiplier)
		rateMultiplier = &v
	}
	account, err := s.runtime.accounts.Create(r.Context(), accountcontract.CreateRequest{
		ProviderID:         providerID,
		Name:               body.Name,
		Platform:           platformFromProvider(provider),
		RuntimeClass:       accountcontract.RuntimeClass(body.RuntimeClass),
		Credential:         credential,
		Metadata:           metadata,
		Extra:              jsonObjectToMap(body.Extra),
		ProxyID:            body.ProxyId,
		Status:             toAccountStatusPtr(body.Status),
		Priority:           body.Priority,
		Weight:             body.Weight,
		RiskLevel:          stringPtrFromAPI(body.RiskLevel),
		UpstreamClient:     body.UpstreamClient,
		Notes:              body.Notes,
		Concurrency:        body.Concurrency,
		RateMultiplier:     rateMultiplier,
		LoadFactor:         body.LoadFactor,
		GroupIDs:           groupIDs,
		ExpiresAt:          body.ExpiresAt,
		AutoPauseOnExpired: body.AutoPauseOnExpired,
	})
	if err != nil {
		var mixedErr *accountservice.MixedChannelError
		switch {
		case errors.As(err, &mixedErr):
			writeJSONAny(w, http.StatusConflict, map[string]any{
				"error":   "mixed_channel_warning",
				"message": mixedErr.Error(),
			})
		case errors.Is(err, accountservice.ErrCredentialMissing), errors.Is(err, accountservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account request", requestID)
		default:
			s.logger.Error("failed to create account", "error", err, "provider_id", providerID, "request_id", requestID)
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

// handleBatchCreateAdminAccount bulk-creates provider accounts in one call
// against a shared defaults set (provider, runtime_class, …). Dedupes by
// name within the batch + against existing accounts; per-row failures
// surface in `results[].error` and the rest still apply. Pattern mirrors
// handleBatchCreateAdminProxies — one place to look when this needs to
// evolve.
func (s *Server) handleBatchCreateAdminAccount(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.BatchCreateProviderAccountsRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid batch account create request", requestID)
		return
	}
	if len(body.Items) == 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "items must be non-empty", requestID)
		return
	}
	if len(body.Items) > accountservice.BatchCreateAccountsMaxItems {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "too many items", requestID)
		return
	}
	providerID, err := strconv.Atoi(string(body.Defaults.ProviderId))
	if err != nil || providerID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid provider id", requestID)
		return
	}
	provider, err := s.runtime.providers.FindByID(r.Context(), providerID)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "provider not found", requestID)
		return
	}
	runtimeClass := accountcontract.RuntimeClass(body.Defaults.RuntimeClass)
	if !accountRuntimeClassAllowed(provider.ConfigSchema, runtimeClass) {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "authentication method not allowed for this provider", requestID)
		return
	}
	// Apply the provider's account_template (e.g. default base_url) once to the
	// shared metadata so every row inherits it without per-row work; the service
	// clones the map before persisting per-row.
	defaultsMetadata := applyProviderTemplateMetadata(provider, jsonObjectToMap(body.Defaults.Metadata))
	var defaultsRiskLevel *string
	if body.Defaults.RiskLevel != nil {
		rl := string(*body.Defaults.RiskLevel)
		defaultsRiskLevel = &rl
	}
	defaults := accountcontract.BatchCreateAccountsDefaults{
		ProviderID:     providerID,
		RuntimeClass:   runtimeClass,
		UpstreamClient: body.Defaults.UpstreamClient,
		GroupID:        body.Defaults.GroupId,
		ProxyID:        body.Defaults.ProxyId,
		Priority:       body.Defaults.Priority,
		Weight:         body.Defaults.Weight,
		RiskLevel:      defaultsRiskLevel,
		Metadata:       defaultsMetadata,
	}
	items := make([]accountcontract.BatchAccountItem, 0, len(body.Items))
	for _, item := range body.Items {
		credential := map[string]any{}
		if item.Credential != nil {
			credential = *item.Credential
		}
		items = append(items, accountcontract.BatchAccountItem{
			Name:       item.Name,
			Credential: credential,
			GroupID:    item.GroupId,
			Priority:   item.Priority,
			Weight:     item.Weight,
		})
	}
	results, err := s.runtime.accounts.BatchCreateAccounts(r.Context(), defaults, items)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid batch account create request", requestID)
		return
	}
	apiResults := make([]apiopenapi.BatchCreateProviderAccountsResultRow, 0, len(results))
	succeeded := 0
	failed := 0
	firstError := ""
	for _, row := range results {
		apiRow := apiopenapi.BatchCreateProviderAccountsResultRow{
			Index: row.Index,
			Name:  row.Name,
		}
		if row.AccountID != nil {
			id := apiopenapi.Id(strconv.Itoa(*row.AccountID))
			apiRow.AccountId = &id
		}
		if row.Error != "" {
			err := row.Error
			apiRow.Error = &err
			failed++
			if firstError == "" {
				firstError = err
			}
		} else if row.AccountID != nil {
			succeeded++
		}
		apiResults = append(apiResults, apiRow)
	}
	// Audit snapshot intentionally excludes per-row credentials. Defaults
	// summary records the provider/runtime_class/group binding only — never
	// the credential map.
	auditDelta := map[string]any{
		"requested":     len(body.Items),
		"succeeded":     succeeded,
		"failed":        failed,
		"provider_id":   providerID,
		"runtime_class": string(runtimeClass),
	}
	if defaults.GroupID != nil {
		auditDelta["group_id"] = *defaults.GroupID
	}
	if firstError != "" {
		auditDelta["first_error"] = firstError
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.batch_create", "provider_account", "bulk", nil, auditDelta))
	writeJSONAny(w, http.StatusOK, apiopenapi.BatchCreateProviderAccountsResponse{
		Data: apiopenapi.BatchCreateProviderAccountsResult{
			Results:   apiResults,
			Succeeded: succeeded,
			Failed:    failed,
		},
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
		s.logger.Error("failed to export accounts", "error", err, "request_id", requestID)
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
	// Build the partial UpdateRequest from the body. Status is required by
	// the spec (kept for back-compat with the original status-only callers);
	// priority/weight/risk_level were added in the widening pass and are
	// applied only when present.
	status := accountcontract.Status(body.Status)
	updateReq := accountcontract.UpdateRequest{Status: &status}
	if body.Priority != nil {
		p := *body.Priority
		updateReq.Priority = &p
	}
	if body.Weight != nil {
		w := *body.Weight
		updateReq.Weight = &w
	}
	if body.RiskLevel != nil {
		rl := *body.RiskLevel
		updateReq.RiskLevel = &rl
	}
	result := s.runtime.accounts.BatchUpdateFields(r.Context(), accountIDs, updateReq)
	updatedIDs := make([]apiopenapi.Id, 0, len(result.Updated))
	for _, updated := range result.Updated {
		updatedIDs = append(updatedIDs, apiopenapi.Id(strconv.Itoa(updated.ID)))
	}
	auditDelta := map[string]any{
		"account_ids":   accountIDs,
		"status":        body.Status,
		"updated_ids":   updatedIDs,
		"updated_count": len(updatedIDs),
		"errors":        result.Errors,
	}
	if body.Priority != nil {
		auditDelta["priority"] = *body.Priority
	}
	if body.Weight != nil {
		auditDelta["weight"] = *body.Weight
	}
	if body.RiskLevel != nil {
		auditDelta["risk_level"] = *body.RiskLevel
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.batch_update", "provider_account", "bulk", nil, auditDelta))
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
	var updateRateMultiplier *float64
	if body.RateMultiplier != nil {
		v := float64(*body.RateMultiplier)
		updateRateMultiplier = &v
	}
	account, err := s.runtime.accounts.Update(r.Context(), accountID, accountcontract.UpdateRequest{
		Name:               body.Name,
		RuntimeClass:       toAccountRuntimeClassPtr(body.RuntimeClass),
		Credential:         credential,
		Metadata:           metadata,
		Extra:              jsonObjectToMapPtr(body.Extra),
		ProxyID:            optionalNullableString(body.ProxyId),
		Status:             toAccountStatusPtr(body.Status),
		Priority:           body.Priority,
		Weight:             body.Weight,
		RiskLevel:          stringPtrFromAPI(body.RiskLevel),
		UpstreamClient:     optionalNullableString(body.UpstreamClient),
		Notes:              body.Notes,
		Concurrency:        body.Concurrency,
		RateMultiplier:     updateRateMultiplier,
		LoadFactor:         optionalNullableInt(body.LoadFactor),
		ExpiresAt:          body.ExpiresAt,
		AutoPauseOnExpired: body.AutoPauseOnExpired,
		Schedulable:        body.Schedulable,
	})
	if err != nil {
		switch {
		case errors.Is(err, accountservice.ErrCredentialMissing), errors.Is(err, accountservice.ErrInvalidInput), errors.Is(err, accountservice.ErrProxyUnavailable):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account update request", requestID)
		default:
			s.logger.Error("failed to update account", "error", err, "account_id", accountID, "request_id", requestID)
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
	runtimeProxyID, err := s.runtime.accounts.ResolveProxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	resp, err := s.runtime.reverseProxy.Refresh(ctx, reverseproxycontract.RefreshRequest{
		Account: reverseproxycontract.AccountRuntime{
			RuntimeClass:   string(runtimeClass),
			UpstreamClient: upstreamClient,
			ProxyID:        runtimeProxyID,
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
	case "codex_cli", "chatgpt_web", "claude_code_cli", "antigravity_desktop", "antigravity":
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
			s.logger.Error("failed to delete account", "error", err, "account_id", accountID, "request_id", requestID)
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to delete account", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "account.delete", "account", strconv.Itoa(accountID), accountAuditSnapshot(before), nil))
	writeJSONAny(w, http.StatusOK, deleteResponse(true, requestID))
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
		s.logger.Error("failed to recover account", "error", err, "account_id", accountID, "request_id", requestID)
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

// adminAccountRefresherAdapter bridges the reverse-proxy Refresher into the
// accounts service's AccountRefresher contract for the on-demand
// /admin/accounts/{id}/refresh path. The proactive worker uses an identical
// adapter; this one is inline so the handler does not pull in the worker
// package just for the adapter type.
type adminAccountRefresherAdapter struct {
	refresher reverseproxycontract.Refresher
	accounts  *accountservice.Service
}

func (a adminAccountRefresherAdapter) RefreshAccount(ctx context.Context, req accountservice.RefreshRequest) (accountservice.RefreshResult, error) {
	proxyID := req.ProxyID
	if a.accounts != nil {
		resolved, err := a.accounts.ResolveProxyURL(ctx, req.ProxyID)
		if err != nil {
			return accountservice.RefreshResult{}, err
		}
		proxyID = resolved
	}
	resp, err := a.refresher.Refresh(ctx, reverseproxycontract.RefreshRequest{
		Account: reverseproxycontract.AccountRuntime{
			AccountID:      req.AccountID,
			RuntimeClass:   string(req.RuntimeClass),
			UpstreamClient: req.UpstreamClient,
			ProxyID:        proxyID,
			UserAgent:      mapString(req.Metadata, "user_agent"),
			Metadata:       req.Metadata,
			Credential:     req.Credential,
		},
	})
	if err != nil {
		return accountservice.RefreshResult{}, err
	}
	return accountservice.RefreshResult{Credential: resp.Credential}, nil
}

func (s *Server) handleRefreshAdminAccount(w http.ResponseWriter, r *http.Request) {
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
	if before.RuntimeClass != accountcontract.RuntimeClassOauthRefresh && before.RuntimeClass != accountcontract.RuntimeClassOauthDeviceCode {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "account is not an oauth runtime class", requestID)
		return
	}
	if s.runtime.reverseProxy == nil {
		s.logger.Error("reverse proxy refresher unavailable for account refresh", "account_id", accountID, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "reverse proxy refresher unavailable", requestID)
		return
	}
	adapter := adminAccountRefresherAdapter{refresher: s.runtime.reverseProxy, accounts: s.runtime.accounts}
	outcome, refreshErr := s.runtime.accounts.RefreshAccessTokenWithOutcome(r.Context(), accountID, adapter)
	updated := outcome.Account
	auditOutcome := map[string]any{"ok": refreshErr == nil, "class": string(outcome.Class), "attempts": outcome.Attempts, "needs_reauth_flipped": outcome.NeedsReauthFlipped}
	if refreshErr != nil {
		auditOutcome["error"] = refreshErr.Error()
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "account.refresh_token", "provider_account", strconv.Itoa(accountID), accountAuditSnapshot(before), auditOutcome))
	if refreshErr != nil {
		// The accounts service has already persisted the failure (incremented
		// refresh_attempts, captured the error, flipped needs_reauth_at when
		// permanent / threshold-crossed). Surface 502 so the UI can show a
		// "refresh failed" toast; the row will reflect the new state on
		// re-fetch.
		writeStandardError(w, http.StatusBadGateway, apiopenapi.INTERNALERROR, "oauth refresh failed: "+refreshErr.Error(), requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderAccountResponse{
		Data:      s.apiAccount(r.Context(), updated),
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
			s.logger.Error("failed to discover account models", "error", err, "account_id", accountID, "request_id", requestID)
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
		s.logger.Error("failed to update account status", "error", err, "account_id", accountID, "status", status, "request_id", requestID)
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
	case errors.Is(err, accountservice.ErrInvalidInput), errors.Is(err, accountservice.ErrCredentialMissing), errors.Is(err, accountservice.ErrProxyUnavailable):
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
		s.logger.Error("failed to list accounts for health summary", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list accounts", requestID)
		return
	}
	usageLogs, err := s.runtime.usage.List(r.Context())
	if err != nil {
		s.logger.Error("failed to list usage logs for health summary", "error", err, "request_id", requestID)
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
			if constrained, ok := mostConstrainedActiveRealQuotaSnapshot(quotas); ok {
				overlayAccountQuotaOnHealth(&snap, constrained)
			}
		}
		snapshots = append(snapshots, snap)
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountHealthSummaryResponse{
		Data:      snapshots,
		RequestId: requestID,
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
		s.logger.Error("failed to list usage logs for account health", "error", err, "account_id", accountID, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list usage logs", requestID)
		return
	}
	snapshot := buildAccountHealthSnapshot(account, usageLogsForAccount(usageLogs, account.ID), time.Now().UTC())
	if latest, err := s.runtime.accounts.LatestHealthSnapshotByAccount(r.Context(), account.ID); err == nil {
		overlayAccountHealthSnapshot(&snapshot, latest)
	}
	if quotas, err := s.runtime.accounts.ListQuotaSnapshotsByAccount(r.Context(), account.ID, 1); err == nil && len(quotas) > 0 {
		overlayAccountQuotaWindowsOnHealth(&snapshot, quotas)
		if constrained, ok := mostConstrainedActiveRealQuotaSnapshot(quotas); ok {
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
		s.logger.Error("failed to list usage logs for account quota", "error", err, "account_id", account.ID, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list usage logs", requestID)
		return
	}
	snapshots, err := s.runtime.accounts.ListQuotaSnapshotsByAccount(r.Context(), account.ID, 50)
	if err != nil {
		s.logger.Error("failed to list account quota snapshots", "error", err, "account_id", account.ID, "request_id", requestID)
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
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountQuotaListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}
