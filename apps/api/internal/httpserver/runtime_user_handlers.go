package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	affiliatecontract "github.com/srapi/srapi/apps/api/internal/modules/affiliate/contract"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	apikeyservice "github.com/srapi/srapi/apps/api/internal/modules/api_keys/service"
	authcontract "github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
	authservice "github.com/srapi/srapi/apps/api/internal/modules/auth/service"
	captchacontract "github.com/srapi/srapi/apps/api/internal/modules/captcha/contract"
	paymentcontract "github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	usersservice "github.com/srapi/srapi/apps/api/internal/modules/users/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// verifyCaptcha enforces human verification on auth endpoints when enabled. It
// returns true when the request may proceed and writes the error response (and
// returns false) otherwise. When captcha is disabled it always returns true.
func (s *Server) verifyCaptcha(w http.ResponseWriter, r *http.Request, requestID string) bool {
	if s.runtime.captcha == nil || !s.runtime.captcha.Enabled() {
		return true
	}
	token := strings.TrimSpace(r.Header.Get("X-Captcha-Token"))
	if token == "" {
		token = strings.TrimSpace(r.Header.Get("Cf-Turnstile-Response"))
	}
	err := s.runtime.captcha.Verify(r.Context(), token, clientIP(r))
	if err == nil {
		return true
	}
	switch {
	case errors.Is(err, captchacontract.ErrCaptchaRequired):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "captcha verification required", requestID)
	case errors.Is(err, captchacontract.ErrCaptchaFailed):
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "captcha verification failed", requestID)
	default:
		writeStandardError(w, http.StatusBadGateway, apiopenapi.INTERNALERROR, "captcha verification unavailable", requestID)
	}
	return false
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if !s.verifyCaptcha(w, r, requestID) {
		return
	}
	var body apiopenapi.LoginRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid login request", requestID)
		return
	}

	result, err := s.runtime.auth.Login(r.Context(), string(body.Email), body.Password)
	if err != nil {
		switch {
		case errors.Is(err, usersservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid login request", requestID)
		case errors.Is(err, usersservice.ErrInvalidCredentials):
			writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "invalid credentials", requestID)
		case errors.Is(err, usersservice.ErrUserDisabled):
			writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "user disabled", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to login", requestID)
		}
		return
	}
	if result.RequiresSecondFactor {
		writeJSONAny(w, http.StatusAccepted, apiopenapi.LoginTwoFactorRequiredResponse{
			Data: apiopenapi.LoginTwoFactorRequired{
				ChallengeId: result.SecondFactorChallengeID,
				ExpiresAt:   *result.SecondFactorChallengeUntil,
				Required:    apiopenapi.LoginTwoFactorRequiredRequired(true),
			},
			RequestId: requestID,
		})
		return
	}

	s.setSessionCookie(w, result)

	writeJSONAny(w, http.StatusOK, apiopenapi.LoginResponse{
		Data: apiopenapi.SessionData{
			CsrfToken: result.Session.CSRFToken,
			ExpiresAt: result.Session.ExpiresAt,
			User:      toAPIUser(result.User),
		},
		RequestId: requestID,
	})
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	settings, err := s.runtime.adminControl.GetAdminSettings(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	if !settings.Security.RegistrationEnabled {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "registration disabled", requestID)
		return
	}
	if !s.verifyCaptcha(w, r, requestID) {
		return
	}

	var body apiopenapi.RegisterRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid registration request", requestID)
		return
	}
	if !registrationEmailSuffixAllowed(string(body.Email), settings.Security.RegistrationEmailSuffixAllowlist) {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid registration request", requestID)
		return
	}
	user, err := s.runtime.users.Create(r.Context(), usersservice.CreateRequest{
		Email:    string(body.Email),
		Name:     body.Name,
		Password: body.Password,
		Balance:  settings.Users.DefaultBalance,
		RPMLimit: registrationRPMLimit(settings.Users.RPMLimitDefault),
	})
	if err != nil {
		switch {
		case errors.Is(err, usersservice.ErrInvalidInput), errors.Is(err, usersservice.ErrUserAlreadyExists):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid registration request", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "registration failed", requestID)
		}
		return
	}

	result, err := s.runtime.auth.CreateSessionForUser(r.Context(), user)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "registration failed", requestID)
		return
	}
	s.setSessionCookie(w, result)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, user.ID, "user.register", "user", strconv.Itoa(user.ID), nil, userAuditSnapshot(user)))

	writeJSONAny(w, http.StatusCreated, apiopenapi.LoginResponse{
		Data: apiopenapi.SessionData{
			CsrfToken: result.Session.CSRFToken,
			ExpiresAt: result.Session.ExpiresAt,
			User:      toAPIUser(result.User),
		},
		RequestId: requestID,
	})
}

func (s *Server) handleRequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	var body apiopenapi.RequestPasswordResetRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid password reset request", requestID)
		return
	}
	result, err := s.runtime.auth.RequestPasswordReset(r.Context(), string(body.Email))
	if err != nil {
		switch {
		case errors.Is(err, authservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid password reset request", requestID)
		case errors.Is(err, authservice.ErrPasswordResetUnavailable):
			writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "password reset unavailable", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to request password reset", requestID)
		}
		return
	}
	if result.UserID != nil {
		s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, *result.UserID, "auth.password_reset.request", "user", strconv.Itoa(*result.UserID), nil, map[string]any{
			"delivery":   "outbox",
			"expires_at": result.ExpiresAt,
		}))
	}
	writeJSONAny(w, http.StatusAccepted, apiopenapi.PasswordResetAcceptedResponse{
		Data: apiopenapi.PasswordResetAccepted{
			Accepted: result.Accepted,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleConfirmPasswordReset(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	var body apiopenapi.ConfirmPasswordResetRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid password reset request", requestID)
		return
	}
	err := s.runtime.auth.ConfirmPasswordReset(r.Context(), body.Token, body.NewPassword)
	if err != nil {
		switch {
		case errors.Is(err, authservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid password reset request", requestID)
		case errors.Is(err, authservice.ErrPasswordResetInvalid):
			writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "invalid password reset token", requestID)
		case errors.Is(err, authservice.ErrPasswordResetUnavailable):
			writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "password reset unavailable", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to reset password", requestID)
		}
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.PasswordResetAcceptedResponse{
		Data: apiopenapi.PasswordResetAccepted{
			Accepted: true,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleRequestEmailVerification(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	var body apiopenapi.RequestEmailVerificationRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid email verification request", requestID)
		return
	}
	result, err := s.runtime.auth.RequestEmailVerification(r.Context(), string(body.Email))
	if err != nil {
		switch {
		case errors.Is(err, authservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid email verification request", requestID)
		case errors.Is(err, authservice.ErrEmailVerificationUnavailable):
			writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "email verification unavailable", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to request email verification", requestID)
		}
		return
	}
	if result.UserID != nil {
		s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, *result.UserID, "auth.email_verification.request", "user", strconv.Itoa(*result.UserID), nil, map[string]any{
			"delivery":   "outbox",
			"expires_at": result.ExpiresAt,
		}))
	}
	writeJSONAny(w, http.StatusAccepted, apiopenapi.EmailVerificationAcceptedResponse{
		Data: apiopenapi.EmailVerificationAccepted{
			Accepted: result.Accepted,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleConfirmEmailVerification(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	var body apiopenapi.ConfirmEmailVerificationRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid email verification request", requestID)
		return
	}
	err := s.runtime.auth.ConfirmEmailVerification(r.Context(), body.Token)
	if err != nil {
		switch {
		case errors.Is(err, authservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid email verification request", requestID)
		case errors.Is(err, authservice.ErrEmailVerificationInvalid):
			writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "invalid email verification token", requestID)
		case errors.Is(err, authservice.ErrEmailVerificationUnavailable):
			writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "email verification unavailable", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to verify email", requestID)
		}
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.EmailVerificationAcceptedResponse{
		Data: apiopenapi.EmailVerificationAccepted{
			Accepted: true,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleLoginSecondFactor(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	var body apiopenapi.LoginTwoFactorRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid two-factor login request", requestID)
		return
	}
	result, err := s.runtime.auth.CompleteSecondFactorLogin(r.Context(), body.ChallengeId, body.Code)
	if err != nil {
		switch {
		case errors.Is(err, authservice.ErrSecondFactorInvalid):
			writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "invalid two-factor code", requestID)
		case errors.Is(err, authservice.ErrSessionUserUnavailable):
			writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "user disabled", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to complete two-factor login", requestID)
		}
		return
	}
	s.setSessionCookie(w, result)
	writeJSONAny(w, http.StatusOK, apiopenapi.LoginResponse{
		Data: apiopenapi.SessionData{
			CsrfToken: result.Session.CSRFToken,
			ExpiresAt: result.Session.ExpiresAt,
			User:      toAPIUser(result.User),
		},
		RequestId: requestID,
	})
}

func registrationRPMLimit(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}

func registrationEmailSuffixAllowed(email string, allowlist []string) bool {
	if len(allowlist) == 0 {
		return true
	}
	normalized := strings.ToLower(strings.TrimSpace(email))
	_, domain, ok := strings.Cut(normalized, "@")
	if !ok || strings.TrimSpace(domain) == "" || strings.Contains(domain, "@") {
		return false
	}
	suffix := "@" + domain
	for _, allowed := range allowlist {
		if suffix == strings.ToLower(strings.TrimSpace(allowed)) {
			return true
		}
	}
	return false
}

func (s *Server) handleCurrentUserTOTPStatus(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	status, err := s.runtime.totp.Status(r.Context(), session.User.ID)
	if err != nil {
		writeTOTPServiceError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.TOTPStatusResponse{
		Data: apiopenapi.TOTPStatus{
			Enabled:      status.Enabled,
			PendingSetup: status.PendingSetup,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleCurrentUserTOTPSetup(w http.ResponseWriter, r *http.Request) {
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
	result, err := s.runtime.totp.BeginSetup(r.Context(), session.User)
	if err != nil {
		writeTOTPServiceError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.TOTPSetupResponse{
		Data: apiopenapi.TOTPSetup{
			Enabled:    result.Enabled,
			OtpAuthUrl: result.OTPAuthURL,
			Secret:     result.Secret,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleCurrentUserTOTPEnable(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.TOTPVerifyRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid totp enable request", requestID)
		return
	}
	result, err := s.runtime.totp.Enable(r.Context(), session.User.ID, body.Code)
	if err != nil {
		writeTOTPServiceError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.TOTPEnableResponse{
		Data: apiopenapi.TOTPEnableResult{
			Enabled:       result.Enabled,
			RecoveryCodes: result.RecoveryCodes,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleCurrentUserTOTPDisable(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.TOTPVerifyRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid totp disable request", requestID)
		return
	}
	if err := s.runtime.totp.Disable(r.Context(), session.User.ID, body.Code); err != nil {
		writeTOTPServiceError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.TOTPStatusResponse{
		Data:      apiopenapi.TOTPStatus{Enabled: false, PendingSetup: false},
		RequestId: requestID,
	})
}

func (s *Server) setSessionCookie(w http.ResponseWriter, result authcontract.LoginResult) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    result.Session.ID,
		Path:     "/",
		Expires:  result.Session.ExpiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.Server.Mode != "local",
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.Server.Mode != "local",
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
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
	if err := s.runtime.auth.Logout(r.Context(), session.Session.ID); err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to logout", requestID)
		return
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// handleRevokeAllCurrentUserSessions signs the user out of every session,
// including the current one, by bulk-revoking their console sessions. The
// caller's cookie is cleared so the browser returns to the sign-in screen.
func (s *Server) handleRevokeAllCurrentUserSessions(w http.ResponseWriter, r *http.Request) {
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
	if err := s.runtime.auth.LogoutUser(r.Context(), session.User.ID); err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to revoke sessions", requestID)
		return
	}
	s.clearSessionCookie(w)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user.sessions_revoke_all", "user", strconv.Itoa(session.User.ID), nil, nil))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCurrentUser(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}

	user := s.currentUserWithAvatar(r.Context(), session.User)
	writeJSONAny(w, http.StatusOK, apiopenapi.UserResponse{
		Data:      toAPIUser(user),
		RequestId: requestID,
	})
}

func (s *Server) handleCurrentUserAuthIdentities(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	identities, err := s.runtime.users.ListAuthIdentities(r.Context(), session.User.ID)
	if err != nil {
		writeUserServiceError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.CurrentUserAuthIdentityListResponse{
		Data:      toAPICurrentUserAuthIdentities(identities),
		RequestId: requestID,
	})
}

func (s *Server) handleUnbindCurrentUserAuthIdentity(w http.ResponseWriter, r *http.Request) {
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
	identityID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || identityID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid auth identity id", requestID)
		return
	}
	before, _ := s.runtime.users.ListAuthIdentities(r.Context(), session.User.ID)
	identities, err := s.runtime.users.UnbindAuthIdentity(r.Context(), session.User.ID, identityID)
	if err != nil {
		writeUserServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user.auth_identity_unbind", "user_auth_identities", strconv.Itoa(identityID), authIdentityAuditSnapshot(before), authIdentityAuditSnapshot(identities)))
	writeJSONAny(w, http.StatusOK, apiopenapi.CurrentUserAuthIdentityListResponse{
		Data:      toAPICurrentUserAuthIdentities(identities),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateCurrentUser(w http.ResponseWriter, r *http.Request) {
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

	var body apiopenapi.UpdateCurrentUserProfileRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid profile update request", requestID)
		return
	}
	before := session.User
	updated, err := s.runtime.users.UpdateProfile(r.Context(), session.User.ID, usersservice.UpdateProfileRequest{Name: body.Name})
	if err != nil {
		writeUserServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user.profile_update", "user", strconv.Itoa(session.User.ID), userAuditSnapshotFromUser(before), userAuditSnapshot(updated)))
	user := s.currentUserWithAvatar(r.Context(), updated.User)
	writeJSONAny(w, http.StatusOK, apiopenapi.UserResponse{
		Data:      toAPIUser(user),
		RequestId: requestID,
	})
}

func (s *Server) handleUploadCurrentUserAvatar(w http.ResponseWriter, r *http.Request) {
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
	if s.runtime.userAvatars == nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "avatar service unavailable", requestID)
		return
	}
	file, err := currentUserAvatarUploadFile(w, r)
	if err != nil {
		writeAvatarServiceError(w, err, requestID)
		return
	}
	avatar, err := s.runtime.userAvatars.Upsert(r.Context(), session.User.ID, bytes.NewReader(file), &session.User.ID)
	if err != nil {
		writeAvatarServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user.avatar_update", "user_avatar", strconv.Itoa(session.User.ID), nil, avatarAuditSnapshot(avatar)))
	writeJSONAny(w, http.StatusOK, apiopenapi.UserAvatarResponse{
		Data:      toAPIUserAvatar(avatar, s.userAvatarURL(r, session.User.ID, avatar.SHA256)),
		RequestId: requestID,
	})
}

func (s *Server) handleDeleteCurrentUserAvatar(w http.ResponseWriter, r *http.Request) {
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
	if s.runtime.userAvatars == nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "avatar service unavailable", requestID)
		return
	}
	before, _ := s.runtime.userAvatars.Get(r.Context(), session.User.ID)
	if err := s.runtime.userAvatars.Delete(r.Context(), session.User.ID, &session.User.ID); err != nil {
		writeAvatarServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user.avatar_delete", "user_avatar", strconv.Itoa(session.User.ID), avatarAuditSnapshot(before), nil))
	writeJSONAny(w, http.StatusOK, apiopenapi.DeleteResponse{
		Data: struct {
			Deleted bool `json:"deleted"`
		}{Deleted: true},
		RequestId: requestID,
	})
}

func (s *Server) handleGetUserAvatar(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireConsoleSession(r); err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	userID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || userID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user id", requestID)
		return
	}
	if s.runtime.userAvatars == nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "avatar service unavailable", requestID)
		return
	}
	avatar, err := s.runtime.userAvatars.Get(r.Context(), userID)
	if err != nil {
		writeAvatarServiceError(w, err, requestID)
		return
	}
	w.Header().Set("Content-Type", avatar.ContentType)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("ETag", `"`+avatar.SHA256+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(avatar.Content)
}

func (s *Server) handleChangeCurrentUserPassword(w http.ResponseWriter, r *http.Request) {
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

	var body apiopenapi.ChangeCurrentUserPasswordRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid password change request", requestID)
		return
	}
	before := session.User
	updated, err := s.runtime.users.ChangePassword(r.Context(), session.User.ID, usersservice.ChangePasswordRequest{
		CurrentPassword: body.CurrentPassword,
		NewPassword:     body.NewPassword,
	})
	if err != nil {
		writeChangePasswordError(w, err, requestID)
		return
	}
	if err := s.runtime.auth.LogoutUser(r.Context(), session.User.ID); err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to revoke sessions", requestID)
		return
	}
	s.clearSessionCookie(w)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user.password_change", "user", strconv.Itoa(session.User.ID), userAuditSnapshotFromUser(before), userAuditSnapshot(updated)))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCurrentUserAnnouncements(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	list, err := s.runtime.adminControl.ListUserAnnouncements(r.Context(), session.User, listOptionsFromRequest(r))
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.UserAnnouncementListResponse{
		Data:       toAPIUserAnnouncements(list.Items),
		Pagination: paginationWithRequest(r, list.Total),
		RequestId:  requestID,
		Unread:     list.Unread,
	})
}

func (s *Server) handleMarkCurrentUserAnnouncementRead(w http.ResponseWriter, r *http.Request) {
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
	id, ok := pathID(w, r, requestID)
	if !ok {
		return
	}
	item, err := s.runtime.adminControl.MarkUserAnnouncementRead(r.Context(), session.User, id)
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.UserAnnouncementResponse{
		Data:      toAPIUserAnnouncement(item),
		RequestId: requestID,
	})
}

func (s *Server) handleCurrentUserBalance(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	user, err := s.runtime.users.FindByID(r.Context(), session.User.ID)
	if err != nil {
		writeUserServiceError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.UserBalanceResponse{
		Data: apiopenapi.UserBalance{
			UserId:   apiopenapi.Id(strconv.Itoa(user.ID)),
			Balance:  user.Balance,
			Currency: user.Currency,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleRedeemCurrentUserRedeemCode(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.RedeemCodeRedemptionRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid redeem code request", requestID)
		return
	}
	result, err := s.runtime.adminControl.RedeemCode(r.Context(), session.User, admincontrolcontract.RedeemCodeRedemptionRequest{
		Code: body.Code,
	})
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.RedeemCodeRedemptionResponse{
		Data:      toAPIRedeemCodeRedemptionResult(result),
		RequestId: requestID,
	})
}

func (s *Server) handleCurrentUserAffiliate(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	summary, err := s.runtime.affiliate.GetSummary(r.Context(), session.User.ID)
	if err != nil {
		writeAffiliateServiceError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AffiliateSummaryResponse{
		Data:      toAPIAffiliateSummary(summary),
		RequestId: requestID,
	})
}

func (s *Server) handleCurrentUserAffiliateLedger(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	items, err := s.runtime.affiliate.ListLedgersByUser(r.Context(), session.User.ID)
	if err != nil {
		writeAffiliateServiceError(w, err, requestID)
		return
	}
	data := make([]apiopenapi.AffiliateLedgerEntry, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIAffiliateLedgerEntry(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AffiliateLedgerEntryListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleCurrentUserAffiliateTransferToBalance(w http.ResponseWriter, r *http.Request) {
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
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "idempotency key is required", requestID)
		return
	}
	var body apiopenapi.AffiliateTransferToBalanceRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid affiliate transfer request", requestID)
		return
	}
	result, err := s.runtime.affiliate.TransferToBalance(r.Context(), affiliatecontract.TransferToBalanceRequest{
		UserID:         session.User.ID,
		Amount:         body.Amount,
		Currency:       optionalStringValue(body.Currency),
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		writeAffiliateServiceError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AffiliateTransferToBalanceResponse{
		Data:      toAPIAffiliateTransferToBalanceResult(result),
		RequestId: requestID,
	})
}

func (s *Server) handleCurrentUserUsage(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	items, err := s.runtime.usage.ListByUser(r.Context(), session.User.ID)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list usage logs", requestID)
		return
	}
	data := make([]apiopenapi.UsageLog, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIUsageLog(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.UsageLogListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleCurrentUserSubscriptions(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	items, err := s.runtime.subscriptions.ListUserSubscriptionsByUser(r.Context(), session.User.ID)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list subscriptions", requestID)
		return
	}
	data := make([]apiopenapi.UserSubscription, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIUserSubscription(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.UserSubscriptionListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleListPaymentMethods(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireConsoleSession(r); err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	items, err := s.runtime.payments.ListMethods(r.Context())
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	data := make([]apiopenapi.PaymentMethod, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIPaymentMethod(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.PaymentMethodListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleCreatePaymentOrder(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreatePaymentOrderRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid payment order request", requestID)
		return
	}
	order, err := s.runtime.payments.CreateOrder(r.Context(), paymentcontract.CreateOrderRequest{
		UserID:      session.User.ID,
		Method:      body.Method,
		Amount:      body.Amount,
		Currency:    optionalStringValue(body.Currency),
		ProductType: paymentcontract.ProductType(body.ProductType),
		ProductID:   optionalStringValue(body.ProductId),
		PromoCode:   optionalStringValue(body.PromoCode),
		ExpiresAt:   body.ExpiresAt,
		Metadata:    jsonObjectToMap(body.Metadata),
	})
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusCreated, apiopenapi.PaymentOrderResponse{
		Data:      toAPIPaymentOrder(order),
		RequestId: requestID,
	})
}

func (s *Server) handleListPaymentOrders(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	items, err := s.runtime.payments.ListOrdersByUser(r.Context(), session.User.ID)
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	data := make([]apiopenapi.PaymentOrder, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIPaymentOrder(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.PaymentOrderListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleGetPaymentOrder(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	orderID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || orderID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid payment order id", requestID)
		return
	}
	order, err := s.runtime.payments.FindOrderByID(r.Context(), orderID)
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	if order.UserID != session.User.ID {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "payment order not found", requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.PaymentOrderResponse{
		Data:      toAPIPaymentOrder(order),
		RequestId: requestID,
	})
}

func (s *Server) handleCancelPaymentOrder(w http.ResponseWriter, r *http.Request) {
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
	orderID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || orderID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid payment order id", requestID)
		return
	}
	order, err := s.runtime.payments.CancelOrder(r.Context(), session.User.ID, orderID)
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.PaymentOrderResponse{
		Data:      toAPIPaymentOrder(order),
		RequestId: requestID,
	})
}

func (s *Server) handlePaymentWebhook(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	provider := strings.TrimSpace(r.PathValue("provider"))
	var body apiopenapi.PaymentWebhookRequest
	if provider == "stripe" || provider == "wechat" {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.cfg.Gateway.MaxBodySize))
		if err != nil {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid payment webhook request", requestID)
			return
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid payment webhook request", requestID)
			return
		}
		body["raw_body"] = string(raw)
	} else {
		if err := s.decodeJSONBody(w, r, &body); err != nil {
			writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid payment webhook request", requestID)
			return
		}
	}
	result, err := s.runtime.payments.HandleWebhook(r.Context(), paymentcontract.WebhookRequest{
		Provider: provider,
		Headers:  singleValueHeaders(r.Header),
		Payload:  map[string]any(body),
	})
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	if provider == "alipay" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.PaymentWebhookResponse{
		Data: apiopenapi.PaymentWebhookResult{
			Handled: result.Handled,
			Order:   toAPIPaymentOrder(result.Order),
		},
		RequestId: requestID,
	})
}

func (s *Server) handleListApiKeys(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}

	keys, err := s.runtime.apiKeys.ListByUser(r.Context(), session.User.ID)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list api keys", requestID)
		return
	}
	keys = filterApiKeys(keys, r.URL.Query().Get("status"))
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].CreatedAt.Before(keys[j].CreatedAt)
	})

	page := 1
	pageSize := 20
	if params := r.URL.Query().Get("page"); params != "" {
		if v, err := strconv.Atoi(params); err == nil && v > 0 {
			page = v
		}
	}
	if params := r.URL.Query().Get("page_size"); params != "" {
		if v, err := strconv.Atoi(params); err == nil && v > 0 {
			pageSize = v
		}
	}

	paged, total, hasNext := paginateApiKeys(keys, page, pageSize)
	data := make([]apiopenapi.ApiKey, 0, len(paged))
	for _, key := range paged {
		data = append(data, toAPIKey(key))
	}

	writeJSONAny(w, http.StatusOK, apiopenapi.ApiKeyListResponse{
		Data: data,
		Pagination: apiopenapi.Pagination{
			Page:     page,
			PageSize: pageSize,
			Total:    total,
			HasNext:  hasNext,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleCreateApiKey(w http.ResponseWriter, r *http.Request) {
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

	var body apiopenapi.CreateApiKeyRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid api key request", requestID)
		return
	}

	groupIDs, err := idsToInts(body.GroupIds)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid group ids", requestID)
		return
	}

	result, err := s.runtime.apiKeys.Create(r.Context(), apikeycontract.CreateRequest{
		UserID:           session.User.ID,
		Name:             body.Name,
		Scopes:           derefStrings(body.Scopes),
		AllowedModels:    derefStrings(body.AllowedModels),
		GroupIDs:         groupIDs,
		RPMLimit:         body.RpmLimit,
		TPMLimit:         body.TpmLimit,
		ConcurrencyLimit: body.ConcurrencyLimit,
		RequestLimit5h:   body.RequestLimit5h,
		RequestLimit1d:   body.RequestLimit1d,
		RequestLimit7d:   body.RequestLimit7d,
		AllowedIPs:       derefStrings(body.AllowedIps),
		DeniedIPs:        derefStrings(body.DeniedIps),
		ExpiresAt:        body.ExpiresAt,
	})
	if err != nil {
		switch {
		case errors.Is(err, apikeyservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid api key request", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to create api key", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "api_key.create", "api_key", strconv.Itoa(result.Key.ID), nil, map[string]any{
		"name":           result.Key.Name,
		"prefix":         result.Key.Prefix,
		"scopes":         result.Key.Scopes,
		"allowed_models": result.Key.AllowedModels,
	}))

	writeJSONAny(w, http.StatusCreated, apiopenapi.CreateApiKeyResponse{
		Data: apiopenapi.ApiKeySecretData{
			ApiKey:       toAPIKey(result.Key),
			PlaintextKey: result.PlaintextKey,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateApiKey(w http.ResponseWriter, r *http.Request) {
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

	keyID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid api key id", requestID)
		return
	}
	before, err := s.apiKeyByUser(r.Context(), session.User.ID, keyID)
	if err != nil {
		switch {
		case errors.Is(err, apikeyservice.ErrKeyNotFound):
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "api key not found", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to load api key", requestID)
		}
		return
	}

	var body apiopenapi.UpdateApiKeyRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid api key update request", requestID)
		return
	}

	var groupIDs *[]int
	if body.GroupIds != nil {
		parsed, err := idsToInts(body.GroupIds)
		if err != nil {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid group ids", requestID)
			return
		}
		groupIDs = &parsed
	}

	updated, err := s.runtime.apiKeys.Update(r.Context(), apikeycontract.UpdateRequest{
		UserID:           session.User.ID,
		KeyID:            keyID,
		Name:             body.Name,
		Status:           toAPIKeyStatusPtr(body.Status),
		Scopes:           body.Scopes,
		AllowedModels:    body.AllowedModels,
		GroupIDs:         groupIDs,
		RPMLimit:         body.RpmLimit,
		TPMLimit:         body.TpmLimit,
		ConcurrencyLimit: body.ConcurrencyLimit,
		RequestLimit5h:   body.RequestLimit5h,
		RequestLimit1d:   body.RequestLimit1d,
		RequestLimit7d:   body.RequestLimit7d,
		AllowedIPs:       body.AllowedIps,
		DeniedIPs:        body.DeniedIps,
		ExpiresAt:        body.ExpiresAt,
	})
	if err != nil {
		switch {
		case errors.Is(err, apikeyservice.ErrKeyNotFound):
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "api key not found", requestID)
		case errors.Is(err, apikeyservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid api key update request", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to update api key", requestID)
		}
		return
	}

	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "api_key.update", "api_key", strconv.Itoa(updated.ID), apiKeyAuditSnapshot(before), apiKeyAuditSnapshot(updated)))
	writeJSONAny(w, http.StatusOK, apiopenapi.ApiKeyResponse{
		Data:      toAPIKey(updated),
		RequestId: requestID,
	})
}

func currentUserAvatarUploadFile(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if !strings.HasPrefix(contentType, "multipart/form-data") {
		return nil, usersservice.ErrInvalidInput
	}
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, usersservice.ErrInvalidInput
	}
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, usersservice.ErrInvalidInput
		}
		if part.FormName() != "avatar" {
			_ = part.Close()
			continue
		}
		defer func() { _ = part.Close() }()
		return io.ReadAll(http.MaxBytesReader(w, part, usersservice.MaxAvatarUploadBytes+1))
	}
	return nil, usersservice.ErrInvalidInput
}

func (s *Server) currentUserWithAvatar(ctx context.Context, user userscontract.User) userscontract.User {
	if s.runtime.userAvatars == nil {
		return user
	}
	return s.runtime.userAvatars.DecorateUser(ctx, user, s.userAvatarPath(user.ID, ""))
}

func (s *Server) userAvatarURL(r *http.Request, userID int, sha string) string {
	path := s.userAvatarPath(userID, sha)
	if r == nil {
		return path
	}
	return path
}

func (s *Server) userAvatarPath(userID int, sha string) string {
	path := "/api/v1/users/" + strconv.Itoa(userID) + "/avatar"
	sha = strings.TrimSpace(sha)
	if sha != "" {
		path += "?v=" + sha
	}
	return path
}

func toAPIUserAvatar(avatar usersservice.Avatar, url string) apiopenapi.UserAvatar {
	return apiopenapi.UserAvatar{
		ByteSize:    avatar.ByteSize,
		ContentType: apiopenapi.UserAvatarContentType(avatar.ContentType),
		Height:      avatar.Height,
		Sha256:      avatar.SHA256,
		UpdatedAt:   avatar.UpdatedAt,
		Url:         url,
		UserId:      apiopenapi.Id(strconv.Itoa(avatar.UserID)),
		Width:       avatar.Width,
	}
}

func avatarAuditSnapshot(avatar usersservice.Avatar) map[string]any {
	if avatar.UserID <= 0 {
		return nil
	}
	return map[string]any{
		"user_id":      avatar.UserID,
		"content_type": avatar.ContentType,
		"byte_size":    avatar.ByteSize,
		"sha256":       avatar.SHA256,
		"width":        avatar.Width,
		"height":       avatar.Height,
		"updated_at":   avatar.UpdatedAt,
	}
}

func writeAvatarServiceError(w http.ResponseWriter, err error, requestID string) {
	var maxBytesErr *http.MaxBytesError
	switch {
	case errors.Is(err, usersservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid avatar request", requestID)
	case errors.Is(err, usersservice.ErrAvatarTooLarge), errors.As(err, &maxBytesErr):
		writeStandardError(w, http.StatusRequestEntityTooLarge, apiopenapi.INVALIDREQUEST, "avatar is too large", requestID)
	case errors.Is(err, usersservice.ErrAvatarNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "avatar not found", requestID)
	default:
		if strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			writeStandardError(w, http.StatusRequestEntityTooLarge, apiopenapi.INVALIDREQUEST, "avatar is too large", requestID)
			return
		}
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "avatar service failed", requestID)
	}
}

func authIdentityAuditSnapshot(identities []userscontract.UserAuthIdentity) map[string]any {
	out := make([]map[string]any, 0, len(identities))
	for _, identity := range identities {
		out = append(out, map[string]any{
			"id":                identity.ID,
			"user_id":           identity.UserID,
			"provider":          identity.Provider,
			"provider_key":      identity.ProviderKey,
			"external":          identity.External,
			"email_verified":    identity.EmailVerified,
			"can_unbind":        identity.CanUnbind,
			"unbind_blocked_by": identity.UnbindBlockedBy,
		})
	}
	return map[string]any{"identities": out}
}
