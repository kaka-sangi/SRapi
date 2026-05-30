package httpserver

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
)

func TestGatewayChatCompletionIdempotencyReplaysAndGuards(t *testing.T) {
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"cmpl_1","object":"chat.completion","model":"idem-upstream","choices":[{"index":0,"message":{"role":"assistant","content":"hello world"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":2,"total_tokens":4}}`)
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"idem-provider","display_name":"Idem Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"idem-model","display_name":"Idem Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"idem-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"idem-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	body := `{"model":"idem-model","messages":[{"role":"user","content":"hello"}]}`
	post := func(key, payload string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	first := post("key-1", body)
	if first.Code != http.StatusOK {
		t.Fatalf("expected first request 200, got %d body=%s", first.Code, first.Body.String())
	}
	if calls != 1 {
		t.Fatalf("expected exactly one upstream call after first request, got %d", calls)
	}
	firstBody := first.Body.String()

	replay := post("key-1", body)
	if replay.Code != http.StatusOK {
		t.Fatalf("expected replay 200, got %d body=%s", replay.Code, replay.Body.String())
	}
	if calls != 1 {
		t.Fatalf("expected replay to reuse the stored response without calling upstream, got %d calls", calls)
	}
	if replay.Body.String() != firstBody {
		t.Fatalf("expected replay body to match first response\nfirst: %s\nreplay: %s", firstBody, replay.Body.String())
	}
	if replay.Header().Get(idempotencyReplayedHeader) != "true" {
		t.Fatalf("expected %s header on replay", idempotencyReplayedHeader)
	}

	mismatch := post("key-1", `{"model":"idem-model","messages":[{"role":"user","content":"a different prompt"}]}`)
	if mismatch.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 on key reuse with a different body, got %d body=%s", mismatch.Code, mismatch.Body.String())
	}
	if calls != 1 {
		t.Fatalf("expected key-reuse mismatch to not call upstream, got %d calls", calls)
	}

	noKey := post("", body)
	if noKey.Code != http.StatusOK {
		t.Fatalf("expected key-less request 200, got %d body=%s", noKey.Code, noKey.Body.String())
	}
	if calls != 2 {
		t.Fatalf("expected key-less request to execute against upstream, got %d calls", calls)
	}
}

func TestGatewayChatCompletionIdempotencyStreamingPassesThrough(t *testing.T) {
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chunk_1\",\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"idem-stream-provider","display_name":"Idem Stream","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"idem-stream-model","display_name":"Idem Stream Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"idem-stream-upstream","status":"active","capability_override":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"idem-stream-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	body := `{"model":"idem-stream-model","stream":true,"messages":[{"role":"user","content":"hello"}]}`
	post := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Idempotency-Key", "stream-key")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	if rec := post(); rec.Code != http.StatusOK {
		t.Fatalf("expected first streaming request 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	// Streaming requests are not replayed in v1 — a second call executes again.
	if rec := post(); rec.Code != http.StatusOK {
		t.Fatalf("expected second streaming request 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if calls != 2 {
		t.Fatalf("expected streaming requests to pass through (execute each time), got %d calls", calls)
	}
}

func TestGatewayEmbeddingsIdempotencyReplays(t *testing.T) {
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"object":"list","model":"idem-embed-upstream","data":[{"object":"embedding","index":0,"embedding":[0.1,0.2,0.3]}],"usage":{"prompt_tokens":3,"total_tokens":3}}`)
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"idem-embed-provider","display_name":"Idem Embed","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"embeddings":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"idem-embed-model","display_name":"Idem Embed Model","status":"active","capabilities":[{"key":"embeddings","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"idem-embed-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"idem-embed-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	body := `{"model":"idem-embed-model","input":"hello"}`
	post := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Idempotency-Key", "embed-key")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	first := post()
	if first.Code != http.StatusOK {
		t.Fatalf("expected first embeddings 200, got %d body=%s", first.Code, first.Body.String())
	}
	replay := post()
	if replay.Code != http.StatusOK || replay.Header().Get(idempotencyReplayedHeader) != "true" {
		t.Fatalf("expected replayed embeddings 200, got %d header=%q", replay.Code, replay.Header().Get(idempotencyReplayedHeader))
	}
	if replay.Body.String() != first.Body.String() {
		t.Fatalf("expected embeddings replay body to match first response")
	}
	if calls != 1 {
		t.Fatalf("expected embeddings replay to reuse stored response, got %d upstream calls", calls)
	}
}
