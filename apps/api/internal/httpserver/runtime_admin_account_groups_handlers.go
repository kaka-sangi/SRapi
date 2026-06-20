package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	groupratelimitscontract "github.com/srapi/srapi/apps/api/internal/modules/group_rate_limits/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Server) handleListAdminAccountGroups(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	groups, err := s.runtime.accounts.ListGroups(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list account groups", requestID)
		return
	}
	data := make([]apiopenapi.AccountGroup, 0, len(groups))
	for _, group := range groups {
		data = append(data, toAPIAccountGroup(group))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountGroupListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminAccountGroup(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreateAccountGroupRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid account group request", requestID)
		return
	}
	description := ""
	if body.Description != nil {
		description = *body.Description
	}
	group, err := s.runtime.accounts.CreateGroup(r.Context(), accountcontract.CreateGroupRequest{
		Name:           body.Name,
		Description:    description,
		ProviderScope:  jsonObjectToMap(body.ProviderScope),
		ModelScope:     jsonObjectToMap(body.ModelScope),
		StrategyHint:   body.StrategyHint,
		Status:         toAccountGroupStatusPtr(body.Status),
		RateMultiplier: body.RateMultiplier,
	})
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account group request", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "account_group.create", "account_group", strconv.Itoa(group.ID), nil, accountGroupAuditSnapshot(group)))
	writeJSONAny(w, http.StatusCreated, apiopenapi.AccountGroupResponse{
		Data:      toAPIAccountGroup(group),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminAccountGroup(w http.ResponseWriter, r *http.Request) {
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
	groupID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account group id", requestID)
		return
	}
	before, err := s.runtime.accounts.FindGroupByID(r.Context(), groupID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account group not found", requestID)
		return
	}
	var body apiopenapi.UpdateAccountGroupRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid account group update request", requestID)
		return
	}
	group, err := s.runtime.accounts.UpdateGroup(r.Context(), groupID, accountcontract.UpdateGroupRequest{
		Name:           body.Name,
		Description:    body.Description,
		ProviderScope:  jsonObjectToMapPtr(body.ProviderScope),
		ModelScope:     jsonObjectToMapPtr(body.ModelScope),
		StrategyHint:   body.StrategyHint,
		Status:         toAccountGroupStatusPtr(body.Status),
		RateMultiplier: body.RateMultiplier,
	})
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account group update request", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "account_group.update", "account_group", strconv.Itoa(group.ID), accountGroupAuditSnapshot(before), accountGroupAuditSnapshot(group)))
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountGroupResponse{
		Data:      toAPIAccountGroup(group),
		RequestId: requestID,
	})
}

func (s *Server) handleDeleteAdminAccountGroup(w http.ResponseWriter, r *http.Request) {
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
	groupID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || groupID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account group id", requestID)
		return
	}
	before, err := s.runtime.accounts.FindGroupByID(r.Context(), groupID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account group not found", requestID)
		return
	}
	if err := s.runtime.accounts.DeleteGroup(r.Context(), groupID); err != nil {
		switch {
		case errors.Is(err, accountservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account group id", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to delete account group", requestID)
		}
		return
	}
	// Cascade the group's rate-limit policy (cross-module). A group with no rate
	// limit configured is the common case, so a not-found result is not an error.
	if err := s.runtime.groupRateLimits.DeleteLimit(r.Context(), groupID); err != nil && !errors.Is(err, groupratelimitscontract.ErrNotFound) {
		s.logger.Warn("failed to clear group rate limit on group delete", "group_id", groupID, "error", err)
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "account_group.delete", "account_group", strconv.Itoa(groupID), accountGroupAuditSnapshot(before), nil))
	writeJSONAny(w, http.StatusOK, deleteResponse(true, requestID))
}

func (s *Server) handleListAdminAccountGroupMembers(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	groupID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || groupID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account group id", requestID)
		return
	}
	if _, err := s.runtime.accounts.FindGroupByID(r.Context(), groupID); err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account group not found", requestID)
		return
	}
	members, err := s.runtime.accounts.ListGroupMembers(r.Context(), groupID)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list account group members", requestID)
		return
	}
	data := make([]apiopenapi.AccountGroupMember, 0, len(members))
	for _, member := range members {
		data = append(data, toAPIAccountGroupMember(member))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountGroupMemberListResponse{
		Data:      data,
		RequestId: requestID,
	})
}

func (s *Server) handleAddAdminAccountGroupMember(w http.ResponseWriter, r *http.Request) {
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
	groupID, accountID, ok := accountGroupMemberPathIDs(w, r, requestID)
	if !ok {
		return
	}
	member, err := s.runtime.accounts.AddAccountToGroup(r.Context(), accountID, groupID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account or group not found", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "account_group.member_add", "account_group", strconv.Itoa(groupID), nil, map[string]any{
		"account_id": accountID,
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountGroupMemberResponse{
		Data:      toAPIAccountGroupMember(member),
		RequestId: requestID,
	})
}

func (s *Server) handleRemoveAdminAccountGroupMember(w http.ResponseWriter, r *http.Request) {
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
	groupID, accountID, ok := accountGroupMemberPathIDs(w, r, requestID)
	if !ok {
		return
	}
	if err := s.runtime.accounts.RemoveAccountFromGroup(r.Context(), accountID, groupID); err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to remove account group membership", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "account_group.member_remove", "account_group", strconv.Itoa(groupID), map[string]any{
		"account_id": accountID,
	}, nil))
	w.WriteHeader(http.StatusNoContent)
}
