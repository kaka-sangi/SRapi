package httpserver

import (
	"net/http"
	"strconv"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Server) handleListAdminProxies(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	status := accountcontract.ProxyStatus(r.URL.Query().Get("status"))
	items, err := s.runtime.accounts.ListProxies(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list proxies", requestID)
		return
	}
	data := make([]apiopenapi.ProxyDefinition, 0, len(items))
	for _, item := range items {
		if status != "" && item.Status != status {
			continue
		}
		data = append(data, toAPIProxyDefinition(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.ProxyDefinitionListResponse{
		Data:       data,
		Pagination: paginationWithRequest(r, len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminProxy(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreateProxyDefinitionRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid proxy request", requestID)
		return
	}
	proxy, err := s.runtime.accounts.CreateProxy(r.Context(), accountcontract.CreateProxyRequest{
		Name:     body.Name,
		Type:     accountcontract.ProxyType(body.Type),
		URL:      optionalStringValue(body.Url),
		Status:   toProxyStatusPtr(body.Status),
		Metadata: jsonObjectToMap(body.Metadata),
	})
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid proxy request", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "proxy.create", "proxy", strconv.Itoa(proxy.ID), nil, proxyAuditSnapshot(proxy)))
	writeJSONAny(w, http.StatusCreated, apiopenapi.ProxyDefinitionResponse{
		Data:      toAPIProxyDefinition(proxy),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminProxy(w http.ResponseWriter, r *http.Request) {
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
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid proxy id", requestID)
		return
	}
	before, err := s.runtime.accounts.FindProxyByID(r.Context(), id)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "proxy not found", requestID)
		return
	}
	var body apiopenapi.UpdateProxyDefinitionRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid proxy request", requestID)
		return
	}
	updated, err := s.runtime.accounts.UpdateProxy(r.Context(), id, accountcontract.UpdateProxyRequest{
		Name:     body.Name,
		Type:     toProxyTypePtr(body.Type),
		URL:      body.Url,
		Status:   toProxyStatusPtr(body.Status),
		Metadata: jsonObjectToMapPtr(body.Metadata),
	})
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid proxy request", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "proxy.update", "proxy", strconv.Itoa(updated.ID), proxyAuditSnapshot(before), proxyAuditSnapshot(updated)))
	writeJSONAny(w, http.StatusOK, apiopenapi.ProxyDefinitionResponse{
		Data:      toAPIProxyDefinition(updated),
		RequestId: requestID,
	})
}
