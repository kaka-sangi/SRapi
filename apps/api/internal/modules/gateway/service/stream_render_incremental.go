package service

import (
	"sort"

	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
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

// ResponsesStreamRenderer is the incremental form of the OpenAI Responses
// renderer, used ONLY by the cross-protocol streaming reader (the hot buffered
// renderResponsesCanonicalStreamEvents path is left untouched). Cross-protocol
// upstreams (anthropic/gemini/openai-chat) never emit Responses lifecycle events,
// so this synthesizes response.created on the first event and builds the terminal
// response.completed from the accumulated output at Finalize.
type ResponsesStreamRenderer struct {
	svc             *Service
	resp            gatewaycontract.CanonicalResponse
	responseID      string
	sequence        *responsesStreamSequence
	nextOutputIndex int
	textStates      *responseStreamTextStates
	imageStates     *responseStreamImageStates
	toolStates      *streamToolCallStates
	started         bool

	pendingUsage      *gatewaycontract.Usage
	pendingStopReason string
	terminalEventName string
	terminalMetadata  map[string]any
}

func (s *Service) newResponsesStreamRenderer(resp gatewaycontract.CanonicalResponse) *ResponsesStreamRenderer {
	return &ResponsesStreamRenderer{
		svc:         s,
		resp:        resp,
		responseID:  responseID(resp),
		sequence:    newResponsesStreamSequence(nil),
		textStates:  newResponseStreamTextStates(nil),
		imageStates: newResponseStreamImageStates(nil),
		toolStates:  newStreamToolCallStates(nil),
	}
}

// NewResponsesStreamRenderer exposes the incremental Responses renderer to the
// HTTP layer for cross-protocol transcoding.
func (s *Service) NewResponsesStreamRenderer(resp gatewaycontract.CanonicalResponse) *ResponsesStreamRenderer {
	return s.newResponsesStreamRenderer(resp)
}

func (r *ResponsesStreamRenderer) ensureStarted(out []StreamEvent) []StreamEvent {
	if r.started {
		return out
	}
	r.started = true
	return append(out, r.sequence.apply(responseCreatedStreamEvent(r.resp, r.responseID)))
}

// FeedEvent renders the Responses SSE events produced by one canonical event.
func (r *ResponsesStreamRenderer) FeedEvent(event gatewaycontract.StreamEvent) []StreamEvent {
	out := r.ensureStarted(make([]StreamEvent, 0, 4))
	switch event.Type {
	case gatewaycontract.StreamEventContentDelta, gatewaycontract.StreamEventToolResult:
		out = r.feedText(out, event, responseStreamDeltaTextBlockType(event, gatewaycontract.ContentBlockText))
	case gatewaycontract.StreamEventReasoning:
		out = r.feedText(out, event, gatewaycontract.ContentBlockReasoning)
	case gatewaycontract.StreamEventToolCallDelta:
		state := r.toolStates.stateFor(event)
		if state.OutputIndex < 0 {
			state.OutputIndex = r.nextOutputIndex
			r.nextOutputIndex++
			out = append(out, r.sequence.apply(responseStreamToolCallStartEvent(state)))
		}
		if delta := event.Delta.ToolArgumentsJSON; delta != "" {
			state.Arguments.WriteString(delta)
			if shouldSuppressHostedWebSearchArgumentDelta(state, delta) {
				return out
			}
			out = append(out, r.sequence.apply(StreamEvent{
				Event: "response.function_call_arguments.delta",
				Data: map[string]any{
					"type":         "response.function_call_arguments.delta",
					"response_id":  r.responseID,
					"item_id":      state.ItemID,
					"output_index": state.OutputIndex,
					"delta":        delta,
				},
			}))
		}
	case gatewaycontract.StreamEventUsage:
		copied := event.Usage
		r.pendingUsage = &copied
	case gatewaycontract.StreamEventStop:
		r.pendingStopReason = firstNonEmpty(event.StopReason, r.pendingStopReason)
		if name := responsesRawTerminalEventName(event.RawEventType); name != "" {
			r.terminalEventName = name
			r.terminalMetadata = cloneMap(event.Metadata)
		}
	}
	return out
}

func (r *ResponsesStreamRenderer) feedText(out []StreamEvent, event gatewaycontract.StreamEvent, fallbackType gatewaycontract.ContentBlockType) []StreamEvent {
	delta := event.Delta.Text
	state := r.textStates.stateFor(event, fallbackType)
	state.mergeMetadata(event.Delta.Metadata)
	if delta == "" && len(event.Delta.Metadata) == 0 {
		return out
	}
	if state.OutputIndex < 0 {
		state.OutputIndex = r.nextOutputIndex
		r.nextOutputIndex++
		out = append(out, r.sequence.applyAll(responseStreamTextStartEvents(r.responseID, state.ItemID, state.OutputIndex, state.BlockType, state.Metadata))...)
	}
	if delta == "" {
		return out
	}
	state.Text.WriteString(delta)
	return append(out, r.sequence.apply(responseStreamTextDeltaEvent(r.responseID, state.ItemID, state.OutputIndex, state.BlockType, delta, state.Metadata)))
}

// Finalize emits the done-groups for every open block and the terminal
// response.completed event built from the accumulated output.
func (r *ResponsesStreamRenderer) Finalize() []StreamEvent {
	out := r.ensureStarted(make([]StreamEvent, 0))
	doneGroups := make([]responseStreamDoneEventGroup, 0)
	for _, state := range r.textStates.openStates() {
		text := state.Text.String()
		doneGroups = append(doneGroups, responseStreamDoneEventGroup{
			OutputIndex: state.OutputIndex,
			Events: []StreamEvent{
				responseStreamTextDoneEvent(r.responseID, state.ItemID, state.OutputIndex, state.BlockType, text, state.Metadata),
				responseStreamContentPartDoneEvent(r.responseID, state.ItemID, state.OutputIndex, state.BlockType, text, state.Metadata),
				responseStreamMessageDoneEvent(state.ItemID, state.OutputIndex, state.BlockType, text, state.Metadata),
			},
		})
	}
	for _, state := range r.toolStates.openStates() {
		arguments := state.Arguments.String()
		if arguments == "" {
			arguments = state.Block.ToolArgumentsJSON
		}
		if group, ok := hostedWebSearchStreamDoneGroup(state, arguments); ok {
			doneGroups = append(doneGroups, group)
			continue
		}
		doneGroups = append(doneGroups, responseStreamDoneEventGroup{
			OutputIndex: state.OutputIndex,
			Events: []StreamEvent{
				{
					Event: "response.function_call_arguments.done",
					Data: map[string]any{
						"type":         "response.function_call_arguments.done",
						"response_id":  r.responseID,
						"item_id":      state.ItemID,
						"output_index": state.OutputIndex,
						"arguments":    arguments,
					},
				},
				{
					Event: "response.output_item.done",
					Data: map[string]any{
						"type":         "response.output_item.done",
						"output_index": state.OutputIndex,
						"item":         responseStreamFunctionCallItem(state.ItemID, state.completedBlock(arguments)),
					},
				},
			},
		})
	}
	sortResponseStreamDoneEventGroups(doneGroups)
	for _, group := range doneGroups {
		out = append(out, r.sequence.applyAll(group.Events)...)
	}

	// Build the terminal response.completed from the accumulated output items,
	// since a streaming upstream never delivers a full Responses object.
	synthetic := r.resp
	synthetic.OutputItems = r.reconstructOutputItems()
	if r.pendingUsage != nil {
		synthetic.Usage = *r.pendingUsage
	}
	if r.pendingStopReason != "" {
		synthetic.StopReason = r.pendingStopReason
	}
	terminalEventName := responsesTerminalEventName(synthetic.StopReason)
	if r.terminalEventName != "" {
		terminalEventName = r.terminalEventName
	}
	terminalResponse := responseStreamTerminalResponsePayload(r.svc.RenderResponses(synthetic), terminalEventName, r.terminalMetadata)
	return append(out, r.sequence.apply(StreamEvent{
		Event: terminalEventName,
		Data: map[string]any{
			"type":     terminalEventName,
			"response": terminalResponse,
		},
	}))
}

// reconstructOutputItems rebuilds the canonical output blocks from the
// accumulated text/reasoning/tool states (in output-index order) so the terminal
// response can be rendered.
func (r *ResponsesStreamRenderer) reconstructOutputItems() []gatewaycontract.ContentBlock {
	type indexed struct {
		idx   int
		block gatewaycontract.ContentBlock
	}
	items := make([]indexed, 0)
	for _, state := range r.textStates.openStates() {
		items = append(items, indexed{state.OutputIndex, gatewaycontract.ContentBlock{
			Type:     state.BlockType,
			Text:     state.Text.String(),
			Metadata: state.Metadata,
		}})
	}
	for _, state := range r.toolStates.openStates() {
		arguments := state.Arguments.String()
		if arguments == "" {
			arguments = state.Block.ToolArgumentsJSON
		}
		items = append(items, indexed{state.OutputIndex, state.completedBlock(arguments)})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].idx < items[j].idx })
	blocks := make([]gatewaycontract.ContentBlock, 0, len(items))
	for _, item := range items {
		blocks = append(blocks, item.block)
	}
	return blocks
}

// GeminiStreamRenderer is the incremental form of renderGeminiCanonicalStreamEvents.
// It is stateless per-event: each canonical event maps to zero or one Gemini
// streamGenerateContent chunk.
type GeminiStreamRenderer struct {
	resp gatewaycontract.CanonicalResponse
}

func (s *Service) newGeminiStreamRenderer(resp gatewaycontract.CanonicalResponse) *GeminiStreamRenderer {
	return &GeminiStreamRenderer{resp: resp}
}

// NewGeminiStreamRenderer exposes the incremental Gemini renderer to the HTTP
// layer for cross-protocol transcoding.
func (s *Service) NewGeminiStreamRenderer(resp gatewaycontract.CanonicalResponse) *GeminiStreamRenderer {
	return s.newGeminiStreamRenderer(resp)
}

// FeedEvent renders the Gemini chunk(s) produced by one canonical event.
func (r *GeminiStreamRenderer) FeedEvent(event gatewaycontract.StreamEvent) []StreamEvent {
	switch event.Type {
	case gatewaycontract.StreamEventContentDelta, gatewaycontract.StreamEventReasoning, gatewaycontract.StreamEventToolCallDelta, gatewaycontract.StreamEventToolResult:
		part, ok := outputGeminiStreamPart(event.Delta)
		if !ok {
			return nil
		}
		return []StreamEvent{{Data: map[string]any{
			"candidates": []apiopenapi.GeminiCandidate{{
				Index: event.ContentIndex,
				Content: apiopenapi.GeminiContent{
					Parts: []apiopenapi.GeminiPart{part},
				},
			}},
		}}}
	case gatewaycontract.StreamEventUsage:
		return []StreamEvent{{Data: map[string]any{
			"candidates":    []apiopenapi.GeminiCandidate{},
			"usageMetadata": geminiUsage(event.Usage),
		}}}
	case gatewaycontract.StreamEventStop:
		return []StreamEvent{{Data: map[string]any{
			"candidates": []apiopenapi.GeminiCandidate{{
				Index:        event.ContentIndex,
				FinishReason: geminiFinishReason(firstNonEmpty(event.StopReason, r.resp.StopReason)),
				Content: apiopenapi.GeminiContent{
					Parts: []apiopenapi.GeminiPart{},
				},
			}},
		}}}
	}
	return nil
}

// Finalize emits no trailing chunk — Gemini's terminal is the finish_reason
// candidate carried by the stop event.
func (r *GeminiStreamRenderer) Finalize() []StreamEvent { return nil }

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
