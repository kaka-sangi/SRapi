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

// TestMePlaygroundChatBillsUser proves the 交界地 playground runs a real,
// session-authenticated gateway chat that streams AND records a usage log for
// the user (the billing path) — without any API key in the request.
func TestMePlaygroundChatBillsUser(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeOpenAISSE(w,
			`{"choices":[{"index":0,"delta":{"role":"assistant","content":"playground-ok"},"finish_reason":null}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`,
		)
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"pg-provider","display_name":"PG","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"pg-model","display_name":"PG Model","status":"active","capabilities":[{"key":"streaming","level":"optional","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"pg-up","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"pg-account","runtime_class":"api_key","credential":{"api_key":"secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	// Models endpoint lists the active model.
	mReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/playground/models", nil)
	mReq.AddCookie(sessionCookie)
	mRec := httptest.NewRecorder()
	handler.ServeHTTP(mRec, mReq)
	if mRec.Code != http.StatusOK {
		t.Fatalf("playground models: expected 200, got %d", mRec.Code)
	}
	var models apiopenapi.PlaygroundModelsResponse
	_ = json.NewDecoder(mRec.Body).Decode(&models)
	if len(models.Data) == 0 {
		t.Fatalf("expected at least one playground model")
	}

	// Chat: session + CSRF, no API key in the request.
	chat := httptest.NewRequest(http.MethodPost, "/api/v1/me/playground/chat", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}],"model":"pg-model"}`))
	chat.AddCookie(sessionCookie)
	chat.Header.Set("X-CSRF-Token", csrf)
	chat.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, chat)
	if rec.Code != http.StatusOK {
		t.Fatalf("playground chat: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "playground-ok") {
		t.Fatalf("expected streamed content in SSE.\nbody: %s", rec.Body.String())
	}

	// Billing path: a usage log must have been recorded for the user.
	uReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=pg-model", nil)
	uReq.AddCookie(sessionCookie)
	uRec := httptest.NewRecorder()
	handler.ServeHTTP(uRec, uReq)
	if uRec.Code != http.StatusOK {
		t.Fatalf("usage-logs: expected 200, got %d", uRec.Code)
	}
	var usage apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(uRec.Body).Decode(&usage); err != nil {
		t.Fatalf("decode usage-logs: %v", err)
	}
	if usage.Pagination.Total < 1 {
		t.Fatalf("expected a usage log recorded for the playground chat, got total=%d", usage.Pagination.Total)
	}
}

// TestMePlaygroundChatRequiresAuth confirms the endpoint is session-gated.
func TestMePlaygroundChatRequiresAuth(t *testing.T) {
	handler := New(config.Load(), nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/me/playground/chat", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}],"model":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no session: expected 401, got %d", rec.Code)
	}
}
