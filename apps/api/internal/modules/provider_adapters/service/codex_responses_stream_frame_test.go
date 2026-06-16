package service

import (
	"reflect"
	"testing"

	contract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// TestCodexResponsesStreamFrameParserMatchesBatch pins the additive per-frame
// Codex Responses parser to the authoritative batch parser
// (parseCodexResponsesStream) so the cross-protocol transcode stays faithful and
// the two cannot drift.
func TestCodexResponsesStreamFrameParserMatchesBatch(t *testing.T) {
	body := []byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello\"}\n\n" +
		"event: response.reasoning_text.delta\ndata: {\"type\":\"response.reasoning_text.delta\",\"delta\":\"thinking\"}\n\n" +
		"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\" world\"}\n\n" +
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"status\":\"completed\",\"usage\":{\"input_tokens\":3,\"output_tokens\":2}}}\n\n" +
		"data: [DONE]\n\n")

	batchResp, err := parseCodexResponsesStream(body, 200, codexResponsesParseOptions{})
	if err != nil {
		t.Fatalf("batch parse: %v", err)
	}

	frames, err := parseSSEFrames(body)
	if err != nil {
		t.Fatalf("parse frames: %v", err)
	}
	parser := newCodexResponsesStreamFrameParser()
	var got []contract.ConversationStreamEvent
	for _, frame := range frames {
		evs, done, feedErr := parser.FeedFrame(frame)
		if feedErr != nil {
			t.Fatalf("FeedFrame: %v", feedErr)
		}
		got = append(got, evs...)
		if done {
			break
		}
	}
	got = append(got, parser.Finalize()...)

	if !reflect.DeepEqual(got, batchResp.StreamEvents) {
		t.Fatalf("incremental codex responses parser != batch:\n inc=%+v\n batch=%+v", got, batchResp.StreamEvents)
	}
	if len(got) == 0 || got[len(got)-1].Type != contract.ConversationStreamEventStop {
		t.Fatalf("expected events ending in a Stop, got %+v", got)
	}
}
