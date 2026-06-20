package httpserver

import (
	"net/http"
	"strconv"

	channelmonitorscontract "github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func toAPIChannelMonitorTemplate(tpl channelmonitorscontract.Template) apiopenapi.ChannelMonitorTemplate {
	return apiopenapi.ChannelMonitorTemplate{
		Id:          int64(tpl.ID),
		Name:        tpl.Name,
		Description: tpl.Description,
		Request:     toAPIChannelMonitorRequest(tpl.Request),
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
	data := make([]apiopenapi.ChannelMonitorTemplate, 0, len(templates))
	for _, tpl := range templates {
		data = append(data, toAPIChannelMonitorTemplate(tpl))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.ChannelMonitorTemplateListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
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
	var body apiopenapi.CreateChannelMonitorTemplateRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid channel monitor template request", requestID)
		return
	}
	request, ok := fromAPIChannelMonitorRequest(body.Request)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid channel monitor template request", requestID)
		return
	}
	tpl, err := s.runtime.channelMonitors.CreateTemplate(r.Context(), channelmonitorscontract.CreateTemplate{
		Name:        body.Name,
		Description: openapiOptionalString(body.Description),
		Request:     request,
	})
	if err != nil {
		s.writeChannelMonitorError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "channel_monitor_template.create", "channel_monitor_template", strconv.Itoa(tpl.ID), nil, map[string]any{"name": tpl.Name}))
	writeJSONAny(w, http.StatusCreated, apiopenapi.ChannelMonitorTemplateResponse{
		Data:      toAPIChannelMonitorTemplate(tpl),
		RequestId: requestID,
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
	var body apiopenapi.UpdateChannelMonitorTemplateRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid channel monitor template request", requestID)
		return
	}
	input := channelmonitorscontract.UpdateTemplate{
		Name:        body.Name,
		Description: body.Description,
	}
	if body.Request != nil {
		req, ok := fromAPIChannelMonitorRequest(body.Request)
		if !ok {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid channel monitor template request", requestID)
			return
		}
		input.Request = &req
	}
	tpl, err := s.runtime.channelMonitors.UpdateTemplate(r.Context(), id, input)
	if err != nil {
		s.writeChannelMonitorError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "channel_monitor_template.update", "channel_monitor_template", strconv.Itoa(tpl.ID), nil, map[string]any{"name": tpl.Name}))
	writeJSONAny(w, http.StatusOK, apiopenapi.ChannelMonitorTemplateResponse{
		Data:      toAPIChannelMonitorTemplate(tpl),
		RequestId: requestID,
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
	writeJSONAny(w, http.StatusOK, deleteResponse(true, requestID))
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
	var body apiopenapi.ApplyChannelMonitorTemplateRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid channel monitor template apply request", requestID)
		return
	}
	monitorIDs := make([]int, 0, len(body.MonitorIds))
	for _, monitorID := range body.MonitorIds {
		converted, ok := positiveIntFromInt64(monitorID)
		if !ok {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid channel monitor template apply request", requestID)
			return
		}
		monitorIDs = append(monitorIDs, converted)
	}
	defs, err := s.runtime.channelMonitors.ApplyTemplate(r.Context(), id, monitorIDs)
	if err != nil {
		s.writeChannelMonitorError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "channel_monitor_template.apply", "channel_monitor_template", strconv.Itoa(id), nil, map[string]any{"applied": len(defs)}))
	data := make([]apiopenapi.ChannelMonitor, 0, len(defs))
	for _, def := range defs {
		data = append(data, toAPIChannelMonitor(def))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.ChannelMonitorListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}
