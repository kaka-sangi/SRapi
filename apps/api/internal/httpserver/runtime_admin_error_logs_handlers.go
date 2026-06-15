package httpserver

import (
	"net/http"
	"strconv"

	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// handleListAdminErrorLogs serves GET /api/v1/admin/error-logs.
//
// Error logs are not a distinct persisted table: this codebase has no
// status_code column on usage_log, so we run in "degraded mode" and DERIVE the
// error-log feed from the usage logs where Success == false. The usual admin
// usage-log filters (user_id/api_key_id/account_id/model/error_class/
// source_endpoint/start/end) are reused via filterUsageLogs, then we keep only
// the failed rows, paginate server-side, and map each survivor to the generated
// ErrorLog DTO.
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
	return apiopenapi.ErrorLog{
		AccountId:      optionalIDString(log.AccountID),
		ApiKeyId:       apiopenapi.Id(strconv.Itoa(log.APIKeyID)),
		AttemptNo:      log.AttemptNo,
		CreatedAt:      log.CreatedAt,
		ErrorClass:     log.ErrorClass,
		Id:             apiopenapi.Id(strconv.Itoa(log.ID)),
		InputTokens:    log.InputTokens,
		LatencyMs:      log.LatencyMS,
		Model:          log.Model,
		OutputTokens:   log.OutputTokens,
		ProviderId:     optionalIDString(log.ProviderID),
		RequestId:      log.RequestID,
		SourceEndpoint: log.SourceEndpoint,
		SourceProtocol: log.SourceProtocol,
		TargetProtocol: log.TargetProtocol,
		UsageEstimated: log.UsageEstimated,
		UserId:         apiopenapi.Id(strconv.Itoa(log.UserID)),
	}
}
