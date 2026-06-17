// Package httpserver — admin handlers + gateway hot-path recorder for the
// ops_error_logs module. Mirrors sub2api's OpsService.RecordError +
// GetErrorLogs + UpdateErrorResolution call sites:
//   - recordOpsErrorLog is invoked from runtime_gateway_failover.go on every
//     provider attempt failure whose class indicates an upstream-side fault.
//   - handleListAdminOpsErrorLogs / handleUpdateAdminOpsErrorLogResolution
//     expose the operator surface under /api/v1/admin/ops/error-logs.
package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	opserrorlogscontract "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

// recordOpsErrorLog persists an operator-facing record of an upstream
// failure. Fire-and-forget: any error is swallowed (and logged) so the
// gateway hot path is never delayed by a telemetry write. The gating mirrors
// sub2api's RecordError emit conditions — server-side or network-class
// failures (5xx, transport errors) are persisted; client-bad classes (4xx
// from policy) are skipped because they're not actionable for operators.
func (s *Server) recordOpsErrorLog(ctx context.Context, authed apikeycontract.AuthResult, canonical gatewaycontract.CanonicalRequest, result schedulercontract.ScheduleResult, providerErr error, errorClass string, upstreamStatus int) {
	if s == nil || s.runtime == nil || s.runtime.opsErrorLogs == nil {
		return
	}
	if !opsErrorLogShouldRecord(errorClass, upstreamStatus) {
		return
	}
	req := opserrorlogscontract.RecordRequest{
		OccurredAt:       time.Now().UTC(),
		RequestID:        canonical.RequestID,
		TraceID:          requestIDFromContext(ctx),
		Platform:         string(canonical.SourceProtocol),
		SourceEndpoint:   canonical.SourceEndpoint,
		Model:            canonical.CanonicalModel,
		ErrorClass:       errorClass,
		ErrorPhase:       "upstream",
		ErrorMessage:     providerErrorMessage(providerErr),
		ErrorBodyExcerpt: opsErrorLogExcerpt(providerErr),
	}
	if upstreamStatus > 0 {
		code := upstreamStatus
		req.StatusCode = &code
	}
	if authed.UserID > 0 {
		uid := authed.UserID
		req.UserID = &uid
	}
	if authed.Key.ID > 0 {
		kid := authed.Key.ID
		req.APIKeyID = &kid
	}
	if result.Candidate.Account.ID > 0 {
		aid := result.Candidate.Account.ID
		req.AccountID = &aid
	}
	if result.Candidate.Provider.ID > 0 {
		pid := result.Candidate.Provider.ID
		req.ProviderID = &pid
	}
	// Best-effort: a failure here should never fail the request. The gateway
	// log surface remains intact via recordGatewayUsage even if this is dropped.
	if err := s.runtime.opsErrorLogs.RecordError(ctx, req); err != nil && s.runtime.logger != nil {
		s.runtime.logger.Warn("ops_error_logs RecordError failed", "request_id", canonical.RequestID, "error", err)
	}
}

// opsErrorLogShouldRecord mirrors sub2api's gate. The taxonomy maps from the
// gateway's four-class scheme:
//   - "server_bad" → always
//   - "transient" → only when upstreamStatus >= 500 (5xx transients);
//     non-5xx transients are usually 408/429/network blips that the failover
//     loop will retry and that don't warrant operator attention.
//   - "network_error" (the platform-side alias used by tests) → always
func opsErrorLogShouldRecord(class string, status int) bool {
	switch class {
	case "server_bad", "network_error":
		return true
	case "transient":
		return status >= 500
	}
	return false
}

// opsErrorLogExcerpt extracts a short body excerpt from a ProviderError, if
// any. Sensitive keys are scrubbed inside the service layer; this function
// only forwards what the adapter already exposed.
func opsErrorLogExcerpt(err error) string {
	if err == nil {
		return ""
	}
	var providerErr provideradaptercontract.ProviderError
	if !errors.As(err, &providerErr) {
		return ""
	}
	if len(providerErr.Metadata) == 0 {
		return ""
	}
	// Marshal the metadata as a JSON snapshot so the redaction pass in the
	// service can structurally redact any sensitive keys (api_key, token, ...).
	encoded, err := json.Marshal(providerErr.Metadata)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// handleListAdminOpsErrorLogs serves GET /api/v1/admin/ops/error-logs.
// Pagination + simple filters (resolution, error_class, user_id, account_id).
func (s *Server) handleListAdminOpsErrorLogs(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeJSONString(w, http.StatusForbidden, `{"error":{"code":"FORBIDDEN","message":"admin access required"},"request_id":"`+requestID+`"}`)
		return
	}
	if s.runtime == nil || s.runtime.opsErrorLogs == nil {
		writeJSONString(w, http.StatusServiceUnavailable, `{"error":{"code":"UNAVAILABLE","message":"ops error logs unavailable"},"request_id":"`+requestID+`"}`)
		return
	}
	filter := opserrorlogscontract.ListFilter{
		Resolution: opserrorlogscontract.Resolution(strings.TrimSpace(r.URL.Query().Get("resolution"))),
		ErrorClass: strings.TrimSpace(r.URL.Query().Get("error_class")),
		Platform:   strings.TrimSpace(r.URL.Query().Get("platform")),
	}
	if v := strings.TrimSpace(r.URL.Query().Get("user_id")); v != "" {
		if id, err := strconv.Atoi(v); err == nil && id > 0 {
			filter.UserID = &id
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("account_id")); v != "" {
		if id, err := strconv.Atoi(v); err == nil && id > 0 {
			filter.AccountID = &id
		}
	}
	page, pageSize := parseOpsErrorLogsPagination(r)
	filter.Page = page
	filter.PageSize = pageSize
	res, err := s.runtime.opsErrorLogs.List(r.Context(), filter)
	if err != nil {
		writeJSONString(w, http.StatusInternalServerError, `{"error":{"code":"INTERNAL_ERROR","message":"failed to list ops error logs"},"request_id":"`+requestID+`"}`)
		return
	}
	payload := map[string]any{
		"data":       opsErrorLogsToDTOs(res.Items),
		"pagination": map[string]any{"page": res.Page, "page_size": res.PageSize, "total": res.Total},
		"request_id": requestID,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		writeJSONString(w, http.StatusInternalServerError, `{"error":{"code":"INTERNAL_ERROR","message":"encode failed"},"request_id":"`+requestID+`"}`)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encoded)
}

// handleUpdateAdminOpsErrorLogResolution serves PATCH /api/v1/admin/ops/error-logs/{id}.
func (s *Server) handleUpdateAdminOpsErrorLogResolution(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeJSONString(w, http.StatusForbidden, `{"error":{"code":"FORBIDDEN","message":"admin access required"},"request_id":"`+requestID+`"}`)
		return
	}
	if s.runtime == nil || s.runtime.opsErrorLogs == nil {
		writeJSONString(w, http.StatusServiceUnavailable, `{"error":{"code":"UNAVAILABLE","message":"ops error logs unavailable"},"request_id":"`+requestID+`"}`)
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/ops/error-logs/")
	idStr = strings.TrimSuffix(idStr, "/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSONString(w, http.StatusBadRequest, `{"error":{"code":"INVALID_ID","message":"invalid id"},"request_id":"`+requestID+`"}`)
		return
	}
	var body struct {
		Resolution string `json:"resolution"`
		Note       string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONString(w, http.StatusBadRequest, `{"error":{"code":"INVALID_BODY","message":"invalid request body"},"request_id":"`+requestID+`"}`)
		return
	}
	resolverID := session.User.ID
	updated, err := s.runtime.opsErrorLogs.UpdateResolution(r.Context(), opserrorlogscontract.UpdateResolutionRequest{
		ID:           id,
		Resolution:   opserrorlogscontract.Resolution(strings.TrimSpace(body.Resolution)),
		Note:         body.Note,
		ResolvedByID: &resolverID,
	})
	if err != nil {
		if errors.Is(err, opserrorlogscontract.ErrNotFound) {
			writeJSONString(w, http.StatusNotFound, `{"error":{"code":"NOT_FOUND","message":"ops error log not found"},"request_id":"`+requestID+`"}`)
			return
		}
		writeJSONString(w, http.StatusBadRequest, `{"error":{"code":"INVALID_INPUT","message":"`+err.Error()+`"},"request_id":"`+requestID+`"}`)
		return
	}
	encoded, err := json.Marshal(map[string]any{
		"data":       opsErrorLogToDTO(updated),
		"request_id": requestID,
	})
	if err != nil {
		writeJSONString(w, http.StatusInternalServerError, `{"error":{"code":"INTERNAL_ERROR","message":"encode failed"},"request_id":"`+requestID+`"}`)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encoded)
}

func parseOpsErrorLogsPagination(r *http.Request) (page, pageSize int) {
	page = 1
	pageSize = 20
	if v := strings.TrimSpace(r.URL.Query().Get("page")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("page_size")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			pageSize = n
		}
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize
}

func opsErrorLogsToDTOs(items []opserrorlogscontract.Entry) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, opsErrorLogToDTO(item))
	}
	return out
}

func opsErrorLogToDTO(entry opserrorlogscontract.Entry) map[string]any {
	dto := map[string]any{
		"id":                 strconv.FormatInt(entry.ID, 10),
		"occurred_at":        entry.OccurredAt.Format(time.RFC3339Nano),
		"request_id":         entry.RequestID,
		"trace_id":           entry.TraceID,
		"platform":           entry.Platform,
		"source_endpoint":    entry.SourceEndpoint,
		"model":              entry.Model,
		"error_class":        entry.ErrorClass,
		"error_phase":        entry.ErrorPhase,
		"error_message":      entry.ErrorMessage,
		"error_body_excerpt": entry.ErrorBodyExcerpt,
		"resolution":         string(entry.Resolution),
		"resolution_note":    entry.ResolutionNote,
		"created_at":         entry.CreatedAt.Format(time.RFC3339Nano),
		"updated_at":         entry.UpdatedAt.Format(time.RFC3339Nano),
	}
	if entry.UserID != nil {
		dto["user_id"] = *entry.UserID
	}
	if entry.APIKeyID != nil {
		dto["api_key_id"] = *entry.APIKeyID
	}
	if entry.AccountID != nil {
		dto["account_id"] = *entry.AccountID
	}
	if entry.ProviderID != nil {
		dto["provider_id"] = *entry.ProviderID
	}
	if entry.StatusCode != nil {
		dto["status_code"] = *entry.StatusCode
	}
	if entry.ResolvedAt != nil {
		dto["resolved_at"] = entry.ResolvedAt.Format(time.RFC3339Nano)
	}
	if entry.ResolvedByID != nil {
		dto["resolved_by_user_id"] = *entry.ResolvedByID
	}
	return dto
}

// writeJSONString writes a pre-formatted JSON string response. Lighter-weight
// than building a struct for these constant error envelopes; mirrors existing
// patterns in this package.
func writeJSONString(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
