package service

import (
	"net/http"
	"strings"
	"testing"

	contract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// TestCodexStreamMidCutSurfacesAsStreamInterrupted pins parseCodexResponsesBody
// to the CLIProxyAPI + sub2api contract: an upstream Codex SSE stream that
// ends WITHOUT a response.completed/.done/.failed terminal event was cut
// mid-flight (TCP reset, reverse-proxy buffer overflow, ctx cancel) and must
// surface as a typed stream_interrupted/502 ProviderError so the scheduler's
// failover classifier can retry on the next candidate account. Previously the
// default RequireTerminalEvent=false caused the parser to return a silent
// success built from whatever partial frames it had — the user observed
// "content stops mid-sentence with NO error to client" with nothing in logs.
func TestCodexStreamMidCutSurfacesAsStreamInterrupted(t *testing.T) {
	// Upstream sends a few output_text.delta frames then closes the stream
	// without emitting response.completed. [DONE] is the SSE end marker but
	// not a Codex terminal event.
	body := []byte(
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello \"}\n\n" +
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"world\"}\n\n" +
			"data: [DONE]\n\n",
	)
	_, err := parseCodexResponsesBody(body, 200)
	if err == nil {
		t.Fatal("expected stream_interrupted error, got nil")
	}
	providerErr, ok := err.(contract.ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T: %v", err, err)
	}
	if providerErr.Class != "stream_interrupted" || providerErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected stream_interrupted/502, got %+v", providerErr)
	}
	if !strings.Contains(providerErr.Message, "terminal event") {
		t.Fatalf("expected error message mentioning terminal event, got %q", providerErr.Message)
	}
}

// TestCodexStreamWithTerminalEventSucceeds is the positive counterpart: a
// well-formed Codex stream that ends with response.completed must parse
// successfully under the new strict default.
func TestCodexStreamWithTerminalEventSucceeds(t *testing.T) {
	body := []byte(
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello \"}\n\n" +
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"world\"}\n\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"status\":\"completed\",\"usage\":{\"input_tokens\":3,\"output_tokens\":2}}}\n\n" +
			"data: [DONE]\n\n",
	)
	resp, err := parseCodexResponsesBody(body, 200)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
}
