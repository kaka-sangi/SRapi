package httpserver

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"

	affiliatecontract "github.com/srapi/srapi/apps/api/internal/modules/affiliate/contract"
	affiliateservice "github.com/srapi/srapi/apps/api/internal/modules/affiliate/service"
	authservice "github.com/srapi/srapi/apps/api/internal/modules/auth/service"
	userattributesservice "github.com/srapi/srapi/apps/api/internal/modules/userattributes/service"
	usersservice "github.com/srapi/srapi/apps/api/internal/modules/users/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

var errPasswordlessRegistrationDisabled = errors.New("passwordless registration disabled")

type passwordlessCodeRequest struct {
	Email      string                           `json:"email"`
	Name       string                           `json:"name,omitempty"`
	InviteCode string                           `json:"invite_code,omitempty"`
	Attributes []currentUserAttributeValueInput `json:"attributes,omitempty"`
}

type passwordlessLoginRequest struct {
	Token string `json:"token"`
}

func (s *Server) handleRequestPasswordlessCode(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if !s.verifyCaptcha(w, r, requestID) {
		return
	}
	var body passwordlessCodeRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid passwordless request", requestID)
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if email == "" || !strings.Contains(email, "@") {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid passwordless request", requestID)
		return
	}
	if err := s.ensurePasswordlessUser(r, body, email); err != nil {
		writePasswordlessRegistrationError(w, err, requestID)
		return
	}
	result, err := s.runtime.auth.RequestPasswordlessLogin(r.Context(), email)
	if err != nil {
		switch {
		case errors.Is(err, authservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid passwordless request", requestID)
		case errors.Is(err, authservice.ErrEmailVerificationUnavailable):
			writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "passwordless login unavailable", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to request passwordless login", requestID)
		}
		return
	}
	if result.UserID != nil {
		s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, *result.UserID, "auth.passwordless.request", "user", strconv.Itoa(*result.UserID), nil, map[string]any{
			"delivery":   "outbox",
			"expires_at": result.ExpiresAt,
		}))
	}
	writeJSONAny(w, http.StatusAccepted, map[string]any{
		"data": map[string]any{
			"accepted": true,
		},
		"request_id": requestID,
	})
}

func (s *Server) handlePasswordlessLogin(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	var body passwordlessLoginRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid passwordless login request", requestID)
		return
	}
	result, err := s.runtime.auth.CompletePasswordlessLogin(r.Context(), body.Token)
	if err != nil {
		switch {
		case errors.Is(err, authservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid passwordless login request", requestID)
		case errors.Is(err, authservice.ErrEmailVerificationInvalid):
			writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "invalid passwordless token", requestID)
		case errors.Is(err, authservice.ErrEmailVerificationUnavailable):
			writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "passwordless login unavailable", requestID)
		case errors.Is(err, authservice.ErrSessionUserUnavailable):
			writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "user disabled", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to login", requestID)
		}
		return
	}
	s.setSessionCookie(w, result)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, result.User.ID, "user.login_passwordless", "user", strconv.Itoa(result.User.ID), nil, userAuditSnapshotFromUser(result.User)))
	writeJSONAny(w, http.StatusOK, apiopenapi.LoginResponse{
		Data: apiopenapi.SessionData{
			CsrfToken: result.Session.CSRFToken,
			ExpiresAt: result.Session.ExpiresAt,
			User:      toAPIUser(result.User),
		},
		RequestId: requestID,
	})
}

func (s *Server) ensurePasswordlessUser(r *http.Request, body passwordlessCodeRequest, email string) error {
	if _, err := s.runtime.users.FindByEmail(r.Context(), email); err == nil {
		return nil
	} else if !errors.Is(err, usersservice.ErrUserNotFound) {
		return err
	}

	settings, err := s.runtime.adminControl.GetAdminSettings(r.Context())
	if err != nil {
		return err
	}
	if !settings.Security.RegistrationEnabled {
		return errPasswordlessRegistrationDisabled
	}
	if !registrationEmailSuffixAllowed(email, settings.Security.RegistrationEmailSuffixAllowlist) {
		return authservice.ErrInvalidInput
	}
	attributeValues := make(map[int]string, len(body.Attributes))
	for _, item := range body.Attributes {
		attributeValues[item.DefinitionID] = item.Value
	}
	if err := s.runtime.userAttributes.ValidateRequiredValues(r.Context(), attributeValues); err != nil {
		return err
	}
	password, err := passwordlessGeneratedPassword()
	if err != nil {
		return err
	}
	inviteCode := strings.TrimSpace(body.InviteCode)
	if inviteCode != "" {
		if _, err := s.runtime.affiliate.ValidateInviteCode(r.Context(), inviteCode); err != nil {
			return err
		}
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = emailName(email)
	}
	user, err := s.runtime.users.Create(r.Context(), usersservice.CreateRequest{
		Email:    email,
		Name:     name,
		Password: password,
		Balance:  settings.Users.DefaultBalance,
		RPMLimit: registrationRPMLimit(settings.Users.RPMLimitDefault),
	})
	if err != nil {
		return err
	}
	if inviteCode != "" {
		if _, err := s.runtime.affiliate.BindInvite(r.Context(), affiliatecontract.BindInviteRequest{
			InviteeUserID: user.ID,
			Code:          inviteCode,
		}); err != nil {
			return err
		}
	}
	if _, err := s.runtime.userAttributes.SetUserValues(r.Context(), user.ID, attributeValues); err != nil {
		return err
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, user.ID, "user.register_passwordless", "user", strconv.Itoa(user.ID), nil, userAuditSnapshot(user)))
	return nil
}

func writePasswordlessRegistrationError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, authservice.ErrInvalidInput),
		errors.Is(err, usersservice.ErrInvalidInput),
		errors.Is(err, usersservice.ErrUserAlreadyExists),
		errors.Is(err, userattributesservice.ErrInvalidInput),
		errors.Is(err, affiliatecontract.ErrNotFound),
		errors.Is(err, affiliatecontract.ErrConflict),
		errors.Is(err, affiliateservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid passwordless request", requestID)
	case errors.Is(err, authservice.ErrEmailVerificationUnavailable):
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "passwordless login unavailable", requestID)
	case errors.Is(err, errPasswordlessRegistrationDisabled):
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "registration disabled", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to request passwordless login", requestID)
	}
}

func passwordlessGeneratedPassword() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func emailName(email string) string {
	local, _, ok := strings.Cut(strings.TrimSpace(email), "@")
	if !ok || strings.TrimSpace(local) == "" {
		return "User"
	}
	return strings.TrimSpace(local)
}
