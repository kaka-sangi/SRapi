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
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// recordOpsErrorLog persists operator-facing gateway failure evidence.
// Fire-and-forget: any error is swallowed (and logged) so the gateway hot path
// is never delayed by a telemetry write. Upstream HTTP errors, transport
// errors, and platform-side no-available-account decisions are persisted;
// pure client-side failures with no operator action stay on usage logs only.
func (s *Server) recordOpsErrorLog(ctx context.Context, rec gatewayUsageRecord) {
	if s == nil || s.runtime == nil || s.runtime.opsErrorLogs == nil {
		return
	}
	rec.SourceEndpoint = gatewayEvidenceEndpoint(ctx, rec.SourceEndpoint)
	errorClass := stringValue(rec.ErrorClass)
	upstreamStatus := opsErrorLogIntValue(rec.StatusCode)
	if !opsErrorLogShouldRecord(errorClass, upstreamStatus) {
		return
	}
	req := opserrorlogscontract.RecordRequest{
		OccurredAt:            time.Now().UTC(),
		RequestID:             rec.RequestID,
		TraceID:               traceIDFromContext(ctx),
		Platform:              rec.SourceProtocol,
		SourceEndpoint:        rec.SourceEndpoint,
		TargetProtocol:        rec.TargetProtocol,
		Model:                 rec.Model,
		StreamCompletionState: rec.StreamCompletionState,
		ErrorClass:            errorClass,
		ErrorPhase:            rec.ErrorPhase,
		ErrorOwner:            rec.ErrorOwner,
		ErrorSource:           rec.ErrorSource,
		ErrorMessage:          rec.ProviderErrorMessage,
		ErrorBodyExcerpt:      rec.ProviderErrorBodyExcerpt,
		UpstreamRequestID:     rec.UpstreamRequestID,
		AttemptNo:             rec.AttemptNo,
		LatencyMS:             rec.LatencyMS,
		InputTokens:           rec.InputTokens,
		OutputTokens:          rec.OutputTokens,
		UsageEstimated:        rec.UsageEstimated,
		UpstreamErrors:        opsErrorLogEventsFromGateway(rec.UpstreamErrors),
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
	if s.runtime.opsErrorLogRecorder != nil {
		_ = s.runtime.opsErrorLogRecorder.enqueue(req)
		return
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), defaultOpsErrorLogWriteTimeout)
	defer cancel()
	if err := s.runtime.opsErrorLogs.RecordError(writeCtx, req); err != nil && s.runtime.logger != nil {
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
	rec.SourceEndpoint = gatewayEvidenceEndpoint(ctx, rec.SourceEndpoint)
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
	for key, value := range rec.DiagnosticMetadata {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		metadata[key] = value
	}
	if _, err := rt.operations.RecordSystemLog(ctx, operationscontract.RecordSystemLogRequest{
		Level:     level,
		Message:   message,
		Source:    "gateway",
		RequestID: rec.RequestID,
		TraceID:   traceIDFromContext(ctx),
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

// opsErrorLogShouldRecord decides whether a gateway failure is
// operator-actionable enough to persist in ops_error_logs. Any upstream HTTP
// response in the 4xx-5xx range is recorded because operators need the
// provider's exact rejection reason. Transport-level failures, stream
// interruptions, and scheduler no-available-account decisions are also
// recorded. Pure client-side failures with no operator action stay on
// usage_log rows only.
func opsErrorLogShouldRecord(class string, status int) bool {
	if status >= 400 && status < 600 {
		return true
	}
	switch class {
	case "server_bad", "network_error", "no_available_account", "stream_interrupted", "stream_idle_timeout":
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
	filter, err := opsErrorLogListFilterFromRequest(r)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.VALIDATIONFAILED, err.Error(), requestID)
		return
	}
	page, pageSize, err := parseOpsErrorLogsPagination(r)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.VALIDATIONFAILED, err.Error(), requestID)
		return
	}
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

// handleListAdminOpsErrorLogFingerprints serves
// GET /api/v1/admin/ops/error-logs/fingerprints.
func (s *Server) handleListAdminOpsErrorLogFingerprints(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeJSONString(w, http.StatusForbidden, `{"error":{"code":"FORBIDDEN","message":"admin access required"},"request_id":"`+requestID+`"}`)
		return
	}
	if s.runtime == nil || s.runtime.opsErrorLogs == nil {
		writeJSONString(w, http.StatusServiceUnavailable, `{"error":{"code":"UNAVAILABLE","message":"ops error logs unavailable"},"request_id":"`+requestID+`"}`)
		return
	}
	filter, err := opsErrorLogListFilterFromRequest(r)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.VALIDATIONFAILED, err.Error(), requestID)
		return
	}
	limit, err := parseOpsErrorLogFingerprintLimit(r)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.VALIDATIONFAILED, err.Error(), requestID)
		return
	}
	res, err := s.runtime.opsErrorLogs.ListFingerprints(r.Context(), opserrorlogscontract.FingerprintFilter{
		ListFilter: filter,
		Limit:      limit,
	})
	if err != nil {
		writeJSONString(w, http.StatusInternalServerError, `{"error":{"code":"INTERNAL_ERROR","message":"failed to list ops error log fingerprints"},"request_id":"`+requestID+`"}`)
		return
	}
	payload := map[string]any{
		"data":       opsErrorLogFingerprintsToDTOs(res.Items),
		"meta":       opsErrorLogFingerprintMetaToDTO(res),
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

func opsErrorLogListFilterFromRequest(r *http.Request) (opserrorlogscontract.ListFilter, error) {
	q := r.URL.Query()
	filter := opserrorlogscontract.ListFilter{
		Resolution:     opserrorlogscontract.Resolution(strings.TrimSpace(q.Get("resolution"))),
		ErrorClass:     strings.TrimSpace(q.Get("error_class")),
		ErrorPhase:     strings.TrimSpace(q.Get("error_phase")),
		ErrorOwner:     strings.TrimSpace(q.Get("error_owner")),
		Platform:       strings.TrimSpace(q.Get("platform")),
		SourceEndpoint: strings.TrimSpace(q.Get("source_endpoint")),
		Model:          strings.TrimSpace(q.Get("model")),
		Query:          strings.TrimSpace(q.Get("q")),
	}
	if filter.Resolution != "" && !validOpsErrorLogResolution(filter.Resolution) {
		return opserrorlogscontract.ListFilter{}, errors.New("invalid resolution")
	}
	if id, ok, err := parseOptionalPositiveInt(q.Get("user_id"), "user_id"); err != nil {
		return opserrorlogscontract.ListFilter{}, err
	} else if ok {
		filter.UserID = &id
	}
	if id, ok, err := parseOptionalPositiveInt(q.Get("account_id"), "account_id"); err != nil {
		return opserrorlogscontract.ListFilter{}, err
	} else if ok {
		filter.AccountID = &id
	}
	if id, ok, err := parseOptionalPositiveInt(q.Get("provider_id"), "provider_id"); err != nil {
		return opserrorlogscontract.ListFilter{}, err
	} else if ok {
		filter.ProviderID = &id
	}
	if status, ok, err := parseOptionalHTTPStatus(q.Get("status_min"), "status_min"); err != nil {
		return opserrorlogscontract.ListFilter{}, err
	} else if ok {
		filter.StatusCodeMin = &status
	}
	if status, ok, err := parseOptionalHTTPStatus(q.Get("status_max"), "status_max"); err != nil {
		return opserrorlogscontract.ListFilter{}, err
	} else if ok {
		filter.StatusCodeMax = &status
	}
	if filter.StatusCodeMin != nil && filter.StatusCodeMax != nil && *filter.StatusCodeMin > *filter.StatusCodeMax {
		return opserrorlogscontract.ListFilter{}, errors.New("status_min must be <= status_max")
	}
	if start, err := parseOptionalRFC3339(q.Get("start")); err != nil {
		return opserrorlogscontract.ListFilter{}, errors.New("invalid start timestamp")
	} else if start != nil {
		filter.From = start
	}
	if end, err := parseOptionalRFC3339(q.Get("end")); err != nil {
		return opserrorlogscontract.ListFilter{}, errors.New("invalid end timestamp")
	} else if end != nil {
		filter.To = end
	}
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		return opserrorlogscontract.ListFilter{}, errors.New("start must be before end")
	}
	return filter, nil
}

func validOpsErrorLogResolution(resolution opserrorlogscontract.Resolution) bool {
	switch resolution {
	case opserrorlogscontract.ResolutionOpen,
		opserrorlogscontract.ResolutionInvestigating,
		opserrorlogscontract.ResolutionResolved,
		opserrorlogscontract.ResolutionMuted:
		return true
	default:
		return false
	}
}

func parseOptionalPositiveInt(raw string, field string) (int, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, false, errors.New("invalid " + field)
	}
	return n, true, nil
}

func parseOptionalHTTPStatus(raw string, field string) (int, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 100 || n > 599 {
		return 0, false, errors.New("invalid " + field)
	}
	return n, true, nil
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

func parseOpsErrorLogsPagination(r *http.Request) (page, pageSize int, err error) {
	page = 1
	pageSize = 20
	if v := strings.TrimSpace(r.URL.Query().Get("page")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return 0, 0, errors.New("invalid page")
		}
		page = n
	}
	if v := strings.TrimSpace(r.URL.Query().Get("page_size")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return 0, 0, errors.New("invalid page_size")
		}
		pageSize = n
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize, nil
}

func parseOpsErrorLogFingerprintLimit(r *http.Request) (int, error) {
	value := strings.TrimSpace(r.URL.Query().Get("limit"))
	if value == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 {
		return 0, errors.New("invalid limit")
	}
	if limit > 100 {
		limit = 100
	}
	return limit, nil
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
	if entry.StreamCompletionState != "" {
		dto["stream_completion_state"] = entry.StreamCompletionState
	}
	if entry.ResolvedAt != nil {
		dto["resolved_at"] = entry.ResolvedAt.Format(time.RFC3339Nano)
	}
	if entry.ResolvedByID != nil {
		dto["resolved_by_user_id"] = strconv.Itoa(*entry.ResolvedByID)
	}
	return dto
}

func opsErrorLogFingerprintsToDTOs(items []opserrorlogscontract.FingerprintSummary) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, opsErrorLogFingerprintToDTO(item))
	}
	return out
}

func opsErrorLogFingerprintToDTO(item opserrorlogscontract.FingerprintSummary) map[string]any {
	dto := map[string]any{
		"fingerprint":           item.Fingerprint,
		"count":                 item.Count,
		"open_count":            item.OpenCount,
		"investigating_count":   item.InvestigatingCount,
		"resolved_count":        item.ResolvedCount,
		"muted_count":           item.MutedCount,
		"first_occurred_at":     item.FirstOccurredAt.Format(time.RFC3339Nano),
		"last_occurred_at":      item.LastOccurredAt.Format(time.RFC3339Nano),
		"source_endpoint":       item.SourceEndpoint,
		"target_protocol":       item.TargetProtocol,
		"model":                 item.Model,
		"status_class":          item.StatusClass,
		"error_class":           item.ErrorClass,
		"error_phase":           item.ErrorPhase,
		"error_owner":           item.ErrorOwner,
		"error_source":          item.ErrorSource,
		"message_pattern":       item.MessagePattern,
		"example_error_message": item.ExampleErrorMessage,
	}
	if item.ExampleEntryID > 0 {
		dto["example_error_log_id"] = strconv.FormatInt(item.ExampleEntryID, 10)
	}
	if item.ExampleRequestID != "" {
		dto["example_request_id"] = item.ExampleRequestID
	}
	if item.StatusCode != nil {
		dto["status_code"] = *item.StatusCode
	}
	return dto
}

func opsErrorLogFingerprintMetaToDTO(res opserrorlogscontract.FingerprintResult) map[string]any {
	dto := map[string]any{
		"total":     res.Total,
		"scanned":   res.Scanned,
		"truncated": res.Truncated,
	}
	if res.WindowStart != nil {
		dto["window_start"] = res.WindowStart.Format(time.RFC3339Nano)
	}
	if res.WindowEnd != nil {
		dto["window_end"] = res.WindowEnd.Format(time.RFC3339Nano)
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
