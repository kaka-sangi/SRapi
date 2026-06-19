package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	modelratelimitscontract "github.com/srapi/srapi/apps/api/internal/modules/model_rate_limits/contract"
	modelratelimitsservice "github.com/srapi/srapi/apps/api/internal/modules/model_rate_limits/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func toAPIModelRateLimit(limit modelratelimitscontract.Limit) apiopenapi.ModelRateLimit {
	return apiopenapi.ModelRateLimit{
		ModelId:        int64(limit.ModelID),
		RpmLimit:       int64(limit.RPMLimit),
		TpmLimit:       int64(limit.TPMLimit),
		MaxConcurrency: int64(limit.MaxConcurrency),
		Enabled:        limit.Enabled,
		CreatedAt:      limit.CreatedAt.UTC(),
		UpdatedAt:      limit.UpdatedAt.UTC(),
	}
}

func (s *Server) handleListAdminModelRateLimits(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	limits, err := s.runtime.modelRateLimits.ListLimits(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list model rate limits", requestID)
		return
	}
	data := make([]apiopenapi.ModelRateLimit, 0, len(limits))
	for _, limit := range limits {
		data = append(data, toAPIModelRateLimit(limit))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.ModelRateLimitListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleUpsertAdminModelRateLimit(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.UpsertModelRateLimitRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid model rate limit request", requestID)
		return
	}
	modelID, ok := positiveIntFromInt64(body.ModelId)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model id", requestID)
		return
	}
	if _, err := s.runtime.models.FindByID(r.Context(), modelID); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "model not found", requestID)
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	rpmLimit, ok := nonNegativeIntFromInt64Ptr(body.RpmLimit)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model rate limit request", requestID)
		return
	}
	tpmLimit, ok := nonNegativeIntFromInt64Ptr(body.TpmLimit)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model rate limit request", requestID)
		return
	}
	maxConcurrency, ok := nonNegativeIntFromInt64Ptr(body.MaxConcurrency)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model rate limit request", requestID)
		return
	}
	limit, err := s.runtime.modelRateLimits.UpsertLimit(r.Context(), modelratelimitscontract.UpsertLimit{
		ModelID:        modelID,
		RPMLimit:       rpmLimit,
		TPMLimit:       tpmLimit,
		MaxConcurrency: maxConcurrency,
		Enabled:        enabled,
	})
	if err != nil {
		s.writeModelRateLimitError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "model_rate_limit.upsert", "model_rate_limit", strconv.Itoa(limit.ModelID), nil, map[string]any{
		"model_id":        limit.ModelID,
		"rpm_limit":       limit.RPMLimit,
		"tpm_limit":       limit.TPMLimit,
		"max_concurrency": limit.MaxConcurrency,
		"enabled":         limit.Enabled,
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.ModelRateLimitResponse{
		Data:      toAPIModelRateLimit(limit),
		RequestId: requestID,
	})
}

func (s *Server) handleDeleteAdminModelRateLimit(w http.ResponseWriter, r *http.Request) {
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
	modelID, err := strconv.Atoi(r.PathValue("modelId"))
	if err != nil || modelID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model id", requestID)
		return
	}
	if err := s.runtime.modelRateLimits.DeleteLimit(r.Context(), modelID); err != nil {
		s.writeModelRateLimitError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "model_rate_limit.delete", "model_rate_limit", strconv.Itoa(modelID), nil, nil))
	writeJSONAny(w, http.StatusOK, deleteResponse(true, requestID))
}

func (s *Server) writeModelRateLimitError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, modelratelimitscontract.ErrNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "model rate limit not found", requestID)
	case errors.Is(err, modelratelimitsservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model rate limit request", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to process model rate limit request", requestID)
	}
}
