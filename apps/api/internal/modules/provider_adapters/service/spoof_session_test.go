package service

import (
	"encoding/json"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// TestSpoofSessionIDInjection proves SpoofSessionID is written into the codex
// session field (prompt_cache_key), and absent when not requested. (The Anthropic
// path uses a metadata.user_id override transform — see TestAnthropicSpoofTransform.)
func TestSpoofSessionIDInjection(t *testing.T) {
	req := contract.ConversationRequest{SpoofSessionID: "sess_abc"}
	if got := codexCanonicalResponsesPayload(req)["prompt_cache_key"]; got != "sess_abc" {
		t.Fatalf("codex prompt_cache_key = %v, want sess_abc", got)
	}
	if _, ok := codexCanonicalResponsesPayload(contract.ConversationRequest{})["prompt_cache_key"]; ok {
		t.Fatal("codex prompt_cache_key should be absent without spoof or caller key")
	}
}

// TestAnthropicSpoofTransform proves a metadata.user_id override transform lands
// in the marshaled Anthropic body (the mechanism the gateway uses for spoofing).
func TestAnthropicSpoofTransform(t *testing.T) {
	raw := []byte(`{"model":"claude","messages":[]}`)
	out, err := applyPayloadTransforms(raw, []contract.PayloadTransform{
		{Action: "override", Path: "metadata.user_id", Value: "sess_abc"},
	})
	if err != nil {
		t.Fatalf("applyPayloadTransforms: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	metadata, _ := doc["metadata"].(map[string]any)
	if metadata["user_id"] != "sess_abc" {
		t.Fatalf("expected metadata.user_id=sess_abc, got %v", doc["metadata"])
	}
}
