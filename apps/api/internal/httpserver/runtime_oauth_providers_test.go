package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// listOAuthProviders calls the public endpoint and decodes the response.
func listOAuthProviders(t *testing.T, handler http.Handler) apiopenapi.EnabledOAuthProviderListResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/providers", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from oauth providers, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.EnabledOAuthProviderListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode oauth providers: %v", err)
	}
	// Defensive: a public, unauthenticated endpoint must never serialize secrets.
	if body := rec.Body.String(); strings.Contains(body, "client_secret") ||
		strings.Contains(body, "token_url") || strings.Contains(body, "client-123") {
		t.Fatalf("oauth providers response leaked secret/url material: %s", body)
	}
	return resp
}

func TestListOAuthProvidersReturnsEnabledStartableOnly(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	settingsResp := mustGetAdminSettings(t, handler, sessionCookie)
	settingsResp.Data.Security.OauthEnabled = true
	// Only oidc is enabled; the (fully valid) github config must be excluded by
	// the enabled gate. Persisted configs are always complete — admin-settings
	// validation rejects incomplete ones — so the startable gate is defensive.
	settingsResp.Data.Security.OauthProviders = []string{"oidc"}
	settingsResp.Data.Security.OauthProviderConfigs = []apiopenapi.OAuthProviderConfig{
		{
			Provider:     apiopenapi.Oidc,
			ProviderKey:  "issuer-main",
			DisplayName:  "Company SSO",
			ClientId:     "client-123",
			AuthorizeUrl: "https://idp.example/authorize",
			RedirectUri:  "http://localhost:8080/api/v1/auth/oauth/oidc/callback",
			Scopes:       []string{"openid", "email"},
		},
		{
			Provider:     apiopenapi.Github,
			ProviderKey:  "gh",
			DisplayName:  "GitHub",
			ClientId:     "gh-client",
			AuthorizeUrl: "https://github.com/login/oauth/authorize",
			RedirectUri:  "http://localhost:8080/api/v1/auth/oauth/github/callback",
		},
	}
	mustUpdateOAuthSettings(t, handler, sessionCookie, loginResp.Data.CsrfToken, settingsResp.Data)

	resp := listOAuthProviders(t, handler)
	if len(resp.Data) != 1 {
		t.Fatalf("expected exactly 1 enabled provider (github excluded), got %d: %+v", len(resp.Data), resp.Data)
	}
	got := resp.Data[0]
	if got.Provider != apiopenapi.Oidc || got.ProviderKey != "issuer-main" || got.DisplayName != "Company SSO" {
		t.Fatalf("unexpected provider entry: %+v", got)
	}
}

func TestListOAuthProvidersEmptyWhenDisabled(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	settingsResp := mustGetAdminSettings(t, handler, sessionCookie)
	settingsResp.Data.Security.OauthEnabled = false
	settingsResp.Data.Security.OauthProviders = []string{"oidc"}
	settingsResp.Data.Security.OauthProviderConfigs = []apiopenapi.OAuthProviderConfig{
		{
			Provider:     apiopenapi.Oidc,
			ProviderKey:  "issuer-main",
			DisplayName:  "Company SSO",
			ClientId:     "client-123",
			AuthorizeUrl: "https://idp.example/authorize",
			RedirectUri:  "http://localhost:8080/api/v1/auth/oauth/oidc/callback",
		},
	}
	mustUpdateOAuthSettings(t, handler, sessionCookie, loginResp.Data.CsrfToken, settingsResp.Data)

	resp := listOAuthProviders(t, handler)
	if len(resp.Data) != 0 {
		t.Fatalf("expected empty list when oauth disabled globally, got %+v", resp.Data)
	}
}
