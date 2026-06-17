package translators

import (
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/translator"
)

// The claude_messages → openai_responses translator is currently an
// identity pass-through (see file comment in
// claude_messages_to_openai_responses.go for the rationale). These tests
// pin (a) the identity behaviour itself byte-for-byte, (b) the
// nil-safe fallthrough contract, and (c) the registry-mediated dispatch
// so a refactor cannot silently remove the registration.

func TestClaudeMessagesToOpenAIResponsesRewriterIdentityOnValidJSON(t *testing.T) {
	in := []byte(`{"model":"claude-3-5-sonnet","messages":[{"role":"user","content":"hi"}]}`)
	out := claudeMessagesToOpenAIResponsesRewriter("claude-3-5-sonnet", in, false)
	if string(out) != string(in) {
		t.Errorf("identity rewriter changed output:\n got  %s\n want %s", out, in)
	}
}

func TestClaudeMessagesToOpenAIResponsesRewriterFallsThroughOnNilEmpty(t *testing.T) {
	if got := claudeMessagesToOpenAIResponsesRewriter("m", nil, false); got != nil {
		t.Errorf("expected nil input to fall through to nil, got %q", got)
	}
	if got := claudeMessagesToOpenAIResponsesRewriter("m", []byte{}, false); len(got) != 0 {
		t.Errorf("expected empty input to fall through to empty, got %q", got)
	}
}

func TestClaudeMessagesToOpenAIResponsesRewriterFallsThroughOnMalformedJSON(t *testing.T) {
	in := []byte(`not-json-bytes`)
	out := claudeMessagesToOpenAIResponsesRewriter("m", in, false)
	if string(out) != string(in) {
		t.Errorf("expected fallthrough on bad JSON, got %q", out)
	}
}

func TestDefaultRegistryHasIdentityForClaudeMessagesToOpenAIResponses(t *testing.T) {
	r := translator.Default()
	in := []byte(`{"model":"claude-3-5-sonnet","messages":[{"role":"user","content":"hi"}],"stream":true}`)
	out := r.TranslateRequest(
		translator.FormatClaudeMessages,
		translator.FormatOpenAIResponses,
		"claude-3-5-sonnet", in, true,
	)
	if string(out) != string(in) {
		t.Errorf("registry-mediated identity changed output: %q != %q", out, in)
	}
}

func TestDefaultRegistryHasNoResponseTransformerForClaudeMessagesToOpenAIResponses(t *testing.T) {
	if translator.Default().HasResponseTransformer(
		translator.FormatClaudeMessages,
		translator.FormatOpenAIResponses,
	) {
		t.Error("response-side transformer should be nil for an identity registration")
	}
}
