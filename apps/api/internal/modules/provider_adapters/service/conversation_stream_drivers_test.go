package service

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	contract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// TestAnthropicStreamFeedFrameMatchesBatch proves the per-frame Anthropic driver
// (FeedFrame + Finalize) produces exactly the same canonical stream events as
// the batch parser's inner loop — so cross-protocol incremental transcoding is
// faithful to the buffered path.
func TestAnthropicStreamFeedFrameMatchesBatch(t *testing.T) {
	body := []byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\",\"usage\":{\"input_tokens\":5}}}\n\n" +
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hi\"}}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	frames, err := parseSSEFrames(body)
	if err != nil {
		t.Fatalf("parse frames: %v", err)
	}

	inc := newAnthropicStreamParseState()
	var got []contract.ConversationStreamEvent
	for _, f := range frames {
		evs, done, err := inc.FeedFrame(f)
		if err != nil {
			t.Fatalf("FeedFrame: %v", err)
		}
		got = append(got, evs...)
		if done {
			break
		}
	}
	got = append(got, inc.Finalize()...)

	ref := newAnthropicStreamParseState()
	for _, f := range frames {
		data := strings.TrimSpace(f.Data)
		if data == "" || data == "[DONE]" {
			continue
		}
		var chunk anthropicStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		ct := f.EventType(chunk.Type)
		chunk.Type = ct
		if ref.handleAnthropicStreamEvent(data, ct, chunk) {
			break
		}
	}
	ref.streamEvents = appendAnthropicTerminalStopEvent(ref.streamEvents, ref.eventIndex, ref.stopReason)

	if !reflect.DeepEqual(got, ref.streamEvents) {
		t.Fatalf("incremental driver != batch loop:\n inc=%+v\n batch=%+v", got, ref.streamEvents)
	}
	if len(got) == 0 || got[len(got)-1].Type != contract.ConversationStreamEventStop {
		t.Fatalf("expected events ending in a Stop, got %+v", got)
	}
}

// TestOpenAIStreamFeedFrameMatchesBatch proves the same for the OpenAI Chat
// Completions per-frame driver.
func TestOpenAIStreamFeedFrameMatchesBatch(t *testing.T) {
	body := []byte("data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\" world\"},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n")
	frames, err := parseSSEFrames(body)
	if err != nil {
		t.Fatalf("parse frames: %v", err)
	}

	inc := newOpenAIStreamParseState()
	var got []contract.ConversationStreamEvent
	for _, f := range frames {
		evs, done, err := inc.FeedFrame(f)
		if err != nil {
			t.Fatalf("FeedFrame: %v", err)
		}
		got = append(got, evs...)
		if done {
			break
		}
	}
	got = append(got, inc.Finalize()...)

	ref := newOpenAIStreamParseState()
	for _, f := range frames {
		data := strings.TrimSpace(f.Data)
		if data == "" || data == "[DONE]" {
			continue
		}
		var chunk openAIChatCompletionStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		ref.handleOpenAIStreamChunk(data, chunk)
	}
	if len(ref.streamEvents) > 0 && ref.streamEvents[len(ref.streamEvents)-1].Type != contract.ConversationStreamEventStop {
		ref.streamEvents = append(ref.streamEvents, contract.ConversationStreamEvent{
			Index:          ref.eventIndex,
			Type:           contract.ConversationStreamEventStop,
			StopReason:     ref.stopReason,
			RawEventType:   "done",
			OriginProtocol: "openai-compatible",
		})
		ref.eventIndex++
	}

	if !reflect.DeepEqual(got, ref.streamEvents) {
		t.Fatalf("incremental driver != batch loop:\n inc=%+v\n batch=%+v", got, ref.streamEvents)
	}
	if len(got) == 0 || got[len(got)-1].Type != contract.ConversationStreamEventStop {
		t.Fatalf("expected events ending in a Stop, got %+v", got)
	}
}
