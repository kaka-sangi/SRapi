package service

import (
	"testing"

	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
)

// TestChatStreamRendererIncrementalContract locks the incremental renderer
// contract that the cross-protocol reader depends on: FeedEvent yields the
// chunk(s) for one event, each renderer instance keeps its own tool-call index
// state (no bleed across streams), and Finalize emits the fallback only when the
// stream produced nothing.
func TestChatStreamRendererIncrementalContract(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	resp := gatewaycontract.CanonicalResponse{Model: "m"}

	// A content delta yields exactly one content chunk.
	r1 := svc.newChatStreamRenderer(resp)
	got := r1.FeedEvent(gatewaycontract.StreamEvent{Type: gatewaycontract.StreamEventContentDelta, Delta: gatewaycontract.ContentBlock{Text: "hi"}})
	if d := chatChunkDelta(got); d["content"] != "hi" {
		t.Fatalf("content event should yield one content chunk, got %+v", got)
	}

	// A responses-style tool call gets index 0 on a fresh renderer.
	toolEvt := gatewaycontract.StreamEvent{Type: gatewaycontract.StreamEventToolCallDelta, RawEventType: "response.output_item.added", ContentIndex: 3, Delta: gatewaycontract.ContentBlock{ToolCallID: "c1", ToolName: "f"}}
	if tc := firstChatToolCall(t, r1.FeedEvent(toolEvt)); tc["index"] != 0 {
		t.Fatalf("first tool index should be 0, got %v", tc["index"])
	}

	// A SECOND fresh renderer must restart tool indices at 0 — no state bleed.
	r2 := svc.newChatStreamRenderer(resp)
	if tc := firstChatToolCall(t, r2.FeedEvent(toolEvt)); tc["index"] != 0 {
		t.Fatalf("a fresh renderer must reset tool index to 0, got %v", tc["index"])
	}

	// Finalize on a renderer that emitted nothing yields one fallback chunk.
	if fb := svc.newChatStreamRenderer(resp).Finalize(); len(fb) != 1 {
		t.Fatalf("empty-stream Finalize should yield one fallback chunk, got %+v", fb)
	}
	// Finalize after emitting yields nothing extra.
	if fb := r1.Finalize(); len(fb) != 0 {
		t.Fatalf("Finalize after emitting should yield no extra chunk, got %+v", fb)
	}
}

func chatChunkDelta(chunks []map[string]any) map[string]any {
	if len(chunks) != 1 {
		return nil
	}
	choices, _ := chunks[0]["choices"].([]map[string]any)
	if len(choices) == 0 {
		return nil
	}
	d, _ := choices[0]["delta"].(map[string]any)
	return d
}

func firstChatToolCall(t *testing.T, chunks []map[string]any) map[string]any {
	t.Helper()
	d := chatChunkDelta(chunks)
	tcs, _ := d["tool_calls"].([]map[string]any)
	if len(tcs) != 1 {
		t.Fatalf("expected one tool_call, got %+v", d)
	}
	return tcs[0]
}
