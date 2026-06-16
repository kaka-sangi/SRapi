package service

import (
	"testing"

	contract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// TestGeminiThoughtPartBecomesThinking guards that a Gemini response part marked
// thought:true is surfaced as a thinking block (carrying its thoughtSignature),
// not leaked to the client as visible assistant text — and that an ordinary text
// part is unaffected.
func TestGeminiThoughtPartBecomesThinking(t *testing.T) {
	got, ok := geminiContentPart(geminiPart{Text: "let me think", Thought: true, ThoughtSignature: "sig_abc"})
	if !ok || got.Kind != contract.ContentPartThinking {
		t.Fatalf("thought part must become a thinking block, got kind=%q ok=%v", got.Kind, ok)
	}
	if got.Text != "let me think" {
		t.Fatalf("thinking text = %q, want %q", got.Text, "let me think")
	}
	if got.Metadata["signature"] != "sig_abc" {
		t.Fatalf("thinking block must carry the thoughtSignature, got metadata %v", got.Metadata)
	}

	// An empty-text thought part that only carries a signature still emits a
	// thinking block so the orphan signature can be passed back next turn.
	orphan, ok := geminiContentPart(geminiPart{Thought: true, ThoughtSignature: "sig_only"})
	if !ok || orphan.Kind != contract.ContentPartThinking || orphan.Metadata["signature"] != "sig_only" {
		t.Fatalf("orphan-signature thought must emit a thinking block, got kind=%q ok=%v meta=%v", orphan.Kind, ok, orphan.Metadata)
	}

	// A plain text part is unchanged.
	plain, ok := geminiContentPart(geminiPart{Text: "hello"})
	if !ok || plain.Kind != contract.ContentPartText {
		t.Fatalf("plain text part must stay text, got kind=%q ok=%v", plain.Kind, ok)
	}
}

// TestGeminiToolResultRecoversFunctionName guards that a tool_result with no
// explicit name recovers the function name from the matching tool_use call id,
// since Gemini's functionResponse must carry the name to correlate the call.
func TestGeminiToolResultRecoversFunctionName(t *testing.T) {
	req := contract.ConversationRequest{
		Messages: []contract.ConversationMessage{
			{Role: "assistant", Parts: []contract.ContentPart{
				{Kind: contract.ContentPartToolUse, ToolCallID: "call_x", ToolName: "get_weather", ToolArgumentsJSON: `{"city":"SF"}`},
			}},
			{Role: "tool", Parts: []contract.ContentPart{
				{Kind: contract.ContentPartToolResult, ToolResultForID: "call_x", Text: `{"temp":20}`},
			}},
		},
	}
	contents := geminiCompatibleContents(req)
	var gotName string
	for _, c := range contents {
		for _, p := range c.Parts {
			if len(p.FunctionResponse) > 0 {
				gotName, _ = p.FunctionResponse["name"].(string)
			}
		}
	}
	if gotName != "get_weather" {
		t.Fatalf("tool_result functionResponse must recover the function name from the call id, got %q", gotName)
	}
}
