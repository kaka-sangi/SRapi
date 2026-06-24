package httpserver

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	usersservice "github.com/srapi/srapi/apps/api/internal/modules/users/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Server) currentAdminSettings(ctx context.Context) (admincontrolcontract.AdminSettings, error) {
	return s.runtime.adminControl.GetAdminSettings(ctx)
}

func (s *Server) requirePaymentsEnabled(w http.ResponseWriter, r *http.Request, requestID string) bool {
	settings, err := s.currentAdminSettings(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return false
	}
	if !settings.Features.PaymentsEnabled || !settings.Payment.Enabled {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "payments disabled", requestID)
		return false
	}
	return true
}

func (s *Server) requireSubscriptionPlansEnabled(w http.ResponseWriter, r *http.Request, requestID string) bool {
	settings, err := s.currentAdminSettings(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return false
	}
	if !settings.Payment.SubscriptionPlansEnabled {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "subscription plans disabled", requestID)
		return false
	}
	return true
}

func (s *Server) handleSiteConfig(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	settings, err := s.currentAdminSettings(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"site_name":      settings.General.SiteName,
			"site_subtitle":  settings.General.SiteSubtitle,
			"logo_url":       settings.General.LogoURL,
			"version_label":  settings.General.VersionLabel,
			"contact_info":   settings.General.ContactInfo,
			"doc_url":        settings.General.DocURL,
			"custom_menus":   publicCustomMenus(settings.General.CustomMenus),
			"user_agreement":       settings.Agreement.UserAgreement,
			"privacy_policy":       settings.Agreement.PrivacyPolicy,
			"maintenance":          publicMaintenanceSummary(settings.Maintenance),
			"email_login_available": settings.Email.SMTPConfigured,
			"payments_enabled":     settings.Features.PaymentsEnabled,
		},
		"request_id": requestID,
	})
}

// publicMaintenanceSummary trims the admin-only knobs out of the maintenance
// settings before exposing them on the unauthenticated site-config endpoint.
// Disabled maintenance returns a zero summary so the frontend banner can
// reliably check the `enabled` flag without inspecting nested fields.
func publicMaintenanceSummary(m admincontrolcontract.AdminSettingsMaintenance) map[string]any {
	if !m.Enabled {
		return map[string]any{"enabled": false}
	}
	out := map[string]any{
		"enabled": true,
		"message": m.Message,
	}
	if m.ExpectedRecoveryAt != nil {
		out["expected_recovery_at"] = m.ExpectedRecoveryAt
	}
	return out
}

func publicCustomMenus(values []admincontrolcontract.CustomMenuItem) []admincontrolcontract.CustomMenuItem {
	out := make([]admincontrolcontract.CustomMenuItem, 0, len(values))
	for _, value := range values {
		if value.Visibility == "admin" {
			continue
		}
		out = append(out, value)
	}
	return out
}

// handleListPublicSubscriptionPlans returns the storefront catalog — only
// for_sale active plans, no auth required so the pricing page can be browsed
// before sign-in. The admin variant (/admin/subscription-plans) returns
// everything including draft/archived/internal plans.
func (s *Server) handleListPublicSubscriptionPlans(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if !s.requireSubscriptionPlansEnabled(w, r, requestID) {
		return
	}
	items, err := s.runtime.subscriptions.ListForSalePlans(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list subscription plans", requestID)
		return
	}
	data := make([]apiopenapi.SubscriptionPlan, 0, len(items))
	for _, item := range items {
		data = append(data, toAPISubscriptionPlan(item))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.SubscriptionPlanListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleDeleteCurrentUser(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	settings, err := s.currentAdminSettings(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	if !settings.Users.UserSelfDeleteEnabled {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "self delete disabled", requestID)
		return
	}
	if err := s.runtime.userAttributes.DeleteUserValues(r.Context(), session.User.ID); err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to delete user attributes", requestID)
		return
	}
	// Revoke all API keys before soft-deleting the user so that keys
	// belonging to the deleted user can no longer authenticate requests.
	if err := s.runtime.apiKeys.RevokeByUser(r.Context(), session.User.ID); err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to revoke user api keys", requestID)
		return
	}
	// Invalidate ALL active console sessions (not just the current one)
	// so the user is immediately signed out everywhere.
	if err := s.runtime.auth.LogoutUser(r.Context(), session.User.ID); err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to invalidate user sessions", requestID)
		return
	}
	if err := s.runtime.users.Delete(r.Context(), session.User.ID); err != nil {
		if err == usersservice.ErrUserNotFound {
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "user not found", requestID)
			return
		}
		writeUserServiceError(w, err, requestID)
		return
	}
	s.clearSessionCookie(w)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user.self_delete", "user", strconv.Itoa(session.User.ID), nil, nil))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"id": session.User.ID, "deleted": true},
		"request_id": requestID,
	})
}

func (s *Server) defaultAPIKeyGroupIDs(ctx context.Context) []int {
	settings, err := s.currentAdminSettings(ctx)
	if err != nil {
		return nil
	}
	name := strings.TrimSpace(settings.Users.DefaultGroup)
	if name == "" {
		return nil
	}
	groups, err := s.runtime.accounts.ListGroups(ctx)
	if err != nil {
		return nil
	}
	for _, group := range groups {
		if strings.EqualFold(strings.TrimSpace(group.Name), name) && group.Status == accountcontract.GroupStatusActive {
			return []int{group.ID}
		}
	}
	return nil
}
