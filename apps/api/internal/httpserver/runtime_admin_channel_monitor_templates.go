package httpserver

import (
	"net/http"
	"strconv"
	"time"

	channelmonitorscontract "github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

type channelMonitorTemplatePayload struct {
	ID          int                          `json:"id"`
	Name        string                       `json:"name"`
	Description string                       `json:"description"`
	Request     channelMonitorRequestPayload `json:"request"`
	CreatedAt   time.Time                    `json:"created_at"`
	UpdatedAt   time.Time                    `json:"updated_at"`
}

type createChannelMonitorTemplateRequest struct {
	Name        string                        `json:"name"`
	Description string                        `json:"description"`
	Request     *channelMonitorRequestPayload `json:"request"`
}

type updateChannelMonitorTemplateRequest struct {
	Name        *string                       `json:"name"`
	Description *string                       `json:"description"`
	Request     *channelMonitorRequestPayload `json:"request"`
}

type applyChannelMonitorTemplateRequest struct {
	MonitorIDs []int `json:"monitor_ids"`
}

func toChannelMonitorTemplatePayload(tpl channelmonitorscontract.Template) channelMonitorTemplatePayload {
	return channelMonitorTemplatePayload{
		ID:          tpl.ID,
		Name:        tpl.Name,
		Description: tpl.Description,
		Request:     toChannelMonitorRequestPayload(tpl.Request),
		CreatedAt:   tpl.CreatedAt.UTC(),
		UpdatedAt:   tpl.UpdatedAt.UTC(),
	}
}

func (s *Server) handleListAdminChannelMonitorTemplates(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	templates, err := s.runtime.channelMonitors.ListTemplates(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list channel monitor templates", requestID)
		return
	}
	data := make([]channelMonitorTemplatePayload, 0, len(templates))
	for _, tpl := range templates {
		data = append(data, toChannelMonitorTemplatePayload(tpl))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       data,
		"pagination": pg,
		"request_id": requestID,
	})
}

func (s *Server) handleCreateAdminChannelMonitorTemplate(w http.ResponseWriter, r *http.Request) {
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
	var body createChannelMonitorTemplateRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid channel monitor template request", requestID)
		return
	}
	tpl, err := s.runtime.channelMonitors.CreateTemplate(r.Context(), channelmonitorscontract.CreateTemplate{
		Name:        body.Name,
		Description: body.Description,
		Request:     fromChannelMonitorRequestPayload(body.Request),
	})
	if err != nil {
		s.writeChannelMonitorError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "channel_monitor_template.create", "channel_monitor_template", strconv.Itoa(tpl.ID), nil, map[string]any{"name": tpl.Name}))
	writeJSONAny(w, http.StatusCreated, map[string]any{
		"data":       toChannelMonitorTemplatePayload(tpl),
		"request_id": requestID,
	})
}

func (s *Server) handleUpdateAdminChannelMonitorTemplate(w http.ResponseWriter, r *http.Request) {
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
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid channel monitor template id", requestID)
		return
	}
	var body updateChannelMonitorTemplateRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid channel monitor template request", requestID)
		return
	}
	input := channelmonitorscontract.UpdateTemplate{
		Name:        body.Name,
		Description: body.Description,
	}
	if body.Request != nil {
		req := fromChannelMonitorRequestPayload(body.Request)
		input.Request = &req
	}
	tpl, err := s.runtime.channelMonitors.UpdateTemplate(r.Context(), id, input)
	if err != nil {
		s.writeChannelMonitorError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "channel_monitor_template.update", "channel_monitor_template", strconv.Itoa(tpl.ID), nil, map[string]any{"name": tpl.Name}))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       toChannelMonitorTemplatePayload(tpl),
		"request_id": requestID,
	})
}

func (s *Server) handleDeleteAdminChannelMonitorTemplate(w http.ResponseWriter, r *http.Request) {
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
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid channel monitor template id", requestID)
		return
	}
	if err := s.runtime.channelMonitors.DeleteTemplate(r.Context(), id); err != nil {
		s.writeChannelMonitorError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "channel_monitor_template.delete", "channel_monitor_template", strconv.Itoa(id), nil, nil))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"id": id, "deleted": true},
		"request_id": requestID,
	})
}

func (s *Server) handleApplyAdminChannelMonitorTemplate(w http.ResponseWriter, r *http.Request) {
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
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid channel monitor template id", requestID)
		return
	}
	var body applyChannelMonitorTemplateRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid channel monitor template apply request", requestID)
		return
	}
	defs, err := s.runtime.channelMonitors.ApplyTemplate(r.Context(), id, body.MonitorIDs)
	if err != nil {
		s.writeChannelMonitorError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "channel_monitor_template.apply", "channel_monitor_template", strconv.Itoa(id), nil, map[string]any{"applied": len(defs)}))
	data := make([]channelMonitorPayload, 0, len(defs))
	for _, def := range defs {
		data = append(data, toChannelMonitorPayload(def))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       data,
		"pagination": pg,
		"request_id": requestID,
	})
}
