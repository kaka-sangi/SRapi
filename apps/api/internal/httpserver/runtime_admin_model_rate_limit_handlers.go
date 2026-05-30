package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	modelratelimitscontract "github.com/srapi/srapi/apps/api/internal/modules/model_rate_limits/contract"
	modelratelimitsservice "github.com/srapi/srapi/apps/api/internal/modules/model_rate_limits/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

type modelRateLimitPayload struct {
	ModelID        int       `json:"model_id"`
	RPMLimit       int       `json:"rpm_limit"`
	MaxConcurrency int       `json:"max_concurrency"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type upsertModelRateLimitRequest struct {
	ModelID        int   `json:"model_id"`
	RPMLimit       int   `json:"rpm_limit"`
	MaxConcurrency int   `json:"max_concurrency"`
	Enabled        *bool `json:"enabled"`
}

func toModelRateLimitPayload(limit modelratelimitscontract.Limit) modelRateLimitPayload {
	return modelRateLimitPayload{
		ModelID:        limit.ModelID,
		RPMLimit:       limit.RPMLimit,
		MaxConcurrency: limit.MaxConcurrency,
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
	data := make([]modelRateLimitPayload, 0, len(limits))
	for _, limit := range limits {
		data = append(data, toModelRateLimitPayload(limit))
	}
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       data,
		"pagination": pagination(len(data)),
		"request_id": requestID,
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
	var body upsertModelRateLimitRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid model rate limit request", requestID)
		return
	}
	if body.ModelID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model id", requestID)
		return
	}
	if _, err := s.runtime.models.FindByID(r.Context(), body.ModelID); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "model not found", requestID)
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	limit, err := s.runtime.modelRateLimits.UpsertLimit(r.Context(), modelratelimitscontract.UpsertLimit{
		ModelID:        body.ModelID,
		RPMLimit:       body.RPMLimit,
		MaxConcurrency: body.MaxConcurrency,
		Enabled:        enabled,
	})
	if err != nil {
		s.writeModelRateLimitError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "model_rate_limit.upsert", "model_rate_limit", strconv.Itoa(limit.ModelID), nil, map[string]any{
		"model_id":        limit.ModelID,
		"rpm_limit":       limit.RPMLimit,
		"max_concurrency": limit.MaxConcurrency,
		"enabled":         limit.Enabled,
	}))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       toModelRateLimitPayload(limit),
		"request_id": requestID,
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
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"model_id": modelID, "deleted": true},
		"request_id": requestID,
	})
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
