package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	channelmonitorscontract "github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/contract"
	channelmonitorsservice "github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

type channelMonitorRequestPayload struct {
	Method              string            `json:"method"`
	URL                 string            `json:"url"`
	Headers             map[string]string `json:"headers"`
	Body                string            `json:"body"`
	ExpectedStatusCodes []int             `json:"expected_status_codes"`
	ResponseJSONPath    string            `json:"response_json_path"`
	ResponseContains    string            `json:"response_contains"`
}

type channelMonitorPayload struct {
	ID               int                          `json:"id"`
	Name             string                       `json:"name"`
	Enabled          bool                         `json:"enabled"`
	Scope            string                       `json:"scope"`
	ScopeRef         string                       `json:"scope_ref"`
	IntervalSeconds  int                          `json:"interval_seconds"`
	Model            string                       `json:"model"`
	Request          channelMonitorRequestPayload `json:"request"`
	CreatedAt        time.Time                    `json:"created_at"`
	UpdatedAt        time.Time                    `json:"updated_at"`
	LastRunAt        *time.Time                   `json:"last_run_at,omitempty"`
	LastRunOK        *bool                        `json:"last_run_ok,omitempty"`
	LastRunLatencyMS *int                         `json:"last_run_latency_ms,omitempty"`
}

type createChannelMonitorRequest struct {
	Name            string                        `json:"name"`
	Enabled         *bool                         `json:"enabled"`
	Scope           string                        `json:"scope"`
	ScopeRef        string                        `json:"scope_ref"`
	IntervalSeconds int                           `json:"interval_seconds"`
	Model           string                        `json:"model"`
	Request         *channelMonitorRequestPayload `json:"request"`
}

type updateChannelMonitorRequest struct {
	Name            *string                       `json:"name"`
	Enabled         *bool                         `json:"enabled"`
	Scope           *string                       `json:"scope"`
	ScopeRef        *string                       `json:"scope_ref"`
	IntervalSeconds *int                          `json:"interval_seconds"`
	Model           *string                       `json:"model"`
	Request         *channelMonitorRequestPayload `json:"request"`
}

func toChannelMonitorRequestPayload(req channelmonitorscontract.CustomRequest) channelMonitorRequestPayload {
	headers := req.Headers
	if headers == nil {
		headers = map[string]string{}
	}
	codes := req.ExpectedStatusCodes
	if codes == nil {
		codes = []int{}
	}
	return channelMonitorRequestPayload{
		Method:              req.Method,
		URL:                 req.URL,
		Headers:             headers,
		Body:                req.Body,
		ExpectedStatusCodes: codes,
		ResponseJSONPath:    req.ResponseJSONPath,
		ResponseContains:    req.ResponseContains,
	}
}

func fromChannelMonitorRequestPayload(payload *channelMonitorRequestPayload) channelmonitorscontract.CustomRequest {
	if payload == nil {
		return channelmonitorscontract.CustomRequest{}
	}
	return channelmonitorscontract.CustomRequest{
		Method:              payload.Method,
		URL:                 payload.URL,
		Headers:             payload.Headers,
		Body:                payload.Body,
		ExpectedStatusCodes: payload.ExpectedStatusCodes,
		ResponseJSONPath:    payload.ResponseJSONPath,
		ResponseContains:    payload.ResponseContains,
	}
}

func toChannelMonitorPayload(def channelmonitorscontract.Definition) channelMonitorPayload {
	return channelMonitorPayload{
		ID:              def.ID,
		Name:            def.Name,
		Enabled:         def.Enabled,
		Scope:           string(def.Scope),
		ScopeRef:        def.ScopeRef,
		IntervalSeconds: def.Interval,
		Model:           def.Model,
		Request:         toChannelMonitorRequestPayload(def.Request),
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
	data := make([]channelMonitorPayload, 0, len(entries))
	for _, entry := range entries {
		payload := toChannelMonitorPayload(entry.Definition)
		if entry.LastRun != nil {
			at := entry.LastRun.At.UTC()
			ok := entry.LastRun.OK
			latency := entry.LastRun.LatencyMS
			payload.LastRunAt = &at
			payload.LastRunOK = &ok
			payload.LastRunLatencyMS = &latency
		}
		data = append(data, payload)
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       data,
		"pagination": pg,
		"request_id": requestID,
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
	var body createChannelMonitorRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid channel monitor request", requestID)
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	def, err := s.runtime.channelMonitors.CreateDefinition(r.Context(), channelmonitorscontract.CreateDefinition{
		Name:     body.Name,
		Enabled:  enabled,
		Scope:    channelmonitorscontract.Scope(body.Scope),
		ScopeRef: body.ScopeRef,
		Interval: body.IntervalSeconds,
		Model:    body.Model,
		Request:  fromChannelMonitorRequestPayload(body.Request),
	})
	if err != nil {
		s.writeChannelMonitorError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "channel_monitor.create", "channel_monitor", strconv.Itoa(def.ID), nil, map[string]any{
		"name":  def.Name,
		"scope": def.Scope,
	}))
	writeJSONAny(w, http.StatusCreated, map[string]any{
		"data":       toChannelMonitorPayload(def),
		"request_id": requestID,
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
	var body updateChannelMonitorRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid channel monitor request", requestID)
		return
	}
	input := channelmonitorscontract.UpdateDefinition{
		Name:     body.Name,
		Enabled:  body.Enabled,
		ScopeRef: body.ScopeRef,
		Interval: body.IntervalSeconds,
		Model:    body.Model,
	}
	if body.Scope != nil {
		scope := channelmonitorscontract.Scope(*body.Scope)
		input.Scope = &scope
	}
	if body.Request != nil {
		req := fromChannelMonitorRequestPayload(body.Request)
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
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       toChannelMonitorPayload(def),
		"request_id": requestID,
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
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"id": id, "deleted": true},
		"request_id": requestID,
	})
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
	data := make([]map[string]any, 0, len(runs))
	for _, run := range runs {
		data = append(data, toChannelMonitorRunPayload(run))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       data,
		"pagination": pg,
		"request_id": requestID,
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
