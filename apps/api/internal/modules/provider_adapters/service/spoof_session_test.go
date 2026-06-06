package service

import (
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// TestSpoofSessionIDInjection proves SpoofSessionID is written into the upstream
// session field for codex (prompt_cache_key) and Anthropic (metadata.user_id),
// and is absent when not requested.
func TestSpoofSessionIDInjection(t *testing.T) {
	req := contract.ConversationRequest{SpoofSessionID: "sess_abc"}
	if got := codexCanonicalResponsesPayload(req)["prompt_cache_key"]; got != "sess_abc" {
		t.Fatalf("codex prompt_cache_key = %v, want sess_abc", got)
	}
	if got := anthropicCompatiblePayload(req).Metadata["user_id"]; got != "sess_abc" {
		t.Fatalf("anthropic metadata.user_id = %v, want sess_abc", got)
	}

	empty := contract.ConversationRequest{}
	if _, ok := codexCanonicalResponsesPayload(empty)["prompt_cache_key"]; ok {
		t.Fatal("codex prompt_cache_key should be absent without spoof or caller key")
	}
	if anthropicCompatiblePayload(empty).Metadata != nil {
		t.Fatal("anthropic metadata should be nil without spoof")
	}
}
