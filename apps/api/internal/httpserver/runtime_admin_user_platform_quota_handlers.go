package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	userplatformquotascontract "github.com/srapi/srapi/apps/api/internal/modules/user_platform_quotas/contract"
	userplatformquotasservice "github.com/srapi/srapi/apps/api/internal/modules/user_platform_quotas/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

type userPlatformQuotaPayload struct {
	UserID       int       `json:"user_id"`
	Platform     string    `json:"platform"`
	DailyLimit   *string   `json:"daily_limit"`
	WeeklyLimit  *string   `json:"weekly_limit"`
	MonthlyLimit *string   `json:"monthly_limit"`
	Currency     string    `json:"currency"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type upsertUserPlatformQuotaRequest struct {
	Platform     string  `json:"platform"`
	DailyLimit   *string `json:"daily_limit"`
	WeeklyLimit  *string `json:"weekly_limit"`
	MonthlyLimit *string `json:"monthly_limit"`
	Currency     string  `json:"currency"`
	Enabled      *bool   `json:"enabled"`
}

func toUserPlatformQuotaPayload(quota userplatformquotascontract.Quota) userPlatformQuotaPayload {
	return userPlatformQuotaPayload{
		UserID:       quota.UserID,
		Platform:     quota.Platform,
		DailyLimit:   quota.DailyLimit,
		WeeklyLimit:  quota.WeeklyLimit,
		MonthlyLimit: quota.MonthlyLimit,
		Currency:     quota.Currency,
		Enabled:      quota.Enabled,
		CreatedAt:    quota.CreatedAt.UTC(),
		UpdatedAt:    quota.UpdatedAt.UTC(),
	}
}

func (s *Server) handleListAdminUserPlatformQuotas(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	userID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || userID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user id", requestID)
		return
	}
	quotas, err := s.runtime.userPlatformQuotas.ListByUser(r.Context(), userID)
	if err != nil {
		s.writeUserPlatformQuotaError(w, err, requestID)
		return
	}
	data := make([]userPlatformQuotaPayload, 0, len(quotas))
	for _, quota := range quotas {
		data = append(data, toUserPlatformQuotaPayload(quota))
	}
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       data,
		"pagination": pagination(len(data)),
		"request_id": requestID,
	})
}

func (s *Server) handleUpsertAdminUserPlatformQuota(w http.ResponseWriter, r *http.Request) {
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
	userID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || userID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user id", requestID)
		return
	}
	if _, err := s.runtime.users.FindByID(r.Context(), userID); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "user not found", requestID)
		return
	}
	var body upsertUserPlatformQuotaRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid user platform quota request", requestID)
		return
	}
	if strings.TrimSpace(body.Platform) == "" {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "platform is required", requestID)
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	currency := strings.TrimSpace(body.Currency)
	if currency == "" {
		currency = "USD"
	}
	quota, err := s.runtime.userPlatformQuotas.UpsertQuota(r.Context(), userplatformquotascontract.UpsertQuota{
		UserID:       userID,
		Platform:     strings.TrimSpace(body.Platform),
		DailyLimit:   trimmedMoneyPtr(body.DailyLimit),
		WeeklyLimit:  trimmedMoneyPtr(body.WeeklyLimit),
		MonthlyLimit: trimmedMoneyPtr(body.MonthlyLimit),
		Currency:     currency,
		Enabled:      enabled,
	})
	if err != nil {
		s.writeUserPlatformQuotaError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user_platform_quota.upsert", "user_platform_quota", strconv.Itoa(userID)+":"+quota.Platform, nil, map[string]any{
		"user_id":       quota.UserID,
		"platform":      quota.Platform,
		"daily_limit":   quota.DailyLimit,
		"weekly_limit":  quota.WeeklyLimit,
		"monthly_limit": quota.MonthlyLimit,
		"enabled":       quota.Enabled,
	}))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       toUserPlatformQuotaPayload(quota),
		"request_id": requestID,
	})
}

func (s *Server) handleDeleteAdminUserPlatformQuota(w http.ResponseWriter, r *http.Request) {
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
	userID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || userID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user id", requestID)
		return
	}
	platform := strings.TrimSpace(r.PathValue("platform"))
	if platform == "" {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid platform", requestID)
		return
	}
	if err := s.runtime.userPlatformQuotas.DeleteQuota(r.Context(), userID, platform); err != nil {
		s.writeUserPlatformQuotaError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user_platform_quota.delete", "user_platform_quota", strconv.Itoa(userID)+":"+platform, nil, nil))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"user_id": userID, "platform": platform, "deleted": true},
		"request_id": requestID,
	})
}

func (s *Server) writeUserPlatformQuotaError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, userplatformquotascontract.ErrNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "user platform quota not found", requestID)
	case errors.Is(err, userplatformquotasservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user platform quota request", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to process user platform quota request", requestID)
	}
}

// trimmedMoneyPtr normalizes an optional money string: nil or blank → nil (the
// window is uncapped), otherwise a trimmed value.
func trimmedMoneyPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
