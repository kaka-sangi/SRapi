package httpserver

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
	platformcrypto "github.com/srapi/srapi/apps/api/internal/platform/crypto"
)

var oauthTokenAuthNone = apiopenapi.OAuthProviderConfigTokenAuthMethodNone

type oauthCallbackTestFlow struct {
	Handler          http.Handler
	PendingCookie    *http.Cookie
	AdminLogin       apiopenapi.LoginResponse
	AdminCookie      *http.Cookie
	TokenForm        url.Values
	UserInfoAuth     string
	CallbackBody     string
	CallbackCode     int
	CallbackLocation string
}

func TestOAuthStartRedirectsWithPKCEAndEncryptedFlowCookie(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	settingsResp := mustGetAdminSettings(t, handler, sessionCookie)
	settingsResp.Data.Security.OauthEnabled = true
	settingsResp.Data.Security.OauthProviders = []string{"oidc"}
	settingsResp.Data.Security.OauthProviderConfigs = []apiopenapi.OAuthProviderConfig{
		{
			Provider:     apiopenapi.AuthIdentityProviderOidc,
			ProviderKey:  "issuer-main",
			DisplayName:  "Issuer",
			ClientId:     "client-123",
			AuthorizeUrl: "https://idp.example/authorize",
			RedirectUri:  "http://localhost:8080/api/v1/auth/oauth/oidc/callback",
			Scopes:       []string{"openid", "email", "profile"},
		},
	}
	mustUpdateOAuthSettings(t, handler, sessionCookie, loginResp.Data.CsrfToken, settingsResp.Data)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/oidc/start?redirect=%2Fdashboard", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected oauth start 302, got %d body=%s", rec.Code, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse location: %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "idp.example" || parsed.Path != "/authorize" {
		t.Fatalf("unexpected oauth location: %s", location)
	}
	query := parsed.Query()
	for key, want := range map[string]string{
		"response_type":         "code",
		"client_id":             "client-123",
		"redirect_uri":          "http://localhost:8080/api/v1/auth/oauth/oidc/callback",
		"scope":                 "openid email profile",
		"code_challenge_method": "S256",
	} {
		if got := query.Get(key); got != want {
			t.Fatalf("query %s = %q, want %q in %s", key, got, want, location)
		}
	}
	if query.Get("state") == "" || query.Get("nonce") == "" || query.Get("code_challenge") == "" {
		t.Fatalf("expected state, nonce, and code challenge in %s", location)
	}
	cookie := oauthFlowCookieFromResponse(t, rec)
	if cookie.Path != oauthFlowCookiePath || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("unexpected oauth flow cookie attributes: %+v", cookie)
	}
	if strings.Contains(cookie.Value, query.Get("state")) || strings.Contains(cookie.Value, query.Get("nonce")) || strings.Contains(cookie.Value, "client-123") {
		t.Fatalf("flow cookie leaked oauth material: %q", cookie.Value)
	}
}

func TestOAuthCallbackExchangesCodeAndCreatesPendingCookie(t *testing.T) {
	var tokenForm url.Values
	var userInfoAuthorization string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected token method %s", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token form: %v", err)
			}
			tokenForm = r.PostForm
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"access-123","token_type":"Bearer"}`)
		case "/userinfo":
			userInfoAuthorization = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"sub":"subject-123","email":"User@Example.COM","email_verified":true,"name":"OAuth User","picture":"https://cdn.example/avatar.png"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	settingsResp := mustGetAdminSettings(t, handler, sessionCookie)
	settingsResp.Data.Security.OauthEnabled = true
	settingsResp.Data.Security.OauthProviders = []string{"oidc"}
	settingsResp.Data.Security.OauthProviderConfigs = []apiopenapi.OAuthProviderConfig{
		{
			Provider:        apiopenapi.AuthIdentityProviderOidc,
			ProviderKey:     "issuer-main",
			DisplayName:     "Issuer",
			ClientId:        "client-123",
			AuthorizeUrl:    "https://idp.example/authorize",
			TokenUrl:        stringPtrValueForAPI(upstream.URL + "/token"),
			UserinfoUrl:     stringPtrValueForAPI(upstream.URL + "/userinfo"),
			TokenAuthMethod: &oauthTokenAuthNone,
			RedirectUri:     "http://localhost:8080/api/v1/auth/oauth/oidc/callback",
			Scopes:          []string{"openid", "email", "profile"},
		},
	}
	mustUpdateOAuthSettings(t, handler, sessionCookie, loginResp.Data.CsrfToken, settingsResp.Data)

	startReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/oidc/start?redirect=%2Fdashboard", nil)
	startRec := httptest.NewRecorder()
	handler.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusFound {
		t.Fatalf("expected oauth start 302, got %d body=%s", startRec.Code, startRec.Body.String())
	}
	flowCookie := oauthFlowCookieFromResponse(t, startRec)
	startLocation, err := url.Parse(startRec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse start location: %v", err)
	}

	callbackReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/oidc/callback?code=callback-code&state="+url.QueryEscape(startLocation.Query().Get("state")), nil)
	callbackReq.AddCookie(flowCookie)
	callbackRec := httptest.NewRecorder()
	handler.ServeHTTP(callbackRec, callbackReq)
	if callbackRec.Code != http.StatusFound {
		t.Fatalf("expected oauth callback 302, got %d body=%s", callbackRec.Code, callbackRec.Body.String())
	}
	if got := callbackRec.Header().Get("Location"); got != "/dashboard" {
		t.Fatalf("expected callback redirect /dashboard, got %q", got)
	}
	for key, want := range map[string]string{
		"grant_type":    "authorization_code",
		"code":          "callback-code",
		"redirect_uri":  "http://localhost:8080/api/v1/auth/oauth/oidc/callback",
		"client_id":     "client-123",
		"code_verifier": "",
	} {
		got := tokenForm.Get(key)
		if key == "code_verifier" {
			if got == "" || strings.Contains(flowCookie.Value, got) {
				t.Fatalf("expected non-empty private code verifier, got %q", got)
			}
			continue
		}
		if got != want {
			t.Fatalf("token form %s = %q, want %q in %+v", key, got, want, tokenForm)
		}
	}
	if userInfoAuthorization != "Bearer access-123" {
		t.Fatalf("unexpected userinfo authorization header %q", userInfoAuthorization)
	}
	pendingCookie := oauthPendingCookieFromResponse(t, callbackRec)
	if pendingCookie.Path != oauthPendingCookiePath || !pendingCookie.HttpOnly || pendingCookie.SameSite != http.SameSiteLaxMode || pendingCookie.Value == "" {
		t.Fatalf("unexpected pending oauth cookie: %+v", pendingCookie)
	}
	if strings.Contains(callbackRec.Header().Get("Location"), pendingCookie.Value) {
		t.Fatalf("pending token leaked into redirect location")
	}
	pendingReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/pending", nil)
	pendingReq.AddCookie(pendingCookie)
	pendingRec := httptest.NewRecorder()
	handler.ServeHTTP(pendingRec, pendingReq)
	if pendingRec.Code != http.StatusOK {
		t.Fatalf("expected pending oauth preview 200, got %d body=%s", pendingRec.Code, pendingRec.Body.String())
	}
	pendingBody := pendingRec.Body.String()
	var pendingResp apiopenapi.OAuthPendingSessionResponse
	if err := json.NewDecoder(strings.NewReader(pendingBody)).Decode(&pendingResp); err != nil {
		t.Fatalf("decode pending oauth preview: %v", err)
	}
	if pendingResp.Data.NextStep != apiopenapi.CreateAccountRequired || pendingResp.Data.Profile.ResolvedEmail != "user@example.com" {
		t.Fatalf("unexpected pending oauth preview: %+v", pendingResp.Data)
	}
	if pendingResp.Data.SubjectHint == "" || strings.Contains(pendingBody, pendingCookie.Value) || strings.Contains(pendingBody, "subject-123") {
		t.Fatalf("pending oauth preview leaked sensitive material: %s", pendingBody)
	}
	secondPendingReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/pending", nil)
	secondPendingReq.AddCookie(pendingCookie)
	secondPendingRec := httptest.NewRecorder()
	handler.ServeHTTP(secondPendingRec, secondPendingReq)
	if secondPendingRec.Code != http.StatusOK {
		t.Fatalf("expected pending oauth preview to be read-only, got %d body=%s", secondPendingRec.Code, secondPendingRec.Body.String())
	}

	bindMissingCSRFReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/bind-current-user", nil)
	bindMissingCSRFReq.AddCookie(sessionCookie)
	bindMissingCSRFReq.AddCookie(pendingCookie)
	bindMissingCSRFRec := httptest.NewRecorder()
	handler.ServeHTTP(bindMissingCSRFRec, bindMissingCSRFReq)
	if bindMissingCSRFRec.Code != http.StatusForbidden {
		t.Fatalf("expected pending oauth bind without csrf 403, got %d body=%s", bindMissingCSRFRec.Code, bindMissingCSRFRec.Body.String())
	}

	bindReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/bind-current-user", nil)
	bindReq.AddCookie(sessionCookie)
	bindReq.AddCookie(pendingCookie)
	bindReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	bindRec := httptest.NewRecorder()
	handler.ServeHTTP(bindRec, bindReq)
	if bindRec.Code != http.StatusOK {
		t.Fatalf("expected pending oauth bind 200, got %d body=%s", bindRec.Code, bindRec.Body.String())
	}
	bindBody := bindRec.Body.String()
	var bindResp apiopenapi.CurrentUserAuthIdentityListResponse
	if err := json.NewDecoder(strings.NewReader(bindBody)).Decode(&bindResp); err != nil {
		t.Fatalf("decode pending oauth bind response: %v", err)
	}
	var external apiopenapi.CurrentUserAuthIdentity
	for _, identity := range bindResp.Data {
		if identity.Provider == apiopenapi.AuthIdentityProviderOidc && identity.External {
			external = identity
			break
		}
	}
	if external.Id == nil || external.UserId != loginResp.Data.User.Id || external.ProviderKey != "issuer-main" || external.Email == nil || *external.Email != "user@example.com" || !external.EmailVerified {
		t.Fatalf("expected bound oidc identity in response, got %+v from %+v", external, bindResp.Data)
	}
	if external.SubjectHint == nil || *external.SubjectHint == "" || strings.Contains(*external.SubjectHint, "subject-123") {
		t.Fatalf("expected safe subject hint, got %+v", external.SubjectHint)
	}
	if external.LastUsedAt == nil || external.VerifiedAt == nil {
		t.Fatalf("expected verified and last-used timestamps, got %+v", external)
	}
	if strings.Contains(bindBody, pendingCookie.Value) || strings.Contains(bindBody, "subject-123") {
		t.Fatalf("pending oauth bind response leaked sensitive material: %s", bindBody)
	}
	clearedPendingCookie := oauthNamedCookieFromResponse(t, bindRec, oauthPendingCookieName)
	if clearedPendingCookie.MaxAge != -1 {
		t.Fatalf("expected pending cookie to be cleared, got %+v", clearedPendingCookie)
	}
	consumedPendingReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/pending", nil)
	consumedPendingReq.AddCookie(pendingCookie)
	consumedPendingRec := httptest.NewRecorder()
	handler.ServeHTTP(consumedPendingRec, consumedPendingReq)
	if consumedPendingRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected consumed pending oauth preview 401, got %d body=%s", consumedPendingRec.Code, consumedPendingRec.Body.String())
	}

	missingPendingReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/pending", nil)
	missingPendingRec := httptest.NewRecorder()
	handler.ServeHTTP(missingPendingRec, missingPendingReq)
	if missingPendingRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing pending oauth cookie 401, got %d body=%s", missingPendingRec.Code, missingPendingRec.Body.String())
	}
	clearedFlowCookie := oauthNamedCookieFromResponse(t, callbackRec, oauthFlowCookieName)
	if clearedFlowCookie.MaxAge != -1 {
		t.Fatalf("expected flow cookie to be cleared, got %+v", clearedFlowCookie)
	}
}

func TestOAuthCallbackRejectsStateMismatch(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	settingsResp := mustGetAdminSettings(t, handler, sessionCookie)
	settingsResp.Data.Security.OauthEnabled = true
	settingsResp.Data.Security.OauthProviders = []string{"oidc"}
	settingsResp.Data.Security.OauthProviderConfigs = []apiopenapi.OAuthProviderConfig{
		{
			Provider:     apiopenapi.AuthIdentityProviderOidc,
			ProviderKey:  "issuer-main",
			DisplayName:  "Issuer",
			ClientId:     "client-123",
			AuthorizeUrl: "https://idp.example/authorize",
			RedirectUri:  "http://localhost:8080/api/v1/auth/oauth/oidc/callback",
			Scopes:       []string{"openid", "email", "profile"},
		},
	}
	mustUpdateOAuthSettings(t, handler, sessionCookie, loginResp.Data.CsrfToken, settingsResp.Data)

	startReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/oidc/start", nil)
	startRec := httptest.NewRecorder()
	handler.ServeHTTP(startRec, startReq)
	flowCookie := oauthFlowCookieFromResponse(t, startRec)

	callbackReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/oidc/callback?code=callback-code&state=wrong-state", nil)
	callbackReq.AddCookie(flowCookie)
	callbackRec := httptest.NewRecorder()
	handler.ServeHTTP(callbackRec, callbackReq)
	if callbackRec.Code != http.StatusBadRequest {
		t.Fatalf("expected state mismatch 400, got %d body=%s", callbackRec.Code, callbackRec.Body.String())
	}
	clearedFlowCookie := oauthNamedCookieFromResponse(t, callbackRec, oauthFlowCookieName)
	if clearedFlowCookie.MaxAge != -1 {
		t.Fatalf("expected flow cookie to be cleared, got %+v", clearedFlowCookie)
	}
}

func TestPendingOAuthBindLoginAuthenticatesExistingAccountAndConsumesPending(t *testing.T) {
	flow := newOAuthPendingCookieForTest(t, config.Load(), "bind-login-subject-123", "bind-login@srapi.local")
	registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"bind-login@srapi.local","name":"Bind Login","password":"password123"}`))
	registerReq.Header.Set("Content-Type", "application/json")
	registerRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(registerRec, registerReq)
	if registerRec.Code != http.StatusCreated {
		t.Fatalf("expected register 201, got %d body=%s", registerRec.Code, registerRec.Body.String())
	}

	bindReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/bind-login", strings.NewReader(`{"email":"bind-login@srapi.local","password":"password123"}`))
	bindReq.Header.Set("Content-Type", "application/json")
	bindReq.AddCookie(flow.PendingCookie)
	bindRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(bindRec, bindReq)
	if bindRec.Code != http.StatusOK {
		t.Fatalf("expected pending oauth bind-login 200, got %d body=%s", bindRec.Code, bindRec.Body.String())
	}
	if oauthNamedCookieFromResponse(t, bindRec, oauthPendingCookieName).MaxAge != -1 {
		t.Fatalf("expected pending cookie to be cleared")
	}
	var loginResp apiopenapi.LoginResponse
	if err := json.NewDecoder(bindRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode bind-login response: %v", err)
	}
	if loginResp.Data.User.Email != "bind-login@srapi.local" || loginResp.Data.CsrfToken == "" {
		t.Fatalf("unexpected bind-login response: %+v", loginResp.Data)
	}
	if loginResp.Data.User.Name != "Bind Login" {
		t.Fatalf("expected display name to stay %q without adopt opt-in, got %q", "Bind Login", loginResp.Data.User.Name)
	}
	if loginResp.Data.User.AvatarUrl != nil {
		t.Fatalf("expected no provider avatar adoption, got %v", *loginResp.Data.User.AvatarUrl)
	}
	sessionCookie := oauthNamedCookieFromResponse(t, bindRec, sessionCookieName)
	if sessionCookie.Value == "" || !sessionCookie.HttpOnly {
		t.Fatalf("expected session cookie, got %+v", sessionCookie)
	}
	bindBody := bindRec.Body.String()
	if strings.Contains(bindBody, flow.PendingCookie.Value) || strings.Contains(bindBody, "bind-login-subject-123") {
		t.Fatalf("bind-login response leaked sensitive material: %s", bindBody)
	}

	identitiesReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/auth-identities", nil)
	identitiesReq.AddCookie(sessionCookie)
	identitiesRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(identitiesRec, identitiesReq)
	if identitiesRec.Code != http.StatusOK {
		t.Fatalf("expected identities 200, got %d body=%s", identitiesRec.Code, identitiesRec.Body.String())
	}
	var identitiesResp apiopenapi.CurrentUserAuthIdentityListResponse
	if err := json.NewDecoder(identitiesRec.Body).Decode(&identitiesResp); err != nil {
		t.Fatalf("decode identities: %v", err)
	}
	var external apiopenapi.CurrentUserAuthIdentity
	for _, identity := range identitiesResp.Data {
		if identity.Provider == apiopenapi.AuthIdentityProviderOidc && identity.External {
			external = identity
			break
		}
	}
	if external.Id == nil || external.Email == nil || *external.Email != "bind-login@srapi.local" || external.SubjectHint == nil || strings.Contains(*external.SubjectHint, "bind-login-subject-123") {
		t.Fatalf("expected safe bound external identity, got %+v from %+v", external, identitiesResp.Data)
	}

	consumedReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/pending", nil)
	consumedReq.AddCookie(flow.PendingCookie)
	consumedRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(consumedRec, consumedReq)
	if consumedRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected consumed pending preview 401, got %d body=%s", consumedRec.Code, consumedRec.Body.String())
	}
}

func TestPendingOAuthBindLoginAdoptsProviderDisplayNameWhenRequested(t *testing.T) {
	flow := newOAuthPendingCookieForTest(t, config.Load(), "bind-login-adopt-subject", "adopt-login@srapi.local")
	registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"adopt-login@srapi.local","name":"Bind Login","password":"password123"}`))
	registerReq.Header.Set("Content-Type", "application/json")
	registerRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(registerRec, registerReq)
	if registerRec.Code != http.StatusCreated {
		t.Fatalf("expected register 201, got %d body=%s", registerRec.Code, registerRec.Body.String())
	}

	bindReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/bind-login", strings.NewReader(`{"email":"adopt-login@srapi.local","password":"password123","adopt_display_name":true}`))
	bindReq.Header.Set("Content-Type", "application/json")
	bindReq.AddCookie(flow.PendingCookie)
	bindRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(bindRec, bindReq)
	if bindRec.Code != http.StatusOK {
		t.Fatalf("expected pending oauth bind-login 200, got %d body=%s", bindRec.Code, bindRec.Body.String())
	}
	bindBody := bindRec.Body.String()
	var loginResp apiopenapi.LoginResponse
	if err := json.NewDecoder(strings.NewReader(bindBody)).Decode(&loginResp); err != nil {
		t.Fatalf("decode bind-login response: %v", err)
	}
	if loginResp.Data.User.Name != "OAuth User" {
		t.Fatalf("expected provider display name to be adopted, got %q", loginResp.Data.User.Name)
	}
	if loginResp.Data.User.AvatarUrl != nil {
		t.Fatalf("provider avatar URL must never be adopted, got %v", *loginResp.Data.User.AvatarUrl)
	}
	if strings.Contains(bindBody, "https://cdn.example/avatar.png") {
		t.Fatalf("provider avatar URL leaked into the adopted profile response: %s", bindBody)
	}
}

func TestPendingOAuthBindCurrentUserAdoptsProviderDisplayNameWhenRequested(t *testing.T) {
	flow := newOAuthPendingCookieForTest(t, config.Load(), "bind-current-adopt-subject", "bind-current-adopt@srapi.local")
	if flow.AdminLogin.Data.User.Name == "OAuth User" {
		t.Fatalf("precondition failed: admin is already named OAuth User")
	}

	bindReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/bind-current-user", strings.NewReader(`{"adopt_display_name":true}`))
	bindReq.Header.Set("Content-Type", "application/json")
	bindReq.AddCookie(flow.AdminCookie)
	bindReq.AddCookie(flow.PendingCookie)
	bindReq.Header.Set("X-CSRF-Token", flow.AdminLogin.Data.CsrfToken)
	bindRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(bindRec, bindReq)
	if bindRec.Code != http.StatusOK {
		t.Fatalf("expected pending oauth bind 200, got %d body=%s", bindRec.Code, bindRec.Body.String())
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	meReq.AddCookie(flow.AdminCookie)
	meRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("expected current user 200, got %d body=%s", meRec.Code, meRec.Body.String())
	}
	var meResp apiopenapi.UserResponse
	if err := json.NewDecoder(meRec.Body).Decode(&meResp); err != nil {
		t.Fatalf("decode current user: %v", err)
	}
	if meResp.Data.Name != "OAuth User" {
		t.Fatalf("expected provider display name to be adopted onto the current user, got %q", meResp.Data.Name)
	}
	if meResp.Data.AvatarUrl != nil {
		t.Fatalf("provider avatar URL must never be adopted, got %v", *meResp.Data.AvatarUrl)
	}
}

func TestPendingOAuthCreateAccountRequiresActionTokenAndBindsIdentity(t *testing.T) {
	flow := newOAuthPendingCookieForTest(t, config.Load(), "create-account-subject-123", "create-account@srapi.local")
	previewReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/pending", nil)
	previewReq.AddCookie(flow.PendingCookie)
	previewRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(previewRec, previewReq)
	if previewRec.Code != http.StatusOK {
		t.Fatalf("expected pending preview 200, got %d body=%s", previewRec.Code, previewRec.Body.String())
	}
	var preview apiopenapi.OAuthPendingSessionResponse
	if err := json.NewDecoder(previewRec.Body).Decode(&preview); err != nil {
		t.Fatalf("decode pending preview: %v", err)
	}
	if preview.Data.NextStep != apiopenapi.CreateAccountRequired || preview.Data.CreateAccountAction == nil || preview.Data.CreateAccountAction.Token == "" {
		t.Fatalf("expected create-account action token, got %+v", preview.Data)
	}
	if strings.Contains(preview.Data.CreateAccountAction.Token, flow.PendingCookie.Value) || strings.Contains(preview.Data.CreateAccountAction.Token, "create-account-subject-123") {
		t.Fatalf("action token leaked pending or provider material: %q", preview.Data.CreateAccountAction.Token)
	}

	missingTokenReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/create-account", strings.NewReader(`{"email":"create-account@srapi.local","password":"password123"}`))
	missingTokenReq.Header.Set("Content-Type", "application/json")
	missingTokenReq.AddCookie(flow.PendingCookie)
	missingTokenRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(missingTokenRec, missingTokenReq)
	if missingTokenRec.Code != http.StatusForbidden {
		t.Fatalf("expected missing action token 403, got %d body=%s", missingTokenRec.Code, missingTokenRec.Body.String())
	}

	createBody, err := json.Marshal(map[string]string{
		"email":        "create-account@srapi.local",
		"name":         "Created From OAuth",
		"password":     "password123",
		"action_token": preview.Data.CreateAccountAction.Token,
	})
	if err != nil {
		t.Fatalf("marshal create body: %v", err)
	}
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/create-account", bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(flow.PendingCookie)
	createRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create-account 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	if oauthNamedCookieFromResponse(t, createRec, oauthPendingCookieName).MaxAge != -1 {
		t.Fatalf("expected pending cookie to be cleared")
	}
	var loginResp apiopenapi.LoginResponse
	if err := json.NewDecoder(createRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode create-account response: %v", err)
	}
	if loginResp.Data.User.Email != "create-account@srapi.local" || loginResp.Data.User.Name != "Created From OAuth" || loginResp.Data.CsrfToken == "" {
		t.Fatalf("unexpected create-account response: %+v", loginResp.Data)
	}
	if loginResp.Data.User.EmailVerifiedAt == nil {
		t.Fatalf("expected verified provider email to mark local email verified")
	}
	sessionCookie := oauthNamedCookieFromResponse(t, createRec, sessionCookieName)
	if sessionCookie.Value == "" || !sessionCookie.HttpOnly {
		t.Fatalf("expected session cookie, got %+v", sessionCookie)
	}
	createRespBody := createRec.Body.String()
	if strings.Contains(createRespBody, flow.PendingCookie.Value) || strings.Contains(createRespBody, "create-account-subject-123") {
		t.Fatalf("create-account response leaked sensitive material: %s", createRespBody)
	}

	identitiesReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/auth-identities", nil)
	identitiesReq.AddCookie(sessionCookie)
	identitiesRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(identitiesRec, identitiesReq)
	if identitiesRec.Code != http.StatusOK {
		t.Fatalf("expected identities 200, got %d body=%s", identitiesRec.Code, identitiesRec.Body.String())
	}
	var identitiesResp apiopenapi.CurrentUserAuthIdentityListResponse
	if err := json.NewDecoder(identitiesRec.Body).Decode(&identitiesResp); err != nil {
		t.Fatalf("decode identities: %v", err)
	}
	var external apiopenapi.CurrentUserAuthIdentity
	for _, identity := range identitiesResp.Data {
		if identity.Provider == apiopenapi.AuthIdentityProviderOidc && identity.External {
			external = identity
			break
		}
	}
	if external.Id == nil || external.Email == nil || *external.Email != "create-account@srapi.local" || external.SubjectHint == nil || strings.Contains(*external.SubjectHint, "create-account-subject-123") {
		t.Fatalf("expected safe bound external identity, got %+v from %+v", external, identitiesResp.Data)
	}

	consumedReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/pending", nil)
	consumedReq.AddCookie(flow.PendingCookie)
	consumedRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(consumedRec, consumedReq)
	if consumedRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected consumed pending preview 401, got %d body=%s", consumedRec.Code, consumedRec.Body.String())
	}
}

func TestPendingOAuthEmailCompletionConfirmsEmaillessPendingSession(t *testing.T) {
	flow := newOAuthPendingCookieForTest(t, config.Load(), "email-completion-subject-123", "")
	previewReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/pending", nil)
	previewReq.AddCookie(flow.PendingCookie)
	previewRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(previewRec, previewReq)
	if previewRec.Code != http.StatusOK {
		t.Fatalf("expected pending preview 200, got %d body=%s", previewRec.Code, previewRec.Body.String())
	}
	var preview apiopenapi.OAuthPendingSessionResponse
	if err := json.NewDecoder(previewRec.Body).Decode(&preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if preview.Data.NextStep != apiopenapi.EmailCompletionRequired || preview.Data.Profile.ResolvedEmail != "" || preview.Data.CreateAccountAction != nil {
		t.Fatalf("expected email completion required preview, got %+v", preview.Data)
	}

	sendReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/send-verify-code", strings.NewReader(`{"email":"Complete@SRapi.Local"}`))
	sendReq.Header.Set("Content-Type", "application/json")
	sendReq.AddCookie(flow.PendingCookie)
	sendRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(sendRec, sendReq)
	if sendRec.Code != http.StatusAccepted {
		t.Fatalf("expected send verify code 202, got %d body=%s", sendRec.Code, sendRec.Body.String())
	}
	if strings.Contains(sendRec.Body.String(), "Complete@SRapi.Local") || strings.Contains(sendRec.Body.String(), flow.PendingCookie.Value) {
		t.Fatalf("send verify response leaked sensitive material: %s", sendRec.Body.String())
	}

	adminLogin, adminCookie := mustLoginAdmin(t, flow.Handler)
	_ = adminLogin
	outboxReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/events/outbox?event_type=PendingOAuthEmailCompletionRequested", nil)
	outboxReq.AddCookie(adminCookie)
	outboxRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(outboxRec, outboxReq)
	if outboxRec.Code != http.StatusOK {
		t.Fatalf("expected outbox list 200, got %d body=%s", outboxRec.Code, outboxRec.Body.String())
	}
	var outboxResp apiopenapi.DomainEventOutboxListResponse
	if err := json.NewDecoder(outboxRec.Body).Decode(&outboxResp); err != nil {
		t.Fatalf("decode outbox response: %v", err)
	}
	if len(outboxResp.Data) != 1 {
		t.Fatalf("expected one pending oauth email outbox event, got %+v", outboxResp.Data)
	}
	payload := outboxResp.Data[0].Payload
	tokenCiphertext, ok := payload["verification_token_ciphertext"].(string)
	if !ok || tokenCiphertext == "" {
		t.Fatalf("expected encrypted email completion token, got %+v", payload)
	}
	if strings.Contains(fmt.Sprint(payload), "Complete@SRapi.Local") || strings.Contains(fmt.Sprint(payload), flow.PendingCookie.Value) || strings.Contains(fmt.Sprint(payload), "email-completion-subject-123") {
		t.Fatalf("outbox payload leaked pending oauth material: %+v", payload)
	}
	rawToken := tokenCiphertext
	if payload := decryptPendingOAuthEmailCompletionTokenForTest(t, tokenCiphertext); !strings.Contains(payload, "complete@srapi.local") {
		t.Fatalf("unexpected pending oauth email completion token payload: %q", payload)
	}

	confirmReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/email-completion/confirm", strings.NewReader(`{"token":"`+rawToken+`"}`))
	confirmReq.Header.Set("Content-Type", "application/json")
	confirmReq.AddCookie(flow.PendingCookie)
	confirmRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(confirmRec, confirmReq)
	if confirmRec.Code != http.StatusOK {
		t.Fatalf("expected email completion confirm 200, got %d body=%s", confirmRec.Code, confirmRec.Body.String())
	}
	var completed apiopenapi.OAuthPendingSessionResponse
	if err := json.NewDecoder(confirmRec.Body).Decode(&completed); err != nil {
		t.Fatalf("decode completed pending preview: %v", err)
	}
	if completed.Data.Profile.ResolvedEmail != "complete@srapi.local" || !completed.Data.Profile.EmailVerified || completed.Data.NextStep != apiopenapi.CreateAccountRequired || completed.Data.CreateAccountAction == nil {
		t.Fatalf("unexpected completed pending preview: %+v", completed.Data)
	}
	if strings.Contains(confirmRec.Body.String(), flow.PendingCookie.Value) || strings.Contains(confirmRec.Body.String(), "email-completion-subject-123") {
		t.Fatalf("confirm response leaked sensitive material: %s", confirmRec.Body.String())
	}

	reuseReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/email-completion/confirm", strings.NewReader(`{"token":"`+rawToken+`"}`))
	reuseReq.Header.Set("Content-Type", "application/json")
	reuseReq.AddCookie(flow.PendingCookie)
	reuseRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(reuseRec, reuseReq)
	if reuseRec.Code != http.StatusBadRequest {
		t.Fatalf("expected repeated email completion confirm 400, got %d body=%s", reuseRec.Code, reuseRec.Body.String())
	}
}

func TestPendingOAuthBindLoginTwoFactorBindsChallengeToPendingCookie(t *testing.T) {
	cfg := config.Load()
	cfg.Security.TOTPEncryptionKey = "totp_http_encryption_key_32_bytes_min"
	flow := newOAuthPendingCookieForTest(t, cfg, "bind-login-2fa-subject-123", "admin@srapi.local")

	setupReq := httptest.NewRequest(http.MethodPost, "/api/v1/me/totp/setup", nil)
	setupReq.AddCookie(flow.AdminCookie)
	setupReq.Header.Set("X-CSRF-Token", flow.AdminLogin.Data.CsrfToken)
	setupRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(setupRec, setupReq)
	if setupRec.Code != http.StatusOK {
		t.Fatalf("expected setup 200, got %d body=%s", setupRec.Code, setupRec.Body.String())
	}
	var setupResp apiopenapi.TOTPSetupResponse
	if err := json.NewDecoder(setupRec.Body).Decode(&setupResp); err != nil {
		t.Fatalf("decode setup response: %v", err)
	}
	code, err := pendingOAuthTestTOTPCode(setupResp.Data.Secret)
	if err != nil {
		t.Fatalf("generate totp code: %v", err)
	}
	enableReq := httptest.NewRequest(http.MethodPost, "/api/v1/me/totp/enable", strings.NewReader(`{"code":"`+code+`"}`))
	enableReq.Header.Set("Content-Type", "application/json")
	enableReq.Header.Set("X-CSRF-Token", flow.AdminLogin.Data.CsrfToken)
	enableReq.AddCookie(flow.AdminCookie)
	enableRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(enableRec, enableReq)
	if enableRec.Code != http.StatusOK {
		t.Fatalf("expected enable 200, got %d body=%s", enableRec.Code, enableRec.Body.String())
	}

	bindReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/bind-login", strings.NewReader(`{"email":"admin@srapi.local","password":"password123","adopt_display_name":true}`))
	bindReq.Header.Set("Content-Type", "application/json")
	bindReq.AddCookie(flow.PendingCookie)
	bindRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(bindRec, bindReq)
	if bindRec.Code != http.StatusAccepted {
		t.Fatalf("expected pending oauth bind-login 202, got %d body=%s", bindRec.Code, bindRec.Body.String())
	}
	if len(bindRec.Result().Cookies()) != 0 {
		t.Fatalf("expected no cookies before pending oauth second factor")
	}
	var challengeResp apiopenapi.LoginTwoFactorRequiredResponse
	if err := json.NewDecoder(bindRec.Body).Decode(&challengeResp); err != nil {
		t.Fatalf("decode challenge response: %v", err)
	}
	if !bool(challengeResp.Data.Required) || challengeResp.Data.ChallengeId == "" {
		t.Fatalf("unexpected challenge response: %+v", challengeResp.Data)
	}
	stillPendingReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/pending", nil)
	stillPendingReq.AddCookie(flow.PendingCookie)
	stillPendingRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(stillPendingRec, stillPendingReq)
	if stillPendingRec.Code != http.StatusOK {
		t.Fatalf("expected pending session to remain before 2fa, got %d body=%s", stillPendingRec.Code, stillPendingRec.Body.String())
	}

	wrongCookieReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/bind-login/2fa", strings.NewReader(`{"challenge_id":"`+challengeResp.Data.ChallengeId+`","code":"123456"}`))
	wrongCookieReq.Header.Set("Content-Type", "application/json")
	wrongCookieReq.AddCookie(&http.Cookie{Name: oauthPendingCookieName, Value: "oauth_pending_wrong"})
	wrongCookieRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(wrongCookieRec, wrongCookieReq)
	if wrongCookieRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected wrong pending cookie challenge rejection 401, got %d body=%s", wrongCookieRec.Code, wrongCookieRec.Body.String())
	}

	code, err = pendingOAuthTestTOTPCode(setupResp.Data.Secret)
	if err != nil {
		t.Fatalf("generate final totp code: %v", err)
	}
	completeReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/bind-login/2fa", strings.NewReader(`{"challenge_id":"`+challengeResp.Data.ChallengeId+`","code":"`+code+`"}`))
	completeReq.Header.Set("Content-Type", "application/json")
	completeReq.AddCookie(flow.PendingCookie)
	completeRec := httptest.NewRecorder()
	flow.Handler.ServeHTTP(completeRec, completeReq)
	if completeRec.Code != http.StatusOK {
		t.Fatalf("expected pending oauth bind-login 2fa 200, got %d body=%s", completeRec.Code, completeRec.Body.String())
	}
	if oauthNamedCookieFromResponse(t, completeRec, oauthPendingCookieName).MaxAge != -1 {
		t.Fatalf("expected pending cookie to be cleared")
	}
	var loginResp apiopenapi.LoginResponse
	if err := json.NewDecoder(completeRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode final bind-login response: %v", err)
	}
	if loginResp.Data.User.Email != "admin@srapi.local" || loginResp.Data.CsrfToken == "" {
		t.Fatalf("unexpected final login response: %+v", loginResp.Data)
	}
	if loginResp.Data.User.Name != "OAuth User" {
		t.Fatalf("expected adopt-display-name to survive the 2fa challenge and update the profile, got %q", loginResp.Data.User.Name)
	}
}

func TestOAuthStartRejectsDisabledAndUnavailableProvider(t *testing.T) {
	handler := New(config.Load(), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/oidc/start", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected disabled oauth 403, got %d body=%s", rec.Code, rec.Body.String())
	}

	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	settingsResp := mustGetAdminSettings(t, handler, sessionCookie)
	settingsResp.Data.Security.OauthEnabled = true
	settingsResp.Data.Security.OauthProviders = []string{"github"}
	settingsResp.Data.Security.OauthProviderConfigs = []apiopenapi.OAuthProviderConfig{
		{
			Provider:     apiopenapi.AuthIdentityProviderGithub,
			ProviderKey:  "github",
			DisplayName:  "GitHub",
			ClientId:     "github-client",
			AuthorizeUrl: "https://github.example/login/oauth/authorize",
			RedirectUri:  "http://localhost:8080/api/v1/auth/oauth/github/callback",
			Scopes:       []string{"read:user", "user:email"},
		},
	}
	mustUpdateOAuthSettings(t, handler, sessionCookie, loginResp.Data.CsrfToken, settingsResp.Data)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/oidc/start", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected unavailable oauth provider 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func mustUpdateOAuthSettings(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, settings apiopenapi.AdminSettings) {
	t.Helper()
	body, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected settings update 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func newOAuthPendingCookieForTest(t *testing.T, cfg config.Config, subject string, email string) oauthCallbackTestFlow {
	t.Helper()
	var tokenForm url.Values
	var userInfoAuthorization string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token form: %v", err)
			}
			tokenForm = r.PostForm
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"access-123","token_type":"Bearer"}`)
		case "/userinfo":
			userInfoAuthorization = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"sub":"`+subject+`","email":"`+email+`","email_verified":true,"name":"OAuth User","picture":"https://cdn.example/avatar.png"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)

	handler := New(cfg, nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	settingsResp := mustGetAdminSettings(t, handler, sessionCookie)
	settingsResp.Data.Security.OauthEnabled = true
	settingsResp.Data.Security.OauthProviders = []string{"oidc"}
	settingsResp.Data.Security.OauthProviderConfigs = []apiopenapi.OAuthProviderConfig{
		{
			Provider:        apiopenapi.AuthIdentityProviderOidc,
			ProviderKey:     "issuer-main",
			DisplayName:     "Issuer",
			ClientId:        "client-123",
			AuthorizeUrl:    "https://idp.example/authorize",
			TokenUrl:        stringPtrValueForAPI(upstream.URL + "/token"),
			UserinfoUrl:     stringPtrValueForAPI(upstream.URL + "/userinfo"),
			TokenAuthMethod: &oauthTokenAuthNone,
			RedirectUri:     "http://localhost:8080/api/v1/auth/oauth/oidc/callback",
			Scopes:          []string{"openid", "email", "profile"},
		},
	}
	mustUpdateOAuthSettings(t, handler, sessionCookie, loginResp.Data.CsrfToken, settingsResp.Data)

	startReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/oidc/start?redirect=%2Fdashboard", nil)
	startRec := httptest.NewRecorder()
	handler.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusFound {
		t.Fatalf("expected oauth start 302, got %d body=%s", startRec.Code, startRec.Body.String())
	}
	flowCookie := oauthFlowCookieFromResponse(t, startRec)
	startLocation, err := url.Parse(startRec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse start location: %v", err)
	}
	callbackReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/oidc/callback?code=callback-code&state="+url.QueryEscape(startLocation.Query().Get("state")), nil)
	callbackReq.AddCookie(flowCookie)
	callbackRec := httptest.NewRecorder()
	handler.ServeHTTP(callbackRec, callbackReq)
	if callbackRec.Code != http.StatusFound {
		t.Fatalf("expected oauth callback 302, got %d body=%s", callbackRec.Code, callbackRec.Body.String())
	}
	return oauthCallbackTestFlow{
		Handler:          handler,
		PendingCookie:    oauthPendingCookieFromResponse(t, callbackRec),
		AdminLogin:       loginResp,
		AdminCookie:      sessionCookie,
		TokenForm:        tokenForm,
		UserInfoAuth:     userInfoAuthorization,
		CallbackBody:     callbackRec.Body.String(),
		CallbackCode:     callbackRec.Code,
		CallbackLocation: callbackRec.Header().Get("Location"),
	}
}

func pendingOAuthTestTOTPCode(secret string) (string, error) {
	return totp.GenerateCodeCustom(secret, time.Now().UTC(), totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
}

func decryptPendingOAuthEmailCompletionTokenForTest(t *testing.T, ciphertext string) string {
	t.Helper()
	parts := strings.Split(ciphertext, ":")
	if len(parts) != 3 || parts[0] != "v1" {
		t.Fatalf("unexpected pending oauth email completion ciphertext shape: %q", ciphertext)
	}
	key, err := platformcrypto.DeriveAESKey(config.Load().Security.MasterKey)
	if err != nil {
		t.Fatalf("derive pending oauth email completion key: %v", err)
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode pending oauth email completion nonce: %v", err)
	}
	rawCiphertext, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode pending oauth email completion ciphertext: %v", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("new aes: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("new gcm: %v", err)
	}
	plaintext, err := gcm.Open(nil, nonce, rawCiphertext, []byte("auth.pending_oauth_email_completion:v1"))
	if err != nil {
		t.Fatalf("decrypt pending oauth email completion token: %v", err)
	}
	return string(plaintext)
}

func oauthFlowCookieFromResponse(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	return oauthNamedCookieFromResponse(t, rec, oauthFlowCookieName)
}

func oauthPendingCookieFromResponse(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	return oauthNamedCookieFromResponse(t, rec, oauthPendingCookieName)
}

func oauthNamedCookieFromResponse(t *testing.T, rec *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("expected %s cookie in %+v", name, rec.Result().Cookies())
	return nil
}
