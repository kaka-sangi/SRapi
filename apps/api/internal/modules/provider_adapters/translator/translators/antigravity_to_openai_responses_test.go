package translators

import (
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/translator"
)

// The antigravity → openai_responses translator is an explicit
// identity (see file comment for rationale — antigravityPayload owns
// the canonical envelope synthesis inline). These tests pin the
// identity behaviour byte-for-byte and prove the registry dispatches
// to it.

func TestAntigravityToOpenAIResponsesRewriterIdentityOnValidJSON(t *testing.T) {
	in := []byte(`{"project":"p","requestId":"r","userAgent":"antigravity","requestType":"agent","model":"gemini-2.5","request":{"contents":[]}}`)
	out := antigravityToOpenAIResponsesRewriter("gemini-2.5", in, false)
	if string(out) != string(in) {
		t.Errorf("identity rewriter changed output:\n got  %s\n want %s", out, in)
	}
}

func TestAntigravityToOpenAIResponsesRewriterFallsThroughOnNilEmpty(t *testing.T) {
	if got := antigravityToOpenAIResponsesRewriter("m", nil, false); got != nil {
		t.Errorf("expected nil input to fall through to nil, got %q", got)
	}
	if got := antigravityToOpenAIResponsesRewriter("m", []byte{}, false); len(got) != 0 {
		t.Errorf("expected empty input to fall through to empty, got %q", got)
	}
}

func TestAntigravityToOpenAIResponsesRewriterFallsThroughOnMalformedJSON(t *testing.T) {
	in := []byte(`}not-json{`)
	out := antigravityToOpenAIResponsesRewriter("m", in, false)
	if string(out) != string(in) {
		t.Errorf("expected fallthrough on bad JSON, got %q", out)
	}
}

func TestDefaultRegistryHasIdentityForAntigravityToOpenAIResponses(t *testing.T) {
	r := translator.Default()
	in := []byte(`{"project":"p","requestType":"image_gen","model":"gemini-2.5","request":{"contents":[]}}`)
	out := r.TranslateRequest(
		translator.FormatAntigravity,
		translator.FormatOpenAIResponses,
		"gemini-2.5", in, false,
	)
	if string(out) != string(in) {
		t.Errorf("registry-mediated identity changed output: %q != %q", out, in)
	}
}

func TestDefaultRegistryHasNoResponseTransformerForAntigravityToOpenAIResponses(t *testing.T) {
	if translator.Default().HasResponseTransformer(
		translator.FormatAntigravity,
		translator.FormatOpenAIResponses,
	) {
		t.Error("response-side transformer should be nil for an identity registration")
	}
}
