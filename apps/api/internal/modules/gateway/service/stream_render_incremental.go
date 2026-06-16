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

// NewChatStreamRenderer exposes the incremental chat renderer to the HTTP layer
// for cross-protocol transcoding. resp seeds the chunk id/model; per-event state
// is the renderer's own.
func (s *Service) NewChatStreamRenderer(resp gatewaycontract.CanonicalResponse) *ChatStreamRenderer {
	return s.newChatStreamRenderer(resp)
}

// NewAnthropicStreamRenderer exposes the incremental Anthropic renderer to the
// HTTP layer for cross-protocol transcoding.
func (s *Service) NewAnthropicStreamRenderer(resp gatewaycontract.CanonicalResponse) *AnthropicStreamRenderer {
	return s.newAnthropicStreamRenderer(resp)
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

// anthropicBlockKey identifies one logical Anthropic content block by the
// provider's content index AND the block kind, so distinct kinds at the same
// provider index never collide into one block.
type anthropicBlockKey struct {
	contentIndex int
	kind         gatewaycontract.ContentBlockType
}

// AnthropicStreamRenderer is the incremental form of
// renderAnthropicCanonicalStreamEvents. It emits message_start lazily on the
// first event, assigns monotonic block indices per (contentIndex, kind), and
// emits content_block_stop(s) + message_delta + message_stop on Finalize.
type AnthropicStreamRenderer struct {
	resp              gatewaycontract.CanonicalResponse
	started           bool
	blockIndexByKey   map[anthropicBlockKey]int
	openBlockOrder    []int
	nextBlockIndex    int
	toolStates        *streamToolCallStates
	textStates        *responseStreamTextStates
	pendingUsage      *gatewaycontract.Usage
	pendingStopReason string
}

func (s *Service) newAnthropicStreamRenderer(resp gatewaycontract.CanonicalResponse) *AnthropicStreamRenderer {
	return &AnthropicStreamRenderer{
		resp:            resp,
		blockIndexByKey: map[anthropicBlockKey]int{},
		openBlockOrder:  make([]int, 0),
		toolStates:      newStreamToolCallStates(resp.OutputItems),
		textStates:      newResponseStreamTextStates(resp.OutputItems),
	}
}

func (r *AnthropicStreamRenderer) blockIndexFor(contentIndex int, kind gatewaycontract.ContentBlockType) (int, bool) {
	key := anthropicBlockKey{contentIndex: contentIndex, kind: kind}
	if idx, ok := r.blockIndexByKey[key]; ok {
		return idx, false
	}
	idx := r.nextBlockIndex
	r.nextBlockIndex++
	r.blockIndexByKey[key] = idx
	return idx, true
}

func (r *AnthropicStreamRenderer) messageStartEvent() StreamEvent {
	return StreamEvent{
		Event: "message_start",
		Data: map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id":            "msg_" + responseID(r.resp),
				"type":          "message",
				"role":          "assistant",
				"model":         r.resp.Model,
				"content":       []any{},
				"stop_reason":   nil,
				"stop_sequence": nil,
				"usage":         anthropicStreamUsage(ptrInt(r.resp.Usage.InputTokens), ptrInt(0), r.resp.Usage.CachedTokens),
			},
		},
	}
}

// FeedEvent renders the Anthropic SSE events produced by one canonical event.
// message_start is emitted exactly once, before any block output.
func (r *AnthropicStreamRenderer) FeedEvent(event gatewaycontract.StreamEvent) []StreamEvent {
	out := make([]StreamEvent, 0, 2)
	if !r.started {
		r.started = true
		out = append(out, r.messageStartEvent())
	}
	switch event.Type {
	case gatewaycontract.StreamEventContentDelta, gatewaycontract.StreamEventReasoning, gatewaycontract.StreamEventToolCallDelta, gatewaycontract.StreamEventToolResult:
		if event.Type == gatewaycontract.StreamEventReasoning && mapStringAny(event.Delta.Metadata, "signature_delta") != "" {
			signature := r.textStates.appendSignature(event)
			blockIdx, isNew := r.blockIndexFor(event.ContentIndex, gatewaycontract.ContentBlockReasoning)
			if isNew {
				r.openBlockOrder = append(r.openBlockOrder, blockIdx)
				out = append(out, StreamEvent{
					Event: "content_block_start",
					Data: map[string]any{
						"type":          "content_block_start",
						"index":         blockIdx,
						"content_block": anthropicStreamContentBlock(anthropicStreamEventStartBlock(event)),
					},
				})
			}
			out = append(out, StreamEvent{
				Event: "content_block_delta",
				Data: map[string]any{
					"type":  "content_block_delta",
					"index": blockIdx,
					"delta": map[string]any{
						"type":      "signature_delta",
						"signature": signature,
					},
				},
			})
			return out
		}
		if event.Type == gatewaycontract.StreamEventReasoning {
			_ = r.textStates.stateFor(event, gatewaycontract.ContentBlockReasoning)
		}
		startBlock := anthropicStreamEventStartBlock(event)
		if event.Type == gatewaycontract.StreamEventToolCallDelta {
			state := r.toolStates.stateFor(event)
			startBlock = state.startBlock()
		}
		blockIdx, isNew := r.blockIndexFor(event.ContentIndex, startBlock.Type)
		if isNew {
			r.openBlockOrder = append(r.openBlockOrder, blockIdx)
			out = append(out, StreamEvent{
				Event: "content_block_start",
				Data: map[string]any{
					"type":          "content_block_start",
					"index":         blockIdx,
					"content_block": anthropicStreamContentBlock(startBlock),
				},
			})
		}
		if delta := anthropicStreamEventDelta(event); len(delta) > 0 {
			out = append(out, StreamEvent{
				Event: "content_block_delta",
				Data: map[string]any{
					"type":  "content_block_delta",
					"index": blockIdx,
					"delta": delta,
				},
			})
		}
	case gatewaycontract.StreamEventUsage:
		copied := event.Usage
		r.pendingUsage = &copied
	case gatewaycontract.StreamEventStop:
		r.pendingStopReason = firstNonEmpty(event.StopReason, r.pendingStopReason)
	}
	return out
}

// Finalize closes every open block and emits message_delta (stop_reason + final
// output tokens) and message_stop.
func (r *AnthropicStreamRenderer) Finalize() []StreamEvent {
	out := make([]StreamEvent, 0, len(r.openBlockOrder)+2)
	if !r.started {
		r.started = true
		out = append(out, r.messageStartEvent())
	}
	for _, index := range r.openBlockOrder {
		out = append(out, StreamEvent{
			Event: "content_block_stop",
			Data: map[string]any{
				"type":  "content_block_stop",
				"index": index,
			},
		})
	}
	if r.pendingUsage != nil || r.pendingStopReason != "" {
		outputTokens := r.resp.Usage.OutputTokens
		if r.pendingUsage != nil {
			outputTokens = r.pendingUsage.OutputTokens
		}
		delta := map[string]any{}
		if r.pendingStopReason != "" {
			delta["stop_reason"] = anthropicStopReason(firstNonEmpty(r.pendingStopReason, r.resp.StopReason))
			delta["stop_sequence"] = nil
		}
		out = append(out, StreamEvent{
			Event: "message_delta",
			Data: map[string]any{
				"type":  "message_delta",
				"delta": delta,
				"usage": anthropicStreamUsage(nil, ptrInt(outputTokens), 0),
			},
		})
	}
	out = append(out, StreamEvent{
		Event: "message_stop",
		Data: map[string]any{
			"type": "message_stop",
		},
	})
	return out
}
