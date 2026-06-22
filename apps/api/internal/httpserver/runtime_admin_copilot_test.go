package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
)

// TestAdminCopilotChatDedicatedReadFlow drives a full copilot turn end-to-end:
// a dedicated LLM (a fake OpenAI upstream) asks to call a read endpoint, and the
// engine dispatches that call IN-PROCESS through the real router using the
// admin's own session — proving auth + routing + result feedback all work.
func TestAdminCopilotChatDedicatedReadFlow(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		body := string(bodyBytes)
		if strings.Contains(body, "tool_call_id") {
			// Second turn: the tool result is present — wrap up (SSE).
			writeOpenAISSE(w,
				`{"choices":[{"index":0,"delta":{"content":"There are no users beyond the admin."},"finish_reason":null}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":5,"total_tokens":10}}`,
			)
			return
		}
		// First turn: ask to call the admin users list (SSE tool-call delta).
		writeOpenAISSE(w,
			`{"choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"call_admin_api","arguments":"{\"method\":\"GET\",\"path\":\"/api/v1/admin/users\"}"}}]},"finish_reason":null}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":5,"total_tokens":10}}`,
		)
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken

	// Configure the copilot: dedicated source pointing at the fake upstream.
	enableDedicatedCopilot(t, handler, sessionCookie, csrf, upstream.URL+"/v1")

	// Drive one chat turn.
	chatReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/copilot/chat", strings.NewReader(`{"messages":[{"role":"user","content":"how many users are there?"}]}`))
	chatReq.AddCookie(sessionCookie)
	chatReq.Header.Set("X-CSRF-Token", csrf)
	chatReq.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, chatReq)

	if rec.Code != http.StatusOK {
		t.Fatalf("copilot chat: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	stream := rec.Body.String()
	for _, want := range []string{"event: tool_call", "event: tool_result", "event: done", "/api/v1/admin/users", "HTTP 200"} {
		if !strings.Contains(stream, want) {
			t.Fatalf("expected SSE stream to contain %q.\nstream:\n%s", want, stream)
		}
	}
	if strings.Contains(stream, "event: pending_action") {
		t.Fatalf("a read call must not require approval.\nstream:\n%s", stream)
	}
}

// TestAdminCopilotChatModelAndEffort proves a per-turn model override and
// reasoning effort reach the upstream request (model name + reasoning_effort for
// the OpenAI protocol).
func TestAdminCopilotChatModelAndEffort(t *testing.T) {
	var lastBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		lastBody = string(b)
		writeOpenAISSE(w,
			`{"choices":[{"index":0,"delta":{"content":"Done."},"finish_reason":null}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
		)
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken
	enableDedicatedCopilot(t, handler, sessionCookie, csrf, upstream.URL+"/v1")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/copilot/chat", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}],"model":"gpt-4o-mini","reasoning_effort":"high"}`))
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("copilot chat: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(lastBody, `"gpt-4o-mini"`) {
		t.Fatalf("expected upstream request to use the overridden model.\nbody: %s", lastBody)
	}
	if !strings.Contains(lastBody, `"reasoning_effort":"high"`) {
		t.Fatalf("expected upstream request to carry reasoning_effort=high.\nbody: %s", lastBody)
	}
}

// TestAdminCopilotChatDisabled confirms the endpoint is gated by the enable flag.
func TestAdminCopilotChatDisabled(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/copilot/chat", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("disabled copilot: expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}

	// Config endpoint should report disabled + not configured.
	cfgReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/copilot/config", nil)
	cfgReq.AddCookie(sessionCookie)
	cfgRec := httptest.NewRecorder()
	handler.ServeHTTP(cfgRec, cfgReq)
	if cfgRec.Code != http.StatusOK {
		t.Fatalf("copilot config: expected 200, got %d", cfgRec.Code)
	}
	var cfg struct {
		Data struct {
			Enabled    bool `json:"enabled"`
			Configured bool `json:"configured"`
		} `json:"data"`
	}
	if err := json.NewDecoder(cfgRec.Body).Decode(&cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg.Data.Enabled {
		t.Fatalf("expected copilot disabled by default")
	}
}

// writeOpenAISSE emits the given JSON chunks as an OpenAI-style SSE stream.
func writeOpenAISSE(w http.ResponseWriter, chunks ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	for _, c := range chunks {
		_, _ = io.WriteString(w, "data: "+c+"\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

// enableDedicatedCopilot reads current settings, flips the copilot to a dedicated
// source pointing at baseURL, and saves.
func enableDedicatedCopilot(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrf, baseURL string) {
	t.Helper()
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	getReq.AddCookie(sessionCookie)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get settings: expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(getRec.Body).Decode(&wrapper); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	var data map[string]any
	if err := json.Unmarshal(wrapper.Data, &data); err != nil {
		t.Fatalf("unmarshal settings data: %v", err)
	}
	data["copilot"] = map[string]any{
		"enabled":                      true,
		"source":                       "dedicated",
		"provider_account_id":          0,
		"provider_account_group_id":    0,
		"model":                        "gpt-4o",
		"dedicated_protocol":           "openai-compatible",
		"dedicated_base_url":           baseURL,
		"dedicated_api_key":            "sk-copilot-test",
		"dedicated_api_key_configured": false,
		"owner_only":                   false,
		"auto_run_reads":               true,
	}
	body, _ := json.Marshal(data)
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", strings.NewReader(string(body)))
	putReq.AddCookie(sessionCookie)
	putReq.Header.Set("X-CSRF-Token", csrf)
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	handler.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("put settings: expected 200, got %d body=%s", putRec.Code, putRec.Body.String())
	}
}
