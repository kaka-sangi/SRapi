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
	opserrorlogscontract "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/contract"
	opserrorlogsmemory "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/store/memory"
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

// TestGatewayChatCompletionsToAnthropicStreamsTransformedIncrementally proves the
// universal cross-protocol streaming pipeline (pair 9): a /v1/chat/completions
// client backed by an Anthropic-compatible upstream receives correctly-shaped
// chat.completion.chunk SSE — transcoded on the fly from the upstream Anthropic
// Messages SSE via the canonical pipeline — and incrementally (the first chunk
// is flushed before the upstream produced the rest), NOT the raw Anthropic
// events (which a chat client cannot parse) and NOT a buffered single response.
func TestGatewayChatCompletionsToAnthropicStreamsTransformedIncrementally(t *testing.T) {
	am := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"model\":\"claude\",\"usage\":{\"input_tokens\":5,\"output_tokens\":0}}}\n\n" +
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n"
	rest := "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\" world\"}}\n\n" +
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"

	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, am)
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
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"x-anthropic-up","display_name":"Anthropic Up","adapter_type":"anthropic-compatible","protocol":"anthropic-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"pair9-model","display_name":"Pair9","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"claude-up","status":"active","capability_override":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"pair9-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"pair9-model","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	rec := &flushCapturingWriter{header: http.Header{}, release: release}
	handler.ServeHTTP(rec, req)

	if rec.code != 0 && rec.code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.code, rec.body.String())
	}
	body := rec.body.String()
	if !strings.Contains(body, "chat.completion.chunk") {
		t.Fatalf("expected transcoded chat.completion.chunk SSE, got:\n%s", body)
	}
	// Raw Anthropic events must NOT leak to the chat client.
	if strings.Contains(body, "message_start") || strings.Contains(body, "content_block_delta") {
		t.Fatalf("raw Anthropic SSE leaked to the chat client instead of being transcoded:\n%s", body)
	}
	for _, want := range []string{`"content":"Hello"`, `"content":" world"`, `"finish_reason":"stop"`, "data: [DONE]"} {
		if !strings.Contains(body, want) {
			t.Fatalf("transcoded stream missing %q, got:\n%s", want, body)
		}
	}
	if !rec.flushed {
		t.Fatal("expected an early flush (incremental streaming), got none")
	}
	// Incremental proof: a chat chunk was flushed before the upstream produced
	// the gated-later " world" delta (a buffering gateway would not have).
	if !strings.Contains(rec.firstFlushBody, "chat.completion.chunk") {
		t.Fatalf("first flush should be a transcoded chat chunk, got: %q", rec.firstFlushBody)
	}
	if strings.Contains(rec.firstFlushBody, `"content":" world"`) {
		t.Fatalf("response was buffered: first flush already contained the later chunk: %q", rec.firstFlushBody)
	}
}

// TestGatewayAnthropicMessagesToOpenAIStreamsTransformedIncrementally proves the
// inverse cross-protocol pair (pair 10): a /v1/messages (Anthropic) client backed
// by an OpenAI-compatible upstream receives transcoded Anthropic Messages SSE
// (message_start / content_block_delta / message_stop) — built on the fly from
// the upstream Chat Completions SSE — incrementally, with no raw OpenAI chunks
// leaking.
func TestGatewayAnthropicMessagesToOpenAIStreamsTransformedIncrementally(t *testing.T) {
	first := "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n"
	rest := "data: {\"choices\":[{\"delta\":{\"content\":\" world\"},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":2,\"total_tokens\":6}}\n\n" +
		"data: [DONE]\n\n"

	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, first)
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
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"x-openai-up","display_name":"OpenAI Up","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"pair10-model","display_name":"Pair10","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"gpt-up","status":"active","capability_override":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"pair10-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"pair10-model","stream":true,"max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	rec := &flushCapturingWriter{header: http.Header{}, release: release}
	handler.ServeHTTP(rec, req)

	if rec.code != 0 && rec.code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.code, rec.body.String())
	}
	body := rec.body.String()
	// Transcoded Anthropic Messages SSE.
	for _, want := range []string{"event: message_start", "event: content_block_delta", `"type":"text_delta"`, "Hello", " world", "event: message_stop"} {
		if !strings.Contains(body, want) {
			t.Fatalf("transcoded Anthropic stream missing %q, got:\n%s", want, body)
		}
	}
	// Raw OpenAI chunks must NOT leak to the Anthropic client.
	if strings.Contains(body, "chat.completion.chunk") || strings.Contains(body, `"choices"`) {
		t.Fatalf("raw OpenAI SSE leaked to the Anthropic client:\n%s", body)
	}
	if !rec.flushed {
		t.Fatal("expected an early flush (incremental streaming), got none")
	}
	if strings.Contains(rec.firstFlushBody, " world") {
		t.Fatalf("response was buffered: first flush already contained the later delta: %q", rec.firstFlushBody)
	}
}

// TestGatewayResponsesToAnthropicStreamsTransformedIncrementally proves the
// responses-client cross-protocol pair: a /v1/responses (OpenAI Responses API)
// client backed by an Anthropic upstream receives transcoded Responses SSE
// (response.created / output_text.delta / response.completed) built on the fly
// from the upstream Anthropic Messages stream, incrementally, with no raw
// Anthropic events leaking.
func TestGatewayResponsesToAnthropicStreamsTransformedIncrementally(t *testing.T) {
	am := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"model\":\"claude\",\"usage\":{\"input_tokens\":5,\"output_tokens\":0}}}\n\n" +
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n"
	rest := "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\" world\"}}\n\n" +
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"

	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, am)
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
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"x-anthropic-up-r","display_name":"Anthropic Up R","adapter_type":"anthropic-compatible","protocol":"anthropic-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"respair-model","display_name":"RespPair","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"claude-up","status":"active","capability_override":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"respair-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"respair-model","stream":true,"input":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	rec := &flushCapturingWriter{header: http.Header{}, release: release}
	handler.ServeHTTP(rec, req)

	if rec.code != 0 && rec.code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.code, rec.body.String())
	}
	body := rec.body.String()
	for _, want := range []string{"event: response.created", "response.output_text.delta", "Hello", " world", "event: response.completed", "data: [DONE]"} {
		if !strings.Contains(body, want) {
			t.Fatalf("transcoded Responses stream missing %q, got:\n%s", want, body)
		}
	}
	if strings.Contains(body, "message_start") || strings.Contains(body, "content_block_delta") || strings.Contains(body, "chat.completion.chunk") {
		t.Fatalf("raw upstream SSE leaked to the Responses client:\n%s", body)
	}
	if !rec.flushed {
		t.Fatal("expected an early flush (incremental streaming), got none")
	}
	if strings.Contains(rec.firstFlushBody, " world") {
		t.Fatalf("response was buffered: first flush already contained the later delta: %q", rec.firstFlushBody)
	}
}

// TestGatewayAnthropicMessagesToCodexStreamsTransformedIncrementally proves the
// last cross-protocol pair: a /v1/messages (Anthropic) client backed by a Codex
// (Responses-API) upstream receives transcoded Anthropic Messages SSE built on
// the fly from the upstream Codex /responses stream, incrementally, with no raw
// Responses events leaking.
func TestGatewayAnthropicMessagesToCodexStreamsTransformedIncrementally(t *testing.T) {
	frame1 := "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello\"}\n\n"
	rest := "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\" world\"}\n\n" +
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"status\":\"completed\",\"usage\":{\"input_tokens\":2,\"output_tokens\":2,\"total_tokens\":4}}}\n\n" +
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
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"x-codex-up","display_name":"Codex Up","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"codexpair-model","display_name":"CodexPair","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"codex-up","status":"active","capability_override":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"codexpair-account","runtime_class":"cli_client_token","upstream_client":"codex_cli","credential":{"cli_client_token":"codex-token"},"metadata":{"base_url":"`+upstream.URL+`/backend-api/codex"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"codexpair-model","stream":true,"max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	rec := &flushCapturingWriter{header: http.Header{}, release: release}
	handler.ServeHTTP(rec, req)

	if rec.code != 0 && rec.code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.code, rec.body.String())
	}
	body := rec.body.String()
	for _, want := range []string{"event: message_start", "event: content_block_delta", `"type":"text_delta"`, "Hello", " world", "event: message_stop"} {
		if !strings.Contains(body, want) {
			t.Fatalf("transcoded Anthropic stream missing %q, got:\n%s", want, body)
		}
	}
	if strings.Contains(body, "response.output_text.delta") || strings.Contains(body, "chat.completion.chunk") {
		t.Fatalf("raw Codex Responses SSE leaked to the Anthropic client:\n%s", body)
	}
	if !rec.flushed {
		t.Fatal("expected an early flush (incremental streaming), got none")
	}
	if strings.Contains(rec.firstFlushBody, " world") {
		t.Fatalf("response was buffered: first flush already contained the later delta: %q", rec.firstFlushBody)
	}
}

// TestGatewayChatCompletionsToGeminiStreamsTransformedIncrementally proves pair 11
// (the gemini-upstream direction): a /v1/chat/completions client backed by a
// Gemini-compatible upstream receives transcoded chat.completion.chunk SSE built
// on the fly from the upstream Gemini streamGenerateContent SSE, incrementally,
// with no raw Gemini candidates leaking.
func TestGatewayChatCompletionsToGeminiStreamsTransformedIncrementally(t *testing.T) {
	first := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hello\"}]},\"index\":0}]}\n\n"
	rest := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\" world\"}]},\"finishReason\":\"STOP\",\"index\":0}],\"usageMetadata\":{\"promptTokenCount\":3,\"candidatesTokenCount\":2,\"totalTokenCount\":5}}\n\n"

	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, first)
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
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"x-gemini-up","display_name":"Gemini Up","adapter_type":"gemini-compatible","protocol":"gemini-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"pair11-model","display_name":"Pair11","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"gemini-up","status":"active","capability_override":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"pair11-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1beta"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"pair11-model","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	rec := &flushCapturingWriter{header: http.Header{}, release: release}
	handler.ServeHTTP(rec, req)

	if rec.code != 0 && rec.code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.code, rec.body.String())
	}
	body := rec.body.String()
	if !strings.Contains(body, "chat.completion.chunk") {
		t.Fatalf("expected transcoded chat.completion.chunk SSE, got:\n%s", body)
	}
	if strings.Contains(body, `"candidates"`) {
		t.Fatalf("raw Gemini SSE leaked to the chat client:\n%s", body)
	}
	for _, want := range []string{`"content":"Hello"`, `"content":" world"`, "data: [DONE]"} {
		if !strings.Contains(body, want) {
			t.Fatalf("transcoded stream missing %q, got:\n%s", want, body)
		}
	}
	if !rec.flushed {
		t.Fatal("expected an early flush (incremental streaming), got none")
	}
	if strings.Contains(rec.firstFlushBody, `"content":" world"`) {
		t.Fatalf("response was buffered: first flush already contained the later chunk: %q", rec.firstFlushBody)
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

// TestGatewayResponsesToCodexStreamsVerbatimIncrementally proves the same-shape
// path: a /v1/responses client backed by a Codex CLI account receives the
// upstream Codex /responses SSE piped through verbatim (Responses-API events,
// NOT rewritten into chat.completion.chunk) and incrementally (first event
// flushed before the upstream produced the rest).
func TestGatewayResponsesToCodexStreamsVerbatimIncrementally(t *testing.T) {
	frame1 := "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello\"}\n\n"
	rest := "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\" world\"}\n\n" +
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_y\",\"status\":\"completed\",\"usage\":{\"input_tokens\":2,\"output_tokens\":2,\"total_tokens\":4}}}\n\n" +
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
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"codex-resp-stream-provider","display_name":"Codex Resp Stream","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"codex-resp-stream-model","display_name":"Codex Resp Stream Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"codex-resp-stream-upstream","status":"active","capability_override":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"codex-resp-stream-account","runtime_class":"cli_client_token","upstream_client":"codex_cli","credential":{"cli_client_token":"codex-token"},"metadata":{"base_url":"`+upstream.URL+`/backend-api/codex"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"codex-resp-stream-model","stream":true,"input":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	rec := &flushCapturingWriter{header: http.Header{}, release: release}
	handler.ServeHTTP(rec, req)

	if rec.code != 0 && rec.code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.code, rec.body.String())
	}
	body := rec.body.String()
	// Verbatim Responses events, NOT rewritten to chat.completion.chunk.
	if !strings.Contains(body, "response.output_text.delta") {
		t.Fatalf("expected verbatim Responses SSE passthrough, got:\n%s", body)
	}
	if strings.Contains(body, "chat.completion.chunk") {
		t.Fatalf("/responses stream was wrongly transformed into chat chunks:\n%s", body)
	}
	for _, want := range []string{"Hello", " world", "data: [DONE]"} {
		if !strings.Contains(body, want) {
			t.Fatalf("verbatim stream missing %q, got:\n%s", want, body)
		}
	}
	// Incremental: the first event was flushed before the upstream sent the rest.
	if !rec.flushed {
		t.Fatal("expected an early flush (incremental streaming), got none")
	}
	if !strings.Contains(rec.firstFlushBody, "Hello") {
		t.Fatalf("first flush should contain the first event, got: %q", rec.firstFlushBody)
	}
	if strings.Contains(rec.firstFlushBody, " world") {
		t.Fatalf("response was buffered: first flush already contained the later event: %q", rec.firstFlushBody)
	}
}

func TestGatewayResponsesStreamEmitsResponseFailedOnIdleTimeout(t *testing.T) {
	first := "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_upstream\",\"object\":\"response\",\"model\":\"responses-idle-upstream\",\"status\":\"in_progress\",\"output\":[]}}\n\n"
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, first)
		if flusher != nil {
			flusher.Flush()
		}
		<-release
	}))
	defer upstream.Close()
	defer close(release)

	cfg := config.Load()
	cfg.Gateway.StreamIdleTimeout = 80 * time.Millisecond
	cfg.Gateway.StreamKeepaliveInterval = 10 * time.Second
	opsStore := opserrorlogsmemory.New()
	handler := New(cfg, nil, WithOpsErrorLogsStore(opsStore))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"responses-idle-provider","display_name":"Responses Idle Provider","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"responses-idle-model","display_name":"Responses Idle Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"responses-idle-upstream","status":"active","capability_override":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"responses-idle-account","runtime_class":"cli_client_token","upstream_client":"codex_cli","credential":{"cli_client_token":"codex-token"},"metadata":{"base_url":"`+upstream.URL+`/backend-api/codex"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"responses-idle-model","stream":true,"input":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set(requestIDHeader, "req_responses_stream_idle_timeout")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "event: response.created") {
		t.Fatalf("expected the first Responses event before the stall, got: %q", body)
	}
	for _, want := range []string{
		"event: response.failed",
		`"type":"response.failed"`,
		`"status":"failed"`,
		`"code":"stream_idle_timeout"`,
		`"message":"upstream stream idle timeout"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected Responses timeout stream to contain %q, got: %q", want, body)
		}
	}
	if strings.Contains(body, "event: error") || strings.Contains(body, "response.completed") || strings.Contains(body, "data: [DONE]") {
		t.Fatalf("Responses timeout stream must fail with response.failed only, got: %q", body)
	}

	opsLog := waitForStreamTimeoutOpsLog(t, opsStore, "req_responses_stream_idle_timeout")
	if opsLog.SourceEndpoint != "/v1/responses" || opsLog.ErrorClass != "stream_idle_timeout" ||
		opsLog.ErrorPhase != "stream" ||
		opsLog.ErrorSource != "upstream_stream" ||
		opsLog.StreamCompletionState != "idle_timeout" {
		t.Fatalf("unexpected Responses stream timeout ops error log: %+v", opsLog)
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

// TestGatewayChatCompletionsStreamEmitsErrorOnIdleTimeout proves a stalled
// upstream surfaces an in-band error frame to the client rather than silently
// truncating the stream (which would leave the client hanging on a connection
// that simply stops producing data).
func TestGatewayChatCompletionsStreamEmitsErrorOnIdleTimeout(t *testing.T) {
	chunk1 := "data: {\"id\":\"chunk_1\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, chunk1)
		if flusher != nil {
			flusher.Flush()
		}
		<-release // stall past the idle timeout so the gateway must give up
	}))
	defer upstream.Close()
	defer close(release)

	cfg := config.Load()
	cfg.Gateway.StreamIdleTimeout = 80 * time.Millisecond
	cfg.Gateway.StreamKeepaliveInterval = 10 * time.Second // keep keepalives out of the body
	opsStore := opserrorlogsmemory.New()
	handler := New(cfg, nil, WithOpsErrorLogsStore(opsStore))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"stream-idle-provider","display_name":"Stream Idle Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"stream-idle-model","display_name":"Stream Idle Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"stream-idle-upstream","status":"active","capability_override":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"stream-idle-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"stream-idle-model","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set(requestIDHeader, "req_stream_idle_timeout")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "chunk_1") {
		t.Fatalf("expected the first chunk before the stall, got: %q", body)
	}
	if !strings.Contains(body, "event: error") || !strings.Contains(body, "stream_idle_timeout") {
		t.Fatalf("expected an idle-timeout error frame, got: %q", body)
	}
	opsLog := waitForStreamTimeoutOpsLog(t, opsStore, "req_stream_idle_timeout")
	if opsLog.ErrorClass != "stream_idle_timeout" ||
		opsLog.ErrorPhase != "stream" ||
		opsLog.ErrorSource != "upstream_stream" ||
		opsLog.StreamCompletionState != "idle_timeout" {
		t.Fatalf("unexpected stream timeout ops error log: %+v", opsLog)
	}
}

func waitForStreamTimeoutOpsLog(t *testing.T, store *opserrorlogsmemory.Store, requestID string) opserrorlogscontract.Entry {
	t.Helper()

	var opsResult opserrorlogscontract.ListResult
	var err error
	deadline := time.Now().Add(2 * time.Second)
	for {
		opsResult, err = store.List(t.Context(), opserrorlogscontract.ListFilter{RequestID: requestID})
		if err != nil {
			t.Fatalf("list ops error logs: %v", err)
		}
		if len(opsResult.Items) > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(opsResult.Items) != 1 {
		t.Fatalf("expected stream timeout ops error log for request %q, got %+v", requestID, opsResult.Items)
	}
	return opsResult.Items[0]
}

func TestResponsesStreamFailureIDSanitizesRequestID(t *testing.T) {
	if got := responsesStreamFailureID("req-a/b c_1"); got != "resp_reqabc_1" {
		t.Fatalf("unexpected sanitized response id %q", got)
	}
	if got := responsesStreamFailureID("resp_existing"); got != "resp_existing" {
		t.Fatalf("expected existing response id to be preserved, got %q", got)
	}
	if got := responsesStreamFailureID(""); got != "resp_stream_error" {
		t.Fatalf("expected fallback response id, got %q", got)
	}
	if got := responsesStreamFailureID(strings.Repeat("a", 200)); len(got) != len("resp_")+96 {
		t.Fatalf("expected bounded response id, got len=%d id=%q", len(got), got)
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

// TestAnthropicStreamUsageAccumulatorRecoversUsage feeds synthetic Anthropic SSE
// chunks into the fallback accumulator and asserts the token tally matches the
// message_start (prompt-side / cache) and message_delta (output) usage events.
// This is the path that recovers usage when the terminal usage event falls
// beyond the bounded meter cap. The chunk boundaries deliberately split SSE
// lines mid-line to prove the accumulator reassembles across reads.
func TestAnthropicStreamUsageAccumulatorRecoversUsage(t *testing.T) {
	full := "event: message_start\n" +
		"data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"usage\":{\"input_tokens\":120,\"cache_read_input_tokens\":40,\"cache_creation_input_tokens\":7,\"output_tokens\":1}}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hi\"}}\n\n" +
		"event: message_delta\n" +
		"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":256}}\n\n" +
		"event: message_stop\n" +
		"data: {\"type\":\"message_stop\"}\n\n"

	// Split into byte-sized chunks of varying length, including boundaries that
	// land in the middle of SSE lines, so the cross-chunk reassembly is exercised.
	var acc anthropicStreamUsageAccumulator
	for i := 0; i < len(full); i += 7 {
		end := i + 7
		if end > len(full) {
			end = len(full)
		}
		// Copy into a fresh, reusable-style buffer to mimic the streaming read
		// loop reusing its byte slice; the accumulator must not alias it.
		buf := make([]byte, end-i)
		copy(buf, full[i:end])
		acc.write(buf)
	}

	if !acc.seen {
		t.Fatalf("expected accumulator to observe usage events, seen=false")
	}
	if acc.usage.InputTokens != 120 {
		t.Fatalf("input_tokens = %d, want 120", acc.usage.InputTokens)
	}
	if acc.usage.OutputTokens != 256 {
		t.Fatalf("output_tokens = %d, want 256 (message_delta should supersede message_start)", acc.usage.OutputTokens)
	}
	if acc.usage.CacheReadInputTokens != 40 {
		t.Fatalf("cache_read_input_tokens = %d, want 40", acc.usage.CacheReadInputTokens)
	}
	if acc.usage.CacheCreationInputTokens != 7 {
		t.Fatalf("cache_creation_input_tokens = %d, want 7", acc.usage.CacheCreationInputTokens)
	}
}

// TestAnthropicStreamUsageAccumulatorNonZeroWins proves the non-zero-wins merge
// semantics ported from sub2api's mergeAnthropicUsage: a later zero-valued field
// must not clobber an earlier non-zero value, while a later non-zero value does.
func TestAnthropicStreamUsageAccumulatorNonZeroWins(t *testing.T) {
	var acc anthropicStreamUsageAccumulator
	// message_start establishes input + cache counts and a provisional output of 1.
	acc.write([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":50,\"cache_read_input_tokens\":10,\"output_tokens\":1}}}\n\n"))
	// message_delta reports output but omits input/cache (zeroes) — they must persist.
	acc.write([]byte("data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":99}}\n\n"))

	if acc.usage.InputTokens != 50 {
		t.Fatalf("input_tokens = %d, want 50 (zero message_delta input must not clobber)", acc.usage.InputTokens)
	}
	if acc.usage.CacheReadInputTokens != 10 {
		t.Fatalf("cache_read_input_tokens = %d, want 10 (must persist across message_delta)", acc.usage.CacheReadInputTokens)
	}
	if acc.usage.OutputTokens != 99 {
		t.Fatalf("output_tokens = %d, want 99 (non-zero message_delta must win)", acc.usage.OutputTokens)
	}
}

// TestAnthropicStreamUsageAccumulatorIgnoresNonUsageStream proves the
// accumulator stays inert on a stream with no Anthropic usage events (e.g. a
// non-Anthropic SSE protocol or a [DONE]-only tail), leaving seen=false so the
// caller keeps the primary meter parse / admission estimate.
func TestAnthropicStreamUsageAccumulatorIgnoresNonUsageStream(t *testing.T) {
	var acc anthropicStreamUsageAccumulator
	acc.write([]byte("event: ping\ndata: {\"type\":\"ping\"}\n\n"))
	acc.write([]byte("data: {\"id\":\"chunk_1\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
	acc.write([]byte("data: [DONE]\n\n"))

	if acc.seen {
		t.Fatalf("expected accumulator to remain inert on a non-Anthropic stream, seen=true usage=%+v", acc.usage)
	}
	if acc.usage.InputTokens != 0 || acc.usage.OutputTokens != 0 {
		t.Fatalf("expected zero usage, got %+v", acc.usage)
	}
}
