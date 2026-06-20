package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	channelmonitorscontract "github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/contract"
	channelmonitorsservice "github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func toAPIChannelMonitorRequest(req channelmonitorscontract.CustomRequest) apiopenapi.ChannelMonitorRequest {
	out := apiopenapi.ChannelMonitorRequest{}
	if req.Method != "" {
		out.Method = &req.Method
	}
	if req.URL != "" {
		out.Url = &req.URL
	}
	if len(req.Headers) > 0 {
		headers := make(map[string]string, len(req.Headers))
		for key, value := range req.Headers {
			headers[key] = value
		}
		out.Headers = &headers
	}
	if req.Body != "" {
		out.Body = &req.Body
	}
	if len(req.ExpectedStatusCodes) > 0 {
		codes := make([]int64, 0, len(req.ExpectedStatusCodes))
		for _, code := range req.ExpectedStatusCodes {
			codes = append(codes, int64(code))
		}
		out.ExpectedStatusCodes = &codes
	}
	if req.ResponseJSONPath != "" {
		out.ResponseJsonPath = &req.ResponseJSONPath
	}
	if req.ResponseContains != "" {
		out.ResponseContains = &req.ResponseContains
	}
	return out
}

func fromAPIChannelMonitorRequest(payload *apiopenapi.ChannelMonitorRequest) (channelmonitorscontract.CustomRequest, bool) {
	if payload == nil {
		return channelmonitorscontract.CustomRequest{}, true
	}
	codes, ok := nonNegativeIntSliceFromInt64Ptr(payload.ExpectedStatusCodes)
	if !ok {
		return channelmonitorscontract.CustomRequest{}, false
	}
	return channelmonitorscontract.CustomRequest{
		Method:              openapiOptionalString(payload.Method),
		URL:                 openapiOptionalString(payload.Url),
		Headers:             openapiOptionalStringMap(payload.Headers),
		Body:                openapiOptionalString(payload.Body),
		ExpectedStatusCodes: codes,
		ResponseJSONPath:    openapiOptionalString(payload.ResponseJsonPath),
		ResponseContains:    openapiOptionalString(payload.ResponseContains),
	}, true
}

func toAPIChannelMonitor(def channelmonitorscontract.Definition) apiopenapi.ChannelMonitor {
	return apiopenapi.ChannelMonitor{
		Id:              int64(def.ID),
		Name:            def.Name,
		Enabled:         def.Enabled,
		Scope:           apiopenapi.ChannelMonitorScope(def.Scope),
		ScopeRef:        def.ScopeRef,
		IntervalSeconds: int64(def.Interval),
		Model:           def.Model,
		Request:         toAPIChannelMonitorRequest(def.Request),
		CreatedAt:       def.CreatedAt.UTC(),
		UpdatedAt:       def.UpdatedAt.UTC(),
	}
}

func (s *Server) handleListAdminChannelMonitors(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	entries, err := s.runtime.channelMonitors.ListDefinitionsWithSummary(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list channel monitors", requestID)
		return
	}
	data := make([]apiopenapi.ChannelMonitor, 0, len(entries))
	for _, entry := range entries {
		payload := toAPIChannelMonitor(entry.Definition)
		if entry.LastRun != nil {
			at := entry.LastRun.At.UTC()
			ok := entry.LastRun.OK
			latency := entry.LastRun.LatencyMS
			payload.LastRunAt = &at
			payload.LastRunOk = &ok
			payload.LastRunLatencyMs = &latency
		}
		if entry.Recent != nil {
			windowDays := entry.Recent.WindowDays
			sampleCount := entry.Recent.SampleCount
			rate := float32(entry.Recent.SuccessRate())
			payload.RecentUptimeWindowDays = &windowDays
			payload.RecentUptimeSampleCount = &sampleCount
			payload.RecentUptimeSuccessRate = &rate
		}
		data = append(data, payload)
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.ChannelMonitorListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminChannelMonitor(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreateChannelMonitorRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid channel monitor request", requestID)
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	interval, ok := nonNegativeIntFromInt64Ptr(body.IntervalSeconds)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid channel monitor request", requestID)
		return
	}
	request, ok := fromAPIChannelMonitorRequest(body.Request)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid channel monitor request", requestID)
		return
	}
	def, err := s.runtime.channelMonitors.CreateDefinition(r.Context(), channelmonitorscontract.CreateDefinition{
		Name:     body.Name,
		Enabled:  enabled,
		Scope:    channelmonitorscontract.Scope(body.Scope),
		ScopeRef: openapiOptionalString(body.ScopeRef),
		Interval: interval,
		Model:    openapiOptionalString(body.Model),
		Request:  request,
	})
	if err != nil {
		s.writeChannelMonitorError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "channel_monitor.create", "channel_monitor", strconv.Itoa(def.ID), nil, map[string]any{
		"name":  def.Name,
		"scope": def.Scope,
	}))
	writeJSONAny(w, http.StatusCreated, apiopenapi.ChannelMonitorResponse{
		Data:      toAPIChannelMonitor(def),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminChannelMonitor(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.UpdateChannelMonitorRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid channel monitor request", requestID)
		return
	}
	interval, ok := intPtrFromInt64Ptr(body.IntervalSeconds)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid channel monitor request", requestID)
		return
	}
	input := channelmonitorscontract.UpdateDefinition{
		Name:     body.Name,
		Enabled:  body.Enabled,
		ScopeRef: body.ScopeRef,
		Interval: interval,
		Model:    body.Model,
	}
	if body.Scope != nil {
		scope := channelmonitorscontract.Scope(*body.Scope)
		input.Scope = &scope
	}
	if body.Request != nil {
		req, ok := fromAPIChannelMonitorRequest(body.Request)
		if !ok {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid channel monitor request", requestID)
			return
		}
		input.Request = &req
	}
	def, err := s.runtime.channelMonitors.UpdateDefinition(r.Context(), id, input)
	if err != nil {
		s.writeChannelMonitorError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "channel_monitor.update", "channel_monitor", strconv.Itoa(def.ID), nil, map[string]any{
		"name":    def.Name,
		"enabled": def.Enabled,
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.ChannelMonitorResponse{
		Data:      toAPIChannelMonitor(def),
		RequestId: requestID,
	})
}

func (s *Server) handleDeleteAdminChannelMonitor(w http.ResponseWriter, r *http.Request) {
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
	if err := s.runtime.channelMonitors.DeleteDefinition(r.Context(), id); err != nil {
		s.writeChannelMonitorError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "channel_monitor.delete", "channel_monitor", strconv.Itoa(id), nil, nil))
	writeJSONAny(w, http.StatusOK, deleteResponse(true, requestID))
}

func (s *Server) handleListAdminChannelMonitorRuns(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid channel monitor id", requestID)
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	runs, err := s.runtime.channelMonitors.ListRuns(r.Context(), id, limit)
	if err != nil {
		s.writeChannelMonitorError(w, err, requestID)
		return
	}
	data := make([]apiopenapi.ChannelMonitorRun, 0, len(runs))
	for _, run := range runs {
		data = append(data, toAPIChannelMonitorRun(run))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.ChannelMonitorRunListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) writeChannelMonitorError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, channelmonitorscontract.ErrNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "channel monitor not found", requestID)
	case errors.Is(err, channelmonitorsservice.ErrDisabled):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "channel monitor is disabled", requestID)
	case errors.Is(err, channelmonitorsservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid channel monitor request", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to process channel monitor request", requestID)
	}
}
