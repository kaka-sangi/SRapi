package httpserver

import (
	"errors"
	"net/http"
	"strings"
	"time"

	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	usageservice "github.com/srapi/srapi/apps/api/internal/modules/usage/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// handleCleanupAdminUsage performs an operator on-demand, bounded deletion of
// usage records, complementing the background retention worker (which only
// purges by age). It mirrors the system-log cleanup contract: admin + CSRF
// gated, requires at least one bounding filter, caps deletion at max_delete,
// supports a dry-run preview, and always records a safe audit summary.
func (s *Server) handleCleanupAdminUsage(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.UsageCleanupRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid usage cleanup request", requestID)
		return
	}
	result, err := s.runtime.usage.CleanupLogs(r.Context(), usageCleanupFilterFromAPI(body))
	if err != nil {
		if errors.Is(err, usageservice.ErrInvalidInput) {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "usage cleanup requires at least one bounded filter", requestID)
			return
		}
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to cleanup usage records", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "usage_log.cleanup", "usage_log", "bulk", nil, usageCleanupAuditSnapshot(body, result)))
	writeJSONAny(w, http.StatusOK, apiopenapi.UsageCleanupResponse{
		Data:      toAPIUsageCleanupResult(result),
		RequestId: requestID,
	})
}

func usageCleanupFilterFromAPI(body apiopenapi.UsageCleanupRequest) usagecontract.CleanupFilter {
	var start, end *time.Time
	if body.Start != nil {
		startValue := (*body.Start).UTC()
		start = &startValue
	}
	if body.End != nil {
		endValue := (*body.End).UTC()
		end = &endValue
	}
	var maxDelete int
	if body.MaxDelete != nil {
		maxDelete = *body.MaxDelete
	}
	var dryRun bool
	if body.DryRun != nil {
		dryRun = *body.DryRun
	}
	return usagecontract.CleanupFilter{
		Model:     optionalStringValue(body.Model),
		Start:     start,
		End:       end,
		DryRun:    dryRun,
		MaxDelete: maxDelete,
	}
}

func toAPIUsageCleanupResult(result usagecontract.CleanupResult) apiopenapi.UsageCleanupResult {
	return apiopenapi.UsageCleanupResult{
		Deleted:   result.Deleted,
		DryRun:    result.DryRun,
		Limited:   result.Limited,
		Matched:   result.Matched,
		MaxDelete: result.MaxDelete,
	}
}

func usageCleanupAuditSnapshot(body apiopenapi.UsageCleanupRequest, result usagecontract.CleanupResult) map[string]any {
	snapshot := map[string]any{
		"dry_run":    result.DryRun,
		"limited":    result.Limited,
		"matched":    result.Matched,
		"deleted":    result.Deleted,
		"max_delete": result.MaxDelete,
	}
	if model := strings.TrimSpace(optionalStringValue(body.Model)); model != "" {
		snapshot["model"] = model
	}
	if body.Start != nil {
		snapshot["start"] = (*body.Start).UTC()
	}
	if body.End != nil {
		snapshot["end"] = (*body.End).UTC()
	}
	return snapshot
}
