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
)

const (
	claudeCodeOAuthClientIDForTest  = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	antigravityOAuthClientIDForTest = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"
)

func TestGatewayClaudeRefreshTokenOnlyCreateCanRequestMessages(t *testing.T) {
	var tokenCalls int
	var messageCalls int
	var tokenPayload map[string]string
	var messageAuthorization string
	var messagePath string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			tokenCalls++
			if r.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("unexpected Claude token headers: %+v", r.Header)
			}
			if err := json.NewDecoder(r.Body).Decode(&tokenPayload); err != nil {
				t.Fatalf("decode Claude token payload: %v", err)
			}
			if tokenPayload["grant_type"] != "refresh_token" ||
				tokenPayload["refresh_token"] != "claude-create-refresh" ||
				tokenPayload["client_id"] != claudeCodeOAuthClientIDForTest {
				t.Fatalf("unexpected Claude refresh payload: %+v", tokenPayload)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"claude-create-access","refresh_token":"claude-create-refresh-rotated","token_type":"Bearer","expires_in":3600}`)
		case "/v1/messages":
			messageCalls++
			messageAuthorization = r.Header.Get("Authorization")
			messagePath = r.URL.Path
			if r.URL.RawQuery != "beta=true" || !strings.Contains(r.Header.Get("Anthropic-Beta"), "oauth-2025-04-20") {
				t.Fatalf("unexpected Claude messages headers/query: query=%q headers=%+v", r.URL.RawQuery, r.Header)
			}
			var payload struct {
				Model  string `json:"model"`
				System []struct {
					Text string `json:"text"`
				} `json:"system"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode Claude messages payload: %v", err)
			}
			if payload.Model != "claude-refresh-upstream" || len(payload.System) < 2 || payload.System[1].Text != "You are Claude Code, Anthropic's official CLI for Claude." {
				t.Fatalf("unexpected Claude messages payload: %+v", payload)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"claude refresh ok"}],"usage":{"input_tokens":3,"output_tokens":4}}`)
		default:
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"claude-refresh-create-provider","display_name":"Claude Refresh Create","adapter_type":"reverse-proxy-claude-code-cli","protocol":"anthropic-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"claude-refresh-create-model","display_name":"Claude Refresh Create Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"claude-refresh-upstream","status":"active"}`)

	accountBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","name":"claude-refresh-create-account","runtime_class":"oauth_refresh","upstream_client":"claude_code_cli","credential":{"refresh_token":"claude-create-refresh"},"metadata":{"base_url":"` + upstream.URL + `/v1","oauth_token_url":"` + upstream.URL + `/oauth/token","user_agent":"claude-cli/test"},"status":"active"}`
	_, rawAccount := mustCreateAdminAccountRaw(t, handler, sessionCookie, loginResp.Data.CsrfToken, accountBody)
	if strings.Contains(rawAccount, "claude-create-refresh") || strings.Contains(rawAccount, "claude-create-access") {
		t.Fatalf("account create response leaked credential: %s", rawAccount)
	}

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/messages", `{"model":"claude-refresh-create-model","max_tokens":32,"messages":[{"role":"user","content":"hello claude refresh"}]}`)
	if !strings.Contains(rec.Body.String(), "claude refresh ok") {
		t.Fatalf("expected Claude response text, got %s", rec.Body.String())
	}
	if tokenCalls != 1 || messageCalls != 1 || messageAuthorization != "Bearer claude-create-access" || messagePath != "/v1/messages" {
		t.Fatalf("unexpected Claude refresh flow: token_calls=%d message_calls=%d auth=%q path=%q", tokenCalls, messageCalls, messageAuthorization, messagePath)
	}
}

func TestAdminAccountImportClaudeRefreshTokenOnlyExchangesTokenWithoutLeakingCredential(t *testing.T) {
	var tokenCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		tokenCalls++
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode Claude import refresh payload: %v", err)
		}
		if payload["refresh_token"] != "claude-import-refresh" || payload["client_id"] != claudeCodeOAuthClientIDForTest {
			t.Fatalf("unexpected Claude import refresh payload: %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"claude-import-access","refresh_token":"claude-import-refresh-rotated","expires_in":3600}`)
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"claude-refresh-import-provider","display_name":"Claude Refresh Import","adapter_type":"reverse-proxy-claude-code-cli","protocol":"anthropic-compatible","status":"active"}`)
	body := `{"accounts":[{"provider_id":"` + string(providerResp.Data.Id) + `","name":"claude-refresh-import-account","runtime_class":"oauth_refresh","upstream_client":"claude_code_cli","credential":{"refresh_token":"claude-import-refresh"},"metadata":{"base_url":"https://claude.invalid/v1","oauth_token_url":"` + upstream.URL + `/oauth/token"},"status":"active"}]}`

	importResp, rawBody := mustImportAdminAccountsRaw(t, handler, sessionCookie, loginResp.Data.CsrfToken, body)
	if importResp.Data.CreatedCount != 1 || importResp.Data.SkippedCount != 0 || len(importResp.Data.Errors) != 0 || tokenCalls != 1 {
		t.Fatalf("unexpected Claude import response: %+v token_calls=%d", importResp.Data, tokenCalls)
	}
	if strings.Contains(rawBody, "claude-import-refresh") || strings.Contains(rawBody, "claude-import-access") {
		t.Fatalf("Claude import response leaked credential: %s", rawBody)
	}
}

func TestGatewayClaudeRefreshTokenOnlyUpdateCanRequestMessages(t *testing.T) {
	var tokenCalls int
	var messageAuthorization string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			tokenCalls++
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode Claude update refresh payload: %v", err)
			}
			if payload["refresh_token"] != "claude-updated-refresh" {
				t.Fatalf("unexpected Claude update refresh payload: %+v", payload)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"claude-updated-access","refresh_token":"claude-updated-refresh-rotated","expires_in":3600}`)
		case "/v1/messages":
			messageAuthorization = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"claude update ok"}],"usage":{"input_tokens":5,"output_tokens":6}}`)
		default:
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"claude-refresh-update-provider","display_name":"Claude Refresh Update","adapter_type":"reverse-proxy-claude-code-cli","protocol":"anthropic-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"claude-refresh-update-model","display_name":"Claude Refresh Update Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"claude-refresh-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"claude-refresh-update-account","runtime_class":"oauth_refresh","upstream_client":"claude_code_cli","credential":{"access_token":"old-claude-access","refresh_token":"old-claude-refresh"},"metadata":{"base_url":"`+upstream.URL+`/v1","oauth_token_url":"`+upstream.URL+`/oauth/token"},"status":"active"}`)

	rawUpdate := mustPatchAdminAccountRaw(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(accountResp.Data.Id), `{"credential":{"refresh_token":"claude-updated-refresh"}}`)
	if strings.Contains(rawUpdate, "claude-updated-refresh") || strings.Contains(rawUpdate, "claude-updated-access") {
		t.Fatalf("Claude account update response leaked credential: %s", rawUpdate)
	}

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/messages", `{"model":"claude-refresh-update-model","max_tokens":32,"messages":[{"role":"user","content":"hello claude update"}]}`)
	if !strings.Contains(rec.Body.String(), "claude update ok") {
		t.Fatalf("expected Claude update response text, got %s", rec.Body.String())
	}
	if tokenCalls != 1 || messageAuthorization != "Bearer claude-updated-access" {
		t.Fatalf("unexpected Claude updated credential use: token_calls=%d auth=%q", tokenCalls, messageAuthorization)
	}
}

func TestGatewayAntigravityRefreshTokenOnlyCreateCanRequestChat(t *testing.T) {
	var tokenCalls int
	var generateAuthorization string
	var generatePath string
	var tokenForm url.Values
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			tokenCalls++
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse Antigravity token form: %v", err)
			}
			tokenForm = cloneURLValues(r.PostForm)
			if r.PostForm.Get("grant_type") != "refresh_token" ||
				r.PostForm.Get("refresh_token") != "antigravity-create-refresh" ||
				r.PostForm.Get("client_id") != antigravityOAuthClientIDForTest ||
				r.PostForm.Get("client_secret") != "antigravity-test-secret" {
				t.Fatalf("unexpected Antigravity refresh form: %v", r.PostForm)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"antigravity-create-access","refresh_token":"antigravity-create-refresh-rotated","token_type":"Bearer","expires_in":3600}`)
		case "/v1internal:generateContent":
			generateAuthorization = r.Header.Get("Authorization")
			generatePath = r.URL.Path
			var payload struct {
				Project string `json:"project"`
				Model   string `json:"model"`
				Request struct {
					Contents []struct {
						Parts []struct {
							Text string `json:"text"`
						} `json:"parts"`
					} `json:"contents"`
				} `json:"request"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode Antigravity payload: %v", err)
			}
			if payload.Project != "project-1" || payload.Model != "antigravity-refresh-upstream" {
				t.Fatalf("unexpected Antigravity payload: %+v", payload)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"response":{"candidates":[{"content":{"parts":[{"text":"antigravity refresh ok"}]}}],"usageMetadata":{"promptTokenCount":6,"candidatesTokenCount":7}}}`)
		default:
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"antigravity-refresh-create-provider","display_name":"Antigravity Refresh Create","adapter_type":"reverse-proxy-antigravity","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"antigravity-refresh-create-model","display_name":"Antigravity Refresh Create Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"antigravity-refresh-upstream","status":"active"}`)

	accountBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","name":"antigravity-refresh-create-account","runtime_class":"oauth_refresh","upstream_client":"antigravity_desktop","credential":{"refresh_token":"antigravity-create-refresh","oauth_client_secret":"antigravity-test-secret"},"metadata":{"base_url":"` + upstream.URL + `","oauth_token_url":"` + upstream.URL + `/oauth/token","project_id":"project-1"},"status":"active"}`
	_, rawAccount := mustCreateAdminAccountRaw(t, handler, sessionCookie, loginResp.Data.CsrfToken, accountBody)
	if strings.Contains(rawAccount, "antigravity-create-refresh") || strings.Contains(rawAccount, "antigravity-create-access") || strings.Contains(rawAccount, "antigravity-test-secret") {
		t.Fatalf("Antigravity account create response leaked credential: %s", rawAccount)
	}

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"antigravity-refresh-create-model","messages":[{"role":"user","content":"hello antigravity refresh"}]}`)
	if !strings.Contains(rec.Body.String(), "antigravity refresh ok") {
		t.Fatalf("expected Antigravity response text, got %s", rec.Body.String())
	}
	if tokenCalls != 1 || tokenForm.Get("refresh_token") != "antigravity-create-refresh" || generateAuthorization != "Bearer antigravity-create-access" || generatePath != "/v1internal:generateContent" {
		t.Fatalf("unexpected Antigravity refresh flow: token_calls=%d form=%v auth=%q path=%q", tokenCalls, tokenForm, generateAuthorization, generatePath)
	}
}

func TestAdminAccountImportAntigravityRefreshTokenOnlyRequiresClientSecret(t *testing.T) {
	tokenCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenCalls++
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse Antigravity import refresh form: %v", err)
		}
		if r.PostForm.Get("client_secret") != "antigravity-import-secret" {
			t.Fatalf("unexpected Antigravity import form: %v", r.PostForm)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"antigravity-import-access","refresh_token":"antigravity-import-refresh-rotated","expires_in":3600}`)
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"antigravity-refresh-import-provider","display_name":"Antigravity Refresh Import","adapter_type":"reverse-proxy-antigravity","protocol":"openai-compatible","status":"active"}`)
	body := `{"accounts":[{"provider_id":"` + string(providerResp.Data.Id) + `","name":"antigravity-refresh-import-account","runtime_class":"oauth_refresh","upstream_client":"antigravity_desktop","credential":{"refresh_token":"antigravity-import-refresh","oauth_client_secret":"antigravity-import-secret"},"metadata":{"base_url":"https://antigravity.invalid","oauth_token_url":"` + upstream.URL + `","project_id":"project-1"},"status":"active"}]}`

	importResp, rawBody := mustImportAdminAccountsRaw(t, handler, sessionCookie, loginResp.Data.CsrfToken, body)
	if importResp.Data.CreatedCount != 1 || importResp.Data.SkippedCount != 0 || len(importResp.Data.Errors) != 0 || tokenCalls != 1 {
		t.Fatalf("unexpected Antigravity import response: %+v token_calls=%d", importResp.Data, tokenCalls)
	}
	if strings.Contains(rawBody, "antigravity-import-refresh") || strings.Contains(rawBody, "antigravity-import-access") || strings.Contains(rawBody, "antigravity-import-secret") {
		t.Fatalf("Antigravity import response leaked credential: %s", rawBody)
	}
}

func TestGatewayAntigravityRefreshTokenOnlyUpdateCanRequestChat(t *testing.T) {
	var tokenCalls int
	var generateAuthorization string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			tokenCalls++
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse Antigravity update refresh form: %v", err)
			}
			if r.PostForm.Get("refresh_token") != "antigravity-updated-refresh" || r.PostForm.Get("client_secret") != "antigravity-updated-secret" {
				t.Fatalf("unexpected Antigravity update refresh form: %v", r.PostForm)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"antigravity-updated-access","refresh_token":"antigravity-updated-refresh-rotated","expires_in":3600}`)
		case "/v1internal:generateContent":
			generateAuthorization = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"response":{"candidates":[{"content":{"parts":[{"text":"antigravity update ok"}]}}],"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":8}}}`)
		default:
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"antigravity-refresh-update-provider","display_name":"Antigravity Refresh Update","adapter_type":"reverse-proxy-antigravity","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"antigravity-refresh-update-model","display_name":"Antigravity Refresh Update Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"antigravity-refresh-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"antigravity-refresh-update-account","runtime_class":"oauth_refresh","upstream_client":"antigravity_desktop","credential":{"access_token":"old-antigravity-access","refresh_token":"old-antigravity-refresh","oauth_client_secret":"old-antigravity-secret"},"metadata":{"base_url":"`+upstream.URL+`","oauth_token_url":"`+upstream.URL+`/oauth/token","project_id":"project-1"},"status":"active"}`)

	rawUpdate := mustPatchAdminAccountRaw(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(accountResp.Data.Id), `{"credential":{"refresh_token":"antigravity-updated-refresh","oauth_client_secret":"antigravity-updated-secret"}}`)
	if strings.Contains(rawUpdate, "antigravity-updated-refresh") || strings.Contains(rawUpdate, "antigravity-updated-access") || strings.Contains(rawUpdate, "antigravity-updated-secret") {
		t.Fatalf("Antigravity account update response leaked credential: %s", rawUpdate)
	}

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"antigravity-refresh-update-model","messages":[{"role":"user","content":"hello antigravity update"}]}`)
	if !strings.Contains(rec.Body.String(), "antigravity update ok") {
		t.Fatalf("expected Antigravity update response text, got %s", rec.Body.String())
	}
	if tokenCalls != 1 || generateAuthorization != "Bearer antigravity-updated-access" {
		t.Fatalf("unexpected Antigravity updated credential use: token_calls=%d auth=%q", tokenCalls, generateAuthorization)
	}
}
