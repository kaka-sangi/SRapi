package service

import (
	"encoding/json"
	"net/http"
	"strings"

	contract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// Per-frame drivers for the upstream stream parsers. They drive the SAME state
// machines as the batch parsers (parseAnthropicCompatibleStream /
// parseOpenAICompatibleStream) one SSE frame at a time, returning the canonical
// ConversationStreamEvents that frame produced — so a cross-protocol reader can
// transcode an upstream stream into the client's protocol incrementally. The
// batch parsers remain the authoritative StreamParse billing hooks.

// NewUpstreamStreamParser returns a per-frame parser for the given upstream
// protocol, or (nil, false) if that protocol has no incremental parser yet (the
// caller then falls back to buffered streaming). The returned parser drives the
// same state machine as the buffered batch parser, one frame at a time.
func NewUpstreamStreamParser(protocol string) (contract.UpstreamStreamParser, bool) {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "anthropic-compatible", "anthropic":
		return &anthropicUpstreamStreamParser{state: newAnthropicStreamParseState()}, true
	case "openai-compatible", "openai":
		return &openAIUpstreamStreamParser{state: newOpenAIStreamParseState()}, true
	default:
		return nil, false
	}
}

type anthropicUpstreamStreamParser struct{ state *anthropicStreamParseState }

func (p *anthropicUpstreamStreamParser) FeedFrame(eventType, data string) ([]contract.ConversationStreamEvent, bool, error) {
	return p.state.FeedFrame(sseFrame{Event: eventType, Data: data})
}

func (p *anthropicUpstreamStreamParser) Finalize() []contract.ConversationStreamEvent {
	return p.state.Finalize()
}

type openAIUpstreamStreamParser struct{ state *openAIStreamParseState }

func (p *openAIUpstreamStreamParser) FeedFrame(eventType, data string) ([]contract.ConversationStreamEvent, bool, error) {
	return p.state.FeedFrame(sseFrame{Event: eventType, Data: data})
}

func (p *openAIUpstreamStreamParser) Finalize() []contract.ConversationStreamEvent {
	return p.state.Finalize()
}

// cloneConversationStreamEventsTail returns a copy of events[from:] (the events
// appended since a parser's pre-call length snapshot).
func cloneConversationStreamEventsTail(events []contract.ConversationStreamEvent, from int) []contract.ConversationStreamEvent {
	if from < 0 || from >= len(events) {
		return nil
	}
	return append([]contract.ConversationStreamEvent(nil), events[from:]...)
}

// FeedFrame processes one Anthropic SSE frame. done is true once message_stop is
// seen or a [DONE] sentinel arrives.
func (s *anthropicStreamParseState) FeedFrame(frame sseFrame) ([]contract.ConversationStreamEvent, bool, error) {
	data := strings.TrimSpace(frame.Data)
	if data == "" {
		return nil, false, nil
	}
	if data == "[DONE]" {
		return nil, true, nil
	}
	if providerErr, ok := providerErrorFromStreamFrame(frame, data, "anthropic-compatible"); ok {
		return nil, false, providerErr
	}
	var chunk anthropicStreamChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return nil, false, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid stream json"}
	}
	chunkType := frame.EventType(chunk.Type)
	chunk.Type = chunkType
	before := len(s.streamEvents)
	done := s.handleAnthropicStreamEvent(data, chunkType, chunk)
	return cloneConversationStreamEventsTail(s.streamEvents, before), done, nil
}

// Finalize appends the terminal stop event (mirroring the post-loop step of
// parseAnthropicCompatibleStream) and returns it.
func (s *anthropicStreamParseState) Finalize() []contract.ConversationStreamEvent {
	before := len(s.streamEvents)
	s.streamEvents = appendAnthropicTerminalStopEvent(s.streamEvents, s.eventIndex, s.stopReason)
	return cloneConversationStreamEventsTail(s.streamEvents, before)
}

// FeedFrame processes one OpenAI Chat Completions SSE frame. OpenAI emits its
// stop event inline on finish_reason, so done is signalled only by [DONE].
func (s *openAIStreamParseState) FeedFrame(frame sseFrame) ([]contract.ConversationStreamEvent, bool, error) {
	data := strings.TrimSpace(frame.Data)
	if data == "" {
		return nil, false, nil
	}
	if data == "[DONE]" {
		return nil, true, nil
	}
	if providerErr, ok := providerErrorFromStreamFrame(frame, data, "openai-compatible"); ok {
		return nil, false, providerErr
	}
	var chunk openAIChatCompletionStreamChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return nil, false, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid stream json"}
	}
	before := len(s.streamEvents)
	s.handleOpenAIStreamChunk(data, chunk)
	return cloneConversationStreamEventsTail(s.streamEvents, before), false, nil
}

// Finalize appends a terminal stop event if the stream did not already end with
// one (mirroring openAIStreamResponse).
func (s *openAIStreamParseState) Finalize() []contract.ConversationStreamEvent {
	before := len(s.streamEvents)
	if len(s.streamEvents) > 0 && s.streamEvents[len(s.streamEvents)-1].Type != contract.ConversationStreamEventStop {
		s.streamEvents = append(s.streamEvents, contract.ConversationStreamEvent{
			Index:          s.eventIndex,
			Type:           contract.ConversationStreamEventStop,
			StopReason:     s.stopReason,
			RawEventType:   "done",
			OriginProtocol: "openai-compatible",
		})
		s.eventIndex++
	}
	return cloneConversationStreamEventsTail(s.streamEvents, before)
}
