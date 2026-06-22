package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// handleListAdminErrorLogs serves GET /api/v1/admin/error-logs.
//
// Error logs are the failed slice of usage logs (success=false). Filters
// (user_id/api_key_id/account_id/model/error_class/source_endpoint/start/end +
// free-text q on request_id), ordering newest-first by id, and LIMIT/OFFSET
// pagination all run in SQL via usage.ListPage — the prior path loaded every
// usage row then filtered, sliced, and reduced to errors in Go memory, which
// dominated wall-clock once the table grew.
func (s *Server) handleListAdminErrorLogs(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	filter, ok := usageListFilterFromRequest(r)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid error log filter", requestID)
		return
	}
	failed := false
	filter.Success = &failed
	filter.Q = strings.TrimSpace(r.URL.Query().Get("q"))
	opts := listOptionsFromRequest(r)
	offset := (opts.Page - 1) * opts.PageSize
	if offset < 0 {
		offset = 0
	}
	page, err := s.runtime.usage.ListPage(r.Context(), filter, opts.PageSize, offset)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list error logs", requestID)
		return
	}
	data := make([]apiopenapi.ErrorLog, 0, len(page.Items))
	for _, item := range page.Items {
		data = append(data, toAPIErrorLog(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.ErrorLogListResponse{
		Data:       data,
		Pagination: paginationFromTotal(page.Total, opts.Page, opts.PageSize),
		RequestId:  requestID,
	})
}

// handleGetAdminErrorLog serves GET /api/v1/admin/error-logs/{id}.
//
// It matches the failed usage log by its numeric id and returns the single
// ErrorLog. The 200 body is the inline {data, request_id} object the OpenAPI
// spec defines for this operation (no dedicated named response struct was
// generated), so we build it as a map rather than a typed wrapper.
func (s *Server) handleGetAdminErrorLog(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	id := r.PathValue("id")
	items, err := s.runtime.usage.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to load error log", requestID)
		return
	}
	for _, item := range items {
		if !item.Success && strconv.Itoa(item.ID) == id {
			writeJSONAny(w, http.StatusOK, map[string]any{
				"data":       toAPIErrorLog(item),
				"request_id": requestID,
			})
			return
		}
	}
	writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "error log not found", requestID)
}

// toAPIErrorLog maps a (failed) usage log onto the generated ErrorLog DTO. It is
// a projection of toAPIUsageLog limited to the fields the error-log schema
// exposes; nullable foreign keys (account/provider) and error_class map to
// pointers, absent when the underlying value is nil.
func toAPIErrorLog(log usagecontract.UsageLog) apiopenapi.ErrorLog {
	out := apiopenapi.ErrorLog{
		AccountId:         optionalIDString(log.AccountID),
		ApiKeyId:          apiopenapi.Id(strconv.Itoa(log.APIKeyID)),
		AttemptNo:         log.AttemptNo,
		CreatedAt:         log.CreatedAt,
		ErrorClass:        log.ErrorClass,
		ErrorMessage:      nonEmptyStringPtr(log.ProviderErrorMessage),
		ErrorBodyExcerpt:  nonEmptyStringPtr(log.ProviderErrorBodyExcerpt),
		UpstreamRequestId: nonEmptyStringPtr(log.UpstreamRequestID),
		ErrorPhase:        nonEmptyStringPtr(log.ErrorPhase),
		ErrorOwner:        nonEmptyStringPtr(log.ErrorOwner),
		ErrorSource:       nonEmptyStringPtr(log.ErrorSource),
		Resolved:          log.Resolved,
		ResolvedBy:        optionalIDString(log.ResolvedBy),
		ResolvedAt:        log.ResolvedAt,
		Id:                apiopenapi.Id(strconv.Itoa(log.ID)),
		InputTokens:       log.InputTokens,
		LatencyMs:         log.LatencyMS,
		Model:             log.Model,
		OutputTokens:      log.OutputTokens,
		ProviderId:        optionalIDString(log.ProviderID),
		RequestId:         log.RequestID,
		SourceEndpoint:    log.SourceEndpoint,
		SourceProtocol:    log.SourceProtocol,
		TargetProtocol:    log.TargetProtocol,
		UsageEstimated:    log.UsageEstimated,
		UserId:            apiopenapi.Id(strconv.Itoa(log.UserID)),
	}
	if log.StatusCode > 0 {
		status := log.StatusCode
		out.StatusCode = &status
	}
	if len(log.UpstreamErrors) > 0 {
		events := make([]apiopenapi.UpstreamErrorEvent, 0, len(log.UpstreamErrors))
		for _, e := range log.UpstreamErrors {
			ev := apiopenapi.UpstreamErrorEvent{
				AtUnixMs:           e.AtUnixMs,
				AttemptNo:          e.AttemptNo,
				AccountName:        e.AccountName,
				UpstreamStatusCode: e.UpstreamStatusCode,
				UpstreamRequestId:  e.UpstreamRequestID,
				UpstreamUrl:        e.UpstreamURL,
				Kind:               e.Kind,
				Message:            e.Message,
				BodyExcerpt:        e.BodyExcerpt,
			}
			if e.AccountID != nil {
				id := strconv.Itoa(*e.AccountID)
				ev.AccountId = &id
			}
			events = append(events, ev)
		}
		out.UpstreamErrors = &events
	}
	return out
}

// handleResolveAdminErrorLog serves PATCH /api/v1/admin/error-logs/{id}/resolve.
//
// Body: {"resolved": true|false}. Toggling sets/clears resolved_by + resolved_at.
// Returns 200 with the updated ErrorLog (inline {data, request_id} shape, like
// the detail endpoint). Returns 404 when no failed usage log matches id; 501
// when the configured store does not implement the optional ResolveUpdater
// capability.
func (s *Server) handleResolveAdminErrorLog(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	id, err := strconv.Atoi(strings.TrimSpace(r.PathValue("id")))
	if err != nil || id <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.VALIDATIONFAILED, "invalid id", requestID)
		return
	}
	var body struct {
		Resolved bool `json:"resolved"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.VALIDATIONFAILED, "invalid request body", requestID)
		return
	}
	actor := session.User.ID
	actorPtr := &actor
	updated, err := s.runtime.usage.Resolve(r.Context(), id, body.Resolved, actorPtr)
	if err != nil {
		if errors.Is(err, usagecontract.ErrNotFound) {
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "error log not found", requestID)
			return
		}
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to update resolve state", requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       toAPIErrorLog(updated),
		"request_id": requestID,
	})
}

// nonEmptyStringPtr returns &value when value is non-empty after trimming
// whitespace; nil otherwise. Used to project optional upstream-error
// fields onto the openapi DTO's *string members so they marshal as absent
// (rather than as the empty string) when the usage log carries no upstream
// message.
func nonEmptyStringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
