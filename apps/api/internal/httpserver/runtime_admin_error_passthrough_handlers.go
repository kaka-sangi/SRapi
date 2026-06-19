package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	errorpassthroughcontract "github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/contract"
	errorpassthroughservice "github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func toAPIErrorPassthroughRule(rule errorpassthroughcontract.Rule) apiopenapi.ErrorPassthroughRule {
	statusCodes := make([]int64, 0, len(rule.StatusCodes))
	for _, code := range rule.StatusCodes {
		statusCodes = append(statusCodes, int64(code))
	}
	classes := rule.Classes
	if classes == nil {
		classes = []string{}
	}
	keywords := rule.Keywords
	if keywords == nil {
		keywords = []string{}
	}
	return apiopenapi.ErrorPassthroughRule{
		Id:             int64(rule.ID),
		Name:           rule.Name,
		Enabled:        rule.Enabled,
		Priority:       int64(rule.Priority),
		Action:         apiopenapi.ErrorPassthroughRuleAction(rule.Action),
		StatusCodes:    statusCodes,
		Classes:        classes,
		Keywords:       keywords,
		ResponseStatus: int64PtrFromIntPtr(rule.ResponseStatus),
		ResponseCode:   int64PtrFromIntPtr(rule.ResponseStatus),
		CustomMessage:  optionalNonEmptyStringPtr(rule.CustomMessage),
		CreatedAt:      rule.CreatedAt.UTC(),
		UpdatedAt:      rule.UpdatedAt.UTC(),
	}
}

func optionalIntSlicePtrFromInt64Ptr(value *[]int64) (*[]int, bool) {
	if value == nil {
		return nil, true
	}
	converted, ok := nonNegativeIntSliceFromInt64Ptr(value)
	if !ok {
		return nil, false
	}
	return &converted, true
}

func firstInt64PtrAsIntPtr(values ...*int64) (*int, bool) {
	for _, value := range values {
		converted, ok := intPtrFromInt64Ptr(value)
		if !ok {
			return nil, false
		}
		if converted != nil {
			return converted, true
		}
	}
	return nil, true
}

func firstOptionalInt64PtrAsIntPtr(values ...*int64) (**int, bool) {
	for _, value := range values {
		converted, ok := intPtrFromInt64Ptr(value)
		if !ok {
			return nil, false
		}
		if value != nil {
			return &converted, true
		}
	}
	return nil, true
}

func (s *Server) handleListAdminErrorPassthroughRules(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	rules, err := s.runtime.errorPassthrough.ListRules(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list error passthrough rules", requestID)
		return
	}
	data := make([]apiopenapi.ErrorPassthroughRule, 0, len(rules))
	for _, rule := range rules {
		data = append(data, toAPIErrorPassthroughRule(rule))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.ErrorPassthroughRuleListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminErrorPassthroughRule(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreateErrorPassthroughRuleRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid error passthrough rule request", requestID)
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	priorityPtr, ok := intPtrFromInt64Ptr(body.Priority)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid error passthrough rule request", requestID)
		return
	}
	priority := 0
	if priorityPtr != nil {
		priority = *priorityPtr
	}
	statusCodes, ok := nonNegativeIntSliceFromInt64Ptr(body.StatusCodes)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid error passthrough rule request", requestID)
		return
	}
	responseStatus, ok := firstInt64PtrAsIntPtr(body.ResponseStatus, body.ResponseCode)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid error passthrough rule request", requestID)
		return
	}
	rule, err := s.runtime.errorPassthrough.CreateRule(r.Context(), errorpassthroughcontract.CreateRule{
		Name:           body.Name,
		Enabled:        enabled,
		Priority:       priority,
		Action:         errorpassthroughcontract.Action(body.Action),
		StatusCodes:    statusCodes,
		Classes:        openapiOptionalStringSlice(body.Classes),
		Keywords:       openapiOptionalStringSlice(body.Keywords),
		ResponseStatus: responseStatus,
		CustomMessage:  openapiOptionalString(body.CustomMessage),
	})
	if err != nil {
		s.writeErrorPassthroughError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "error_passthrough_rule.create", "error_passthrough_rule", strconv.Itoa(rule.ID), nil, map[string]any{
		"name":    rule.Name,
		"action":  rule.Action,
		"enabled": rule.Enabled,
	}))
	writeJSONAny(w, http.StatusCreated, apiopenapi.ErrorPassthroughRuleResponse{
		Data:      toAPIErrorPassthroughRule(rule),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminErrorPassthroughRule(w http.ResponseWriter, r *http.Request) {
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
	ruleID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || ruleID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid error passthrough rule id", requestID)
		return
	}
	var body apiopenapi.UpdateErrorPassthroughRuleRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid error passthrough rule request", requestID)
		return
	}
	priority, ok := intPtrFromInt64Ptr(body.Priority)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid error passthrough rule request", requestID)
		return
	}
	statusCodes, ok := optionalIntSlicePtrFromInt64Ptr(body.StatusCodes)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid error passthrough rule request", requestID)
		return
	}
	responseStatus, ok := firstOptionalInt64PtrAsIntPtr(body.ResponseStatus, body.ResponseCode)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid error passthrough rule request", requestID)
		return
	}
	input := errorpassthroughcontract.UpdateRule{
		Name:           body.Name,
		Enabled:        body.Enabled,
		Priority:       priority,
		StatusCodes:    statusCodes,
		Classes:        body.Classes,
		Keywords:       body.Keywords,
		ResponseStatus: responseStatus,
	}
	if body.CustomMessage != nil {
		input.CustomMessage = body.CustomMessage
	}
	if body.Action != nil {
		action := errorpassthroughcontract.Action(*body.Action)
		input.Action = &action
	}
	rule, err := s.runtime.errorPassthrough.UpdateRule(r.Context(), ruleID, input)
	if err != nil {
		s.writeErrorPassthroughError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "error_passthrough_rule.update", "error_passthrough_rule", strconv.Itoa(rule.ID), nil, map[string]any{
		"name":    rule.Name,
		"action":  rule.Action,
		"enabled": rule.Enabled,
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.ErrorPassthroughRuleResponse{
		Data:      toAPIErrorPassthroughRule(rule),
		RequestId: requestID,
	})
}

func (s *Server) handleDeleteAdminErrorPassthroughRule(w http.ResponseWriter, r *http.Request) {
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
	ruleID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || ruleID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid error passthrough rule id", requestID)
		return
	}
	if err := s.runtime.errorPassthrough.DeleteRule(r.Context(), ruleID); err != nil {
		s.writeErrorPassthroughError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "error_passthrough_rule.delete", "error_passthrough_rule", strconv.Itoa(ruleID), nil, nil))
	writeJSONAny(w, http.StatusOK, deleteResponse(true, requestID))
}

func (s *Server) writeErrorPassthroughError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, errorpassthroughcontract.ErrNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "error passthrough rule not found", requestID)
	case errors.Is(err, errorpassthroughservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid error passthrough rule request", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to process error passthrough rule request", requestID)
	}
}
