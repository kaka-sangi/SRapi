package httpserver

import (
	"net/http"
	"strconv"

	channelmonitorscontract "github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func toAPIChannelMonitorRun(run channelmonitorscontract.RunResult) apiopenapi.ChannelMonitorRun {
	results := make([]apiopenapi.ChannelMonitorCheckResult, 0, len(run.Results))
	for _, result := range run.Results {
		entry := apiopenapi.ChannelMonitorCheckResult{
			AccountId:   int64(result.AccountID),
			AccountName: result.AccountName,
			ProviderId:  int64(result.ProviderID),
			Model:       result.Model,
			Ok:          result.OK,
			StatusCode:  int64(result.StatusCode),
			LatencyMs:   int64(result.LatencyMS),
		}
		if result.ErrorClass != "" {
			entry.ErrorClass = &result.ErrorClass
		}
		if result.Metadata != nil {
			metadata := apiopenapi.JsonObject(result.Metadata)
			entry.Metadata = &metadata
		}
		results = append(results, entry)
	}
	return apiopenapi.ChannelMonitorRun{
		Id:           int64(run.ID),
		MonitorId:    int64(run.MonitorID),
		RunId:        run.RunID,
		Ok:           run.OK,
		CheckedCount: int64(run.CheckedCount),
		OkCount:      int64(run.OKCount),
		LatencyMs:    int64(run.LatencyMS),
		Trigger:      run.Trigger,
		Results:      results,
		CreatedAt:    run.CreatedAt.UTC(),
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
	writeJSONAny(w, http.StatusOK, apiopenapi.ChannelMonitorRunResponse{
		Data:      toAPIChannelMonitorRun(run),
		RequestId: requestID,
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
