package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	authcontract "github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
	authservice "github.com/srapi/srapi/apps/api/internal/modules/auth/service"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	usersservice "github.com/srapi/srapi/apps/api/internal/modules/users/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

const (
	oauthTokenAuthMethodNone        = "none"
	oauthProviderHTTPTimeout        = 10 * time.Second
	oauthProviderBodyLimit          = 1 << 20
	pendingOAuthActionCreateAccount = "create_account"
	// pendingOAuthAdoptDisplayNameMaxRunes mirrors the self-service profile name
	// limit enforced by users.UpdateProfile so adoption never produces a name the
	// user could not have set themselves.
	pendingOAuthAdoptDisplayNameMaxRunes = 120
)

func (s *Server) handleStartOAuthAuthorization(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	provider := userscontract.AuthIdentityProvider(strings.ToLower(strings.TrimSpace(r.PathValue("provider"))))
	if !validOAuthStartProvider(provider) {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid oauth provider", requestID)
		return
	}
	intent := authcontract.PendingOAuthIntent(strings.ToLower(strings.TrimSpace(r.URL.Query().Get("intent"))))
	if intent == "" {
		intent = authcontract.PendingOAuthIntentLogin
	}
	if intent != authcontract.PendingOAuthIntentLogin {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "unsupported oauth intent", requestID)
		return
	}

	settings, err := s.runtime.adminControl.GetAdminSettings(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	if !settings.Security.OAuthEnabled {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "oauth disabled", requestID)
		return
	}
	config, ok := selectOAuthProviderConfig(settings.Security, provider, r.URL.Query().Get("provider_key"))
	if !ok {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "oauth provider unavailable", requestID)
		return
	}
	result, err := s.runtime.auth.StartOAuthAuthorization(authservice.StartOAuthAuthorizationRequest{
		Intent:     intent,
		Provider:   provider,
		RedirectTo: r.URL.Query().Get("redirect"),
		Config: authservice.OAuthAuthorizationProviderConfig{
			Provider:     provider,
			ProviderKey:  config.ProviderKey,
			ClientID:     config.ClientID,
			AuthorizeURL: config.AuthorizeURL,
			RedirectURI:  config.RedirectURI,
			Scopes:       append([]string(nil), config.Scopes...),
		},
	})
	if err != nil {
		switch {
		case errors.Is(err, authservice.ErrOAuthUnavailable):
			writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "oauth unavailable", requestID)
		case errors.Is(err, authservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid oauth request", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to start oauth", requestID)
		}
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oauthFlowCookieName,
		Value:    result.FlowCookieValue,
		Path:     oauthFlowCookiePath,
		Expires:  result.ExpiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.Server.Mode != "local",
	})
	http.Redirect(w, r, result.AuthorizationURL, http.StatusFound)
}

func (s *Server) handleCompleteOAuthAuthorization(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	requestID := requestIDFromContext(r.Context())
	provider := userscontract.AuthIdentityProvider(strings.ToLower(strings.TrimSpace(r.PathValue("provider"))))
	if !validOAuthStartProvider(provider) {
		s.clearOAuthFlowCookie(w)
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid oauth provider", requestID)
		return
	}
	if strings.TrimSpace(r.URL.Query().Get("error")) != "" {
		s.clearOAuthFlowCookie(w)
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "oauth provider rejected authorization", requestID)
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || state == "" {
		s.clearOAuthFlowCookie(w)
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid oauth callback", requestID)
		return
	}
	cookie, err := r.Cookie(oauthFlowCookieName)
	if err != nil {
		s.clearOAuthFlowCookie(w)
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "missing oauth flow", requestID)
		return
	}
	flow, err := s.runtime.auth.DecodeOAuthAuthorizationFlow(cookie.Value)
	if err != nil || flow.Provider != provider || flow.State != state {
		s.clearOAuthFlowCookie(w)
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid oauth flow", requestID)
		return
	}

	settings, err := s.runtime.adminControl.GetAdminSettings(r.Context())
	if err != nil {
		s.clearOAuthFlowCookie(w)
		writeAdminControlError(w, err, requestID)
		return
	}
	if !settings.Security.OAuthEnabled {
		s.clearOAuthFlowCookie(w)
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "oauth disabled", requestID)
		return
	}
	config, ok := selectOAuthProviderConfig(settings.Security, provider, flow.ProviderKey)
	if !ok || strings.TrimSpace(config.ClientID) != flow.ClientID || strings.TrimSpace(config.RedirectURI) != flow.RedirectURI {
		s.clearOAuthFlowCookie(w)
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "oauth provider unavailable", requestID)
		return
	}
	if !callbackOAuthProviderConfigReady(config) {
		s.clearOAuthFlowCookie(w)
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "oauth callback unavailable", requestID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), oauthProviderHTTPTimeout)
	defer cancel()
	accessToken, idToken, err := exchangeOAuthAuthorizationCode(ctx, http.DefaultClient, config, flow, code, s.runtime.oauthClientSecret(flow.ProviderKey))
	if err != nil {
		s.clearOAuthFlowCookie(w)
		s.logger.Warn("oauth token exchange failed", "provider", provider, "provider_key", flow.ProviderKey, "error", err)
		writeStandardError(w, http.StatusBadGateway, apiopenapi.PROVIDERAUTHFAILED, "oauth provider exchange failed", requestID)
		return
	}
	if issuer := s.runtime.oauthIssuer(flow.ProviderKey); issuer != "" {
		if err := verifyOIDCIDToken(ctx, issuer, flow.ClientID, idToken, flow.Nonce); err != nil {
			s.clearOAuthFlowCookie(w)
			s.logger.Warn("oauth id_token verification failed", "provider", provider, "provider_key", flow.ProviderKey, "error", err)
			writeStandardError(w, http.StatusBadGateway, apiopenapi.PROVIDERAUTHFAILED, "oauth id_token verification failed", requestID)
			return
		}
	}
	userInfo, err := fetchOAuthUserInfo(ctx, http.DefaultClient, config.UserInfoURL, accessToken)
	if err != nil {
		s.clearOAuthFlowCookie(w)
		s.logger.Warn("oauth userinfo fetch failed", "provider", provider, "provider_key", flow.ProviderKey, "error", err)
		writeStandardError(w, http.StatusBadGateway, apiopenapi.PROVIDERAUTHFAILED, "oauth provider profile failed", requestID)
		return
	}
	subject, profile, err := pendingOAuthProfileFromUserInfo(userInfo)
	if err != nil {
		s.clearOAuthFlowCookie(w)
		writeStandardError(w, http.StatusBadGateway, apiopenapi.PROVIDERAUTHFAILED, "oauth provider profile invalid", requestID)
		return
	}
	subjectHash, err := s.runtime.auth.HashOAuthProviderSubject(provider, flow.ProviderKey, subject)
	if err != nil {
		s.clearOAuthFlowCookie(w)
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "oauth unavailable", requestID)
		return
	}
	profile.SubjectHint = oauthSubjectHint(provider, subjectHash)
	pending, err := s.runtime.auth.CreatePendingOAuthSession(r.Context(), authservice.CreatePendingOAuthSessionRequest{
		Intent:              flow.Intent,
		Provider:            provider,
		ProviderKey:         flow.ProviderKey,
		ProviderSubjectHash: subjectHash,
		RedirectTo:          flow.RedirectTo,
		Profile:             profile,
	})
	if err != nil {
		s.clearOAuthFlowCookie(w)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to create oauth pending session", requestID)
		return
	}
	s.clearOAuthFlowCookie(w)
	http.SetCookie(w, &http.Cookie{
		Name:     oauthPendingCookieName,
		Value:    pending.SessionToken,
		Path:     oauthPendingCookiePath,
		Expires:  pending.Session.ExpiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.Server.Mode != "local",
	})
	http.Redirect(w, r, pending.Session.RedirectTo, http.StatusFound)
}

func (s *Server) handleGetPendingOAuthSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	requestID := requestIDFromContext(r.Context())
	cookie, err := r.Cookie(oauthPendingCookieName)
	if err != nil {
		s.clearOAuthPendingCookie(w)
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "pending oauth session required", requestID)
		return
	}
	session, err := s.runtime.auth.FindPendingOAuthSession(r.Context(), cookie.Value)
	if err != nil {
		s.clearOAuthPendingCookie(w)
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "pending oauth session invalid", requestID)
		return
	}
	preview, err := s.toOAuthPendingSession(r.Context(), session)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to inspect pending oauth session", requestID)
		return
	}
	if preview.NextStep == apiopenapi.CreateAccountRequired {
		actionToken, err := s.runtime.auth.IssuePendingOAuthActionToken(r.Context(), cookie.Value, pendingOAuthActionCreateAccount)
		if err != nil {
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to inspect pending oauth session", requestID)
			return
		}
		preview.CreateAccountAction = &apiopenapi.PendingOAuthAction{
			ExpiresAt: actionToken.ExpiresAt,
			Token:     actionToken.Token,
		}
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.OAuthPendingSessionResponse{
		Data:      preview,
		RequestId: requestID,
	})
}

func (s *Server) handleBindPendingOAuthCurrentUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
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
	var bindBody apiopenapi.PendingOAuthBindCurrentUserRequest
	if err := s.decodeJSONBody(w, r, &bindBody); err != nil && !errors.Is(err, io.EOF) {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid pending oauth bind request", requestID)
		return
	}
	adoptDisplayName := bindBody.AdoptDisplayName != nil && *bindBody.AdoptDisplayName
	cookie, err := r.Cookie(oauthPendingCookieName)
	if err != nil {
		s.clearOAuthPendingCookie(w)
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "pending oauth session required", requestID)
		return
	}
	pending, err := s.runtime.auth.FindPendingOAuthSession(r.Context(), cookie.Value)
	if err != nil {
		s.clearOAuthPendingCookie(w)
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "pending oauth session invalid", requestID)
		return
	}
	if pending.Intent != authcontract.PendingOAuthIntentLogin && pending.Intent != authcontract.PendingOAuthIntentBindCurrentUser {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "unsupported pending oauth intent", requestID)
		return
	}
	if pending.TargetUserID != nil && *pending.TargetUserID != session.User.ID {
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "pending oauth session targets another user", requestID)
		return
	}

	before, _ := s.runtime.users.ListAuthIdentities(r.Context(), session.User.ID)
	identities, err := s.runtime.users.BindAuthIdentity(r.Context(), usersservice.BindAuthIdentityRequest{
		UserID:              session.User.ID,
		Provider:            pending.Provider,
		ProviderKey:         pending.ProviderKey,
		ProviderSubjectHash: pending.ProviderSubjectHash,
		SubjectHint:         pending.SubjectHint,
		DisplayName:         pending.DisplayName,
		Email:               pending.ResolvedEmail,
		EmailVerified:       pending.EmailVerified,
		AvatarURL:           pending.AvatarURL,
	})
	if err != nil {
		writeUserServiceError(w, err, requestID)
		return
	}
	if _, err := s.runtime.auth.ConsumePendingOAuthSession(r.Context(), cookie.Value); err != nil {
		s.clearOAuthPendingCookie(w)
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "pending oauth session invalid", requestID)
		return
	}
	s.clearOAuthPendingCookie(w)
	_, displayNameAdopted := s.adoptPendingOAuthDisplayName(r.Context(), session.User.ID, session.User.Name, pending, adoptDisplayName)
	after := authIdentityAuditSnapshot(identities)
	after["display_name_adopted"] = displayNameAdopted
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user.auth_identity_bind_oauth", "user_auth_identities", strconv.Itoa(session.User.ID), authIdentityAuditSnapshot(before), after))
	writeJSONAny(w, http.StatusOK, apiopenapi.CurrentUserAuthIdentityListResponse{
		Data:      toAPICurrentUserAuthIdentities(identities),
		RequestId: requestID,
	})
}

func (s *Server) handleBindPendingOAuthLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	requestID := requestIDFromContext(r.Context())
	cookie, err := r.Cookie(oauthPendingCookieName)
	if err != nil {
		s.clearOAuthPendingCookie(w)
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "pending oauth session required", requestID)
		return
	}
	var body apiopenapi.PendingOAuthBindLoginRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid pending oauth bind-login request", requestID)
		return
	}
	adoptDisplayName := body.AdoptDisplayName != nil && *body.AdoptDisplayName
	prepared, err := s.runtime.auth.PreparePendingOAuthBindLogin(r.Context(), cookie.Value, string(body.Email), body.Password, adoptDisplayName)
	if err != nil {
		s.writePendingOAuthBindLoginError(w, err, requestID)
		return
	}
	if prepared.RequiresSecondFactor {
		writeJSONAny(w, http.StatusAccepted, apiopenapi.LoginTwoFactorRequiredResponse{
			Data: apiopenapi.LoginTwoFactorRequired{
				ChallengeId: prepared.SecondFactorChallengeID,
				ExpiresAt:   *prepared.SecondFactorChallengeUntil,
				Required:    apiopenapi.LoginTwoFactorRequiredRequired(true),
			},
			RequestId: requestID,
		})
		return
	}
	s.completePendingOAuthLoginBind(w, r, requestID, cookie.Value, prepared.User, adoptDisplayName)
}

func (s *Server) handleCompletePendingOAuthBindLoginTwoFactor(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	requestID := requestIDFromContext(r.Context())
	cookie, err := r.Cookie(oauthPendingCookieName)
	if err != nil {
		s.clearOAuthPendingCookie(w)
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "pending oauth session required", requestID)
		return
	}
	var body apiopenapi.PendingOAuthBindLoginTwoFactorRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid pending oauth bind-login two-factor request", requestID)
		return
	}
	completed, err := s.runtime.auth.CompletePendingOAuthBindLoginSecondFactor(r.Context(), cookie.Value, body.ChallengeId, body.Code)
	if err != nil {
		s.writePendingOAuthBindLoginError(w, err, requestID)
		return
	}
	s.completePendingOAuthLoginBind(w, r, requestID, cookie.Value, completed.User, completed.AdoptDisplayName)
}

func (s *Server) handleSendPendingOAuthEmailCompletion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	requestID := requestIDFromContext(r.Context())
	cookie, err := r.Cookie(oauthPendingCookieName)
	if err != nil {
		s.clearOAuthPendingCookie(w)
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "pending oauth session required", requestID)
		return
	}
	var body apiopenapi.PendingOAuthEmailCompletionRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid pending oauth email completion request", requestID)
		return
	}
	result, err := s.runtime.auth.RequestPendingOAuthEmailCompletion(r.Context(), cookie.Value, string(body.Email))
	if err != nil {
		s.writePendingOAuthEmailCompletionError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusAccepted, apiopenapi.PendingOAuthEmailCompletionAcceptedResponse{
		Data: apiopenapi.PendingOAuthEmailCompletionAccepted{
			Accepted:  result.Accepted,
			ExpiresAt: result.ExpiresAt,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleConfirmPendingOAuthEmailCompletion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	requestID := requestIDFromContext(r.Context())
	cookie, err := r.Cookie(oauthPendingCookieName)
	if err != nil {
		s.clearOAuthPendingCookie(w)
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "pending oauth session required", requestID)
		return
	}
	var body apiopenapi.ConfirmPendingOAuthEmailCompletionRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid pending oauth email completion request", requestID)
		return
	}
	pending, err := s.runtime.auth.ConfirmPendingOAuthEmailCompletion(r.Context(), cookie.Value, body.Token)
	if err != nil {
		s.writePendingOAuthEmailCompletionError(w, err, requestID)
		return
	}
	preview, err := s.toOAuthPendingSession(r.Context(), pending)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to inspect pending oauth session", requestID)
		return
	}
	if preview.NextStep == apiopenapi.CreateAccountRequired {
		actionToken, err := s.runtime.auth.IssuePendingOAuthActionToken(r.Context(), cookie.Value, pendingOAuthActionCreateAccount)
		if err != nil {
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to inspect pending oauth session", requestID)
			return
		}
		preview.CreateAccountAction = &apiopenapi.PendingOAuthAction{
			ExpiresAt: actionToken.ExpiresAt,
			Token:     actionToken.Token,
		}
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.OAuthPendingSessionResponse{
		Data:      preview,
		RequestId: requestID,
	})
}

func (s *Server) handleCreatePendingOAuthAccount(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	requestID := requestIDFromContext(r.Context())
	cookie, err := r.Cookie(oauthPendingCookieName)
	if err != nil {
		s.clearOAuthPendingCookie(w)
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "pending oauth session required", requestID)
		return
	}
	var body apiopenapi.PendingOAuthCreateAccountRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid pending oauth create-account request", requestID)
		return
	}
	pending, err := s.runtime.auth.VerifyPendingOAuthActionToken(r.Context(), cookie.Value, pendingOAuthActionCreateAccount, body.ActionToken)
	if err != nil {
		s.writePendingOAuthCreateAccountError(w, err, requestID)
		return
	}
	if pending.Intent != authcontract.PendingOAuthIntentLogin {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "unsupported pending oauth intent", requestID)
		return
	}
	if pending.TargetUserID != nil {
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "pending oauth session targets an existing user", requestID)
		return
	}
	if pending.ResolvedEmail == "" {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "pending oauth email completion required", requestID)
		return
	}
	if !strings.EqualFold(strings.TrimSpace(string(body.Email)), strings.TrimSpace(pending.ResolvedEmail)) {
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "pending oauth email mismatch", requestID)
		return
	}

	settings, err := s.runtime.adminControl.GetAdminSettings(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	if !settings.Security.RegistrationEnabled {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "registration disabled", requestID)
		return
	}
	if !registrationEmailSuffixAllowed(string(body.Email), settings.Security.RegistrationEmailSuffixAllowlist) {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid pending oauth create-account request", requestID)
		return
	}

	name := ""
	if body.Name != nil {
		name = strings.TrimSpace(*body.Name)
	}
	if name == "" {
		name = strings.TrimSpace(pending.DisplayName)
	}
	if name == "" {
		name = strings.TrimSpace(pending.ResolvedEmail)
	}
	var verifiedAt *time.Time
	if pending.EmailVerified {
		now := time.Now().UTC()
		verifiedAt = &now
	}
	user, err := s.runtime.users.Create(r.Context(), usersservice.CreateRequest{
		Email:           string(body.Email),
		Name:            name,
		Password:        body.Password,
		Balance:         settings.Users.DefaultBalance,
		RPMLimit:        registrationRPMLimit(settings.Users.RPMLimitDefault),
		EmailVerifiedAt: verifiedAt,
	})
	if err != nil {
		s.writePendingOAuthCreateAccountError(w, err, requestID)
		return
	}
	if _, err := s.bindPendingOAuthIdentityForUser(r.Context(), user.ID, pending); err != nil {
		s.rollbackPendingOAuthCreatedUser(r.Context(), user.ID)
		s.writePendingOAuthCreateAccountError(w, err, requestID)
		return
	}
	if _, err := s.runtime.auth.ConsumePendingOAuthSession(r.Context(), cookie.Value); err != nil {
		s.rollbackPendingOAuthCreatedUser(r.Context(), user.ID)
		s.clearOAuthPendingCookie(w)
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "pending oauth session invalid", requestID)
		return
	}
	loginResult, err := s.runtime.auth.CreateSessionForUser(r.Context(), user)
	if err != nil {
		s.clearOAuthPendingCookie(w)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to create session", requestID)
		return
	}
	s.setSessionCookie(w, loginResult)
	s.clearOAuthPendingCookie(w)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, user.ID, "user.register_oauth_pending", "user", strconv.Itoa(user.ID), nil, map[string]any{
		"provider":       pending.Provider,
		"provider_key":   pending.ProviderKey,
		"email":          user.Email,
		"email_verified": pending.EmailVerified,
	}))
	writeJSONAny(w, http.StatusCreated, apiopenapi.LoginResponse{
		Data: apiopenapi.SessionData{
			CsrfToken: loginResult.Session.CSRFToken,
			ExpiresAt: loginResult.Session.ExpiresAt,
			User:      toAPIUser(loginResult.User),
		},
		RequestId: requestID,
	})
}

func (s *Server) completePendingOAuthLoginBind(w http.ResponseWriter, r *http.Request, requestID string, pendingToken string, user userscontract.StoredUser, adoptDisplayName bool) {
	pending, err := s.runtime.auth.FindPendingOAuthSession(r.Context(), pendingToken)
	if err != nil {
		s.clearOAuthPendingCookie(w)
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "pending oauth session invalid", requestID)
		return
	}
	if pending.Intent != authcontract.PendingOAuthIntentLogin {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "unsupported pending oauth intent", requestID)
		return
	}
	if pending.TargetUserID != nil && *pending.TargetUserID != user.ID {
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "pending oauth session targets another user", requestID)
		return
	}

	before, _ := s.runtime.users.ListAuthIdentities(r.Context(), user.ID)
	identities, err := s.bindPendingOAuthIdentityForUser(r.Context(), user.ID, pending)
	if err != nil {
		writeUserServiceError(w, err, requestID)
		return
	}
	if _, err := s.runtime.auth.ConsumePendingOAuthSession(r.Context(), pendingToken); err != nil {
		s.clearOAuthPendingCookie(w)
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "pending oauth session invalid", requestID)
		return
	}
	updatedUser, displayNameAdopted := s.adoptPendingOAuthDisplayName(r.Context(), user.ID, user.Name, pending, adoptDisplayName)
	if displayNameAdopted {
		user = updatedUser
	}
	loginResult, err := s.runtime.auth.CreateSessionForUser(r.Context(), user)
	if err != nil {
		s.clearOAuthPendingCookie(w)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to create session", requestID)
		return
	}
	s.setSessionCookie(w, loginResult)
	s.clearOAuthPendingCookie(w)
	after := authIdentityAuditSnapshot(identities)
	after["display_name_adopted"] = displayNameAdopted
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, user.ID, "user.auth_identity_bind_oauth_login", "user_auth_identities", strconv.Itoa(user.ID), authIdentityAuditSnapshot(before), after))
	writeJSONAny(w, http.StatusOK, apiopenapi.LoginResponse{
		Data: apiopenapi.SessionData{
			CsrfToken: loginResult.Session.CSRFToken,
			ExpiresAt: loginResult.Session.ExpiresAt,
			User:      toAPIUser(loginResult.User),
		},
		RequestId: requestID,
	})
}

// adoptPendingOAuthDisplayName best-effort applies the provider-returned display
// name to the user's own profile when the caller explicitly opted in. It is
// deliberately scoped to the display name only: provider avatar URLs are never
// adopted because SRapi avatars use a controlled upload/storage model and
// fetching a remote URL would add SSRF and privacy exposure. Adoption is
// skipped (never an error) when the provider supplied no name, the name exceeds
// the self-service profile limit, the name is unchanged, or the profile update
// fails, so a successful identity bind is never undone by an adoption preference.
func (s *Server) adoptPendingOAuthDisplayName(ctx context.Context, userID int, currentName string, pending authcontract.PendingOAuthSession, adopt bool) (userscontract.StoredUser, bool) {
	if !adopt {
		return userscontract.StoredUser{}, false
	}
	name := strings.TrimSpace(pending.DisplayName)
	if name == "" || utf8.RuneCountInString(name) > pendingOAuthAdoptDisplayNameMaxRunes || name == strings.TrimSpace(currentName) {
		return userscontract.StoredUser{}, false
	}
	updated, err := s.runtime.users.UpdateProfile(ctx, userID, usersservice.UpdateProfileRequest{Name: name})
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("pending oauth display name adoption skipped", "user_id", userID, "error", err)
		}
		return userscontract.StoredUser{}, false
	}
	return updated, true
}

func (s *Server) bindPendingOAuthIdentityForUser(ctx context.Context, userID int, pending authcontract.PendingOAuthSession) ([]userscontract.UserAuthIdentity, error) {
	return s.runtime.users.BindAuthIdentity(ctx, usersservice.BindAuthIdentityRequest{
		UserID:              userID,
		Provider:            pending.Provider,
		ProviderKey:         pending.ProviderKey,
		ProviderSubjectHash: pending.ProviderSubjectHash,
		SubjectHint:         pending.SubjectHint,
		DisplayName:         pending.DisplayName,
		Email:               pending.ResolvedEmail,
		EmailVerified:       pending.EmailVerified,
		AvatarURL:           pending.AvatarURL,
	})
}

func (s *Server) rollbackPendingOAuthCreatedUser(ctx context.Context, userID int) {
	if userID <= 0 {
		return
	}
	if err := s.runtime.users.Delete(ctx, userID); err != nil && s.logger != nil {
		s.logger.Warn("pending oauth created user rollback failed", "user_id", userID, "error", err)
	}
}

func (s *Server) writePendingOAuthBindLoginError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, authservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid pending oauth bind-login request", requestID)
	case errors.Is(err, authservice.ErrPendingOAuthInvalid):
		s.clearOAuthPendingCookie(w)
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "pending oauth session invalid", requestID)
	case errors.Is(err, authservice.ErrSecondFactorInvalid):
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "invalid two-factor code", requestID)
	case errors.Is(err, authservice.ErrPendingOAuthTargetMismatch):
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "pending oauth session targets another user", requestID)
	case errors.Is(err, authservice.ErrSessionUserUnavailable):
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "user disabled", requestID)
	case errors.Is(err, usersservice.ErrInvalidCredentials):
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "invalid credentials", requestID)
	case errors.Is(err, usersservice.ErrUserDisabled):
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "user disabled", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to bind pending oauth login", requestID)
	}
}

func (s *Server) writePendingOAuthCreateAccountError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, authservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid pending oauth create-account request", requestID)
	case errors.Is(err, authservice.ErrPendingOAuthInvalid):
		s.clearOAuthPendingCookie(w)
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "pending oauth session invalid", requestID)
	case errors.Is(err, authservice.ErrCSRFTokenInvalid):
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid pending oauth action token", requestID)
	case errors.Is(err, usersservice.ErrInvalidInput), errors.Is(err, usersservice.ErrUserAlreadyExists):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid pending oauth create-account request", requestID)
	case errors.Is(err, usersservice.ErrIdentityAlreadyBound):
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "pending oauth identity already bound", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to create pending oauth account", requestID)
	}
}

func (s *Server) writePendingOAuthEmailCompletionError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, authservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid pending oauth email completion request", requestID)
	case errors.Is(err, authservice.ErrPendingOAuthInvalid):
		s.clearOAuthPendingCookie(w)
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "pending oauth session invalid", requestID)
	case errors.Is(err, authservice.ErrPendingOAuthEmailInvalid):
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "invalid pending oauth email completion token", requestID)
	case errors.Is(err, authservice.ErrPendingOAuthUnavailable):
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "pending oauth email completion unavailable", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to complete pending oauth email", requestID)
	}
}

func (s *Server) clearOAuthFlowCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     oauthFlowCookieName,
		Value:    "",
		Path:     oauthFlowCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.Server.Mode != "local",
	})
}

func (s *Server) clearOAuthPendingCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     oauthPendingCookieName,
		Value:    "",
		Path:     oauthPendingCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.Server.Mode != "local",
	})
}

func (s *Server) toOAuthPendingSession(ctx context.Context, session authcontract.PendingOAuthSession) (apiopenapi.OAuthPendingSession, error) {
	nextStep, existingAccountBindable, err := s.oauthPendingNextStep(ctx, session)
	if err != nil {
		return apiopenapi.OAuthPendingSession{}, err
	}
	return apiopenapi.OAuthPendingSession{
		ExistingAccountBindable: existingAccountBindable,
		ExpiresAt:               session.ExpiresAt,
		Intent:                  apiopenapi.PendingOAuthIntent(session.Intent),
		NextStep:                nextStep,
		Profile: apiopenapi.OAuthPendingProfile{
			AvatarUrl:     session.AvatarURL,
			DisplayName:   session.DisplayName,
			EmailVerified: session.EmailVerified,
			ResolvedEmail: session.ResolvedEmail,
		},
		Provider:    apiopenapi.AuthIdentityProvider(session.Provider),
		ProviderKey: session.ProviderKey,
		Redirect:    session.RedirectTo,
		SubjectHint: session.SubjectHint,
	}, nil
}

func (s *Server) oauthPendingNextStep(ctx context.Context, session authcontract.PendingOAuthSession) (apiopenapi.OAuthPendingNextStep, bool, error) {
	if session.Intent == authcontract.PendingOAuthIntentBindCurrentUser {
		return apiopenapi.BindCurrentUserRequired, false, nil
	}
	if session.TargetUserID != nil {
		return apiopenapi.ReadyForLogin, false, nil
	}
	email := strings.ToLower(strings.TrimSpace(session.ResolvedEmail))
	if email == "" {
		return apiopenapi.EmailCompletionRequired, false, nil
	}
	user, err := s.runtime.users.FindByEmail(ctx, email)
	if err == nil {
		return apiopenapi.BindExistingLoginRequired, user.Status == userscontract.StatusActive, nil
	}
	if errors.Is(err, usersservice.ErrUserNotFound) {
		return apiopenapi.CreateAccountRequired, false, nil
	}
	return "", false, err
}

func validOAuthStartProvider(provider userscontract.AuthIdentityProvider) bool {
	switch provider {
	case userscontract.AuthIdentityProviderOIDC,
		userscontract.AuthIdentityProviderGitHub,
		userscontract.AuthIdentityProviderGoogle,
		userscontract.AuthIdentityProviderLinuxDo,
		userscontract.AuthIdentityProviderWeChat,
		userscontract.AuthIdentityProviderDingTalk:
		return true
	default:
		return false
	}
}

func selectOAuthProviderConfig(settings admincontrolcontract.AdminSettingsSecurity, provider userscontract.AuthIdentityProvider, providerKey string) (admincontrolcontract.OAuthProviderConfig, bool) {
	providerKey = strings.TrimSpace(providerKey)
	if !oauthProviderEnabled(settings.OAuthProviders, provider) {
		return admincontrolcontract.OAuthProviderConfig{}, false
	}
	for _, config := range settings.OAuthProviderConfigs {
		if userscontract.AuthIdentityProvider(config.Provider) != provider {
			continue
		}
		if providerKey != "" && config.ProviderKey != providerKey {
			continue
		}
		return config, true
	}
	return admincontrolcontract.OAuthProviderConfig{}, false
}

func oauthProviderEnabled(enabled []string, provider userscontract.AuthIdentityProvider) bool {
	if len(enabled) == 0 {
		return true
	}
	for _, value := range enabled {
		if strings.EqualFold(strings.TrimSpace(value), string(provider)) {
			return true
		}
	}
	return false
}

func callbackOAuthProviderConfigReady(config admincontrolcontract.OAuthProviderConfig) bool {
	if strings.ToLower(strings.TrimSpace(config.TokenAuthMethod)) != oauthTokenAuthMethodNone {
		return false
	}
	return validOAuthBackchannelURL(config.TokenURL) && validOAuthBackchannelURL(config.UserInfoURL)
}

// oauthClientSecret returns the confidential-client secret for a console OAuth
// provider key, sourced from deployment env (never AdminSettings). Empty means a
// public client.
func (rt *runtimeState) oauthClientSecret(providerKey string) string {
	if rt == nil || rt.cfg.OAuth.ClientSecrets == nil {
		return ""
	}
	return rt.cfg.OAuth.ClientSecrets[strings.TrimSpace(providerKey)]
}

// oauthIssuer returns the configured OIDC issuer for a console OAuth provider
// key. When set, the provider's id_token is verified at callback. Empty disables
// id_token verification (the access_token + userinfo flow still authenticates).
func (rt *runtimeState) oauthIssuer(providerKey string) string {
	if rt == nil || rt.cfg.OAuth.Issuers == nil {
		return ""
	}
	return strings.TrimSpace(rt.cfg.OAuth.Issuers[strings.TrimSpace(providerKey)])
}

func exchangeOAuthAuthorizationCode(ctx context.Context, client *http.Client, config admincontrolcontract.OAuthProviderConfig, flow authservice.OAuthAuthorizationFlowState, code string, clientSecret string) (accessToken string, idToken string, err error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", strings.TrimSpace(code))
	form.Set("redirect_uri", flow.RedirectURI)
	form.Set("client_id", flow.ClientID)
	form.Set("code_verifier", flow.CodeVerifier)
	// Confidential clients additionally authenticate with a client_secret
	// (client_secret_post). PKCE and the secret coexist; public clients omit it.
	if secret := strings.TrimSpace(clientSecret); secret != "" {
		form.Set("client_secret", secret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSpace(config.TokenURL), strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return "", "", fmt.Errorf("oauth token endpoint status %d", resp.StatusCode)
	}
	var body struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
		TokenType   string `json:"token_type"`
		Error       string `json:"error"`
	}
	if err := decodeOAuthProviderJSON(resp.Body, &body); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(body.Error) != "" || strings.TrimSpace(body.AccessToken) == "" {
		return "", "", errors.New("oauth token endpoint returned no access token")
	}
	if body.TokenType != "" && !strings.EqualFold(strings.TrimSpace(body.TokenType), "bearer") {
		return "", "", errors.New("oauth token endpoint returned unsupported token type")
	}
	return strings.TrimSpace(body.AccessToken), strings.TrimSpace(body.IDToken), nil
}

func fetchOAuthUserInfo(ctx context.Context, client *http.Client, userInfoURL string, accessToken string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(userInfoURL), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("oauth userinfo endpoint status %d", resp.StatusCode)
	}
	var body map[string]any
	if err := decodeOAuthProviderJSON(resp.Body, &body); err != nil {
		return nil, err
	}
	return body, nil
}

func decodeOAuthProviderJSON(body io.Reader, dst any) error {
	decoder := json.NewDecoder(io.LimitReader(body, oauthProviderBodyLimit))
	decoder.UseNumber()
	return decoder.Decode(dst)
}

func pendingOAuthProfileFromUserInfo(values map[string]any) (string, authservice.PendingOAuthProfile, error) {
	subject := firstOAuthString(values, "sub", "id", "openid", "unionid")
	if subject == "" || len(subject) > 512 || !utf8.ValidString(subject) || strings.ContainsAny(subject, "\r\n\t") {
		return "", authservice.PendingOAuthProfile{}, errors.New("oauth profile missing subject")
	}
	email := firstOAuthString(values, "email", "mail")
	if !strings.Contains(email, "@") || strings.ContainsAny(email, "\r\n\t ") {
		email = ""
	}
	profile := authservice.PendingOAuthProfile{
		ResolvedEmail: truncateOAuthString(email, 320),
		DisplayName:   truncateOAuthString(firstOAuthString(values, "name", "display_name", "login", "username", "nickname"), 160),
		EmailVerified: oauthBoolValue(values["email_verified"]),
		AvatarURL:     safeOAuthProfileURL(firstOAuthString(values, "picture", "avatar_url", "avatar", "headimgurl")),
	}
	return subject, profile, nil
}

func firstOAuthString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		value := oauthStringValue(values[key])
		if value != "" {
			return value
		}
	}
	return ""
}

func oauthStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return strings.TrimSpace(typed.String())
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	default:
		return ""
	}
}

func oauthBoolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return err == nil && parsed
	case json.Number:
		return typed.String() == "1"
	default:
		return false
	}
}

func safeOAuthProfileURL(value string) string {
	value = truncateOAuthString(value, 2048)
	if value == "" || strings.ContainsAny(value, "\r\n\t ") {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ""
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return ""
	}
	return value
}

func truncateOAuthString(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) || maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}

func oauthSubjectHint(provider userscontract.AuthIdentityProvider, subjectHash string) string {
	subjectHash = strings.TrimSpace(subjectHash)
	if len(subjectHash) > 12 {
		subjectHash = subjectHash[:12]
	}
	return string(provider) + ":" + subjectHash
}

func validOAuthBackchannelURL(value string) bool {
	parsed, ok := parseOAuthURL(value)
	if !ok {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	return parsed.Scheme == "http" && localOAuthHost(parsed.Hostname())
}

func parseOAuthURL(value string) (*url.URL, bool) {
	if value == "" || strings.ContainsAny(value, "\r\n\t ") {
		return nil, false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, false
	}
	return parsed, true
}

func localOAuthHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}
