package service

import (
	"testing"

	contract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// TestCodexNonStreamReasoningSummarySurfacesAsThinking guards the non-streaming
// Responses path: a reasoning output item carries its chain-of-thought in
// summary:[{type:summary_text,text:...}], not in a top-level text field. The
// parser must assemble those summary parts into a thinking content part, exactly
// as the streaming path accumulates reasoning_summary_text deltas; otherwise the
// model's reasoning is silently dropped on non-stream responses.
func TestCodexNonStreamReasoningSummarySurfacesAsThinking(t *testing.T) {
	body := []byte(`{"id":"resp_x","status":"completed","output":[` +
		`{"type":"reasoning","summary":[{"type":"summary_text","text":"step one"},{"type":"summary_text","text":"step two"}]},` +
		`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"final answer"}]}` +
		`]}`)
	resp, err := parseCodexResponsesBody(body, 200)
	if err != nil {
		t.Fatalf("parse codex responses body: %v", err)
	}
	var thinking, text string
	for _, p := range resp.Parts {
		switch p.Kind {
		case contract.ContentPartThinking:
			thinking = p.Text
		case contract.ContentPartText:
			text = p.Text
		}
	}
	if thinking != "step one\nstep two" {
		t.Fatalf("reasoning summary must surface as thinking, got %q (parts=%+v)", thinking, resp.Parts)
	}
	if text != "final answer" {
		t.Fatalf("message text must survive alongside reasoning, got %q", text)
	}
}
