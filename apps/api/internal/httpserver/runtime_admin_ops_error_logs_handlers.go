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

	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	opserrorlogscontract "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/contract"
)

// recordOpsErrorLog persists an operator-facing record of an upstream
// failure. Fire-and-forget: any error is swallowed (and logged) so the
// gateway hot path is never delayed by a telemetry write. The gating mirrors
// sub2api's RecordError emit conditions — server-side or network-class
// failures (5xx, transport errors) are persisted; client-bad classes (4xx
// from policy) are skipped because they're not actionable for operators.
func (s *Server) recordOpsErrorLog(ctx context.Context, rec gatewayUsageRecord) {
	if s == nil || s.runtime == nil || s.runtime.opsErrorLogs == nil {
		return
	}
	errorClass := stringValue(rec.ErrorClass)
	upstreamStatus := opsErrorLogIntValue(rec.StatusCode)
	if !opsErrorLogShouldRecord(errorClass, upstreamStatus) {
		return
	}
	req := opserrorlogscontract.RecordRequest{
		OccurredAt:        time.Now().UTC(),
		RequestID:         rec.RequestID,
		TraceID:           requestIDFromContext(ctx),
		Platform:          rec.SourceProtocol,
		SourceEndpoint:    rec.SourceEndpoint,
		TargetProtocol:    rec.TargetProtocol,
		Model:             rec.Model,
		ErrorClass:        errorClass,
		ErrorPhase:        rec.ErrorPhase,
		ErrorOwner:        rec.ErrorOwner,
		ErrorSource:       rec.ErrorSource,
		ErrorMessage:      rec.ProviderErrorMessage,
		ErrorBodyExcerpt:  rec.ProviderErrorBodyExcerpt,
		UpstreamRequestID: rec.UpstreamRequestID,
		AttemptNo:         rec.AttemptNo,
		LatencyMS:         rec.LatencyMS,
		InputTokens:       rec.InputTokens,
		OutputTokens:      rec.OutputTokens,
		UsageEstimated:    rec.UsageEstimated,
		UpstreamErrors:    opsErrorLogEventsFromGateway(rec.UpstreamErrors),
	}
	if upstreamStatus > 0 {
		code := upstreamStatus
		req.StatusCode = &code
	}
	if rec.Authed.UserID > 0 {
		uid := rec.Authed.UserID
		req.UserID = &uid
	}
	if rec.Authed.Key.ID > 0 {
		kid := rec.Authed.Key.ID
		req.APIKeyID = &kid
	}
	if prefix := strings.TrimSpace(rec.Authed.Key.Prefix); prefix != "" {
		req.APIKeyPrefix = prefix
	}
	if rec.AccountID != nil && *rec.AccountID > 0 {
		req.AccountID = rec.AccountID
	}
	if rec.ProviderID != nil && *rec.ProviderID > 0 {
		req.ProviderID = rec.ProviderID
	}
	// Best-effort: a failure here should never fail the request. The gateway
	// log surface remains intact via recordGatewayUsage even if this is dropped.
	if err := s.runtime.opsErrorLogs.RecordError(ctx, req); err != nil && s.runtime.logger != nil {
		s.runtime.logger.Warn("ops_error_logs RecordError failed", "request_id", rec.RequestID, "error", err)
	}
}

// recordGatewaySystemLog forwards a failed gateway attempt onto the
// operations-owned system-log stream. WARN is used for upstream rejections,
// ERROR for server/transport failures, and INFO for no-available-account
// decisions that are operator-visible but not anomalous.
func (s *Server) recordGatewaySystemLog(ctx context.Context, rec gatewayUsageRecord) {
	if s == nil || s.runtime == nil {
		return
	}
	s.runtime.recordGatewaySystemLog(ctx, rec)
}

// recordGatewaySystemLog forwards a failed gateway attempt onto the
// operations-owned system-log stream. It is intentionally independent from
// ops_error_logs so the compact operational timeline remains available when
// the structured error-log store is down or filtered.
func (rt *runtimeState) recordGatewaySystemLog(ctx context.Context, rec gatewayUsageRecord) {
	if rt == nil || rt.operations == nil {
		return
	}
	errorClass := stringValue(rec.ErrorClass)
	upstreamStatus := opsErrorLogIntValue(rec.StatusCode)
	level := operationscontract.OpsSystemLogLevelWarn
	if errorClass == "no_available_account" {
		level = operationscontract.OpsSystemLogLevelInfo
	} else if upstreamStatus >= 500 || errorClass == "network_error" {
		level = operationscontract.OpsSystemLogLevelError
	}
	message := strings.TrimSpace(rec.ProviderErrorMessage)
	if strings.TrimSpace(message) == "" {
		message = errorClass
	}
	metadata := map[string]any{
		"request_id":      rec.RequestID,
		"source_endpoint": rec.SourceEndpoint,
		"source_protocol": rec.SourceProtocol,
		"target_protocol": rec.TargetProtocol,
		"canonical_model": rec.Model,
		"attempt_no":      rec.AttemptNo,
		"error_class":     errorClass,
		"upstream_status": upstreamStatus,
	}
	if rec.AccountID != nil && *rec.AccountID > 0 {
		metadata["account_id"] = *rec.AccountID
	}
	if rec.ProviderID != nil && *rec.ProviderID > 0 {
		metadata["provider_id"] = *rec.ProviderID
	}
	if _, err := rt.operations.RecordSystemLog(ctx, operationscontract.RecordSystemLogRequest{
		Level:     level,
		Message:   message,
		Source:    "gateway",
		RequestID: rec.RequestID,
		TraceID:   requestIDFromContext(ctx),
		Metadata:  metadata,
		CreatedAt: time.Now().UTC(),
	}); err != nil && rt.logger != nil {
		rt.logger.Warn("operations RecordSystemLog failed", "request_id", rec.RequestID, "error", err)
	}
}

func opsErrorLogEventsFromGateway(events []gatewayUpstreamErrorEvent) []opserrorlogscontract.UpstreamErrorEvent {
	if len(events) == 0 {
		return nil
	}
	out := make([]opserrorlogscontract.UpstreamErrorEvent, 0, len(events))
	for _, event := range events {
		out = append(out, opserrorlogscontract.UpstreamErrorEvent{
			AtUnixMs:           event.AtUnixMs,
			AttemptNo:          event.AttemptNo,
			AccountID:          event.AccountID,
			AccountName:        event.AccountName,
			UpstreamStatusCode: event.UpstreamStatusCode,
			UpstreamRequestID:  event.UpstreamRequestID,
			UpstreamURL:        event.UpstreamURL,
			Kind:               event.Kind,
			Message:            event.Message,
			BodyExcerpt:        event.BodyExcerpt,
		})
	}
	return out
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func opsErrorLogIntValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

// opsErrorLogShouldRecord decides whether a failed upstream attempt is
// operator-actionable enough to persist in ops_error_logs. Earlier this gate
// dropped every 4xx (so an upstream "provider rejected request" 400 — exactly
// the Hermes /compact case — never surfaced in the admin panel). The widened
// gate mirrors sub2api: any upstream HTTP response in the 4xx-5xx range is
// recorded (operator needs to see Codex's actual rejection reason), and
// transport-level failures (network_error class) are always recorded. Pure
// client-side failures with no upstream call (decode error, model_not_found
// before the scheduler, etc.) are still skipped — they live on usage_log
// rows but do not pollute the system-log timeline.
func opsErrorLogShouldRecord(class string, status int) bool {
	if status >= 400 && status < 600 {
		return true
	}
	switch class {
	case "server_bad", "network_error":
		return true
	}
	return false
}

// handleListAdminOpsErrorLogs serves GET /api/v1/admin/ops/error-logs.
// Pagination + filters for the operator-facing error evidence feed.
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
		Model:      strings.TrimSpace(r.URL.Query().Get("model")),
		Query:      strings.TrimSpace(r.URL.Query().Get("q")),
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
	if v := strings.TrimSpace(r.URL.Query().Get("provider_id")); v != "" {
		if id, err := strconv.Atoi(v); err == nil && id > 0 {
			filter.ProviderID = &id
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("status_min")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			filter.StatusCodeMin = &n
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("status_max")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			filter.StatusCodeMax = &n
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("start")); v != "" {
		if at, err := time.Parse(time.RFC3339, v); err == nil {
			filter.From = &at
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("end")); v != "" {
		if at, err := time.Parse(time.RFC3339, v); err == nil {
			filter.To = &at
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
		"data": opsErrorLogsToDTOs(res.Items),
		"pagination": map[string]any{
			"page":      res.Page,
			"page_size": res.PageSize,
			"total":     res.Total,
			"has_next":  res.Page*res.PageSize < res.Total,
		},
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

// handleGetAdminOpsErrorLog serves GET /api/v1/admin/ops/error-logs/{id}.
func (s *Server) handleGetAdminOpsErrorLog(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeJSONString(w, http.StatusForbidden, `{"error":{"code":"FORBIDDEN","message":"admin access required"},"request_id":"`+requestID+`"}`)
		return
	}
	if s.runtime == nil || s.runtime.opsErrorLogs == nil {
		writeJSONString(w, http.StatusServiceUnavailable, `{"error":{"code":"UNAVAILABLE","message":"ops error logs unavailable"},"request_id":"`+requestID+`"}`)
		return
	}
	id, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("id")), 10, 64)
	if err != nil || id <= 0 {
		writeJSONString(w, http.StatusBadRequest, `{"error":{"code":"INVALID_ID","message":"invalid id"},"request_id":"`+requestID+`"}`)
		return
	}
	entry, err := s.runtime.opsErrorLogs.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, opserrorlogscontract.ErrNotFound) {
			writeJSONString(w, http.StatusNotFound, `{"error":{"code":"NOT_FOUND","message":"ops error log not found"},"request_id":"`+requestID+`"}`)
			return
		}
		writeJSONString(w, http.StatusInternalServerError, `{"error":{"code":"INTERNAL_ERROR","message":"failed to get ops error log"},"request_id":"`+requestID+`"}`)
		return
	}
	encoded, err := json.Marshal(map[string]any{
		"data":       opsErrorLogToDTO(entry),
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
	id, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("id")), 10, 64)
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
		"id":                  strconv.FormatInt(entry.ID, 10),
		"occurred_at":         entry.OccurredAt.Format(time.RFC3339Nano),
		"request_id":          entry.RequestID,
		"trace_id":            entry.TraceID,
		"api_key_prefix":      entry.APIKeyPrefix,
		"platform":            entry.Platform,
		"source_endpoint":     entry.SourceEndpoint,
		"source_protocol":     entry.Platform,
		"target_protocol":     entry.TargetProtocol,
		"model":               entry.Model,
		"upstream_request_id": entry.UpstreamRequestID,
		"attempt_no":          entry.AttemptNo,
		"latency_ms":          entry.LatencyMS,
		"input_tokens":        entry.InputTokens,
		"output_tokens":       entry.OutputTokens,
		"usage_estimated":     entry.UsageEstimated,
		"error_class":         entry.ErrorClass,
		"error_phase":         entry.ErrorPhase,
		"error_owner":         entry.ErrorOwner,
		"error_source":        entry.ErrorSource,
		"error_message":       entry.ErrorMessage,
		"error_body_excerpt":  entry.ErrorBodyExcerpt,
		"upstream_errors":     opsErrorLogEventsToDTO(entry.UpstreamErrors),
		"resolution":          string(entry.Resolution),
		"resolution_note":     entry.ResolutionNote,
		"created_at":          entry.CreatedAt.Format(time.RFC3339Nano),
		"updated_at":          entry.UpdatedAt.Format(time.RFC3339Nano),
	}
	if entry.UserID != nil {
		dto["user_id"] = strconv.Itoa(*entry.UserID)
	}
	if entry.APIKeyID != nil {
		dto["api_key_id"] = strconv.Itoa(*entry.APIKeyID)
	}
	if entry.AccountID != nil {
		dto["account_id"] = strconv.Itoa(*entry.AccountID)
	}
	if entry.ProviderID != nil {
		dto["provider_id"] = strconv.Itoa(*entry.ProviderID)
	}
	if entry.StatusCode != nil {
		dto["status_code"] = *entry.StatusCode
	}
	if entry.ResolvedAt != nil {
		dto["resolved_at"] = entry.ResolvedAt.Format(time.RFC3339Nano)
	}
	if entry.ResolvedByID != nil {
		dto["resolved_by_user_id"] = strconv.Itoa(*entry.ResolvedByID)
	}
	return dto
}

func opsErrorLogEventsToDTO(events []opserrorlogscontract.UpstreamErrorEvent) []map[string]any {
	out := make([]map[string]any, 0, len(events))
	for _, event := range events {
		item := map[string]any{
			"at_unix_ms":           event.AtUnixMs,
			"attempt_no":           event.AttemptNo,
			"account_name":         event.AccountName,
			"upstream_status_code": event.UpstreamStatusCode,
			"upstream_request_id":  event.UpstreamRequestID,
			"upstream_url":         event.UpstreamURL,
			"kind":                 event.Kind,
			"message":              event.Message,
			"body_excerpt":         event.BodyExcerpt,
		}
		if event.AccountID != nil {
			item["account_id"] = strconv.Itoa(*event.AccountID)
		}
		out = append(out, item)
	}
	return out
}

// writeJSONString writes a pre-formatted JSON string response. Lighter-weight
// than building a struct for these constant error envelopes; mirrors existing
// patterns in this package.
func writeJSONString(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
