package translators

import (
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/translator"
)

// The gemini_request → openai_responses translator is an explicit
// identity pass-through (see file comment for rationale). These tests
// pin the identity behaviour byte-for-byte and prove the registry
// dispatches to it.

func TestGeminiRequestToOpenAIResponsesRewriterIdentityOnValidJSON(t *testing.T) {
	in := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	out := geminiRequestToOpenAIResponsesRewriter("gemini-2.5-flash", in, false)
	if string(out) != string(in) {
		t.Errorf("identity rewriter changed output:\n got  %s\n want %s", out, in)
	}
}

func TestGeminiRequestToOpenAIResponsesRewriterFallsThroughOnNilEmpty(t *testing.T) {
	if got := geminiRequestToOpenAIResponsesRewriter("m", nil, false); got != nil {
		t.Errorf("expected nil input to fall through to nil, got %q", got)
	}
	if got := geminiRequestToOpenAIResponsesRewriter("m", []byte{}, false); len(got) != 0 {
		t.Errorf("expected empty input to fall through to empty, got %q", got)
	}
}

func TestGeminiRequestToOpenAIResponsesRewriterFallsThroughOnMalformedJSON(t *testing.T) {
	in := []byte(`{not-json`)
	out := geminiRequestToOpenAIResponsesRewriter("m", in, false)
	if string(out) != string(in) {
		t.Errorf("expected fallthrough on bad JSON, got %q", out)
	}
}

func TestDefaultRegistryHasIdentityForGeminiRequestToOpenAIResponses(t *testing.T) {
	r := translator.Default()
	in := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"generationConfig":{"temperature":0.5}}`)
	out := r.TranslateRequest(
		translator.FormatGeminiRequest,
		translator.FormatOpenAIResponses,
		"gemini-2.5-flash", in, false,
	)
	if string(out) != string(in) {
		t.Errorf("registry-mediated identity changed output: %q != %q", out, in)
	}
}

func TestDefaultRegistryHasNoResponseTransformerForGeminiRequestToOpenAIResponses(t *testing.T) {
	if translator.Default().HasResponseTransformer(
		translator.FormatGeminiRequest,
		translator.FormatOpenAIResponses,
	) {
		t.Error("response-side transformer should be nil for an identity registration")
	}
}
