package signature

import (
	"encoding/base64"
	"testing"
)

// buildValidClaudeSignature constructs the smallest payload that passes
// non-strict validation: the single-layer base64 encoding of a payload
// whose first byte is the 0x12 Claude protobuf marker. The remaining
// bytes are arbitrary (strict mode walks the protobuf tree; the tests
// here exercise non-strict validation, which only checks the marker).
func buildValidClaudeSingleLayerE(t *testing.T) string {
	t.Helper()
	// Minimal payload starting with 0x12.
	payload := []byte{0x12, 0x00}
	sig := base64.StdEncoding.EncodeToString(payload)
	if sig[0] != 'E' {
		t.Fatalf("expected single-layer E-form, got %q", sig)
	}
	return sig
}

func buildValidClaudeDoubleLayerR(t *testing.T) string {
	t.Helper()
	single := buildValidClaudeSingleLayerE(t)
	double := base64.StdEncoding.EncodeToString([]byte(single))
	if double[0] != 'R' {
		t.Fatalf("expected double-layer R-form, got %q", double)
	}
	return double
}

func TestIsValidClaudeThinkingSignature_EmptyIsInvalid(t *testing.T) {
	if IsValidClaudeThinkingSignature("") {
		t.Fatal("empty signature must be invalid")
	}
}

func TestIsValidClaudeThinkingSignature_BadPrefixIsInvalid(t *testing.T) {
	if IsValidClaudeThinkingSignature("ZZZbogus") {
		t.Fatal("non E/R prefix must be invalid")
	}
}

func TestIsValidClaudeThinkingSignature_SingleLayerE(t *testing.T) {
	sig := buildValidClaudeSingleLayerE(t)
	if !IsValidClaudeThinkingSignature(sig) {
		t.Fatalf("E-form signature %q should be valid", sig)
	}
}

func TestIsValidClaudeThinkingSignature_DoubleLayerR(t *testing.T) {
	sig := buildValidClaudeDoubleLayerR(t)
	if !IsValidClaudeThinkingSignature(sig) {
		t.Fatalf("R-form signature %q should be valid", sig)
	}
}

func TestIsValidClaudeThinkingSignature_StripsCachePrefix(t *testing.T) {
	sig := "modelGroup#" + buildValidClaudeSingleLayerE(t)
	if !IsValidClaudeThinkingSignature(sig) {
		t.Fatal("cache-prefixed signature should be valid")
	}
}

func TestIsValidClaudeThinkingSignature_PrefixOnlyOption(t *testing.T) {
	if !IsValidClaudeThinkingSignature("Egarbagebase64", ClaudeSignatureValidationOptions{PrefixOnly: true}) {
		t.Fatal("PrefixOnly should accept any E-prefixed string")
	}
	if IsValidClaudeThinkingSignature("Xbogus", ClaudeSignatureValidationOptions{PrefixOnly: true}) {
		t.Fatal("PrefixOnly should reject non E/R prefix")
	}
}

func TestStripInvalidClaudeThinkingBlocks_DropsForgedThinking(t *testing.T) {
	msgs := []map[string]any{
		{
			"role": "assistant",
			"content": []map[string]any{
				{"type": "text", "text": "hi"},
				{"type": "thinking", "thinking": "x", "signature": "FORGED-NOT-A-SIG"},
				{"type": "text", "text": "ok"},
			},
		},
	}
	out := StripInvalidClaudeThinkingBlocks(msgs)
	parts, ok := out[0]["content"].([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any content, got %T", out[0]["content"])
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts after strip, got %d: %+v", len(parts), parts)
	}
	for _, p := range parts {
		if p["type"] == "thinking" {
			t.Fatalf("forged thinking block survived strip: %+v", p)
		}
	}
}

func TestStripInvalidClaudeThinkingBlocks_PreservesValidThinking(t *testing.T) {
	good := buildValidClaudeSingleLayerE(t)
	msgs := []map[string]any{
		{
			"role": "assistant",
			"content": []map[string]any{
				{"type": "thinking", "thinking": "x", "signature": good},
				{"type": "text", "text": "hello"},
			},
		},
	}
	out := StripInvalidClaudeThinkingBlocks(msgs)
	parts := out[0]["content"].([]map[string]any)
	if len(parts) != 2 {
		t.Fatalf("expected both blocks preserved, got %d", len(parts))
	}
}

func TestStripInvalidClaudeThinkingBlocks_AnySliceContent(t *testing.T) {
	good := buildValidClaudeSingleLayerE(t)
	msgs := []map[string]any{
		{
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "thinking", "signature": "BAD"},
				map[string]any{"type": "thinking", "signature": good},
				map[string]any{"type": "text", "text": "tail"},
			},
		},
	}
	out := StripInvalidClaudeThinkingBlocks(msgs)
	parts, ok := out[0]["content"].([]any)
	if !ok {
		t.Fatalf("expected []any content, got %T", out[0]["content"])
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts after strip, got %d", len(parts))
	}
}

func TestStripInvalidClaudeThinkingBlocks_PreservesEmptyPlaceholderWhenOpt(t *testing.T) {
	msgs := []map[string]any{
		{
			"role": "assistant",
			"content": []map[string]any{
				{"type": "thinking", "thinking": "", "signature": ""},
			},
		},
	}
	out := StripInvalidClaudeThinkingBlocks(msgs, ClaudeSignatureValidationOptions{AllowEmptySignatureWithEmptyText: true})
	parts := out[0]["content"].([]map[string]any)
	if len(parts) != 1 {
		t.Fatalf("empty placeholder should be preserved when opt set, got %d", len(parts))
	}
}

func TestStripInvalidClaudeThinkingBlocksAndEmptyMessages_DropsEmpty(t *testing.T) {
	msgs := []map[string]any{
		{
			"role": "assistant",
			"content": []map[string]any{
				{"type": "thinking", "signature": "BAD"},
			},
		},
		{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "hi"},
			},
		},
	}
	out := StripInvalidClaudeThinkingBlocksAndEmptyMessages(msgs)
	if len(out) != 1 {
		t.Fatalf("expected 1 message after drop, got %d", len(out))
	}
	if out[0]["role"] != "user" {
		t.Fatalf("expected the user message to survive, got %+v", out[0])
	}
}

func TestStripInvalidClaudeThinkingBlocks_NilSafe(t *testing.T) {
	if got := StripInvalidClaudeThinkingBlocks(nil); got != nil {
		t.Fatalf("nil input should pass through, got %+v", got)
	}
	msgs := []map[string]any{nil, {"role": "user"}}
	got := StripInvalidClaudeThinkingBlocks(msgs)
	if len(got) != 2 {
		t.Fatalf("nil-safe traversal should preserve length, got %d", len(got))
	}
}
