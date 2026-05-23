package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

const codexOAuthClientIDForTest = "app_EMoamEEZ73f0CkXaXp7hrann"

func TestGatewayCodexRefreshTokenOnlyCreateCanRequestResponses(t *testing.T) {
	var tokenCalls int
	var responseCalls int
	var tokenForm url.Values
	var responseAuthorization string
	var responsePath string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			tokenCalls++
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token form: %v", err)
			}
			tokenForm = cloneURLValues(r.PostForm)
			if r.Method != http.MethodPost ||
				r.PostForm.Get("grant_type") != "refresh_token" ||
				r.PostForm.Get("refresh_token") != "create-refresh" ||
				r.PostForm.Get("client_id") != codexOAuthClientIDForTest ||
				r.PostForm.Get("scope") != "openid profile email" {
				t.Fatalf("unexpected codex refresh request: method=%s form=%v", r.Method, r.PostForm)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"create-access","refresh_token":"create-refresh-rotated","id_token":"id-token","token_type":"Bearer","expires_in":3600}`)
		case "/backend-api/codex/responses":
			responseCalls++
			responseAuthorization = r.Header.Get("Authorization")
			responsePath = r.URL.Path
			if r.Header.Get("Originator") != "codex_cli_rs" || r.Header.Get("User-Agent") != "codex-cli/test" {
				t.Fatalf("unexpected codex headers: %+v", r.Header)
			}
			var payload struct {
				Model  string `json:"model"`
				Stream bool   `json:"stream"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode codex responses payload: %v", err)
			}
			if payload.Model != "codex-upstream" || !payload.Stream {
				t.Fatalf("unexpected codex responses payload: %+v", payload)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"codex create ok\"}\n\ndata: [DONE]\n\n")
		default:
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"codex-refresh-create-provider","display_name":"Codex Refresh Create","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"codex-refresh-create-model","display_name":"Codex Refresh Create Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"codex-upstream","status":"active"}`)

	accountBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","name":"codex-refresh-create-account","runtime_class":"oauth_refresh","upstream_client":"codex_cli","credential":{"refresh_token":"create-refresh"},"metadata":{"base_url":"` + upstream.URL + `/backend-api/codex","oauth_token_url":"` + upstream.URL + `/oauth/token","user_agent":"codex-cli/test"},"status":"active"}`
	_, rawAccount := mustCreateAdminAccountRaw(t, handler, sessionCookie, loginResp.Data.CsrfToken, accountBody)
	if strings.Contains(rawAccount, "create-refresh") || strings.Contains(rawAccount, "create-access") {
		t.Fatalf("account create response leaked credential: %s", rawAccount)
	}

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/responses", `{"model":"codex-refresh-create-model","input":"hello codex"}`)
	if !strings.Contains(rec.Body.String(), "codex create ok") {
		t.Fatalf("expected codex response text, got %s", rec.Body.String())
	}
	if tokenCalls != 1 {
		t.Fatalf("expected one token refresh call, got %d form=%v", tokenCalls, tokenForm)
	}
	if responseCalls != 1 || responseAuthorization != "Bearer create-access" || responsePath != "/backend-api/codex/responses" {
		t.Fatalf("unexpected codex upstream call count=%d auth=%q path=%q", responseCalls, responseAuthorization, responsePath)
	}
}

func TestAdminAccountImportCodexRefreshTokenOnlyExchangesTokenWithoutLeakingCredential(t *testing.T) {
	var tokenCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		tokenCalls++
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse token form: %v", err)
		}
		if r.PostForm.Get("refresh_token") != "import-refresh" || r.PostForm.Get("client_id") != codexOAuthClientIDForTest {
			t.Fatalf("unexpected import refresh form: %v", r.PostForm)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"import-access","refresh_token":"import-refresh-rotated","expires_in":3600}`)
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"codex-refresh-import-provider","display_name":"Codex Refresh Import","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)
	body := `{"accounts":[{"provider_id":"` + string(providerResp.Data.Id) + `","name":"codex-refresh-import-account","runtime_class":"oauth_refresh","upstream_client":"codex_cli","credential":{"refresh_token":"import-refresh"},"metadata":{"base_url":"https://codex.invalid/backend-api/codex","oauth_token_url":"` + upstream.URL + `/oauth/token"},"status":"active"}]}`

	importResp, rawBody := mustImportAdminAccountsRaw(t, handler, sessionCookie, loginResp.Data.CsrfToken, body)
	if importResp.Data.CreatedCount != 1 || importResp.Data.SkippedCount != 0 || len(importResp.Data.Errors) != 0 || tokenCalls != 1 {
		t.Fatalf("unexpected import response: %+v token_calls=%d", importResp.Data, tokenCalls)
	}
	if strings.Contains(rawBody, "import-refresh") || strings.Contains(rawBody, "import-access") {
		t.Fatalf("import response leaked credential: %s", rawBody)
	}
}

func TestGatewayCodexRefreshTokenOnlyUpdateCanRequestResponses(t *testing.T) {
	var tokenCalls int
	var responseAuthorization string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			tokenCalls++
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token form: %v", err)
			}
			if r.PostForm.Get("refresh_token") != "updated-refresh" || r.PostForm.Get("scope") != "openid profile email" {
				t.Fatalf("unexpected update refresh form: %v", r.PostForm)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"updated-access","refresh_token":"updated-refresh-rotated","expires_in":3600}`)
		case "/backend-api/codex/responses":
			responseAuthorization = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"codex update ok\"}\n\ndata: [DONE]\n\n")
		default:
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"codex-refresh-update-provider","display_name":"Codex Refresh Update","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"codex-refresh-update-model","display_name":"Codex Refresh Update Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"codex-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"codex-refresh-update-account","runtime_class":"oauth_refresh","upstream_client":"codex_cli","credential":{"access_token":"old-access","refresh_token":"old-refresh"},"metadata":{"base_url":"`+upstream.URL+`/backend-api/codex","oauth_token_url":"`+upstream.URL+`/oauth/token"},"status":"active"}`)

	updateBody := `{"credential":{"refresh_token":"updated-refresh"}}`
	rawUpdate := mustPatchAdminAccountRaw(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(accountResp.Data.Id), updateBody)
	if strings.Contains(rawUpdate, "updated-refresh") || strings.Contains(rawUpdate, "updated-access") {
		t.Fatalf("account update response leaked credential: %s", rawUpdate)
	}

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/responses", `{"model":"codex-refresh-update-model","input":"hello codex update"}`)
	if !strings.Contains(rec.Body.String(), "codex update ok") {
		t.Fatalf("expected codex update response text, got %s", rec.Body.String())
	}
	if tokenCalls != 1 || responseAuthorization != "Bearer updated-access" {
		t.Fatalf("unexpected updated credential use: token_calls=%d auth=%q", tokenCalls, responseAuthorization)
	}
}

func mustCreateAdminAccountRaw(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) (apiopenapi.ProviderAccountResponse, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected account create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	raw := rec.Body.String()
	var resp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(strings.NewReader(raw)).Decode(&resp); err != nil {
		t.Fatalf("decode account response: %v", err)
	}
	return resp, raw
}

func mustImportAdminAccountsRaw(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) (apiopenapi.ProviderAccountImportResponse, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected account import 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	raw := rec.Body.String()
	var resp apiopenapi.ProviderAccountImportResponse
	if err := json.NewDecoder(strings.NewReader(raw)).Decode(&resp); err != nil {
		t.Fatalf("decode account import response: %v", err)
	}
	return resp, raw
}

func mustPatchAdminAccountRaw(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, accountID, body string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/accounts/"+accountID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected account update 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func cloneURLValues(values url.Values) url.Values {
	out := make(url.Values, len(values))
	for key, vals := range values {
		out[key] = append([]string(nil), vals...)
	}
	return out
}
