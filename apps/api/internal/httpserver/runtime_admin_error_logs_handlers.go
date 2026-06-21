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
// Error logs are derived from the failed usage logs that carry gateway error
// classification and bounded upstream error context. The usual admin usage-log
// filters (user_id/api_key_id/account_id/model/error_class/source_endpoint/
// start/end) are reused via filterUsageLogs, then we keep only the failed rows,
// paginate server-side, and map each survivor to the generated ErrorLog DTO.
func (s *Server) handleListAdminErrorLogs(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	items, err := s.runtime.usage.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list error logs", requestID)
		return
	}
	items = filterUsageLogs(items, r)
	items = errorLogsFromUsageLogs(items)
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		items = filterErrorLogsByQuery(items, q)
	}
	total := len(items)
	opts := listOptionsFromRequest(r)
	start := (opts.Page - 1) * opts.PageSize
	if start > total {
		start = total
	}
	end := start + opts.PageSize
	if end > total {
		end = total
	}
	paged := items[start:end]
	data := make([]apiopenapi.ErrorLog, 0, len(paged))
	for _, item := range paged {
		data = append(data, toAPIErrorLog(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.ErrorLogListResponse{
		Data:       data,
		Pagination: paginationWithRequest(r, total),
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

// errorLogsFromUsageLogs keeps only the failed usage logs (Success == false),
// which is the degraded-mode definition of an error log in this codebase.
func errorLogsFromUsageLogs(items []usagecontract.UsageLog) []usagecontract.UsageLog {
	out := make([]usagecontract.UsageLog, 0, len(items))
	for _, item := range items {
		if !item.Success {
			out = append(out, item)
		}
	}
	return out
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

// filterErrorLogsByQuery applies the free-text q filter against
// error_message + request_id (case-insensitive substring match), mirroring
// sub2api's ops_error_logs search box. Falls through unchanged when q is empty.
func filterErrorLogsByQuery(items []usagecontract.UsageLog, q string) []usagecontract.UsageLog {
	needle := strings.ToLower(strings.TrimSpace(q))
	if needle == "" {
		return items
	}
	out := make([]usagecontract.UsageLog, 0, len(items))
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.ProviderErrorMessage), needle) ||
			strings.Contains(strings.ToLower(item.RequestID), needle) ||
			strings.Contains(strings.ToLower(item.UpstreamRequestID), needle) {
			out = append(out, item)
		}
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
