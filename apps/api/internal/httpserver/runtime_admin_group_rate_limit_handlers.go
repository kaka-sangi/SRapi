package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	groupratelimitscontract "github.com/srapi/srapi/apps/api/internal/modules/group_rate_limits/contract"
	groupratelimitsservice "github.com/srapi/srapi/apps/api/internal/modules/group_rate_limits/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func toAPIGroupRateLimit(limit groupratelimitscontract.Limit) apiopenapi.AccountGroupRateLimit {
	return apiopenapi.AccountGroupRateLimit{
		AccountGroupId: int64(limit.GroupID),
		RpmLimit:       int64(limit.RPMLimit),
		TpmLimit:       int64(limit.TPMLimit),
		MaxConcurrency: int64(limit.MaxConcurrency),
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
	data := make([]apiopenapi.AccountGroupRateLimit, 0, len(limits))
	for _, limit := range limits {
		data = append(data, toAPIGroupRateLimit(limit))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.GroupRateLimitListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
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
	var body apiopenapi.UpsertGroupRateLimitRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid group rate limit request", requestID)
		return
	}
	groupID, ok := positiveIntFromInt64(body.AccountGroupId)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account group id", requestID)
		return
	}
	if _, err := s.runtime.accounts.FindGroupByID(r.Context(), groupID); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "account group not found", requestID)
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	rpmLimit, ok := nonNegativeIntFromInt64Ptr(body.RpmLimit)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid group rate limit request", requestID)
		return
	}
	tpmLimit, ok := nonNegativeIntFromInt64Ptr(body.TpmLimit)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid group rate limit request", requestID)
		return
	}
	maxConcurrency, ok := nonNegativeIntFromInt64Ptr(body.MaxConcurrency)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid group rate limit request", requestID)
		return
	}
	limit, err := s.runtime.groupRateLimits.UpsertLimit(r.Context(), groupratelimitscontract.UpsertLimit{
		GroupID:        groupID,
		RPMLimit:       rpmLimit,
		TPMLimit:       tpmLimit,
		MaxConcurrency: maxConcurrency,
		Enabled:        enabled,
	})
	if err != nil {
		s.writeGroupRateLimitError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "group_rate_limit.upsert", "account_group_rate_limit", strconv.Itoa(limit.GroupID), nil, map[string]any{
		"account_group_id": limit.GroupID,
		"rpm_limit":        limit.RPMLimit,
		"tpm_limit":        limit.TPMLimit,
		"max_concurrency":  limit.MaxConcurrency,
		"enabled":          limit.Enabled,
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.GroupRateLimitResponse{
		Data:      toAPIGroupRateLimit(limit),
		RequestId: requestID,
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
	writeJSONAny(w, http.StatusOK, deleteResponse(true, requestID))
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
