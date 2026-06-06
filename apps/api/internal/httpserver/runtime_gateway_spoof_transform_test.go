package httpserver

import (
	"testing"

	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

func TestAnthropicSpoofSessionTransform(t *testing.T) {
	// Anthropic target + spoof id → an override transform for metadata.user_id.
	tr := anthropicSpoofSessionTransform(provideradaptercontract.ConversationRequest{
		TargetProtocol: string(gatewaycontract.ProtocolAnthropicCompatible),
		SpoofSessionID: "sess_abc",
	})
	if tr == nil || tr.Action != "override" || tr.Path != "metadata.user_id" || tr.Value != "sess_abc" {
		t.Fatalf("expected metadata.user_id override transform, got %+v", tr)
	}

	// No spoof id → no transform.
	if anthropicSpoofSessionTransform(provideradaptercontract.ConversationRequest{
		TargetProtocol: string(gatewaycontract.ProtocolAnthropicCompatible),
	}) != nil {
		t.Fatal("expected no transform without a spoof id")
	}

	// Non-Anthropic target → no transform (codex uses prompt_cache_key instead).
	if anthropicSpoofSessionTransform(provideradaptercontract.ConversationRequest{
		TargetProtocol: string(gatewaycontract.ProtocolOpenAICompatible),
		SpoofSessionID: "sess_abc",
	}) != nil {
		t.Fatal("expected no Anthropic transform for a non-Anthropic target")
	}
}
