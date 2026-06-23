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

func TestIsWarmupRequest_MaxTokensOneSmallModel(t *testing.T) {
	one := 1
	cases := []struct {
		name    string
		req     contract.ConversationRequest
		warmup  bool
	}{
		{
			name:   "max_tokens=1 on haiku model",
			req:    contract.ConversationRequest{Model: "claude-3-5-haiku-20241022", MaxOutputTokens: &one, RawBody: []byte(`{}`)},
			warmup: true,
		},
		{
			name:   "max_tokens=1 on gpt-4o-mini",
			req:    contract.ConversationRequest{Model: "gpt-4o-mini", MaxOutputTokens: &one, RawBody: []byte(`{}`)},
			warmup: true,
		},
		{
			name:   "max_tokens=1 on large model should not match",
			req:    contract.ConversationRequest{Model: "claude-sonnet-4-20250514", MaxOutputTokens: &one, RawBody: []byte(`{}`)},
			warmup: false,
		},
		{
			name:   "max_tokens=100 on haiku is not warmup",
			req:    contract.ConversationRequest{Model: "claude-3-5-haiku-20241022", MaxOutputTokens: intPtr(100), RawBody: []byte(`{}`)},
			warmup: false,
		},
		{
			name:   "nil max_tokens on haiku is not warmup",
			req:    contract.ConversationRequest{Model: "claude-3-5-haiku-20241022", RawBody: []byte(`{}`)},
			warmup: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isWarmupRequest(tc.req)
			if got != tc.warmup {
				t.Fatalf("isWarmupRequest() = %v, want %v", got, tc.warmup)
			}
		})
	}
}

func intPtr(v int) *int { return &v }

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
