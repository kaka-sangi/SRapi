package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	payloadrulescontract "github.com/srapi/srapi/apps/api/internal/modules/payload_rules/contract"
	payloadrulesservice "github.com/srapi/srapi/apps/api/internal/modules/payload_rules/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func toAPIPayloadRule(rule payloadrulescontract.Rule) apiopenapi.PayloadRule {
	params := rule.Params
	if params == nil {
		params = map[string]any{}
	}
	return apiopenapi.PayloadRule{
		Id:            int64(rule.ID),
		Name:          rule.Name,
		Enabled:       rule.Enabled,
		Priority:      int64(rule.Priority),
		Action:        apiopenapi.PayloadRuleAction(rule.Action),
		MatchModel:    rule.MatchModel,
		MatchProtocol: rule.MatchProtocol,
		Params:        params,
		CreatedAt:     rule.CreatedAt.UTC(),
		UpdatedAt:     rule.UpdatedAt.UTC(),
	}
}

func toImportPayloadRule(rule payloadrulescontract.Rule) apiopenapi.CreatePayloadRuleRequest {
	params := rule.Params
	if params == nil {
		params = map[string]any{}
	}
	enabled := rule.Enabled
	priority := int64(rule.Priority)
	return apiopenapi.CreatePayloadRuleRequest{
		Name:          rule.Name,
		Enabled:       &enabled,
		Priority:      &priority,
		Action:        apiopenapi.CreatePayloadRuleRequestAction(rule.Action),
		MatchModel:    optionalNonEmptyStringPtr(rule.MatchModel),
		MatchProtocol: optionalNonEmptyStringPtr(rule.MatchProtocol),
		Params:        params,
	}
}

func (s *Server) handleListAdminPayloadRules(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	rules, err := s.runtime.payloadRules.ListRules(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list payload rules", requestID)
		return
	}
	data := make([]apiopenapi.PayloadRule, 0, len(rules))
	for _, rule := range rules {
		data = append(data, toAPIPayloadRule(rule))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.PayloadRuleListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminPayloadRule(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreatePayloadRuleRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid payload rule request", requestID)
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	priorityPtr, ok := intPtrFromInt64Ptr(body.Priority)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid payload rule request", requestID)
		return
	}
	priority := 0
	if priorityPtr != nil {
		priority = *priorityPtr
	}
	rule, err := s.runtime.payloadRules.CreateRule(r.Context(), payloadrulescontract.CreateRule{
		Name:          body.Name,
		Enabled:       enabled,
		Priority:      priority,
		Action:        payloadrulescontract.Action(body.Action),
		MatchModel:    openapiOptionalString(body.MatchModel),
		MatchProtocol: openapiOptionalString(body.MatchProtocol),
		Params:        body.Params,
	})
	if err != nil {
		s.writePayloadRuleError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "payload_rule.create", "payload_rule", strconv.Itoa(rule.ID), nil, map[string]any{
		"name":    rule.Name,
		"action":  rule.Action,
		"enabled": rule.Enabled,
	}))
	writeJSONAny(w, http.StatusCreated, apiopenapi.PayloadRuleResponse{
		Data:      toAPIPayloadRule(rule),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminPayloadRule(w http.ResponseWriter, r *http.Request) {
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
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid payload rule id", requestID)
		return
	}
	var body apiopenapi.UpdatePayloadRuleRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid payload rule request", requestID)
		return
	}
	priority, ok := intPtrFromInt64Ptr(body.Priority)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid payload rule request", requestID)
		return
	}
	input := payloadrulescontract.UpdateRule{
		Name:          body.Name,
		Enabled:       body.Enabled,
		Priority:      priority,
		MatchModel:    body.MatchModel,
		MatchProtocol: body.MatchProtocol,
		Params:        body.Params,
	}
	if body.Action != nil {
		action := payloadrulescontract.Action(*body.Action)
		input.Action = &action
	}
	rule, err := s.runtime.payloadRules.UpdateRule(r.Context(), ruleID, input)
	if err != nil {
		s.writePayloadRuleError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "payload_rule.update", "payload_rule", strconv.Itoa(rule.ID), nil, map[string]any{
		"name":    rule.Name,
		"action":  rule.Action,
		"enabled": rule.Enabled,
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.PayloadRuleResponse{
		Data:      toAPIPayloadRule(rule),
		RequestId: requestID,
	})
}

func (s *Server) handleDeleteAdminPayloadRule(w http.ResponseWriter, r *http.Request) {
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
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid payload rule id", requestID)
		return
	}
	if err := s.runtime.payloadRules.DeleteRule(r.Context(), ruleID); err != nil {
		s.writePayloadRuleError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "payload_rule.delete", "payload_rule", strconv.Itoa(ruleID), nil, nil))
	writeJSONAny(w, http.StatusOK, deleteResponse(true, requestID))
}

func (s *Server) writePayloadRuleError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, payloadrulescontract.ErrNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "payload rule not found", requestID)
	case errors.Is(err, payloadrulesservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid payload rule request", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to process payload rule request", requestID)
	}
}
