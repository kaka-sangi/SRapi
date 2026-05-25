package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	usersservice "github.com/srapi/srapi/apps/api/internal/modules/users/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Server) handleListAdminRoles(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	roles, err := s.runtime.users.ListRoles(r.Context())
	if err != nil {
		writeRoleServiceError(w, err, requestID)
		return
	}
	data := make([]apiopenapi.Role, 0, len(roles))
	for _, role := range roles {
		data = append(data, toAPIRole(role))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.RoleListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminRole(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreateRoleRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid role request", requestID)
		return
	}
	role, err := s.runtime.users.CreateRole(r.Context(), usersservice.CreateRoleRequest{
		Name:        string(body.Name),
		Description: optionalStringValue(body.Description),
		Permissions: body.Permissions,
	})
	if err != nil {
		writeRoleServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "role.create", "role", strconv.Itoa(role.ID), nil, map[string]any{
		"name":        role.Name,
		"description": role.Description,
		"permissions": role.Permissions,
	}))
	writeJSONAny(w, http.StatusCreated, apiopenapi.RoleResponse{
		Data:      toAPIRole(role),
		RequestId: requestID,
	})
}

func writeRoleServiceError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, usersservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid role request", requestID)
	case errors.Is(err, usersservice.ErrUserAlreadyExists):
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "role already exists", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "role service failed", requestID)
	}
}
