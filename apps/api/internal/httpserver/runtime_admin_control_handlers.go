package httpserver

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	paymentcontract "github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	schedulerservice "github.com/srapi/srapi/apps/api/internal/modules/scheduler/service"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

const (
	maxBulkPricingRuleImportItems      = 500
	pricingPresetSourceLiteLLMFallback = "built_in_litellm_fallback"
)

type pricingPreset struct {
	Family       string
	Input        string
	Output       string
	CacheRead    string
	CacheWrite   string
	CacheWrite5m string
	CacheWrite1h string
	ImageOutput  string
	Source       string
}

var builtInPricingPresets = []pricingPreset{
	{Family: "gpt-4o", Input: "2.50000000", Output: "10.00000000", CacheRead: "1.25000000", CacheWrite: "2.50000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-4o-mini", Input: "0.15000000", Output: "0.60000000", CacheRead: "0.07500000", CacheWrite: "0.15000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-4.1", Input: "2.00000000", Output: "8.00000000", CacheRead: "0.50000000", CacheWrite: "2.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-4.1-mini", Input: "0.40000000", Output: "1.60000000", CacheRead: "0.10000000", CacheWrite: "0.40000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-4.1-nano", Input: "0.10000000", Output: "0.40000000", CacheRead: "0.02500000", CacheWrite: "0.10000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-5", Input: "1.25000000", Output: "10.00000000", CacheRead: "0.12500000", CacheWrite: "1.25000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-5-mini", Input: "0.25000000", Output: "2.00000000", CacheRead: "0.02500000", CacheWrite: "0.25000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-5-nano", Input: "0.05000000", Output: "0.40000000", CacheRead: "0.00500000", CacheWrite: "0.05000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-5-pro", Input: "15.00000000", Output: "120.00000000", CacheRead: "0.00000000", CacheWrite: "15.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-5.1", Input: "1.25000000", Output: "10.00000000", CacheRead: "0.12500000", CacheWrite: "1.25000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-5.1-chat-latest", Input: "1.25000000", Output: "10.00000000", CacheRead: "0.12500000", CacheWrite: "1.25000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-5.1-codex", Input: "1.25000000", Output: "10.00000000", CacheRead: "0.12500000", CacheWrite: "1.25000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-5.1-codex-max", Input: "1.25000000", Output: "10.00000000", CacheRead: "0.12500000", CacheWrite: "1.25000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-5.1-codex-mini", Input: "0.25000000", Output: "2.00000000", CacheRead: "0.02500000", CacheWrite: "0.25000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-5.2", Input: "1.75000000", Output: "14.00000000", CacheRead: "0.17500000", CacheWrite: "1.75000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-5.2-chat-latest", Input: "1.75000000", Output: "14.00000000", CacheRead: "0.17500000", CacheWrite: "1.75000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-5.2-codex", Input: "1.75000000", Output: "14.00000000", CacheRead: "0.17500000", CacheWrite: "1.75000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-5.2-pro", Input: "21.00000000", Output: "168.00000000", CacheRead: "0.00000000", CacheWrite: "21.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-5.4", Input: "2.50000000", Output: "15.00000000", CacheRead: "0.25000000", CacheWrite: "2.50000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-5.4-mini", Input: "0.75000000", Output: "4.50000000", CacheRead: "0.07500000", CacheWrite: "0.75000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-5.4-nano", Input: "0.20000000", Output: "1.25000000", CacheRead: "0.02000000", CacheWrite: "0.20000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-5.4-pro", Input: "30.00000000", Output: "180.00000000", CacheRead: "3.00000000", CacheWrite: "30.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-5.5", Input: "5.00000000", Output: "30.00000000", CacheRead: "0.50000000", CacheWrite: "5.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-5.5-pro", Input: "30.00000000", Output: "180.00000000", CacheRead: "3.00000000", CacheWrite: "30.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "o3", Input: "2.00000000", Output: "8.00000000", CacheRead: "0.50000000", CacheWrite: "2.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "o3-mini", Input: "1.10000000", Output: "4.40000000", CacheRead: "0.55000000", CacheWrite: "1.10000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "o3-pro", Input: "20.00000000", Output: "80.00000000", CacheRead: "0.00000000", CacheWrite: "20.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "o4-mini", Input: "1.10000000", Output: "4.40000000", CacheRead: "0.27500000", CacheWrite: "1.10000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "codex-auto-review", Input: "5.00000000", Output: "30.00000000", CacheRead: "0.50000000", CacheWrite: "5.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "codex-mini-latest", Input: "0.25000000", Output: "2.00000000", CacheRead: "0.02500000", CacheWrite: "0.25000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-image-1", Input: "5.00000000", Output: "0.00000000", CacheRead: "1.25000000", CacheWrite: "5.00000000", ImageOutput: "40.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-image-1-mini", Input: "2.00000000", Output: "0.00000000", CacheRead: "0.20000000", CacheWrite: "2.00000000", ImageOutput: "8.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-image-1.5", Input: "5.00000000", Output: "10.00000000", CacheRead: "1.25000000", CacheWrite: "5.00000000", ImageOutput: "32.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-image-2", Input: "5.00000000", Output: "10.00000000", CacheRead: "1.25000000", CacheWrite: "5.00000000", ImageOutput: "30.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-4o-mini-transcribe", Input: "1.25000000", Output: "5.00000000", CacheRead: "0.00000000", CacheWrite: "1.25000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-4o-transcribe", Input: "2.50000000", Output: "10.00000000", CacheRead: "0.00000000", CacheWrite: "2.50000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gpt-4o-mini-tts", Input: "2.50000000", Output: "10.00000000", CacheRead: "0.00000000", CacheWrite: "2.50000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "claude-3-7-sonnet-20250219", Input: "3.00000000", Output: "15.00000000", CacheRead: "0.30000000", CacheWrite: "3.75000000", CacheWrite1h: "6.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "claude-haiku-4-5", Input: "1.00000000", Output: "5.00000000", CacheRead: "0.10000000", CacheWrite: "1.25000000", CacheWrite1h: "2.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "claude-sonnet-4-5", Input: "3.00000000", Output: "15.00000000", CacheRead: "0.30000000", CacheWrite: "3.75000000", CacheWrite1h: "6.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "claude-sonnet-4-6", Input: "3.00000000", Output: "15.00000000", CacheRead: "0.30000000", CacheWrite: "3.75000000", CacheWrite1h: "6.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "claude-opus-4-5", Input: "5.00000000", Output: "25.00000000", CacheRead: "0.50000000", CacheWrite: "6.25000000", CacheWrite1h: "10.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "claude-opus-4-6", Input: "5.00000000", Output: "25.00000000", CacheRead: "0.50000000", CacheWrite: "6.25000000", CacheWrite1h: "10.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "claude-opus-4-7", Input: "5.00000000", Output: "25.00000000", CacheRead: "0.50000000", CacheWrite: "6.25000000", CacheWrite1h: "10.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "claude-opus-4-8", Input: "5.00000000", Output: "25.00000000", CacheRead: "0.50000000", CacheWrite: "6.25000000", CacheWrite1h: "10.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gemini-2.0-flash", Input: "0.10000000", Output: "0.40000000", CacheRead: "0.02500000", CacheWrite: "0.10000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gemini-2.0-flash-lite", Input: "0.07500000", Output: "0.30000000", CacheRead: "0.01875000", CacheWrite: "0.07500000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gemini-2.5-flash", Input: "0.30000000", Output: "2.50000000", CacheRead: "0.03000000", CacheWrite: "0.30000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gemini-2.5-flash-lite", Input: "0.10000000", Output: "0.40000000", CacheRead: "0.01000000", CacheWrite: "0.10000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gemini-2.5-pro", Input: "1.25000000", Output: "10.00000000", CacheRead: "0.12500000", CacheWrite: "1.25000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gemini-3-flash", Input: "0.50000000", Output: "3.00000000", CacheRead: "0.05000000", CacheWrite: "0.50000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gemini-3-pro-preview", Input: "2.00000000", Output: "12.00000000", CacheRead: "0.20000000", CacheWrite: "2.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gemini-3.1-flash-lite", Input: "0.25000000", Output: "1.50000000", CacheRead: "0.02500000", CacheWrite: "0.25000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gemini-3.1-pro-preview", Input: "2.00000000", Output: "12.00000000", CacheRead: "0.20000000", CacheWrite: "2.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gemini-3.1-pro-low", Input: "2.00000000", Output: "12.00000000", CacheRead: "0.20000000", CacheWrite: "2.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gemini-3.1-pro-high", Input: "2.00000000", Output: "12.00000000", CacheRead: "0.20000000", CacheWrite: "2.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "gemini-3.5-flash", Input: "1.50000000", Output: "9.00000000", CacheRead: "0.15000000", CacheWrite: "1.50000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "deepseek-chat", Input: "0.28000000", Output: "0.42000000", CacheRead: "0.02800000", CacheWrite: "0.28000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "deepseek-reasoner", Input: "0.28000000", Output: "0.42000000", CacheRead: "0.02800000", CacheWrite: "0.28000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "grok-4.3", Input: "3.00000000", Output: "15.00000000", CacheRead: "0.75000000", CacheWrite: "3.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "grok-4.3-latest", Input: "3.00000000", Output: "15.00000000", CacheRead: "0.75000000", CacheWrite: "3.00000000", Source: pricingPresetSourceLiteLLMFallback},
	{Family: "grok-latest", Input: "3.00000000", Output: "15.00000000", CacheRead: "0.75000000", CacheWrite: "3.00000000", Source: pricingPresetSourceLiteLLMFallback},
}

// handleListAdminUsageLogs serves GET /api/v1/admin/usage-logs. Filtering,
// ordering (newest-first by id), and LIMIT/OFFSET pagination all run in the
// database via usage.ListPage — the prior path loaded every usage row before
// filtering and paginating in Go memory, which dominated wall-clock once the
// table grew past a few thousand rows.
func (s *Server) handleListAdminUsageLogs(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	filter, ok := usageListFilterFromRequest(r)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid usage log filter", requestID)
		return
	}
	opts := listOptionsFromRequest(r)
	offset := (opts.Page - 1) * opts.PageSize
	if offset < 0 {
		offset = 0
	}
	page, err := s.runtime.usage.ListPage(r.Context(), filter, opts.PageSize, offset)
	if err != nil {
		s.logger.Error("failed to list usage logs", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list usage logs", requestID)
		return
	}
	data := make([]apiopenapi.UsageLog, 0, len(page.Items))
	for _, item := range page.Items {
		data = append(data, toAPIUsageLog(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.UsageLogListResponse{
		Data:       data,
		Pagination: paginationFromTotal(page.Total, opts.Page, opts.PageSize),
		RequestId:  requestID,
	})
}

func (s *Server) handleListAdminAuditLogs(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	q := r.URL.Query()
	filter := auditcontract.ListFilter{
		Action:       strings.TrimSpace(q.Get("action")),
		ResourceType: strings.TrimSpace(q.Get("resource_type")),
	}
	if raw := strings.TrimSpace(q.Get("actor_user_id")); raw != "" {
		uid, err := strconv.Atoi(raw)
		if err != nil || uid <= 0 {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid actor user id", requestID)
			return
		}
		filter.ActorUserID = &uid
	}
	if since := parseUsageFilterTime(q.Get("since")); !since.IsZero() {
		t := since
		filter.Since = &t
	}
	limit, offset, page, pageSize := paginationParams(r)
	result, err := s.runtime.audit.ListPage(r.Context(), filter, limit, offset)
	if err != nil {
		s.logger.Error("failed to list audit logs", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list audit logs", requestID)
		return
	}
	data := make([]apiopenapi.AuditLog, 0, len(result.Items))
	for _, item := range result.Items {
		data = append(data, toAPIAuditLog(item))
	}
	var pg apiopenapi.Pagination
	if pageSize == 0 {
		pg = apiopenapi.Pagination{Page: 1, PageSize: len(data), Total: result.Total, HasNext: false}
	} else {
		pg = paginationFromTotal(result.Total, page, pageSize)
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AuditLogListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleListAdminBillingLedger(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	filter := billingcontract.LedgerListFilter{
		ReferenceType: strings.TrimSpace(r.URL.Query().Get("reference_type")),
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("user_id")); raw != "" {
		uid, err := strconv.Atoi(raw)
		if err != nil || uid <= 0 {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user id", requestID)
			return
		}
		filter.UserID = &uid
	}
	limit, offset, page, pageSize := paginationParams(r)
	filter.Limit = limit
	filter.Offset = offset
	result, err := s.runtime.billing.ListPage(r.Context(), filter)
	if err != nil {
		s.logger.Error("failed to list billing ledger", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list billing ledger", requestID)
		return
	}
	data := make([]apiopenapi.BillingLedgerEntry, 0, len(result.Items))
	for _, item := range result.Items {
		data = append(data, toAPIBillingLedgerEntry(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.BillingLedgerListResponse{
		Data:       data,
		Pagination: paginationFromTotal(result.Total, page, pageSize),
		RequestId:  requestID,
	})
}

func (s *Server) handleListAdminPaymentProviders(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	items, err := s.runtime.payments.ListProviderInstances(r.Context())
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	data := make([]apiopenapi.PaymentProviderInstance, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIPaymentProviderInstance(item))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.PaymentProviderInstanceListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminPaymentProvider(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreatePaymentProviderInstanceRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid payment provider request", requestID)
		return
	}
	provider, err := s.runtime.payments.CreateProviderInstance(r.Context(), paymentcontract.CreateProviderInstanceRequest{
		Provider:         body.Provider,
		Name:             body.Name,
		Status:           toPaymentProviderStatusPtr(body.Status),
		Config:           jsonObjectValueToMap(body.Config),
		SupportedMethods: derefStrings(body.SupportedMethods),
		Limits:           jsonObjectToMap(body.Limits),
		SortOrder:        body.SortOrder,
		FeeRate:          body.FeeRate,
		Weight:           body.Weight,
		Metadata:         jsonObjectToMap(body.Metadata),
	})
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "payment_provider.create", "payment_provider", strconv.Itoa(provider.ID), nil, map[string]any{
		"provider":          provider.Provider,
		"name":              provider.Name,
		"status":            provider.Status,
		"supported_methods": provider.SupportedMethods,
		"sort_order":        provider.SortOrder,
	}))
	writeJSONAny(w, http.StatusCreated, apiopenapi.PaymentProviderInstanceResponse{
		Data:      toAPIPaymentProviderInstance(provider),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminPaymentProvider(w http.ResponseWriter, r *http.Request) {
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
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid payment provider id", requestID)
		return
	}
	var body apiopenapi.UpdatePaymentProviderInstanceRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid payment provider request", requestID)
		return
	}
	before, err := s.runtime.payments.FindProviderInstanceByID(r.Context(), providerID)
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	updated, err := s.runtime.payments.UpdateProviderInstance(r.Context(), providerID, paymentcontract.UpdateProviderInstanceRequest{
		Name:             body.Name,
		Status:           toPaymentProviderStatusPtr(body.Status),
		Config:           jsonObjectToMapPtr(body.Config),
		SupportedMethods: body.SupportedMethods,
		Limits:           jsonObjectToMapPtr(body.Limits),
		SortOrder:        body.SortOrder,
		FeeRate:          body.FeeRate,
		Weight:           body.Weight,
		Metadata:         jsonObjectToMapPtr(body.Metadata),
	})
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "payment_provider.update", "payment_provider", strconv.Itoa(updated.ID), paymentProviderAuditSnapshot(before), paymentProviderAuditSnapshot(updated)))
	writeJSONAny(w, http.StatusOK, apiopenapi.PaymentProviderInstanceResponse{
		Data:      toAPIPaymentProviderInstance(updated),
		RequestId: requestID,
	})
}

func (s *Server) handleDeleteAdminPaymentProvider(w http.ResponseWriter, r *http.Request) {
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
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid payment provider id", requestID)
		return
	}
	before, err := s.runtime.payments.FindProviderInstanceByID(r.Context(), providerID)
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	if err := s.runtime.payments.DeleteProviderInstance(r.Context(), providerID); err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "payment_provider.delete", "payment_provider", strconv.Itoa(providerID), paymentProviderAuditSnapshot(before), nil))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"id": providerID, "deleted": true},
		"request_id": requestID,
	})
}

func (s *Server) handleTestAdminPaymentProvider(w http.ResponseWriter, r *http.Request) {
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
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid payment provider id", requestID)
		return
	}
	startedAt := time.Now()
	test, err := s.runtime.payments.TestProviderInstance(r.Context(), providerID)
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	result := adminTestResult(test.OK, test.Message, startedAt, apiopenapi.Id(strconv.Itoa(test.ProviderInstance.ID)), nil, test.Checks)
	result.Status = apiopenapi.AdminTestResultStatus(test.Status)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "payment_provider.test", "payment_provider", strconv.Itoa(test.ProviderInstance.ID), nil, map[string]any{
		"ok":     result.Ok,
		"status": result.Status,
		"checks": result.Checks,
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.AdminTestResultResponse{
		Data:      result,
		RequestId: requestID,
	})
}

func (s *Server) handleGetAdminPaymentDashboard(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminPermission(r, userscontract.PermissionPaymentRead); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "permission required", requestID)
		return
	}
	days := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid days", requestID)
			return
		}
		days = parsed
	}
	snapshot, err := s.runtime.payments.AggregatePaymentDashboard(r.Context(), days)
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AdminPaymentDashboardResponse{
		Data:      toAPIPaymentDashboard(snapshot),
		RequestId: requestID,
	})
}

func toAPIPaymentDashboard(snap paymentcontract.PaymentDashboardSnapshot) apiopenapi.AdminPaymentDashboard {
	methods := make([]apiopenapi.AdminPaymentMethodBreakdown, 0, len(snap.PaymentMethods))
	for _, m := range snap.PaymentMethods {
		methods = append(methods, apiopenapi.AdminPaymentMethodBreakdown{
			Provider: m.Provider,
			Count:    m.Count,
			Amount:   m.Amount,
		})
	}
	topUsers := make([]apiopenapi.AdminPaymentTopUser, 0, len(snap.TopUsers))
	for _, u := range snap.TopUsers {
		topUsers = append(topUsers, apiopenapi.AdminPaymentTopUser{
			UserId:     apiopenapi.Id(strconv.Itoa(u.UserID)),
			Amount:     u.Amount,
			OrderCount: u.OrderCount,
		})
	}
	return apiopenapi.AdminPaymentDashboard{
		DayRange: snap.DayRange,
		Currency: snap.Currency,
		Totals: apiopenapi.AdminPaymentDashboardTotals{
			OrderCount: snap.Totals.OrderCount,
			PaidCount:  snap.Totals.PaidCount,
			PaidAmount: snap.Totals.PaidAmount,
		},
		PaymentMethods: methods,
		TopUsers:       topUsers,
	}
}

func (s *Server) handleListAdminPaymentOrders(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminPermission(r, userscontract.PermissionPaymentRead); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "permission required", requestID)
		return
	}
	filter := paymentcontract.OrderListFilter{
		Status: strings.TrimSpace(r.URL.Query().Get("status")),
	}
	if userIDRaw := strings.TrimSpace(r.URL.Query().Get("user_id")); userIDRaw != "" {
		userID, parseErr := strconv.Atoi(userIDRaw)
		if parseErr != nil || userID <= 0 {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user id", requestID)
			return
		}
		filter.UserID = &userID
	}
	limit, offset, page, pageSize := paginationParams(r)
	result, err := s.runtime.payments.ListOrdersPage(r.Context(), filter, limit, offset)
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	data := make([]apiopenapi.PaymentOrder, 0, len(result.Items))
	for _, item := range result.Items {
		data = append(data, toAPIPaymentOrder(item))
	}
	var pg apiopenapi.Pagination
	if pageSize == 0 {
		pg = apiopenapi.Pagination{Page: 1, PageSize: len(data), Total: result.Total, HasNext: false}
	} else {
		pg = paginationFromTotal(result.Total, page, pageSize)
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.PaymentOrderListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleRefundAdminPaymentOrder(w http.ResponseWriter, r *http.Request) {
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
	orderID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || orderID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid payment order id", requestID)
		return
	}
	var body apiopenapi.RefundPaymentOrderRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid refund request", requestID)
		return
	}
	order, err := s.runtime.payments.RequestRefund(r.Context(), paymentcontract.RefundRequest{
		ActorUserID: session.User.ID,
		OrderID:     orderID,
		Amount:      optionalStringValue(body.Amount),
		Reason:      optionalStringValue(body.Reason),
	})
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.PaymentOrderResponse{
		Data:      toAPIPaymentOrder(order),
		RequestId: requestID,
	})
}

func (s *Server) handleListAdminPaymentOrderAuditLogs(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminPermission(r, userscontract.PermissionPaymentRead); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "permission required", requestID)
		return
	}
	orderID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || orderID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid payment order id", requestID)
		return
	}
	items, err := s.runtime.payments.ListAuditLogsByOrder(r.Context(), orderID)
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	data := make([]apiopenapi.PaymentAuditLog, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIPaymentAuditLog(item))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.PaymentAuditLogListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleListAdminSubscriptionPlans(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	items, err := s.runtime.subscriptions.ListPlans(r.Context())
	if err != nil {
		s.logger.Error("failed to list subscription plans", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list subscription plans", requestID)
		return
	}
	data := make([]apiopenapi.SubscriptionPlan, 0, len(items))
	for _, item := range items {
		data = append(data, toAPISubscriptionPlan(item))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.SubscriptionPlanListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminSubscriptionPlan(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if !s.requireSubscriptionPlansEnabled(w, r, requestID) {
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	var body apiopenapi.CreateSubscriptionPlanRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid subscription plan request", requestID)
		return
	}
	description := ""
	if body.Description != nil {
		description = *body.Description
	}
	plan, err := s.runtime.subscriptions.CreatePlan(r.Context(), subscriptioncontract.CreatePlanRequest{
		Name:         body.Name,
		Description:  description,
		Price:        body.Price,
		Currency:     body.Currency,
		ValidityDays: body.ValidityDays,
		Entitlements: jsonObjectToMap(body.Entitlements),
		ForSale:      body.ForSale,
		SortOrder:    body.SortOrder,
		Status:       toSubscriptionPlanStatusPtr(body.Status),
	})
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid subscription plan request", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "subscription_plan.create", "subscription_plan", strconv.Itoa(plan.ID), nil, subscriptionPlanAuditSnapshot(plan)))
	writeJSONAny(w, http.StatusCreated, apiopenapi.SubscriptionPlanResponse{
		Data:      toAPISubscriptionPlan(plan),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminSubscriptionPlan(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if !s.requireSubscriptionPlansEnabled(w, r, requestID) {
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	planID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || planID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid plan id", requestID)
		return
	}
	var beforeSnapshot map[string]any
	if existing, findErr := s.runtime.subscriptions.FindPlanByID(r.Context(), planID); findErr == nil {
		beforeSnapshot = subscriptionPlanAuditSnapshot(existing)
	}
	var body apiopenapi.UpdateSubscriptionPlanRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid subscription plan request", requestID)
		return
	}
	plan, err := s.runtime.subscriptions.UpdatePlan(r.Context(), planID, subscriptioncontract.UpdatePlanRequest{
		Name:         body.Name,
		Description:  body.Description,
		Price:        body.Price,
		Currency:     body.Currency,
		ValidityDays: body.ValidityDays,
		Entitlements: jsonObjectToMapPtr(body.Entitlements),
		ForSale:      body.ForSale,
		SortOrder:    body.SortOrder,
		Status:       toSubscriptionPlanStatusPtr(body.Status),
	})
	if err != nil {
		if errors.Is(err, subscriptioncontract.ErrNotFound) {
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "subscription plan not found", requestID)
			return
		}
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid subscription plan request", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "subscription_plan.update", "subscription_plan", strconv.Itoa(plan.ID), beforeSnapshot, subscriptionPlanAuditSnapshot(plan)))
	writeJSONAny(w, http.StatusOK, apiopenapi.SubscriptionPlanResponse{
		Data:      toAPISubscriptionPlan(plan),
		RequestId: requestID,
	})
}

func (s *Server) handleDeleteAdminSubscriptionPlan(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if !s.requireSubscriptionPlansEnabled(w, r, requestID) {
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	planID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || planID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid plan id", requestID)
		return
	}
	var beforeSnapshot map[string]any
	if existing, findErr := s.runtime.subscriptions.FindPlanByID(r.Context(), planID); findErr == nil {
		beforeSnapshot = subscriptionPlanAuditSnapshot(existing)
	}
	if err := s.runtime.subscriptions.DeletePlan(r.Context(), planID); err != nil {
		if errors.Is(err, subscriptioncontract.ErrNotFound) {
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "subscription plan not found", requestID)
			return
		}
		s.logger.Error("failed to delete subscription plan", "error", err, "plan_id", planID, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to delete subscription plan", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "subscription_plan.delete", "subscription_plan", strconv.Itoa(planID), beforeSnapshot, nil))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"id": planID, "deleted": true},
		"request_id": requestID,
	})
}

func (s *Server) handleListAdminUserSubscriptions(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	var (
		items []subscriptioncontract.UserSubscription
		err   error
	)
	if userIDRaw := strings.TrimSpace(r.URL.Query().Get("user_id")); userIDRaw != "" {
		userID, parseErr := strconv.Atoi(userIDRaw)
		if parseErr != nil || userID <= 0 {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user id", requestID)
			return
		}
		items, err = s.runtime.subscriptions.ListUserSubscriptionsByUser(r.Context(), userID)
	} else {
		items, err = s.runtime.subscriptions.ListUserSubscriptions(r.Context())
	}
	if err != nil {
		s.logger.Error("failed to list user subscriptions", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list user subscriptions", requestID)
		return
	}
	data := make([]apiopenapi.UserSubscription, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIUserSubscription(item))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.UserSubscriptionListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminUserSubscription(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if !s.requireSubscriptionPlansEnabled(w, r, requestID) {
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	var body apiopenapi.CreateUserSubscriptionRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid user subscription request", requestID)
		return
	}
	userID, err := strconv.Atoi(string(body.UserId))
	if err != nil || userID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user id", requestID)
		return
	}
	planID, err := strconv.Atoi(string(body.PlanId))
	if err != nil || planID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid plan id", requestID)
		return
	}
	if _, err := s.runtime.users.FindByID(r.Context(), userID); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "user not found", requestID)
		return
	}
	sourceType := ""
	if body.SourceType != nil {
		sourceType = *body.SourceType
	}
	sourceID := ""
	if body.SourceId != nil {
		sourceID = *body.SourceId
	}
	subscription, err := s.runtime.subscriptions.CreateUserSubscription(r.Context(), subscriptioncontract.CreateSubscriptionRequest{
		UserID:     userID,
		PlanID:     planID,
		Status:     toUserSubscriptionStatusPtr(body.Status),
		StartsAt:   body.StartsAt,
		ExpiresAt:  body.ExpiresAt,
		SourceType: sourceType,
		SourceID:   sourceID,
	})
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user subscription request", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user_subscription.create", "user_subscription", strconv.Itoa(subscription.ID), nil, userSubscriptionAuditSnapshot(subscription)))
	writeJSONAny(w, http.StatusCreated, apiopenapi.UserSubscriptionResponse{
		Data:      toAPIUserSubscription(subscription),
		RequestId: requestID,
	})
}

func (s *Server) handleDeleteAdminUserSubscription(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if !s.requireSubscriptionPlansEnabled(w, r, requestID) {
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	subscriptionID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || subscriptionID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user subscription id", requestID)
		return
	}
	if err := s.runtime.subscriptions.DeleteUserSubscription(r.Context(), subscriptionID); err != nil {
		if errors.Is(err, subscriptioncontract.ErrNotFound) {
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "user subscription not found", requestID)
			return
		}
		s.logger.Error("failed to delete user subscription", "error", err, "subscription_id", subscriptionID, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to delete user subscription", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user_subscription.delete", "user_subscription", strconv.Itoa(subscriptionID), nil, nil))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"id": subscriptionID, "deleted": true},
		"request_id": requestID,
	})
}

func (s *Server) handleListAdminPricingRules(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	items, err := s.runtime.billing.ListPricingRules(r.Context())
	if err != nil {
		s.logger.Error("failed to list pricing rules", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list pricing rules", requestID)
		return
	}
	data := make([]apiopenapi.PricingRule, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIPricingRule(item))
	}
	data = s.filterAPIPricingRules(r.Context(), data, r.URL.Query().Get("model_id"), r.URL.Query().Get("provider_id"), r.URL.Query().Get("q"))
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.PricingRuleListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminPricingRule(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreatePricingRuleRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid pricing rule request", requestID)
		return
	}
	pricingReq, message := s.pricingRuleRequestFromAPI(r.Context(), body)
	if message != "" {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, message, requestID)
		return
	}
	rule, err := s.runtime.billing.CreatePricingRule(r.Context(), pricingReq)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid pricing rule request", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "pricing_rule.create", "pricing_rule", strconv.Itoa(rule.ID), nil, pricingRuleAuditSnapshot(rule)))
	writeJSONAny(w, http.StatusCreated, apiopenapi.PricingRuleResponse{
		Data:      toAPIPricingRule(rule),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminPricingRule(w http.ResponseWriter, r *http.Request) {
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
	ruleID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || ruleID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid pricing rule id", requestID)
		return
	}
	var beforeSnapshot map[string]any
	if existing, findErr := s.runtime.billing.FindPricingRuleByID(r.Context(), ruleID); findErr == nil {
		beforeSnapshot = pricingRuleAuditSnapshot(existing)
	}
	var body apiopenapi.UpdatePricingRuleRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid pricing rule request", requestID)
		return
	}
	req := billingcontract.UpdatePricingRuleRequest{
		BillingMode:                       optionalBillingModeFromAPI(body.BillingMode),
		InputPricePerMillionTokens:        body.InputPricePerMillionTokens,
		OutputPricePerMillionTokens:       body.OutputPricePerMillionTokens,
		CacheReadPricePerMillionTokens:    body.CacheReadPricePerMillionTokens,
		CacheWritePricePerMillionTokens:   body.CacheWritePricePerMillionTokens,
		CacheWrite5mPricePerMillionTokens: body.CacheWrite5mPricePerMillionTokens,
		CacheWrite1hPricePerMillionTokens: body.CacheWrite1hPricePerMillionTokens,
		ImageOutputPricePerMillionTokens:  body.ImageOutputPricePerMillionTokens,
		PerRequestPrice:                   body.PerRequestPrice,
		ServiceTierMultipliers:            body.ServiceTierMultipliers,
		LongContextMultiplier:             body.LongContextMultiplier,
		Currency:                          body.Currency,
	}
	if body.LongContextThresholdTokens != nil {
		req.LongContextThresholdTokens = &body.LongContextThresholdTokens
	}
	if body.Intervals != nil {
		intervals := pricingIntervalsFromAPI(*body.Intervals)
		req.Intervals = &intervals
	}
	if body.EffectiveFrom != nil {
		req.EffectiveFrom = &body.EffectiveFrom
	}
	if body.EffectiveTo != nil {
		req.EffectiveTo = &body.EffectiveTo
	}
	rule, err := s.runtime.billing.UpdatePricingRule(r.Context(), ruleID, req)
	if err != nil {
		if errors.Is(err, billingcontract.ErrNotFound) {
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "pricing rule not found", requestID)
			return
		}
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid pricing rule request", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "pricing_rule.update", "pricing_rule", strconv.Itoa(rule.ID), beforeSnapshot, pricingRuleAuditSnapshot(rule)))
	writeJSONAny(w, http.StatusOK, apiopenapi.PricingRuleResponse{
		Data:      toAPIPricingRule(rule),
		RequestId: requestID,
	})
}

func (s *Server) handleDeleteAdminPricingRule(w http.ResponseWriter, r *http.Request) {
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
	ruleID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || ruleID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid pricing rule id", requestID)
		return
	}
	if err := s.runtime.billing.DeletePricingRule(r.Context(), ruleID); err != nil {
		if errors.Is(err, billingcontract.ErrNotFound) {
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "pricing rule not found", requestID)
			return
		}
		s.logger.Error("failed to delete pricing rule", "error", err, "rule_id", ruleID, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to delete pricing rule", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "pricing_rule.delete", "pricing_rule", strconv.Itoa(ruleID), nil, nil))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"id": ruleID, "deleted": true},
		"request_id": requestID,
	})
}

func (s *Server) handleBulkImportAdminPricingRules(w http.ResponseWriter, r *http.Request) {
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
	body, err := s.decodeBulkPricingRuleImport(w, r)
	if err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid pricing rule import request", requestID)
		return
	}
	if len(body.Items) == 0 || len(body.Items) > maxBulkPricingRuleImportItems {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "pricing rule import requires 1 to 500 items", requestID)
		return
	}
	dryRun := body.DryRun != nil && *body.DryRun
	errorsOut := make([]apiopenapi.BulkPricingRuleImportError, 0)
	rules := make([]apiopenapi.PricingRule, 0, len(body.Items))
	validated := 0
	created := 0
	for idx, item := range body.Items {
		pricingReq, message := s.pricingRuleRequestFromAPI(r.Context(), item)
		if message == "" {
			if err := s.runtime.billing.ValidatePricingRule(pricingReq); err != nil {
				message = "invalid pricing rule request"
			}
		}
		if message != "" {
			errorsOut = append(errorsOut, apiopenapi.BulkPricingRuleImportError{Index: idx, Message: message})
			continue
		}
		validated++
		if dryRun {
			continue
		}
		rule, err := s.runtime.billing.CreatePricingRule(r.Context(), pricingReq)
		if err != nil {
			errorsOut = append(errorsOut, apiopenapi.BulkPricingRuleImportError{Index: idx, Message: "invalid pricing rule request"})
			continue
		}
		created++
		rules = append(rules, toAPIPricingRule(rule))
	}
	if created > 0 {
		s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "pricing_rule.bulk_import", "pricing_rule", "bulk", nil, map[string]any{
			"requested": len(body.Items),
			"validated": validated,
			"created":   created,
			"errors":    len(errorsOut),
			"dry_run":   dryRun,
		}))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.BulkPricingRuleImportResponse{
		Data: apiopenapi.BulkPricingRuleImportResult{
			Created:   created,
			DryRun:    dryRun,
			Errors:    errorsOut,
			Requested: len(body.Items),
			Rules:     rules,
			Validated: validated,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleListAdminPricingRulePresets(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	data := make([]apiopenapi.PricingRulePreset, 0, len(builtInPricingPresets))
	for _, preset := range builtInPricingPresets {
		data = append(data, pricingPresetToAPI(preset))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.PricingRulePresetListResponse{
		Data:      data,
		RequestId: requestID,
	})
}

func (s *Server) handleInstallAdminPricingRulePresets(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.InstallPricingRulePresetsRequest
	if r.Body != nil {
		if err := s.decodeJSONBodyAllowEmpty(w, r, &body); err != nil {
			writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid pricing preset request", requestID)
			return
		}
	}
	dryRun := body.DryRun != nil && *body.DryRun
	presets := filterPricingPresets(body.Families)
	if len(presets) == 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "no pricing presets matched", requestID)
		return
	}
	errorsOut := make([]apiopenapi.BulkPricingRuleImportError, 0)
	rules := make([]apiopenapi.PricingRule, 0, len(presets))
	validated := 0
	created := 0
	for idx, preset := range presets {
		req := pricingPresetToCreateRequest(preset)
		if err := s.runtime.billing.ValidatePricingRule(req); err != nil {
			errorsOut = append(errorsOut, apiopenapi.BulkPricingRuleImportError{Index: idx, Message: "invalid pricing preset"})
			continue
		}
		validated++
		if dryRun {
			continue
		}
		rule, err := s.upsertPricingPreset(r.Context(), req)
		if err != nil {
			errorsOut = append(errorsOut, apiopenapi.BulkPricingRuleImportError{Index: idx, Message: "invalid pricing preset"})
			continue
		}
		created++
		rules = append(rules, toAPIPricingRule(rule))
	}
	if created > 0 {
		s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "pricing_rule.presets_install", "pricing_rule", "presets", nil, map[string]any{
			"requested": len(presets),
			"validated": validated,
			"created":   created,
			"errors":    len(errorsOut),
			"dry_run":   dryRun,
		}))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.PricingRulePresetInstallResponse{
		Data: apiopenapi.BulkPricingRuleImportResult{
			Created:   created,
			DryRun:    dryRun,
			Errors:    errorsOut,
			Requested: len(presets),
			Rules:     rules,
			Validated: validated,
		},
		RequestId: requestID,
	})
}

func (s *Server) decodeJSONBodyAllowEmpty(w http.ResponseWriter, r *http.Request, dst any) error {
	limited := http.MaxBytesReader(w, r.Body, s.cfg.Gateway.MaxBodySize)
	payload, err := io.ReadAll(limited)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return errRequestTooLarge
		}
		return err
	}
	if strings.TrimSpace(string(payload)) == "" {
		return nil
	}
	return decodeStrictJSON(payload, dst)
}

func filterPricingPresets(families *[]string) []pricingPreset {
	if families == nil || len(*families) == 0 {
		return append([]pricingPreset(nil), builtInPricingPresets...)
	}
	allowed := make(map[string]struct{}, len(*families))
	for _, family := range *families {
		if family = strings.ToLower(strings.TrimSpace(family)); family != "" {
			allowed[family] = struct{}{}
		}
	}
	out := make([]pricingPreset, 0, len(allowed))
	for _, preset := range builtInPricingPresets {
		if _, ok := allowed[strings.ToLower(strings.TrimSpace(preset.Family))]; ok {
			out = append(out, preset)
		}
	}
	return out
}

func pricingPresetToAPI(preset pricingPreset) apiopenapi.PricingRulePreset {
	return apiopenapi.PricingRulePreset{
		BillingMode:                       apiopenapi.Token,
		CacheReadPricePerMillionTokens:    preset.CacheRead,
		CacheWrite1hPricePerMillionTokens: optionalString(preset.CacheWrite1h),
		CacheWrite5mPricePerMillionTokens: optionalString(preset.CacheWrite5m),
		CacheWritePricePerMillionTokens:   preset.CacheWrite,
		Currency:                          "USD",
		ImageOutputPricePerMillionTokens:  optionalString(preset.ImageOutput),
		InputPricePerMillionTokens:        preset.Input,
		ModelFamily:                       preset.Family,
		OutputPricePerMillionTokens:       preset.Output,
		PerRequestPrice:                   optionalString("0.00000000"),
		Source:                            optionalString(preset.Source),
	}
}

func pricingPresetToCreateRequest(preset pricingPreset) billingcontract.CreatePricingRuleRequest {
	return billingcontract.CreatePricingRuleRequest{
		ModelID:                           0,
		ModelFamily:                       strings.ToLower(strings.TrimSpace(preset.Family)),
		ProviderID:                        0,
		BillingMode:                       billingcontract.BillingModeToken,
		InputPricePerMillionTokens:        preset.Input,
		OutputPricePerMillionTokens:       preset.Output,
		CacheReadPricePerMillionTokens:    preset.CacheRead,
		CacheWritePricePerMillionTokens:   preset.CacheWrite,
		CacheWrite5mPricePerMillionTokens: preset.CacheWrite5m,
		CacheWrite1hPricePerMillionTokens: preset.CacheWrite1h,
		ImageOutputPricePerMillionTokens:  preset.ImageOutput,
		PerRequestPrice:                   "0",
		Currency:                          "USD",
	}
}

func (s *Server) upsertPricingPreset(ctx context.Context, req billingcontract.CreatePricingRuleRequest) (billingcontract.PricingRule, error) {
	existing, err := s.runtime.billing.ListPricingRules(ctx)
	if err != nil {
		return billingcontract.PricingRule{}, err
	}
	for _, rule := range existing {
		if samePricingRuleNaturalKey(rule, req) {
			return s.runtime.billing.UpdatePricingRule(ctx, rule.ID, billingcontract.UpdatePricingRuleRequest{
				BillingMode:                       &req.BillingMode,
				InputPricePerMillionTokens:        &req.InputPricePerMillionTokens,
				OutputPricePerMillionTokens:       &req.OutputPricePerMillionTokens,
				CacheReadPricePerMillionTokens:    &req.CacheReadPricePerMillionTokens,
				CacheWritePricePerMillionTokens:   &req.CacheWritePricePerMillionTokens,
				CacheWrite5mPricePerMillionTokens: &req.CacheWrite5mPricePerMillionTokens,
				CacheWrite1hPricePerMillionTokens: &req.CacheWrite1hPricePerMillionTokens,
				ImageOutputPricePerMillionTokens:  &req.ImageOutputPricePerMillionTokens,
				PerRequestPrice:                   &req.PerRequestPrice,
				ServiceTierMultipliers:            &req.ServiceTierMultipliers,
				LongContextThresholdTokens:        &req.LongContextThresholdTokens,
				LongContextMultiplier:             &req.LongContextMultiplier,
				Intervals:                         &req.Intervals,
				Currency:                          &req.Currency,
				EffectiveFrom:                     &req.EffectiveFrom,
				EffectiveTo:                       &req.EffectiveTo,
			})
		}
	}
	return s.runtime.billing.CreatePricingRule(ctx, req)
}

func samePricingRuleNaturalKey(rule billingcontract.PricingRule, req billingcontract.CreatePricingRuleRequest) bool {
	if rule.ProviderID != req.ProviderID {
		return false
	}
	if req.ModelID > 0 {
		return rule.ModelID == req.ModelID
	}
	return rule.ModelID == 0 && strings.EqualFold(strings.TrimSpace(rule.ModelFamily), strings.TrimSpace(req.ModelFamily))
}

func (s *Server) decodeBulkPricingRuleImport(w http.ResponseWriter, r *http.Request) (apiopenapi.BulkPricingRuleImportRequest, error) {
	limited := http.MaxBytesReader(w, r.Body, s.cfg.Gateway.MaxBodySize)
	dryRun, err := parseBoolQuery(r.URL.Query().Get("dry_run"))
	if err != nil {
		return apiopenapi.BulkPricingRuleImportRequest{}, err
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))), "text/csv") {
		return decodeCSVPricingRuleImport(limited, dryRun)
	}
	payload, err := io.ReadAll(limited)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return apiopenapi.BulkPricingRuleImportRequest{}, errRequestTooLarge
		}
		return apiopenapi.BulkPricingRuleImportRequest{}, err
	}
	var body apiopenapi.BulkPricingRuleImportRequest
	if err := decodeStrictJSON(payload, &body); err == nil {
		if body.DryRun == nil {
			body.DryRun = dryRun
		}
		return body, nil
	}
	var items []apiopenapi.CreatePricingRuleRequest
	if err := decodeStrictJSON(payload, &items); err != nil {
		return apiopenapi.BulkPricingRuleImportRequest{}, err
	}
	return apiopenapi.BulkPricingRuleImportRequest{
		DryRun: dryRun,
		Items:  items,
	}, nil
}

func decodeStrictJSON(payload []byte, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst)
}

func (s *Server) pricingRuleRequestFromAPI(ctx context.Context, body apiopenapi.CreatePricingRuleRequest) (billingcontract.CreatePricingRuleRequest, string) {
	modelID, err := strconv.Atoi(string(body.ModelId))
	if err != nil || modelID < 0 {
		return billingcontract.CreatePricingRuleRequest{}, "invalid model id"
	}
	modelFamily := strings.ToLower(strings.TrimSpace(optionalStringValue(body.ModelFamily)))
	if modelID == 0 && modelFamily == "" {
		return billingcontract.CreatePricingRuleRequest{}, "model_family is required when model_id is 0"
	}
	providerID, err := strconv.Atoi(string(body.ProviderId))
	if err != nil || providerID < 0 {
		return billingcontract.CreatePricingRuleRequest{}, "invalid provider id"
	}
	if modelID > 0 {
		if _, err := s.runtime.models.FindByID(ctx, modelID); err != nil {
			return billingcontract.CreatePricingRuleRequest{}, "model not found"
		}
	}
	if providerID > 0 {
		if _, err := s.runtime.providers.FindByID(ctx, providerID); err != nil {
			return billingcontract.CreatePricingRuleRequest{}, "provider not found"
		}
	}
	return billingcontract.CreatePricingRuleRequest{
		ModelID:                           modelID,
		ModelFamily:                       modelFamily,
		ProviderID:                        providerID,
		BillingMode:                       billingModeFromAPI(body.BillingMode),
		InputPricePerMillionTokens:        body.InputPricePerMillionTokens,
		OutputPricePerMillionTokens:       body.OutputPricePerMillionTokens,
		CacheReadPricePerMillionTokens:    body.CacheReadPricePerMillionTokens,
		CacheWritePricePerMillionTokens:   body.CacheWritePricePerMillionTokens,
		CacheWrite5mPricePerMillionTokens: optionalStringValue(body.CacheWrite5mPricePerMillionTokens),
		CacheWrite1hPricePerMillionTokens: optionalStringValue(body.CacheWrite1hPricePerMillionTokens),
		ImageOutputPricePerMillionTokens:  optionalStringValue(body.ImageOutputPricePerMillionTokens),
		PerRequestPrice:                   optionalStringValue(body.PerRequestPrice),
		ServiceTierMultipliers:            mapStringStringValue(body.ServiceTierMultipliers),
		LongContextThresholdTokens:        body.LongContextThresholdTokens,
		LongContextMultiplier:             optionalStringValue(body.LongContextMultiplier),
		Intervals:                         pricingIntervalsFromAPIPtr(body.Intervals),
		Currency:                          body.Currency,
		EffectiveFrom:                     body.EffectiveFrom,
		EffectiveTo:                       body.EffectiveTo,
	}, ""
}

func decodeCSVPricingRuleImport(body io.Reader, dryRun *bool) (apiopenapi.BulkPricingRuleImportRequest, error) {
	reader := csv.NewReader(body)
	reader.TrimLeadingSpace = true
	header, err := reader.Read()
	if err != nil {
		return apiopenapi.BulkPricingRuleImportRequest{}, err
	}
	columns := make(map[string]int, len(header))
	for idx, column := range header {
		columns[strings.ToLower(strings.TrimSpace(column))] = idx
	}
	items := make([]apiopenapi.CreatePricingRuleRequest, 0)
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return apiopenapi.BulkPricingRuleImportRequest{}, err
		}
		item, err := pricingRuleFromCSVRecord(columns, record)
		if err != nil {
			return apiopenapi.BulkPricingRuleImportRequest{}, err
		}
		items = append(items, item)
	}
	return apiopenapi.BulkPricingRuleImportRequest{DryRun: dryRun, Items: items}, nil
}

func pricingRuleFromCSVRecord(columns map[string]int, record []string) (apiopenapi.CreatePricingRuleRequest, error) {
	effectiveFrom, err := csvOptionalTime(columns, record, "effective_from")
	if err != nil {
		return apiopenapi.CreatePricingRuleRequest{}, err
	}
	effectiveTo, err := csvOptionalTime(columns, record, "effective_to")
	if err != nil {
		return apiopenapi.CreatePricingRuleRequest{}, err
	}
	return apiopenapi.CreatePricingRuleRequest{
		ModelId:                           apiopenapi.Id(csvValue(columns, record, "model_id")),
		ProviderId:                        apiopenapi.Id(csvValue(columns, record, "provider_id")),
		InputPricePerMillionTokens:        csvValue(columns, record, "input_price_per_million_tokens"),
		OutputPricePerMillionTokens:       csvValue(columns, record, "output_price_per_million_tokens"),
		CacheReadPricePerMillionTokens:    csvValue(columns, record, "cache_read_price_per_million_tokens"),
		CacheWritePricePerMillionTokens:   csvValue(columns, record, "cache_write_price_per_million_tokens"),
		CacheWrite5mPricePerMillionTokens: ptrStringValue(csvValue(columns, record, "cache_write_5m_price_per_million_tokens")),
		CacheWrite1hPricePerMillionTokens: ptrStringValue(csvValue(columns, record, "cache_write_1h_price_per_million_tokens")),
		ImageOutputPricePerMillionTokens:  ptrStringValue(csvValue(columns, record, "image_output_price_per_million_tokens")),
		PerRequestPrice:                   ptrStringValue(csvValue(columns, record, "per_request_price")),
		Currency:                          csvValue(columns, record, "currency"),
		EffectiveFrom:                     effectiveFrom,
		EffectiveTo:                       effectiveTo,
	}, nil
}

func pricingIntervalsFromAPI(values []apiopenapi.PricingIntervalInput) []billingcontract.PricingInterval {
	out := make([]billingcontract.PricingInterval, 0, len(values))
	for _, value := range values {
		out = append(out, billingcontract.PricingInterval{
			MinTokens:                       optionalIntValue(value.MinTokens),
			MaxTokens:                       value.MaxTokens,
			TierLabel:                       optionalStringValue(value.TierLabel),
			ImageSize:                       optionalStringValue(value.ImageSize),
			InputPricePerMillionTokens:      optionalStringValue(value.InputPricePerMillionTokens),
			OutputPricePerMillionTokens:     optionalStringValue(value.OutputPricePerMillionTokens),
			CacheReadPricePerMillionTokens:  optionalStringValue(value.CacheReadPricePerMillionTokens),
			CacheWritePricePerMillionTokens: optionalStringValue(value.CacheWritePricePerMillionTokens),
			PerImagePrice:                   optionalStringValue(value.PerImagePrice),
		})
	}
	return out
}

func pricingIntervalsFromAPIPtr(values *[]apiopenapi.PricingIntervalInput) []billingcontract.PricingInterval {
	if values == nil {
		return nil
	}
	return pricingIntervalsFromAPI(*values)
}

func billingModeFromAPI(value *apiopenapi.BillingMode) billingcontract.BillingMode {
	if value == nil {
		return ""
	}
	return billingcontract.BillingMode(*value)
}

func optionalBillingModeFromAPI(value *apiopenapi.BillingMode) *billingcontract.BillingMode {
	if value == nil {
		return nil
	}
	mode := billingcontract.BillingMode(*value)
	return &mode
}

func optionalIntValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func csvValue(columns map[string]int, record []string, name string) string {
	idx, ok := columns[name]
	if !ok || idx < 0 || idx >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[idx])
}

func csvOptionalTime(columns map[string]int, record []string, name string) (*time.Time, error) {
	value := csvValue(columns, record, name)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseBoolQuery(value string) (*bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func (s *Server) handleListAdminOutboxEvents(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	q := r.URL.Query()
	filter := eventscontract.OutboxListFilter{
		Status:    eventscontract.OutboxStatus(strings.TrimSpace(q.Get("status"))),
		EventType: strings.TrimSpace(q.Get("event_type")),
	}
	limit, offset, page, pageSize := paginationParams(r)
	result, err := s.runtime.events.ListOutboxPage(r.Context(), filter, limit, offset)
	if err != nil {
		s.logger.Error("failed to list outbox events", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list outbox events", requestID)
		return
	}
	data := make([]apiopenapi.DomainEventOutbox, 0, len(result.Items))
	for _, item := range result.Items {
		data = append(data, toAPIDomainEventOutbox(item))
	}
	var pg apiopenapi.Pagination
	if pageSize == 0 {
		pg = apiopenapi.Pagination{Page: 1, PageSize: len(data), Total: result.Total, HasNext: false}
	} else {
		pg = paginationFromTotal(result.Total, page, pageSize)
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.DomainEventOutboxListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleListAdminOpsRealtimeSlots(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	list, err := s.runtime.realtime.ListActiveSlots(r.Context())
	if err != nil {
		s.logger.Error("failed to list realtime slots", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list realtime slots", requestID)
		return
	}
	data := make([]apiopenapi.RealtimeActiveSlot, 0, len(list.Slots))
	for _, slot := range list.Slots {
		data = append(data, toAPIRealtimeActiveSlot(slot))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.RealtimeActiveSlotListResponse{
		Counters:   toAPIRealtimeActiveSlotCounters(list),
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleListAdminOpsSLOs(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	items, err := s.runtime.operations.ListSLOs(r.Context())
	if err != nil {
		writeOperationsServiceError(w, err, requestID)
		return
	}
	data := make([]apiopenapi.OpsSLO, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIOpsSLO(item))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.OpsSLOListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminOpsSLO(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreateOpsSLORequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid slo request", requestID)
		return
	}
	createReq, err := toCreateSLORequest(body)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid slo request", requestID)
		return
	}
	created, err := s.runtime.operations.CreateSLO(r.Context(), createReq)
	if err != nil {
		writeOperationsServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "ops_slo.create", "ops_slo", strconv.Itoa(created.ID), nil, opsSLOAuditSnapshot(created)))
	writeJSONAny(w, http.StatusCreated, apiopenapi.OpsSLODefinitionResponse{
		Data:      toAPIOpsSLODefinition(created),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminOpsSLO(w http.ResponseWriter, r *http.Request) {
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
	sloID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || sloID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid slo id", requestID)
		return
	}
	current, err := s.runtime.operations.ListSLOs(r.Context())
	var beforeSnapshot map[string]any
	if err == nil {
		for _, item := range current {
			if item.Definition.ID == sloID {
				beforeSnapshot = opsSLOAuditSnapshot(item.Definition)
				break
			}
		}
	}
	var body apiopenapi.UpdateOpsSLORequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid slo request", requestID)
		return
	}
	updateReq, err := toUpdateSLORequest(body)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid slo request", requestID)
		return
	}
	updated, err := s.runtime.operations.UpdateSLO(r.Context(), sloID, updateReq)
	if err != nil {
		writeOperationsServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "ops_slo.update", "ops_slo", strconv.Itoa(updated.ID), beforeSnapshot, opsSLOAuditSnapshot(updated)))
	writeJSONAny(w, http.StatusOK, apiopenapi.OpsSLODefinitionResponse{
		Data:      toAPIOpsSLODefinition(updated),
		RequestId: requestID,
	})
}

func (s *Server) handleDeleteAdminOpsSLO(w http.ResponseWriter, r *http.Request) {
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
	sloID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || sloID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid slo id", requestID)
		return
	}
	if err := s.runtime.operations.DeleteSLO(r.Context(), sloID); err != nil {
		writeOperationsServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "ops_slo.delete", "ops_slo", strconv.Itoa(sloID), nil, nil))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"id": sloID, "deleted": true},
		"request_id": requestID,
	})
}

func (s *Server) handleListAdminOpsAlerts(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	items, err := s.runtime.operations.ListAlerts(r.Context())
	if err != nil {
		writeOperationsServiceError(w, err, requestID)
		return
	}
	items = filterOpsAlerts(items, r.URL.Query().Get("status"), r.URL.Query().Get("severity"))
	data := make([]apiopenapi.OpsAlertEvent, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIOpsAlert(item))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.OpsAlertListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleAcknowledgeAdminOpsAlert(w http.ResponseWriter, r *http.Request) {
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
	alertID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || alertID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid alert id", requestID)
		return
	}
	acknowledged, err := s.runtime.operations.AcknowledgeAlert(r.Context(), alertID, operationscontract.AckAlertRequest{
		ActorUserID: session.User.ID,
	})
	if err != nil {
		writeOperationsServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "ops_alert.ack", "ops_alert", strconv.Itoa(acknowledged.ID), nil, opsAlertAckAuditSnapshot(acknowledged)))
	writeJSONAny(w, http.StatusOK, apiopenapi.OpsAlertResponse{
		Data:      toAPIOpsAlert(acknowledged),
		RequestId: requestID,
	})
}

func (s *Server) handleListAdminCapabilities(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	items := filterCapabilityDefinitions(s.runtime.capabilities, r.URL.Query().Get("category"), r.URL.Query().Get("status"))
	data := make([]apiopenapi.CapabilityDefinition, 0, len(items))
	for _, item := range items {
		data = append(data, toAPICapabilityDefinition(item))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.CapabilityListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleAdminSchedulerOverview(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	decisions, err := s.runtime.scheduler.ListDecisions(r.Context())
	if err != nil {
		s.logger.Error("failed to list scheduler decisions for overview", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list scheduler decisions", requestID)
		return
	}
	usageLogs, err := s.runtime.usage.List(r.Context())
	if err != nil {
		s.logger.Error("failed to list usage logs for scheduler overview", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list usage logs", requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.SchedulerOverviewResponse{
		Data:      buildSchedulerOverview(decisions, usageLogs),
		RequestId: requestID,
	})
}

func (s *Server) handleListAdminSchedulerDecisions(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	start, end, err := schedulerDecisionWindowFromRequest(r)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, err.Error(), requestID)
		return
	}
	q := r.URL.Query()
	accountIDRaw := strings.TrimSpace(q.Get("account_id"))
	providerIDValue, hasProviderID, validProviderID := positiveIDFilter(q.Get("provider_id"))
	accountIDValue, hasAccountID, validAccountID := positiveIDFilter(accountIDRaw)
	if !validAccountID || !validProviderID {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid id filter", requestID)
		return
	}
	limit, offset, page, pageSize := paginationParams(r)
	// AccountID matches not just SelectedAccountID but any account_<id> key
	// inside the Scores / RejectReasons JSON evidence maps, which the SQL
	// store cannot express today. Fall back to the legacy whole-table path
	// when AccountID is set so semantics stay identical.
	if hasAccountID {
		items, err := s.runtime.scheduler.ListDecisions(r.Context())
		if err != nil {
			s.logger.Error("failed to list scheduler decisions (account filter fallback)", "error", err, "request_id", requestID)
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list scheduler decisions", requestID)
			return
		}
		items = filterSchedulerDecisions(items, q.Get("request_id"), q.Get("model"), q.Get("source_endpoint"), accountIDRaw, q.Get("provider_id"), start, end)
		data := make([]apiopenapi.SchedulerDecision, 0, len(items))
		for _, item := range items {
			data = append(data, toAPISchedulerDecision(item))
		}
		data, pg := paginate(r, data)
		writeJSONAny(w, http.StatusOK, apiopenapi.SchedulerDecisionListResponse{
			Data:       data,
			Pagination: pg,
			RequestId:  requestID,
		})
		return
	}
	_ = accountIDValue
	filter := schedulercontract.DecisionListFilter{
		RequestID:      strings.TrimSpace(q.Get("request_id")),
		Model:          strings.TrimSpace(q.Get("model")),
		SourceEndpoint: strings.TrimSpace(q.Get("source_endpoint")),
		Start:          start,
		End:            end,
	}
	if hasProviderID {
		v := providerIDValue
		filter.ProviderID = &v
	}
	result, supported, err := s.runtime.scheduler.ListDecisionsPage(r.Context(), filter, limit, offset)
	if err != nil {
		s.logger.Error("failed to list scheduler decisions page", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list scheduler decisions", requestID)
		return
	}
	if !supported {
		// Store doesn't implement the page reader (e.g. memory store used in
		// tests). Fall back to the legacy whole-table path so behavior stays
		// consistent across store implementations.
		items, listErr := s.runtime.scheduler.ListDecisions(r.Context())
		if listErr != nil {
			s.logger.Error("failed to list scheduler decisions (unsupported page fallback)", "error", listErr, "request_id", requestID)
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list scheduler decisions", requestID)
			return
		}
		items = filterSchedulerDecisions(items, q.Get("request_id"), q.Get("model"), q.Get("source_endpoint"), "", q.Get("provider_id"), start, end)
		data := make([]apiopenapi.SchedulerDecision, 0, len(items))
		for _, item := range items {
			data = append(data, toAPISchedulerDecision(item))
		}
		data, pg := paginate(r, data)
		writeJSONAny(w, http.StatusOK, apiopenapi.SchedulerDecisionListResponse{
			Data:       data,
			Pagination: pg,
			RequestId:  requestID,
		})
		return
	}
	data := make([]apiopenapi.SchedulerDecision, 0, len(result.Items))
	for _, item := range result.Items {
		data = append(data, toAPISchedulerDecision(item))
	}
	var pg apiopenapi.Pagination
	if pageSize == 0 {
		pg = apiopenapi.Pagination{Page: 1, PageSize: len(data), Total: result.Total, HasNext: false}
	} else {
		pg = paginationFromTotal(result.Total, page, pageSize)
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.SchedulerDecisionListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleListSchedulerStrategies(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	strategies, err := s.runtime.scheduler.ListStrategies(r.Context())
	if err != nil {
		s.logger.Error("failed to list scheduler strategies", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list scheduler strategies", requestID)
		return
	}
	data := make([]apiopenapi.SchedulerStrategy, 0, len(strategies))
	for _, strategy := range strategies {
		data = append(data, toAPISchedulerStrategy(strategy))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.SchedulerStrategyListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateSchedulerStrategy(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.SchedulerStrategyMutationRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid scheduler strategy request", requestID)
		return
	}
	input, err := schedulerStrategyMutationFromAPI(body, session.User.ID)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid scheduler strategy request", requestID)
		return
	}
	created, err := s.runtime.scheduler.CreateStrategy(r.Context(), input)
	if err != nil {
		writeSchedulerStrategyMutationError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusCreated, apiopenapi.SchedulerStrategyResponse{
		Data:      toAPISchedulerStrategy(created),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateSchedulerStrategy(w http.ResponseWriter, r *http.Request) {
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
	id, ok := pathID(w, r, requestID)
	if !ok {
		return
	}
	var body apiopenapi.SchedulerStrategyMutationRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid scheduler strategy request", requestID)
		return
	}
	input, err := schedulerStrategyMutationFromAPI(body, session.User.ID)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid scheduler strategy request", requestID)
		return
	}
	updated, err := s.runtime.scheduler.UpdateStrategy(r.Context(), id, input)
	if err != nil {
		writeSchedulerStrategyMutationError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.SchedulerStrategyResponse{
		Data:      toAPISchedulerStrategy(updated),
		RequestId: requestID,
	})
}

func (s *Server) handleActivateSchedulerStrategy(w http.ResponseWriter, r *http.Request) {
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
	id, ok := pathID(w, r, requestID)
	if !ok {
		return
	}
	updated, err := s.runtime.scheduler.ActivateStrategy(r.Context(), id)
	if err != nil {
		writeSchedulerStrategyMutationError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.SchedulerStrategyResponse{
		Data:      toAPISchedulerStrategy(updated),
		RequestId: requestID,
	})
}

func (s *Server) handleDeprecateSchedulerStrategy(w http.ResponseWriter, r *http.Request) {
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
	id, ok := pathID(w, r, requestID)
	if !ok {
		return
	}
	updated, err := s.runtime.scheduler.DeprecateStrategy(r.Context(), id)
	if err != nil {
		writeSchedulerStrategyMutationError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.SchedulerStrategyResponse{
		Data:      toAPISchedulerStrategy(updated),
		RequestId: requestID,
	})
}

func (s *Server) handleSimulateSchedulerStrategy(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.SchedulerSimulationRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid scheduler simulation request", requestID)
		return
	}
	req, err := toSchedulerSimulationRequest(body)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid scheduler simulation request", requestID)
		return
	}
	result, err := s.runtime.scheduler.SimulateStrategy(r.Context(), req)
	if err != nil {
		writeSchedulerSimulationError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.SchedulerSimulationResponse{
		Data:      toAPISchedulerSimulationResult(result),
		RequestId: requestID,
	})
}

func (s *Server) handleReplaySchedulerStrategy(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.SchedulerReplayRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid scheduler replay request", requestID)
		return
	}
	req := toSchedulerReplayRequest(body)
	result, err := s.runtime.scheduler.ReplayStrategies(r.Context(), req)
	if err != nil {
		writeSchedulerReplayError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.SchedulerReplayResponse{
		Data:      toAPISchedulerReplayResult(result),
		RequestId: requestID,
	})
}

func writeSchedulerSimulationError(w http.ResponseWriter, err error, requestID string) {
	if errors.Is(err, schedulerservice.ErrInvalidInput) {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid scheduler simulation request", requestID)
		return
	}
	writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to simulate scheduler strategy", requestID)
}

func writeSchedulerStrategyMutationError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, schedulerservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid scheduler strategy request", requestID)
	case errors.Is(err, schedulercontract.ErrNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "scheduler strategy not found", requestID)
	case errors.Is(err, schedulercontract.ErrConflict):
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "scheduler strategy already exists", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to update scheduler strategy", requestID)
	}
}

func writeSchedulerReplayError(w http.ResponseWriter, err error, requestID string) {
	if errors.Is(err, schedulerservice.ErrInvalidInput) {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid scheduler replay request", requestID)
		return
	}
	writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to replay scheduler strategy", requestID)
}
