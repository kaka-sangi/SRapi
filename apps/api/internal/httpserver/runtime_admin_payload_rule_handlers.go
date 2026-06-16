package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	payloadrulescontract "github.com/srapi/srapi/apps/api/internal/modules/payload_rules/contract"
	payloadrulesservice "github.com/srapi/srapi/apps/api/internal/modules/payload_rules/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

type payloadRulePayload struct {
	ID            int            `json:"id"`
	Name          string         `json:"name"`
	Enabled       bool           `json:"enabled"`
	Priority      int            `json:"priority"`
	Action        string         `json:"action"`
	MatchModel    string         `json:"match_model"`
	MatchProtocol string         `json:"match_protocol"`
	Params        map[string]any `json:"params"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type createPayloadRuleRequest struct {
	Name          string         `json:"name"`
	Enabled       *bool          `json:"enabled"`
	Priority      int            `json:"priority"`
	Action        string         `json:"action"`
	MatchModel    string         `json:"match_model"`
	MatchProtocol string         `json:"match_protocol"`
	Params        map[string]any `json:"params"`
}

type updatePayloadRuleRequest struct {
	Name          *string         `json:"name"`
	Enabled       *bool           `json:"enabled"`
	Priority      *int            `json:"priority"`
	Action        *string         `json:"action"`
	MatchModel    *string         `json:"match_model"`
	MatchProtocol *string         `json:"match_protocol"`
	Params        *map[string]any `json:"params"`
}

func toPayloadRulePayload(rule payloadrulescontract.Rule) payloadRulePayload {
	params := rule.Params
	if params == nil {
		params = map[string]any{}
	}
	return payloadRulePayload{
		ID:            rule.ID,
		Name:          rule.Name,
		Enabled:       rule.Enabled,
		Priority:      rule.Priority,
		Action:        string(rule.Action),
		MatchModel:    rule.MatchModel,
		MatchProtocol: rule.MatchProtocol,
		Params:        params,
		CreatedAt:     rule.CreatedAt.UTC(),
		UpdatedAt:     rule.UpdatedAt.UTC(),
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
	data := make([]payloadRulePayload, 0, len(rules))
	for _, rule := range rules {
		data = append(data, toPayloadRulePayload(rule))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       data,
		"pagination": pg,
		"request_id": requestID,
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
	var body createPayloadRuleRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid payload rule request", requestID)
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	rule, err := s.runtime.payloadRules.CreateRule(r.Context(), payloadrulescontract.CreateRule{
		Name:          body.Name,
		Enabled:       enabled,
		Priority:      body.Priority,
		Action:        payloadrulescontract.Action(body.Action),
		MatchModel:    body.MatchModel,
		MatchProtocol: body.MatchProtocol,
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
	writeJSONAny(w, http.StatusCreated, map[string]any{
		"data":       toPayloadRulePayload(rule),
		"request_id": requestID,
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
	var body updatePayloadRuleRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid payload rule request", requestID)
		return
	}
	input := payloadrulescontract.UpdateRule{
		Name:          body.Name,
		Enabled:       body.Enabled,
		Priority:      body.Priority,
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
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       toPayloadRulePayload(rule),
		"request_id": requestID,
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
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"id": ruleID, "deleted": true},
		"request_id": requestID,
	})
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
