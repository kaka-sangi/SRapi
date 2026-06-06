package service

import (
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

func TestAccountInterceptWarmupEnabled(t *testing.T) {
	if accountInterceptWarmupEnabled(nil) {
		t.Fatal("nil metadata must be disabled")
	}
	if !accountInterceptWarmupEnabled(map[string]any{"intercept_warmup_requests": true}) {
		t.Fatal("bool true must enable")
	}
	if accountInterceptWarmupEnabled(map[string]any{"intercept_warmup_requests": "true"}) {
		t.Fatal("string value must NOT enable (strict bool)")
	}
}

func TestIsWarmupRequest(t *testing.T) {
	warmup := contract.ConversationRequest{
		RawBody: []byte(`{"messages":[{"role":"user","content":"Please write a 5-10 word title for the following conversation:\nhi"}]}`),
	}
	if !isWarmupRequest(warmup) {
		t.Fatal("title-generation request should be detected as warmup")
	}
	normal := contract.ConversationRequest{
		RawBody: []byte(`{"messages":[{"role":"user","content":"summarize this PR for me"}]}`),
	}
	if isWarmupRequest(normal) {
		t.Fatal("a genuine request must NOT be detected as warmup")
	}
}

func TestWarmupMockResponse(t *testing.T) {
	m := warmupMockResponse(contract.ConversationRequest{RequestID: "r1"})
	if m.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", m.StatusCode)
	}
	if len(m.Parts) != 1 || m.Parts[0].Text == "" {
		t.Fatalf("expected a text part, got %+v", m.Parts)
	}
	if m.Usage != (contract.Usage{}) {
		t.Fatalf("warmup mock must report zero usage, got %+v", m.Usage)
	}
}
