package httpserver

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	provisioningcontract "github.com/srapi/srapi/apps/api/internal/modules/account_provisioning/contract"
	provisioningservice "github.com/srapi/srapi/apps/api/internal/modules/account_provisioning/service"
	authcontract "github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// accountOAuthProvisionTimeout bounds a single provider round-trip during a
// provisioning step so a slow upstream cannot pin a request goroutine.
const accountOAuthProvisionTimeout = 20 * time.Second

// registerAdminAccountOAuthRoutes wires the interactive upstream-account OAuth
// provisioning surface that replaces hand-pasting access_token/refresh_token:
// an authorization-code (PKCE) flow and an RFC 8628 device-code flow. All
// routes are admin-gated + CSRF-protected (read-only status excepted) and the
// minted tokens are returned write-only to feed POST /admin/accounts.
func (s *Server) registerAdminAccountOAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/admin/accounts/oauth/authorize-url", s.handleStartAdminAccountOAuthAuthorizeURL)
	mux.HandleFunc("POST /api/v1/admin/accounts/oauth/exchange", s.handleExchangeAdminAccountOAuthCode)
	mux.HandleFunc("POST /api/v1/admin/accounts/oauth/device-code/start", s.handleStartAdminAccountOAuthDeviceCode)
	mux.HandleFunc("POST /api/v1/admin/accounts/oauth/device-code/poll", s.handlePollAdminAccountOAuthDeviceCode)
	mux.HandleFunc("GET /api/v1/admin/accounts/oauth/pending/{id}", s.handleGetAdminAccountOAuthPending)
}

func (s *Server) handleStartAdminAccountOAuthAuthorizeURL(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, ok := s.requireAdminWriteSession(w, r, requestID)
	if !ok {
		return
	}
	provisioning := s.runtime.accountProvisioning
	if provisioning == nil {
		writeStandardError(w, http.StatusServiceUnavailable, apiopenapi.INTERNALERROR, "account oauth provisioning unavailable", requestID)
		return
	}
	var body apiopenapi.AccountOAuthAuthorizeUrlRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid oauth provisioning request", requestID)
		return
	}
	result, err := provisioning.StartAuthorizationURL(accountOAuthConfigFromAPI(body.Config))
	if err != nil {
		s.writeAccountOAuthProvisioningError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.oauth_provision_start", "provider_account_oauth", result.SessionID, nil, map[string]any{
		"mode":      string(provisioningcontract.ModeAuthorizationCode),
		"client_id": strings.TrimSpace(body.Config.ClientId),
	}))
	writeJSONAny(w, http.StatusCreated, apiopenapi.AccountOAuthAuthorizeUrlResponse{
		Data: apiopenapi.AccountOAuthAuthorizeUrl{
			SessionId:        result.SessionID,
			AuthorizationUrl: result.AuthorizationURL,
			State:            result.State,
			ExpiresAt:        result.ExpiresAt,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleExchangeAdminAccountOAuthCode(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, ok := s.requireAdminWriteSession(w, r, requestID)
	if !ok {
		return
	}
	provisioning := s.runtime.accountProvisioning
	if provisioning == nil {
		writeStandardError(w, http.StatusServiceUnavailable, apiopenapi.INTERNALERROR, "account oauth provisioning unavailable", requestID)
		return
	}
	var body apiopenapi.AccountOAuthExchangeRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid oauth exchange request", requestID)
		return
	}
	code, state, err := normalizeAccountOAuthExchangeInput(derefString(body.Code), derefString(body.CallbackUrl), body.State)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid oauth callback", requestID)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), accountOAuthProvisionTimeout)
	defer cancel()
	credential, err := provisioning.ExchangeCode(ctx, body.SessionId, code, state)
	if err != nil {
		s.writeAccountOAuthProvisioningError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.oauth_provision_exchange", "provider_account_oauth", strings.TrimSpace(body.SessionId), nil, map[string]any{
		"mode":              string(provisioningcontract.ModeAuthorizationCode),
		"has_refresh_token": credential.RefreshToken != "",
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountOAuthCredentialResponse{
		Data:      accountOAuthCredentialToAPI(body.SessionId, credential),
		RequestId: requestID,
	})
}

func normalizeAccountOAuthExchangeInput(codeValue, callbackURLValue, stateValue string) (string, string, error) {
	state := strings.TrimSpace(stateValue)
	callbackURL := strings.TrimSpace(callbackURLValue)
	code := strings.TrimSpace(codeValue)
	if callbackURL == "" && looksLikeAccountOAuthCallbackURL(code) {
		callbackURL = code
		code = ""
	}
	if callbackURL != "" {
		parsed, err := url.Parse(callbackURL)
		if err != nil || parsed.RawQuery == "" {
			return "", "", provisioningservice.ErrInvalidInput
		}
		query := parsed.Query()
		if providerErr := strings.TrimSpace(query.Get("error")); providerErr != "" {
			return "", "", provisioningservice.ErrInvalidInput
		}
		code = strings.TrimSpace(query.Get("code"))
		if callbackState := strings.TrimSpace(query.Get("state")); callbackState != "" {
			state = callbackState
		}
	}
	if code == "" || state == "" {
		return "", "", provisioningservice.ErrInvalidInput
	}
	return code, state, nil
}

func looksLikeAccountOAuthCallbackURL(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return false
	}
	return parsed.Scheme != "" && parsed.Host != ""
}

func (s *Server) handleStartAdminAccountOAuthDeviceCode(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, ok := s.requireAdminWriteSession(w, r, requestID)
	if !ok {
		return
	}
	provisioning := s.runtime.accountProvisioning
	if provisioning == nil {
		writeStandardError(w, http.StatusServiceUnavailable, apiopenapi.INTERNALERROR, "account oauth provisioning unavailable", requestID)
		return
	}
	var body apiopenapi.AccountOAuthDeviceCodeRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid oauth device-code request", requestID)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), accountOAuthProvisionTimeout)
	defer cancel()
	result, err := provisioning.StartDeviceCode(ctx, accountOAuthConfigFromAPI(body.Config))
	if err != nil {
		s.writeAccountOAuthProvisioningError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.oauth_provision_device_start", "provider_account_oauth", result.SessionID, nil, map[string]any{
		"mode":      string(provisioningcontract.ModeDeviceCode),
		"client_id": strings.TrimSpace(body.Config.ClientId),
	}))
	data := apiopenapi.AccountOAuthDeviceCode{
		SessionId:       result.SessionID,
		UserCode:        result.UserCode,
		VerificationUri: result.VerificationURI,
		Interval:        result.IntervalSecs,
		ExpiresAt:       result.ExpiresAt,
	}
	if complete := strings.TrimSpace(result.VerificationURIComplete); complete != "" {
		data.VerificationUriComplete = &complete
	}
	writeJSONAny(w, http.StatusCreated, apiopenapi.AccountOAuthDeviceCodeResponse{
		Data:      data,
		RequestId: requestID,
	})
}

func (s *Server) handlePollAdminAccountOAuthDeviceCode(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, ok := s.requireAdminWriteSession(w, r, requestID)
	if !ok {
		return
	}
	provisioning := s.runtime.accountProvisioning
	if provisioning == nil {
		writeStandardError(w, http.StatusServiceUnavailable, apiopenapi.INTERNALERROR, "account oauth provisioning unavailable", requestID)
		return
	}
	var body apiopenapi.AccountOAuthDevicePollRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid oauth device-code poll request", requestID)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), accountOAuthProvisionTimeout)
	defer cancel()
	credential, err := provisioning.PollDeviceCode(ctx, body.SessionId)
	if err != nil {
		if errors.Is(err, provisioningservice.ErrAuthorizationPending) || errors.Is(err, provisioningservice.ErrSlowDown) {
			writeJSONAny(w, http.StatusAccepted, apiopenapi.AccountOAuthPendingResponse{
				Data:      accountOAuthPendingStatusResponse(body.SessionId, provisioningcontract.ModeDeviceCode, provisioningcontract.StatusPending, ""),
				RequestId: requestID,
			})
			return
		}
		s.writeAccountOAuthProvisioningError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.oauth_provision_device_complete", "provider_account_oauth", strings.TrimSpace(body.SessionId), nil, map[string]any{
		"mode":              string(provisioningcontract.ModeDeviceCode),
		"has_refresh_token": credential.RefreshToken != "",
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountOAuthCredentialResponse{
		Data:      accountOAuthCredentialToAPI(body.SessionId, credential),
		RequestId: requestID,
	})
}

func (s *Server) handleGetAdminAccountOAuthPending(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	provisioning := s.runtime.accountProvisioning
	if provisioning == nil {
		writeStandardError(w, http.StatusServiceUnavailable, apiopenapi.INTERNALERROR, "account oauth provisioning unavailable", requestID)
		return
	}
	sessionID := strings.TrimSpace(r.PathValue("id"))
	if sessionID == "" {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid session id", requestID)
		return
	}
	pending, err := provisioning.Status(sessionID)
	if err != nil {
		s.writeAccountOAuthProvisioningError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountOAuthPendingResponse{
		Data:      accountOAuthPendingStatusResponse(pending.ID, pending.Mode, pending.Status, pending.FailureReason),
		RequestId: requestID,
	})
}

// requireAdminWriteSession enforces admin auth + CSRF in one place for the
// provisioning write endpoints.
func (s *Server) requireAdminWriteSession(w http.ResponseWriter, r *http.Request, requestID string) (authcontract.LoginResult, bool) {
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return authcontract.LoginResult{}, false
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return authcontract.LoginResult{}, false
	}
	return session, true
}

func accountOAuthConfigFromAPI(config apiopenapi.AccountOAuthProviderConfig) provisioningcontract.ProviderOAuthConfig {
	out := provisioningcontract.ProviderOAuthConfig{
		ClientID:           strings.TrimSpace(config.ClientId),
		ClientSecret:       derefString(config.ClientSecret),
		AuthorizeURL:       derefString(config.AuthorizeUrl),
		TokenURL:           derefString(config.TokenUrl),
		DeviceAuthorizeURL: derefString(config.DeviceAuthorizeUrl),
		RedirectURI:        derefString(config.RedirectUri),
		UsePKCE:            true,
	}
	if config.UsePkce != nil {
		out.UsePKCE = *config.UsePkce
	}
	if config.Scopes != nil {
		out.Scopes = append([]string(nil), *config.Scopes...)
	}
	return out
}

func accountOAuthCredentialToAPI(sessionID string, credential provisioningcontract.MintedCredential) apiopenapi.AccountOAuthCredential {
	credMap := credential.Credential()
	hasAccess := credential.AccessToken != ""
	hasRefresh := credential.RefreshToken != ""
	out := apiopenapi.AccountOAuthCredential{
		SessionId:       strings.TrimSpace(sessionID),
		Credential:      credMap,
		HasAccessToken:  &hasAccess,
		HasRefreshToken: &hasRefresh,
	}
	if credential.TokenType != "" {
		tokenType := credential.TokenType
		out.TokenType = &tokenType
	}
	if credential.Scope != "" {
		scope := credential.Scope
		out.Scope = &scope
	}
	if credential.ExpiresInSec > 0 {
		expiresIn := credential.ExpiresInSec
		out.ExpiresIn = &expiresIn
	}
	return out
}

func accountOAuthPendingStatusResponse(sessionID string, mode provisioningcontract.Mode, status provisioningcontract.Status, failureReason string) apiopenapi.AccountOAuthPending {
	out := apiopenapi.AccountOAuthPending{
		SessionId: strings.TrimSpace(sessionID),
		Mode:      apiopenapi.AccountOAuthPendingMode(mode),
		Status:    apiopenapi.AccountOAuthPendingStatus(status),
	}
	if reason := strings.TrimSpace(failureReason); reason != "" {
		out.FailureReason = &reason
	}
	return out
}

func (s *Server) writeAccountOAuthProvisioningError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, provisioningservice.ErrInvalidInput),
		errors.Is(err, provisioningservice.ErrWrongMode),
		errors.Is(err, provisioningservice.ErrStateMismatch):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid oauth provisioning request", requestID)
	case errors.Is(err, provisioningservice.ErrSessionNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "oauth provisioning session not found", requestID)
	case errors.Is(err, provisioningservice.ErrSessionExpired):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "oauth provisioning session expired", requestID)
	case errors.Is(err, provisioningservice.ErrProviderRejected):
		writeStandardError(w, http.StatusBadGateway, apiopenapi.PROVIDERAUTHFAILED, "oauth provider rejected authorization", requestID)
	case errors.Is(err, provisioningservice.ErrProviderUnavailable):
		writeStandardError(w, http.StatusBadGateway, apiopenapi.PROVIDERAUTHFAILED, "oauth provider unavailable", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to provision oauth account", requestID)
	}
}
