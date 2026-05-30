package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	groupratelimitscontract "github.com/srapi/srapi/apps/api/internal/modules/group_rate_limits/contract"
	groupratelimitsservice "github.com/srapi/srapi/apps/api/internal/modules/group_rate_limits/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

type groupRateLimitPayload struct {
	GroupID        int       `json:"account_group_id"`
	RPMLimit       int       `json:"rpm_limit"`
	MaxConcurrency int       `json:"max_concurrency"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type upsertGroupRateLimitRequest struct {
	GroupID        int   `json:"account_group_id"`
	RPMLimit       int   `json:"rpm_limit"`
	MaxConcurrency int   `json:"max_concurrency"`
	Enabled        *bool `json:"enabled"`
}

func toGroupRateLimitPayload(limit groupratelimitscontract.Limit) groupRateLimitPayload {
	return groupRateLimitPayload{
		GroupID:        limit.GroupID,
		RPMLimit:       limit.RPMLimit,
		MaxConcurrency: limit.MaxConcurrency,
		Enabled:        limit.Enabled,
		CreatedAt:      limit.CreatedAt.UTC(),
		UpdatedAt:      limit.UpdatedAt.UTC(),
	}
}

func (s *Server) handleListAdminGroupRateLimits(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	limits, err := s.runtime.groupRateLimits.ListLimits(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list group rate limits", requestID)
		return
	}
	data := make([]groupRateLimitPayload, 0, len(limits))
	for _, limit := range limits {
		data = append(data, toGroupRateLimitPayload(limit))
	}
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       data,
		"pagination": pagination(len(data)),
		"request_id": requestID,
	})
}

func (s *Server) handleUpsertAdminGroupRateLimit(w http.ResponseWriter, r *http.Request) {
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
	var body upsertGroupRateLimitRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid group rate limit request", requestID)
		return
	}
	if body.GroupID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account group id", requestID)
		return
	}
	if _, err := s.runtime.accounts.FindGroupByID(r.Context(), body.GroupID); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "account group not found", requestID)
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	limit, err := s.runtime.groupRateLimits.UpsertLimit(r.Context(), groupratelimitscontract.UpsertLimit{
		GroupID:        body.GroupID,
		RPMLimit:       body.RPMLimit,
		MaxConcurrency: body.MaxConcurrency,
		Enabled:        enabled,
	})
	if err != nil {
		s.writeGroupRateLimitError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "group_rate_limit.upsert", "account_group_rate_limit", strconv.Itoa(limit.GroupID), nil, map[string]any{
		"account_group_id": limit.GroupID,
		"rpm_limit":        limit.RPMLimit,
		"max_concurrency":  limit.MaxConcurrency,
		"enabled":          limit.Enabled,
	}))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       toGroupRateLimitPayload(limit),
		"request_id": requestID,
	})
}

func (s *Server) handleDeleteAdminGroupRateLimit(w http.ResponseWriter, r *http.Request) {
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
	groupID, err := strconv.Atoi(r.PathValue("groupId"))
	if err != nil || groupID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account group id", requestID)
		return
	}
	if err := s.runtime.groupRateLimits.DeleteLimit(r.Context(), groupID); err != nil {
		s.writeGroupRateLimitError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "group_rate_limit.delete", "account_group_rate_limit", strconv.Itoa(groupID), nil, nil))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"account_group_id": groupID, "deleted": true},
		"request_id": requestID,
	})
}

func (s *Server) writeGroupRateLimitError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, groupratelimitscontract.ErrNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "group rate limit not found", requestID)
	case errors.Is(err, groupratelimitsservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid group rate limit request", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to process group rate limit request", requestID)
	}
}
