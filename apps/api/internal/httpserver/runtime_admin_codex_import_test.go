package httpserver

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// codexTestJWT builds an unsigned (signature placeholder) JWT whose payload
// carries the OpenAI auth claims the importer extracts. The handler decodes the
// payload only and never verifies the signature.
func codexTestJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal jwt claims: %v", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	return header + "." + payload + ".sig"
}

func mustImportCodexSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) (apiopenapi.CodexSessionImportResponse, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/import/codex-session", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected codex session import 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	raw := rec.Body.String()
	var resp apiopenapi.CodexSessionImportResponse
	if err := json.NewDecoder(strings.NewReader(raw)).Decode(&resp); err != nil {
		t.Fatalf("decode codex session import response: %v", err)
	}
	return resp, raw
}

func TestAdminImportCodexSessionFromFullSessionJSON(t *testing.T) {
	var tokenCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		tokenCalls++
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse token form: %v", err)
		}
		if r.PostForm.Get("refresh_token") != "session-refresh" || r.PostForm.Get("client_id") != codexOAuthClientIDForTest {
			t.Fatalf("unexpected codex import refresh form: %v", r.PostForm)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"minted-access","refresh_token":"session-refresh-rotated","expires_in":3600}`)
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"codex-session-import-provider","display_name":"Codex Session Import","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)

	accessJWT := codexTestJWT(t, map[string]any{
		"sub":   "auth0|user-123",
		"email": "ada@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "acct-abc",
			"chatgpt_user_id":    "user-abc",
			"chatgpt_plan_type":  "pro",
			"organizations":      []map[string]any{{"id": "org-default", "is_default": true}},
		},
	})
	sessionJSON := map[string]any{
		"oauth_token_url": upstream.URL + "/oauth/token",
		"base_url":        upstream.URL + "/backend-api/codex",
		"tokens": map[string]any{
			"access_token":  accessJWT,
			"refresh_token": "session-refresh",
		},
	}
	sessionBytes, err := json.Marshal(sessionJSON)
	if err != nil {
		t.Fatalf("marshal session json: %v", err)
	}
	contentEscaped, err := json.Marshal(string(sessionBytes))
	if err != nil {
		t.Fatalf("escape content: %v", err)
	}

	body := `{"provider_id":"` + string(providerResp.Data.Id) + `","content":` + string(contentEscaped) + `,"name":"Ada Codex"}`

	resp, raw := mustImportCodexSession(t, handler, sessionCookie, loginResp.Data.CsrfToken, body)
	if resp.Data.Total != 1 || resp.Data.Created != 1 || resp.Data.Updated != 0 || resp.Data.Failed != 0 || resp.Data.Skipped != 0 {
		t.Fatalf("unexpected counts: %+v", resp.Data)
	}
	if tokenCalls != 1 {
		t.Fatalf("expected one token mint during create, got %d", tokenCalls)
	}
	if strings.Contains(raw, "session-refresh") || strings.Contains(raw, "minted-access") || strings.Contains(raw, accessJWT) {
		t.Fatalf("codex import response leaked credential: %s", raw)
	}
	if len(resp.Data.Items) != 1 || resp.Data.Items[0].Action != apiopenapi.CodexSessionImportItemActionCreated || resp.Data.Items[0].AccountId == nil {
		t.Fatalf("unexpected created item: %+v", resp.Data.Items)
	}

	// Inspect the created account: identity is recorded in plaintext metadata.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(*resp.Data.Items[0].AccountId), nil)
	getReq.AddCookie(sessionCookie)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected account inspect 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	var getResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(getRec.Body).Decode(&getResp); err != nil {
		t.Fatalf("decode account inspect: %v", err)
	}
	if getResp.Data.Metadata == nil {
		t.Fatalf("expected account metadata")
	}
	metadata := *getResp.Data.Metadata
	if metadata["codex_account_id"] != "acct-abc" || metadata["codex_email"] != "ada@example.com" || metadata["codex_plan_type"] != "pro" || metadata["codex_organization_id"] != "org-default" {
		t.Fatalf("unexpected codex metadata: %+v", metadata)
	}

	// Re-importing the same identity updates the existing account (no skip).
	tokenCalls = 0
	resp2, _ := mustImportCodexSession(t, handler, sessionCookie, loginResp.Data.CsrfToken, body)
	if resp2.Data.Updated != 1 || resp2.Data.Created != 0 {
		t.Fatalf("expected update on re-import, got %+v", resp2.Data)
	}
}

func TestAdminImportCodexSessionRawAccessTokenBatch(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"codex-raw-batch-provider","display_name":"Codex Raw Batch","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)

	jwtA := codexTestJWT(t, map[string]any{
		"email": "alice@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "acct-A",
			"chatgpt_user_id":    "user-A",
		},
	})
	jwtB := codexTestJWT(t, map[string]any{
		"email": "bob@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "acct-B",
			"chatgpt_user_id":    "user-B",
		},
	})
	// NDJSON: two raw access tokens plus a duplicate of the first.
	ndjson := jwtA + "\n" + jwtB + "\n" + jwtA
	contentEscaped, err := json.Marshal(ndjson)
	if err != nil {
		t.Fatalf("escape ndjson: %v", err)
	}
	body := `{"provider_id":"` + string(providerResp.Data.Id) + `","content":` + string(contentEscaped) + `}`

	resp, raw := mustImportCodexSession(t, handler, sessionCookie, loginResp.Data.CsrfToken, body)
	if resp.Data.Total != 3 || resp.Data.Created != 2 || resp.Data.Skipped != 1 || resp.Data.Failed != 0 {
		t.Fatalf("unexpected raw batch counts: %+v", resp.Data)
	}
	if strings.Contains(raw, jwtA) || strings.Contains(raw, jwtB) {
		t.Fatalf("raw batch response leaked access token: %s", raw)
	}
	// No refresh token in any entry => each should carry the no-renew warning.
	if len(resp.Data.Warnings) == 0 {
		t.Fatalf("expected refresh-token warnings, got none")
	}
}

func TestAdminImportCodexSessionNoRefreshTokenRequiresValidExpiry(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"codex-no-refresh-provider","display_name":"Codex No Refresh","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)

	// Expired access token (no refresh token) must fail the item.
	expiredJWT := codexTestJWT(t, map[string]any{
		"email": "stale@example.com",
		"exp":   time.Now().Add(-time.Hour).Unix(),
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "acct-stale",
		},
	})
	expiredEscaped, err := json.Marshal(expiredJWT)
	if err != nil {
		t.Fatalf("escape expired jwt: %v", err)
	}
	expiredBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","content":` + string(expiredEscaped) + `}`
	expiredResp, _ := mustImportCodexSession(t, handler, sessionCookie, loginResp.Data.CsrfToken, expiredBody)
	if expiredResp.Data.Failed != 1 || expiredResp.Data.Created != 0 {
		t.Fatalf("expected expired no-refresh import to fail, got %+v", expiredResp.Data)
	}
	if len(expiredResp.Data.Errors) != 1 || !strings.Contains(expiredResp.Data.Errors[0].Message, "expired") {
		t.Fatalf("expected expiry error, got %+v", expiredResp.Data.Errors)
	}

	// Valid (future expiry) access token with no refresh token imports and is
	// recorded with auto-pause-on-expiry metadata.
	validJWT := codexTestJWT(t, map[string]any{
		"email": "fresh@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "acct-fresh",
		},
	})
	validEscaped, err := json.Marshal(validJWT)
	if err != nil {
		t.Fatalf("escape valid jwt: %v", err)
	}
	validBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","content":` + string(validEscaped) + `}`
	validResp, _ := mustImportCodexSession(t, handler, sessionCookie, loginResp.Data.CsrfToken, validBody)
	if validResp.Data.Created != 1 || validResp.Data.Failed != 0 {
		t.Fatalf("expected valid no-refresh import to succeed, got %+v", validResp.Data)
	}
	if len(validResp.Data.Items) != 1 || validResp.Data.Items[0].AccountId == nil {
		t.Fatalf("unexpected valid item: %+v", validResp.Data.Items)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(*validResp.Data.Items[0].AccountId), nil)
	getReq.AddCookie(sessionCookie)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected account inspect 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	var getResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(getRec.Body).Decode(&getResp); err != nil {
		t.Fatalf("decode account inspect: %v", err)
	}
	if getResp.Data.Metadata == nil || (*getResp.Data.Metadata)["auto_pause_on_expired"] != true {
		t.Fatalf("expected auto_pause_on_expired metadata, got %+v", getResp.Data.Metadata)
	}
}
