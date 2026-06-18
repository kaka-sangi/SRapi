package service

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestCodexExtractFinalResponseReturnsTerminalResponse pins the verbatim
// port of sub2api extractCodexFinalResponse (openai_gateway_service.go:
// 5329). When the SSE body has a response.completed event, the helper
// returns its `response` field as JSON.
func TestCodexExtractFinalResponseReturnsTerminalResponse(t *testing.T) {
	body := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"r1\"}}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"cmp_x\",\"object\":\"response.compaction\",\"output\":[],\"input_tokens\":7,\"output_tokens\":1}}\n\n" +
		"data: [DONE]\n\n"
	extracted, ok := codexExtractFinalResponse(body)
	if !ok {
		t.Fatalf("expected ok=true, got false")
	}
	var payload struct {
		ID           string `json:"id"`
		Object       string `json:"object"`
		InputTokens  int    `json:"input_tokens"`
		OutputTokens int    `json:"output_tokens"`
	}
	if err := json.Unmarshal(extracted, &payload); err != nil {
		t.Fatalf("invalid JSON: %v (raw=%q)", err, string(extracted))
	}
	if payload.ID != "cmp_x" || payload.Object != "response.compaction" ||
		payload.InputTokens != 7 || payload.OutputTokens != 1 {
		t.Fatalf("unexpected: %+v", payload)
	}
}

// TestCodexReconstructResponseOutputFromSSEAccumulatesText pins sub2api
// BufferedResponseAccumulator.ProcessEvent (responses_to_chatcompletions
// .go:449): response.output_text.delta events accumulate, BuildOutput
// yields a message item with output_text content. This is exactly what
// codex CLI's compact parser needs to find a `text` field.
func TestCodexReconstructResponseOutputFromSSEAccumulatesText(t *testing.T) {
	body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"summary \"}\n\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"text.\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"cmp\",\"output\":[]}}\n\n"
	rebuilt, ok := codexReconstructResponseOutputFromSSE(body)
	if !ok {
		t.Fatalf("expected reconstruction, got ok=false")
	}
	var output []map[string]any
	if err := json.Unmarshal(rebuilt, &output); err != nil {
		t.Fatalf("invalid output JSON: %v (raw=%q)", err, string(rebuilt))
	}
	if len(output) != 1 {
		t.Fatalf("expected 1 output item (message), got %d (%+v)", len(output), output)
	}
	msg := output[0]
	if msg["type"] != "message" || msg["role"] != "assistant" {
		t.Fatalf("expected message role=assistant, got %+v", msg)
	}
	content, ok := msg["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("expected one content part, got %+v", msg["content"])
	}
	part := content[0].(map[string]any)
	if part["type"] != "output_text" || part["text"] != "summary text." {
		t.Fatalf("expected output_text with concatenated deltas, got %+v", part)
	}
}

// TestCodexReconstructResponseOutputFromSSEAccumulatesReasoning pins
// the reasoning_summary_text path. Codex sometimes ships the compact
// summary via reasoning_summary_text deltas instead of output_text.
// BuildOutput should emit a reasoning item first when those are present.
func TestCodexReconstructResponseOutputFromSSEAccumulatesReasoning(t *testing.T) {
	body := "data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"Reasoning \"}\n\n" +
		"data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"summary.\"}\n\n"
	rebuilt, ok := codexReconstructResponseOutputFromSSE(body)
	if !ok {
		t.Fatalf("expected ok=true, got false")
	}
	var output []map[string]any
	_ = json.Unmarshal(rebuilt, &output)
	if len(output) != 1 || output[0]["type"] != "reasoning" {
		t.Fatalf("expected reasoning item, got %+v", output)
	}
	summary, ok := output[0]["summary"].([]any)
	if !ok || len(summary) != 1 {
		t.Fatalf("expected summary array, got %+v", output[0]["summary"])
	}
	part := summary[0].(map[string]any)
	if part["type"] != "summary_text" || part["text"] != "Reasoning summary." {
		t.Fatalf("expected summary_text concatenation, got %+v", part)
	}
}

// TestCodexConvertCompactSSEBodyToJSONInjectsReconstructedOutput is the
// END-TO-END regression for the live "missing field text at column N"
// failure. The upstream's terminal event has output=[] and the summary
// text is in delta events. After conversion the body MUST contain the
// message + output_text content block so codex CLI's parser finds text.
func TestCodexConvertCompactSSEBodyToJSONInjectsReconstructedOutput(t *testing.T) {
	body := []byte(
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"compact \"}\n\n" +
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"summary\"}\n\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"cmp_done\",\"object\":\"response.compaction\",\"output\":[],\"input_tokens\":12,\"output_tokens\":3}}\n\n" +
			"data: [DONE]\n\n",
	)
	rewritten, ok := codexConvertCompactSSEBodyToJSON(body)
	if !ok {
		t.Fatalf("expected conversion ok=true, got false")
	}
	if bytes.Contains(rewritten, []byte("data:")) {
		t.Fatalf("rewritten body must not contain SSE markers, got %q", string(rewritten))
	}
	var payload struct {
		ID           string           `json:"id"`
		Object       string           `json:"object"`
		Output       []map[string]any `json:"output"`
		InputTokens  int              `json:"input_tokens"`
		OutputTokens int              `json:"output_tokens"`
	}
	if err := json.Unmarshal(rewritten, &payload); err != nil {
		t.Fatalf("rewritten body must be valid JSON, got %q (err=%v)", string(rewritten), err)
	}
	if payload.ID != "cmp_done" || payload.Object != "response.compaction" {
		t.Fatalf("compact metadata must survive, got %+v", payload)
	}
	if payload.InputTokens != 12 || payload.OutputTokens != 3 {
		t.Fatalf("token counts must survive, got %+v", payload)
	}
	if len(payload.Output) != 1 {
		t.Fatalf("output[] must be reconstructed with one message item, got %+v", payload.Output)
	}
	msg := payload.Output[0]
	if msg["type"] != "message" {
		t.Fatalf("first output item must be a message, got %+v", msg)
	}
	content := msg["content"].([]any)
	part := content[0].(map[string]any)
	if part["text"] != "compact summary" {
		t.Fatalf("output[0].content[0].text must be the reconstructed delta text, got %v", part["text"])
	}
}

// TestCodexConvertCompactSSEBodyToJSONLeavesPopulatedOutputAlone covers
// the no-op path: when the terminal event already has output[]
// populated, the reconstruction is skipped (sub2api parity
// handlePassthroughSSEToJSON line 4018: "if len(output) == 0").
func TestCodexConvertCompactSSEBodyToJSONLeavesPopulatedOutputAlone(t *testing.T) {
	body := []byte(
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"ignored\"}\n\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"cmp\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"existing\"}]}]}}\n\n",
	)
	rewritten, ok := codexConvertCompactSSEBodyToJSON(body)
	if !ok {
		t.Fatalf("expected ok=true, got false")
	}
	if !strings.Contains(string(rewritten), `"text":"existing"`) {
		t.Fatalf("existing output text must survive, got %q", string(rewritten))
	}
	if strings.Contains(string(rewritten), `"text":"ignored"`) {
		t.Fatalf("delta text must NOT be merged into populated output, got %q", string(rewritten))
	}
}

func TestCodexConvertCompactSSEBodyToJSONRepairsTextlessOutput(t *testing.T) {
	body := []byte(
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"fixed \"}\n\n" +
			"data: {\"type\":\"response.output_text.done\",\"text\":\"fixed text\"}\n\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"cmp\",\"object\":\"response.compaction\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\"}]}]}}\n\n",
	)
	rewritten, ok := codexConvertCompactSSEBodyToJSON(body)
	if !ok {
		t.Fatalf("expected ok=true, got false")
	}
	var payload struct {
		Output []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(rewritten, &payload); err != nil {
		t.Fatalf("rewritten body must be valid JSON, got %q (err=%v)", string(rewritten), err)
	}
	if len(payload.Output) != 1 || len(payload.Output[0].Content) != 1 {
		t.Fatalf("expected one repaired output_text content part, got %+v", payload.Output)
	}
	part := payload.Output[0].Content[0]
	if part.Type != "output_text" || part.Text != "fixed text" {
		t.Fatalf("expected repaired output_text text, got %+v in %s", part, string(rewritten))
	}
}

// TestCodexConvertCompactSSEBodyToJSONReturnsFalseOnPureJSON guards
// the gate: when the body is already JSON (not SSE), the helper
// returns ok=false so the caller skips conversion. The gateway then
// passes the JSON through verbatim.
func TestCodexConvertCompactSSEBodyToJSONReturnsFalseOnPureJSON(t *testing.T) {
	body := []byte(`{"id":"cmp","object":"response.compaction","text":"already json"}`)
	if rewritten, ok := codexConvertCompactSSEBodyToJSON(body); ok {
		t.Fatalf("expected ok=false on JSON, got %q", string(rewritten))
	}
}

// TestCodexConvertCompactSSEBodyToJSONReturnsFalseWithoutTerminalEvent
// covers the SSE-without-terminal path: when the upstream stream cut
// off before response.completed, conversion fails so the caller can
// pass the partial SSE through (matches sub2api fallback at
// openai_gateway_service.go:4050).
func TestCodexConvertCompactSSEBodyToJSONReturnsFalseWithoutTerminalEvent(t *testing.T) {
	body := []byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
	if _, ok := codexConvertCompactSSEBodyToJSON(body); ok {
		t.Fatalf("expected ok=false without terminal event")
	}
}
