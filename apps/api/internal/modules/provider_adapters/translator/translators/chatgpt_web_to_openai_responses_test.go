package translators

import (
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/translator"
)

// The chatgpt_web → openai_responses translator is an explicit
// identity (see file comment for rationale —
// chatGPTWebConversationPayload + chatGPTWebMultimodalPayloadBytes
// own the canonical envelope synthesis inline; the PR-3 wiring keeps
// transport-layer concerns out of the translator). These tests pin the
// identity behaviour byte-for-byte.

func TestChatGPTWebToOpenAIResponsesRewriterIdentityOnValidJSON(t *testing.T) {
	in := []byte(`{"action":"next","messages":[{"id":"x","author":{"role":"user"},"content":{"content_type":"text","parts":["hi"]}}],"model":"gpt-4o","parent_message_id":"p"}`)
	out := chatGPTWebToOpenAIResponsesRewriter("gpt-4o", in, false)
	if string(out) != string(in) {
		t.Errorf("identity rewriter changed output:\n got  %s\n want %s", out, in)
	}
}

func TestChatGPTWebToOpenAIResponsesRewriterFallsThroughOnNilEmpty(t *testing.T) {
	if got := chatGPTWebToOpenAIResponsesRewriter("m", nil, false); got != nil {
		t.Errorf("expected nil input to fall through to nil, got %q", got)
	}
	if got := chatGPTWebToOpenAIResponsesRewriter("m", []byte{}, false); len(got) != 0 {
		t.Errorf("expected empty input to fall through to empty, got %q", got)
	}
}

func TestChatGPTWebToOpenAIResponsesRewriterFallsThroughOnMalformedJSON(t *testing.T) {
	in := []byte(`!@#$`)
	out := chatGPTWebToOpenAIResponsesRewriter("m", in, false)
	if string(out) != string(in) {
		t.Errorf("expected fallthrough on bad JSON, got %q", out)
	}
}

func TestDefaultRegistryHasIdentityForChatGPTWebToOpenAIResponses(t *testing.T) {
	r := translator.Default()
	in := []byte(`{"action":"next","messages":[],"model":"gpt-4o","parent_message_id":"p","conversation_mode":{"kind":"primary_assistant"}}`)
	out := r.TranslateRequest(
		translator.FormatChatGPTWeb,
		translator.FormatOpenAIResponses,
		"gpt-4o", in, true,
	)
	if string(out) != string(in) {
		t.Errorf("registry-mediated identity changed output: %q != %q", out, in)
	}
}

func TestDefaultRegistryHasNoResponseTransformerForChatGPTWebToOpenAIResponses(t *testing.T) {
	if translator.Default().HasResponseTransformer(
		translator.FormatChatGPTWeb,
		translator.FormatOpenAIResponses,
	) {
		t.Error("response-side transformer should be nil for an identity registration")
	}
}
