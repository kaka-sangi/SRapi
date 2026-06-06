package httpserver

import (
	"testing"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
)

func TestGatewaySpoofSessionID(t *testing.T) {
	canonical := gatewaycontract.CanonicalRequest{
		Messages: []gatewaycontract.Message{
			{Role: "user", Content: []gatewaycontract.ContentBlock{{Type: gatewaycontract.ContentBlockText, Text: "hello"}}},
		},
	}

	// Disabled by default.
	if id := gatewaySpoofSessionID(accountcontract.ProviderAccount{}, canonical); id != "" {
		t.Fatalf("expected no spoof id when disabled, got %q", id)
	}

	enabled := accountcontract.ProviderAccount{Metadata: map[string]any{"spoof_session_id": true}}
	id1 := gatewaySpoofSessionID(enabled, canonical)
	if id1 == "" {
		t.Fatal("expected a spoof id when enabled with a derivable session")
	}

	// Stable across turns of the same conversation.
	turn2 := gatewaycontract.CanonicalRequest{
		Messages: []gatewaycontract.Message{
			{Role: "user", Content: []gatewaycontract.ContentBlock{{Type: gatewaycontract.ContentBlockText, Text: "hello"}}},
			{Role: "assistant", Content: []gatewaycontract.ContentBlock{{Type: gatewaycontract.ContentBlockText, Text: "hi"}}},
			{Role: "user", Content: []gatewaycontract.ContentBlock{{Type: gatewaycontract.ContentBlockText, Text: "more"}}},
		},
	}
	if id2 := gatewaySpoofSessionID(enabled, turn2); id2 != id1 {
		t.Fatalf("spoof id must be stable across turns, got %q then %q", id1, id2)
	}
}
