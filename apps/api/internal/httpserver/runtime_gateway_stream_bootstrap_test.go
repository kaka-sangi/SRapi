package httpserver

import (
	"io"
	"strings"
	"testing"
)

func TestBootstrapPeekStreamNormalSSE(t *testing.T) {
	body := io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
	peeked, err := bootstrapPeekStream(body)
	if err != nil {
		t.Fatalf("expected no error for normal SSE, got %v", err)
	}
	all, _ := io.ReadAll(peeked)
	if !strings.Contains(string(all), "hello") {
		t.Fatalf("expected peeked body to contain original data, got %q", all)
	}
}

func TestBootstrapPeekStreamJSONError(t *testing.T) {
	body := io.NopCloser(strings.NewReader(`{"error":{"message":"insufficient_quota","type":"insufficient_quota","code":"insufficient_quota"}}`))
	_, err := bootstrapPeekStream(body)
	if err == nil {
		t.Fatal("expected error for JSON error body")
	}
}

func TestBootstrapPeekStreamSSEErrorEvent(t *testing.T) {
	body := io.NopCloser(strings.NewReader("event: error\ndata: {\"error\":\"overloaded\"}\n\n"))
	_, err := bootstrapPeekStream(body)
	if err == nil {
		t.Fatal("expected error for SSE error event")
	}
}

func TestBootstrapPeekStreamNilBody(t *testing.T) {
	_, err := bootstrapPeekStream(nil)
	if err == nil {
		t.Fatal("expected error for nil body")
	}
}

func TestBootstrapPeekStreamEmptyBody(t *testing.T) {
	body := io.NopCloser(strings.NewReader(""))
	_, err := bootstrapPeekStream(body)
	if err == nil {
		t.Fatal("expected error for empty body")
	}
}

func TestBootstrapPeekStreamPreservesFullBody(t *testing.T) {
	original := "data: {\"id\":\"1\"}\n\ndata: {\"id\":\"2\"}\n\ndata: [DONE]\n\n"
	body := io.NopCloser(strings.NewReader(original))
	peeked, err := bootstrapPeekStream(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	all, _ := io.ReadAll(peeked)
	if string(all) != original {
		t.Fatalf("body corrupted:\ngot:  %q\nwant: %q", all, original)
	}
}
