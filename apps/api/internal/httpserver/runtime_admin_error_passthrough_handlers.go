package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	errorpassthroughcontract "github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/contract"
	errorpassthroughservice "github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

type errorPassthroughRulePayload struct {
	ID             int       `json:"id"`
	Name           string    `json:"name"`
	Enabled        bool      `json:"enabled"`
	Priority       int       `json:"priority"`
	Action         string    `json:"action"`
	StatusCodes    []int     `json:"status_codes"`
	Classes        []string  `json:"classes"`
	Keywords       []string  `json:"keywords"`
	ResponseStatus *int      `json:"response_status,omitempty"`
	ResponseCode   *int      `json:"response_code,omitempty"`
	CustomMessage  string    `json:"custom_message,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type createErrorPassthroughRuleRequest struct {
	Name           string   `json:"name"`
	Enabled        *bool    `json:"enabled"`
	Priority       int      `json:"priority"`
	Action         string   `json:"action"`
	StatusCodes    []int    `json:"status_codes"`
	Classes        []string `json:"classes"`
	Keywords       []string `json:"keywords"`
	ResponseStatus *int     `json:"response_status"`
	ResponseCode   *int     `json:"response_code"`
	CustomMessage  string   `json:"custom_message"`
}

type updateErrorPassthroughRuleRequest struct {
	Name           *string   `json:"name"`
	Enabled        *bool     `json:"enabled"`
	Priority       *int      `json:"priority"`
	Action         *string   `json:"action"`
	StatusCodes    *[]int    `json:"status_codes"`
	Classes        *[]string `json:"classes"`
	Keywords       *[]string `json:"keywords"`
	ResponseStatus *int      `json:"response_status"`
	ResponseCode   *int      `json:"response_code"`
	CustomMessage  *string   `json:"custom_message"`
}

func toErrorPassthroughRulePayload(rule errorpassthroughcontract.Rule) errorPassthroughRulePayload {
	statusCodes := rule.StatusCodes
	if statusCodes == nil {
		statusCodes = []int{}
	}
	classes := rule.Classes
	if classes == nil {
		classes = []string{}
	}
	keywords := rule.Keywords
	if keywords == nil {
		keywords = []string{}
	}
	return errorPassthroughRulePayload{
		ID:             rule.ID,
		Name:           rule.Name,
		Enabled:        rule.Enabled,
		Priority:       rule.Priority,
		Action:         string(rule.Action),
		StatusCodes:    statusCodes,
		Classes:        classes,
		Keywords:       keywords,
		ResponseStatus: cloneIntPtr(rule.ResponseStatus),
		ResponseCode:   cloneIntPtr(rule.ResponseStatus),
		CustomMessage:  rule.CustomMessage,
		CreatedAt:      rule.CreatedAt.UTC(),
		UpdatedAt:      rule.UpdatedAt.UTC(),
	}
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
	data := make([]errorPassthroughRulePayload, 0, len(rules))
	for _, rule := range rules {
		data = append(data, toErrorPassthroughRulePayload(rule))
	}
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       data,
		"pagination": pagination(len(data)),
		"request_id": requestID,
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
	var body createErrorPassthroughRuleRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid error passthrough rule request", requestID)
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	rule, err := s.runtime.errorPassthrough.CreateRule(r.Context(), errorpassthroughcontract.CreateRule{
		Name:        body.Name,
		Enabled:     enabled,
		Priority:    body.Priority,
		Action:      errorpassthroughcontract.Action(body.Action),
		StatusCodes: body.StatusCodes,
		Classes:     body.Classes,
		Keywords:    body.Keywords,
		ResponseStatus: firstIntPtr(
			body.ResponseStatus,
			body.ResponseCode,
		),
		CustomMessage: body.CustomMessage,
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
	writeJSONAny(w, http.StatusCreated, map[string]any{
		"data":       toErrorPassthroughRulePayload(rule),
		"request_id": requestID,
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
	var body updateErrorPassthroughRuleRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid error passthrough rule request", requestID)
		return
	}
	input := errorpassthroughcontract.UpdateRule{
		Name:        body.Name,
		Enabled:     body.Enabled,
		Priority:    body.Priority,
		StatusCodes: body.StatusCodes,
		Classes:     body.Classes,
		Keywords:    body.Keywords,
	}
	if responseStatus := firstIntPtr(body.ResponseStatus, body.ResponseCode); responseStatus != nil {
		input.ResponseStatus = &responseStatus
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
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       toErrorPassthroughRulePayload(rule),
		"request_id": requestID,
	})
}

func firstIntPtr(values ...*int) *int {
	for _, value := range values {
		if value == nil {
			continue
		}
		return cloneIntPtr(value)
	}
	return nil
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
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"id": ruleID, "deleted": true},
		"request_id": requestID,
	})
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
