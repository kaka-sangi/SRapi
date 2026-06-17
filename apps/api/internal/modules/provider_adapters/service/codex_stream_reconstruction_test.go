package service

import (
	"errors"
	"testing"
	"time"
)

func mkEvent(t, data string) SSEEvent {
	return SSEEvent{Type: t, Data: []byte(data)}
}

func TestReconstruct_IndexedOrderingAndFallback(t *testing.T) {
	events := []SSEEvent{
		mkEvent("response.output_item.done", `{"item":{"id":"b","type":"message"},"output_index":1}`),
		mkEvent("response.output_item.done", `{"item":{"id":"a","type":"reasoning"},"output_index":0}`),
		mkEvent("response.output_item.done", `{"item":{"id":"x","type":"tool_call"}}`), // fallback, no index
		mkEvent("response.completed", `{"response":{"output":[]}}`),
	}
	out, ctxOver, err := ReconstructCodexCompletedOutput(events)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ctxOver {
		t.Fatalf("contextLengthExceeded should be false")
	}
	if len(out) != 3 {
		t.Fatalf("got %d items, want 3", len(out))
	}
	if got, _ := out[0]["id"].(string); got != "a" {
		t.Fatalf("first item id = %q want \"a\"", got)
	}
	if got, _ := out[1]["id"].(string); got != "b" {
		t.Fatalf("second item id = %q want \"b\"", got)
	}
	if got, _ := out[2]["id"].(string); got != "x" {
		t.Fatalf("fallback item id = %q want \"x\"", got)
	}
}

func TestReconstruct_CompletedAlreadyHasOutput_NoPatch(t *testing.T) {
	events := []SSEEvent{
		mkEvent("response.output_item.done", `{"item":{"id":"stale"},"output_index":0}`),
		mkEvent("response.completed", `{"response":{"output":[{"id":"final"}]}}`),
	}
	out, _, err := ReconstructCodexCompletedOutput(events)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d, want 1", len(out))
	}
	if got, _ := out[0]["id"].(string); got != "final" {
		t.Fatalf("kept patched item; got id=%q want \"final\"", got)
	}
}

func TestReconstruct_ContextLengthExceeded_FromResponseFailed(t *testing.T) {
	events := []SSEEvent{
		mkEvent("response.failed", `{"response":{"error":{"code":"context_length_exceeded","message":"too long"}}}`),
	}
	_, ctxOver, err := ReconstructCodexCompletedOutput(events)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !ctxOver {
		t.Fatalf("expected contextLengthExceeded=true")
	}
}

func TestReconstruct_ContextLengthExceeded_FromMessage(t *testing.T) {
	events := []SSEEvent{
		mkEvent("error", `{"error":{"code":"unknown","message":"This request exceeds the context window of 128k tokens."}}`),
	}
	_, ctxOver, _ := ReconstructCodexCompletedOutput(events)
	if !ctxOver {
		t.Fatalf("expected contextLengthExceeded=true (via message substring)")
	}
}

func TestReconstruct_EmptyStreamReturnsErr(t *testing.T) {
	_, _, err := ReconstructCodexCompletedOutput(nil)
	if !errors.Is(err, errCodexStreamIncomplete) {
		t.Fatalf("got %v, want errCodexStreamIncomplete", err)
	}
}

func TestReconstruct_MalformedItemIgnored(t *testing.T) {
	events := []SSEEvent{
		mkEvent("response.output_item.done", `{"item":"not_an_object","output_index":0}`),
		mkEvent("response.completed", `{"response":{"output":[]}}`),
	}
	out, _, err := ReconstructCodexCompletedOutput(events)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("malformed item should be skipped, got %d", len(out))
	}
}

func TestClassify_ContextLengthIsTerminal(t *testing.T) {
	v := ClassifyCodexStreamTerminalError(errors.New("context_length_exceeded: request too long"))
	if v.Class != CodexErrClassContextLengthExceeded {
		t.Fatalf("class = %q, want %q", v.Class, CodexErrClassContextLengthExceeded)
	}
	if v.Retryable {
		t.Fatalf("context_length_exceeded must NOT be retryable")
	}
}

func TestClassify_RateLimit_RetryableWithRetryAfter(t *testing.T) {
	v := ClassifyCodexStreamTerminalError(errors.New("rate_limit_exceeded: please retry after 7s"))
	if v.Class != CodexErrClassRateLimit {
		t.Fatalf("class = %q want %q", v.Class, CodexErrClassRateLimit)
	}
	if !v.Retryable {
		t.Fatalf("rate_limit_exceeded must be retryable")
	}
	if v.RetryAfter != 7*time.Second {
		t.Fatalf("RetryAfter = %v want 7s", v.RetryAfter)
	}
}

func TestClassify_5xxRetryable(t *testing.T) {
	for _, msg := range []string{"upstream_5xx", "bad gateway", "service unavailable"} {
		v := ClassifyCodexStreamTerminalError(errors.New(msg))
		if v.Class != CodexErrClassUpstream5xx || !v.Retryable {
			t.Fatalf("%q → %+v", msg, v)
		}
	}
}

func TestClassify_TransientNetworkRetryable(t *testing.T) {
	for _, msg := range []string{"i/o timeout", "unexpected EOF", "stream_interrupted: connection reset"} {
		v := ClassifyCodexStreamTerminalError(errors.New(msg))
		if v.Class != CodexErrClassTransientNetwork || !v.Retryable {
			t.Fatalf("%q → %+v", msg, v)
		}
	}
}

func TestClassify_NilAndUnknown(t *testing.T) {
	if v := ClassifyCodexStreamTerminalError(nil); v.Class != CodexErrClassNone || v.Retryable {
		t.Fatalf("nil err → %+v", v)
	}
	if v := ClassifyCodexStreamTerminalError(errors.New("garbage")); v.Class != CodexErrClassUnknown || v.Retryable {
		t.Fatalf("unknown err → %+v", v)
	}
}
