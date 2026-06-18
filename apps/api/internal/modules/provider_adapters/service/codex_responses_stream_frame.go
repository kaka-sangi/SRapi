package service

import (
	"encoding/json"
	"net/http"
	"strings"

	contract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// codexResponsesStreamFrameParser is the per-frame, stream-events-only form of
// the Codex Responses SSE parser. It mirrors the canonical-stream-event
// generation of parseCodexResponsesStream one frame at a time, so a
// cross-protocol reader can transcode a Codex (Responses) upstream into the
// client's protocol incrementally.
//
// It is ADDITIVE: parseCodexResponsesStream (the authoritative usage/billing
// parser) is left untouched. An equivalence test pins this parser's events to
// the batch parser's StreamEvents so the two cannot drift.
type codexResponsesStreamFrameParser struct {
	deltaBuilder           strings.Builder
	reasoningBuilder       strings.Builder
	refusalBuilder         strings.Builder
	completedRefusal       string
	functionStates         *codexFunctionCallStreamStates
	textAnnotationsByIndex map[codexTextAnnotationKey][]map[string]any
	streamEvents           []contract.ConversationStreamEvent
	eventIndex             int
	seenRenderableEvent    bool
	stopEmitted            bool
}

func newCodexResponsesStreamFrameParser() *codexResponsesStreamFrameParser {
	return &codexResponsesStreamFrameParser{
		functionStates:         newCodexFunctionCallStreamStates(),
		textAnnotationsByIndex: map[codexTextAnnotationKey][]map[string]any{},
		streamEvents:           make([]contract.ConversationStreamEvent, 0),
	}
}

func (s *codexResponsesStreamFrameParser) append(event contract.ConversationStreamEvent) {
	event.Index = s.eventIndex
	if event.Type == contract.ConversationStreamEventStop {
		s.stopEmitted = true
	}
	s.streamEvents = append(s.streamEvents, event)
	s.eventIndex++
}

func (s *codexResponsesStreamFrameParser) FeedFrame(frame sseFrame) ([]contract.ConversationStreamEvent, bool, error) {
	data := strings.TrimSpace(frame.Data)
	if data == "" {
		return nil, false, nil
	}
	if data == "[DONE]" {
		return nil, true, nil
	}
	var event codexResponsesEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return nil, false, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid stream json"}
	}
	eventType := frame.EventType(event.Type)
	event.Type = eventType
	if providerErr, ok := codexEventProviderError(event); ok && eventType != "response.failed" {
		return nil, false, providerErr
	}
	before := len(s.streamEvents)
	if event.Usage != nil && event.Usage.HasTokenUsage() {
		s.append(codexStreamUsageEvent(*event.Usage, data, s.deltaBuilder.String()))
	}
	s.functionStates.mergeEvent(event)
	if event.Response != nil && event.Response.Usage.HasTokenUsage() {
		s.append(codexStreamUsageEvent(event.Response.Usage, data, s.deltaBuilder.String()))
	}
	switch eventType {
	case "response.created", "response.in_progress", "response.queued":
		if !s.seenRenderableEvent {
			s.append(codexMetadataStreamEvent(event, eventType, data))
		}
	case "response.output_item.added":
		s.seenRenderableEvent = true
		if streamEvent, ok := s.functionStates.startEvent(event, eventType, data); ok {
			s.append(streamEvent)
		}
	case "response.output_item.done":
		s.seenRenderableEvent = true
		if event.Item != nil {
			item := codexOutputItemWithStreamAnnotations(*event.Item, codexOutputIndex(event), s.textAnnotationsByIndex)
			if codexOutputItemIsFunctionCall(item) && !s.functionStates.hasArgumentDeltas(event) {
				if streamEvent, ok := codexFunctionCallStreamEvent(item, codexOutputIndex(event), data); ok {
					s.append(streamEvent)
				}
			}
		}
	case "response.image_generation_call.partial_image":
		s.seenRenderableEvent = true
		if streamEvent, ok := codexImageGenerationPartialStreamEvent(event, eventType, data); ok {
			s.append(streamEvent)
		}
	case "response.output_text.delta":
		s.seenRenderableEvent = true
		s.deltaBuilder.WriteString(event.Delta)
		if event.Delta != "" {
			s.append(codexContentStreamEvent(event, eventType, data, textContentDelta(event.Delta)))
		}
	case "response.output_text.annotation.added":
		s.seenRenderableEvent = true
		if len(event.Annotation) > 0 {
			key := codexTextAnnotationKeyForEvent(event)
			annotation := cloneMap(event.Annotation)
			s.textAnnotationsByIndex[key] = append(s.textAnnotationsByIndex[key], annotation)
			s.append(codexContentStreamEvent(event, eventType, data, codexAnnotationContentDelta(annotation)))
		}
	case "response.refusal.delta":
		s.seenRenderableEvent = true
		s.refusalBuilder.WriteString(event.Delta)
		if event.Delta != "" {
			s.append(codexContentStreamEvent(event, eventType, data, contract.ContentPart{
				Kind:           contract.ContentPartRefusal,
				Text:           event.Delta,
				OriginProtocol: "openai-compatible",
			}))
		}
	case "response.reasoning_text.delta", "response.reasoning_summary_text.delta":
		s.seenRenderableEvent = true
		s.reasoningBuilder.WriteString(event.Delta)
		if event.Delta != "" {
			s.append(codexReasoningStreamEvent(event, eventType, data))
		}
	case "response.function_call_arguments.delta":
		s.seenRenderableEvent = true
		if event.Delta != "" {
			s.append(s.functionStates.deltaEvent(event, eventType, data))
		}
	case "response.refusal.done":
		if strings.TrimSpace(event.Refusal) != "" {
			s.completedRefusal = event.Refusal
		}
	case "response.completed", "response.done", "response.incomplete", "response.cancelled", "response.canceled", "response.failed":
		if eventType != "response.failed" {
			if providerErr, ok := codexEventProviderError(event); ok {
				return nil, false, providerErr
			}
		}
		s.append(codexTerminalStreamEvent(event, eventType, data, s.completedRefusal, s.refusalBuilder.String()))
	}
	return cloneConversationStreamEventsTail(s.streamEvents, before), false, nil
}

func (s *codexResponsesStreamFrameParser) Finalize() []contract.ConversationStreamEvent {
	if s.stopEmitted {
		return nil
	}
	before := len(s.streamEvents)
	s.append(contract.ConversationStreamEvent{
		Type:           contract.ConversationStreamEventStop,
		StopReason:     contract.StopReasonEndTurn,
		RawEventType:   "done",
		OriginProtocol: "openai-compatible",
	})
	return cloneConversationStreamEventsTail(s.streamEvents, before)
}

type codexResponsesUpstreamStreamParser struct {
	state *codexResponsesStreamFrameParser
}

func (p *codexResponsesUpstreamStreamParser) FeedFrame(eventType, data string) ([]contract.ConversationStreamEvent, bool, error) {
	return p.state.FeedFrame(sseFrame{Event: eventType, Data: data})
}

func (p *codexResponsesUpstreamStreamParser) Finalize() []contract.ConversationStreamEvent {
	return p.state.Finalize()
}
