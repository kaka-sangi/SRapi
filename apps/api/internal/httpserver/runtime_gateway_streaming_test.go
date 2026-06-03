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
