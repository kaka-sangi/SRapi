package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
)

// adminAccountMutation issues an authenticated admin account create/update and
// returns the raw recorder so callers can assert rejection status codes.
func adminAccountMutation(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// TestAdminAccountAuthMethodAllowlistScopesRuntimeClass verifies that the
// per-provider auth_methods allowlist (stored in config_schema) gates which
// runtime classes an account may use, while providers without an allowlist
// remain unrestricted (legacy / manually-created providers).
func TestAdminAccountAuthMethodAllowlistScopesRuntimeClass(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken

	// Provider with a narrow allowlist (mirrors a third-party OpenAI-compatible
	// preset): only api_key and custom_reverse_proxy are accepted.
	scoped := mustCreateProvider(t, handler, sessionCookie, csrf,
		`{"name":"deepseek-scoped","display_name":"DeepSeek Scoped","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","config_schema":{"auth_methods":["api_key","custom_reverse_proxy"]}}`)
	scopedID := string(scoped.Data.Id)

	// Allowed method succeeds.
	allowed := mustCreateAccount(t, handler, sessionCookie, csrf,
		`{"provider_id":"`+scopedID+`","name":"deepseek-apikey","runtime_class":"api_key","credential":{"api_key":"secret"},"status":"active"}`)

	// Disallowed method (oauth_refresh not in allowlist) is rejected with 400.
	rec := adminAccountMutation(t, handler, sessionCookie, csrf, http.MethodPost, "/api/v1/admin/accounts",
		`{"provider_id":"`+scopedID+`","name":"deepseek-oauth","runtime_class":"oauth_refresh","credential":{"access_token":"a","refresh_token":"b"},"status":"active"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 rejecting oauth_refresh on scoped provider, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "authentication method not allowed") {
		t.Fatalf("expected auth-method rejection message, got %s", rec.Body.String())
	}

	// Updating the allowed account to a disallowed method is also rejected.
	rec = adminAccountMutation(t, handler, sessionCookie, csrf, http.MethodPatch, "/api/v1/admin/accounts/"+string(allowed.Data.Id),
		`{"runtime_class":"web_session_cookie","credential":{"cookie":"x"}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 rejecting web_session_cookie update on scoped provider, got %d body=%s", rec.Code, rec.Body.String())
	}

	// Antigravity-style allowlist accepts desktop_client_token.
	antigravity := mustCreateProvider(t, handler, sessionCookie, csrf,
		`{"name":"antigravity-scoped","display_name":"Antigravity Scoped","adapter_type":"reverse-proxy-antigravity","protocol":"openai-compatible","status":"active","config_schema":{"auth_methods":["desktop_client_token","ide_plugin_token","oauth_refresh","custom_reverse_proxy"]}}`)
	mustCreateAccount(t, handler, sessionCookie, csrf,
		`{"provider_id":"`+string(antigravity.Data.Id)+`","name":"antigravity-desktop","runtime_class":"desktop_client_token","upstream_client":"antigravity_desktop","credential":{"access_token":"desktop-token"},"metadata":{"base_url":"https://example.test","project_id":"p1"},"status":"active"}`)
	// And rejects api_key, which is not in its allowlist.
	rec = adminAccountMutation(t, handler, sessionCookie, csrf, http.MethodPost, "/api/v1/admin/accounts",
		`{"provider_id":"`+string(antigravity.Data.Id)+`","name":"antigravity-apikey","runtime_class":"api_key","credential":{"api_key":"secret"},"status":"active"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 rejecting api_key on antigravity-scoped provider, got %d body=%s", rec.Code, rec.Body.String())
	}

	// Provider with NO allowlist (manual / legacy) accepts any runtime class.
	manual := mustCreateProvider(t, handler, sessionCookie, csrf,
		`{"name":"manual-open","display_name":"Manual Open","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, csrf,
		`{"provider_id":"`+string(manual.Data.Id)+`","name":"manual-cookie","runtime_class":"web_session_cookie","credential":{"cookie":"session=abc"},"status":"active"}`)
}
