package httpserver

import (
	"net/http"
	"strconv"

	channelmonitorscontract "github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func toChannelMonitorRunPayload(run channelmonitorscontract.RunResult) map[string]any {
	results := make([]map[string]any, 0, len(run.Results))
	for _, result := range run.Results {
		entry := map[string]any{
			"account_id":   result.AccountID,
			"account_name": result.AccountName,
			"provider_id":  result.ProviderID,
			"model":        result.Model,
			"ok":           result.OK,
			"status_code":  result.StatusCode,
			"latency_ms":   result.LatencyMS,
		}
		if result.ErrorClass != "" {
			entry["error_class"] = result.ErrorClass
		}
		if result.Metadata != nil {
			entry["metadata"] = result.Metadata
		}
		results = append(results, entry)
	}
	return map[string]any{
		"id":            run.ID,
		"monitor_id":    run.MonitorID,
		"run_id":        run.RunID,
		"ok":            run.OK,
		"checked_count": run.CheckedCount,
		"ok_count":      run.OKCount,
		"latency_ms":    run.LatencyMS,
		"trigger":       run.Trigger,
		"results":       results,
		"created_at":    run.CreatedAt.UTC(),
	}
}

func (s *Server) handleRunAdminChannelMonitor(w http.ResponseWriter, r *http.Request) {
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
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid channel monitor id", requestID)
		return
	}
	run, err := s.runtime.channelMonitors.RunDefinition(r.Context(), id, s.channelMonitorRunnerDependencies(), channelmonitorscontract.TriggerManual)
	if err != nil {
		s.writeChannelMonitorError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "channel_monitor.run", "channel_monitor", strconv.Itoa(run.MonitorID), nil, map[string]any{
		"checked": run.CheckedCount,
		"ok":      run.OKCount,
	}))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       toChannelMonitorRunPayload(run),
		"request_id": requestID,
	})
}

func (s *Server) channelMonitorRunnerDependencies() channelmonitorscontract.RunnerDependencies {
	return channelmonitorscontract.RunnerDependencies{
		Accounts:  s.runtime.accounts,
		Providers: s.runtime.providers,
		Models:    s.runtime.models,
		Adapter:   s.runtime.adapters,
	}
}
