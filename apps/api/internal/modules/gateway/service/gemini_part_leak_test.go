package service

import (
	"encoding/json"
	"strings"
	"testing"

	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
)

// TestOutputGeminiPartsDoNotLeakInternalType guards against the internal
// canonical marker "srapi_type" leaking into Gemini wire parts. Gemini types its
// parts structurally (text/functionCall/functionResponse), so the marker must
// never appear; a strict Gemini client would otherwise see an unknown field.
func TestOutputGeminiPartsDoNotLeakInternalType(t *testing.T) {
	blocks := []gatewaycontract.ContentBlock{
		{Type: gatewaycontract.ContentBlockText, Role: "assistant", Text: "pong"},
		{Type: gatewaycontract.ContentBlockToolCall, Role: "assistant", ToolName: "lookup", ToolCallID: "call_1", ToolArgumentsJSON: `{"q":"x"}`},
	}
	parts := outputGeminiParts(blocks)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	for i, part := range parts {
		raw, err := json.Marshal(part)
		if err != nil {
			t.Fatalf("marshal part %d: %v", i, err)
		}
		if strings.Contains(string(raw), "srapi_type") {
			t.Fatalf("part %d leaked srapi_type: %s", i, raw)
		}
	}
	if parts[0].Text == nil || *parts[0].Text != "pong" {
		t.Fatalf("expected text part 'pong', got %+v", parts[0])
	}
	if parts[1].FunctionCall == nil {
		t.Fatalf("expected function call part, got %+v", parts[1])
	}
}
