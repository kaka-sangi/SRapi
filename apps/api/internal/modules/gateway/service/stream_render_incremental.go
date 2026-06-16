package service

import (
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
)

// Incremental client-side stream renderers.
//
// Each renderer is the stateful form of the corresponding RenderXxxStreamEvents
// method: it is fed canonical gatewaycontract.StreamEvent values ONE AT A TIME
// via FeedEvent and flushed at end-of-stream via Finalize. This lets a
// cross-protocol stream reader transcode an upstream SSE stream into the
// client's protocol on the fly (real time-to-first-byte) instead of buffering
// the whole upstream response first.
//
// The existing slice-based Render* methods are kept as thin wrappers that feed
// the full event slice through the same renderer — so the buffered path and the
// incremental path are byte-identical by construction, and the existing render
// tests are the equivalence gate.

// ChatStreamRenderer is the incremental form of RenderChatStreamChunks. Its only
// mutable state is the tool-call index map; chunks are otherwise a pure function
// of each event.
type ChatStreamRenderer struct {
	svc     *Service
	resp    gatewaycontract.CanonicalResponse
	tools   *chatStreamToolCallIndexes
	emitted bool
}

func (s *Service) newChatStreamRenderer(resp gatewaycontract.CanonicalResponse) *ChatStreamRenderer {
	return &ChatStreamRenderer{svc: s, resp: resp, tools: newChatStreamToolCallIndexes()}
}

// FeedEvent renders the chat chunks produced by a single canonical event (zero
// or one chunk). It mirrors the switch body of the original loop exactly.
func (r *ChatStreamRenderer) FeedEvent(event gatewaycontract.StreamEvent) []map[string]any {
	resp := r.resp
	switch event.Type {
	case gatewaycontract.StreamEventContentDelta, gatewaycontract.StreamEventToolResult:
		if text := event.Delta.Text; text != "" {
			r.emitted = true
			return []map[string]any{chatStreamChunkWithIndex(resp, streamEventChoiceIndex(event), map[string]any{"content": text}, nil, nil)}
		}
	case gatewaycontract.StreamEventReasoning:
		if text := event.Delta.Text; text != "" {
			r.emitted = true
			return []map[string]any{chatStreamChunkWithIndex(resp, streamEventChoiceIndex(event), map[string]any{"reasoning_content": text}, nil, nil)}
		}
	case gatewaycontract.StreamEventToolCallDelta:
		if toolCall := chatStreamToolCallDelta(event, r.tools.indexFor(event)); toolCall != nil {
			r.emitted = true
			return []map[string]any{chatStreamChunkWithIndex(resp, streamEventChoiceIndex(event), map[string]any{"tool_calls": []map[string]any{toolCall}}, nil, nil)}
		}
	case gatewaycontract.StreamEventUsage:
		chunk := chatStreamChunk(resp, nil, nil, tokenUsage(event.Usage))
		chunk["choices"] = []map[string]any{}
		r.emitted = true
		return []map[string]any{chunk}
	case gatewaycontract.StreamEventStop:
		reason := firstNonEmpty(event.StopReason, resp.StopReason)
		r.emitted = true
		return []map[string]any{chatStreamChunkWithIndex(resp, streamEventChoiceIndex(event), map[string]any{}, openAIChatFinishReason(reason), nil)}
	}
	return nil
}

// Finalize emits the single-chunk fallback when the stream produced no chunks
// (no events, or no renderable events) — matching the original len(chunks)==0
// behavior.
func (r *ChatStreamRenderer) Finalize() []map[string]any {
	if !r.emitted {
		return []map[string]any{r.svc.RenderChatStreamChunk(r.resp)}
	}
	return nil
}
