package httpserver

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
)

// flushCapturingWriter is an http.ResponseWriter + http.Flusher that records the
// body as it stood at the FIRST flush, and signals a channel on that first flush.
// It lets a test prove a response was delivered incrementally rather than
// buffered: a streaming handler flushes the first chunk early; a buffering one
// only flushes once everything is written.
type flushCapturingWriter struct {
	header         http.Header
	body           bytes.Buffer
	code           int
	firstFlushBody string
	flushed        bool
	releaseOnce    sync.Once
	release        chan struct{}
}

func (f *flushCapturingWriter) Header() http.Header         { return f.header }
func (f *flushCapturingWriter) Write(b []byte) (int, error) { return f.body.Write(b) }
func (f *flushCapturingWriter) WriteHeader(code int)        { f.code = code }

func (f *flushCapturingWriter) Flush() {
	if !f.flushed {
		f.flushed = true
		f.firstFlushBody = f.body.String()
	}
	f.releaseOnce.Do(func() { close(f.release) })
}

// TestGatewayChatCompletionsStreamsSameProtocolSSEIncrementally proves the
// gateway forwards same-protocol SSE to the client chunk-by-chunk (low
// time-to-first-byte) instead of buffering the whole upstream response. The
// upstream sends the first chunk, flushes, then blocks until the client's first
// flush is observed; this can only complete if the gateway delivered chunk 1
// before the upstream produced the rest.
func TestGatewayChatCompletionsStreamsSameProtocolSSEIncrementally(t *testing.T) {
	chunk1 := "data: {\"id\":\"chunk_1\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"
	chunk2 := "data: {\"id\":\"chunk_2\",\"choices\":[{\"delta\":{\"content\":\" world\"},\"finish_reason\":\"stop\"}]}\n\n"
	usageChunk := "data: {\"id\":\"usage\",\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n\n"
	done := "data: [DONE]\n\n"
	full := chunk1 + chunk2 + usageChunk + done

	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, chunk1)
		if flusher != nil {
			flusher.Flush()
		}
		// Block until the client has received (flushed) the first chunk, or a
		// safety timeout. A buffering gateway never flushes early, so this
		// falls through after the timeout and the assertions below catch it.
		select {
		case <-release:
		case <-time.After(2 * time.Second):
		}
		_, _ = io.WriteString(w, chunk2+usageChunk+done)
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"stream-sse-provider","display_name":"Stream SSE Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"stream-sse-model","display_name":"Stream SSE Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"stream-sse-upstream","status":"active","capability_override":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"stream-sse-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"stream-sse-model","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	rec := &flushCapturingWriter{header: http.Header{}, release: release}
	handler.ServeHTTP(rec, req)

	if rec.code != 0 && rec.code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.code, rec.body.String())
	}
	if got := rec.header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected text/event-stream content type, got %q", got)
	}
	// X-Accel-Buffering: no keeps nginx/ingress from buffering the stream and
	// collapsing it into one all-at-once delivery downstream.
	if got := rec.header.Get("X-Accel-Buffering"); got != "no" {
		t.Fatalf("expected X-Accel-Buffering: no on the SSE response, got %q", got)
	}
	if rec.body.String() != full {
		t.Fatalf("expected full SSE passthrough\nexpected:\n%s\nactual:\n%s", full, rec.body.String())
	}
	if !rec.flushed {
		t.Fatalf("expected at least one flush before completion (incremental streaming), got none")
	}
	if !strings.Contains(rec.firstFlushBody, "chunk_1") {
		t.Fatalf("expected first flush to contain the first chunk, got: %q", rec.firstFlushBody)
	}
	if strings.Contains(rec.firstFlushBody, "chunk_2") {
		t.Fatalf("response was buffered: first flush already contained later chunks: %q", rec.firstFlushBody)
	}
}

// TestGatewayChatCompletionsToCodexStreamsTransformedChunksIncrementally proves
// the cross-shape streaming fix: a chat/completions client backed by a Codex CLI
// account receives correctly-shaped chat.completion.chunk SSE — transformed on
// the fly from the upstream Codex /responses SSE — and incrementally (the first
// chunk is flushed before the upstream produced the rest), not the raw Responses
// events (which a chat client can't parse) and not a buffered single response.
func TestGatewayChatCompletionsToCodexStreamsTransformedChunksIncrementally(t *testing.T) {
	frame1 := "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello\"}\n\n"
	rest := "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\" world\"}\n\n" +
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_x\",\"status\":\"completed\",\"usage\":{\"input_tokens\":2,\"output_tokens\":2,\"total_tokens\":4}}}\n\n" +
		"data: [DONE]\n\n"

	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/responses") {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, frame1)
		if flusher != nil {
			flusher.Flush()
		}
		select {
		case <-release:
		case <-time.After(2 * time.Second):
		}
		_, _ = io.WriteString(w, rest)
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"codex-chat-stream-provider","display_name":"Codex Chat Stream","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"codex-chat-stream-model","display_name":"Codex Chat Stream Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"codex-chat-stream-upstream","status":"active","capability_override":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"codex-chat-stream-account","runtime_class":"cli_client_token","upstream_client":"codex_cli","credential":{"cli_client_token":"codex-token"},"metadata":{"base_url":"`+upstream.URL+`/backend-api/codex"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"codex-chat-stream-model","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	rec := &flushCapturingWriter{header: http.Header{}, release: release}
	handler.ServeHTTP(rec, req)

	if rec.code != 0 && rec.code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.code, rec.body.String())
	}
	body := rec.body.String()
	if !strings.Contains(body, "chat.completion.chunk") {
		t.Fatalf("expected transformed chat.completion.chunk SSE, got:\n%s", body)
	}
	if strings.Contains(body, "response.output_text.delta") {
		t.Fatalf("raw Responses SSE leaked to the chat client instead of being transformed:\n%s", body)
	}
	for _, want := range []string{`"content":"Hello"`, `"content":" world"`, `"finish_reason":"stop"`, "data: [DONE]"} {
		if !strings.Contains(body, want) {
			t.Fatalf("transformed stream missing %q, got:\n%s", want, body)
		}
	}
	if !rec.flushed {
		t.Fatal("expected an early flush (incremental streaming), got none")
	}
	if !strings.Contains(rec.firstFlushBody, `"content":"Hello"`) {
		t.Fatalf("first flush should contain the first transformed chunk, got: %q", rec.firstFlushBody)
	}
	if strings.Contains(rec.firstFlushBody, `"content":" world"`) {
		t.Fatalf("response was buffered: first flush already contained the later chunk: %q", rec.firstFlushBody)
	}
}

func TestGatewayChatCompletionsStreamEmitsKeepaliveDuringUpstreamGap(t *testing.T) {
	chunk1 := "data: {\"id\":\"chunk_1\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"
	chunk2 := "data: {\"id\":\"chunk_2\",\"choices\":[{\"delta\":{\"content\":\" world\"},\"finish_reason\":\"stop\"}]}\n\n"
	done := "data: [DONE]\n\n"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, chunk1)
		if flusher != nil {
			flusher.Flush()
		}
		time.Sleep(100 * time.Millisecond)
		_, _ = io.WriteString(w, chunk2+done)
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	cfg := config.Load()
	cfg.Gateway.StreamIdleTimeout = 300 * time.Millisecond
	cfg.Gateway.StreamKeepaliveInterval = 25 * time.Millisecond
	handler := New(cfg, nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"stream-keepalive-provider","display_name":"Stream Keepalive Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"stream-keepalive-model","display_name":"Stream Keepalive Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"stream-keepalive-upstream","status":"active","capability_override":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"stream-keepalive-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"stream-keepalive-model","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	rec := &flushCapturingWriter{header: http.Header{}, release: make(chan struct{})}
	handler.ServeHTTP(rec, req)

	body := rec.body.String()
	if !strings.Contains(body, "\n\n:\n\n") {
		t.Fatalf("expected SSE keepalive comment between upstream chunks, got: %q", body)
	}
	if !strings.Contains(body, "chunk_2") || !strings.Contains(body, "[DONE]") {
		t.Fatalf("expected stream to complete after keepalive, got: %q", body)
	}
}

func TestGatewayCodexImageGenerationStreamsTransformedEventsIncrementally(t *testing.T) {
	partial := "data: {\"type\":\"response.image_generation_call.partial_image\",\"partial_image_b64\":\"cGFydGlhbA==\",\"partial_image_index\":0,\"output_format\":\"png\",\"background\":\"auto\"}\n\n"
	completed := "data: {\"type\":\"response.completed\",\"response\":{\"created_at\":1710000001,\"usage\":{\"input_tokens\":5,\"output_tokens\":9,\"total_tokens\":14},\"tools\":[{\"type\":\"image_generation\",\"model\":\"gpt-image-2\",\"background\":\"auto\",\"output_format\":\"png\",\"quality\":\"high\",\"size\":\"1024x1024\"}],\"output\":[{\"type\":\"image_generation_call\",\"result\":\"ZmluYWw=\",\"output_format\":\"png\"}]}}\n\n"
	done := "data: [DONE]\n\n"

	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, partial)
		if flusher != nil {
			flusher.Flush()
		}
		select {
		case <-release:
		case <-time.After(2 * time.Second):
		}
		_, _ = io.WriteString(w, completed+done)
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"codex-image-stream-provider","display_name":"Codex Image Stream","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active","capabilities":{"images":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"codex-image-stream-model","display_name":"Codex Image Stream Model","status":"active","capabilities":[{"key":"images","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"gpt-image-2","status":"active","capability_override":[{"key":"images","level":"required","status":"stable","version":"v1"},{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"codex-image-stream-account","runtime_class":"cli_client_token","upstream_client":"codex_cli","credential":{"cli_client_token":"codex-token"},"metadata":{"base_url":"`+upstream.URL+`/backend-api/codex"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"codex-image-stream-model","prompt":"draw a cat","stream":true,"response_format":"url"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	rec := &flushCapturingWriter{header: http.Header{}, release: release}
	handler.ServeHTTP(rec, req)

	if rec.code != 0 && rec.code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.code, rec.body.String())
	}
	if got := rec.header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected text/event-stream content type, got %q", got)
	}
	body := rec.body.String()
	for _, expected := range []string{
		"event: image_generation.partial_image",
		`"url":"data:image/png;base64,cGFydGlhbA=="`,
		"event: image_generation.completed",
		`"url":"data:image/png;base64,ZmluYWw="`,
		"data: [DONE]",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected transformed image stream to contain %q, got %s", expected, body)
		}
	}
	if !rec.flushed || !strings.Contains(rec.firstFlushBody, "image_generation.partial_image") {
		t.Fatalf("expected partial image event in first flush, got %q", rec.firstFlushBody)
	}
	if strings.Contains(rec.firstFlushBody, "image_generation.completed") {
		t.Fatalf("image stream was buffered: first flush already contained completion: %q", rec.firstFlushBody)
	}
}

func TestGatewayImageGenerationStreamEmitsKeepaliveDuringUpstreamGap(t *testing.T) {
	partial := "data: {\"type\":\"response.image_generation_call.partial_image\",\"partial_image_b64\":\"cGFydGlhbA==\",\"partial_image_index\":0,\"output_format\":\"png\"}\n\n"
	completed := "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":5,\"output_tokens\":9,\"total_tokens\":14},\"output\":[{\"type\":\"image_generation_call\",\"result\":\"ZmluYWw=\",\"output_format\":\"png\"}]}}\n\n"
	done := "data: [DONE]\n\n"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, partial)
		if flusher != nil {
			flusher.Flush()
		}
		time.Sleep(100 * time.Millisecond)
		_, _ = io.WriteString(w, completed+done)
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	cfg := config.Load()
	cfg.Gateway.ImageStreamIdleTimeout = 300 * time.Millisecond
	cfg.Gateway.ImageStreamKeepaliveInterval = 25 * time.Millisecond
	handler := New(cfg, nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"codex-image-keepalive-provider","display_name":"Codex Image Keepalive","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active","capabilities":{"images":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"codex-image-keepalive-model","display_name":"Codex Image Keepalive Model","status":"active","capabilities":[{"key":"images","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"gpt-image-2","status":"active","capability_override":[{"key":"images","level":"required","status":"stable","version":"v1"},{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"codex-image-keepalive-account","runtime_class":"cli_client_token","upstream_client":"codex_cli","credential":{"cli_client_token":"codex-token"},"metadata":{"base_url":"`+upstream.URL+`/backend-api/codex"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"codex-image-keepalive-model","prompt":"draw a cat","stream":true,"response_format":"url"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	rec := &flushCapturingWriter{header: http.Header{}, release: make(chan struct{})}
	handler.ServeHTTP(rec, req)

	body := rec.body.String()
	if !strings.Contains(body, "\n\n:\n\n") {
		t.Fatalf("expected SSE keepalive comment between image events, got: %q", body)
	}
	if !strings.Contains(body, "event: image_generation.completed") || !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("expected image stream to complete after keepalive, got: %q", body)
	}
}

func TestGatewayImageGenerationStreamUsesImageIdleTimeout(t *testing.T) {
	partial := "data: {\"type\":\"response.image_generation_call.partial_image\",\"partial_image_b64\":\"cGFydGlhbA==\",\"partial_image_index\":0,\"output_format\":\"png\"}\n\n"
	completed := "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":5,\"output_tokens\":9,\"total_tokens\":14},\"output\":[{\"type\":\"image_generation_call\",\"result\":\"ZmluYWw=\",\"output_format\":\"png\"}]}}\n\n"
	done := "data: [DONE]\n\n"

	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, partial)
		if flusher != nil {
			flusher.Flush()
		}
		select {
		case <-release:
		case <-time.After(2 * time.Second):
		}
		time.Sleep(100 * time.Millisecond)
		_, _ = io.WriteString(w, completed+done)
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	cfg := config.Load()
	cfg.Gateway.StreamIdleTimeout = 50 * time.Millisecond
	cfg.Gateway.ImageStreamIdleTimeout = 300 * time.Millisecond
	handler := New(cfg, nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"codex-image-idle-provider","display_name":"Codex Image Idle","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active","capabilities":{"images":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"codex-image-idle-model","display_name":"Codex Image Idle Model","status":"active","capabilities":[{"key":"images","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"gpt-image-2","status":"active","capability_override":[{"key":"images","level":"required","status":"stable","version":"v1"},{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"codex-image-idle-account","runtime_class":"cli_client_token","upstream_client":"codex_cli","credential":{"cli_client_token":"codex-token"},"metadata":{"base_url":"`+upstream.URL+`/backend-api/codex"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"codex-image-idle-model","prompt":"draw a cat","stream":true,"response_format":"url"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	rec := &flushCapturingWriter{header: http.Header{}, release: release}
	handler.ServeHTTP(rec, req)

	body := rec.body.String()
	if !strings.Contains(body, "event: image_generation.completed") || !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("expected image stream to outlive ordinary stream timeout, got: %s", body)
	}
}

func TestGatewayImageGenerationStreamIdleTimeoutEmitsErrorEvent(t *testing.T) {
	partial := "data: {\"type\":\"response.image_generation_call.partial_image\",\"partial_image_b64\":\"cGFydGlhbA==\",\"partial_image_index\":0,\"output_format\":\"png\"}\n\n"
	hang := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, partial)
		if flusher != nil {
			flusher.Flush()
		}
		select {
		case <-hang:
		case <-time.After(8 * time.Second):
		}
	}))
	defer upstream.Close()
	defer close(hang)

	cfg := config.Load()
	cfg.Gateway.ImageStreamIdleTimeout = 50 * time.Millisecond
	cfg.Gateway.ImageStreamKeepaliveInterval = 0
	handler := New(cfg, nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"codex-image-timeout-error-provider","display_name":"Codex Image Timeout Error","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active","capabilities":{"images":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"codex-image-timeout-error-model","display_name":"Codex Image Timeout Error Model","status":"active","capabilities":[{"key":"images","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"gpt-image-2","status":"active","capability_override":[{"key":"images","level":"required","status":"stable","version":"v1"},{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"codex-image-timeout-error-account","runtime_class":"cli_client_token","upstream_client":"codex_cli","credential":{"cli_client_token":"codex-token"},"metadata":{"base_url":"`+upstream.URL+`/backend-api/codex"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"codex-image-timeout-error-model","prompt":"draw a cat","stream":true,"response_format":"url"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	rec := &flushCapturingWriter{header: http.Header{}, release: make(chan struct{})}
	start := time.Now()
	handler.ServeHTTP(rec, req)
	elapsed := time.Since(start)

	body := rec.body.String()
	for _, expected := range []string{
		"event: image_generation.partial_image",
		"event: error",
		`"message":"upstream image stream idle timeout"`,
		`"code":"stream_idle_timeout"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected image timeout stream to contain %q, got %s", expected, body)
		}
	}
	if strings.Contains(body, "data: [DONE]") {
		t.Fatalf("timeout stream should be interrupted without DONE, got: %s", body)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("image idle timeout did not cut the hung stream promptly; elapsed=%s", elapsed)
	}
}

// TestGatewayChatCompletionStreamIdleTimeoutCutsHungUpstream proves the
// configured StreamIdleTimeout is actually enforced: when an upstream delivers
// one chunk then stalls forever, the gateway cuts the stream (rather than
// holding the client open indefinitely) and records a stream_idle_timeout.
func TestGatewayChatCompletionStreamIdleTimeoutCutsHungUpstream(t *testing.T) {
	chunk1 := "data: {\"id\":\"chunk_1\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"

	hang := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, chunk1)
		if flusher != nil {
			flusher.Flush()
		}
		// Stall: send no more so the gateway must idle-timeout and cut us off.
		// Bounded so the handler can't outlive the test if cleanup races.
		select {
		case <-hang:
		case <-time.After(8 * time.Second):
		}
	}))
	// Order matters: release the stalled handler (close hang) BEFORE upstream.Close,
	// which waits for in-flight handlers. Deferred LIFO runs close(hang) first.
	defer upstream.Close()
	defer close(hang)

	cfg := config.Load()
	// Leave enough room for test-server scheduling jitter so this exercises a
	// post-chunk idle timeout instead of racing first-byte delivery.
	cfg.Gateway.StreamIdleTimeout = 300 * time.Millisecond
	handler := New(cfg, nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"idle-provider","display_name":"Idle Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"idle-model","display_name":"Idle Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"idle-upstream","status":"active","capability_override":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"idle-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"idle-model","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	rec := &flushCapturingWriter{header: http.Header{}, release: make(chan struct{})}
	start := time.Now()
	handler.ServeHTTP(rec, req)
	elapsed := time.Since(start)

	if !strings.Contains(rec.body.String(), "chunk_1") {
		t.Fatalf("expected the first chunk delivered before the idle cut, got: %q", rec.body.String())
	}
	if strings.Contains(rec.body.String(), "[DONE]") {
		t.Fatalf("stream should have been cut by the idle timeout, not completed: %q", rec.body.String())
	}
	// The configured idle timeout plus scheduling slack must remain far below
	// an unbounded upstream stall.
	if elapsed > 2*time.Second {
		t.Fatalf("idle timeout did not cut the hung stream promptly; elapsed=%s", elapsed)
	}
}
